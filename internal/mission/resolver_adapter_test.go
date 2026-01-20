package mission

import (
	"testing"

	"github.com/zero-day-ai/gibson/internal/component"
)

func TestMissionDefinitionAdapter(t *testing.T) {
	// Create a test mission definition
	def := &MissionDefinition{
		Name: "test-mission",
		Nodes: map[string]*MissionNode{
			"node1": {
				ID:        "node1",
				Type:      NodeTypeAgent,
				AgentName: "test-agent",
			},
			"node2": {
				ID:       "node2",
				Type:     NodeTypeTool,
				ToolName: "test-tool",
			},
			"node3": {
				ID:         "node3",
				Type:       NodeTypePlugin,
				PluginName: "test-plugin",
			},
			"node4": {
				ID:   "node4",
				Type: NodeTypeCondition,
			},
		},
		Dependencies: &MissionDependencies{
			Agents:  []string{"explicit-agent"},
			Tools:   []string{"explicit-tool"},
			Plugins: []string{"explicit-plugin"},
		},
	}

	// Create adapter
	adapter := NewResolverAdapter(def)

	// Test Nodes method
	t.Run("Nodes", func(t *testing.T) {
		nodes := adapter.Nodes()
		if len(nodes) != 4 {
			t.Errorf("expected 4 nodes, got %d", len(nodes))
		}

		// Test node adapter methods
		for _, node := range nodes {
			id := node.ID()
			nodeType := node.Type()
			componentRef := node.ComponentRef()

			if id == "" {
				t.Error("node ID should not be empty")
			}
			if nodeType == "" {
				t.Error("node type should not be empty")
			}

			// Condition nodes should have empty component ref
			if nodeType == "condition" && componentRef != "" {
				t.Errorf("condition node should have empty component ref, got %s", componentRef)
			}

			// Component nodes should have non-empty component ref
			if (nodeType == "agent" || nodeType == "tool" || nodeType == "plugin") && componentRef == "" {
				t.Errorf("%s node should have non-empty component ref", nodeType)
			}
		}
	})

	// Test Dependencies method
	t.Run("Dependencies", func(t *testing.T) {
		deps := adapter.Dependencies()
		if len(deps) != 3 {
			t.Errorf("expected 3 explicit dependencies, got %d", len(deps))
		}

		// Verify kinds
		kindCounts := make(map[component.ComponentKind]int)
		for _, dep := range deps {
			kindCounts[dep.Kind()]++
			if dep.Name() == "" {
				t.Error("dependency name should not be empty")
			}
		}

		if kindCounts[component.ComponentKindAgent] != 1 {
			t.Errorf("expected 1 agent dependency, got %d", kindCounts[component.ComponentKindAgent])
		}
		if kindCounts[component.ComponentKindTool] != 1 {
			t.Errorf("expected 1 tool dependency, got %d", kindCounts[component.ComponentKindTool])
		}
		if kindCounts[component.ComponentKindPlugin] != 1 {
			t.Errorf("expected 1 plugin dependency, got %d", kindCounts[component.ComponentKindPlugin])
		}
	})

	// Test empty dependencies
	t.Run("EmptyDependencies", func(t *testing.T) {
		emptyDef := &MissionDefinition{
			Name:  "empty-mission",
			Nodes: make(map[string]*MissionNode),
		}
		emptyAdapter := NewResolverAdapter(emptyDef)

		nodes := emptyAdapter.Nodes()
		if len(nodes) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(nodes))
		}

		deps := emptyAdapter.Dependencies()
		if len(deps) != 0 {
			t.Errorf("expected 0 dependencies, got %d", len(deps))
		}
	})
}

func TestNodeAdapterComponentRef(t *testing.T) {
	tests := []struct {
		name         string
		node         *MissionNode
		expectedRef  string
		expectedType string
	}{
		{
			name: "agent node",
			node: &MissionNode{
				ID:        "agent1",
				Type:      NodeTypeAgent,
				AgentName: "my-agent",
			},
			expectedRef:  "my-agent",
			expectedType: "agent",
		},
		{
			name: "tool node",
			node: &MissionNode{
				ID:       "tool1",
				Type:     NodeTypeTool,
				ToolName: "my-tool",
			},
			expectedRef:  "my-tool",
			expectedType: "tool",
		},
		{
			name: "plugin node",
			node: &MissionNode{
				ID:         "plugin1",
				Type:       NodeTypePlugin,
				PluginName: "my-plugin",
			},
			expectedRef:  "my-plugin",
			expectedType: "plugin",
		},
		{
			name: "condition node",
			node: &MissionNode{
				ID:   "cond1",
				Type: NodeTypeCondition,
			},
			expectedRef:  "",
			expectedType: "condition",
		},
		{
			name: "parallel node",
			node: &MissionNode{
				ID:   "parallel1",
				Type: NodeTypeParallel,
			},
			expectedRef:  "",
			expectedType: "parallel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &missionNodeAdapter{node: tt.node}

			id := adapter.ID()
			if id != tt.node.ID {
				t.Errorf("expected ID %s, got %s", tt.node.ID, id)
			}

			nodeType := adapter.Type()
			if nodeType != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, nodeType)
			}

			componentRef := adapter.ComponentRef()
			if componentRef != tt.expectedRef {
				t.Errorf("expected component ref %s, got %s", tt.expectedRef, componentRef)
			}
		})
	}
}

func TestDependencyAdapter(t *testing.T) {
	adapter := &missionDependencyAdapter{
		kind:    component.ComponentKindAgent,
		name:    "test-agent",
		version: ">=1.0.0",
	}

	if adapter.Kind() != component.ComponentKindAgent {
		t.Errorf("expected kind %s, got %s", component.ComponentKindAgent, adapter.Kind())
	}
	if adapter.Name() != "test-agent" {
		t.Errorf("expected name test-agent, got %s", adapter.Name())
	}
	if adapter.Version() != ">=1.0.0" {
		t.Errorf("expected version >=1.0.0, got %s", adapter.Version())
	}
}
