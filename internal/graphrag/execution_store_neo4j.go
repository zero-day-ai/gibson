package graphrag

import (
	"context"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
)

// Neo4jExecutionGraphStore implements ExecutionGraphStore using Neo4j as the backend.
// It records execution traces, performance metrics, and agent lifecycles in a graph database
// for observability, analysis, and cross-mission correlation.
//
// The store creates the following node types:
//   - Mission: Top-level mission execution nodes
//   - AgentExecution: Agent execution instances with trace correlation
//   - LLMCall: Individual LLM API calls with cost and performance metrics
//   - ToolCall: Tool execution events with success/failure tracking
//
// Relationships:
//   - Mission -[:HAS_EXECUTION]-> AgentExecution
//   - AgentExecution -[:MADE_LLM_CALL]-> LLMCall
//   - AgentExecution -[:CALLED_TOOL]-> ToolCall
//   - AgentExecution -[:DELEGATED_TO]-> AgentExecution
//
// Thread-safety: This implementation is safe for concurrent access as the underlying
// Neo4j client handles session pooling and transaction management.
type Neo4jExecutionGraphStore struct {
	client graphClient
}

// graphClient is an interface that defines the subset of GraphClient methods
// needed by the execution store. This allows for easier mocking in tests.
type graphClient interface {
	ExecuteWrite(ctx context.Context, cypher string, params map[string]any) (graph.QueryResult, error)
}

// NewNeo4jExecutionGraphStore creates a new Neo4j-backed execution graph store.
// The provided client must already be connected to Neo4j.
func NewNeo4jExecutionGraphStore(client graphClient) *Neo4jExecutionGraphStore {
	return &Neo4jExecutionGraphStore{
		client: client,
	}
}

// RecordMissionStart records the start of a mission execution.
// Creates or updates a Mission node with trace correlation and start timestamp.
func (s *Neo4jExecutionGraphStore) RecordMissionStart(ctx context.Context, event MissionStartEvent) error {
	cypher := `
		MERGE (m:Mission {id: $mission_id})
		SET m.name = $name,
		    m.trace_id = $trace_id,
		    m.started_at = $started_at,
		    m.status = 'running'
		RETURN m
	`

	params := map[string]any{
		"mission_id": event.MissionID.String(),
		"name":       event.MissionName,
		"trace_id":   event.TraceID,
		"started_at": event.StartedAt.Unix(),
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record mission start in Neo4j",
			"mission_id", event.MissionID,
			"error", err)
		// Don't fail the mission if graph recording fails
		return nil
	}

	return nil
}

// RecordMissionComplete records the completion of a mission.
// Updates the Mission node with final status, findings count, and duration.
func (s *Neo4jExecutionGraphStore) RecordMissionComplete(ctx context.Context, event MissionCompleteEvent) error {
	cypher := `
		MATCH (m:Mission {id: $mission_id})
		SET m.status = $status,
		    m.findings_count = $findings_count,
		    m.completed_at = $completed_at,
		    m.duration_ms = $duration_ms
		RETURN m
	`

	params := map[string]any{
		"mission_id":     event.MissionID.String(),
		"status":         event.Status,
		"findings_count": event.FindingsCount,
		"completed_at":   event.CompletedAt.Unix(),
		"duration_ms":    event.DurationMs,
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record mission complete in Neo4j",
			"mission_id", event.MissionID,
			"error", err)
		return nil
	}

	return nil
}

// RecordAgentStart records the start of an agent's execution.
// Creates an AgentExecution node and links it to the parent Mission.
func (s *Neo4jExecutionGraphStore) RecordAgentStart(ctx context.Context, event AgentStartEvent) error {
	cypher := `
		MERGE (a:AgentExecution {trace_id: $trace_id, span_id: $span_id})
		SET a.agent_name = $agent_name,
		    a.task_id = $task_id,
		    a.mission_id = $mission_id,
		    a.started_at = $started_at,
		    a.status = 'running'
		WITH a
		OPTIONAL MATCH (m:Mission {id: $mission_id})
		FOREACH (_ IN CASE WHEN m IS NOT NULL THEN [1] ELSE [] END |
		    MERGE (m)-[:HAS_EXECUTION]->(a)
		)
		RETURN a
	`

	params := map[string]any{
		"trace_id":   event.TraceID,
		"span_id":    event.SpanID,
		"agent_name": event.AgentName,
		"task_id":    event.TaskID,
		"mission_id": event.MissionID.String(),
		"started_at": event.StartedAt.Unix(),
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record agent start in Neo4j",
			"agent_name", event.AgentName,
			"trace_id", event.TraceID,
			"error", err)
		return nil
	}

	return nil
}

// RecordAgentComplete records the completion of an agent's execution.
// Updates the AgentExecution node with final status, findings count, and duration.
func (s *Neo4jExecutionGraphStore) RecordAgentComplete(ctx context.Context, event AgentCompleteEvent) error {
	cypher := `
		MATCH (a:AgentExecution {trace_id: $trace_id, span_id: $span_id})
		SET a.status = $status,
		    a.findings_count = $findings_count,
		    a.completed_at = $completed_at,
		    a.duration_ms = $duration_ms
		RETURN a
	`

	params := map[string]any{
		"trace_id":       event.TraceID,
		"span_id":        event.SpanID,
		"status":         event.Status,
		"findings_count": event.FindingsCount,
		"completed_at":   event.CompletedAt.Unix(),
		"duration_ms":    event.DurationMs,
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record agent complete in Neo4j",
			"agent_name", event.AgentName,
			"trace_id", event.TraceID,
			"error", err)
		return nil
	}

	return nil
}

// RecordLLMCall records a single LLM API call.
// Creates an LLMCall node and links it to the executing agent.
func (s *Neo4jExecutionGraphStore) RecordLLMCall(ctx context.Context, event LLMCallEvent) error {
	cypher := `
		MERGE (l:LLMCall {trace_id: $trace_id, span_id: $span_id})
		SET l.model = $model,
		    l.provider = $provider,
		    l.slot = $slot,
		    l.tokens_in = $tokens_in,
		    l.tokens_out = $tokens_out,
		    l.cost_usd = $cost_usd,
		    l.latency_ms = $latency_ms,
		    l.timestamp = $timestamp
		WITH l
		OPTIONAL MATCH (a:AgentExecution {mission_id: $mission_id})
		WHERE a.agent_name = $agent_name AND a.status = 'running'
		WITH l, a
		ORDER BY a.started_at DESC
		LIMIT 1
		FOREACH (_ IN CASE WHEN a IS NOT NULL THEN [1] ELSE [] END |
		    MERGE (a)-[:MADE_LLM_CALL]->(l)
		)
		RETURN l
	`

	params := map[string]any{
		"trace_id":   event.TraceID,
		"span_id":    event.SpanID,
		"model":      event.Model,
		"provider":   event.Provider,
		"slot":       event.Slot,
		"tokens_in":  event.TokensIn,
		"tokens_out": event.TokensOut,
		"cost_usd":   event.CostUSD,
		"latency_ms": event.LatencyMs,
		"timestamp":  event.Timestamp.Unix(),
		"mission_id": event.MissionID.String(),
		"agent_name": event.AgentName,
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record LLM call in Neo4j",
			"agent_name", event.AgentName,
			"model", event.Model,
			"trace_id", event.TraceID,
			"error", err)
		return nil
	}

	return nil
}

// RecordToolCall records a single tool execution.
// Creates a ToolCall node and links it to the executing agent.
func (s *Neo4jExecutionGraphStore) RecordToolCall(ctx context.Context, event ToolCallEvent) error {
	cypher := `
		MERGE (t:ToolCall {trace_id: $trace_id, span_id: $span_id})
		SET t.tool_name = $tool_name,
		    t.duration_ms = $duration_ms,
		    t.success = $success,
		    t.error = $error,
		    t.timestamp = $timestamp
		WITH t
		OPTIONAL MATCH (a:AgentExecution {mission_id: $mission_id})
		WHERE a.agent_name = $agent_name AND a.status = 'running'
		WITH t, a
		ORDER BY a.started_at DESC
		LIMIT 1
		FOREACH (_ IN CASE WHEN a IS NOT NULL THEN [1] ELSE [] END |
		    MERGE (a)-[:CALLED_TOOL]->(t)
		)
		RETURN t
	`

	params := map[string]any{
		"trace_id":    event.TraceID,
		"span_id":     event.SpanID,
		"tool_name":   event.ToolName,
		"duration_ms": event.DurationMs,
		"success":     event.Success,
		"error":       event.Error,
		"timestamp":   event.Timestamp.Unix(),
		"mission_id":  event.MissionID.String(),
		"agent_name":  event.AgentName,
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record tool call in Neo4j",
			"agent_name", event.AgentName,
			"tool_name", event.ToolName,
			"trace_id", event.TraceID,
			"error", err)
		return nil
	}

	return nil
}

// RecordDelegation records when one agent delegates work to another.
// Creates a DELEGATED_TO relationship between AgentExecution nodes.
func (s *Neo4jExecutionGraphStore) RecordDelegation(ctx context.Context, event DelegationEvent) error {
	cypher := `
		MATCH (from:AgentExecution {mission_id: $mission_id})
		WHERE from.agent_name = $from_agent AND from.status = 'running'
		WITH from
		ORDER BY from.started_at DESC
		LIMIT 1
		WITH from
		MATCH (to:AgentExecution {mission_id: $mission_id})
		WHERE to.agent_name = $to_agent AND to.status = 'running'
		WITH from, to
		ORDER BY to.started_at DESC
		LIMIT 1
		MERGE (from)-[d:DELEGATED_TO]->(to)
		SET d.task_id = $task_id,
		    d.trace_id = $trace_id,
		    d.span_id = $span_id,
		    d.timestamp = $timestamp
		RETURN d
	`

	params := map[string]any{
		"mission_id": event.MissionID.String(),
		"from_agent": event.FromAgent,
		"to_agent":   event.ToAgent,
		"task_id":    event.TaskID,
		"trace_id":   event.TraceID,
		"span_id":    event.SpanID,
		"timestamp":  event.Timestamp.Unix(),
	}

	if err := s.executeWrite(ctx, cypher, params); err != nil {
		slog.Warn("failed to record delegation in Neo4j",
			"from_agent", event.FromAgent,
			"to_agent", event.ToAgent,
			"trace_id", event.TraceID,
			"error", err)
		return nil
	}

	return nil
}

// Close releases all resources. This is a no-op as the Neo4j client is managed externally.
func (s *Neo4jExecutionGraphStore) Close() error {
	// Client lifecycle is managed by the caller, not by this store
	return nil
}

// executeWrite is a helper method to execute write transactions with proper session management.
// It handles session creation, transaction execution, and error handling.
func (s *Neo4jExecutionGraphStore) executeWrite(ctx context.Context, cypher string, params map[string]any) error {
	_, err := s.client.ExecuteWrite(ctx, cypher, params)
	return err
}
