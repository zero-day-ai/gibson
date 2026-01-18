package orchestrator

import (
	"strings"
	"testing"
)

// TestNewInventoryPromptFormatter tests constructor with default and custom options
func TestNewInventoryPromptFormatter(t *testing.T) {
	tests := []struct {
		name             string
		opts             []FormatterOption
		expectedBudget   int
		expectedVerbose  bool
	}{
		{
			name:            "default options",
			opts:            nil,
			expectedBudget:  500,
			expectedVerbose: false,
		},
		{
			name:            "custom token budget",
			opts:            []FormatterOption{WithMaxTokenBudget(1000)},
			expectedBudget:  1000,
			expectedVerbose: false,
		},
		{
			name:            "verbose mode enabled",
			opts:            []FormatterOption{WithVerboseMode(true)},
			expectedBudget:  500,
			expectedVerbose: true,
		},
		{
			name: "combined options",
			opts: []FormatterOption{
				WithMaxTokenBudget(750),
				WithVerboseMode(true),
			},
			expectedBudget:  750,
			expectedVerbose: true,
		},
		{
			name:            "unlimited budget",
			opts:            []FormatterOption{WithMaxTokenBudget(0)},
			expectedBudget:  0,
			expectedVerbose: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewInventoryPromptFormatter(tt.opts...)

			if formatter.maxTokenBudget != tt.expectedBudget {
				t.Errorf("maxTokenBudget = %d, want %d", formatter.maxTokenBudget, tt.expectedBudget)
			}

			if formatter.verboseMode != tt.expectedVerbose {
				t.Errorf("verboseMode = %v, want %v", formatter.verboseMode, tt.expectedVerbose)
			}
		})
	}
}

// TestFormatAgents tests agent table formatting
func TestFormatAgents(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name              string
		agents            []AgentSummary
		missionTargetType string
		wantContains      []string
		wantNotContains   []string
	}{
		{
			name:   "empty agents list",
			agents: []AgentSummary{},
			wantContains: []string{},
		},
		{
			name: "single agent",
			agents: []AgentSummary{
				{
					Name:         "davinci",
					Capabilities: []string{"prompt_injection", "jailbreak"},
					TargetTypes:  []string{"llm_chat", "llm_api"},
					HealthStatus: "healthy",
					Instances:    2,
				},
			},
			wantContains: []string{
				"### Agents (for spawn_agent)",
				"| Name | Capabilities | Target Types | Health | Instances |",
				"davinci",
				"prompt_injection, jailbreak",
				"llm_chat, llm_api",
				"healthy",
				"2",
			},
		},
		{
			name: "agent with matching target type",
			agents: []AgentSummary{
				{
					Name:         "davinci",
					Capabilities: []string{"prompt_injection"},
					TargetTypes:  []string{"llm_chat", "llm_api"},
					HealthStatus: "healthy",
					Instances:    1,
				},
			},
			missionTargetType: "llm_chat",
			wantContains: []string{
				"**davinci**",       // Agent name should be bold
				"**llm_chat**",      // Matching target type should be bold
				"llm_api",           // Non-matching target type should not be bold
			},
		},
		{
			name: "multiple agents",
			agents: []AgentSummary{
				{
					Name:         "davinci",
					Capabilities: []string{"prompt_injection"},
					TargetTypes:  []string{"llm_chat"},
					HealthStatus: "healthy",
					Instances:    2,
				},
				{
					Name:         "k8skiller",
					Capabilities: []string{"container_escape", "rbac_abuse"},
					TargetTypes:  []string{"kubernetes"},
					HealthStatus: "degraded",
					Instances:    1,
				},
			},
			missionTargetType: "llm_chat",
			wantContains: []string{
				"**davinci**",  // Matching agent bold
				"k8skiller",    // Non-matching agent not bold
				"healthy",
				"degraded",
			},
		},
		{
			name: "agent with long capabilities list",
			agents: []AgentSummary{
				{
					Name: "multi-agent",
					Capabilities: []string{
						"cap1", "cap2", "cap3", "cap4", "cap5",
						"cap6", "cap7", "cap8", "cap9", "cap10",
					},
					TargetTypes:  []string{"test"},
					HealthStatus: "healthy",
					Instances:    1,
				},
			},
			wantContains: []string{
				"multi-agent",
				"...", // Should be truncated
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatAgents(tt.agents, tt.missionTargetType)

			if len(tt.agents) == 0 && result != "" {
				t.Errorf("expected empty string for empty agents list, got: %s", result)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("FormatAgents() missing expected content: %q\nGot:\n%s", want, result)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result, notWant) {
					t.Errorf("FormatAgents() contains unexpected content: %q\nGot:\n%s", notWant, result)
				}
			}
		})
	}
}

// TestFormatTools tests tool table formatting
func TestFormatTools(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name         string
		tools        []ToolSummary
		wantContains []string
	}{
		{
			name:         "empty tools list",
			tools:        []ToolSummary{},
			wantContains: []string{},
		},
		{
			name: "single tool",
			tools: []ToolSummary{
				{
					Name:         "nmap",
					Tags:         []string{"network", "enumeration"},
					Description:  "Port scanning and service detection",
					HealthStatus: "healthy",
				},
			},
			wantContains: []string{
				"### Tools",
				"| Name | Tags | Description | Health |",
				"nmap",
				"network, enumeration",
				"Port scanning and service detection",
				"healthy",
			},
		},
		{
			name: "multiple tools",
			tools: []ToolSummary{
				{
					Name:         "nmap",
					Tags:         []string{"network"},
					Description:  "Port scanner",
					HealthStatus: "healthy",
				},
				{
					Name:         "sqlmap",
					Tags:         []string{"web", "injection"},
					Description:  "SQL injection testing",
					HealthStatus: "healthy",
				},
			},
			wantContains: []string{
				"nmap",
				"sqlmap",
				"network",
				"web, injection",
			},
		},
		{
			name: "tool with long description",
			tools: []ToolSummary{
				{
					Name: "long-tool",
					Tags: []string{"test"},
					Description: "This is a very long description that should be truncated " +
						"because it exceeds the maximum length allowed in the table format",
					HealthStatus: "healthy",
				},
			},
			wantContains: []string{
				"long-tool",
				"...", // Should be truncated
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatTools(tt.tools)

			if len(tt.tools) == 0 && result != "" {
				t.Errorf("expected empty string for empty tools list, got: %s", result)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("FormatTools() missing expected content: %q\nGot:\n%s", want, result)
				}
			}
		})
	}
}

// TestFormatPlugins tests plugin table formatting
func TestFormatPlugins(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name         string
		plugins      []PluginSummary
		wantContains []string
	}{
		{
			name:         "empty plugins list",
			plugins:      []PluginSummary{},
			wantContains: []string{},
		},
		{
			name: "single plugin",
			plugins: []PluginSummary{
				{
					Name: "mitre-lookup",
					Methods: []MethodSummary{
						{Name: "getTechnique"},
						{Name: "mapToTactic"},
					},
					HealthStatus: "healthy",
				},
			},
			wantContains: []string{
				"### Plugins",
				"| Name | Methods | Health |",
				"mitre-lookup",
				"getTechnique, mapToTactic",
				"healthy",
			},
		},
		{
			name: "multiple plugins",
			plugins: []PluginSummary{
				{
					Name: "mitre-lookup",
					Methods: []MethodSummary{
						{Name: "getTechnique"},
					},
					HealthStatus: "healthy",
				},
				{
					Name: "report-gen",
					Methods: []MethodSummary{
						{Name: "generateHTML"},
						{Name: "generatePDF"},
					},
					HealthStatus: "healthy",
				},
			},
			wantContains: []string{
				"mitre-lookup",
				"report-gen",
				"getTechnique",
				"generateHTML, generatePDF",
			},
		},
		{
			name: "plugin with many methods",
			plugins: []PluginSummary{
				{
					Name: "big-plugin",
					Methods: []MethodSummary{
						{Name: "method1"}, {Name: "method2"}, {Name: "method3"},
						{Name: "method4"}, {Name: "method5"}, {Name: "method6"},
						{Name: "method7"}, {Name: "method8"}, {Name: "method9"},
					},
					HealthStatus: "healthy",
				},
			},
			wantContains: []string{
				"big-plugin",
				"...", // Should be truncated
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatPlugins(tt.plugins)

			if len(tt.plugins) == 0 && result != "" {
				t.Errorf("expected empty string for empty plugins list, got: %s", result)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("FormatPlugins() missing expected content: %q\nGot:\n%s", want, result)
				}
			}
		})
	}
}

// TestFormatCondensed tests condensed format
func TestFormatCondensed(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name         string
		inv          *ComponentInventory
		wantContains []string
	}{
		{
			name:         "nil inventory",
			inv:          nil,
			wantContains: []string{},
		},
		{
			name: "empty inventory",
			inv: &ComponentInventory{
				Agents:  []AgentSummary{},
				Tools:   []ToolSummary{},
				Plugins: []PluginSummary{},
			},
			wantContains: []string{
				"## Available Components (0 total)",
			},
		},
		{
			name: "inventory with all component types",
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{Name: "davinci"},
					{Name: "k8skiller"},
				},
				Tools: []ToolSummary{
					{Name: "nmap"},
					{Name: "sqlmap"},
					{Name: "ffuf"},
				},
				Plugins: []PluginSummary{
					{Name: "mitre-lookup"},
				},
			},
			wantContains: []string{
				"## Available Components (6 total)",
				"**Agents (2):** davinci, k8skiller",
				"**Tools (3):** nmap, sqlmap, ffuf",
				"**Plugins (1):** mitre-lookup",
				"*Use spawn_agent only with agent names from the list above.*",
			},
		},
		{
			name: "stale inventory",
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{Name: "agent1"},
				},
				IsStale: true,
			},
			wantContains: []string{
				"*(Cached - registry unavailable)*",
				"**Agents (1):** agent1",
			},
		},
		{
			name: "only agents",
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{Name: "agent1"},
					{Name: "agent2"},
				},
			},
			wantContains: []string{
				"## Available Components (2 total)",
				"**Agents (2):** agent1, agent2",
				"*Use spawn_agent only with agent names from the list above.*",
			},
		},
		{
			name: "only tools",
			inv: &ComponentInventory{
				Tools: []ToolSummary{
					{Name: "tool1"},
				},
			},
			wantContains: []string{
				"## Available Components (1 total)",
				"**Tools (1):** tool1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatCondensed(tt.inv)

			if tt.inv == nil && result != "" {
				t.Errorf("expected empty string for nil inventory, got: %s", result)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("FormatCondensed() missing expected content: %q\nGot:\n%s", want, result)
				}
			}
		})
	}
}

// TestFormat tests the main Format function with budget management
func TestFormat(t *testing.T) {
	tests := []struct {
		name              string
		formatter         *InventoryPromptFormatter
		inv               *ComponentInventory
		missionTargetType string
		wantFormat        string // "full" or "condensed"
	}{
		{
			name:       "nil inventory",
			formatter:  NewInventoryPromptFormatter(),
			inv:        nil,
			wantFormat: "",
		},
		{
			name:      "small inventory with default budget - full format",
			formatter: NewInventoryPromptFormatter(),
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{
						Name:         "davinci",
						Capabilities: []string{"prompt_injection"},
						TargetTypes:  []string{"llm_chat"},
						HealthStatus: "healthy",
						Instances:    1,
					},
				},
			},
			missionTargetType: "llm_chat",
			wantFormat:        "full",
		},
		{
			name:      "large inventory with small budget - condensed format",
			formatter: NewInventoryPromptFormatter(WithMaxTokenBudget(50)),
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{Name: "agent1", Capabilities: []string{"cap1"}, TargetTypes: []string{"type1"}, HealthStatus: "healthy", Instances: 1},
					{Name: "agent2", Capabilities: []string{"cap2"}, TargetTypes: []string{"type2"}, HealthStatus: "healthy", Instances: 1},
					{Name: "agent3", Capabilities: []string{"cap3"}, TargetTypes: []string{"type3"}, HealthStatus: "healthy", Instances: 1},
				},
				Tools: []ToolSummary{
					{Name: "tool1", Tags: []string{"tag1"}, Description: "desc1", HealthStatus: "healthy"},
					{Name: "tool2", Tags: []string{"tag2"}, Description: "desc2", HealthStatus: "healthy"},
				},
				Plugins: []PluginSummary{
					{Name: "plugin1", Methods: []MethodSummary{{Name: "method1"}}, HealthStatus: "healthy"},
				},
			},
			wantFormat: "condensed",
		},
		{
			name:      "unlimited budget - always full format",
			formatter: NewInventoryPromptFormatter(WithMaxTokenBudget(0)),
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{Name: "agent1", Capabilities: []string{"cap1"}, TargetTypes: []string{"type1"}, HealthStatus: "healthy", Instances: 1},
				},
			},
			wantFormat: "full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.formatter.Format(tt.inv, tt.missionTargetType)

			if tt.inv == nil {
				if result != "" {
					t.Errorf("expected empty string for nil inventory, got: %s", result)
				}
				return
			}

			switch tt.wantFormat {
			case "full":
				// Full format should have table headers
				if !strings.Contains(result, "| Name |") {
					t.Errorf("expected full table format, but got condensed format or error")
				}
			case "condensed":
				// Condensed format should not have table headers
				if strings.Contains(result, "| Name |") {
					t.Errorf("expected condensed format, but got full table format")
				}
				// Should still contain component names
				if tt.inv.Agents != nil && len(tt.inv.Agents) > 0 {
					if !strings.Contains(result, "**Agents") {
						t.Errorf("condensed format missing Agents section")
					}
				}
			}
		})
	}
}

// TestEstimateTokens tests token estimation
func TestEstimateTokens(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name        string
		inv         *ComponentInventory
		expectZero  bool
		expectRange bool
		minTokens   int
		maxTokens   int
	}{
		{
			name:       "nil inventory",
			inv:        nil,
			expectZero: true,
		},
		{
			name: "empty inventory",
			inv: &ComponentInventory{
				Agents:  []AgentSummary{},
				Tools:   []ToolSummary{},
				Plugins: []PluginSummary{},
			},
			expectRange: true,
			minTokens:   0,
			maxTokens:   50, // Header text only
		},
		{
			name: "small inventory",
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{
						Name:         "davinci",
						Capabilities: []string{"prompt_injection"},
						TargetTypes:  []string{"llm_chat"},
						HealthStatus: "healthy",
						Instances:    1,
					},
				},
			},
			expectRange: true,
			minTokens:   50,
			maxTokens:   200,
		},
		{
			name: "large inventory",
			inv: &ComponentInventory{
				Agents: []AgentSummary{
					{Name: "agent1", Capabilities: []string{"cap1", "cap2"}, TargetTypes: []string{"type1"}, HealthStatus: "healthy", Instances: 1},
					{Name: "agent2", Capabilities: []string{"cap3", "cap4"}, TargetTypes: []string{"type2"}, HealthStatus: "healthy", Instances: 2},
					{Name: "agent3", Capabilities: []string{"cap5", "cap6"}, TargetTypes: []string{"type3"}, HealthStatus: "degraded", Instances: 1},
				},
				Tools: []ToolSummary{
					{Name: "tool1", Tags: []string{"tag1"}, Description: "description1", HealthStatus: "healthy"},
					{Name: "tool2", Tags: []string{"tag2"}, Description: "description2", HealthStatus: "healthy"},
				},
				Plugins: []PluginSummary{
					{Name: "plugin1", Methods: []MethodSummary{{Name: "method1"}}, HealthStatus: "healthy"},
				},
			},
			expectRange: true,
			minTokens:   100,
			maxTokens:   500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := formatter.EstimateTokens(tt.inv)

			if tt.expectZero {
				if tokens != 0 {
					t.Errorf("EstimateTokens() = %d, want 0", tokens)
				}
				return
			}

			if tt.expectRange {
				if tokens < tt.minTokens || tokens > tt.maxTokens {
					t.Errorf("EstimateTokens() = %d, want between %d and %d",
						tokens, tt.minTokens, tt.maxTokens)
				}
			}

			// Verify estimation is reasonable (not negative, not absurdly high)
			if tokens < 0 {
				t.Errorf("EstimateTokens() = %d, should not be negative", tokens)
			}
		})
	}
}

// TestTruncateString tests string truncation helper
func TestTruncateString(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "no truncation needed",
			input:  "short",
			maxLen: 10,
			want:   "short",
		},
		{
			name:   "exact length",
			input:  "exactly10c",
			maxLen: 10,
			want:   "exactly10c",
		},
		{
			name:   "truncation needed",
			input:  "this is a very long string that needs truncation",
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name:   "very short max length",
			input:  "truncate",
			maxLen: 3,
			want:   "...",
		},
		{
			name:   "max length less than ellipsis",
			input:  "test",
			maxLen: 2,
			want:   "...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.truncateString(tt.input, tt.maxLen)
			if result != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q",
					tt.input, tt.maxLen, result, tt.want)
			}
		})
	}
}

// TestJoinAndTruncate tests string joining with truncation
func TestJoinAndTruncate(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name   string
		items  []string
		sep    string
		maxLen int
		want   string
	}{
		{
			name:   "empty list",
			items:  []string{},
			sep:    ", ",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "single item",
			items:  []string{"item1"},
			sep:    ", ",
			maxLen: 10,
			want:   "item1",
		},
		{
			name:   "multiple items no truncation",
			items:  []string{"a", "b", "c"},
			sep:    ", ",
			maxLen: 20,
			want:   "a, b, c",
		},
		{
			name:   "multiple items with truncation",
			items:  []string{"item1", "item2", "item3", "item4"},
			sep:    ", ",
			maxLen: 15,
			want:   "item1, item2...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.joinAndTruncate(tt.items, tt.sep, tt.maxLen)
			if result != tt.want {
				t.Errorf("joinAndTruncate(%v, %q, %d) = %q, want %q",
					tt.items, tt.sep, tt.maxLen, result, tt.want)
			}
		})
	}
}

// TestAgentMatchesTargetType tests target type matching logic
func TestAgentMatchesTargetType(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	tests := []struct {
		name       string
		agent      AgentSummary
		targetType string
		want       bool
	}{
		{
			name: "exact match",
			agent: AgentSummary{
				TargetTypes: []string{"llm_chat", "llm_api"},
			},
			targetType: "llm_chat",
			want:       true,
		},
		{
			name: "no match",
			agent: AgentSummary{
				TargetTypes: []string{"kubernetes"},
			},
			targetType: "llm_chat",
			want:       false,
		},
		{
			name: "empty target types",
			agent: AgentSummary{
				TargetTypes: []string{},
			},
			targetType: "llm_chat",
			want:       false,
		},
		{
			name: "empty target type to match",
			agent: AgentSummary{
				TargetTypes: []string{"llm_chat"},
			},
			targetType: "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.agentMatchesTargetType(tt.agent, tt.targetType)
			if result != tt.want {
				t.Errorf("agentMatchesTargetType() = %v, want %v", result, tt.want)
			}
		})
	}
}

// TestFormatFullTablesWithStaleInventory tests handling of stale inventory
func TestFormatFullTablesWithStaleInventory(t *testing.T) {
	formatter := NewInventoryPromptFormatter()

	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{
				Name:         "agent1",
				Capabilities: []string{"cap1"},
				TargetTypes:  []string{"type1"},
				HealthStatus: "healthy",
				Instances:    1,
			},
		},
		IsStale: true,
	}

	result := formatter.Format(inv, "")

	if !strings.Contains(result, "stale") {
		t.Errorf("Format() should include stale warning for stale inventory, got:\n%s", result)
	}
}

// BenchmarkFormat benchmarks the Format function
func BenchmarkFormat(b *testing.B) {
	formatter := NewInventoryPromptFormatter()

	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "agent1", Capabilities: []string{"cap1"}, TargetTypes: []string{"type1"}, HealthStatus: "healthy", Instances: 1},
			{Name: "agent2", Capabilities: []string{"cap2"}, TargetTypes: []string{"type2"}, HealthStatus: "healthy", Instances: 1},
		},
		Tools: []ToolSummary{
			{Name: "tool1", Tags: []string{"tag1"}, Description: "desc1", HealthStatus: "healthy"},
			{Name: "tool2", Tags: []string{"tag2"}, Description: "desc2", HealthStatus: "healthy"},
		},
		Plugins: []PluginSummary{
			{Name: "plugin1", Methods: []MethodSummary{{Name: "method1"}}, HealthStatus: "healthy"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = formatter.Format(inv, "type1")
	}
}

// BenchmarkEstimateTokens benchmarks token estimation
func BenchmarkEstimateTokens(b *testing.B) {
	formatter := NewInventoryPromptFormatter()

	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "agent1", Capabilities: []string{"cap1"}, TargetTypes: []string{"type1"}, HealthStatus: "healthy", Instances: 1},
			{Name: "agent2", Capabilities: []string{"cap2"}, TargetTypes: []string{"type2"}, HealthStatus: "healthy", Instances: 1},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = formatter.EstimateTokens(inv)
	}
}
