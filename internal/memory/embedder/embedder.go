package embedder

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/types"
)

// Embedder generates embedding vectors from text content.
// Implementations must be thread-safe for concurrent access.
type Embedder interface {
	// Embed generates an embedding vector for a single text.
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch generates embeddings for multiple texts efficiently.
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

	// Dimensions returns the dimensionality of embedding vectors.
	Dimensions() int

	// Model returns the name of the embedding model being used.
	Model() string

	// Health returns the health status of the embedder.
	Health(ctx context.Context) types.HealthStatus
}

// EmbedderConfig holds configuration for embedding providers.
type EmbedderConfig struct {
	// Provider specifies which embedder implementation to use.
	// Options: "openai", "llm", "mock"
	Provider string `yaml:"provider" json:"provider" mapstructure:"provider"`

	// Model is the specific embedding model to use.
	// For OpenAI: "text-embedding-3-small" (1536 dims) or "text-embedding-3-large" (3072 dims)
	Model string `yaml:"model" json:"model" mapstructure:"model"`

	// APIKey is the API key for the embedding provider.
	// Can also be provided via environment variable (e.g., OPENAI_API_KEY)
	APIKey string `yaml:"api_key" json:"api_key" mapstructure:"api_key"`

	// BaseURL is the base URL for the embedding API.
	// For OpenAI, this defaults to "https://api.openai.com/v1"
	BaseURL string `yaml:"base_url" json:"base_url" mapstructure:"base_url"`

	// MaxRetries is the maximum number of retry attempts for transient failures.
	MaxRetries int `yaml:"max_retries" json:"max_retries" mapstructure:"max_retries"`

	// Timeout is the request timeout in seconds.
	Timeout int `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
}

// Validate checks if the EmbedderConfig is valid.
func (c *EmbedderConfig) Validate() error {
	if c.Provider == "" {
		return types.NewError(ErrCodeInvalidConfig, "embedder provider cannot be empty")
	}

	if c.Model == "" {
		return types.NewError(ErrCodeInvalidConfig, "embedder model cannot be empty")
	}

	// OpenAI provider requires API key
	if c.Provider == "openai" && c.APIKey == "" {
		return types.NewError(ErrCodeInvalidConfig,
			"OpenAI embedder requires api_key (or OPENAI_API_KEY environment variable)")
	}

	if c.MaxRetries < 0 {
		return types.NewError(ErrCodeInvalidConfig, "max_retries must be non-negative")
	}

	if c.Timeout < 0 {
		return types.NewError(ErrCodeInvalidConfig, "timeout must be non-negative")
	}

	return nil
}

// DefaultEmbedderConfig returns a default configuration for OpenAI embedder.
func DefaultEmbedderConfig() EmbedderConfig {
	return EmbedderConfig{
		Provider:   "openai",
		Model:      "text-embedding-3-small",
		APIKey:     "", // Must be provided via config or env var
		BaseURL:    "https://api.openai.com/v1",
		MaxRetries: 3,
		Timeout:    30,
	}
}
