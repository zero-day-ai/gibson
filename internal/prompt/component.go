package prompt

import (
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/plugin"
	"github.com/zero-day-ai/gibson/internal/tool"
)

// ToolWithPrompt extends Tool with prompt capabilities.
// Tools can contribute prompts that describe their usage, provide examples,
// or add context to the LLM interaction. Tool prompts typically appear at
// PositionTools in the message sequence.
type ToolWithPrompt interface {
	tool.Tool

	// Prompts returns prompts contributed by this tool
	// Typically at PositionTools
	Prompts() []Prompt
}

// PluginWithPrompts extends Plugin with prompt capabilities.
// Plugins can contribute prompts that explain their data sources, query methods,
// or provide examples of how to use their capabilities. Plugin prompts typically
// appear at PositionPlugins in the message sequence.
type PluginWithPrompts interface {
	plugin.Plugin

	// Prompts returns prompts contributed by this plugin
	// Typically at PositionPlugins
	Prompts() []Prompt
}

// AgentWithPrompts extends Agent with prompt capabilities.
// Agents can define system prompts that guide their behavior, task prompts
// that provide instructions, and persona prompts that define different
// operational modes or personalities.
type AgentWithPrompts interface {
	agent.Agent

	// SystemPrompt returns the agent's system prompt (optional)
	// System prompts typically appear at PositionSystem and define the
	// agent's core behavior, capabilities, and constraints.
	// Returns nil if the agent does not define a system prompt.
	SystemPrompt() *Prompt

	// TaskPrompt returns the task-specific prompt (optional)
	// Task prompts typically appear at PositionUser and provide specific
	// instructions for the current task execution.
	// Returns nil if the agent does not define a task prompt.
	TaskPrompt() *Prompt

	// Personas returns available persona prompts
	// Persona prompts allow agents to adopt different operational modes,
	// expertise levels, or communication styles. They typically appear
	// at PositionSystem or PositionContext.
	// Returns an empty slice if no personas are defined.
	Personas() []Prompt
}

// ToolHasPrompts checks if a tool implements ToolWithPrompt interface.
// Returns true if the tool can contribute prompts to the message sequence.
func ToolHasPrompts(t tool.Tool) bool {
	_, ok := t.(ToolWithPrompt)
	return ok
}

// GetToolPrompts returns prompts from a tool if it implements ToolWithPrompt.
// Returns an empty slice if the tool does not implement ToolWithPrompt or
// if the tool returns no prompts.
func GetToolPrompts(t tool.Tool) []Prompt {
	if twp, ok := t.(ToolWithPrompt); ok {
		prompts := twp.Prompts()
		if prompts == nil {
			return []Prompt{}
		}
		return prompts
	}
	return []Prompt{}
}

// PluginHasPrompts checks if a plugin implements PluginWithPrompts interface.
// Returns true if the plugin can contribute prompts to the message sequence.
func PluginHasPrompts(p plugin.Plugin) bool {
	_, ok := p.(PluginWithPrompts)
	return ok
}

// GetPluginPrompts returns prompts from a plugin if it implements PluginWithPrompts.
// Returns an empty slice if the plugin does not implement PluginWithPrompts or
// if the plugin returns no prompts.
func GetPluginPrompts(p plugin.Plugin) []Prompt {
	if pwp, ok := p.(PluginWithPrompts); ok {
		prompts := pwp.Prompts()
		if prompts == nil {
			return []Prompt{}
		}
		return prompts
	}
	return []Prompt{}
}

// AgentHasPrompts checks if an agent implements AgentWithPrompts interface.
// Returns true if the agent can contribute prompts to the message sequence.
func AgentHasPrompts(a agent.Agent) bool {
	_, ok := a.(AgentWithPrompts)
	return ok
}

// GetAgentPrompts returns all prompts from an agent if it implements AgentWithPrompts.
// This includes the system prompt, task prompt, and all persona prompts.
// Returns an empty slice if the agent does not implement AgentWithPrompts.
//
// The returned prompts are collected in the following order:
// 1. System prompt (if not nil)
// 2. Task prompt (if not nil)
// 3. Persona prompts (if any)
func GetAgentPrompts(a agent.Agent) []Prompt {
	awp, ok := a.(AgentWithPrompts)
	if !ok {
		return []Prompt{}
	}

	prompts := []Prompt{}

	// Add system prompt if present
	if systemPrompt := awp.SystemPrompt(); systemPrompt != nil {
		prompts = append(prompts, *systemPrompt)
	}

	// Add task prompt if present
	if taskPrompt := awp.TaskPrompt(); taskPrompt != nil {
		prompts = append(prompts, *taskPrompt)
	}

	// Add persona prompts if present
	personas := awp.Personas()
	if personas != nil {
		prompts = append(prompts, personas...)
	}

	return prompts
}
