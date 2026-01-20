package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

const (
	// langfuseAPIIngestion is the Langfuse ingestion API endpoint
	langfuseAPIIngestion = "/api/public/ingestion"

	// defaultHTTPTimeout is the default timeout for HTTP requests to Langfuse
	defaultHTTPTimeout = 30 * time.Second
)

// LangfuseMetrics tracks Langfuse operation statistics
type LangfuseMetrics struct {
	EventsQueued     int64     `json:"events_queued"`
	EventsSent       int64     `json:"events_sent"`
	EventsFailed     int64     `json:"events_failed"`
	BytesSent        int64     `json:"bytes_sent"`
	LastFlushTime    time.Time `json:"last_flush_time"`
	LastSuccessTime  time.Time `json:"last_success_time"`
	LastErrorTime    time.Time `json:"last_error_time,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
	ConsecutiveFails int       `json:"consecutive_fails"`
}

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
//     └── metadata: {summary, total_tokens, duration}
type MissionTracer struct {
	client           *http.Client
	host             string
	publicKey        string
	secretKey        string
	mu               sync.RWMutex    // Protects concurrent access to tracer state and health fields
	lastError        error           // Last connectivity error
	lastSuccess      time.Time       // Last successful connection
	consecutiveFails int             // Consecutive failed connection attempts
	metrics          LangfuseMetrics // Operation metrics
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

// MessageLog represents a single message in an LLM conversation for Langfuse.
// This provides structured message data for observability and debugging.
type MessageLog struct {
	Role       string `json:"role"`                   // Message role (system, user, assistant, tool)
	Content    string `json:"content"`                // Message content
	Name       string `json:"name,omitempty"`         // Optional name (for function/tool messages)
	ToolCallID string `json:"tool_call_id,omitempty"` // Optional tool call ID reference
}

// RequestMetadata captures LLM request configuration for observability.
// This enables full visibility into the parameters used for each LLM call.
type RequestMetadata struct {
	Model       string  `json:"model"`                // LLM model name
	Temperature float64 `json:"temperature"`          // Temperature parameter (0.0-1.0)
	MaxTokens   int     `json:"max_tokens"`           // Maximum tokens to generate
	TopP        float64 `json:"top_p,omitempty"`      // Top-p nucleus sampling parameter
	SlotName    string  `json:"slot_name"`            // LLM slot name used
	Provider    string  `json:"provider,omitempty"`   // LLM provider (e.g., "openai", "anthropic")
}

// DecisionLog captures orchestrator decision information for Langfuse.
type DecisionLog struct {
	Decision      *schema.Decision // The decision node
	Prompt        string           // Full prompt sent to LLM
	Response      string           // Full response from LLM
	Model         string           // LLM model used
	GraphSnapshot string           // Graph state at decision time
	Neo4jNodeID   string           // Neo4j node ID for correlation
	OTELTraceID   string           // OTEL trace ID for correlation with infrastructure traces

	// Messages contains the structured message array sent to the LLM
	// This provides detailed visibility into the conversation structure
	Messages []MessageLog `json:"messages,omitempty"`

	// RequestMeta contains the LLM request configuration parameters
	// This enables full observability of decision-making settings
	RequestMeta *RequestMetadata `json:"request_meta,omitempty"`
}

// AgentExecutionLog captures agent execution information for Langfuse.
type AgentExecutionLog struct {
	Execution   *schema.AgentExecution // The execution node
	AgentName   string                 // Name of the agent
	Config      map[string]any         // Configuration used
	Neo4jNodeID string                 // Neo4j node ID for correlation
	SpanID      string                 // Parent span ID (set by StartAgentExecution)
	OTELTraceID string                 // OTEL trace ID for correlation with infrastructure traces
}

// ToolExecutionLog captures tool execution information for Langfuse.
type ToolExecutionLog struct {
	Execution   *schema.ToolExecution // The tool execution node
	Neo4jNodeID string                // Neo4j node ID for correlation
	SpanID      string                // Span ID for this tool execution
	OTELTraceID string                // OTEL trace ID for correlation with infrastructure traces
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

	// Extract OTEL trace ID from context for correlation
	otelTraceID := extractOTELTraceIDFromContext(ctx)

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
				"mission_id":    mission.ID.String(),
				"mission_name":  mission.Name,
				"objective":     mission.Objective,
				"target_ref":    mission.TargetRef,
				"status":        mission.Status.String(),
				"otel_trace_id": otelTraceID,
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
				"otel_trace_id":   log.OTELTraceID,
				"messages":        log.Messages,     // Structured message array
				"request_config":  log.RequestMeta,  // Request configuration parameters
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
				"otel_trace_id":    log.OTELTraceID,
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
				"otel_trace_id":     log.OTELTraceID,
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
				"mission_id":       trace.MissionID.String(),
				"mission_name":     trace.Name,
				"status":           summary.Status,
				"total_decisions":  summary.TotalDecisions,
				"total_executions": summary.TotalExecutions,
				"total_tools":      summary.TotalTools,
				"total_tokens":     summary.TotalTokens,
				"total_cost_usd":   summary.TotalCost,
				"duration_seconds": summary.Duration.Seconds(),
				"outcome":          summary.Outcome,
				"graph_stats":      summary.GraphStats,
			},
		},
	}

	// Send completion span
	if err := mt.sendEvent(ctx, event); err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to end mission trace", err)
	}

	return nil
}

// SendEvent sends a custom event to the Langfuse ingestion API.
// This is a public wrapper around sendEvent for agent-level tracing from middleware.
// Events are sent immediately without batching for real-time observability.
//
// Use this method when you need to send custom generation or span events that don't
// fit the high-level Log* methods (e.g., agent-level LLM calls, tool calls from harness).
//
// Parameters:
//   - ctx: Context for the HTTP request
//   - event: The event to send (must be a valid Langfuse ingestion event)
//
// Returns:
//   - error: Any error encountered during sending
func (mt *MissionTracer) SendEvent(ctx context.Context, event map[string]any) error {
	return mt.sendEvent(ctx, event)
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
	// Read lock for sending - we'll upgrade for health tracking
	mt.mu.RLock()
	host := mt.host
	publicKey := mt.publicKey
	secretKey := mt.secretKey
	mt.mu.RUnlock()

	// Extract event type for logging
	eventType := "unknown"
	if t, ok := event["type"].(string); ok {
		eventType = t
	}

	// Wrap event in batch format expected by Langfuse API
	payload := map[string]any{
		"batch": []map[string]any{event},
	}

	// Marshal to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Track queued event
	mt.mu.Lock()
	mt.metrics.EventsQueued++
	mt.mu.Unlock()

	// Create HTTP request
	url := host + langfuseAPIIngestion
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(publicKey, secretKey)

	// Send request
	resp, err := mt.client.Do(req)
	if err != nil {
		slog.Error("langfuse HTTP request failed",
			"url", url,
			"error", err,
			"event_type", eventType,
		)

		// Track connectivity failure and metrics
		mt.mu.Lock()
		mt.lastError = err
		mt.consecutiveFails++
		mt.metrics.EventsFailed++
		mt.metrics.LastErrorTime = time.Now()
		mt.metrics.LastError = err.Error()
		mt.metrics.ConsecutiveFails++
		mt.mu.Unlock()

		return NewExporterConnectionError(url, err)
	}
	defer resp.Body.Close()

	// Read response body for debugging (limit to 4KB)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respErr := fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		slog.Error("langfuse HTTP request returned error",
			"url", url,
			"status", resp.StatusCode,
			"body", string(body),
			"event_type", eventType,
		)

		// Track connectivity failure and metrics
		mt.mu.Lock()
		mt.lastError = respErr
		mt.consecutiveFails++
		mt.metrics.EventsFailed++
		mt.metrics.LastErrorTime = time.Now()
		mt.metrics.LastError = respErr.Error()
		mt.metrics.ConsecutiveFails++
		mt.mu.Unlock()

		return NewExporterConnectionError(url, respErr)
	}

	slog.Debug("langfuse event sent successfully",
		"event_type", eventType,
		"status", resp.StatusCode,
	)

	// Track successful connection and metrics
	mt.mu.Lock()
	mt.lastSuccess = time.Now()
	mt.consecutiveFails = 0
	mt.lastError = nil
	mt.metrics.EventsSent++
	mt.metrics.BytesSent += int64(len(data))
	mt.metrics.LastSuccessTime = time.Now()
	mt.metrics.ConsecutiveFails = 0
	mt.mu.Unlock()

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

// GetMetrics returns a copy of current Langfuse metrics.
// This method is safe for concurrent access and provides a snapshot
// of the current metrics state.
//
// Returns:
//   - LangfuseMetrics: A copy of the current metrics
func (mt *MissionTracer) GetMetrics() LangfuseMetrics {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.metrics // Return copy
}

// extractOTELTraceIDFromContext extracts the OpenTelemetry trace ID from context.
// This enables correlation between Langfuse traces and infrastructure traces in Grafana/Jaeger.
// Returns empty string if no valid OTEL span is found in the context.
func extractOTELTraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// CheckConnectivity verifies that the MissionTracer can reach the Langfuse API.
// This method sends a lightweight HTTP request to the Langfuse ingestion endpoint
// to validate connectivity without creating any trace data.
//
// The method uses HEAD request to minimize bandwidth and avoid creating test data.
// If HEAD is not supported (405), it falls back to GET with immediate cancellation.
//
// This is intended for startup health checks and should not be called frequently.
//
// Parameters:
//   - ctx: Context for the HTTP request
//
// Returns:
//   - error: Any error encountered during connectivity check
func (mt *MissionTracer) CheckConnectivity(ctx context.Context) error {
	mt.mu.RLock()
	host := mt.host
	publicKey := mt.publicKey
	secretKey := mt.secretKey
	mt.mu.RUnlock()

	// Try HEAD request first (most efficient)
	url := host + langfuseAPIIngestion
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create connectivity check request: %w", err)
	}

	// Set authentication headers
	req.SetBasicAuth(publicKey, secretKey)

	// Send request
	resp, err := mt.client.Do(req)
	if err != nil {
		slog.Debug("langfuse connectivity check failed",
			"url", url,
			"method", "HEAD",
			"error", err,
		)

		// Track connectivity failure
		mt.mu.Lock()
		mt.lastError = err
		mt.consecutiveFails++
		mt.mu.Unlock()

		return NewExporterConnectionError(url, err)
	}
	defer resp.Body.Close()

	// Check if HEAD is supported
	if resp.StatusCode == http.StatusMethodNotAllowed {
		// Fall back to GET
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create fallback connectivity check request: %w", err)
		}
		req.SetBasicAuth(publicKey, secretKey)

		resp, err = mt.client.Do(req)
		if err != nil {
			slog.Debug("langfuse connectivity check failed",
				"url", url,
				"method", "GET",
				"error", err,
			)

			// Track connectivity failure
			mt.mu.Lock()
			mt.lastError = err
			mt.consecutiveFails++
			mt.mu.Unlock()

			return NewExporterConnectionError(url, err)
		}
		defer resp.Body.Close()
	}

	// Accept any 2xx or 4xx status (4xx means we reached the server but auth/validation failed)
	// We consider 4xx as "connected" since the server is reachable
	if resp.StatusCode >= 500 {
		respErr := fmt.Errorf("server error: HTTP %d", resp.StatusCode)
		slog.Debug("langfuse connectivity check returned server error",
			"url", url,
			"status", resp.StatusCode,
		)

		// Track connectivity failure
		mt.mu.Lock()
		mt.lastError = respErr
		mt.consecutiveFails++
		mt.mu.Unlock()

		return NewExporterConnectionError(url, respErr)
	}

	slog.Debug("langfuse connectivity check successful",
		"url", url,
		"status", resp.StatusCode,
	)

	// Track successful connection
	mt.mu.Lock()
	mt.lastSuccess = time.Now()
	mt.consecutiveFails = 0
	mt.lastError = nil
	mt.mu.Unlock()

	return nil
}

// IsHealthy returns true if Langfuse has been reachable recently (within 5 minutes).
// This method checks the timestamp of the last successful connection to determine health.
//
// Returns:
//   - bool: true if healthy, false otherwise
func (mt *MissionTracer) IsHealthy() bool {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return time.Since(mt.lastSuccess) < 5*time.Minute
}

// LastError returns the last connectivity error, or nil if healthy.
// This method provides access to the most recent error encountered.
//
// Returns:
//   - error: The last error, or nil if no recent errors
func (mt *MissionTracer) LastError() error {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.lastError
}

// LastSuccessTime returns when Langfuse was last successfully reached.
// This method provides the timestamp of the most recent successful connection.
//
// Returns:
//   - time.Time: Timestamp of last successful connection
func (mt *MissionTracer) LastSuccessTime() time.Time {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.lastSuccess
}
