package models

// ParseResponse represents the response from the document parser service
type ParseResponse struct {
	Text     string `json:"text"`
	FileName string `json:"file_name"`
	FileType string `json:"file_type"`
	Size     int64  `json:"size"`
}

// EmbeddingsRequest represents a request for text embeddings
type EmbeddingsRequest struct {
	Texts   []string `json:"texts"`
	IsQuery bool     `json:"is_query"`
}

// EmbeddingsResponse represents the response containing embeddings
type EmbeddingsResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// SplitDocumentRequest represents a request for semantic document splitting
type SplitDocumentRequest struct {
	Text      string `json:"text"`
	ChunkSize int    `json:"chunk_size"`
	Overlap   int    `json:"overlap"`
}

// SplitDocumentResponse represents a response with semantic chunks
type SplitDocumentResponse struct {
	Chunks      []string `json:"chunks"`
	NumChunks   int      `json:"num_chunks"`
	TotalChars  int      `json:"total_chars"`
	AvgChunkLen int      `json:"avg_chunk_size"`
}

// GenerateRequest represents a request for text generation
type GenerateRequest struct {
	Messages     []map[string]string `json:"messages"`
	MaxNewTokens int                 `json:"max_new_tokens"`
	Temperature  float64             `json:"temperature"`
	TopP         float64             `json:"top_p"`
	TopK         int                 `json:"top_k"`
	DoSample     bool                `json:"do_sample"`
	SystemPrompt string              `json:"system_prompt"`
}

// GenerateResponse represents the response from text generation
type GenerateResponse struct {
	Text string `json:"text"`
}

// VectorAddRequest represents a request to add documents to vector DB
type VectorAddRequest struct {
	BotID      string              `json:"bot_id"`
	Texts      []string            `json:"texts"`
	Embeddings [][]float32         `json:"embeddings"`
	Metadata   []map[string]string `json:"metadata"`
}

// VectorSearchRequest represents a vector search request
type VectorSearchRequest struct {
	BotID          string    `json:"bot_id"`
	QueryEmbedding []float32 `json:"query_embedding"`
	Limit          int       `json:"limit"`
}

// VectorSearchResponse represents the response from vector search
type VectorSearchResponse struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data"`
	Error   string         `json:"error"`
}

type VectorListResponse struct {
	Success   bool             `json:"success"`
	Error     string           `json:"error,omitempty"`
	Documents []map[string]any `json:"documents,omitempty"`
}

// UploadRequest represents a document upload request
type UploadRequest struct {
	ClientID string `form:"client_id" validate:"required"`
}

// SearchRequest represents a document search request
type SearchRequest struct {
	ClientID string `json:"client_id" validate:"required"`
	Query    string `json:"query" validate:"required"`
	Limit    int    `json:"limit" validate:"omitempty,gte=1,lte=100"`
}

// RAGChatRequest represents a RAG chat request with model parameters
type RAGChatRequest struct {
	ClientID     string  `json:"client_id" validate:"required"`
	Query        string  `json:"query" validate:"required"`
	Message      string  `json:"message"` // Alternative field name for query
	Limit        int     `json:"limit" validate:"omitempty,gte=1,lte=100"`
	Temperature  float64 `json:"temperature" validate:"omitempty,gte=0,lte=2"`
	TopP         float64 `json:"top_p" validate:"omitempty,gte=0,lte=1"`
	TopK         int     `json:"top_k" validate:"omitempty,gte=1,lte=200"`
	MaxNewTokens int     `json:"max_new_tokens" validate:"omitempty,gte=1,lte=4096"`
	DoSample     bool    `json:"do_sample"`
	SystemPrompt string  `json:"system_prompt" validate:"omitempty,max=2000"`
}

// GenerationDefaults holds default generation parameters
type GenerationDefaults struct {
	MaxNewTokens int
	Temperature  float64
	TopP         float64
	TopK         int
	DoSample     bool
	SystemBase   string
	UserPrompt   string
}

// SetDefaults sets default values for optional RAG parameters from config
func (r *RAGChatRequest) SetDefaults(maxResults int, genDefaults GenerationDefaults) {
	if r.Limit <= 0 {
		r.Limit = maxResults
	}
	if r.Temperature == 0 {
		r.Temperature = genDefaults.Temperature
	}
	if r.TopP == 0 {
		r.TopP = genDefaults.TopP
	}
	if r.TopK == 0 {
		r.TopK = genDefaults.TopK
	}
	if r.MaxNewTokens == 0 {
		r.MaxNewTokens = genDefaults.MaxNewTokens
	}
	if !r.DoSample {
		r.DoSample = genDefaults.DoSample
	}
	if r.SystemPrompt == "" {
		r.SystemPrompt = "You are a helpful assistant. /no_think"
	}
}
