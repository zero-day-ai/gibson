package observability

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/llm"
	"go.opentelemetry.io/otel/attribute"
)

// GenAI attribute keys following OpenTelemetry GenAI semantic conventions
// https://opentelemetry.io/docs/specs/semconv/gen-ai/
const (
	// GenAISystem identifies the Generative AI product or service being used
	GenAISystem = "gen_ai.system"

	// GenAIRequestModel is the name of the LLM model requested
	GenAIRequestModel = "gen_ai.request.model"

	// GenAIRequestTemperature is the temperature setting for the LLM request
	GenAIRequestTemperature = "gen_ai.request.temperature"

	// GenAIRequestMaxTokens is the maximum number of tokens requested
	GenAIRequestMaxTokens = "gen_ai.request.max_tokens"

	// GenAIRequestTopP is the top_p sampling parameter
	GenAIRequestTopP = "gen_ai.request.top_p"

	// GenAIResponseModel is the name of the LLM model that generated the response
	GenAIResponseModel = "gen_ai.response.model"

	// GenAIResponseFinishReason indicates why the model stopped generating tokens
	GenAIResponseFinishReason = "gen_ai.response.finish_reason"

	// GenAIUsageInputTokens is the number of tokens in the prompt
	GenAIUsageInputTokens = "gen_ai.usage.input_tokens"

	// GenAIUsageOutputTokens is the number of tokens in the generated completion
	GenAIUsageOutputTokens = "gen_ai.usage.output_tokens"

	// GenAIPrompt is the full prompt sent to the LLM (may contain sensitive data)
	GenAIPrompt = "gen_ai.prompt"

	// GenAICompletion is the full response from the LLM (may contain sensitive data)
	GenAICompletion = "gen_ai.completion"
)

// Span name constants for GenAI operations
const (
	// SpanGenAIChat represents a chat completion operation
	SpanGenAIChat = "gen_ai.chat"

	// SpanGenAIChatStream represents a streaming chat completion operation
	SpanGenAIChatStream = "gen_ai.chat.stream"

	// SpanGenAITool represents a tool/function call operation
	SpanGenAITool = "gen_ai.tool"

	// SpanGenAIEmbeddings represents an embeddings generation operation
	SpanGenAIEmbeddings = "gen_ai.embeddings"
)

// RequestAttributes creates OpenTelemetry attributes from an LLM completion request.
// The provider parameter identifies the LLM system (e.g., "openai", "anthropic", "ollama").
func RequestAttributes(req *llm.CompletionRequest, provider string) []attribute.KeyValue {
	if req == nil {
		return []attribute.KeyValue{}
	}

	attrs := []attribute.KeyValue{
		attribute.String(GenAISystem, provider),
		attribute.String(GenAIRequestModel, req.Model),
	}

	// Add optional parameters if they are set
	if req.Temperature > 0 {
		attrs = append(attrs, attribute.Float64(GenAIRequestTemperature, req.Temperature))
	}

	if req.MaxTokens > 0 {
		attrs = append(attrs, attribute.Int(GenAIRequestMaxTokens, req.MaxTokens))
	}

	if req.TopP > 0 {
		attrs = append(attrs, attribute.Float64(GenAIRequestTopP, req.TopP))
	}

	return attrs
}

// ResponseAttributes creates OpenTelemetry attributes from an LLM completion response.
func ResponseAttributes(resp *llm.CompletionResponse) []attribute.KeyValue {
	if resp == nil {
		return []attribute.KeyValue{}
	}

	attrs := []attribute.KeyValue{
		attribute.String(GenAIResponseModel, resp.Model),
		attribute.String(GenAIResponseFinishReason, string(resp.FinishReason)),
	}

	// Add completion content if available
	if resp.Message.Content != "" {
		attrs = append(attrs, attribute.String(GenAICompletion, resp.Message.Content))
	}

	return attrs
}

// UsageAttributes creates OpenTelemetry attributes from token usage statistics.
func UsageAttributes(usage llm.CompletionTokenUsage) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int(GenAIUsageInputTokens, usage.PromptTokens),
		attribute.Int(GenAIUsageOutputTokens, usage.CompletionTokens),
	}
}

// PromptAttributes creates an attribute containing the full prompt.
// Use with caution as this may contain sensitive data. Consider enabling
// only in development/debug environments.
func PromptAttributes(req *llm.CompletionRequest) []attribute.KeyValue {
	if req == nil || len(req.Messages) == 0 {
		return []attribute.KeyValue{}
	}

	// Concatenate all message contents for the prompt attribute
	var promptBuilder string
	for i, msg := range req.Messages {
		if i > 0 {
			promptBuilder += "\n"
		}
		promptBuilder += fmt.Sprintf("[%s] %s", msg.Role, msg.Content)
	}

	return []attribute.KeyValue{
		attribute.String(GenAIPrompt, promptBuilder),
	}
}
