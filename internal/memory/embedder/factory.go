package embedder

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/types"
)

// EmbedderType represents available embedder implementations.
type EmbedderType string

const (
	// EmbedderTypeNative uses all-MiniLM-L6-v2 for local offline embedding generation.
	// No API keys required, runs entirely on CPU with ONNX Runtime.
	// Produces 384-dimensional embeddings.
	EmbedderTypeNative EmbedderType = "native"

	// EmbedderTypeOpenAI uses OpenAI's embedding API (text-embedding-3-small/large).
	// Requires OPENAI_API_KEY environment variable.
	// Produces 1536 or 3072-dimensional embeddings depending on model.
	EmbedderTypeOpenAI EmbedderType = "openai"
)

// CreateEmbedder creates an embedder based on the provided configuration.
//
// Supported provider types:
//   - "native": all-MiniLM-L6-v2 (384 dims, offline, no API key) - DEFAULT
//   - "openai": OpenAI Embeddings API (1536+ dims, requires API key)
//
// Returns an error if embedder initialization fails. The daemon should fail fast
// if the embedder cannot be created - vector search is a core feature.
func CreateEmbedder(config EmbedderConfig) (Embedder, error) {
	switch EmbedderType(config.Provider) {
	case EmbedderTypeNative:
		return CreateNativeEmbedder()

	case EmbedderTypeOpenAI:
		// OpenAI embedder not yet implemented
		return nil, types.NewError(ErrCodeInvalidConfig,
			"OpenAI embedder not yet implemented - use 'native' provider")

	default:
		return nil, types.NewError(ErrCodeInvalidConfig,
			fmt.Sprintf("unknown embedder provider '%s' - must be 'native' or 'openai'",
				config.Provider))
	}
}

// ValidateEmbedderConfig validates an embedder configuration.
// Returns an error if the configuration is invalid or incomplete.
func ValidateEmbedderConfig(config EmbedderConfig) error {
	if config.Provider == "" {
		return types.NewError(ErrCodeInvalidConfig, "embedder provider cannot be empty")
	}

	switch EmbedderType(config.Provider) {
	case EmbedderTypeNative:
		// Native embedder has no additional config requirements
		return nil

	case EmbedderTypeOpenAI:
		// OpenAI embedder requires API key and model
		if config.APIKey == "" {
			return types.NewError(ErrCodeInvalidConfig,
				"OpenAI embedder requires api_key (or OPENAI_API_KEY environment variable)")
		}
		if config.Model == "" {
			return types.NewError(ErrCodeInvalidConfig,
				"OpenAI embedder requires model (e.g., 'text-embedding-3-small')")
		}
		return nil

	default:
		return types.NewError(ErrCodeInvalidConfig,
			fmt.Sprintf("unknown embedder provider '%s' - must be 'native' or 'openai'",
				config.Provider))
	}
}
