package harness

import (
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/plugin"
	"github.com/zero-day-ai/gibson/internal/schema"
	"github.com/zero-day-ai/gibson/internal/tool"
)

// ToolDescriptor provides lightweight metadata about a tool without requiring
// the full tool interface. Used for discovery, filtering, and capability queries.
type ToolDescriptor struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Tags         []string          `json:"tags"`
	InputSchema  schema.JSONSchema `json:"input_schema"`
	OutputSchema schema.JSONSchema `json:"output_schema"`
}

// FromTool creates a ToolDescriptor from a Tool interface.
// This extracts metadata without exposing the full tool implementation.
func FromTool(t tool.Tool) ToolDescriptor {
	return ToolDescriptor{
		Name:         t.Name(),
		Description:  t.Description(),
		Version:      t.Version(),
		Tags:         t.Tags(),
		InputSchema:  t.InputSchema(),
		OutputSchema: t.OutputSchema(),
	}
}

// PluginDescriptor provides lightweight metadata about a plugin.
// Used for discovery and capability queries without requiring plugin initialization.
type PluginDescriptor struct {
	Name       string                    `json:"name"`
	Version    string                    `json:"version"`
	Methods    []plugin.MethodDescriptor `json:"methods"`
	IsExternal bool                      `json:"is_external"`
	Status     plugin.PluginStatus       `json:"status"`
}

// FromPlugin creates a PluginDescriptor from a Plugin interface.
// This extracts metadata and available methods from the plugin.
func FromPlugin(p plugin.Plugin) PluginDescriptor {
	return PluginDescriptor{
		Name:       p.Name(),
		Version:    p.Version(),
		Methods:    p.Methods(),
		IsExternal: false,
		Status:     plugin.PluginStatusUninitialized,
	}
}

// AgentDescriptor provides lightweight metadata about an agent.
// Used for discovery, filtering, and delegation without instantiating the agent.
type AgentDescriptor struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Description  string                 `json:"description"`
	Capabilities []string               `json:"capabilities"`
	Slots        []agent.SlotDefinition `json:"slots"`
	IsExternal   bool                   `json:"is_external"`
}

// FromAgent creates an AgentDescriptor from an Agent interface.
// This extracts metadata about the agent's capabilities and requirements.
func FromAgent(a agent.Agent) AgentDescriptor {
	return AgentDescriptor{
		Name:         a.Name(),
		Version:      a.Version(),
		Description:  a.Description(),
		Capabilities: a.Capabilities(),
		Slots:        a.LLMSlots(),
		IsExternal:   false,
	}
}

// HasMethod checks if a plugin descriptor supports a specific method
func (p PluginDescriptor) HasMethod(methodName string) bool {
	for _, method := range p.Methods {
		if method.Name == methodName {
			return true
		}
	}
	return false
}

// GetMethod retrieves a method descriptor by name
func (p PluginDescriptor) GetMethod(methodName string) *plugin.MethodDescriptor {
	for i, method := range p.Methods {
		if method.Name == methodName {
			return &p.Methods[i]
		}
	}
	return nil
}

// HasTag checks if a tool descriptor has a specific tag
func (t ToolDescriptor) HasTag(tag string) bool {
	for _, t := range t.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// HasCapability checks if an agent descriptor has a specific capability
func (a AgentDescriptor) HasCapability(capability string) bool {
	for _, c := range a.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// RequiresSlot checks if the agent requires a specific slot
func (a AgentDescriptor) RequiresSlot(slotName string) bool {
	for _, slot := range a.Slots {
		if slot.Name == slotName {
			return slot.Required
		}
	}
	return false
}

// GetSlot retrieves a slot definition by name
func (a AgentDescriptor) GetSlot(slotName string) *agent.SlotDefinition {
	for i, slot := range a.Slots {
		if slot.Name == slotName {
			return &a.Slots[i]
		}
	}
	return nil
}
