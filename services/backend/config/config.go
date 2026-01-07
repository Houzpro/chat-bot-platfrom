package config

import (
	"backend/models"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server     ServerConfig
	Services   ServicesConfig
	RAG        RAGConfig
	HTTPClient HTTPClientConfig
	Generation models.GenerationDefaults
}

type ServerConfig struct {
	Port string
}

type ServicesConfig struct {
	DocParserURL string
	VectorURL    string
	AIURL        string
}

type RAGConfig struct {
	ChunkSize       int
	ChunkOverlap    int
	MaxDocChars     int
	MaxContextChars int
	MaxResults      int
	ScoreThreshold  float64
}

type HTTPClientConfig struct {
	Timeout time.Duration
}

// Load loads configuration from environment variables with validation
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("PORT", ""),
		},
		Services: ServicesConfig{
			DocParserURL: getEnv("DOC_PARSER_URL", ""),
			VectorURL:    getEnv("VECTOR_URL", ""),
			AIURL:        getEnv("AI_URL", ""),
		},
		RAG: RAGConfig{
			ChunkSize:       getEnvInt("CHUNK_SIZE", 0),
			ChunkOverlap:    getEnvInt("CHUNK_OVERLAP", 0),
			MaxDocChars:     getEnvInt("RAG_MAX_DOC_CHARS", 0),
			MaxContextChars: getEnvInt("RAG_MAX_CONTEXT_CHARS", 16000),
			MaxResults:      getEnvInt("RAG_MAX_RESULTS", 100),
			ScoreThreshold:  getEnvFloat("RAG_SCORE_THRESHOLD", 0.5),
		},
		HTTPClient: HTTPClientConfig{
			Timeout: time.Duration(getEnvInt("HTTP_TIMEOUT_SEC", 0)) * time.Second,
		},
		Generation: models.GenerationDefaults{
			MaxNewTokens: getEnvInt("GEN_MAX_NEW_TOKENS", 0),
			Temperature:  getEnvFloat("GEN_TEMPERATURE", 0),
			TopP:         getEnvFloat("GEN_TOP_P", 0),
			TopK:         getEnvInt("GEN_TOP_K", 0),
			DoSample:     getEnvBool("GEN_DO_SAMPLE", false),
			SystemBase:   getEnv("GEN_SYSTEM_BASE_PROMPT", ""),
			UserPrompt:   getEnv("GEN_USER_PROMPT", ""),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("PORT cannot be empty")
	}
	if c.Services.DocParserURL == "" {
		return fmt.Errorf("DOC_PARSER_URL cannot be empty")
	}
	if c.Services.VectorURL == "" {
		return fmt.Errorf("VECTOR_URL cannot be empty")
	}
	if c.Services.AIURL == "" {
		return fmt.Errorf("AI_URL cannot be empty")
	}
	if c.RAG.ChunkSize <= 0 {
		return fmt.Errorf("CHUNK_SIZE must be positive")
	}
	if c.RAG.ChunkOverlap < 0 {
		return fmt.Errorf("CHUNK_OVERLAP cannot be negative")
	}
	if c.RAG.MaxResults <= 0 {
		return fmt.Errorf("RAG_MAX_RESULTS must be positive")
	}
	if c.RAG.MaxContextChars <= 0 {
		return fmt.Errorf("RAG_MAX_CONTEXT_CHARS must be positive")
	}
	if c.HTTPClient.Timeout <= 0 {
		return fmt.Errorf("HTTP_TIMEOUT_SEC must be positive")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		fmt.Fprintf(os.Stderr, "WARNING: Invalid integer value for %s: %s, using default: %d\n", key, value, defaultValue)
	}
	if defaultValue == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Environment variable %s is required but not set\n", key)
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
