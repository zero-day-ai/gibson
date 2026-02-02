// Package contextkeys provides shared context key definitions used across Gibson packages.
// This package exists to avoid circular imports between packages that need to read/write
// context values (e.g., harness and registry).
package contextkeys

import "context"

// Key is the type for all Gibson context keys.
type Key string

const (
	// AgentRunID stores the unique identifier for an agent execution.
	// Used for DISCOVERED relationships and provenance tracking in GraphRAG.
	AgentRunID Key = "gibson.agent_run_id"

	// ToolExecutionID stores the unique identifier for a tool execution.
	ToolExecutionID Key = "gibson.tool_execution_id"

	// MissionRunID stores the unique identifier for a mission run.
	// Used for mission-scoped GraphRAG storage.
	MissionRunID Key = "gibson.mission_run_id"

	// AgentName stores the current agent name for policy lookup.
	AgentName Key = "gibson.agent_name"

	// MissionID stores the mission ID (raw string, not types.ID).
	MissionID Key = "gibson.mission_id"
)

// WithAgentRunID returns a new context with the agent run ID set.
func WithAgentRunID(ctx context.Context, agentRunID string) context.Context {
	return context.WithValue(ctx, AgentRunID, agentRunID)
}

// GetAgentRunID retrieves the agent run ID from context.
// Returns empty string if not set.
func GetAgentRunID(ctx context.Context) string {
	if v := ctx.Value(AgentRunID); v != nil {
		return v.(string)
	}
	return ""
}

// WithToolExecutionID returns a new context with the tool execution ID set.
func WithToolExecutionID(ctx context.Context, toolExecutionID string) context.Context {
	return context.WithValue(ctx, ToolExecutionID, toolExecutionID)
}

// GetToolExecutionID retrieves the tool execution ID from context.
// Returns empty string if not set.
func GetToolExecutionID(ctx context.Context) string {
	if v := ctx.Value(ToolExecutionID); v != nil {
		return v.(string)
	}
	return ""
}

// WithMissionRunID returns a new context with the mission run ID set.
func WithMissionRunID(ctx context.Context, missionRunID string) context.Context {
	return context.WithValue(ctx, MissionRunID, missionRunID)
}

// GetMissionRunID retrieves the mission run ID from context.
// Returns empty string if not set.
func GetMissionRunID(ctx context.Context) string {
	if v := ctx.Value(MissionRunID); v != nil {
		return v.(string)
	}
	return ""
}
