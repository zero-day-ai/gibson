package harness

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionContext represents the broader mission context for agent execution.
// It provides agents with awareness of the overall mission, current phase,
// constraints, and other mission-level metadata.
type MissionContext struct {
	ID           types.ID       `json:"id"`
	Name         string         `json:"name"`
	CurrentAgent string         `json:"current_agent"`
	Phase        string         `json:"phase"`
	Constraints  []string       `json:"constraints"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	// MissionRunID is the unique identifier for this specific mission execution.
	// Created by MissionGraphManager.CreateMissionRunNode() at mission start.
	// Used for mission-scoped GraphRAG storage.
	MissionRunID string `json:"mission_run_id,omitempty"`
	// AgentRunID is the unique identifier for this specific agent execution.
	// Used for DISCOVERED relationships and provenance tracking.
	AgentRunID string `json:"agent_run_id,omitempty"`
	// RunNumber is the sequential run number for this mission (1, 2, 3...).
	// Used for mission memory queries and historical comparisons.
	RunNumber int `json:"run_number,omitempty"`
}

// NewMissionContext creates a new mission context with the given ID, name, and current agent.
// Phase, constraints, and metadata are initialized to empty/default values.
func NewMissionContext(id types.ID, name, currentAgent string) MissionContext {
	return MissionContext{
		ID:           id,
		Name:         name,
		CurrentAgent: currentAgent,
		Phase:        "",
		Constraints:  []string{},
		Metadata:     make(map[string]any),
	}
}

// WithPhase sets the mission phase
func (m MissionContext) WithPhase(phase string) MissionContext {
	m.Phase = phase
	return m
}

// WithConstraints sets the mission constraints
func (m MissionContext) WithConstraints(constraints ...string) MissionContext {
	m.Constraints = constraints
	return m
}

// WithMetadata sets a metadata key-value pair
func (m MissionContext) WithMetadata(key string, value any) MissionContext {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[key] = value
	return m
}

// TargetInfo represents information about a target system or service.
// It provides agents with the necessary details to interact with targets
// including authentication headers and provider-specific metadata.
type TargetInfo struct {
	ID         types.ID       `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Provider   string         `json:"provider,omitempty"`
	Connection map[string]any `json:"connection,omitempty"` // Schema-based connection parameters
	Metadata   map[string]any `json:"metadata,omitempty"`

	// Deprecated: Use Connection["url"] instead. Kept for backward compatibility.
	URL string `json:"url,omitempty"`
	// Deprecated: Use Connection["headers"] instead. Kept for backward compatibility.
	Headers map[string]string `json:"headers,omitempty"`
}

// NewTargetInfo creates a new target info with the given ID, name, URL, and type.
// Provider, headers, and metadata are initialized to empty/default values.
// For targets with connection parameters, use NewTargetInfoFull instead.
func NewTargetInfo(id types.ID, name, url, targetType string) TargetInfo {
	return TargetInfo{
		ID:       id,
		Name:     name,
		URL:      url,
		Type:     targetType,
		Provider: "",
		Headers:  make(map[string]string),
		Metadata: make(map[string]any),
	}
}

// NewTargetInfoFull creates a new target info with full connection parameters.
// This constructor should be used when creating TargetInfo from a Target entity
// that has schema-based connection configuration.
func NewTargetInfoFull(id types.ID, name, url, targetType string, connection map[string]any) TargetInfo {
	return TargetInfo{
		ID:         id,
		Name:       name,
		URL:        url,
		Type:       targetType,
		Provider:   "",
		Connection: connection,
		Headers:    make(map[string]string),
		Metadata:   make(map[string]any),
	}
}

// GetConnection returns the connection parameters for this target.
// Returns nil if no connection parameters are set.
func (t TargetInfo) GetConnection() map[string]any {
	return t.Connection
}

// WithProvider sets the provider for this target
func (t TargetInfo) WithProvider(provider string) TargetInfo {
	t.Provider = provider
	return t
}

// WithHeader adds a header key-value pair
func (t TargetInfo) WithHeader(key, value string) TargetInfo {
	if t.Headers == nil {
		t.Headers = make(map[string]string)
	}
	t.Headers[key] = value
	return t
}

// WithHeaders sets multiple headers at once
func (t TargetInfo) WithHeaders(headers map[string]string) TargetInfo {
	if t.Headers == nil {
		t.Headers = make(map[string]string)
	}
	for k, v := range headers {
		t.Headers[k] = v
	}
	return t
}

// WithMetadata sets a metadata key-value pair
func (t TargetInfo) WithMetadata(key string, value any) TargetInfo {
	if t.Metadata == nil {
		t.Metadata = make(map[string]any)
	}
	t.Metadata[key] = value
	return t
}

// contextKey is a type for context keys to avoid collisions with other packages.
type contextKey string

const (
	agentRunIDKey      contextKey = "gibson.agent_run_id"
	toolExecutionIDKey contextKey = "gibson.tool_execution_id"
	missionRunIDKey    contextKey = "gibson.mission_run_id"
)

// ContextWithAgentRunID returns a new context with the agent run ID set.
// The agent run ID format should be: agent_run:{trace_id}:{span_id}
func ContextWithAgentRunID(ctx context.Context, agentRunID string) context.Context {
	return context.WithValue(ctx, agentRunIDKey, agentRunID)
}

// AgentRunIDFromContext retrieves the agent run ID from context.
// Returns empty string if not set.
func AgentRunIDFromContext(ctx context.Context) string {
	if v := ctx.Value(agentRunIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// ContextWithToolExecutionID returns a new context with the tool execution ID set.
// The tool execution ID format should be: tool_execution:{trace_id}:{span_id}:{timestamp}
func ContextWithToolExecutionID(ctx context.Context, toolExecutionID string) context.Context {
	return context.WithValue(ctx, toolExecutionIDKey, toolExecutionID)
}

// ToolExecutionIDFromContext retrieves the tool execution ID from context.
// Returns empty string if not set.
func ToolExecutionIDFromContext(ctx context.Context) string {
	if v := ctx.Value(toolExecutionIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// ContextWithMissionRunID returns a new context with the mission run ID set.
// The mission run ID is used for GraphRAG mission-scoped storage, allowing agents
// to automatically associate stored nodes with the current mission run.
func ContextWithMissionRunID(ctx context.Context, missionRunID string) context.Context {
	return context.WithValue(ctx, missionRunIDKey, missionRunID)
}

// MissionRunIDFromContext retrieves the mission run ID from context.
// Returns empty string if not set.
func MissionRunIDFromContext(ctx context.Context) string {
	if v := ctx.Value(missionRunIDKey); v != nil {
		return v.(string)
	}
	return ""
}
