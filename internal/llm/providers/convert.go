package providers

import (
	"context"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"
	"github.com/zero-day-ai/gibson/internal/llm"
)

// toSchemaMessages converts Gibson messages to langchaingo MessageContent
func toSchemaMessages(messages []llm.Message) []llms.MessageContent {
	result := make([]llms.MessageContent, 0, len(messages))

	for _, msg := range messages {
		var msgContent llms.MessageContent

		switch msg.Role {
		case llm.RoleSystem:
			msgContent = llms.MessageContent{
				Role: llms.ChatMessageTypeSystem,
				Parts: []llms.ContentPart{
					llms.TextPart(msg.Content),
				},
			}
		case llm.RoleUser:
			msgContent = llms.MessageContent{
				Role: llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{
					llms.TextPart(msg.Content),
				},
			}
		case llm.RoleAssistant:
			msgContent = llms.MessageContent{
				Role: llms.ChatMessageTypeAI,
				Parts: []llms.ContentPart{
					llms.TextPart(msg.Content),
				},
			}
		case llm.RoleTool:
			msgContent = llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.TextPart(msg.Content),
				},
			}
		default:
			msgContent = llms.MessageContent{
				Role: llms.ChatMessageTypeHuman,
				Parts: []llms.ContentPart{
					llms.TextPart(msg.Content),
				},
			}
		}

		result = append(result, msgContent)
	}

	return result
}

// fromLangchainResponse converts langchaingo response to Gibson response
func fromLangchainResponse(resp *llms.ContentResponse, model string) *llm.CompletionResponse {
	if resp == nil {
		return &llm.CompletionResponse{
			Model: model,
			ID:    uuid.New().String(),
		}
	}

	var content string
	var toolCalls []llm.ToolCall
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Content != "" {
			content = choice.Content
		}

		// Extract tool calls from the response
		if len(choice.ToolCalls) > 0 {
			toolCalls = make([]llm.ToolCall, 0, len(choice.ToolCalls))
			for _, tc := range choice.ToolCalls {
				var name, arguments string
				if tc.FunctionCall != nil {
					name = tc.FunctionCall.Name
					arguments = tc.FunctionCall.Arguments
				}

				toolCalls = append(toolCalls, llm.ToolCall{
					ID:        tc.ID,
					Type:      tc.Type,
					Name:      name,
					Arguments: arguments,
				})
			}
		}
	}

	finishReason := llm.FinishReasonStop
	if len(resp.Choices) > 0 {
		if reason := resp.Choices[0].StopReason; reason != "" {
			switch reason {
			case "stop":
				finishReason = llm.FinishReasonStop
			case "length", "max_tokens":
				finishReason = llm.FinishReasonLength
			case "tool_calls", "function_call":
				finishReason = llm.FinishReasonToolCalls
			case "content_filter":
				finishReason = llm.FinishReasonContentFilter
			default:
				finishReason = llm.FinishReasonStop
			}
		}

		// If we have tool calls but no explicit finish reason, set it to tool_calls
		if len(toolCalls) > 0 && finishReason == llm.FinishReasonStop {
			finishReason = llm.FinishReasonToolCalls
		}
	}

	return &llm.CompletionResponse{
		ID:    uuid.New().String(),
		Model: model,
		Message: llm.Message{
			Role:      llm.RoleAssistant,
			Content:   content,
			ToolCalls: toolCalls,
		},
		FinishReason: finishReason,
		Usage:        llm.CompletionTokenUsage{},
	}
}

// buildCallOptions converts Gibson request to langchaingo call options
func buildCallOptions(req llm.CompletionRequest) []llms.CallOption {
	callOpts := make([]llms.CallOption, 0)

	if req.Temperature > 0 {
		callOpts = append(callOpts, llms.WithTemperature(req.Temperature))
	}

	if req.MaxTokens > 0 {
		callOpts = append(callOpts, llms.WithMaxTokens(req.MaxTokens))
	}

	if req.TopP > 0 {
		callOpts = append(callOpts, llms.WithTopP(req.TopP))
	}

	if len(req.StopSequences) > 0 {
		callOpts = append(callOpts, llms.WithStopWords(req.StopSequences))
	}

	if req.Model != "" {
		callOpts = append(callOpts, llms.WithModel(req.Model))
	}

	return callOpts
}

// buildStreamingCallOptions builds call options with streaming
func buildStreamingCallOptions(req llm.CompletionRequest, streamFunc func(ctx context.Context, chunk []byte) error) []llms.CallOption {
	callOpts := buildCallOptions(req)
	callOpts = append(callOpts, llms.WithStreamingFunc(streamFunc))
	return callOpts
}

// toSchemaTools converts Gibson ToolDef to langchaingo Tool format
func toSchemaTools(tools []llm.ToolDef) []llms.Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]llms.Tool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return result
}

// buildCallOptionsWithTools adds tools to call options
func buildCallOptionsWithTools(req llm.CompletionRequest, tools []llm.ToolDef) []llms.CallOption {
	callOpts := buildCallOptions(req)
	if len(tools) > 0 {
		callOpts = append(callOpts, llms.WithTools(toSchemaTools(tools)))
	}
	return callOpts
}
