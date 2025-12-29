package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestAssembler_BasicAssembly(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register a simple system prompt
	systemPrompt := Prompt{
		ID:       "test-system",
		Position: PositionSystem,
		Content:  "You are a helpful assistant.",
		Priority: 100,
	}
	if err := registry.Register(systemPrompt); err != nil {
		t.Fatalf("failed to register prompt: %v", err)
	}

	// Register a simple user prompt
	userPrompt := Prompt{
		ID:       "test-user",
		Position: PositionUser,
		Content:  "Hello, how are you?",
		Priority: 100,
	}
	if err := registry.Register(userPrompt); err != nil {
		t.Fatalf("failed to register prompt: %v", err)
	}

	// Assemble
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry: registry,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Verify system content
	if result.System != "You are a helpful assistant." {
		t.Errorf("expected system='You are a helpful assistant.', got='%s'", result.System)
	}

	// Verify user content
	if result.User != "Hello, how are you?" {
		t.Errorf("expected user='Hello, how are you?', got='%s'", result.User)
	}

	// Verify messages
	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}

	if result.Messages[0].Role != "system" {
		t.Errorf("expected first message role='system', got='%s'", result.Messages[0].Role)
	}
	if result.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("expected first message content='You are a helpful assistant.', got='%s'", result.Messages[0].Content)
	}

	if result.Messages[1].Role != "user" {
		t.Errorf("expected second message role='user', got='%s'", result.Messages[1].Role)
	}
	if result.Messages[1].Content != "Hello, how are you?" {
		t.Errorf("expected second message content='Hello, how are you?', got='%s'", result.Messages[1].Content)
	}
}

func TestAssembler_PositionOrdering(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register prompts in various positions
	prompts := []Prompt{
		{ID: "prefix", Position: PositionSystemPrefix, Content: "PREFIX", Priority: 100},
		{ID: "system", Position: PositionSystem, Content: "SYSTEM", Priority: 100},
		{ID: "suffix", Position: PositionSystemSuffix, Content: "SUFFIX", Priority: 100},
		{ID: "context", Position: PositionContext, Content: "CONTEXT", Priority: 100},
		{ID: "tools", Position: PositionTools, Content: "TOOLS", Priority: 100},
		{ID: "plugins", Position: PositionPlugins, Content: "PLUGINS", Priority: 100},
		{ID: "agents", Position: PositionAgents, Content: "AGENTS", Priority: 100},
		{ID: "constraints", Position: PositionConstraints, Content: "CONSTRAINTS", Priority: 100},
		{ID: "examples", Position: PositionExamples, Content: "EXAMPLES", Priority: 100},
		{ID: "user-prefix", Position: PositionUserPrefix, Content: "USER-PREFIX", Priority: 100},
		{ID: "user", Position: PositionUser, Content: "USER", Priority: 100},
		{ID: "user-suffix", Position: PositionUserSuffix, Content: "USER-SUFFIX", Priority: 100},
	}

	for _, p := range prompts {
		if err := registry.Register(p); err != nil {
			t.Fatalf("failed to register prompt %s: %v", p.ID, err)
		}
	}

	// Assemble with all options enabled
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry:       registry,
		IncludeTools:   true,
		IncludePlugins: true,
		IncludeAgents:  true,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Verify system positions are in correct order
	expectedSystem := "PREFIX\n\nSYSTEM\n\nSUFFIX\n\nCONTEXT\n\nTOOLS\n\nPLUGINS\n\nAGENTS\n\nCONSTRAINTS\n\nEXAMPLES"
	if result.System != expectedSystem {
		t.Errorf("system content order incorrect:\nexpected:\n%s\ngot:\n%s", expectedSystem, result.System)
	}

	// Verify user positions are in correct order
	expectedUser := "USER-PREFIX\n\nUSER\n\nUSER-SUFFIX"
	if result.User != expectedUser {
		t.Errorf("user content order incorrect:\nexpected:\n%s\ngot:\n%s", expectedUser, result.User)
	}
}

func TestAssembler_PrioritySorting(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register multiple prompts in the same position with different priorities
	prompts := []Prompt{
		{ID: "low", Position: PositionSystem, Content: "LOW", Priority: 10},
		{ID: "high", Position: PositionSystem, Content: "HIGH", Priority: 100},
		{ID: "medium", Position: PositionSystem, Content: "MEDIUM", Priority: 50},
	}

	for _, p := range prompts {
		if err := registry.Register(p); err != nil {
			t.Fatalf("failed to register prompt %s: %v", p.ID, err)
		}
	}

	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry: registry,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Higher priority should come first
	expected := "HIGH\n\nMEDIUM\n\nLOW"
	if result.System != expected {
		t.Errorf("priority sorting incorrect:\nexpected:\n%s\ngot:\n%s", expected, result.System)
	}
}

func TestAssembler_ConditionFiltering(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register prompts with conditions
	includePrompt := Prompt{
		ID:       "include",
		Position: PositionSystem,
		Content:  "INCLUDED",
		Priority: 100,
		Conditions: []Condition{
			{Field: "agent.enabled", Operator: OpEqual, Value: true},
		},
	}

	excludePrompt := Prompt{
		ID:       "exclude",
		Position: PositionSystem,
		Content:  "EXCLUDED",
		Priority: 90,
		Conditions: []Condition{
			{Field: "agent.enabled", Operator: OpEqual, Value: false},
		},
	}

	if err := registry.Register(includePrompt); err != nil {
		t.Fatalf("failed to register include prompt: %v", err)
	}
	if err := registry.Register(excludePrompt); err != nil {
		t.Fatalf("failed to register exclude prompt: %v", err)
	}

	// Create context with agent.enabled = true
	ctx := NewRenderContext()
	ctx.Agent["enabled"] = true

	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry:      registry,
		RenderContext: ctx,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Only the included prompt should appear
	if result.System != "INCLUDED" {
		t.Errorf("expected system='INCLUDED', got='%s'", result.System)
	}

	// Verify only one prompt was included
	if len(result.Prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(result.Prompts))
	}
}

func TestAssembler_SystemOverride(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register various system prompts
	prompts := []Prompt{
		{ID: "system1", Position: PositionSystem, Content: "System 1", Priority: 100},
		{ID: "system2", Position: PositionSystemPrefix, Content: "Prefix", Priority: 100},
		{ID: "user", Position: PositionUser, Content: "User message", Priority: 100},
	}

	for _, p := range prompts {
		if err := registry.Register(p); err != nil {
			t.Fatalf("failed to register prompt %s: %v", p.ID, err)
		}
	}

	// Assemble with system override
	override := "OVERRIDE SYSTEM MESSAGE"
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry:       registry,
		SystemOverride: override,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// System should be the override, not the registered prompts
	if result.System != override {
		t.Errorf("expected system='%s', got='%s'", override, result.System)
	}

	// User should still be assembled normally
	if result.User != "User message" {
		t.Errorf("expected user='User message', got='%s'", result.User)
	}
}

func TestAssembler_TemplateRendering(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register a prompt with a template
	prompt := Prompt{
		ID:       "template",
		Position: PositionSystem,
		Content:  "Hello, {{.Agent.name}}! Your mission is {{.Mission.objective}}.",
		Priority: 100,
	}

	if err := registry.Register(prompt); err != nil {
		t.Fatalf("failed to register prompt: %v", err)
	}

	// Create context with data
	ctx := NewRenderContext()
	ctx.Agent["name"] = "Gibson"
	ctx.Mission["objective"] = "test the system"

	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry:      registry,
		RenderContext: ctx,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	expected := "Hello, Gibson! Your mission is test the system."
	if result.System != expected {
		t.Errorf("expected system='%s', got='%s'", expected, result.System)
	}
}

func TestAssembler_EmptyRegistry(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Assemble with empty registry
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry: registry,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// System and User should be empty
	if result.System != "" {
		t.Errorf("expected empty system, got='%s'", result.System)
	}
	if result.User != "" {
		t.Errorf("expected empty user, got='%s'", result.User)
	}

	// Messages should be empty (no empty messages added)
	if len(result.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result.Messages))
	}

	// Prompts should be empty
	if len(result.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(result.Prompts))
	}
}

func TestAssembler_ExtraPrompts(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register a base prompt
	basePrompt := Prompt{
		ID:       "base",
		Position: PositionSystem,
		Content:  "BASE",
		Priority: 100,
	}
	if err := registry.Register(basePrompt); err != nil {
		t.Fatalf("failed to register base prompt: %v", err)
	}

	// Add extra prompts via options
	extraPrompts := []Prompt{
		{ID: "extra1", Position: PositionSystem, Content: "EXTRA1", Priority: 50},
		{ID: "extra2", Position: PositionUser, Content: "EXTRA-USER", Priority: 100},
	}

	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry:     registry,
		ExtraPrompts: extraPrompts,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// BASE should come first (higher priority), then EXTRA1
	if !strings.Contains(result.System, "BASE") || !strings.Contains(result.System, "EXTRA1") {
		t.Errorf("system should contain both BASE and EXTRA1, got='%s'", result.System)
	}

	// EXTRA-USER should be in user content
	if result.User != "EXTRA-USER" {
		t.Errorf("expected user='EXTRA-USER', got='%s'", result.User)
	}
}

func TestAssembler_PersonaPrompt(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register a persona prompt
	personaPrompt := Prompt{
		ID:       "persona:hacker",
		Position: PositionSystem,
		Content:  "You are an expert hacker.",
		Priority: 100,
	}
	if err := registry.Register(personaPrompt); err != nil {
		t.Fatalf("failed to register persona prompt: %v", err)
	}

	// Assemble with persona
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry: registry,
		Persona:  "hacker",
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Persona prompt should be included
	if result.System != "You are an expert hacker." {
		t.Errorf("expected persona prompt, got='%s'", result.System)
	}
}

func TestAssembler_IncludeFlags(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register prompts in optional positions
	prompts := []Prompt{
		{ID: "tools", Position: PositionTools, Content: "TOOLS", Priority: 100},
		{ID: "plugins", Position: PositionPlugins, Content: "PLUGINS", Priority: 100},
		{ID: "agents", Position: PositionAgents, Content: "AGENTS", Priority: 100},
	}

	for _, p := range prompts {
		if err := registry.Register(p); err != nil {
			t.Fatalf("failed to register prompt %s: %v", p.ID, err)
		}
	}

	tests := []struct {
		name           string
		includeTools   bool
		includePlugins bool
		includeAgents  bool
		expectContains []string
		expectMissing  []string
	}{
		{
			name:           "all excluded",
			includeTools:   false,
			includePlugins: false,
			includeAgents:  false,
			expectContains: []string{},
			expectMissing:  []string{"TOOLS", "PLUGINS", "AGENTS"},
		},
		{
			name:           "only tools",
			includeTools:   true,
			includePlugins: false,
			includeAgents:  false,
			expectContains: []string{"TOOLS"},
			expectMissing:  []string{"PLUGINS", "AGENTS"},
		},
		{
			name:           "only plugins",
			includeTools:   false,
			includePlugins: true,
			includeAgents:  false,
			expectContains: []string{"PLUGINS"},
			expectMissing:  []string{"TOOLS", "AGENTS"},
		},
		{
			name:           "only agents",
			includeTools:   false,
			includePlugins: false,
			includeAgents:  true,
			expectContains: []string{"AGENTS"},
			expectMissing:  []string{"TOOLS", "PLUGINS"},
		},
		{
			name:           "all included",
			includeTools:   true,
			includePlugins: true,
			includeAgents:  true,
			expectContains: []string{"TOOLS", "PLUGINS", "AGENTS"},
			expectMissing:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := assembler.Assemble(context.Background(), AssembleOptions{
				Registry:       registry,
				IncludeTools:   tt.includeTools,
				IncludePlugins: tt.includePlugins,
				IncludeAgents:  tt.includeAgents,
			})

			if err != nil {
				t.Fatalf("Assemble failed: %v", err)
			}

			for _, expect := range tt.expectContains {
				if !strings.Contains(result.System, expect) {
					t.Errorf("expected system to contain '%s', got='%s'", expect, result.System)
				}
			}

			for _, expect := range tt.expectMissing {
				if strings.Contains(result.System, expect) {
					t.Errorf("expected system NOT to contain '%s', got='%s'", expect, result.System)
				}
			}
		})
	}
}

func TestAssembler_NilRegistry(t *testing.T) {
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Assemble with nil registry
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry: nil,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Should return empty result
	if result.System != "" || result.User != "" || len(result.Messages) != 0 {
		t.Errorf("expected empty result with nil registry")
	}
}

func TestAssembler_NilRenderContext(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register a simple prompt
	prompt := Prompt{
		ID:       "test",
		Position: PositionSystem,
		Content:  "Simple prompt without templates",
		Priority: 100,
	}
	if err := registry.Register(prompt); err != nil {
		t.Fatalf("failed to register prompt: %v", err)
	}

	// Assemble with nil context
	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry:      registry,
		RenderContext: nil,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Should still work with simple prompts
	if result.System != "Simple prompt without templates" {
		t.Errorf("expected prompt content, got='%s'", result.System)
	}
}

func TestAssembler_WhitespaceHandling(t *testing.T) {
	registry := NewPromptRegistry()
	renderer := NewTemplateRenderer()
	assembler := NewAssembler(renderer)

	// Register prompts with various whitespace
	prompts := []Prompt{
		{ID: "p1", Position: PositionSystem, Content: "  Content with spaces  ", Priority: 100},
		{ID: "p2", Position: PositionSystem, Content: "\nContent with newlines\n", Priority: 90},
		{ID: "p3", Position: PositionSystem, Content: "   ", Priority: 80}, // Only whitespace
	}

	for _, p := range prompts {
		if err := registry.Register(p); err != nil {
			t.Fatalf("failed to register prompt %s: %v", p.ID, err)
		}
	}

	result, err := assembler.Assemble(context.Background(), AssembleOptions{
		Registry: registry,
	})

	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Whitespace should be trimmed, empty content excluded
	expected := "Content with spaces\n\nContent with newlines"
	if result.System != expected {
		t.Errorf("whitespace handling incorrect:\nexpected:\n%s\ngot:\n%s", expected, result.System)
	}
}
