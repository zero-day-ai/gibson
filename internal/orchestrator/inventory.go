package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ComponentInventory represents a snapshot of all registered components in the system.
// It aggregates agents, tools, and plugins discovered from the registry, providing
// a unified view for orchestration decisions.
//
// The inventory includes helper methods for component lookups, filtering, and validation.
// An inventory can become stale if components register/deregister after it was created.
type ComponentInventory struct {
	// Agents lists all registered agents with their metadata
	Agents []AgentSummary `json:"agents"`

	// Tools lists all registered tools with their metadata
	Tools []ToolSummary `json:"tools"`

	// Plugins lists all registered plugins with their metadata
	Plugins []PluginSummary `json:"plugins"`

	// GatheredAt is the timestamp when this inventory was collected
	GatheredAt time.Time `json:"gathered_at"`

	// IsStale indicates if this inventory may be outdated (set by caller)
	IsStale bool `json:"is_stale"`

	// TotalComponents is the sum of all component counts
	TotalComponents int `json:"total_components"`
}

// AgentSummary provides high-level information about a registered agent.
// This is an enriched version of registry.AgentInfo that includes additional
// fields useful for orchestration decisions.
type AgentSummary struct {
	// Name is the agent's unique identifier
	Name string `json:"name"`

	// Version is the semantic version
	Version string `json:"version"`

	// Description explains what the agent does
	Description string `json:"description"`

	// Capabilities lists security testing capabilities (e.g., ["prompt_injection", "jailbreak"])
	Capabilities []string `json:"capabilities"`

	// TargetTypes lists supported target types (e.g., ["llm_chat", "llm_api"])
	TargetTypes []string `json:"target_types"`

	// TechniqueTypes lists attack techniques employed
	TechniqueTypes []string `json:"technique_types"`

	// Slots describes LLM slot requirements
	Slots []SlotSummary `json:"slots"`

	// Instances is the number of running instances
	Instances int `json:"instances"`

	// HealthStatus indicates overall health ("healthy", "degraded", "unhealthy")
	HealthStatus string `json:"health_status"`

	// Endpoints lists all instance endpoints
	Endpoints []string `json:"endpoints"`

	// IsExternal indicates if this agent runs via gRPC
	IsExternal bool `json:"is_external"`
}

// ToolSummary provides high-level information about a registered tool.
type ToolSummary struct {
	// Name is the tool's unique identifier
	Name string `json:"name"`

	// Version is the semantic version
	Version string `json:"version"`

	// Description explains what the tool does
	Description string `json:"description"`

	// Tags categorize the tool (e.g., ["network", "scanner"])
	Tags []string `json:"tags"`

	// InputSummary provides a brief description of expected input
	InputSummary string `json:"input_summary"`

	// OutputSummary provides a brief description of output format
	OutputSummary string `json:"output_summary"`

	// InputSchema contains the JSON schema for input (optional, omitted if empty)
	InputSchema map[string]any `json:"input_schema,omitempty"`

	// OutputSchema contains the JSON schema for output (optional, omitted if empty)
	OutputSchema map[string]any `json:"output_schema,omitempty"`

	// Capabilities describes runtime privileges and features available to the tool.
	// Nil if the tool does not implement CapabilityProvider or has no specific requirements.
	Capabilities *CapabilitiesSummary `json:"capabilities,omitempty"`

	// Instances is the number of running instances
	Instances int `json:"instances"`

	// HealthStatus indicates overall health
	HealthStatus string `json:"health_status"`

	// IsExternal indicates if this tool runs via gRPC
	IsExternal bool `json:"is_external"`
}

// PluginSummary provides high-level information about a registered plugin.
type PluginSummary struct {
	// Name is the plugin's unique identifier
	Name string `json:"name"`

	// Version is the semantic version
	Version string `json:"version"`

	// Description explains what the plugin does
	Description string `json:"description"`

	// Methods lists all available plugin methods
	Methods []MethodSummary `json:"methods"`

	// Instances is the number of running instances
	Instances int `json:"instances"`

	// HealthStatus indicates overall health
	HealthStatus string `json:"health_status"`

	// IsExternal indicates if this plugin runs via gRPC
	IsExternal bool `json:"is_external"`
}

// SlotSummary describes an LLM slot requirement for an agent.
type SlotSummary struct {
	// Name is the slot identifier
	Name string `json:"name"`

	// Description explains what the slot is used for
	Description string `json:"description"`

	// Required indicates if this slot must be satisfied
	Required bool `json:"required"`

	// DefaultProvider is the default LLM provider (e.g., "anthropic", "openai")
	DefaultProvider string `json:"default_provider"`

	// DefaultModel is the default model name
	DefaultModel string `json:"default_model"`
}

// MethodSummary describes a plugin method.
type MethodSummary struct {
	// Name is the method identifier
	Name string `json:"name"`

	// Description explains what the method does
	Description string `json:"description"`

	// InputSummary provides a brief description of expected input
	InputSummary string `json:"input_summary"`

	// OutputSummary provides a brief description of output format
	OutputSummary string `json:"output_summary"`
}

// CapabilitiesSummary describes runtime privileges and features available to a tool.
// This is the orchestrator's representation of tool capabilities for prompt formatting.
type CapabilitiesSummary struct {
	// HasRoot indicates the tool is running as uid 0 (root user)
	HasRoot bool `json:"has_root"`

	// HasSudo indicates passwordless sudo access is available
	HasSudo bool `json:"has_sudo"`

	// CanRawSocket indicates the ability to create raw network sockets
	CanRawSocket bool `json:"can_raw_socket"`

	// Features contains tool-specific feature availability flags
	Features map[string]bool `json:"features,omitempty"`

	// BlockedArgs lists command-line arguments that cannot be used
	BlockedArgs []string `json:"blocked_args,omitempty"`

	// ArgAlternatives maps blocked arguments to their safer alternatives
	ArgAlternatives map[string]string `json:"arg_alternatives,omitempty"`
}

// HasAgent checks if an agent with the given name exists in the inventory.
func (i *ComponentInventory) HasAgent(name string) bool {
	for _, agent := range i.Agents {
		if agent.Name == name {
			return true
		}
	}
	return false
}

// HasTool checks if a tool with the given name exists in the inventory.
func (i *ComponentInventory) HasTool(name string) bool {
	for _, tool := range i.Tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

// HasPlugin checks if a plugin with the given name exists in the inventory.
func (i *ComponentInventory) HasPlugin(name string) bool {
	for _, plugin := range i.Plugins {
		if plugin.Name == name {
			return true
		}
	}
	return false
}

// GetAgent retrieves an agent by name, or nil if not found.
func (i *ComponentInventory) GetAgent(name string) *AgentSummary {
	for idx := range i.Agents {
		if i.Agents[idx].Name == name {
			return &i.Agents[idx]
		}
	}
	return nil
}

// GetTool retrieves a tool by name, or nil if not found.
func (i *ComponentInventory) GetTool(name string) *ToolSummary {
	for idx := range i.Tools {
		if i.Tools[idx].Name == name {
			return &i.Tools[idx]
		}
	}
	return nil
}

// GetPlugin retrieves a plugin by name, or nil if not found.
func (i *ComponentInventory) GetPlugin(name string) *PluginSummary {
	for idx := range i.Plugins {
		if i.Plugins[idx].Name == name {
			return &i.Plugins[idx]
		}
	}
	return nil
}

// AgentNames returns a slice of all agent names in the inventory.
func (i *ComponentInventory) AgentNames() []string {
	names := make([]string, len(i.Agents))
	for idx, agent := range i.Agents {
		names[idx] = agent.Name
	}
	return names
}

// ToolNames returns a slice of all tool names in the inventory.
func (i *ComponentInventory) ToolNames() []string {
	names := make([]string, len(i.Tools))
	for idx, tool := range i.Tools {
		names[idx] = tool.Name
	}
	return names
}

// PluginNames returns a slice of all plugin names in the inventory.
func (i *ComponentInventory) PluginNames() []string {
	names := make([]string, len(i.Plugins))
	for idx, plugin := range i.Plugins {
		names[idx] = plugin.Name
	}
	return names
}

// FilterAgentsByCapability returns agents that have the specified capability.
// The capability match is case-insensitive.
func (i *ComponentInventory) FilterAgentsByCapability(capability string) []AgentSummary {
	result := make([]AgentSummary, 0)
	lowerCap := strings.ToLower(capability)

	for _, agent := range i.Agents {
		for _, cap := range agent.Capabilities {
			if strings.ToLower(cap) == lowerCap {
				result = append(result, agent)
				break
			}
		}
	}

	return result
}

// FilterAgentsByTargetType returns agents that support the specified target type.
// The target type match is case-insensitive.
func (i *ComponentInventory) FilterAgentsByTargetType(targetType string) []AgentSummary {
	result := make([]AgentSummary, 0)
	lowerType := strings.ToLower(targetType)

	for _, agent := range i.Agents {
		for _, tt := range agent.TargetTypes {
			if strings.ToLower(tt) == lowerType {
				result = append(result, agent)
				break
			}
		}
	}

	return result
}

// FilterToolsByTag returns tools that have the specified tag.
// The tag match is case-insensitive.
func (i *ComponentInventory) FilterToolsByTag(tag string) []ToolSummary {
	result := make([]ToolSummary, 0)
	lowerTag := strings.ToLower(tag)

	for _, tool := range i.Tools {
		for _, t := range tool.Tags {
			if strings.ToLower(t) == lowerTag {
				result = append(result, tool)
				break
			}
		}
	}

	return result
}

// ComponentValidationError is returned when a requested component does not exist
// or is invalid. It provides helpful information about what went wrong and suggests
// available alternatives.
//
// This error supports errors.Is() for matching against sentinel validation errors.
type ComponentValidationError struct {
	// ComponentType is the type of component ("agent", "tool", "plugin")
	ComponentType string

	// RequestedName is the name that was requested
	RequestedName string

	// Available lists all registered components of this type
	Available []string

	// Suggestions lists components that are similar to the requested name
	// (e.g., via fuzzy matching or edit distance)
	Suggestions []string

	// Message is a custom error message (optional)
	Message string
}

// Error implements the error interface.
func (e *ComponentValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s '%s' not found", e.ComponentType, e.RequestedName))

	if len(e.Suggestions) > 0 {
		sb.WriteString(fmt.Sprintf(" - did you mean: %s?", strings.Join(e.Suggestions, ", ")))
	}

	if len(e.Available) > 0 {
		sb.WriteString(fmt.Sprintf(" (available %ss: %s)", e.ComponentType, strings.Join(e.Available, ", ")))
	} else {
		sb.WriteString(fmt.Sprintf(" (no %ss registered)", e.ComponentType))
	}

	return sb.String()
}

// Is implements error matching for errors.Is().
// It returns true if the target is also a ComponentValidationError with the same
// ComponentType and RequestedName.
func (e *ComponentValidationError) Is(target error) bool {
	var t *ComponentValidationError
	if !errors.As(target, &t) {
		return false
	}

	// Match if component type and requested name are the same
	// (ignoring case for component type)
	return strings.EqualFold(e.ComponentType, t.ComponentType) &&
		e.RequestedName == t.RequestedName
}

// NewAgentNotFoundError creates a ComponentValidationError for a missing agent.
func NewAgentNotFoundError(name string, available []string) *ComponentValidationError {
	return &ComponentValidationError{
		ComponentType: "agent",
		RequestedName: name,
		Available:     available,
		Suggestions:   []string{},
	}
}

// NewToolNotFoundError creates a ComponentValidationError for a missing tool.
func NewToolNotFoundError(name string, available []string) *ComponentValidationError {
	return &ComponentValidationError{
		ComponentType: "tool",
		RequestedName: name,
		Available:     available,
		Suggestions:   []string{},
	}
}

// NewPluginNotFoundError creates a ComponentValidationError for a missing plugin.
func NewPluginNotFoundError(name string, available []string) *ComponentValidationError {
	return &ComponentValidationError{
		ComponentType: "plugin",
		RequestedName: name,
		Available:     available,
		Suggestions:   []string{},
	}
}
