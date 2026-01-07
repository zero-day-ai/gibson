package providers

import (
	"context"
	"os"

	"github.com/tmc/langchaingo/llms/openai"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

// OpenAIProvider implements LLMProvider for OpenAI's GPT models
type OpenAIProvider struct {
	client *openai.LLM
	config llm.ProviderConfig
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(cfg llm.ProviderConfig) (*OpenAIProvider, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if apiKey == "" {
		return nil, llm.NewAuthError("openai", nil)
	}

	opts := []openai.Option{
		openai.WithToken(apiKey),
	}

	if cfg.DefaultModel != "" {
		opts = append(opts, openai.WithModel(cfg.DefaultModel))
	}

	if cfg.BaseURL != "" {
		opts = append(opts, openai.WithBaseURL(cfg.BaseURL))
	}

	client, err := openai.New(opts...)
	if err != nil {
		return nil, llm.TranslateError("openai", err)
	}

	return &OpenAIProvider{
		client: client,
		config: cfg,
	}, nil
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Models returns information about available models
func (p *OpenAIProvider) Models(ctx context.Context) ([]llm.ModelInfo, error) {
	models := []llm.ModelInfo{
		{
			Name:          "gpt-4-turbo",
			ContextWindow: 128000,
			MaxOutput:     4096,
			Features:      []string{"chat", "streaming", "tools", "vision", "json"},
		},
		{
			Name:          "gpt-4",
			ContextWindow: 8192,
			MaxOutput:     4096,
			Features:      []string{"chat", "streaming", "tools"},
		},
		{
			Name:          "gpt-3.5-turbo",
			ContextWindow: 16385,
			MaxOutput:     4096,
			Features:      []string{"chat", "streaming", "tools", "json"},
		},
	}
	return models, nil
}

// Complete sends a completion request
func (p *OpenAIProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	messages := toSchemaMessages(req.Messages)
	callOpts := buildCallOptions(req)

	resp, err := p.client.GenerateContent(ctx, messages, callOpts...)
	if err != nil {
		return nil, llm.TranslateError("openai", err)
	}

	return fromLangchainResponse(resp, req.Model), nil
}

// CompleteWithTools sends a completion request with tool definitions
func (p *OpenAIProvider) CompleteWithTools(ctx context.Context, req llm.CompletionRequest, tools []llm.ToolDef) (*llm.CompletionResponse, error) {
	messages := toSchemaMessages(req.Messages)
	callOpts := buildCallOptionsWithTools(req, tools)

	resp, err := p.client.GenerateContent(ctx, messages, callOpts...)
	if err != nil {
		return nil, llm.TranslateError("openai", err)
	}

	return fromLangchainResponse(resp, req.Model), nil
}

// Stream sends a streaming completion request
func (p *OpenAIProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
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
				Error: llm.TranslateError("openai", err),
			}
		}
	}()

	return chunkChan, nil
}

// Health checks the provider health
func (p *OpenAIProvider) Health(ctx context.Context) types.HealthStatus {
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
