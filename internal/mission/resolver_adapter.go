package mission

import (
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/resolver"
)

// missionDefinitionAdapter adapts MissionDefinition to resolver.MissionDefinition interface.
// This avoids circular dependencies between mission and resolver packages.
type missionDefinitionAdapter struct {
	def *MissionDefinition
}

// NewResolverAdapter creates a new adapter that implements resolver.MissionDefinition.
func NewResolverAdapter(def *MissionDefinition) resolver.MissionDefinition {
	return &missionDefinitionAdapter{def: def}
}

// Nodes returns all nodes in the mission.
func (m *missionDefinitionAdapter) Nodes() []resolver.MissionNode {
	if m.def.Nodes == nil {
		return []resolver.MissionNode{}
	}

	nodes := make([]resolver.MissionNode, 0, len(m.def.Nodes))
	for _, node := range m.def.Nodes {
		nodes = append(nodes, &missionNodeAdapter{node: node})
	}
	return nodes
}

// Dependencies returns explicitly declared dependencies from the mission YAML.
func (m *missionDefinitionAdapter) Dependencies() []resolver.MissionDependency {
	if m.def.Dependencies == nil {
		return []resolver.MissionDependency{}
	}

	deps := make([]resolver.MissionDependency, 0)

	// Add agent dependencies
	for _, agent := range m.def.Dependencies.Agents {
		deps = append(deps, &missionDependencyAdapter{
			kind:    component.ComponentKindAgent,
			name:    agent,
			version: "", // TODO: Parse version from agent string if present
		})
	}

	// Add tool dependencies
	for _, tool := range m.def.Dependencies.Tools {
		deps = append(deps, &missionDependencyAdapter{
			kind:    component.ComponentKindTool,
			name:    tool,
			version: "", // TODO: Parse version from tool string if present
		})
	}

	// Add plugin dependencies
	for _, plugin := range m.def.Dependencies.Plugins {
		deps = append(deps, &missionDependencyAdapter{
			kind:    component.ComponentKindPlugin,
			name:    plugin,
			version: "", // TODO: Parse version from plugin string if present
		})
	}

	return deps
}

// missionNodeAdapter adapts MissionNode to resolver.MissionNode interface.
type missionNodeAdapter struct {
	node *MissionNode
}

// ID returns the unique identifier for this node within the mission.
func (m *missionNodeAdapter) ID() string {
	return m.node.ID
}

// Type returns the node type (agent, tool, plugin, condition, parallel, join).
func (m *missionNodeAdapter) Type() string {
	return string(m.node.Type)
}

// ComponentRef returns the name of the component referenced by this node.
// Returns empty string for non-component nodes (condition, parallel, join).
func (m *missionNodeAdapter) ComponentRef() string {
	switch m.node.Type {
	case NodeTypeAgent:
		return m.node.AgentName
	case NodeTypeTool:
		return m.node.ToolName
	case NodeTypePlugin:
		return m.node.PluginName
	default:
		return ""
	}
}

// missionDependencyAdapter adapts a dependency string to resolver.MissionDependency interface.
type missionDependencyAdapter struct {
	kind    component.ComponentKind
	name    string
	version string
}

// Kind returns the component kind (agent, tool, plugin).
func (m *missionDependencyAdapter) Kind() component.ComponentKind {
	return m.kind
}

// Name returns the component name.
func (m *missionDependencyAdapter) Name() string {
	return m.name
}

// Version returns the version constraint (e.g., ">=1.0.0", "^2.0.0").
// Returns empty string if no version constraint is specified.
func (m *missionDependencyAdapter) Version() string {
	return m.version
}
