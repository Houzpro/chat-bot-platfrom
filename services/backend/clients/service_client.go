package clients

import (
	"backend/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// Client handles external service communication
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new service client
func NewClient(httpClient *http.Client) *Client {
	return &Client{
		httpClient: httpClient,
	}
}

// ParseDocument calls the document parser service
func (c *Client) ParseDocument(url, filename string, reader io.Reader) (*models.ParseResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, reader); err != nil {
		return nil, fmt.Errorf("copy file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(url, "/")+"/parse", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("parser service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed models.ParseResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &parsed, nil
}

// CreateEmbeddings calls the AI service to create passage/document embeddings.
func (c *Client) CreateEmbeddings(aiURL string, texts []string) ([][]float32, error) {
	return c.createEmbeddings(aiURL, texts, false)
}

// CreateQueryEmbeddings calls the AI service with query mode enabled (adds query prefix for e5 models).
func (c *Client) CreateQueryEmbeddings(aiURL string, texts []string) ([][]float32, error) {
	return c.createEmbeddings(aiURL, texts, true)
}

func (c *Client) createEmbeddings(aiURL string, texts []string, isQuery bool) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts array is empty")
	}

	reqBody, err := json.Marshal(models.EmbeddingsRequest{Texts: texts, IsQuery: isQuery})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(aiURL, "/")+"/embeddings",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var out models.EmbeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(out.Embeddings) == 0 {
		return nil, fmt.Errorf("received empty embeddings")
	}

	return out.Embeddings, nil
}

// SplitDocument calls the AI service for semantic chunking
func (c *Client) SplitDocument(aiURL string, text string, chunkSize, overlap int) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text is empty")
	}

	reqBody, err := json.Marshal(models.SplitDocumentRequest{
		Text:      text,
		ChunkSize: chunkSize,
		Overlap:   overlap,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(aiURL, "/")+"/split-document",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var out models.SplitDocumentResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(out.Chunks) == 0 {
		return nil, fmt.Errorf("split-document returned no chunks")
	}

	return out.Chunks, nil
}

// AddVectorDocuments adds documents to the vector database
func (c *Client) AddVectorDocuments(vectorURL, clientID string, texts []string, embeddings [][]float32, metadata []map[string]string) error {
	if len(texts) != len(embeddings) {
		return fmt.Errorf("texts and embeddings length mismatch: %d vs %d", len(texts), len(embeddings))
	}

	reqBody, err := json.Marshal(models.VectorAddRequest{
		BotID:      clientID,
		Texts:      texts,
		Embeddings: embeddings,
		Metadata:   metadata,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(vectorURL, "/")+"/documents/add",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vector service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SearchVectorDocuments searches for similar documents in the vector database
func (c *Client) SearchVectorDocuments(vectorURL, clientID string, queryEmbedding []float32, limit int) ([]map[string]any, error) {
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}

	reqBody, err := json.Marshal(models.VectorSearchRequest{
		BotID:          clientID,
		QueryEmbedding: queryEmbedding,
		Limit:          limit,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(vectorURL, "/")+"/documents/search",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vector service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var out models.VectorSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !out.Success {
		return nil, fmt.Errorf("vector search failed: %s", out.Error)
	}

	rawDocs, ok := out.Data["documents"].([]any)
	if !ok {
		return []map[string]any{}, nil
	}

	docs := make([]map[string]any, 0, len(rawDocs))
	for _, d := range rawDocs {
		if m, ok := d.(map[string]any); ok {
			docs = append(docs, m)
		}
	}

	return docs, nil
}

// ListVectorDocuments fetches documents without similarity filtering (fallback)
func (c *Client) ListVectorDocuments(vectorURL, clientID string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 100
	}
	url := fmt.Sprintf("%s/documents/list/%s?limit=%d", strings.TrimRight(vectorURL, "/"), clientID, limit)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vector service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var out models.VectorSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !out.Success {
		return nil, fmt.Errorf("vector list failed: %s", out.Error)
	}

	documentsRaw, ok := out.Data["documents"].([]any)
	if !ok {
		return []map[string]any{}, nil
	}

	docs := make([]map[string]any, 0, len(documentsRaw))
	for _, d := range documentsRaw {
		if m, ok := d.(map[string]any); ok {
			docs = append(docs, m)
		}
	}

	return docs, nil
}

// StreamGeneration creates a streaming HTTP request to the AI service
func (c *Client) StreamGeneration(aiURL string, req models.GenerateRequest) (*http.Response, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(aiURL, "/")+"/ask",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("AI service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return resp, nil
}

// AdvancedSearch calls the AI service for advanced RAG search with reranking
func (c *Client) AdvancedSearch(aiURL, botID, query string, vectorResults []map[string]any, topK int, maxContextChars int) (map[string]any, error) {
	reqBody, err := json.Marshal(map[string]any{
		"bot_id":            botID,
		"query":             query,
		"vector_results":    vectorResults,
		"top_k":             topK,
		"max_context_chars": maxContextChars,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(aiURL, "/")+"/advanced-search",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// BuildBM25Index calls the AI service to build BM25 index for a bot
func (c *Client) BuildBM25Index(aiURL, botID string, documents []map[string]any) error {
	reqBody, err := json.Marshal(map[string]any{
		"bot_id":    botID,
		"documents": documents,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		strings.TrimRight(aiURL, "/")+"/build-bm25-index",
		"application/json",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("AI service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
