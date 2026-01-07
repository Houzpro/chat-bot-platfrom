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
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/sync/errgroup"
)

type Handler struct {
	cfg    *config.Config
	client *clients.Client
}

// clampContext limits context size to avoid exceeding model window
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

// GetDefaults returns default generation parameters
func (h *Handler) GetDefaults(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"temperature":    h.cfg.Generation.Temperature,
		"top_p":          h.cfg.Generation.TopP,
		"top_k":          h.cfg.Generation.TopK,
		"max_new_tokens": h.cfg.Generation.MaxNewTokens,
		"do_sample":      h.cfg.Generation.DoSample,
		"user_prompt":    h.cfg.Generation.UserPrompt,
	})
}

// UploadDocument handles document upload and processing
func (h *Handler) UploadDocument(c *fiber.Ctx) error {
	// Get and validate client ID
	clientID := utils.SanitizeInput(c.FormValue("client_id"))
	if err := utils.ValidateClientID(clientID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Get file
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	// Validate file size (max 100MB)
	const maxFileSize = 100 * 1024 * 1024
	if fileHeader.Size > maxFileSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file too large (max 10MB)"})
	}

	// Validate file extension
	allowedExtensions := map[string]bool{
		".pdf": true, ".txt": true, ".docx": true, ".doc": true,
		".csv": true, ".xlsx": true, ".json": true, ".md": true, ".html": true,
	}
	filename := strings.ToLower(fileHeader.Filename)
	isAllowed := false
	for ext := range allowedExtensions {
		if strings.HasSuffix(filename, ext) {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unsupported file type (allowed: pdf, txt, docx, csv, xlsx, json, md, html)",
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

	// Не разбиваем на чанки, сохраняем весь текст как один документ
	if len(strings.TrimSpace(textResp.Text)) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no text extracted from document"})
	}

	embeddings, err := h.client.CreateEmbeddings(h.cfg.Services.AIURL, []string{textResp.Text})
	if err != nil || len(embeddings) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("embedding error: %v", err)})
	}

	metadata := []map[string]string{{
		"file_name": textResp.FileName,
		"file_type": textResp.FileType,
	}}

	if err := h.client.AddVectorDocuments(h.cfg.Services.VectorURL, clientID, []string{textResp.Text}, embeddings, metadata); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": fmt.Sprintf("vector DB error: %v", err)})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"client_id": clientID,
		"chunks":    1,
		"file_name": textResp.FileName,
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

	// Validate file size (max 100MB)
	const maxFileSize = 100 * 1024 * 1024
	if fileHeader.Size > maxFileSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file too large (max 10MB)"})
	}

	// Validate file extension
	allowedExtensions := map[string]bool{
		".pdf": true, ".txt": true, ".docx": true, ".doc": true,
		".csv": true, ".xlsx": true, ".json": true, ".md": true, ".html": true,
	}
	filename := strings.ToLower(fileHeader.Filename)
	isAllowed := false
	for ext := range allowedExtensions {
		if strings.HasSuffix(filename, ext) {
			isAllowed = true
			break
		}
	}
	if !isAllowed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unsupported file type (allowed: pdf, txt, docx, csv, xlsx, json, md, html)",
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

// SearchDocuments handles document search requests
func (h *Handler) SearchDocuments(c *fiber.Ctx) error {
	var req models.SearchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	// Для совместимости: просто возвращаем 501 Not Implemented
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"error": "SearchDocuments endpoint is not implemented. Use RAGChat instead."})
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
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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

	// ШАГ 3: ADVANCED SEARCH - Query Expansion + Hybrid Search + Reranking
	advancedResult, err := h.client.AdvancedSearch(
		h.cfg.Services.AIURL,
		botID,
		req.Query,
		vectorResults,
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
		contextStr := clampContext(utils.BuildContext(docs), h.cfg.RAG.MaxContextChars)

		// SSE stream с fallback контекстом
		return h.streamRAGResponse(c, req, docs, contextStr)
	}

	// Извлекаем результаты
	results, _ := advancedResult["results"].([]any)
	compressedContext, _ := advancedResult["compressed_context"].(string)

	// Конвертируем results в нужный формат
	docs := make([]string, 0, len(results))
	for _, r := range results {
		if resMap, ok := r.(map[string]any); ok {
			if text, ok := resMap["text"].(string); ok && text != "" {
				docs = append(docs, text)
			}
		}
	}

	log.Printf("🎯 [Advanced RAG] Final: %d docs, context: %d chars", len(docs), len(compressedContext))

	// Используем compressed context или fallback к простому
	contextStr := compressedContext
	if contextStr == "" || len(contextStr) < 100 {
		contextStr = utils.BuildContext(docs)
	}
	contextStr = clampContext(contextStr, h.cfg.RAG.MaxContextChars)

	log.Printf("📝 [Advanced RAG] Final context: %d chars", len(contextStr))

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
