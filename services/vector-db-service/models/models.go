package models

type AddDocumentsRequest struct {
	BotID      string              `json:"bot_id"` // Changed from client_id to bot_id
	Texts      []string            `json:"texts"`
	Embeddings [][]float32         `json:"embeddings"`
	Metadata   []map[string]string `json:"metadata"`
}

type SearchRequest struct {
	BotID          string    `json:"bot_id"` // Changed from client_id to bot_id
	QueryEmbedding []float32 `json:"query_embedding"`
	Limit          int       `json:"limit"`
}

type EnsureCollectionRequest struct {
	BotID string `json:"bot_id"` // Changed from client_id to bot_id
}

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type StatsResponse struct {
	Success        bool   `json:"success"`
	BotID          string `json:"bot_id"` // Changed from client_id
	TotalDocuments int    `json:"total_documents"`
}
