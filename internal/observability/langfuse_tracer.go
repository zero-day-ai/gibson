package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

const (
	// langfuseAPIIngestion is the Langfuse ingestion API endpoint
	langfuseAPIIngestion = "/api/public/ingestion"

	// defaultHTTPTimeout is the default timeout for HTTP requests to Langfuse
	defaultHTTPTimeout = 30 * time.Second
)

// MissionTracer provides mission-aware tracing to Langfuse.
// It creates a hierarchical trace structure that aligns with the orchestrator execution model.
//
// Trace Hierarchy:
//   - Trace: mission-{mission_id}
//     ├── Generation: orchestrator-decision-1
//     │   ├── input: full prompt with graph state
//     │   ├── output: Decision JSON
//     │   └── metadata: {tokens, latency, graph_snapshot}
//     ├── Span: agent-execution-{id}
//     │   ├── Span: tool-call-nmap
//     │   │   └── metadata: {input, output, duration}
//     │   ├── Span: tool-call-httpx
//     │   │   └── metadata: {input, output, duration}
//     │   └── Span: graph-storage
//     │       └── metadata: {nodes_created, relationships}
//     ├── Generation: orchestrator-decision-2
//     │   └── ...
//     └── Span: mission-complete
//         └── metadata: {summary, total_tokens, duration}
type MissionTracer struct {
	client    *http.Client
	host      string
	publicKey string
	secretKey string
	mu        sync.RWMutex // Protects concurrent access to tracer state
}

// NewMissionTracer creates a new mission-aware Langfuse tracer.
//
// Parameters:
//   - cfg: Langfuse configuration with host, public key, and secret key
//
// Returns:
//   - *MissionTracer: The initialized tracer
//   - error: Any error encountered during initialization
func NewMissionTracer(cfg LangfuseConfig) (*MissionTracer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, WrapObservabilityError(ErrAuthenticationFailed, "invalid langfuse configuration", err)
	}

	return &MissionTracer{
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		host:      cfg.Host,
		publicKey: cfg.PublicKey,
		secretKey: cfg.SecretKey,
	}, nil
}

// MissionTrace represents an active mission trace in Langfuse.
// It tracks the trace ID and associated mission metadata for correlation.
type MissionTrace struct {
	TraceID   string         // Langfuse trace ID
	MissionID types.ID       // Gibson mission ID
	Name      string         // Mission name
	StartTime time.Time      // When the trace was started
	Metadata  map[string]any // Additional metadata
}

// DecisionLog captures orchestrator decision information for Langfuse.
type DecisionLog struct {
	Decision      *schema.Decision // The decision node
	Prompt        string           // Full prompt sent to LLM
	Response      string           // Full response from LLM
	Model         string           // LLM model used
	GraphSnapshot string           // Graph state at decision time
	Neo4jNodeID   string           // Neo4j node ID for correlation
}

// AgentExecutionLog captures agent execution information for Langfuse.
type AgentExecutionLog struct {
	Execution   *schema.AgentExecution // The execution node
	AgentName   string                 // Name of the agent
	Config      map[string]any         // Configuration used
	Neo4jNodeID string                 // Neo4j node ID for correlation
	SpanID      string                 // Parent span ID (set by StartAgentExecution)
}

// ToolExecutionLog captures tool execution information for Langfuse.
type ToolExecutionLog struct {
	Execution   *schema.ToolExecution // The tool execution node
	Neo4jNodeID string                // Neo4j node ID for correlation
	SpanID      string                // Span ID for this tool execution
}

// MissionTraceSummary captures final mission statistics for tracing.
// This extends the basic MissionSummary with additional trace-specific fields.
type MissionTraceSummary struct {
	Status          string         // Mission final status
	TotalDecisions  int            // Number of orchestrator decisions
	TotalExecutions int            // Number of agent executions
	TotalTools      int            // Number of tool executions
	TotalTokens     int            // Total LLM tokens used
	TotalCost       float64        // Estimated total cost in USD
	Duration        time.Duration  // Total mission duration
	Outcome         string         // Human-readable outcome summary
	GraphStats      map[string]int // Graph statistics (nodes, relationships, etc.)
}

// StartMissionTrace creates a new trace for a mission in Langfuse.
// This should be called when a mission begins execution.
//
// Parameters:
//   - ctx: Context for the operation
//   - mission: The mission being traced
//
// Returns:
//   - *MissionTrace: The created trace handle
//   - error: Any error encountered during trace creation
func (mt *MissionTracer) StartMissionTrace(ctx context.Context, mission *schema.Mission) (*MissionTrace, error) {
	if mission == nil {
		return nil, fmt.Errorf("mission cannot be nil")
	}

	traceID := fmt.Sprintf("mission-%s", mission.ID.String())
	now := time.Now()

	// Create trace-create event
	event := map[string]any{
		"type":      "trace-create",
		"id":        generateEventID("trace", mission.ID.String()),
		"timestamp": now.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":        traceID,
			"name":      fmt.Sprintf("Mission: %s", mission.Name),
			"timestamp": now.Format(time.RFC3339Nano),
			"metadata": map[string]any{
				"mission_id":   mission.ID.String(),
				"mission_name": mission.Name,
				"objective":    mission.Objective,
				"target_ref":   mission.TargetRef,
				"status":       mission.Status.String(),
			},
		},
	}

	// Send trace creation event
	if err := mt.sendEvent(ctx, event); err != nil {
		return nil, WrapObservabilityError(ErrExporterConnection, "failed to create mission trace", err)
	}

	trace := &MissionTrace{
		TraceID:   traceID,
		MissionID: mission.ID,
		Name:      mission.Name,
		StartTime: now,
		Metadata: map[string]any{
			"objective":  mission.Objective,
			"target_ref": mission.TargetRef,
		},
	}

	return trace, nil
}

// LogDecision logs an orchestrator decision as a Generation in Langfuse.
// Decisions are logged as generations because they involve LLM reasoning.
//
// Parameters:
//   - ctx: Context for the operation
//   - trace: The mission trace to log to
//   - log: Decision information including prompt, response, and metadata
//
// Returns:
//   - error: Any error encountered during logging
func (mt *MissionTracer) LogDecision(ctx context.Context, trace *MissionTrace, log *DecisionLog) error {
	if trace == nil || log == nil || log.Decision == nil {
		return fmt.Errorf("trace, log, and decision cannot be nil")
	}

	decision := log.Decision
	spanID := fmt.Sprintf("decision-%s", decision.ID.String())

	// Calculate latency in seconds for storage
	latencySec := float64(decision.LatencyMs) / 1000.0

	// Create generation-create event
	event := map[string]any{
		"type":      "generation-create",
		"id":        generateEventID("gen", decision.ID.String()),
		"timestamp": decision.Timestamp.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":               spanID,
			"traceId":          trace.TraceID,
			"name":             fmt.Sprintf("orchestrator-decision-%d", decision.Iteration),
			"startTime":        decision.Timestamp.Format(time.RFC3339Nano),
			"endTime":          decision.Timestamp.Add(time.Duration(decision.LatencyMs) * time.Millisecond).Format(time.RFC3339Nano),
			"model":            log.Model,
			"input":            log.Prompt,
			"output":           log.Response,
			"promptTokens":     decision.PromptTokens,
			"completionTokens": decision.CompletionTokens,
			"level":            "DEFAULT",
			"metadata": map[string]any{
				"decision_id":     decision.ID.String(),
				"iteration":       decision.Iteration,
				"action":          decision.Action.String(),
				"target_node_id":  decision.TargetNodeID,
				"reasoning":       decision.Reasoning,
				"confidence":      decision.Confidence,
				"modifications":   decision.Modifications,
				"graph_snapshot":  log.GraphSnapshot,
				"neo4j_node_id":   log.Neo4jNodeID,
				"latency_seconds": latencySec,
				"total_tokens":    decision.TotalTokens(),
			},
		},
	}

	// Send generation event
	if err := mt.sendEvent(ctx, event); err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to log decision", err)
	}

	return nil
}

// LogAgentExecution logs an agent execution as a Span in Langfuse.
// Agent executions contain tool executions as child spans.
//
// Parameters:
//   - ctx: Context for the operation
//   - trace: The mission trace to log to
//   - log: Agent execution information
//
// Returns:
//   - error: Any error encountered during logging
func (mt *MissionTracer) LogAgentExecution(ctx context.Context, trace *MissionTrace, log *AgentExecutionLog) error {
	if trace == nil || log == nil || log.Execution == nil {
		return fmt.Errorf("trace, log, and execution cannot be nil")
	}

	exec := log.Execution
	spanID := fmt.Sprintf("agent-exec-%s", exec.ID.String())

	// Store span ID for child tool executions
	log.SpanID = spanID

	// Determine end time and status level
	endTime := time.Now()
	if exec.CompletedAt != nil {
		endTime = *exec.CompletedAt
	}

	level := "DEFAULT"
	if exec.Status == schema.ExecutionStatusFailed {
		level = "ERROR"
	}

	// Create span-create event
	event := map[string]any{
		"type":      "span-create",
		"id":        generateEventID("span", exec.ID.String()),
		"timestamp": exec.StartedAt.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":        spanID,
			"traceId":   trace.TraceID,
			"name":      fmt.Sprintf("agent-execution-%s", log.AgentName),
			"startTime": exec.StartedAt.Format(time.RFC3339Nano),
			"endTime":   endTime.Format(time.RFC3339Nano),
			"level":     level,
			"metadata": map[string]any{
				"execution_id":     exec.ID.String(),
				"workflow_node_id": exec.WorkflowNodeID,
				"agent_name":       log.AgentName,
				"status":           exec.Status.String(),
				"attempt":          exec.Attempt,
				"config_used":      exec.ConfigUsed,
				"result":           exec.Result,
				"error":            exec.Error,
				"neo4j_node_id":    log.Neo4jNodeID,
				"duration_seconds": exec.Duration().Seconds(),
			},
		},
	}

	// Add error message if failed
	if exec.Error != "" {
		event["body"].(map[string]any)["statusMessage"] = exec.Error
	}

	// Send span event
	if err := mt.sendEvent(ctx, event); err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to log agent execution", err)
	}

	return nil
}

// LogToolExecution logs a tool execution as a child Span in Langfuse.
// Tool executions are nested under their parent agent execution.
//
// Parameters:
//   - ctx: Context for the operation
//   - parentLog: The parent agent execution log (provides parent span ID)
//   - log: Tool execution information
//
// Returns:
//   - error: Any error encountered during logging
func (mt *MissionTracer) LogToolExecution(ctx context.Context, parentLog *AgentExecutionLog, log *ToolExecutionLog) error {
	if parentLog == nil || log == nil || log.Execution == nil {
		return fmt.Errorf("parentLog, log, and execution cannot be nil")
	}

	tool := log.Execution
	spanID := fmt.Sprintf("tool-exec-%s", tool.ID.String())
	log.SpanID = spanID

	// Determine end time and status level
	endTime := time.Now()
	if tool.CompletedAt != nil {
		endTime = *tool.CompletedAt
	}

	level := "DEFAULT"
	if tool.Status == schema.ExecutionStatusFailed {
		level = "ERROR"
	}

	// Create span-create event as child of agent execution
	event := map[string]any{
		"type":      "span-create",
		"id":        generateEventID("span", tool.ID.String()),
		"timestamp": tool.StartedAt.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":                  spanID,
			"traceId":             fmt.Sprintf("mission-%s", tool.AgentExecutionID.String()), // Use mission ID as trace ID
			"parentObservationId": parentLog.SpanID,                                          // Link to parent agent execution
			"name":                fmt.Sprintf("tool-call-%s", tool.ToolName),
			"startTime":           tool.StartedAt.Format(time.RFC3339Nano),
			"endTime":             endTime.Format(time.RFC3339Nano),
			"level":               level,
			"metadata": map[string]any{
				"tool_execution_id": tool.ID.String(),
				"tool_name":         tool.ToolName,
				"input":             tool.Input,
				"output":            tool.Output,
				"status":            tool.Status.String(),
				"error":             tool.Error,
				"neo4j_node_id":     log.Neo4jNodeID,
				"duration_seconds":  tool.Duration().Seconds(),
			},
		},
	}

	// Add error message if failed
	if tool.Error != "" {
		event["body"].(map[string]any)["statusMessage"] = tool.Error
	}

	// Send span event
	if err := mt.sendEvent(ctx, event); err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to log tool execution", err)
	}

	return nil
}

// EndMissionTrace logs the final mission completion span and summary to Langfuse.
// This should be called when a mission completes or fails.
//
// Parameters:
//   - ctx: Context for the operation
//   - trace: The mission trace to finalize
//   - summary: Mission completion summary with statistics
//
// Returns:
//   - error: Any error encountered during logging
func (mt *MissionTracer) EndMissionTrace(ctx context.Context, trace *MissionTrace, summary *MissionTraceSummary) error {
	if trace == nil || summary == nil {
		return fmt.Errorf("trace and summary cannot be nil")
	}

	now := time.Now()
	spanID := fmt.Sprintf("mission-complete-%s", trace.MissionID.String())

	// Determine level based on status
	level := "DEFAULT"
	if summary.Status == schema.MissionStatusFailed.String() {
		level = "ERROR"
	}

	// Create completion span
	event := map[string]any{
		"type":      "span-create",
		"id":        generateEventID("span", trace.MissionID.String()),
		"timestamp": now.Format(time.RFC3339Nano),
		"body": map[string]any{
			"id":        spanID,
			"traceId":   trace.TraceID,
			"name":      "mission-complete",
			"startTime": trace.StartTime.Format(time.RFC3339Nano),
			"endTime":   now.Format(time.RFC3339Nano),
			"level":     level,
			"metadata": map[string]any{
				"mission_id":        trace.MissionID.String(),
				"mission_name":      trace.Name,
				"status":            summary.Status,
				"total_decisions":   summary.TotalDecisions,
				"total_executions":  summary.TotalExecutions,
				"total_tools":       summary.TotalTools,
				"total_tokens":      summary.TotalTokens,
				"total_cost_usd":    summary.TotalCost,
				"duration_seconds":  summary.Duration.Seconds(),
				"outcome":           summary.Outcome,
				"graph_stats":       summary.GraphStats,
			},
		},
	}

	// Send completion span
	if err := mt.sendEvent(ctx, event); err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to end mission trace", err)
	}

	return nil
}

// sendEvent sends a single event to the Langfuse ingestion API.
// Events are sent immediately without batching for real-time observability.
//
// Parameters:
//   - ctx: Context for the HTTP request
//   - event: The event to send (must be a valid Langfuse ingestion event)
//
// Returns:
//   - error: Any error encountered during sending
func (mt *MissionTracer) sendEvent(ctx context.Context, event map[string]any) error {
	mt.mu.RLock()
	defer mt.mu.RUnlock()

	// Wrap event in batch format expected by Langfuse API
	payload := map[string]any{
		"batch": []map[string]any{event},
	}

	// Marshal to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create HTTP request
	url := mt.host + langfuseAPIIngestion
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(mt.publicKey, mt.secretKey)

	// Send request
	resp, err := mt.client.Do(req)
	if err != nil {
		return NewExporterConnectionError(url, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewExporterConnectionError(url, fmt.Errorf("HTTP %d: request failed", resp.StatusCode))
	}

	return nil
}

// generateEventID creates a unique event ID for Langfuse ingestion.
// The event ID is used to deduplicate events on the Langfuse side.
//
// Parameters:
//   - prefix: Event type prefix (e.g., "trace", "span", "gen")
//   - id: The ID to use for generating the event ID
//
// Returns:
//   - string: The generated event ID
func generateEventID(prefix, id string) string {
	// Use first 12 chars of ID to keep event IDs reasonably short
	shortID := id
	if len(id) > 12 {
		shortID = id[:12]
	}
	return fmt.Sprintf("%s-%s-%d", prefix, shortID, time.Now().UnixNano())
}

// Close shuts down the MissionTracer and releases resources.
// This should be called when the tracer is no longer needed.
func (mt *MissionTracer) Close() error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Close HTTP client transport if it has one
	if mt.client != nil && mt.client.Transport != nil {
		if transport, ok := mt.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	return nil
}
