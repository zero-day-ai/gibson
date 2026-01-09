package middleware

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/llm"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span names following OpenTelemetry GenAI semantic conventions
const (
	SpanGenAIChat       = "gen_ai.chat"
	SpanGenAIChatStream = "gen_ai.chat.stream"
	SpanGenAITool       = "gen_ai.tool"
	SpanPluginQuery     = "gibson.plugin.query"
	SpanAgentDelegate   = "gibson.agent.delegate"
	SpanFindingSubmit   = "gibson.finding.submit"
	SpanMemoryGet       = "gibson.memory.get"
	SpanMemorySet       = "gibson.memory.set"
	SpanMemorySearch    = "gibson.memory.search"
)

// Attribute keys
const (
	AttrMissionID        = "gibson.mission.id"
	AttrAgentName        = "gibson.agent.name"
	AttrToolName         = "gibson.tool.name"
	AttrPluginName       = "gibson.plugin.name"
	AttrPluginMethod     = "gibson.plugin.method"
	AttrDelegationTarget = "gibson.delegation.target_agent"
	AttrLLMSlot          = "gibson.llm.slot"
	AttrErrorCode        = "error.code"
	AttrErrorType        = "error.type"
)

// TracingMiddleware creates a middleware that adds OpenTelemetry tracing to all harness operations.
func TracingMiddleware(tracer trace.Tracer) Middleware {
	return func(next Operation) Operation {
		return func(ctx context.Context, req any) (any, error) {
			opType := GetOperationType(ctx)
			spanName := getSpanNameForOperation(opType)

			ctx, span := tracer.Start(ctx, spanName)
			defer span.End()

			// Add base attributes
			addBaseAttributes(span, ctx)
			addPreExecutionAttributes(span, ctx, opType, req)

			// Execute
			resp, err := next(ctx, req)

			// Record result
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.SetAttributes(
					attribute.String(AttrErrorCode, getErrorCodeForOperation(opType)),
					attribute.String(AttrErrorType, fmt.Sprintf("%T", err)),
				)
			} else {
				span.SetStatus(codes.Ok, "")
				addPostExecutionAttributes(span, ctx, opType, resp)
			}

			return resp, err
		}
	}
}

func getSpanNameForOperation(opType OperationType) string {
	switch opType {
	case OpComplete, OpCompleteWithTools:
		return SpanGenAIChat
	case OpStream:
		return SpanGenAIChatStream
	case OpCallTool:
		return SpanGenAITool
	case OpQueryPlugin:
		return SpanPluginQuery
	case OpDelegateToAgent:
		return SpanAgentDelegate
	case OpSubmitFinding:
		return SpanFindingSubmit
	case OpMemoryGet:
		return SpanMemoryGet
	case OpMemorySet:
		return SpanMemorySet
	case OpMemorySearch:
		return SpanMemorySearch
	default:
		return fmt.Sprintf("gibson.harness.%s", opType)
	}
}

func getErrorCodeForOperation(opType OperationType) string {
	switch opType {
	case OpComplete, OpCompleteWithTools:
		return "COMPLETION_ERROR"
	case OpStream:
		return "STREAM_ERROR"
	case OpCallTool:
		return "TOOL_EXECUTION_ERROR"
	case OpQueryPlugin:
		return "PLUGIN_QUERY_ERROR"
	case OpDelegateToAgent:
		return "DELEGATION_ERROR"
	case OpSubmitFinding:
		return "FINDING_SUBMIT_ERROR"
	case OpMemoryGet, OpMemorySet, OpMemorySearch, OpMemoryDelete, OpMemoryList:
		return "MEMORY_ERROR"
	default:
		return "HARNESS_ERROR"
	}
}

func addBaseAttributes(span trace.Span, ctx context.Context) {
	missionID, agentName := GetMissionContext(ctx)
	if missionID != "" {
		span.SetAttributes(attribute.String(AttrMissionID, missionID))
	}
	if agentName != "" {
		span.SetAttributes(attribute.String(AttrAgentName, agentName))
	}
}

func addPreExecutionAttributes(span trace.Span, ctx context.Context, opType OperationType, req any) {
	switch opType {
	case OpComplete, OpCompleteWithTools:
		if slot := GetSlotName(ctx); slot != "" {
			span.SetAttributes(attribute.String(AttrLLMSlot, slot))
		}
	case OpCallTool:
		if toolName := GetToolName(ctx); toolName != "" {
			span.SetAttributes(attribute.String(AttrToolName, toolName))
		}
	case OpQueryPlugin:
		pluginName, method := GetPluginInfo(ctx)
		if pluginName != "" {
			span.SetAttributes(
				attribute.String(AttrPluginName, pluginName),
				attribute.String(AttrPluginMethod, method),
			)
		}
	case OpDelegateToAgent:
		if targetAgent := GetAgentTargetName(ctx); targetAgent != "" {
			span.SetAttributes(attribute.String(AttrDelegationTarget, targetAgent))
		}
	}
}

func addPostExecutionAttributes(span trace.Span, ctx context.Context, opType OperationType, resp any) {
	switch opType {
	case OpComplete, OpCompleteWithTools:
		if resp != nil {
			if completionResp, ok := resp.(*llm.CompletionResponse); ok {
				span.SetAttributes(
					attribute.String("gen_ai.response.id", completionResp.ID),
					attribute.String("gen_ai.request.model", completionResp.Model),
					attribute.String("gen_ai.response.finish_reason", string(completionResp.FinishReason)),
					attribute.Int("gen_ai.usage.input_tokens", completionResp.Usage.PromptTokens),
					attribute.Int("gen_ai.usage.output_tokens", completionResp.Usage.CompletionTokens),
				)
				if opType == OpCompleteWithTools && len(completionResp.Message.ToolCalls) > 0 {
					span.SetAttributes(attribute.Int("gen_ai.response.tool_calls.count", len(completionResp.Message.ToolCalls)))
					toolCallNames := make([]string, len(completionResp.Message.ToolCalls))
					for i, tc := range completionResp.Message.ToolCalls {
						toolCallNames[i] = tc.Name
					}
					span.SetAttributes(attribute.StringSlice("gen_ai.response.tool_calls.names", toolCallNames))
				}
			}
		}
	}
}
