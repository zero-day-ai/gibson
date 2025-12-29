package examples

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/prompt"
	"github.com/zero-day-ai/gibson/internal/prompt/transformers"
)

// ExamplePromptRelay demonstrates how to use the prompt relay system
// to delegate tasks from a parent agent to sub-agents with context injection
// and scope narrowing.
func ExamplePromptRelay() {
	// Create the relay
	relay := prompt.NewPromptRelay()

	// Define original prompts from parent agent
	originalPrompts := []prompt.Prompt{
		{
			ID:       "security_orchestrator_system",
			Position: prompt.PositionSystem,
			Content:  "You are a security orchestrator managing vulnerability scanning operations.",
			Priority: 10,
		},
		{
			ID:       "target_context",
			Position: prompt.PositionContext,
			Content:  "Target: Production authentication service with 10K active users.",
			Priority: 8,
		},
		{
			ID:       "available_tools",
			Position: prompt.PositionTools,
			Content:  "Tools: static_analyzer, dynamic_scanner, penetration_tester, report_generator",
			Priority: 5,
		},
		{
			ID:       "security_constraints",
			Position: prompt.PositionConstraints,
			Content:  "Constraints: Read-only access, No invasive testing, Business hours only",
			Priority: 4,
		},
		{
			ID:       "user_request",
			Position: prompt.PositionUser,
			Content:  "Perform comprehensive security audit focusing on authentication vulnerabilities",
			Priority: 1,
		},
	}

	// Setup relay context for delegating to Static Analyzer
	staticAnalyzerCtx := &prompt.RelayContext{
		SourceAgent: "SecurityOrchestrator",
		TargetAgent: "StaticAnalyzer",
		Task:        "Perform static code analysis on authentication module",
		Memory: map[string]any{
			"module":        "authentication",
			"language":      "go",
			"analysis_type": "security",
			"priority":      "high",
		},
		Constraints: []string{
			"Focus on OWASP Top 10 vulnerabilities",
			"Check for hardcoded credentials",
			"Analyze input validation",
			"Report severity >= MEDIUM",
		},
		Prompts: originalPrompts,
	}

	// Create context injector to add delegation context
	contextInjector := transformers.NewContextInjector()
	contextInjector.IncludeMemory = true
	contextInjector.IncludeConstraints = true

	// Create scope narrower to filter relevant prompts
	scopeNarrower := transformers.NewScopeNarrower()
	scopeNarrower.AllowedPositions = []prompt.Position{
		prompt.PositionSystemPrefix, // For injected context
		prompt.PositionSystem,
		prompt.PositionContext,
		prompt.PositionTools,
		prompt.PositionUser,
	}
	scopeNarrower.KeywordFilter = []string{"security", "authentication", "tools", "audit"}

	// Execute relay transformation
	staticAnalyzerPrompts, err := relay.Relay(staticAnalyzerCtx, contextInjector, scopeNarrower)
	if err != nil {
		fmt.Printf("Relay failed: %v\n", err)
		return
	}

	// Display transformed prompts
	fmt.Println("=== Static Analyzer Prompts ===")
	fmt.Printf("Total: %d prompts (from %d original)\n\n", len(staticAnalyzerPrompts), len(originalPrompts))

	for i, p := range staticAnalyzerPrompts {
		fmt.Printf("[%d] Position: %-15s Priority: %d\n", i+1, p.Position, p.Priority)
		fmt.Printf("    ID: %s\n", p.ID)
		if p.Position == prompt.PositionSystemPrefix {
			fmt.Printf("    Content: %s\n", p.Content)
		} else {
			fmt.Printf("    Content: %.60s...\n", p.Content)
		}
		fmt.Println()
	}

	// Verify original prompts are unchanged (immutability)
	fmt.Printf("Original prompts unchanged: %d prompts still present\n", len(originalPrompts))
	fmt.Printf("First original prompt ID: %s\n\n", originalPrompts[0].ID)

	// Example: Delegate to different sub-agent with different scope
	dynamicScannerCtx := &prompt.RelayContext{
		SourceAgent: "SecurityOrchestrator",
		TargetAgent: "DynamicScanner",
		Task:        "Perform runtime security testing on authentication endpoints",
		Memory: map[string]any{
			"endpoints":    []string{"/login", "/logout", "/refresh", "/reset-password"},
			"test_level":   "safe",
			"max_requests": 1000,
		},
		Constraints: []string{
			"Use only safe testing methods",
			"Monitor for DoS patterns",
			"Respect rate limits: 100 req/min",
		},
		Prompts: originalPrompts,
	}

	// Different scope for dynamic scanner (exclude tools, focus on constraints)
	dynamicScopeNarrower := transformers.NewScopeNarrower()
	dynamicScopeNarrower.AllowedPositions = []prompt.Position{
		prompt.PositionSystemPrefix,
		prompt.PositionContext,
		prompt.PositionConstraints,
		prompt.PositionUser,
	}

	dynamicScannerPrompts, err := relay.Relay(dynamicScannerCtx, contextInjector, dynamicScopeNarrower)
	if err != nil {
		fmt.Printf("Relay failed: %v\n", err)
		return
	}

	fmt.Println("=== Dynamic Scanner Prompts ===")
	fmt.Printf("Total: %d prompts (different scope from Static Analyzer)\n\n", len(dynamicScannerPrompts))

	// Example: Custom template for specific formatting
	fmt.Println("=== Custom Template Example ===")

	customInjector := transformers.NewContextInjector()
	customInjector.ContextTemplate = `┌─────────────────────────────────────┐
│ SUB-AGENT DELEGATION                │
├─────────────────────────────────────┤
│ Source: {SourceAgent}               │
│ Target: {TargetAgent}               │
│ Task:   {Task}                      │
│ Limits: {Constraints}               │
└─────────────────────────────────────┘`

	reporterCtx := &prompt.RelayContext{
		SourceAgent: "StaticAnalyzer",
		TargetAgent: "ReportGenerator",
		Task:        "Generate security audit report",
		Constraints: []string{"Include remediation steps", "Severity ranking"},
		Prompts: []prompt.Prompt{
			{ID: "report", Position: prompt.PositionSystem, Content: "Generate formatted security report"},
		},
	}

	reporterPrompts, err := relay.Relay(reporterCtx, customInjector)
	if err != nil {
		fmt.Printf("Relay failed: %v\n", err)
		return
	}

	if len(reporterPrompts) > 0 {
		fmt.Println("Custom context prompt:")
		fmt.Println(reporterPrompts[0].Content)
	}

	fmt.Println("\n✓ Prompt relay demonstration complete!")
}

// ExampleCustomTransformer demonstrates how to create a custom transformer
func ExampleCustomTransformer() {
	// Custom transformer that adds metadata to all prompts
	type MetadataTransformer struct {
		Timestamp string
		Version   string
	}

	// Implement PromptTransformer interface
	metadataTransformer := struct {
		name      string
		timestamp string
		version   string
		transform func(*prompt.RelayContext, []prompt.Prompt) ([]prompt.Prompt, error)
	}{
		name:      "MetadataEnricher",
		timestamp: "2024-01-01T00:00:00Z",
		version:   "1.0.0",
		transform: func(ctx *prompt.RelayContext, prompts []prompt.Prompt) ([]prompt.Prompt, error) {
			result := make([]prompt.Prompt, len(prompts))

			for i, p := range prompts {
				// Create copy with metadata
				modified := p
				if modified.Metadata == nil {
					modified.Metadata = make(map[string]any)
				}
				modified.Metadata["relay_timestamp"] = "2024-01-01T00:00:00Z"
				modified.Metadata["relay_version"] = "1.0.0"
				modified.Metadata["delegation_path"] = ctx.SourceAgent + " -> " + ctx.TargetAgent

				result[i] = modified
			}

			return result, nil
		},
	}

	relay := prompt.NewPromptRelay()
	ctx := &prompt.RelayContext{
		SourceAgent: "Parent",
		TargetAgent: "Child",
		Task:        "Example task",
		Prompts: []prompt.Prompt{
			{ID: "test", Position: prompt.PositionSystem, Content: "Test prompt"},
		},
	}

	// Note: This is a demonstration. In practice, you'd implement PromptTransformer properly.
	fmt.Println("Custom transformer example:")
	fmt.Printf("Transformer name: %s\n", metadataTransformer.name)
	fmt.Printf("Version: %s\n", metadataTransformer.version)

	result, err := metadataTransformer.transform(ctx, ctx.Prompts)
	if err != nil {
		fmt.Printf("Transform failed: %v\n", err)
		return
	}

	if len(result) > 0 && result[0].Metadata != nil {
		fmt.Printf("Added metadata: %v\n", result[0].Metadata)
	}

	// Use with relay
	_ = relay // relay would be used with proper transformer implementation

	fmt.Println("✓ Custom transformer demonstration complete!")
}
