package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/loader"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
	pb "github.com/zero-day-ai/sdk/api/gen/proto"
	sdkfinding "github.com/zero-day-ai/sdk/finding"
	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
	"github.com/zero-day-ai/sdk/graphrag/domain"
	"github.com/zero-day-ai/sdk/schema"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CredentialStore provides access to stored credentials.
// This interface is implemented by the daemon's credential manager
// to provide secure credential retrieval with decryption.
type CredentialStore interface {
	// GetCredential retrieves a credential by name, decrypting it if necessary.
	// Returns the credential with its secret value populated.
	GetCredential(ctx context.Context, name string) (*types.Credential, string, error)
}

// EventBusPublisher is an interface for publishing daemon-wide events.
// This allows the callback service to publish tool and LLM events
// to the daemon's event bus for graph processing.
type EventBusPublisher interface {
	Publish(ctx context.Context, event interface{}) error
}

// HarnessCallbackService implements the gRPC HarnessCallbackService server.
// It receives harness operation requests from remote agents via gRPC and
// delegates them to the appropriate registered harness instance.
//
// The service uses mission-based harness lookup via CallbackHarnessRegistry,
// requiring explicit mission_id and agent_name in the callback context.
// This enforces clean separation and supports concurrent execution of the
// same agent in different missions.
//
// When an agent running in standalone mode makes a harness call, the SDK's
// CallbackClient sends a gRPC request with context information (mission ID
// and agent name), which is used to look up the correct harness instance.
//
// To register this service with a gRPC server:
//
//	service := harness.NewHarnessCallbackServiceWithRegistry(logger, registry)
//	pb.RegisterHarnessCallbackServiceServer(grpcServer, service)
//
// Before executing an agent task, register its harness:
//
//	registry.RegisterHarnessForMission(missionID, agentName, harness)
//	defer registry.UnregisterHarnessForMission(missionID, agentName)
type HarnessCallbackService struct {
	pb.UnimplementedHarnessCallbackServiceServer

	// activeHarnesses maps task IDs to their corresponding harness instances (legacy mode)
	activeHarnesses sync.Map // map[string]AgentHarness

	// registry provides mission-based harness lookup for external agents (new mode)
	registry *CallbackHarnessRegistry

	// credentialStore provides access to stored credentials
	credentialStore CredentialStore

	// graphLoader loads domain nodes into Neo4j using the GraphNode interface
	graphLoader *loader.GraphLoader

	// eventBus publishes tool and LLM events for graph processing
	eventBus EventBusPublisher

	// spanProcessors receives spans exported from remote agents for tracing integration
	spanProcessors []sdktrace.SpanProcessor

	// tracerProvider for creating real spans from proxy span data
	tracerProvider *sdktrace.TracerProvider

	// mu protects spanProcessors for concurrent access
	mu sync.RWMutex

	// logger for service-level logging
	logger *slog.Logger
}

// CallbackServiceOption configures the callback service.
type CallbackServiceOption func(*HarnessCallbackService)

// WithSpanProcessors adds span processors to receive tracing spans from remote agents.
func WithSpanProcessors(processors ...sdktrace.SpanProcessor) CallbackServiceOption {
	return func(s *HarnessCallbackService) {
		s.spanProcessors = append(s.spanProcessors, processors...)
	}
}

// WithTracerProvider sets the TracerProvider for creating real spans from proxy span data.
// When set, proxy spans from remote agents are re-created as real spans and passed through
// the TracerProvider's span processors (e.g., Langfuse exporter).
func WithTracerProvider(tp *sdktrace.TracerProvider) CallbackServiceOption {
	return func(s *HarnessCallbackService) {
		s.tracerProvider = tp
	}
}

// WithCredentialStore sets the credential store for secure credential retrieval.
// When set, agents can retrieve stored credentials by name via the GetCredential RPC.
func WithCredentialStore(store CredentialStore) CallbackServiceOption {
	return func(s *HarnessCallbackService) {
		s.credentialStore = store
	}
}

// WithEventBus sets the event bus for publishing tool and LLM events.
// When set, the callback service publishes events for tool calls and LLM requests
// that can be consumed by the execution graph engine.
func WithEventBus(eventBus EventBusPublisher) CallbackServiceOption {
	return func(s *HarnessCallbackService) {
		s.eventBus = eventBus
	}
}

// WithGraphLoader sets the GraphLoader for processing DiscoveryResult tool outputs.
// When set, the callback service will check if tool output is a DiscoveryResult
// and use the loader to create nodes and relationships in Neo4j.
func WithGraphLoader(graphLoader *loader.GraphLoader) CallbackServiceOption {
	return func(s *HarnessCallbackService) {
		s.graphLoader = graphLoader
	}
}

// NewHarnessCallbackService creates a new callback service instance with
// task-based harness lookup (legacy mode).
func NewHarnessCallbackService(logger *slog.Logger, opts ...CallbackServiceOption) *HarnessCallbackService {
	if logger == nil {
		logger = slog.Default()
	}

	s := &HarnessCallbackService{
		logger: logger.With("component", "harness_callback_service"),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// NewHarnessCallbackServiceWithRegistry creates a new callback service instance
// with mission-based harness lookup via the provided registry.
//
// This is the preferred constructor for external agents, as it supports
// concurrent execution of the same agent in different missions.
//
// Parameters:
//   - logger: Structured logger for service events
//   - registry: The harness registry for mission-based lookups
//   - opts: Optional configuration options (e.g., WithSpanProcessors)
//
// Returns:
//   - *HarnessCallbackService: A new service instance ready to be registered
func NewHarnessCallbackServiceWithRegistry(logger *slog.Logger, registry *CallbackHarnessRegistry, opts ...CallbackServiceOption) *HarnessCallbackService {
	if logger == nil {
		logger = slog.Default()
	}

	s := &HarnessCallbackService{
		registry: registry,
		logger:   logger.With("component", "harness_callback_service"),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// RegisterHarness registers a harness instance for a specific task.
// This must be called before an agent starts execution so that callbacks
// can find the correct harness instance.
func (s *HarnessCallbackService) RegisterHarness(taskID string, harness AgentHarness) {
	s.activeHarnesses.Store(taskID, harness)
	s.logger.Debug("registered harness for task", "task_id", taskID)
}

// UnregisterHarness removes a harness instance when a task completes.
// This prevents memory leaks from accumulating completed tasks.
func (s *HarnessCallbackService) UnregisterHarness(taskID string) {
	s.activeHarnesses.Delete(taskID)
	s.logger.Debug("unregistered harness for task", "task_id", taskID)
}

// getHarness retrieves the harness for a request based on the context information.
//
// This method requires explicit mission ID and agent name for harness lookup via registry.
// The legacy string parsing and task-based lookup have been removed to enforce the use
// of the explicit mission_id field in ContextInfo.
//
// Parameters:
//   - ctx: Request context
//   - contextInfo: Context information from the gRPC request
//
// Returns:
//   - AgentHarness: The harness instance to use for this request
//   - error: Non-nil if no harness is found or if context info is invalid
func (s *HarnessCallbackService) getHarness(ctx context.Context, contextInfo *pb.ContextInfo) (AgentHarness, error) {
	if contextInfo == nil {
		return nil, status.Error(codes.InvalidArgument, "missing context info in request")
	}

	// Require explicit mission ID and agent name
	if contextInfo.MissionId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing mission_id in context info - ensure agent SDK is v0.7.0+")
	}

	if contextInfo.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "missing agent_name in context info")
	}

	// Registry must be configured for mission-based lookup
	if s.registry == nil {
		return nil, status.Error(codes.Internal, "callback registry not configured")
	}

	// Perform registry lookup with explicit mission ID and agent name
	harness, err := s.registry.Lookup(contextInfo.MissionId, contextInfo.AgentName)
	if err != nil {
		s.logger.Error("harness lookup failed",
			"mission_id", contextInfo.MissionId,
			"agent_name", contextInfo.AgentName,
			"task_id", contextInfo.TaskId,
			"error", err,
		)
		return nil, status.Errorf(codes.NotFound, "no active harness for mission %s, agent %s: %v",
			contextInfo.MissionId, contextInfo.AgentName, err)
	}

	s.logger.Debug("harness lookup succeeded",
		"mission_id", contextInfo.MissionId,
		"agent_name", contextInfo.AgentName,
		"task_id", contextInfo.TaskId,
	)

	return harness, nil
}

// getGraphRAGHarness retrieves a harness that supports GraphRAG operations.
func (s *HarnessCallbackService) getGraphRAGHarness(ctx context.Context, contextInfo *pb.ContextInfo) (GraphRAGSupport, error) {
	harness, err := s.getHarness(ctx, contextInfo)
	if err != nil {
		return nil, err
	}

	graphRAG, ok := harness.(GraphRAGSupport)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "GraphRAG not supported by this harness")
	}

	return graphRAG, nil
}

// ============================================================================
// LLM Operations
// ============================================================================

// LLMComplete implements the LLM completion RPC.
func (s *HarnessCallbackService) LLMComplete(ctx context.Context, req *pb.LLMCompleteRequest) (*pb.LLMCompleteResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Publish llm.request.started event
	// Include parent_span_id from context so taxonomy engine can create MADE_CALL relationship
	s.publishEvent(ctx, "llm.request.started", map[string]interface{}{
		"slot":           req.Slot,
		"mission_id":     req.Context.MissionId,
		"agent_name":     req.Context.AgentName,
		"task_id":        req.Context.TaskId,
		"message_count":  len(req.Messages),
		"parent_span_id": req.Context.SpanId, // Agent's span ID becomes LLM call's parent
	})

	// Convert proto messages to llm.Message
	messages := s.protoToMessages(req.Messages)

	// Build completion options
	var opts []CompletionOption
	if req.Temperature != nil {
		opts = append(opts, WithTemperature(*req.Temperature))
	}
	if req.MaxTokens != nil {
		opts = append(opts, WithMaxTokens(int(*req.MaxTokens)))
	}
	if req.TopP != nil {
		opts = append(opts, WithTopP(*req.TopP))
	}
	if len(req.Stop) > 0 {
		opts = append(opts, WithStopSequences(req.Stop...))
	}

	// Execute completion
	resp, err := harness.Complete(ctx, req.Slot, messages, opts...)
	if err != nil {
		s.logger.Error("LLM completion failed", "error", err, "task_id", req.Context.TaskId)

		// Publish llm.request.failed event
		s.publishEvent(ctx, "llm.request.failed", map[string]interface{}{
			"slot":           req.Slot,
			"mission_id":     req.Context.MissionId,
			"agent_name":     req.Context.AgentName,
			"task_id":        req.Context.TaskId,
			"error":          err.Error(),
			"parent_span_id": req.Context.SpanId,
		})

		return &pb.LLMCompleteResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Publish llm.request.completed event
	s.publishEvent(ctx, "llm.request.completed", map[string]interface{}{
		"slot":              req.Slot,
		"mission_id":        req.Context.MissionId,
		"agent_name":        req.Context.AgentName,
		"task_id":           req.Context.TaskId,
		"finish_reason":     string(resp.FinishReason),
		"prompt_tokens":     resp.Usage.PromptTokens,
		"completion_tokens": resp.Usage.CompletionTokens,
		"total_tokens":      resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		"parent_span_id":    req.Context.SpanId,
	})

	// Convert response
	return &pb.LLMCompleteResponse{
		Content:      resp.Message.Content,
		ToolCalls:    s.toolCallsToProto(resp.Message.ToolCalls),
		FinishReason: string(resp.FinishReason),
		Usage: &pb.TokenUsage{
			InputTokens:  int32(resp.Usage.PromptTokens),
			OutputTokens: int32(resp.Usage.CompletionTokens),
			TotalTokens:  int32(resp.Usage.PromptTokens + resp.Usage.CompletionTokens),
		},
	}, nil
}

// LLMCompleteWithTools implements the LLM completion with tools RPC.
func (s *HarnessCallbackService) LLMCompleteWithTools(ctx context.Context, req *pb.LLMCompleteWithToolsRequest) (*pb.LLMCompleteResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Convert proto messages and tools
	messages := s.protoToMessages(req.Messages)
	tools := s.protoToToolDefs(req.Tools)

	// Execute completion with tools
	resp, err := harness.CompleteWithTools(ctx, req.Slot, messages, tools)
	if err != nil {
		s.logger.Error("LLM completion with tools failed", "error", err, "task_id", req.Context.TaskId)
		return &pb.LLMCompleteResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert response
	return &pb.LLMCompleteResponse{
		Content:      resp.Message.Content,
		ToolCalls:    s.toolCallsToProto(resp.Message.ToolCalls),
		FinishReason: string(resp.FinishReason),
		Usage: &pb.TokenUsage{
			InputTokens:  int32(resp.Usage.PromptTokens),
			OutputTokens: int32(resp.Usage.CompletionTokens),
			TotalTokens:  int32(resp.Usage.PromptTokens + resp.Usage.CompletionTokens),
		},
	}, nil
}

// LLMStream implements the streaming LLM completion RPC.
func (s *HarnessCallbackService) LLMStream(req *pb.LLMStreamRequest, stream pb.HarnessCallbackService_LLMStreamServer) error {
	harness, err := s.getHarness(stream.Context(), req.Context)
	if err != nil {
		return err
	}

	// Convert proto messages
	messages := s.protoToMessages(req.Messages)

	// Build completion options
	var opts []CompletionOption
	if req.Temperature != nil {
		opts = append(opts, WithTemperature(*req.Temperature))
	}
	if req.MaxTokens != nil {
		opts = append(opts, WithMaxTokens(int(*req.MaxTokens)))
	}
	if req.TopP != nil {
		opts = append(opts, WithTopP(*req.TopP))
	}
	if len(req.Stop) > 0 {
		opts = append(opts, WithStopSequences(req.Stop...))
	}

	// Execute streaming completion
	chunkChan, err := harness.Stream(stream.Context(), req.Slot, messages, opts...)
	if err != nil {
		s.logger.Error("LLM stream failed", "error", err, "task_id", req.Context.TaskId)
		return status.Errorf(codes.Internal, "stream failed: %v", err)
	}

	// Forward chunks to client
	for chunk := range chunkChan {
		// Check for error in chunk
		if chunk.Error != nil {
			protoChunk := &pb.LLMStreamChunk{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: chunk.Error.Error(),
				},
			}
			_ = stream.Send(protoChunk)
			return nil
		}

		protoChunk := &pb.LLMStreamChunk{
			Delta:        chunk.Delta.Content,
			FinishReason: string(chunk.FinishReason),
		}

		if err := stream.Send(protoChunk); err != nil {
			s.logger.Error("failed to send stream chunk", "error", err)
			return status.Errorf(codes.Internal, "stream send failed: %v", err)
		}
	}

	return nil
}

// LLMCompleteStructured implements the structured LLM completion RPC.
// This uses provider-native structured output mechanisms (tool_use for Anthropic,
// response_format for OpenAI) to guarantee JSON responses matching the schema.
func (s *HarnessCallbackService) LLMCompleteStructured(ctx context.Context, req *pb.LLMCompleteStructuredRequest) (*pb.LLMCompleteStructuredResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Convert proto messages to llm.Message
	messages := s.protoToMessages(req.Messages)

	// Parse the schema JSON to reconstruct the schema type
	var schemaData map[string]any
	if err := json.Unmarshal([]byte(req.SchemaJson), &schemaData); err != nil {
		s.logger.Error("failed to parse schema JSON", "error", err, "task_id", req.Context.TaskId)
		return &pb.LLMCompleteStructuredResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid schema JSON: %v", err),
			},
		}, nil
	}

	// Execute structured completion
	// The harness.CompleteStructured method takes a schema type instance
	// For callback mode, we pass the parsed map which will be used to build the response format
	result, err := harness.CompleteStructuredAny(ctx, req.Slot, messages, schemaData)
	if err != nil {
		s.logger.Error("LLM structured completion failed", "error", err, "task_id", req.Context.TaskId)
		return &pb.LLMCompleteStructuredResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Serialize result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("failed to serialize structured result", "error", err, "task_id", req.Context.TaskId)
		return &pb.LLMCompleteStructuredResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: fmt.Sprintf("failed to serialize result: %v", err),
			},
		}, nil
	}

	return &pb.LLMCompleteStructuredResponse{
		ResultJson: string(resultJSON),
		// Note: Token usage would need to be extracted from the completion response
		// For now we return nil usage since we don't have access to it from CompleteStructuredAny
	}, nil
}

// ============================================================================
// Tool Operations
// ============================================================================

// CallTool implements the tool execution RPC.
func (s *HarnessCallbackService) CallTool(ctx context.Context, req *pb.CallToolRequest) (*pb.CallToolResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Publish tool.call.started event
	// Include parent_span_id from context so taxonomy engine can create EXECUTED_BY relationship
	s.publishEvent(ctx, "tool.call.started", map[string]interface{}{
		"tool_name":      req.Name,
		"mission_id":     req.Context.MissionId,
		"agent_name":     req.Context.AgentName,
		"task_id":        req.Context.TaskId,
		"parent_span_id": req.Context.SpanId, // Agent's span ID becomes tool call's parent
	})

	// Deserialize input
	var input map[string]any
	if err := json.Unmarshal([]byte(req.InputJson), &input); err != nil {
		// Publish tool.call.failed event for invalid input
		s.publishEvent(ctx, "tool.call.failed", map[string]interface{}{
			"tool_name":      req.Name,
			"mission_id":     req.Context.MissionId,
			"agent_name":     req.Context.AgentName,
			"task_id":        req.Context.TaskId,
			"error":          fmt.Sprintf("invalid input JSON: %v", err),
			"parent_span_id": req.Context.SpanId,
		})

		return &pb.CallToolResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid input JSON: %v", err),
			},
		}, nil
	}

	// Execute tool
	output, err := harness.CallTool(ctx, req.Name, input)
	if err != nil {
		s.logger.Error("tool execution failed", "error", err, "tool", req.Name)

		// Publish tool.call.failed event
		s.publishEvent(ctx, "tool.call.failed", map[string]interface{}{
			"tool_name":      req.Name,
			"mission_id":     req.Context.MissionId,
			"agent_name":     req.Context.AgentName,
			"task_id":        req.Context.TaskId,
			"error":          err.Error(),
			"parent_span_id": req.Context.SpanId,
		})

		return &pb.CallToolResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Serialize output
	outputJSON, err := json.Marshal(output)
	if err != nil {
		// Publish tool.call.failed event for serialization error
		s.publishEvent(ctx, "tool.call.failed", map[string]interface{}{
			"tool_name":      req.Name,
			"mission_id":     req.Context.MissionId,
			"agent_name":     req.Context.AgentName,
			"task_id":        req.Context.TaskId,
			"error":          fmt.Sprintf("failed to serialize output: %v", err),
			"parent_span_id": req.Context.SpanId,
		})

		return &pb.CallToolResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: fmt.Sprintf("failed to serialize output: %v", err),
			},
		}, nil
	}

	// Publish tool.call.completed event
	s.publishEvent(ctx, "tool.call.completed", map[string]interface{}{
		"tool_name":      req.Name,
		"mission_id":     req.Context.MissionId,
		"agent_name":     req.Context.AgentName,
		"task_id":        req.Context.TaskId,
		"parent_span_id": req.Context.SpanId,
	})

	// Graph tool output automatically
	if output != nil {
		// Get agent run ID from context - try multiple sources
		agentRunID := s.extractAgentRunID(ctx, req.Context)

		// Process tool output in background to avoid blocking the tool response
		// Graphing errors are non-fatal and shouldn't fail the tool execution
		go func() {
			// Use background context with timeout for graphing
			graphCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Check if output is a DiscoveryResult and use GraphLoader
			if s.graphLoader != nil {
				// Tools return map[string]any with DiscoveryResult under "discovery_result" key.
				// Extract it if present, otherwise try to unmarshal the entire output.
				var toMarshal any = output
				if dr, ok := output["discovery_result"]; ok {
					toMarshal = dr
				}

				outputJSON, marshalErr := json.Marshal(toMarshal)
				if marshalErr == nil {
					var discoveryResult domain.DiscoveryResult
					if unmarshalErr := json.Unmarshal(outputJSON, &discoveryResult); unmarshalErr == nil {
						// Check if this is actually a non-empty DiscoveryResult
						if !discoveryResult.IsEmpty() {
							s.logger.Info("processing DiscoveryResult with GraphLoader",
								"tool", req.Name,
								"node_count", discoveryResult.NodeCount(),
								"agent_run_id", agentRunID,
								"mission_id", req.Context.MissionId)

							// Construct execution context
							execCtx := loader.ExecContext{
								AgentRunID:      agentRunID,
								ToolExecutionID: req.Context.TaskId, // Use task ID as tool execution ID
								MissionID:       req.Context.MissionId,
							}

							// Load all discovered nodes into the graph
							result, err := s.graphLoader.Load(graphCtx, execCtx, discoveryResult.AllNodes())
							if err != nil {
								s.logger.Warn("GraphLoader failed to load nodes",
									"tool", req.Name,
									"agent_run_id", agentRunID,
									"error", err)
							} else {
								s.logger.Info("GraphLoader successfully loaded nodes",
									"tool", req.Name,
									"nodes_created", result.NodesCreated,
									"nodes_updated", result.NodesUpdated,
									"relationships_created", result.RelationshipsCreated,
									"errors", len(result.Errors))

								// Log any partial errors
								if result.HasErrors() {
									for i, err := range result.Errors {
										s.logger.Warn("GraphLoader partial error",
											"tool", req.Name,
											"error_index", i,
											"error", err)
									}
								}
							}
						}
					}
				}
			}
		}()
	}

	return &pb.CallToolResponse{
		OutputJson: string(outputJSON),
	}, nil
}

// ListTools implements the tool listing RPC.
func (s *HarnessCallbackService) ListTools(ctx context.Context, req *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Get tool descriptors
	tools := harness.ListTools()

	// Convert to proto with structured schemas (including taxonomy)
	protoTools := make([]*pb.HarnessToolDescriptor, len(tools))
	for i, tool := range tools {
		// Marshal legacy JSON schemas for backward compatibility
		inputSchemaJSON, err := json.Marshal(tool.InputSchema)
		if err != nil {
			s.logger.Error("failed to marshal tool input schema", "error", err, "tool", tool.Name)
			inputSchemaJSON = []byte("{}")
		}
		outputSchemaJSON, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			s.logger.Error("failed to marshal tool output schema", "error", err, "tool", tool.Name)
			outputSchemaJSON = []byte("{}")
		}

		protoTools[i] = &pb.HarnessToolDescriptor{
			Name:             tool.Name,
			Description:      tool.Description,
			SchemaJson:       string(inputSchemaJSON),                  // Legacy field for backward compatibility
			OutputSchemaJson: string(outputSchemaJSON),                 // Legacy output schema
			InputSchema:      SchemaToCallbackProto(tool.InputSchema),  // Structured schema with taxonomy
			OutputSchema:     SchemaToCallbackProto(tool.OutputSchema), // Structured output schema with taxonomy
		}
	}

	return &pb.ListToolsResponse{
		Tools: protoTools,
	}, nil
}

// ============================================================================
// Plugin Operations
// ============================================================================

// QueryPlugin implements the plugin query RPC.
func (s *HarnessCallbackService) QueryPlugin(ctx context.Context, req *pb.QueryPluginRequest) (*pb.QueryPluginResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize params
	var params map[string]any
	if err := json.Unmarshal([]byte(req.ParamsJson), &params); err != nil {
		return &pb.QueryPluginResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid params JSON: %v", err),
			},
		}, nil
	}

	// Query plugin
	result, err := harness.QueryPlugin(ctx, req.Name, req.Method, params)
	if err != nil {
		s.logger.Error("plugin query failed", "error", err, "plugin", req.Name, "method", req.Method)
		return &pb.QueryPluginResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Serialize result
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &pb.QueryPluginResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: fmt.Sprintf("failed to serialize result: %v", err),
			},
		}, nil
	}

	return &pb.QueryPluginResponse{
		ResultJson: string(resultJSON),
	}, nil
}

// ListPlugins implements the plugin listing RPC.
func (s *HarnessCallbackService) ListPlugins(ctx context.Context, req *pb.ListPluginsRequest) (*pb.ListPluginsResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Get plugin descriptors
	plugins := harness.ListPlugins()

	// Convert to proto
	protoPlugins := make([]*pb.HarnessPluginDescriptor, len(plugins))
	for i, plugin := range plugins {
		// Extract method names from MethodDescriptor slice
		methodNames := make([]string, len(plugin.Methods))
		for j, method := range plugin.Methods {
			methodNames[j] = method.Name
		}

		protoPlugins[i] = &pb.HarnessPluginDescriptor{
			Name:        plugin.Name,
			Description: "",
			Version:     plugin.Version,
			Methods:     methodNames,
		}
	}

	return &pb.ListPluginsResponse{
		Plugins: protoPlugins,
	}, nil
}

// ============================================================================
// Agent Operations
// ============================================================================

// DelegateToAgent implements the agent delegation RPC.
func (s *HarnessCallbackService) DelegateToAgent(ctx context.Context, req *pb.DelegateToAgentRequest) (*pb.DelegateToAgentResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize task
	var task agent.Task
	if err := json.Unmarshal([]byte(req.TaskJson), &task); err != nil {
		return &pb.DelegateToAgentResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid task JSON: %v", err),
			},
		}, nil
	}

	// Delegate to agent
	result, err := harness.DelegateToAgent(ctx, req.Name, task)
	if err != nil {
		s.logger.Error("agent delegation failed", "error", err, "agent", req.Name)
		return &pb.DelegateToAgentResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Serialize result
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &pb.DelegateToAgentResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: fmt.Sprintf("failed to serialize result: %v", err),
			},
		}, nil
	}

	return &pb.DelegateToAgentResponse{
		ResultJson: string(resultJSON),
	}, nil
}

// ListAgents implements the agent listing RPC.
func (s *HarnessCallbackService) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Get agent descriptors
	agents := harness.ListAgents()

	// Convert to proto
	protoAgents := make([]*pb.HarnessAgentDescriptor, len(agents))
	for i, agent := range agents {
		protoAgents[i] = &pb.HarnessAgentDescriptor{
			Name:        agent.Name,
			Version:     agent.Version,
			Description: agent.Description,
			// Note: Capabilities are []string in the AgentDescriptor
			// but we need to convert them appropriately
		}
	}

	return &pb.ListAgentsResponse{
		Agents: protoAgents,
	}, nil
}

// ============================================================================
// Finding Operations
// ============================================================================

// SubmitFinding implements the finding submission RPC.
func (s *HarnessCallbackService) SubmitFinding(ctx context.Context, req *pb.SubmitFindingRequest) (*pb.SubmitFindingResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize finding
	var finding agent.Finding
	if err := json.Unmarshal([]byte(req.FindingJson), &finding); err != nil {
		return &pb.SubmitFindingResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid finding JSON: %v", err),
			},
		}, nil
	}

	// Submit finding
	if err := harness.SubmitFinding(ctx, finding); err != nil {
		s.logger.Error("finding submission failed", "error", err)
		return &pb.SubmitFindingResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	return &pb.SubmitFindingResponse{}, nil
}

// GetFindings implements the finding retrieval RPC.
func (s *HarnessCallbackService) GetFindings(ctx context.Context, req *pb.GetFindingsRequest) (*pb.GetFindingsResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize filter
	var filter FindingFilter
	if err := json.Unmarshal([]byte(req.FilterJson), &filter); err != nil {
		return &pb.GetFindingsResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid filter JSON: %v", err),
			},
		}, nil
	}

	// Get findings
	findings, err := harness.GetFindings(ctx, filter)
	if err != nil {
		s.logger.Error("get findings failed", "error", err)
		return &pb.GetFindingsResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Serialize findings
	findingsJSON := make([]string, len(findings))
	for i, finding := range findings {
		data, err := json.Marshal(finding)
		if err != nil {
			return &pb.GetFindingsResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to serialize finding: %v", err),
				},
			}, nil
		}
		findingsJSON[i] = string(data)
	}

	return &pb.GetFindingsResponse{
		FindingsJson: findingsJSON,
	}, nil
}

// ============================================================================
// Memory Operations
// ============================================================================

// MemoryGet implements the memory get RPC with tier routing.
func (s *HarnessCallbackService) MemoryGet(ctx context.Context, req *pb.MemoryGetRequest) (*pb.MemoryGetResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Default to WORKING tier for backward compatibility
	tier := req.Tier
	if tier == pb.MemoryTier_MEMORY_TIER_UNSPECIFIED {
		tier = pb.MemoryTier_MEMORY_TIER_WORKING
	}

	switch tier {
	case pb.MemoryTier_MEMORY_TIER_WORKING:
		// Working memory: existing logic
		value, found := harness.Memory().Working().Get(req.Key)
		if !found {
			return &pb.MemoryGetResponse{
				Found: false,
			}, nil
		}

		// Serialize value
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return &pb.MemoryGetResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to serialize value: %v", err),
				},
			}, nil
		}

		return &pb.MemoryGetResponse{
			ValueJson: string(valueJSON),
			Found:     true,
		}, nil

	case pb.MemoryTier_MEMORY_TIER_MISSION:
		// Mission memory: use Retrieve method
		item, err := harness.Memory().Mission().Retrieve(ctx, req.Key)
		if err != nil {
			// Check for not found error
			if err.Error() == "memory: item not found" || err.Error() == "not found" {
				return &pb.MemoryGetResponse{
					Found: false,
				}, nil
			}
			return &pb.MemoryGetResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to retrieve from mission memory: %v", err),
				},
			}, nil
		}

		// Serialize value and metadata to JSON
		valueJSON, err := json.Marshal(item.Value)
		if err != nil {
			return &pb.MemoryGetResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to serialize value: %v", err),
				},
			}, nil
		}

		metadataJSON, err := json.Marshal(item.Metadata)
		if err != nil {
			return &pb.MemoryGetResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to serialize metadata: %v", err),
				},
			}, nil
		}

		return &pb.MemoryGetResponse{
			ValueJson:    string(valueJSON),
			MetadataJson: string(metadataJSON),
			Found:        true,
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
		}, nil

	case pb.MemoryTier_MEMORY_TIER_LONG_TERM:
		// Long-term memory does not support Get by key
		return &pb.MemoryGetResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: "Long-term memory does not support Get by key. Use LongTermMemorySearch instead.",
			},
		}, nil

	default:
		return &pb.MemoryGetResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("unknown memory tier: %v", tier),
			},
		}, nil
	}
}

// MemorySet implements the memory set RPC with tier routing.
func (s *HarnessCallbackService) MemorySet(ctx context.Context, req *pb.MemorySetRequest) (*pb.MemorySetResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize value
	var value any
	if err := json.Unmarshal([]byte(req.ValueJson), &value); err != nil {
		return &pb.MemorySetResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid value JSON: %v", err),
			},
		}, nil
	}

	// Default to WORKING tier for backward compatibility
	tier := req.Tier
	if tier == pb.MemoryTier_MEMORY_TIER_UNSPECIFIED {
		tier = pb.MemoryTier_MEMORY_TIER_WORKING
	}

	switch tier {
	case pb.MemoryTier_MEMORY_TIER_WORKING:
		// Working memory: existing logic
		if err := harness.Memory().Working().Set(req.Key, value); err != nil {
			return &pb.MemorySetResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to set value: %v", err),
				},
			}, nil
		}
		return &pb.MemorySetResponse{}, nil

	case pb.MemoryTier_MEMORY_TIER_MISSION:
		// Mission memory: use Store method
		// Deserialize metadata if provided
		var metadata map[string]any
		if req.MetadataJson != "" {
			if err := json.Unmarshal([]byte(req.MetadataJson), &metadata); err != nil {
				return &pb.MemorySetResponse{
					Error: &pb.HarnessError{
						Code:    "INVALID_ARGUMENT",
						Message: fmt.Sprintf("invalid metadata JSON: %v", err),
					},
				}, nil
			}
		}

		if err := harness.Memory().Mission().Store(ctx, req.Key, value, metadata); err != nil {
			return &pb.MemorySetResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to store in mission memory: %v", err),
				},
			}, nil
		}
		return &pb.MemorySetResponse{}, nil

	case pb.MemoryTier_MEMORY_TIER_LONG_TERM:
		// Long-term memory does not support Set by key
		return &pb.MemorySetResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: "Long-term memory does not support Set by key. Use LongTermMemoryStore instead.",
			},
		}, nil

	default:
		return &pb.MemorySetResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("unknown memory tier: %v", tier),
			},
		}, nil
	}
}

// MemoryDelete implements the memory delete RPC with tier routing.
func (s *HarnessCallbackService) MemoryDelete(ctx context.Context, req *pb.MemoryDeleteRequest) (*pb.MemoryDeleteResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Default to WORKING tier for backward compatibility
	tier := req.Tier
	if tier == pb.MemoryTier_MEMORY_TIER_UNSPECIFIED {
		tier = pb.MemoryTier_MEMORY_TIER_WORKING
	}

	switch tier {
	case pb.MemoryTier_MEMORY_TIER_WORKING:
		// Working memory: existing logic
		harness.Memory().Working().Delete(req.Key)
		return &pb.MemoryDeleteResponse{}, nil

	case pb.MemoryTier_MEMORY_TIER_MISSION:
		// Mission memory: use Delete method
		if err := harness.Memory().Mission().Delete(ctx, req.Key); err != nil {
			return &pb.MemoryDeleteResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to delete from mission memory: %v", err),
				},
			}, nil
		}
		return &pb.MemoryDeleteResponse{}, nil

	case pb.MemoryTier_MEMORY_TIER_LONG_TERM:
		// Long-term memory does not support Delete by key
		return &pb.MemoryDeleteResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: "Long-term memory does not support Delete by key. Use LongTermMemoryDelete instead.",
			},
		}, nil

	default:
		return &pb.MemoryDeleteResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("unknown memory tier: %v", tier),
			},
		}, nil
	}
}

// MemoryList implements the memory list RPC with tier routing.
func (s *HarnessCallbackService) MemoryList(ctx context.Context, req *pb.MemoryListRequest) (*pb.MemoryListResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Default to WORKING tier for backward compatibility
	tier := req.Tier
	if tier == pb.MemoryTier_MEMORY_TIER_UNSPECIFIED {
		tier = pb.MemoryTier_MEMORY_TIER_WORKING
	}

	switch tier {
	case pb.MemoryTier_MEMORY_TIER_WORKING:
		// List keys from working memory
		// Note: The proto request has a prefix field, but the working memory List() doesn't support prefix filtering
		// We'll get all keys and filter by prefix if needed
		allKeys := harness.Memory().Working().List()

		// Filter by prefix if provided
		var keys []string
		if req.Prefix != "" {
			for _, key := range allKeys {
				if len(key) >= len(req.Prefix) && key[:len(req.Prefix)] == req.Prefix {
					keys = append(keys, key)
				}
			}
		} else {
			keys = allKeys
		}

		return &pb.MemoryListResponse{
			Keys: keys,
		}, nil

	case pb.MemoryTier_MEMORY_TIER_MISSION:
		// Mission memory: use Keys method
		allKeys, err := harness.Memory().Mission().Keys(ctx)
		if err != nil {
			return &pb.MemoryListResponse{
				Error: &pb.HarnessError{
					Code:    "INTERNAL",
					Message: fmt.Sprintf("failed to list keys from mission memory: %v", err),
				},
			}, nil
		}

		// Filter by prefix if provided
		var keys []string
		if req.Prefix != "" {
			for _, key := range allKeys {
				if len(key) >= len(req.Prefix) && key[:len(req.Prefix)] == req.Prefix {
					keys = append(keys, key)
				}
			}
		} else {
			keys = allKeys
		}

		return &pb.MemoryListResponse{
			Keys: keys,
		}, nil

	case pb.MemoryTier_MEMORY_TIER_LONG_TERM:
		// Long-term memory does not support listing keys
		return &pb.MemoryListResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: "Long-term memory does not support listing keys.",
			},
		}, nil

	default:
		return &pb.MemoryListResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("unknown memory tier: %v", tier),
			},
		}, nil
	}
}

// LongTermMemoryStore implements the long-term memory store RPC.
func (s *HarnessCallbackService) LongTermMemoryStore(ctx context.Context, req *pb.LongTermMemoryStoreRequest) (*pb.LongTermMemoryStoreResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize metadata
	var metadata map[string]any
	if req.MetadataJson != "" {
		if err := json.Unmarshal([]byte(req.MetadataJson), &metadata); err != nil {
			return &pb.LongTermMemoryStoreResponse{
				Error: &pb.HarnessError{Code: "INVALID_ARGUMENT", Message: fmt.Sprintf("invalid metadata JSON: %v", err)},
			}, nil
		}
	}

	// Generate UUID for the content - SDK interface returns ID, daemon requires ID input
	id := uuid.New().String()

	// Daemon's LongTermMemory.Store takes (ctx, id, content, metadata)
	err = harness.Memory().LongTerm().Store(ctx, id, req.Content, metadata)
	if err != nil {
		return &pb.LongTermMemoryStoreResponse{
			Error: &pb.HarnessError{Code: "INTERNAL", Message: err.Error()},
		}, nil
	}

	return &pb.LongTermMemoryStoreResponse{Id: id}, nil
}

// LongTermMemorySearch implements the long-term memory search RPC.
func (s *HarnessCallbackService) LongTermMemorySearch(ctx context.Context, req *pb.LongTermMemorySearchRequest) (*pb.LongTermMemorySearchResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize filters
	var filters map[string]any
	if req.FiltersJson != "" {
		if err := json.Unmarshal([]byte(req.FiltersJson), &filters); err != nil {
			return &pb.LongTermMemorySearchResponse{
				Error: &pb.HarnessError{Code: "INVALID_ARGUMENT", Message: fmt.Sprintf("invalid filters JSON: %v", err)},
			}, nil
		}
	}

	results, err := harness.Memory().LongTerm().Search(ctx, req.Query, int(req.TopK), filters)
	if err != nil {
		return &pb.LongTermMemorySearchResponse{
			Error: &pb.HarnessError{Code: "INTERNAL", Message: err.Error()},
		}, nil
	}

	pbResults := make([]*pb.LongTermMemoryResult, len(results))
	for i, r := range results {
		metadataJSON, _ := json.Marshal(r.Item.Metadata)
		pbResults[i] = &pb.LongTermMemoryResult{
			Id:           r.Item.Key,
			Content:      r.Item.Value.(string), // Content is stored as string
			MetadataJson: string(metadataJSON),
			Score:        r.Score,
			CreatedAt:    r.Item.CreatedAt.Format(time.RFC3339),
		}
	}

	return &pb.LongTermMemorySearchResponse{Results: pbResults}, nil
}

// LongTermMemoryDelete implements the long-term memory delete RPC.
func (s *HarnessCallbackService) LongTermMemoryDelete(ctx context.Context, req *pb.LongTermMemoryDeleteRequest) (*pb.LongTermMemoryDeleteResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	err = harness.Memory().LongTerm().Delete(ctx, req.Id)
	if err != nil {
		return &pb.LongTermMemoryDeleteResponse{
			Error: &pb.HarnessError{Code: "INTERNAL", Message: err.Error()},
		}, nil
	}

	return &pb.LongTermMemoryDeleteResponse{}, nil
}

// MissionMemorySearch implements the mission memory search RPC.
func (s *HarnessCallbackService) MissionMemorySearch(ctx context.Context, req *pb.MissionMemorySearchRequest) (*pb.MissionMemorySearchResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	results, err := harness.Memory().Mission().Search(ctx, req.Query, int(req.Limit))
	if err != nil {
		return &pb.MissionMemorySearchResponse{
			Error: &pb.HarnessError{Code: "INTERNAL", Message: err.Error()},
		}, nil
	}

	pbResults := make([]*pb.MissionMemoryResult, len(results))
	for i, r := range results {
		valueJSON, _ := json.Marshal(r.Item.Value)
		metadataJSON, _ := json.Marshal(r.Item.Metadata)
		pbResults[i] = &pb.MissionMemoryResult{
			Key:          r.Item.Key,
			ValueJson:    string(valueJSON),
			MetadataJson: string(metadataJSON),
			Score:        r.Score,
			CreatedAt:    r.Item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    r.Item.UpdatedAt.Format(time.RFC3339),
		}
	}

	return &pb.MissionMemorySearchResponse{Results: pbResults}, nil
}

// MissionMemoryHistory implements the mission memory history RPC.
func (s *HarnessCallbackService) MissionMemoryHistory(ctx context.Context, req *pb.MissionMemoryHistoryRequest) (*pb.MissionMemoryHistoryResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	items, err := harness.Memory().Mission().History(ctx, int(req.Limit))
	if err != nil {
		return &pb.MissionMemoryHistoryResponse{
			Error: &pb.HarnessError{Code: "INTERNAL", Message: err.Error()},
		}, nil
	}

	pbItems := make([]*pb.MissionMemoryItem, len(items))
	for i, item := range items {
		valueJSON, _ := json.Marshal(item.Value)
		metadataJSON, _ := json.Marshal(item.Metadata)
		pbItems[i] = &pb.MissionMemoryItem{
			Key:          item.Key,
			ValueJson:    string(valueJSON),
			MetadataJson: string(metadataJSON),
			CreatedAt:    item.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
		}
	}

	return &pb.MissionMemoryHistoryResponse{Items: pbItems}, nil
}

// MissionMemoryGetPreviousRunValue implements the mission memory get previous run value RPC.
func (s *HarnessCallbackService) MissionMemoryGetPreviousRunValue(ctx context.Context, req *pb.MissionMemoryGetPreviousRunValueRequest) (*pb.MissionMemoryGetPreviousRunValueResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	value, err := harness.Memory().Mission().GetPreviousRunValue(ctx, req.Key)
	if err != nil {
		// Check for specific errors
		errMsg := err.Error()
		return &pb.MissionMemoryGetPreviousRunValueResponse{
			Found: false,
			Error: &pb.HarnessError{Code: "NOT_FOUND", Message: errMsg},
		}, nil
	}

	valueJSON, _ := json.Marshal(value)
	return &pb.MissionMemoryGetPreviousRunValueResponse{
		ValueJson: string(valueJSON),
		Found:     true,
	}, nil
}

// MissionMemoryGetValueHistory implements the mission memory get value history RPC.
func (s *HarnessCallbackService) MissionMemoryGetValueHistory(ctx context.Context, req *pb.MissionMemoryGetValueHistoryRequest) (*pb.MissionMemoryGetValueHistoryResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	history, err := harness.Memory().Mission().GetValueHistory(ctx, req.Key)
	if err != nil {
		return &pb.MissionMemoryGetValueHistoryResponse{
			Error: &pb.HarnessError{Code: "INTERNAL", Message: err.Error()},
		}, nil
	}

	pbValues := make([]*pb.HistoricalValueItem, len(history))
	for i, h := range history {
		valueJSON, _ := json.Marshal(h.Value)
		pbValues[i] = &pb.HistoricalValueItem{
			ValueJson: string(valueJSON),
			RunNumber: int32(h.RunNumber),
			MissionId: h.MissionID,
			StoredAt:  h.StoredAt.Format(time.RFC3339),
		}
	}

	return &pb.MissionMemoryGetValueHistoryResponse{Values: pbValues}, nil
}

// MissionMemoryContinuityMode implements the mission memory continuity mode RPC.
func (s *HarnessCallbackService) MissionMemoryContinuityMode(ctx context.Context, req *pb.MissionMemoryContinuityModeRequest) (*pb.MissionMemoryContinuityModeResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	mode := harness.Memory().Mission().ContinuityMode()
	return &pb.MissionMemoryContinuityModeResponse{
		Mode: string(mode),
	}, nil
}

// ============================================================================
// GraphRAG Query Operations
// ============================================================================

// GraphRAGSupport interface for harnesses that support GraphRAG operations.
// The DefaultAgentHarness and MiddlewareHarness implement these methods.
type GraphRAGSupport interface {
	QueryGraphRAG(ctx context.Context, query sdkgraphrag.Query) ([]sdkgraphrag.Result, error)
	FindSimilarAttacks(ctx context.Context, content string, topK int) ([]sdkgraphrag.AttackPattern, error)
	FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]sdkgraphrag.FindingNode, error)
	GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]sdkgraphrag.AttackChain, error)
	GetRelatedFindings(ctx context.Context, findingID string) ([]sdkgraphrag.FindingNode, error)
	StoreGraphNode(ctx context.Context, node sdkgraphrag.GraphNode) (string, error)
	CreateGraphRelationship(ctx context.Context, rel sdkgraphrag.Relationship) error
	StoreGraphBatch(ctx context.Context, batch sdkgraphrag.Batch) ([]string, error)
	TraverseGraph(ctx context.Context, startNodeID string, opts sdkgraphrag.TraversalOptions) ([]sdkgraphrag.TraversalResult, error)
	GraphRAGHealth(ctx context.Context) types.HealthStatus
}

// GraphRAGQuery implements the GraphRAG query RPC.
func (s *HarnessCallbackService) GraphRAGQuery(ctx context.Context, req *pb.GraphRAGQueryRequest) (*pb.GraphRAGQueryResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Check if harness supports GraphRAG
	graphRAG, ok := harness.(GraphRAGSupport)
	if !ok {
		return &pb.GraphRAGQueryResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: "GraphRAG not supported by this harness",
			},
		}, nil
	}

	// Deserialize query
	var query sdkgraphrag.Query
	if err := json.Unmarshal([]byte(req.QueryJson), &query); err != nil {
		return &pb.GraphRAGQueryResponse{
			Error: &pb.HarnessError{
				Code:    "INVALID_ARGUMENT",
				Message: fmt.Sprintf("invalid query JSON: %v", err),
			},
		}, nil
	}

	// Execute query
	results, err := graphRAG.QueryGraphRAG(ctx, query)
	if err != nil {
		s.logger.Error("GraphRAG query failed", "error", err)
		return &pb.GraphRAGQueryResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert results to proto
	protoResults := make([]*pb.GraphRAGResult, len(results))
	for i, result := range results {
		protoResults[i] = &pb.GraphRAGResult{
			Node:        s.graphNodeToProto(result.Node),
			Score:       result.Score,
			VectorScore: result.VectorScore,
			GraphScore:  result.GraphScore,
			Path:        result.Path,
			Distance:    int32(result.Distance),
		}
	}

	return &pb.GraphRAGQueryResponse{
		Results: protoResults,
	}, nil
}

// FindSimilarAttacks implements the find similar attacks RPC.
func (s *HarnessCallbackService) FindSimilarAttacks(ctx context.Context, req *pb.FindSimilarAttacksRequest) (*pb.FindSimilarAttacksResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.FindSimilarAttacksResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Find similar attacks
	attacks, err := graphRAG.FindSimilarAttacks(ctx, req.Content, int(req.TopK))
	if err != nil {
		s.logger.Error("find similar attacks failed", "error", err)
		return &pb.FindSimilarAttacksResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert to proto
	protoAttacks := make([]*pb.AttackPattern, len(attacks))
	for i, attack := range attacks {
		protoAttacks[i] = &pb.AttackPattern{
			TechniqueId: attack.TechniqueID,
			Name:        attack.Name,
			Description: attack.Description,
			Tactics:     attack.Tactics,
			Platforms:   attack.Platforms,
			Similarity:  attack.Similarity,
		}
	}

	return &pb.FindSimilarAttacksResponse{
		Attacks: protoAttacks,
	}, nil
}

// FindSimilarFindings implements the find similar findings RPC.
func (s *HarnessCallbackService) FindSimilarFindings(ctx context.Context, req *pb.FindSimilarFindingsRequest) (*pb.FindSimilarFindingsResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.FindSimilarFindingsResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Find similar findings
	findings, err := graphRAG.FindSimilarFindings(ctx, req.FindingId, int(req.TopK))
	if err != nil {
		s.logger.Error("find similar findings failed", "error", err)
		return &pb.FindSimilarFindingsResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert to proto
	protoFindings := make([]*pb.FindingNode, len(findings))
	for i, finding := range findings {
		protoFindings[i] = &pb.FindingNode{
			Id:          finding.ID,
			Title:       finding.Title,
			Description: finding.Description,
			Severity:    finding.Severity,
			Category:    finding.Category,
			Confidence:  finding.Confidence,
			Similarity:  finding.Similarity,
		}
	}

	return &pb.FindSimilarFindingsResponse{
		Findings: protoFindings,
	}, nil
}

// GetAttackChains implements the get attack chains RPC.
func (s *HarnessCallbackService) GetAttackChains(ctx context.Context, req *pb.GetAttackChainsRequest) (*pb.GetAttackChainsResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.GetAttackChainsResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Get attack chains
	chains, err := graphRAG.GetAttackChains(ctx, req.TechniqueId, int(req.MaxDepth))
	if err != nil {
		s.logger.Error("get attack chains failed", "error", err)
		return &pb.GetAttackChainsResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert to proto
	protoChains := make([]*pb.AttackChain, len(chains))
	for i, chain := range chains {
		protoSteps := make([]*pb.AttackStep, len(chain.Steps))
		for j, step := range chain.Steps {
			protoSteps[j] = &pb.AttackStep{
				Order:       int32(step.Order),
				TechniqueId: step.TechniqueID,
				NodeId:      step.NodeID,
				Description: step.Description,
				Confidence:  step.Confidence,
			}
		}

		protoChains[i] = &pb.AttackChain{
			Id:       chain.ID,
			Name:     chain.Name,
			Severity: chain.Severity,
			Steps:    protoSteps,
		}
	}

	return &pb.GetAttackChainsResponse{
		Chains: protoChains,
	}, nil
}

// GetRelatedFindings implements the get related findings RPC.
func (s *HarnessCallbackService) GetRelatedFindings(ctx context.Context, req *pb.GetRelatedFindingsRequest) (*pb.GetRelatedFindingsResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.GetRelatedFindingsResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Get related findings
	findings, err := graphRAG.GetRelatedFindings(ctx, req.FindingId)
	if err != nil {
		s.logger.Error("get related findings failed", "error", err)
		return &pb.GetRelatedFindingsResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert to proto
	protoFindings := make([]*pb.FindingNode, len(findings))
	for i, finding := range findings {
		protoFindings[i] = &pb.FindingNode{
			Id:          finding.ID,
			Title:       finding.Title,
			Description: finding.Description,
			Severity:    finding.Severity,
			Category:    finding.Category,
			Confidence:  finding.Confidence,
			Similarity:  finding.Similarity,
		}
	}

	return &pb.GetRelatedFindingsResponse{
		Findings: protoFindings,
	}, nil
}

// ============================================================================
// GraphRAG Storage Operations
// ============================================================================

// StoreGraphNode implements the store graph node RPC.
func (s *HarnessCallbackService) StoreGraphNode(ctx context.Context, req *pb.StoreGraphNodeRequest) (*pb.StoreGraphNodeResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.StoreGraphNodeResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert proto node to SDK node
	node := s.protoToGraphNode(req.Node)

	// Store node
	nodeID, err := graphRAG.StoreGraphNode(ctx, node)
	if err != nil {
		s.logger.Error("store graph node failed", "error", err)
		return &pb.StoreGraphNodeResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	return &pb.StoreGraphNodeResponse{
		NodeId: nodeID,
	}, nil
}

// CreateGraphRelationship implements the create graph relationship RPC.
func (s *HarnessCallbackService) CreateGraphRelationship(ctx context.Context, req *pb.CreateGraphRelationshipRequest) (*pb.CreateGraphRelationshipResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.CreateGraphRelationshipResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert proto relationship to SDK relationship
	rel := s.protoToRelationship(req.Relationship)

	// Create relationship
	if err := graphRAG.CreateGraphRelationship(ctx, rel); err != nil {
		s.logger.Error("create graph relationship failed", "error", err)
		return &pb.CreateGraphRelationshipResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	return &pb.CreateGraphRelationshipResponse{}, nil
}

// StoreGraphBatch implements the store graph batch RPC.
func (s *HarnessCallbackService) StoreGraphBatch(ctx context.Context, req *pb.StoreGraphBatchRequest) (*pb.StoreGraphBatchResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.StoreGraphBatchResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert proto batch to SDK batch
	batch := sdkgraphrag.Batch{
		Nodes:         make([]sdkgraphrag.GraphNode, len(req.Nodes)),
		Relationships: make([]sdkgraphrag.Relationship, len(req.Relationships)),
	}

	for i, protoNode := range req.Nodes {
		batch.Nodes[i] = s.protoToGraphNode(protoNode)
	}

	for i, protoRel := range req.Relationships {
		batch.Relationships[i] = s.protoToRelationship(protoRel)
	}

	// Store batch
	nodeIDs, err := graphRAG.StoreGraphBatch(ctx, batch)
	if err != nil {
		s.logger.Error("store graph batch failed", "error", err)
		return &pb.StoreGraphBatchResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	return &pb.StoreGraphBatchResponse{
		NodeIds: nodeIDs,
	}, nil
}

// TraverseGraph implements the traverse graph RPC.
func (s *HarnessCallbackService) TraverseGraph(ctx context.Context, req *pb.TraverseGraphRequest) (*pb.TraverseGraphResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return &pb.TraverseGraphResponse{
			Error: &pb.HarnessError{
				Code:    "UNIMPLEMENTED",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert proto options to SDK options
	opts := sdkgraphrag.TraversalOptions{
		MaxDepth:          int(req.Options.MaxDepth),
		RelationshipTypes: req.Options.RelationshipTypes,
		NodeTypes:         req.Options.NodeTypes,
		Direction:         req.Options.Direction,
	}

	// Traverse graph
	results, err := graphRAG.TraverseGraph(ctx, req.StartNodeId, opts)
	if err != nil {
		s.logger.Error("traverse graph failed", "error", err)
		return &pb.TraverseGraphResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: err.Error(),
			},
		}, nil
	}

	// Convert results to proto
	protoResults := make([]*pb.TraversalResult, len(results))
	for i, result := range results {
		protoResults[i] = &pb.TraversalResult{
			Node:     s.graphNodeToProto(result.Node),
			Path:     result.Path,
			Distance: int32(result.Distance),
		}
	}

	return &pb.TraverseGraphResponse{
		Results: protoResults,
	}, nil
}

// GraphRAGHealth implements the GraphRAG health check RPC.
func (s *HarnessCallbackService) GraphRAGHealth(ctx context.Context, req *pb.GraphRAGHealthRequest) (*pb.GraphRAGHealthResponse, error) {
	graphRAG, err := s.getGraphRAGHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Get health status
	healthStatus := graphRAG.GraphRAGHealth(ctx)

	return &pb.GraphRAGHealthResponse{
		Status: &pb.HarnessHealthStatus{
			State:   string(healthStatus.State),
			Message: healthStatus.Message,
		},
	}, nil
}

// ============================================================================
// Distributed Tracing Operations
// ============================================================================

// RecordSpan implements the span recording RPC for distributed tracing.
// It receives spans from remote agents and forwards them to registered span processors.
func (s *HarnessCallbackService) RecordSpan(ctx context.Context, req *pb.RecordSpanRequest) (*pb.RecordSpanResponse, error) {
	if req.Span == nil {
		return &pb.RecordSpanResponse{}, nil
	}

	// Convert proto span to span data and export
	spanData := s.protoToSpanData(req.Span)
	s.exportSpanData(spanData)

	return &pb.RecordSpanResponse{}, nil
}

// RecordSpans implements the batch span recording RPC for distributed tracing.
// It receives multiple spans from remote agents and forwards them to registered span processors.
func (s *HarnessCallbackService) RecordSpans(ctx context.Context, req *pb.RecordSpansRequest) (*pb.RecordSpansResponse, error) {
	for _, protoSpan := range req.Spans {
		if protoSpan == nil {
			continue
		}

		// Convert proto span to span data and export
		spanData := s.protoToSpanData(protoSpan)
		s.exportSpanData(spanData)
	}

	return &pb.RecordSpansResponse{}, nil
}

// exportSpanData creates a real span using the TracerProvider and immediately ends it.
// This allows the span to be processed by registered span processors.
func (s *HarnessCallbackService) exportSpanData(data *proxySpanData) {
	if data == nil || s.tracerProvider == nil {
		return
	}

	// Create parent context with the original trace context
	parentCtx := trace.ContextWithSpanContext(context.Background(), data.Parent)

	// Get tracer from the provider
	tracer := s.tracerProvider.Tracer("gibson-agent-proxy")

	// Create span with original attributes and timing
	_, span := tracer.Start(parentCtx, data.Name,
		trace.WithSpanKind(data.SpanKind),
		trace.WithTimestamp(data.StartTime),
		trace.WithAttributes(data.Attributes...),
	)

	// Add events
	for _, event := range data.Events {
		span.AddEvent(event.Name, trace.WithTimestamp(event.Time), trace.WithAttributes(event.Attributes...))
	}

	// Set status
	span.SetStatus(otelcodes.Code(data.Status.Code), data.Status.Description)

	// End span with original end time
	span.End(trace.WithTimestamp(data.EndTime))
}

// exportSpan forwards a span to all registered span processors.
// Deprecated: Use exportSpanData instead for proxy spans.
func (s *HarnessCallbackService) exportSpan(span sdktrace.ReadOnlySpan) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, processor := range s.spanProcessors {
		processor.OnEnd(span)
	}
}

// ============================================================================
// Helper Methods for Proto Conversions
// ============================================================================

func (s *HarnessCallbackService) protoToMessages(protoMessages []*pb.LLMMessage) []llm.Message {
	messages := make([]llm.Message, len(protoMessages))
	for i, protoMsg := range protoMessages {
		msg := llm.Message{
			Role:    llm.Role(protoMsg.Role),
			Content: protoMsg.Content,
			Name:    protoMsg.Name,
		}

		if len(protoMsg.ToolCalls) > 0 {
			msg.ToolCalls = make([]llm.ToolCall, len(protoMsg.ToolCalls))
			for j, protoCall := range protoMsg.ToolCalls {
				msg.ToolCalls[j] = llm.ToolCall{
					ID:        protoCall.Id,
					Name:      protoCall.Name,
					Arguments: protoCall.Arguments,
				}
			}
		}

		// Note: proto has ToolResults but internal llm.Message doesn't have ToolResults
		// Tool results are represented differently in the internal API

		messages[i] = msg
	}
	return messages
}

func (s *HarnessCallbackService) toolCallsToProto(calls []llm.ToolCall) []*pb.ToolCall {
	protoCalls := make([]*pb.ToolCall, len(calls))
	for i, call := range calls {
		protoCalls[i] = &pb.ToolCall{
			Id:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		}
	}
	return protoCalls
}

func (s *HarnessCallbackService) protoToToolDefs(protoTools []*pb.ToolDef) []llm.ToolDef {
	tools := make([]llm.ToolDef, len(protoTools))
	for i, protoTool := range protoTools {
		// Parameters are stored as JSON string in proto, unmarshal to JSONSchema
		var params schema.JSON
		if protoTool.ParametersJson != "" {
			if err := json.Unmarshal([]byte(protoTool.ParametersJson), &params); err != nil {
				s.logger.Error("failed to unmarshal tool parameters", "error", err, "tool", protoTool.Name)
				// Continue with empty params rather than failing entirely
			}
		}

		tools[i] = llm.ToolDef{
			Name:        protoTool.Name,
			Description: protoTool.Description,
			Parameters:  params,
		}
	}
	return tools
}

func (s *HarnessCallbackService) graphNodeToProto(node sdkgraphrag.GraphNode) *pb.GraphNode {
	propsJSON, err := json.Marshal(node.Properties)
	if err != nil {
		s.logger.Error("failed to marshal graph node properties", "error", err, "node_id", node.ID)
		propsJSON = []byte("{}")
	}
	return &pb.GraphNode{
		Id:             node.ID,
		Type:           node.Type,
		PropertiesJson: string(propsJSON),
		Content:        node.Content,
		MissionId:      node.MissionID,
		AgentName:      node.AgentName,
		CreatedAt:      node.CreatedAt.Unix(),
		UpdatedAt:      node.UpdatedAt.Unix(),
	}
}

func (s *HarnessCallbackService) protoToGraphNode(protoNode *pb.GraphNode) sdkgraphrag.GraphNode {
	var props map[string]any
	if protoNode.PropertiesJson != "" {
		if err := json.Unmarshal([]byte(protoNode.PropertiesJson), &props); err != nil {
			s.logger.Error("failed to unmarshal graph node properties", "error", err, "node_id", protoNode.Id)
			// Continue with empty props
		}
	}

	return sdkgraphrag.GraphNode{
		ID:         protoNode.Id,
		Type:       protoNode.Type,
		Properties: props,
		Content:    protoNode.Content,
		MissionID:  protoNode.MissionId,
		AgentName:  protoNode.AgentName,
	}
}

func (s *HarnessCallbackService) protoToRelationship(protoRel *pb.Relationship) sdkgraphrag.Relationship {
	var props map[string]any
	if protoRel.PropertiesJson != "" {
		if err := json.Unmarshal([]byte(protoRel.PropertiesJson), &props); err != nil {
			s.logger.Error("failed to unmarshal relationship properties", "error", err, "from", protoRel.FromId, "to", protoRel.ToId)
			// Continue with empty props
		}
	}

	return sdkgraphrag.Relationship{
		FromID:        protoRel.FromId,
		ToID:          protoRel.ToId,
		Type:          protoRel.Type,
		Properties:    props,
		Bidirectional: protoRel.Bidirectional,
	}
}

// protoToSpanData converts a proto Span to a proxySpanData container.
// Since sdktrace.ReadOnlySpan has an unexported method, we can't implement it directly.
// Instead, we extract the data and export it directly via the Langfuse exporter.
func (s *HarnessCallbackService) protoToSpanData(protoSpan *pb.Span) *proxySpanData {
	// Parse trace ID and span ID from hex strings
	var traceID trace.TraceID
	var spanID trace.SpanID
	var parentSpanID trace.SpanID

	// Convert hex string to TraceID (16 bytes)
	if len(protoSpan.TraceId) == 32 {
		for i := 0; i < 16; i++ {
			_, _ = fmt.Sscanf(protoSpan.TraceId[i*2:i*2+2], "%02x", &traceID[i])
		}
	}

	// Convert hex string to SpanID (8 bytes)
	if len(protoSpan.SpanId) == 16 {
		for i := 0; i < 8; i++ {
			_, _ = fmt.Sscanf(protoSpan.SpanId[i*2:i*2+2], "%02x", &spanID[i])
		}
	}

	// Convert parent span ID if present
	hasParent := false
	if len(protoSpan.ParentSpanId) == 16 {
		hasParent = true
		for i := 0; i < 8; i++ {
			_, _ = fmt.Sscanf(protoSpan.ParentSpanId[i*2:i*2+2], "%02x", &parentSpanID[i])
		}
	}

	// Create span context
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})

	// Create parent span context
	var parentContext trace.SpanContext
	if hasParent {
		parentContext = trace.NewSpanContext(trace.SpanContextConfig{
			TraceID: traceID,
			SpanID:  parentSpanID,
		})
	}

	// Create proxy span data container
	return s.createProxySpanData(
		protoSpan.Name,
		spanContext,
		parentContext,
		s.protoSpanKindToOtel(protoSpan.Kind),
		time.Unix(0, protoSpan.StartTimeUnixNano),
		time.Unix(0, protoSpan.EndTimeUnixNano),
		s.protoAttributesToOtel(protoSpan.Attributes),
		s.protoEventsToOtel(protoSpan.Events),
		sdktrace.Status{
			Code:        s.protoStatusCodeToOtel(protoSpan.StatusCode),
			Description: protoSpan.StatusMessage,
		},
	)
}

// protoSpanKindToOtel converts proto SpanKind to OpenTelemetry SpanKind.
func (s *HarnessCallbackService) protoSpanKindToOtel(kind pb.SpanKind) trace.SpanKind {
	switch kind {
	case pb.SpanKind_SPAN_KIND_INTERNAL:
		return trace.SpanKindInternal
	case pb.SpanKind_SPAN_KIND_SERVER:
		return trace.SpanKindServer
	case pb.SpanKind_SPAN_KIND_CLIENT:
		return trace.SpanKindClient
	case pb.SpanKind_SPAN_KIND_PRODUCER:
		return trace.SpanKindProducer
	case pb.SpanKind_SPAN_KIND_CONSUMER:
		return trace.SpanKindConsumer
	default:
		return trace.SpanKindUnspecified
	}
}

// protoStatusCodeToOtel converts proto StatusCode to OpenTelemetry status code.
func (s *HarnessCallbackService) protoStatusCodeToOtel(code pb.StatusCode) otelcodes.Code {
	switch code {
	case pb.StatusCode_STATUS_CODE_OK:
		return otelcodes.Ok
	case pb.StatusCode_STATUS_CODE_ERROR:
		return otelcodes.Error
	default:
		return otelcodes.Unset
	}
}

// protoAttributesToOtel converts proto KeyValue attributes to OpenTelemetry attributes.
func (s *HarnessCallbackService) protoAttributesToOtel(protoAttrs []*pb.KeyValue) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(protoAttrs))
	for _, protoAttr := range protoAttrs {
		if protoAttr.Value == nil {
			continue
		}

		key := attribute.Key(protoAttr.Key)
		// Handle different value types from AnyValue
		switch v := protoAttr.Value.Value.(type) {
		case *pb.AnyValue_StringValue:
			attrs = append(attrs, key.String(v.StringValue))
		case *pb.AnyValue_BoolValue:
			attrs = append(attrs, key.Bool(v.BoolValue))
		case *pb.AnyValue_IntValue:
			attrs = append(attrs, key.Int64(v.IntValue))
		case *pb.AnyValue_DoubleValue:
			attrs = append(attrs, key.Float64(v.DoubleValue))
		}
	}
	return attrs
}

// protoEventsToOtel converts proto SpanEvents to OpenTelemetry Events.
func (s *HarnessCallbackService) protoEventsToOtel(protoEvents []*pb.SpanEvent) []sdktrace.Event {
	events := make([]sdktrace.Event, len(protoEvents))
	for i, protoEvent := range protoEvents {
		events[i] = sdktrace.Event{
			Name:       protoEvent.Name,
			Time:       time.Unix(0, protoEvent.TimeUnixNano),
			Attributes: s.protoAttributesToOtel(protoEvent.Attributes),
		}
	}
	return events
}

// ============================================================================
// Span Data Container
// ============================================================================

// proxySpanData contains the data extracted from a proto span for export.
// Since sdktrace.ReadOnlySpan has an unexported private() method that prevents
// external implementation, we store the span data and export directly to Langfuse.
type proxySpanData struct {
	Name        string
	SpanContext trace.SpanContext
	Parent      trace.SpanContext
	SpanKind    trace.SpanKind
	StartTime   time.Time
	EndTime     time.Time
	Attributes  []attribute.KeyValue
	Events      []sdktrace.Event
	Status      sdktrace.Status
}

// createProxySpanData creates a proxySpanData from the proto span data.
func (s *HarnessCallbackService) createProxySpanData(
	name string,
	spanContext trace.SpanContext,
	parent trace.SpanContext,
	spanKind trace.SpanKind,
	startTime time.Time,
	endTime time.Time,
	attributes []attribute.KeyValue,
	events []sdktrace.Event,
	status sdktrace.Status,
) *proxySpanData {
	return &proxySpanData{
		Name:        name,
		SpanContext: spanContext,
		Parent:      parent,
		SpanKind:    spanKind,
		StartTime:   startTime,
		EndTime:     endTime,
		Attributes:  attributes,
		Events:      events,
		Status:      status,
	}
}

// ============================================================================
// Credential Operations
// ============================================================================

// GetCredential retrieves a credential by name from the credential store.
// The credential is decrypted and returned with its secret value.
func (s *HarnessCallbackService) GetCredential(ctx context.Context, req *pb.GetCredentialRequest) (*pb.GetCredentialResponse, error) {
	// Validate context
	if req.Context == nil {
		return &pb.GetCredentialResponse{
			Error: &pb.HarnessError{
				Code:    codes.InvalidArgument.String(),
				Message: "missing context info in request",
			},
		}, nil
	}

	// Log request
	s.logger.Debug("GetCredential request",
		"name", req.Name,
		"mission_id", req.Context.MissionId,
		"agent_name", req.Context.AgentName,
	)

	// Check if credential store is configured
	if s.credentialStore == nil {
		s.logger.Warn("GetCredential called but credential store not configured")
		return &pb.GetCredentialResponse{
			Error: &pb.HarnessError{
				Code:    codes.Unavailable.String(),
				Message: "credential store not available",
			},
		}, nil
	}

	// Retrieve credential
	cred, secret, err := s.credentialStore.GetCredential(ctx, req.Name)
	if err != nil {
		s.logger.Warn("GetCredential failed", "name", req.Name, "error", err)
		return &pb.GetCredentialResponse{
			Error: &pb.HarnessError{
				Code:    codes.NotFound.String(),
				Message: fmt.Sprintf("credential %q not found: %v", req.Name, err),
			},
		}, nil
	}

	// Map internal credential type to proto type
	var credType pb.CredentialType
	switch cred.Type {
	case types.CredentialTypeAPIKey:
		credType = pb.CredentialType_CREDENTIAL_TYPE_API_KEY
	case types.CredentialTypeBearer:
		credType = pb.CredentialType_CREDENTIAL_TYPE_BEARER
	case types.CredentialTypeBasic:
		credType = pb.CredentialType_CREDENTIAL_TYPE_BASIC
	case types.CredentialTypeOAuth:
		credType = pb.CredentialType_CREDENTIAL_TYPE_OAUTH
	case types.CredentialTypeCustom:
		credType = pb.CredentialType_CREDENTIAL_TYPE_CUSTOM
	default:
		credType = pb.CredentialType_CREDENTIAL_TYPE_API_KEY
	}

	// Encode metadata as JSON
	var metadataJSON string
	if cred.Tags != nil || cred.Provider != "" {
		metadata := map[string]any{
			"provider": cred.Provider,
			"tags":     cred.Tags,
		}
		metadataBytes, err := json.Marshal(metadata)
		if err == nil {
			metadataJSON = string(metadataBytes)
		}
	}

	s.logger.Debug("GetCredential succeeded", "name", req.Name)

	return &pb.GetCredentialResponse{
		Credential: &pb.Credential{
			Name:         cred.Name,
			Type:         credType,
			Secret:       secret,
			MetadataJson: metadataJSON,
		},
	}, nil
}

// ============================================================================
// Helper Methods for Taxonomy Engine Integration
// ============================================================================

// extractAgentRunID extracts the agent run ID from context.
// Tries multiple sources: trace span ID, mission ID, task ID, or generates a fallback.
func (s *HarnessCallbackService) extractAgentRunID(ctx context.Context, contextInfo *pb.ContextInfo) string {
	// Priority 1: Use trace span ID if available (most specific)
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}

	// Priority 2: Use task ID if available (unique per execution)
	if contextInfo != nil && contextInfo.TaskId != "" {
		return contextInfo.TaskId
	}

	// Priority 3: Use mission ID (less specific but still useful)
	if contextInfo != nil && contextInfo.MissionId != "" {
		return contextInfo.MissionId
	}

	// Fallback: Generate a unique ID
	return uuid.New().String()
}

// publishEvent publishes an event to the event bus if configured.
// This is a helper method that safely publishes events without blocking
// callback responses. Events are published in a goroutine to avoid latency.
func (s *HarnessCallbackService) publishEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.eventBus == nil {
		return // Event bus not configured, skip
	}

	// Extract trace context from OpenTelemetry span
	var traceID, spanID, parentSpanID string
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		traceID = spanCtx.TraceID().String()
		spanID = spanCtx.SpanID().String()
	}

	// Extract parent_span_id from data if provided (for relationship creation)
	// This is passed explicitly by callers who know their parent span
	if psid, ok := data["parent_span_id"].(string); ok {
		parentSpanID = psid
	}

	// IMPORTANT: Add trace context to data map for taxonomy engine
	// The taxonomy engine reads from data map, not the event struct fields
	// This ensures LLMCall, ToolExecution nodes can create relationships
	timestamp := time.Now()
	data["trace_id"] = traceID
	data["span_id"] = spanID
	data["parent_span_id"] = parentSpanID
	data["timestamp"] = timestamp.Format(time.RFC3339Nano)

	// Create event structure matching daemon.GraphEvent
	event := struct {
		Type         string
		TraceID      string
		SpanID       string
		ParentSpanID string
		Timestamp    time.Time
		Data         map[string]interface{}
	}{
		Type:         eventType,
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Timestamp:    timestamp,
		Data:         data,
	}

	// Publish in background to avoid blocking the callback response
	go func() {
		if err := s.eventBus.Publish(context.Background(), event); err != nil {
			s.logger.Warn("failed to publish event",
				"event_type", eventType,
				"error", err,
			)
		}
	}()
}

// ============================================================================
// Taxonomy Operations
// ============================================================================

// GetTaxonomySchema returns the full taxonomy schema to agents.
// NOTE: Taxonomy has been removed. This returns an empty response.
func (s *HarnessCallbackService) GetTaxonomySchema(ctx context.Context, req *pb.GetTaxonomySchemaRequest) (*pb.GetTaxonomySchemaResponse, error) {
	s.logger.Debug("GetTaxonomySchema called (taxonomy removed)")
	return &pb.GetTaxonomySchemaResponse{
		Version: "0.0.0",
	}, nil
}

// GenerateNodeID generates a deterministic node ID.
// NOTE: Taxonomy has been removed. Use domain types instead which generate their own IDs.
func (s *HarnessCallbackService) GenerateNodeID(ctx context.Context, req *pb.GenerateNodeIDRequest) (*pb.GenerateNodeIDResponse, error) {
	s.logger.Debug("GenerateNodeID called (taxonomy removed)")
	return &pb.GenerateNodeIDResponse{
		Error: &pb.HarnessError{
			Code:    codes.Unimplemented.String(),
			Message: "taxonomy has been removed; use domain types which generate their own IDs",
		},
	}, nil
}

// ValidateFinding validates a finding.
// NOTE: Taxonomy-based validation has been removed. Basic validation is still performed.
func (s *HarnessCallbackService) ValidateFinding(ctx context.Context, req *pb.ValidateFindingRequest) (*pb.ValidationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	s.logger.Debug("ValidateFinding called")

	// Parse finding from JSON
	var finding sdkfinding.Finding
	if err := json.Unmarshal([]byte(req.FindingJson), &finding); err != nil {
		return &pb.ValidationResponse{
			Error: &pb.HarnessError{
				Code:    codes.InvalidArgument.String(),
				Message: fmt.Sprintf("failed to parse finding JSON: %v", err),
			},
		}, nil
	}

	resp := &pb.ValidationResponse{Valid: true}

	// Validate severity
	validSeverities := []string{"critical", "high", "medium", "low", "informational"}
	severityValid := false
	for _, sev := range validSeverities {
		if string(finding.Severity) == sev {
			severityValid = true
			break
		}
	}
	if !severityValid && finding.Severity != "" {
		resp.Valid = false
		resp.Errors = append(resp.Errors, &pb.ValidationError{
			Field:   "severity",
			Message: fmt.Sprintf("invalid severity: %s", finding.Severity),
			Code:    "INVALID_ENUM",
		})
	}

	// Validate required fields
	if finding.Title == "" {
		resp.Valid = false
		resp.Errors = append(resp.Errors, &pb.ValidationError{
			Field:   "title",
			Message: "title is required",
			Code:    "MISSING_REQUIRED",
		})
	}

	s.logger.Debug("ValidateFinding completed", "valid", resp.Valid, "errors", len(resp.Errors))

	return resp, nil
}

// ValidateGraphNode validates a graph node.
// NOTE: Taxonomy-based validation has been removed. Returns success with a warning.
func (s *HarnessCallbackService) ValidateGraphNode(ctx context.Context, req *pb.ValidateGraphNodeRequest) (*pb.ValidationResponse, error) {
	s.logger.Debug("ValidateGraphNode called (taxonomy removed)")
	return &pb.ValidationResponse{
		Valid:    true,
		Warnings: []string{"taxonomy-based validation has been removed; use domain types for type-safe node creation"},
	}, nil
}

// ValidateRelationship validates a relationship.
// NOTE: Taxonomy-based validation has been removed. Returns success with a warning.
func (s *HarnessCallbackService) ValidateRelationship(ctx context.Context, req *pb.ValidateRelationshipRequest) (*pb.ValidationResponse, error) {
	s.logger.Debug("ValidateRelationship called (taxonomy removed)")
	return &pb.ValidationResponse{
		Valid:    true,
		Warnings: []string{"taxonomy-based validation has been removed; use domain types for type-safe relationship creation"},
	}, nil
}
