package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/schema"
	pb "github.com/zero-day-ai/sdk/api/gen/proto"
	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
	sdktypes "github.com/zero-day-ai/sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HarnessCallbackService implements the gRPC HarnessCallbackService server.
// It receives harness operation requests from remote agents via gRPC and
// delegates them to the appropriate registered harness instance.
//
// The service supports two modes of harness lookup:
//  1. Task-based lookup (legacy): Uses activeHarnesses sync.Map keyed by task ID
//  2. Mission-based lookup (new): Uses CallbackHarnessRegistry keyed by "missionID:agentName"
//
// When an agent running in standalone mode makes a harness call, the SDK's
// CallbackClient sends a gRPC request with context information (task ID or
// mission ID + agent name), which is used to look up the correct harness instance.
//
// To register this service with a gRPC server:
//
//	service := harness.NewHarnessCallbackService(logger)
//	pb.RegisterHarnessCallbackServiceServer(grpcServer, service)
//
// Before executing an agent task, register its harness:
//
//	service.RegisterHarness(taskID, harness)
//	defer service.UnregisterHarness(taskID)
type HarnessCallbackService struct {
	pb.UnimplementedHarnessCallbackServiceServer

	// activeHarnesses maps task IDs to their corresponding harness instances (legacy mode)
	activeHarnesses sync.Map // map[string]AgentHarness

	// registry provides mission-based harness lookup for external agents (new mode)
	registry *CallbackHarnessRegistry

	// logger for service-level logging
	logger *slog.Logger
}

// NewHarnessCallbackService creates a new callback service instance with
// task-based harness lookup (legacy mode).
func NewHarnessCallbackService(logger *slog.Logger) *HarnessCallbackService {
	if logger == nil {
		logger = slog.Default()
	}

	return &HarnessCallbackService{
		logger: logger.With("component", "harness_callback_service"),
	}
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
//
// Returns:
//   - *HarnessCallbackService: A new service instance ready to be registered
func NewHarnessCallbackServiceWithRegistry(logger *slog.Logger, registry *CallbackHarnessRegistry) *HarnessCallbackService {
	if logger == nil {
		logger = slog.Default()
	}

	return &HarnessCallbackService{
		registry: registry,
		logger:   logger.With("component", "harness_callback_service"),
	}
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
// This method supports two lookup modes:
//  1. Mission-based (preferred): Uses registry with missionID + agentName
//  2. Task-based (legacy): Uses activeHarnesses sync.Map with taskID
//
// The method tries mission-based lookup first if both missionID and agentName
// are provided. If the registry doesn't find a harness, it falls back to
// task-based lookup using the taskID.
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

	// Try mission-based lookup first if registry is configured
	// Note: Mission-based lookup requires both mission ID and agent name in the context.
	// The mission ID is typically extracted from the task ID (format: "missionID:taskID")
	// or passed explicitly in a future version of the protocol.
	if s.registry != nil && contextInfo.TaskId != "" && contextInfo.AgentName != "" {
		// Extract mission ID from task ID if it follows the format "missionID:taskName"
		// For now, we'll use the task_id as-is since the registry registration
		// happens with the actual mission ID extracted from the harness context.
		// This is a temporary approach until we add mission_id to the ContextInfo proto.

		// Try to extract mission ID from task ID (if it contains a colon separator)
		taskParts := strings.Split(contextInfo.TaskId, ":")
		if len(taskParts) >= 2 {
			missionID := taskParts[0]
			harness, err := s.registry.Lookup(missionID, contextInfo.AgentName)
			if err == nil {
				s.logger.Debug("harness lookup succeeded via mission-based registry",
					"mission_id", missionID,
					"agent_name", contextInfo.AgentName,
					"task_id", contextInfo.TaskId,
					"lookup_mode", "mission-based",
				)
				return harness, nil
			}

			s.logger.Debug("mission-based harness lookup failed, trying task-based fallback",
				"mission_id", missionID,
				"agent_name", contextInfo.AgentName,
				"task_id", contextInfo.TaskId,
				"error", err,
			)
		}
	}

	// Fall back to task-based lookup (legacy mode)
	if contextInfo.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "missing task ID and mission ID/agent name in request context")
	}

	taskID := contextInfo.TaskId
	value, ok := s.activeHarnesses.Load(taskID)
	if !ok {
		s.logger.Error("harness not found for task", "task_id", taskID)
		return nil, status.Errorf(codes.NotFound, "no active harness for task: %s", taskID)
	}

	harness, ok := value.(AgentHarness)
	if !ok {
		s.logger.Error("invalid harness type", "task_id", taskID)
		return nil, status.Error(codes.Internal, "invalid harness type")
	}

	s.logger.Debug("harness lookup succeeded",
		"task_id", taskID,
		"lookup_mode", "task-based",
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

// ============================================================================
// Tool Operations
// ============================================================================

// CallTool implements the tool execution RPC.
func (s *HarnessCallbackService) CallTool(ctx context.Context, req *pb.CallToolRequest) (*pb.CallToolResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Deserialize input
	var input map[string]any
	if err := json.Unmarshal([]byte(req.InputJson), &input); err != nil {
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
		return &pb.CallToolResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: fmt.Sprintf("failed to serialize output: %v", err),
			},
		}, nil
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

	// Convert to proto
	protoTools := make([]*pb.HarnessToolDescriptor, len(tools))
	for i, tool := range tools {
		schemaJSON, err := json.Marshal(tool.InputSchema)
		if err != nil {
			s.logger.Error("failed to marshal tool schema", "error", err, "tool", tool.Name)
			schemaJSON = []byte("{}")
		}
		protoTools[i] = &pb.HarnessToolDescriptor{
			Name:        tool.Name,
			Description: tool.Description,
			SchemaJson:  string(schemaJSON),
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

// MemoryGet implements the memory get RPC.
func (s *HarnessCallbackService) MemoryGet(ctx context.Context, req *pb.MemoryGetRequest) (*pb.MemoryGetResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Get value from working memory
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
}

// MemorySet implements the memory set RPC.
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

	// Set value in working memory
	if err := harness.Memory().Working().Set(req.Key, value); err != nil {
		return &pb.MemorySetResponse{
			Error: &pb.HarnessError{
				Code:    "INTERNAL",
				Message: fmt.Sprintf("failed to set value: %v", err),
			},
		}, nil
	}

	return &pb.MemorySetResponse{}, nil
}

// MemoryDelete implements the memory delete RPC.
func (s *HarnessCallbackService) MemoryDelete(ctx context.Context, req *pb.MemoryDeleteRequest) (*pb.MemoryDeleteResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

	// Delete from working memory
	harness.Memory().Working().Delete(req.Key)

	return &pb.MemoryDeleteResponse{}, nil
}

// MemoryList implements the memory list RPC.
func (s *HarnessCallbackService) MemoryList(ctx context.Context, req *pb.MemoryListRequest) (*pb.MemoryListResponse, error) {
	harness, err := s.getHarness(ctx, req.Context)
	if err != nil {
		return nil, err
	}

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
}

// ============================================================================
// GraphRAG Query Operations
// ============================================================================

// GraphRAGSupport interface for harnesses that support GraphRAG operations.
// The DefaultAgentHarness implements these methods.
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
	GraphRAGHealth(ctx context.Context) sdktypes.HealthStatus
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
			State:   healthStatus.Status,
			Message: healthStatus.Message,
		},
	}, nil
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
		var params schema.JSONSchema
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
