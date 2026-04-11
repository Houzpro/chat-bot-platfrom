package handlers

import (
	"backend/clients"
	"backend/config"
	"backend/models"
	"backend/utils"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/sync/errgroup"
)

type Handler struct {
	cfg    *config.Config
	client *clients.Client
}

// clampContext limits context size to avoid exceeding model window.
// Legacy string-level trim — kept for the fallback path where we only have a
// pre-built string and no per-document ordering. Prefer buildClampedContext
// when you still have the docs slice in relevance order.
func clampContext(contextStr string, maxChars int) string {
	limit := maxChars
	if limit <= 0 {
		limit = 16000
	}
	if len(contextStr) > limit {
		return contextStr[:limit]
	}
	return contextStr
}

// buildClampedContext assembles a context string from docs in relevance order,
// adding whole chunks one by one until the next chunk would exceed maxChars.
// This guarantees the top-ranked document survives truncation — unlike the old
// approach of clamping a pre-joined compressed_context from the tail, which
// could drop the single most relevant chunk if it physically sat near the end
// of the joined string. Returns the assembled context and the number of docs
// that actually fit.
func buildClampedContext(docs []string, maxChars int) (string, int) {
	limit := maxChars
	if limit <= 0 {
		limit = 16000
	}
	if len(docs) == 0 {
		return "", 0
	}

	var b strings.Builder
	kept := 0
	for i, d := range docs {
		if d == "" {
			continue
		}
		// "Document N:\n" + text + "\n\n" between docs
		header := fmt.Sprintf("Document %d:\n", i+1)
		sep := ""
		if b.Len() > 0 {
			sep = "\n\n"
		}
		addLen := len(sep) + len(header) + len(d)
		if b.Len()+addLen > limit {
			// If nothing has been added yet, at least include a truncated first doc
			// so we never return an empty context for a non-empty docs slice.
			if b.Len() == 0 {
				room := limit - len(header)
				if room <= 0 {
					return d[:min(len(d), limit)], 1
				}
				b.WriteString(header)
				b.WriteString(d[:min(len(d), room)])
				kept = 1
			}
			break
		}
		b.WriteString(sep)
		b.WriteString(header)
		b.WriteString(d)
		kept++
	}
	return b.String(), kept
}

// normalizeBotID strips a leading "bot_" prefix if callers provide the collection-style ID.
// This keeps the bot UUID consistent across services and avoids double-prefix collection names.
func normalizeBotID(botID string) string {
	return strings.TrimPrefix(botID, "bot_")
}

func NewHandler(cfg *config.Config, client *clients.Client) *Handler {
	return &Handler{
		cfg:    cfg,
		client: client,
	}
}

// Health returns service health status
func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":     "ok",
		"service":    "backend-gateway",
		"doc_parser": h.cfg.Services.DocParserURL,
		"vector":     h.cfg.Services.VectorURL,
		"ai":         h.cfg.Services.AIURL,
	})
}

// GetDefaults returns default generation parameters along with the upper
// limit for max_new_tokens derived from GEN_MAX_NEW_TOKENS in env. The
// frontend uses max_new_tokens_limit as the ceiling for its input so that
// env remains the single source of truth.
func (h *Handler) GetDefaults(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"temperature":          h.cfg.Generation.Temperature,
		"top_p":                h.cfg.Generation.TopP,
		"top_k":                h.cfg.Generation.TopK,
		"max_new_tokens":       h.cfg.Generation.MaxNewTokens,
		"max_new_tokens_limit": h.cfg.Generation.MaxNewTokens,
		"do_sample":            h.cfg.Generation.DoSample,
		"user_prompt":          h.cfg.Generation.UserPrompt,
		// Upload limits — single source of truth for frontend validation.
		"max_file_size":       h.cfg.Upload.MaxFileSize,
		"allowed_extensions":  h.cfg.Upload.AllowedExtensions,
	})
}

// UploadDocumentForBot handles document upload for a specific bot (requires auth and ownership)
func (h *Handler) UploadDocumentForBot(c *fiber.Ctx) error {
	botID := normalizeBotID(c.Params("id"))
	log.Printf("[UploadDocumentForBot] Received bot_id from URL: %q", botID)

	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id is required"})
	}

	// Get file
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	// Validate file size from config
	if fileHeader.Size > h.cfg.Upload.MaxFileSize {
		maxMB := h.cfg.Upload.MaxFileSize / (1024 * 1024)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("file too large (max %dMB)", maxMB)})
	}

	// Validate file extension from config
	filename := strings.ToLower(fileHeader.Filename)
	isAllowed := false
	for _, ext := range h.cfg.Upload.AllowedExtensions {
		if strings.HasSuffix(filename, ext) {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("unsupported file type (allowed: %s)", strings.Join(h.cfg.Upload.AllowedExtensions, ", ")),
		})
	}

	// Open file
	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "cannot open file"})
	}
	defer file.Close()

	// Parse document
	textResp, err := h.client.ParseDocument(h.cfg.Services.DocParserURL, fileHeader.Filename, file)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("parse error: %v", err)})
	}

	if len(strings.TrimSpace(textResp.Text)) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no text extracted from document"})
	}

	// Split into semantic chunks via AI service (fallback to local chunking on error)
	var chunks []string
	chunks, err = h.client.SplitDocument(h.cfg.Services.AIURL, textResp.Text, h.cfg.RAG.ChunkSize, h.cfg.RAG.ChunkOverlap)
	if err != nil || len(chunks) == 0 {
		log.Printf("[UploadDocumentForBot] split-document failed: %v; falling back to simple chunking", err)
		chunks = utils.ChunkText(textResp.Text, h.cfg.RAG.ChunkSize, h.cfg.RAG.ChunkOverlap)
	}
	if len(chunks) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no chunks created from document"})
	}

	log.Printf("[UploadDocumentForBot] Creating embeddings for %d chunks from %s", len(chunks), textResp.FileName)
	embeddings, err := h.client.CreateEmbeddings(h.cfg.Services.AIURL, chunks)
	if err != nil || len(embeddings) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("embedding error: %v", err)})
	}

	if len(embeddings) != len(chunks) {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "embedding count mismatch"})
	}

	metadata := make([]map[string]string, len(chunks))
	for i := range chunks {
		metadata[i] = map[string]string{
			"file_name":   textResp.FileName,
			"file_type":   textResp.FileType,
			"chunk_index": fmt.Sprintf("%d", i),
		}
	}

	// Add to vector DB using bot_id
	log.Printf("[UploadDocumentForBot] Adding to vector DB with bot_id: %q, chunks: %d", botID, len(chunks))
	if err := h.client.AddVectorDocuments(h.cfg.Services.VectorURL, botID, chunks, embeddings, metadata); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("vector DB error: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"bot_id":    botID,
		"chunks":    len(chunks),
		"file_name": textResp.FileName,
	})
}

// RAGChat handles RAG-based chat requests with streaming
func (h *Handler) RAGChat(c *fiber.Ctx) error {
	var req models.RAGChatRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Validate and sanitize inputs
	req.ClientID = utils.SanitizeInput(req.ClientID)
	req.Query = utils.SanitizeInput(req.Query)
	req.SystemPrompt = utils.SanitizeInput(req.SystemPrompt)

	if err := utils.ValidateClientID(req.ClientID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := utils.ValidateQuery(req.Query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Set defaults and validate parameters
	req.SetDefaults(h.cfg.RAG.MaxResults, h.cfg.Generation)

	// Additional validation
	if req.Limit > 100 {
		req.Limit = 100
	}
	if req.Temperature > 2 {
		req.Temperature = 2
	}
	if req.TopP > 1 {
		req.TopP = 1
	}
	if req.TopK > 200 {
		req.TopK = 200
	}
	if req.MaxNewTokens > 8192 {
		req.MaxNewTokens = 8192
	}
	if len(req.SystemPrompt) > 2000 {
		req.SystemPrompt = req.SystemPrompt[:2000]
	}

	// Create context with timeout for async operations
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.RAG.ContextTimeout)
	defer cancel()

	// Execute embedding creation
	var embedding [][]float32
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		select {
		case <-gctx.Done():
			return gctx.Err()
		default:
		}

		emb, err := h.client.CreateQueryEmbeddings(h.cfg.Services.AIURL, []string{req.Query})
		if err != nil || len(emb) == 0 {
			return fmt.Errorf("failed to create query embedding: %w", err)
		}
		embedding = emb
		return nil
	})

	if err := g.Wait(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Search for relevant documents; fallback to full list if empty
	searchResults, err := h.client.SearchVectorDocuments(h.cfg.Services.VectorURL, req.ClientID, embedding[0], req.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("search error: %v", err)})
	}
	if len(searchResults) == 0 {
		fallback, listErr := h.client.ListVectorDocuments(h.cfg.Services.VectorURL, req.ClientID, 500)
		if listErr == nil {
			searchResults = fallback
		}
	}

	// Extract and build context
	snippetWindow := h.cfg.RAG.MaxDocChars / 2
	if snippetWindow < 800 {
		snippetWindow = 800
	}
	docs := utils.ExtractRelevantTexts(searchResults, req.Query, h.cfg.RAG.MaxDocChars, snippetWindow)
	contextStr := clampContext(utils.BuildContext(docs), h.cfg.RAG.MaxContextChars)

	// Setup SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("X-Accel-Buffering", "no") // Disable nginx buffering

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Send documents info first
		docsJSON, _ := json.Marshal(map[string]interface{}{
			"documents": docs,
		})
		fmt.Fprintf(w, "data: %s\n\n", docsJSON)
		w.Flush()

		// Prepare generation request
		systemPromptWithContext := fmt.Sprintf("%s\n\nContext:\n%s", req.SystemPrompt, contextStr)
		genReq := models.GenerateRequest{
			Messages:     []map[string]string{{"role": "user", "content": req.Query}},
			MaxNewTokens: req.MaxNewTokens,
			Temperature:  req.Temperature,
			TopP:         req.TopP,
			TopK:         req.TopK,
			DoSample:     req.DoSample,
			SystemPrompt: systemPromptWithContext,
		}

		// Call streaming generation
		resp, err := h.client.StreamGeneration(h.cfg.Services.AIURL, genReq)
		if err != nil {
			errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", errJSON)
			w.Flush()
			return
		}
		defer resp.Body.Close()

		// Stream response
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				fmt.Fprintf(w, "%s\n\n", line)
				w.Flush()
			}
		}

		// Send completion marker
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.Flush()
	})

	return nil
}

// PublicRAGChat handles public chat requests using ADVANCED SEARCH (90%+ accuracy)
func (h *Handler) PublicRAGChat(c *fiber.Ctx) error {
	botID := normalizeBotID(c.Params("bot_id"))
	var req models.RAGChatRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Поддержка передачи query/message через body
	if req.Query == "" && req.Message != "" {
		req.Query = req.Message
	}
	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query is required"})
	}

	// Подставляем bot_id
	req.ClientID = botID
	req.SetDefaults(h.cfg.RAG.MaxResults, h.cfg.Generation)

	// Валидация параметров
	if req.Limit > 100 {
		req.Limit = 100
	}
	if req.Temperature > 2 {
		req.Temperature = 2
	}
	if req.TopP > 1 {
		req.TopP = 1
	}
	if req.TopK > 200 {
		req.TopK = 200
	}
	if req.MaxNewTokens > 8192 {
		req.MaxNewTokens = 8192
	}
	if len(req.SystemPrompt) > 2000 {
		req.SystemPrompt = req.SystemPrompt[:2000]
	}

	log.Printf("🔍 [Advanced RAG] Bot: %s, Query: %s", botID, req.Query)

	// ШАГ 1: Создаём embedding для запроса
	embeddings, err := h.client.CreateQueryEmbeddings(h.cfg.Services.AIURL, []string{req.Query})
	if err != nil || len(embeddings) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "embedding error: " + err.Error()})
	}

	// ШАГ 2: Векторный поиск (initial candidates) - МАКСИМАЛЬНЫЙ охват
	searchLimit := h.cfg.RAG.MaxResults
	if searchLimit <= 0 {
		searchLimit = 60 // Увеличено до 60 для максимального покрытия
	}
	log.Printf("🔍 [Advanced RAG] Requesting %d vector candidates", searchLimit)

	vectorResults, err := h.client.SearchVectorDocuments(h.cfg.Services.VectorURL, botID, embeddings[0], searchLimit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "vector search error: " + err.Error()})
	}

	// Fallback если векторный поиск не дал результатов
	if len(vectorResults) == 0 {
		log.Printf("⚠️ [Advanced RAG] No vector results, using fallback")
		fallback, listErr := h.client.ListVectorDocuments(h.cfg.Services.VectorURL, botID, 100)
		if listErr == nil {
			vectorResults = fallback
		}
	}

	log.Printf("📊 [Advanced RAG] Vector search: %d initial candidates", len(vectorResults))

	// ШАГ 2.5: Получаем ВСЕ документы бота для Window Retrieval (поиск соседних чанков)
	allDocs, allDocsErr := h.client.ListVectorDocuments(h.cfg.Services.VectorURL, botID, 5000)
	if allDocsErr != nil {
		log.Printf("⚠️ [Advanced RAG] Failed to get all docs for window retrieval: %v", allDocsErr)
		allDocs = vectorResults // fallback: используем только vector results
	} else {
		log.Printf("📊 [Advanced RAG] Total docs for window retrieval: %d", len(allDocs))
	}

	// ШАГ 3: ADVANCED SEARCH - Query Expansion + Hybrid Search + Reranking
	advancedResult, err := h.client.AdvancedSearch(
		h.cfg.Services.AIURL,
		botID,
		req.Query,
		vectorResults,
		allDocs,
		35, // top_k после reranking (увеличено до 35 для полноты контекста)
		h.cfg.RAG.MaxContextChars,
	)
	if err != nil {
		log.Printf("⚠️ [Advanced RAG] Advanced search failed: %v, using fallback", err)
		// Fallback к простому подходу
		docs := make([]string, 0, len(vectorResults))
		for _, doc := range vectorResults {
			if text, ok := doc["text"].(string); ok && text != "" {
				docs = append(docs, text)
				if len(docs) >= 10 {
					break
				}
			}
		}
		contextStr, _ := buildClampedContext(docs, h.cfg.RAG.MaxContextChars)

		// SSE stream с fallback контекстом
		return h.streamRAGResponse(c, req, docs, contextStr)
	}

	// Извлекаем результаты
	results, _ := advancedResult["results"].([]any)
	compressedContext, _ := advancedResult["compressed_context"].(string)
	promptAddition, _ := advancedResult["prompt_addition"].(string)

	// Log pipeline trace if available
	if traceData, ok := advancedResult["trace"].(map[string]any); ok {
		if traceID, ok := traceData["trace_id"].(string); ok {
			log.Printf("📊 [Advanced RAG] Trace: %s", traceID)
		}
		if bestScore, ok := traceData["best_score"].(float64); ok {
			log.Printf("📊 [Advanced RAG] Best relevance score: %.2f", bestScore)
		}
	}
	if routerDecision, ok := advancedResult["router_decision"].(map[string]any); ok {
		queryType, _ := routerDecision["query_type"].(string)
		suggestedTool, _ := routerDecision["suggested_tool"].(string)
		log.Printf("🧭 [Advanced RAG] Router: type=%s, tool=%s", queryType, suggestedTool)
	}

	// Конвертируем results в нужный формат (порядок = порядок релевантности после reranking)
	docs := make([]string, 0, len(results))
	for _, r := range results {
		if resMap, ok := r.(map[string]any); ok {
			if text, ok := resMap["text"].(string); ok && text != "" {
				docs = append(docs, text)
			}
		}
	}
	log.Printf("🎯 [Advanced RAG] Final: %d docs from advanced search", len(docs))

	// Build context from docs in relevance order, adding whole chunks until
	// we hit MaxContextChars. This guarantees the top-ranked chunk is always
	// included — fixes a bug where the old path clamped a pre-joined
	// compressed_context from the tail and could drop the single most relevant
	// chunk if it happened to sit near the end of the joined string.
	// compressed_context from python-ai is intentionally ignored here because
	// it is ordered by document position, not by relevance.
	_ = compressedContext
	contextStr, kept := buildClampedContext(docs, h.cfg.RAG.MaxContextChars)
	log.Printf("📝 [Advanced RAG] Context assembled: %d/%d docs fit, %d chars (limit %d)",
		kept, len(docs), len(contextStr), h.cfg.RAG.MaxContextChars)

	// Prepend prompt_addition (enumeration/global/multi_hop instructions) to system prompt
	if promptAddition != "" {
		req.SystemPrompt = promptAddition + "\n\n" + req.SystemPrompt
	}

	return h.streamRAGResponse(c, req, docs, contextStr)
}

// streamRAGResponse handles SSE streaming for RAG responses
func (h *Handler) streamRAGResponse(c *fiber.Ctx, req models.RAGChatRequest, docs []string, contextStr string) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Отправляем документы
		docsJSON, _ := json.Marshal(map[string]interface{}{"documents": docs})
		fmt.Fprintf(w, "data: %s\n\n", docsJSON)
		w.Flush()

		// Формируем system prompt с контекстом
		systemPromptWithContext := req.SystemPrompt + "\n\nContext:\n" + contextStr

		genReq := models.GenerateRequest{
			Messages:     []map[string]string{{"role": "user", "content": req.Query}},
			MaxNewTokens: req.MaxNewTokens,
			Temperature:  req.Temperature,
			TopP:         req.TopP,
			TopK:         req.TopK,
			DoSample:     req.DoSample,
			SystemPrompt: systemPromptWithContext,
		}

		resp, err := h.client.StreamGeneration(h.cfg.Services.AIURL, genReq)
		if err != nil {
			errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
			fmt.Fprintf(w, "data: %s\n\n", errJSON)
			w.Flush()
			return
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				fmt.Fprintf(w, "%s\n\n", line)
				w.Flush()
			}
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.Flush()
	})

	return nil
}
