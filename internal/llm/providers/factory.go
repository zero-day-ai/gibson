package providers

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/llm"
)

// NewProvider creates a new LLM provider based on the configuration
func NewProvider(cfg llm.ProviderConfig) (llm.LLMProvider, error) {
	switch cfg.Type {
	case llm.ProviderAnthropic:
		return NewAnthropicProvider(cfg)

	case llm.ProviderOpenAI:
		return NewOpenAIProvider(cfg)

	case llm.ProviderGoogle:
		return NewGoogleProvider(cfg)

	case "ollama":
		return NewOllamaProvider(cfg)

	case "mock":
		// For mock provider
		return NewMockProvider([]string{"Mock response"}), nil

	default:
		return nil, llm.NewInvalidInputError("factory", fmt.Sprintf("unknown provider type: %s", cfg.Type))
	}
}
