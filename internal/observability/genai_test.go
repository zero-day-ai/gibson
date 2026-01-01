package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/llm"
	"go.opentelemetry.io/otel/attribute"
)

func TestRequestAttributes(t *testing.T) {
	tests := []struct {
		name     string
		req      *llm.CompletionRequest
		provider string
		want     map[string]any
	}{
		{
			name: "basic request",
			req: &llm.CompletionRequest{
				Model: "gpt-4",
			},
			provider: "openai",
			want: map[string]any{
				GenAISystem:       "openai",
				GenAIRequestModel: "gpt-4",
			},
		},
		{
			name: "request with temperature",
			req: &llm.CompletionRequest{
				Model:       "claude-3-opus",
				Temperature: 0.7,
			},
			provider: "anthropic",
			want: map[string]any{
				GenAISystem:             "anthropic",
				GenAIRequestModel:       "claude-3-opus",
				GenAIRequestTemperature: 0.7,
			},
		},
		{
			name: "request with max tokens",
			req: &llm.CompletionRequest{
				Model:     "llama2",
				MaxTokens: 2048,
			},
			provider: "ollama",
			want: map[string]any{
				GenAISystem:           "ollama",
				GenAIRequestModel:     "llama2",
				GenAIRequestMaxTokens: int64(2048),
			},
		},
		{
			name: "request with top_p",
			req: &llm.CompletionRequest{
				Model: "gpt-3.5-turbo",
				TopP:  0.95,
			},
			provider: "openai",
			want: map[string]any{
				GenAISystem:       "openai",
				GenAIRequestModel: "gpt-3.5-turbo",
				GenAIRequestTopP:  0.95,
			},
		},
		{
			name: "full request with all parameters",
			req: &llm.CompletionRequest{
				Model:       "gpt-4-turbo",
				Temperature: 0.8,
				MaxTokens:   4096,
				TopP:        0.9,
			},
			provider: "openai",
			want: map[string]any{
				GenAISystem:             "openai",
				GenAIRequestModel:       "gpt-4-turbo",
				GenAIRequestTemperature: 0.8,
				GenAIRequestMaxTokens:   int64(4096),
				GenAIRequestTopP:        0.9,
			},
		},
		{
			name:     "nil request",
			req:      nil,
			provider: "openai",
			want:     map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := RequestAttributes(tt.req, tt.provider)

			// Convert attributes to map for easier comparison
			got := make(map[string]any)
			for _, attr := range attrs {
				got[string(attr.Key)] = attr.Value.AsInterface()
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResponseAttributes(t *testing.T) {
	tests := []struct {
		name string
		resp *llm.CompletionResponse
		want map[string]any
	}{
		{
			name: "basic response",
			resp: &llm.CompletionResponse{
				Model:        "gpt-4",
				FinishReason: llm.FinishReasonStop,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "Hello, world!",
				},
			},
			want: map[string]any{
				GenAIResponseModel:        "gpt-4",
				GenAIResponseFinishReason: "stop",
				GenAICompletion:           "Hello, world!",
			},
		},
		{
			name: "response with tool calls finish reason",
			resp: &llm.CompletionResponse{
				Model:        "gpt-4-turbo",
				FinishReason: llm.FinishReasonToolCalls,
				Message: llm.Message{
					Role: llm.RoleAssistant,
				},
			},
			want: map[string]any{
				GenAIResponseModel:        "gpt-4-turbo",
				GenAIResponseFinishReason: "tool_calls",
			},
		},
		{
			name: "response with length finish reason",
			resp: &llm.CompletionResponse{
				Model:        "claude-3-sonnet",
				FinishReason: llm.FinishReasonLength,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "This is a partial response...",
				},
			},
			want: map[string]any{
				GenAIResponseModel:        "claude-3-sonnet",
				GenAIResponseFinishReason: "length",
				GenAICompletion:           "This is a partial response...",
			},
		},
		{
			name: "response with content filter",
			resp: &llm.CompletionResponse{
				Model:        "gpt-4",
				FinishReason: llm.FinishReasonContentFilter,
				Message: llm.Message{
					Role: llm.RoleAssistant,
				},
			},
			want: map[string]any{
				GenAIResponseModel:        "gpt-4",
				GenAIResponseFinishReason: "content_filter",
			},
		},
		{
			name: "nil response",
			resp: nil,
			want: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := ResponseAttributes(tt.resp)

			// Convert attributes to map for easier comparison
			got := make(map[string]any)
			for _, attr := range attrs {
				got[string(attr.Key)] = attr.Value.AsInterface()
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUsageAttributes(t *testing.T) {
	tests := []struct {
		name  string
		usage llm.CompletionTokenUsage
		want  map[string]any
	}{
		{
			name: "basic usage",
			usage: llm.CompletionTokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
			want: map[string]any{
				GenAIUsageInputTokens:  int64(100),
				GenAIUsageOutputTokens: int64(50),
			},
		},
		{
			name: "large usage",
			usage: llm.CompletionTokenUsage{
				PromptTokens:     8192,
				CompletionTokens: 4096,
				TotalTokens:      12288,
			},
			want: map[string]any{
				GenAIUsageInputTokens:  int64(8192),
				GenAIUsageOutputTokens: int64(4096),
			},
		},
		{
			name: "zero usage",
			usage: llm.CompletionTokenUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
			want: map[string]any{
				GenAIUsageInputTokens:  int64(0),
				GenAIUsageOutputTokens: int64(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := UsageAttributes(tt.usage)

			// Convert attributes to map for easier comparison
			got := make(map[string]any)
			for _, attr := range attrs {
				got[string(attr.Key)] = attr.Value.AsInterface()
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPromptAttributes(t *testing.T) {
	tests := []struct {
		name string
		req  *llm.CompletionRequest
		want string
	}{
		{
			name: "single message",
			req: &llm.CompletionRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Hello"},
				},
			},
			want: "[user] Hello",
		},
		{
			name: "multiple messages",
			req: &llm.CompletionRequest{
				Messages: []llm.Message{
					{Role: llm.RoleSystem, Content: "You are a helpful assistant"},
					{Role: llm.RoleUser, Content: "What is Go?"},
					{Role: llm.RoleAssistant, Content: "Go is a programming language"},
				},
			},
			want: "[system] You are a helpful assistant\n[user] What is Go?\n[assistant] Go is a programming language",
		},
		{
			name: "nil request",
			req:  nil,
			want: "",
		},
		{
			name: "empty messages",
			req: &llm.CompletionRequest{
				Messages: []llm.Message{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := PromptAttributes(tt.req)

			if tt.want == "" {
				assert.Empty(t, attrs)
				return
			}

			require.Len(t, attrs, 1)
			assert.Equal(t, GenAIPrompt, string(attrs[0].Key))
			assert.Equal(t, tt.want, attrs[0].Value.AsString())
		})
	}
}

func TestGenAIAttributeKeyConstants(t *testing.T) {
	// Test that attribute keys follow the correct naming convention
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"GenAI System", GenAISystem, "gen_ai.system"},
		{"GenAI Request Model", GenAIRequestModel, "gen_ai.request.model"},
		{"GenAI Request Temperature", GenAIRequestTemperature, "gen_ai.request.temperature"},
		{"GenAI Request Max Tokens", GenAIRequestMaxTokens, "gen_ai.request.max_tokens"},
		{"GenAI Request Top P", GenAIRequestTopP, "gen_ai.request.top_p"},
		{"GenAI Response Model", GenAIResponseModel, "gen_ai.response.model"},
		{"GenAI Response Finish Reason", GenAIResponseFinishReason, "gen_ai.response.finish_reason"},
		{"GenAI Usage Input Tokens", GenAIUsageInputTokens, "gen_ai.usage.input_tokens"},
		{"GenAI Usage Output Tokens", GenAIUsageOutputTokens, "gen_ai.usage.output_tokens"},
		{"GenAI Prompt", GenAIPrompt, "gen_ai.prompt"},
		{"GenAI Completion", GenAICompletion, "gen_ai.completion"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestGenAISpanNameConstants(t *testing.T) {
	// Test that span names follow the correct naming convention
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Chat Span", SpanGenAIChat, "gen_ai.chat"},
		{"Chat Stream Span", SpanGenAIChatStream, "gen_ai.chat.stream"},
		{"Tool Span", SpanGenAITool, "gen_ai.tool"},
		{"Embeddings Span", SpanGenAIEmbeddings, "gen_ai.embeddings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestRequestAttributesTypes(t *testing.T) {
	// Verify that attributes have the correct types
	req := &llm.CompletionRequest{
		Model:       "gpt-4",
		Temperature: 0.7,
		MaxTokens:   1000,
		TopP:        0.9,
	}

	attrs := RequestAttributes(req, "openai")

	// Build a type map
	typeMap := make(map[string]attribute.Type)
	for _, attr := range attrs {
		typeMap[string(attr.Key)] = attr.Value.Type()
	}

	assert.Equal(t, attribute.STRING, typeMap[GenAISystem])
	assert.Equal(t, attribute.STRING, typeMap[GenAIRequestModel])
	assert.Equal(t, attribute.FLOAT64, typeMap[GenAIRequestTemperature])
	assert.Equal(t, attribute.INT64, typeMap[GenAIRequestMaxTokens])
	assert.Equal(t, attribute.FLOAT64, typeMap[GenAIRequestTopP])
}

func TestResponseAttributesTypes(t *testing.T) {
	// Verify that attributes have the correct types
	resp := &llm.CompletionResponse{
		Model:        "gpt-4",
		FinishReason: llm.FinishReasonStop,
		Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Test",
		},
	}

	attrs := ResponseAttributes(resp)

	// Build a type map
	typeMap := make(map[string]attribute.Type)
	for _, attr := range attrs {
		typeMap[string(attr.Key)] = attr.Value.Type()
	}

	assert.Equal(t, attribute.STRING, typeMap[GenAIResponseModel])
	assert.Equal(t, attribute.STRING, typeMap[GenAIResponseFinishReason])
	assert.Equal(t, attribute.STRING, typeMap[GenAICompletion])
}

func TestUsageAttributesTypes(t *testing.T) {
	// Verify that attributes have the correct types
	usage := llm.CompletionTokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	attrs := UsageAttributes(usage)

	// Build a type map
	typeMap := make(map[string]attribute.Type)
	for _, attr := range attrs {
		typeMap[string(attr.Key)] = attr.Value.Type()
	}

	assert.Equal(t, attribute.INT64, typeMap[GenAIUsageInputTokens])
	assert.Equal(t, attribute.INT64, typeMap[GenAIUsageOutputTokens])
}
