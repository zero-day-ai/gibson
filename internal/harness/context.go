package harness

import "github.com/zero-day-ai/gibson/internal/types"

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
	ID       types.ID          `json:"id"`
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	Type     string            `json:"type"`
	Provider string            `json:"provider,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// NewTargetInfo creates a new target info with the given ID, name, URL, and type.
// Provider, headers, and metadata are initialized to empty/default values.
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
