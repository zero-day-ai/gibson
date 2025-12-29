package providers

import (
	"context"
	"os"

	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

// AnthropicProvider implements LLMProvider for Anthropic's Claude models
type AnthropicProvider struct {
	client *anthropic.LLM
	config llm.ProviderConfig
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(cfg llm.ProviderConfig) (*AnthropicProvider, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	if apiKey == "" {
		return nil, llm.NewAuthError("anthropic", nil)
	}

	opts := []anthropic.Option{
		anthropic.WithToken(apiKey),
	}

	if cfg.DefaultModel != "" {
		opts = append(opts, anthropic.WithModel(cfg.DefaultModel))
	}

	client, err := anthropic.New(opts...)
	if err != nil {
		return nil, llm.TranslateError("anthropic", err)
	}

	return &AnthropicProvider{
		client: client,
		config: cfg,
	}, nil
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// Models returns information about available models
func (p *AnthropicProvider) Models(ctx context.Context) ([]llm.ModelInfo, error) {
	models := []llm.ModelInfo{
		{
			Name:          "claude-3-5-sonnet-20241022",
			ContextWindow: 200000,
			MaxOutput:     8192,
			Features:      []string{"chat", "streaming", "tools", "vision"},
		},
		{
			Name:          "claude-3-opus-20240229",
			ContextWindow: 200000,
			MaxOutput:     4096,
			Features:      []string{"chat", "streaming", "tools", "vision"},
		},
		{
			Name:          "claude-3-sonnet-20240229",
			ContextWindow: 200000,
			MaxOutput:     4096,
			Features:      []string{"chat", "streaming", "tools", "vision"},
		},
		{
			Name:          "claude-3-haiku-20240307",
			ContextWindow: 200000,
			MaxOutput:     4096,
			Features:      []string{"chat", "streaming", "tools", "vision"},
		},
	}
	return models, nil
}

// Complete sends a completion request
func (p *AnthropicProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	messages := toSchemaMessages(req.Messages)
	callOpts := buildCallOptions(req)

	resp, err := p.client.GenerateContent(ctx, messages, callOpts...)
	if err != nil {
		return nil, llm.TranslateError("anthropic", err)
	}

	return fromLangchainResponse(resp, req.Model), nil
}

// CompleteWithTools sends a completion request with tool definitions
func (p *AnthropicProvider) CompleteWithTools(ctx context.Context, req llm.CompletionRequest, tools []llm.ToolDef) (*llm.CompletionResponse, error) {
	// TODO: Implement tool support
	return p.Complete(ctx, req)
}

// Stream sends a streaming completion request
func (p *AnthropicProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	chunkChan := make(chan llm.StreamChunk, 10)

	messages := toSchemaMessages(req.Messages)
	callOpts := buildStreamingCallOptions(req, func(ctx context.Context, chunk []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunkChan <- llm.StreamChunk{
			Delta: llm.StreamDelta{
				Content: string(chunk),
			},
		}:
			return nil
		}
	})

	go func() {
		defer close(chunkChan)
		_, err := p.client.GenerateContent(ctx, messages, callOpts...)
		if err != nil {
			chunkChan <- llm.StreamChunk{
				Error: llm.TranslateError("anthropic", err),
			}
		}
	}()

	return chunkChan, nil
}

// Health checks the provider health
func (p *AnthropicProvider) Health(ctx context.Context) types.HealthStatus {
	// Try a simple API call to check health
	req := llm.CompletionRequest{
		Model: p.config.DefaultModel,
		Messages: []llm.Message{
			llm.NewUserMessage("test"),
		},
		MaxTokens: 1,
	}

	_, err := p.Complete(ctx, req)
	if err != nil {
		return types.NewHealthStatus(types.HealthStateUnhealthy, err.Error())
	}

	return types.NewHealthStatus(types.HealthStateHealthy, "")
}
