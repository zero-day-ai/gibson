package orchestrator

import (
	"errors"
	"testing"
	"time"
)

func TestComponentInventory_HasAgent(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci", Version: "1.0.0"},
			{Name: "k8skiller", Version: "1.0.0"},
		},
	}

	tests := []struct {
		name     string
		agent    string
		expected bool
	}{
		{"existing agent", "davinci", true},
		{"another existing agent", "k8skiller", true},
		{"non-existent agent", "nonexistent", false},
		{"case sensitive", "DaVinci", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inv.HasAgent(tt.agent)
			if result != tt.expected {
				t.Errorf("HasAgent(%q) = %v, want %v", tt.agent, result, tt.expected)
			}
		})
	}
}

func TestComponentInventory_HasTool(t *testing.T) {
	inv := &ComponentInventory{
		Tools: []ToolSummary{
			{Name: "nmap", Version: "1.0.0"},
			{Name: "sqlmap", Version: "1.0.0"},
		},
	}

	tests := []struct {
		name     string
		tool     string
		expected bool
	}{
		{"existing tool", "nmap", true},
		{"another existing tool", "sqlmap", true},
		{"non-existent tool", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inv.HasTool(tt.tool)
			if result != tt.expected {
				t.Errorf("HasTool(%q) = %v, want %v", tt.tool, result, tt.expected)
			}
		})
	}
}

func TestComponentInventory_HasPlugin(t *testing.T) {
	inv := &ComponentInventory{
		Plugins: []PluginSummary{
			{Name: "mitre-lookup", Version: "1.0.0"},
		},
	}

	tests := []struct {
		name     string
		plugin   string
		expected bool
	}{
		{"existing plugin", "mitre-lookup", true},
		{"non-existent plugin", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inv.HasPlugin(tt.plugin)
			if result != tt.expected {
				t.Errorf("HasPlugin(%q) = %v, want %v", tt.plugin, result, tt.expected)
			}
		})
	}
}

func TestComponentInventory_GetAgent(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{
				Name:         "davinci",
				Version:      "1.0.0",
				Description:  "DaVinci test agent",
				Capabilities: []string{"jailbreak"},
			},
		},
	}

	t.Run("existing agent", func(t *testing.T) {
		agent := inv.GetAgent("davinci")
		if agent == nil {
			t.Fatal("GetAgent returned nil for existing agent")
		}
		if agent.Name != "davinci" {
			t.Errorf("GetAgent returned wrong agent: got %q, want %q", agent.Name, "davinci")
		}
		if agent.Description != "DaVinci test agent" {
			t.Errorf("GetAgent returned wrong description: got %q, want %q", agent.Description, "DaVinci test agent")
		}
	})

	t.Run("non-existent agent", func(t *testing.T) {
		agent := inv.GetAgent("nonexistent")
		if agent != nil {
			t.Errorf("GetAgent returned non-nil for non-existent agent: %+v", agent)
		}
	})
}

func TestComponentInventory_GetTool(t *testing.T) {
	inv := &ComponentInventory{
		Tools: []ToolSummary{
			{Name: "nmap", Version: "1.0.0", Description: "Network scanner"},
		},
	}

	t.Run("existing tool", func(t *testing.T) {
		tool := inv.GetTool("nmap")
		if tool == nil {
			t.Fatal("GetTool returned nil for existing tool")
		}
		if tool.Name != "nmap" {
			t.Errorf("GetTool returned wrong tool: got %q, want %q", tool.Name, "nmap")
		}
	})

	t.Run("non-existent tool", func(t *testing.T) {
		tool := inv.GetTool("nonexistent")
		if tool != nil {
			t.Errorf("GetTool returned non-nil for non-existent tool: %+v", tool)
		}
	})
}

func TestComponentInventory_GetPlugin(t *testing.T) {
	inv := &ComponentInventory{
		Plugins: []PluginSummary{
			{Name: "mitre-lookup", Version: "1.0.0"},
		},
	}

	t.Run("existing plugin", func(t *testing.T) {
		plugin := inv.GetPlugin("mitre-lookup")
		if plugin == nil {
			t.Fatal("GetPlugin returned nil for existing plugin")
		}
		if plugin.Name != "mitre-lookup" {
			t.Errorf("GetPlugin returned wrong plugin: got %q, want %q", plugin.Name, "mitre-lookup")
		}
	})

	t.Run("non-existent plugin", func(t *testing.T) {
		plugin := inv.GetPlugin("nonexistent")
		if plugin != nil {
			t.Errorf("GetPlugin returned non-nil for non-existent plugin: %+v", plugin)
		}
	})
}

func TestComponentInventory_AgentNames(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "k8skiller"},
			{Name: "michelangelo"},
		},
	}

	names := inv.AgentNames()
	if len(names) != 3 {
		t.Fatalf("AgentNames() returned %d names, want 3", len(names))
	}

	expected := map[string]bool{
		"davinci":      true,
		"k8skiller":    true,
		"michelangelo": true,
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("AgentNames() returned unexpected name: %q", name)
		}
		delete(expected, name)
	}

	if len(expected) > 0 {
		t.Errorf("AgentNames() missing names: %v", expected)
	}
}

func TestComponentInventory_ToolNames(t *testing.T) {
	inv := &ComponentInventory{
		Tools: []ToolSummary{
			{Name: "nmap"},
			{Name: "sqlmap"},
		},
	}

	names := inv.ToolNames()
	if len(names) != 2 {
		t.Fatalf("ToolNames() returned %d names, want 2", len(names))
	}

	expected := map[string]bool{
		"nmap":   true,
		"sqlmap": true,
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("ToolNames() returned unexpected name: %q", name)
		}
		delete(expected, name)
	}
}

func TestComponentInventory_PluginNames(t *testing.T) {
	inv := &ComponentInventory{
		Plugins: []PluginSummary{
			{Name: "mitre-lookup"},
			{Name: "cve-search"},
		},
	}

	names := inv.PluginNames()
	if len(names) != 2 {
		t.Fatalf("PluginNames() returned %d names, want 2", len(names))
	}

	expected := map[string]bool{
		"mitre-lookup": true,
		"cve-search":   true,
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("PluginNames() returned unexpected name: %q", name)
		}
		delete(expected, name)
	}
}

func TestComponentInventory_FilterAgentsByCapability(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci", Capabilities: []string{"jailbreak", "prompt_injection"}},
			{Name: "k8skiller", Capabilities: []string{"container_escape"}},
			{Name: "michelangelo", Capabilities: []string{"jailbreak", "model_extraction"}},
		},
	}

	tests := []struct {
		name       string
		capability string
		expected   []string
	}{
		{
			name:       "jailbreak capability",
			capability: "jailbreak",
			expected:   []string{"davinci", "michelangelo"},
		},
		{
			name:       "container_escape capability",
			capability: "container_escape",
			expected:   []string{"k8skiller"},
		},
		{
			name:       "non-existent capability",
			capability: "nonexistent",
			expected:   []string{},
		},
		{
			name:       "case insensitive",
			capability: "JAILBREAK",
			expected:   []string{"davinci", "michelangelo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inv.FilterAgentsByCapability(tt.capability)
			if len(result) != len(tt.expected) {
				t.Fatalf("FilterAgentsByCapability(%q) returned %d agents, want %d",
					tt.capability, len(result), len(tt.expected))
			}

			names := make(map[string]bool)
			for _, agent := range result {
				names[agent.Name] = true
			}

			for _, expected := range tt.expected {
				if !names[expected] {
					t.Errorf("FilterAgentsByCapability(%q) missing agent %q", tt.capability, expected)
				}
			}
		})
	}
}

func TestComponentInventory_FilterAgentsByTargetType(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci", TargetTypes: []string{"llm_chat", "llm_api"}},
			{Name: "k8skiller", TargetTypes: []string{"kubernetes"}},
			{Name: "michelangelo", TargetTypes: []string{"llm_chat"}},
		},
	}

	tests := []struct {
		name       string
		targetType string
		expected   []string
	}{
		{
			name:       "llm_chat target type",
			targetType: "llm_chat",
			expected:   []string{"davinci", "michelangelo"},
		},
		{
			name:       "kubernetes target type",
			targetType: "kubernetes",
			expected:   []string{"k8skiller"},
		},
		{
			name:       "non-existent target type",
			targetType: "nonexistent",
			expected:   []string{},
		},
		{
			name:       "case insensitive",
			targetType: "LLM_CHAT",
			expected:   []string{"davinci", "michelangelo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inv.FilterAgentsByTargetType(tt.targetType)
			if len(result) != len(tt.expected) {
				t.Fatalf("FilterAgentsByTargetType(%q) returned %d agents, want %d",
					tt.targetType, len(result), len(tt.expected))
			}

			names := make(map[string]bool)
			for _, agent := range result {
				names[agent.Name] = true
			}

			for _, expected := range tt.expected {
				if !names[expected] {
					t.Errorf("FilterAgentsByTargetType(%q) missing agent %q", tt.targetType, expected)
				}
			}
		})
	}
}

func TestComponentInventory_FilterToolsByTag(t *testing.T) {
	inv := &ComponentInventory{
		Tools: []ToolSummary{
			{Name: "nmap", Tags: []string{"network", "scanner"}},
			{Name: "sqlmap", Tags: []string{"database", "scanner"}},
			{Name: "hydra", Tags: []string{"password", "brute-force"}},
		},
	}

	tests := []struct {
		name     string
		tag      string
		expected []string
	}{
		{
			name:     "scanner tag",
			tag:      "scanner",
			expected: []string{"nmap", "sqlmap"},
		},
		{
			name:     "network tag",
			tag:      "network",
			expected: []string{"nmap"},
		},
		{
			name:     "non-existent tag",
			tag:      "nonexistent",
			expected: []string{},
		},
		{
			name:     "case insensitive",
			tag:      "SCANNER",
			expected: []string{"nmap", "sqlmap"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inv.FilterToolsByTag(tt.tag)
			if len(result) != len(tt.expected) {
				t.Fatalf("FilterToolsByTag(%q) returned %d tools, want %d",
					tt.tag, len(result), len(tt.expected))
			}

			names := make(map[string]bool)
			for _, tool := range result {
				names[tool.Name] = true
			}

			for _, expected := range tt.expected {
				if !names[expected] {
					t.Errorf("FilterToolsByTag(%q) missing tool %q", tt.tag, expected)
				}
			}
		})
	}
}

// TestComponentValidationError_Error and TestComponentValidationError_Is
// are tested in inventory_validator_test.go

func TestNewAgentNotFoundError(t *testing.T) {
	err := NewAgentNotFoundError("davinci", []string{"k8skiller", "michelangelo"})

	if err.ComponentType != "agent" {
		t.Errorf("ComponentType = %q, want %q", err.ComponentType, "agent")
	}
	if err.RequestedName != "davinci" {
		t.Errorf("RequestedName = %q, want %q", err.RequestedName, "davinci")
	}
	if len(err.Available) != 2 {
		t.Errorf("len(Available) = %d, want 2", len(err.Available))
	}
}

func TestNewToolNotFoundError(t *testing.T) {
	err := NewToolNotFoundError("nmap", []string{"sqlmap"})

	if err.ComponentType != "tool" {
		t.Errorf("ComponentType = %q, want %q", err.ComponentType, "tool")
	}
	if err.RequestedName != "nmap" {
		t.Errorf("RequestedName = %q, want %q", err.RequestedName, "nmap")
	}
}

func TestNewPluginNotFoundError(t *testing.T) {
	err := NewPluginNotFoundError("mitre-lookup", []string{})

	if err.ComponentType != "plugin" {
		t.Errorf("ComponentType = %q, want %q", err.ComponentType, "plugin")
	}
	if err.RequestedName != "mitre-lookup" {
		t.Errorf("RequestedName = %q, want %q", err.RequestedName, "mitre-lookup")
	}
}

func TestComponentInventory_TotalComponents(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "k8skiller"},
		},
		Tools: []ToolSummary{
			{Name: "nmap"},
		},
		Plugins: []PluginSummary{
			{Name: "mitre-lookup"},
			{Name: "cve-search"},
		},
		GatheredAt:      time.Now(),
		IsStale:         false,
		TotalComponents: 5,
	}

	if inv.TotalComponents != 5 {
		t.Errorf("TotalComponents = %d, want 5", inv.TotalComponents)
	}
}

// TestComponentInventory_EmptyInventory tests methods on empty inventory
func TestComponentInventory_EmptyInventory(t *testing.T) {
	inv := &ComponentInventory{
		Agents:  []AgentSummary{},
		Tools:   []ToolSummary{},
		Plugins: []PluginSummary{},
	}

	// Test Has* methods on empty inventory
	if inv.HasAgent("any") {
		t.Error("HasAgent should return false for empty inventory")
	}
	if inv.HasTool("any") {
		t.Error("HasTool should return false for empty inventory")
	}
	if inv.HasPlugin("any") {
		t.Error("HasPlugin should return false for empty inventory")
	}

	// Test Get* methods on empty inventory
	if inv.GetAgent("any") != nil {
		t.Error("GetAgent should return nil for empty inventory")
	}
	if inv.GetTool("any") != nil {
		t.Error("GetTool should return nil for empty inventory")
	}
	if inv.GetPlugin("any") != nil {
		t.Error("GetPlugin should return nil for empty inventory")
	}

	// Test Names methods on empty inventory
	agentNames := inv.AgentNames()
	if len(agentNames) != 0 {
		t.Errorf("AgentNames() returned %d names, want 0", len(agentNames))
	}
	toolNames := inv.ToolNames()
	if len(toolNames) != 0 {
		t.Errorf("ToolNames() returned %d names, want 0", len(toolNames))
	}
	pluginNames := inv.PluginNames()
	if len(pluginNames) != 0 {
		t.Errorf("PluginNames() returned %d names, want 0", len(pluginNames))
	}

	// Test Filter methods on empty inventory
	filtered := inv.FilterAgentsByCapability("jailbreak")
	if len(filtered) != 0 {
		t.Errorf("FilterAgentsByCapability returned %d agents, want 0", len(filtered))
	}

	filtered = inv.FilterAgentsByTargetType("llm_chat")
	if len(filtered) != 0 {
		t.Errorf("FilterAgentsByTargetType returned %d agents, want 0", len(filtered))
	}

	filteredTools := inv.FilterToolsByTag("network")
	if len(filteredTools) != 0 {
		t.Errorf("FilterToolsByTag returned %d tools, want 0", len(filteredTools))
	}
}

// TestComponentValidationError_Error tests the Error() method
func TestComponentValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ComponentValidationError
		contains []string
	}{
		{
			name: "agent not found with suggestions",
			err: &ComponentValidationError{
				ComponentType: "agent",
				RequestedName: "davinki",
				Available:     []string{"davinci", "k8skiller"},
				Suggestions:   []string{"davinci"},
			},
			contains: []string{"agent", "davinki", "not found", "did you mean", "davinci"},
		},
		{
			name: "tool not found without suggestions",
			err: &ComponentValidationError{
				ComponentType: "tool",
				RequestedName: "unknown",
				Available:     []string{"nmap", "sqlmap"},
				Suggestions:   []string{},
			},
			contains: []string{"tool", "unknown", "not found", "available tools", "nmap", "sqlmap"},
		},
		{
			name: "plugin not found with no available plugins",
			err: &ComponentValidationError{
				ComponentType: "plugin",
				RequestedName: "missing",
				Available:     []string{},
				Suggestions:   []string{},
			},
			contains: []string{"plugin", "missing", "not found", "no plugins registered"},
		},
		{
			name: "custom message",
			err: &ComponentValidationError{
				ComponentType: "agent",
				RequestedName: "test",
				Message:       "Custom error message",
			},
			contains: []string{"Custom error message"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			for _, expected := range tt.contains {
				if !contains(errMsg, expected) {
					t.Errorf("Error message missing %q\nGot: %s", expected, errMsg)
				}
			}
		})
	}
}

// TestComponentValidationError_Is tests the Is() method for error matching
func TestComponentValidationError_Is(t *testing.T) {
	err1 := &ComponentValidationError{
		ComponentType: "agent",
		RequestedName: "davinci",
		Available:     []string{"k8skiller"},
	}

	err2 := &ComponentValidationError{
		ComponentType: "agent",
		RequestedName: "davinci",
		Available:     []string{"michelangelo"}, // Different available list
	}

	err3 := &ComponentValidationError{
		ComponentType: "tool",
		RequestedName: "davinci", // Same name, different type
		Available:     []string{},
	}

	err4 := &ComponentValidationError{
		ComponentType: "agent",
		RequestedName: "k8skiller", // Different name, same type
		Available:     []string{},
	}

	// Test matching errors (same type and name)
	if !err1.Is(err2) {
		t.Error("err1 should match err2 (same type and name)")
	}
	if !err2.Is(err1) {
		t.Error("err2 should match err1 (symmetric)")
	}

	// Test non-matching errors (different type)
	if err1.Is(err3) {
		t.Error("err1 should not match err3 (different type)")
	}

	// Test non-matching errors (different name)
	if err1.Is(err4) {
		t.Error("err1 should not match err4 (different name)")
	}

	// Test case-insensitive type matching
	err5 := &ComponentValidationError{
		ComponentType: "AGENT", // Uppercase
		RequestedName: "davinci",
	}
	if !err1.Is(err5) {
		t.Error("err1 should match err5 (case-insensitive type)")
	}

	// Test Is with non-ComponentValidationError
	var otherErr error = errors.New("different error")
	if err1.Is(otherErr) {
		t.Error("ComponentValidationError should not match different error type")
	}
}

// contains helper is defined in decision_test.go
