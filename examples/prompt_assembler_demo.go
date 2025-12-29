// +build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/zero-day-ai/gibson/internal/prompt"
)

func main() {
	// Create the prompt system components
	registry := prompt.NewPromptRegistry()
	renderer := prompt.NewTemplateRenderer()
	assembler := prompt.NewAssembler(renderer)

	// Register prompts demonstrating various features
	prompts := []prompt.Prompt{
		// System prefix - highest level instructions
		{
			ID:       "system-prefix",
			Position: prompt.PositionSystemPrefix,
			Content:  "You are Gibson, an autonomous AI agent framework.",
			Priority: 100,
		},

		// Main system prompt with template
		{
			ID:       "system-main",
			Position: prompt.PositionSystem,
			Content:  "Your primary objective is: {{.Mission.objective}}",
			Priority: 100,
		},

		// Context with conditional inclusion
		{
			ID:       "debug-context",
			Position: prompt.PositionContext,
			Content:  "DEBUG MODE: Verbose logging enabled. Agent version: {{.Agent.version}}",
			Priority: 100,
			Conditions: []prompt.Condition{
				{Field: "agent.debug", Operator: "eq", Value: true},
			},
		},

		// Production context (mutually exclusive with debug)
		{
			ID:       "prod-context",
			Position: prompt.PositionContext,
			Content:  "Production mode. Agent: {{.Agent.name}} v{{.Agent.version}}",
			Priority: 90,
			Conditions: []prompt.Condition{
				{Field: "agent.debug", Operator: "eq", Value: false},
			},
		},

		// Tools prompt
		{
			ID:       "available-tools",
			Position: prompt.PositionTools,
			Content:  "Available tools: web_search, file_operations, code_execution",
			Priority: 100,
		},

		// Plugins prompt
		{
			ID:       "active-plugins",
			Position: prompt.PositionPlugins,
			Content:  "Active plugins: security_scanner, vulnerability_detector",
			Priority: 100,
		},

		// Constraints
		{
			ID:       "safety-constraints",
			Position: prompt.PositionConstraints,
			Content:  "CONSTRAINTS:\n- Always verify actions before execution\n- Respect rate limits\n- Maintain operational security",
			Priority: 100,
		},

		// Examples
		{
			ID:       "examples",
			Position: prompt.PositionExamples,
			Content:  "EXAMPLE:\nUser: Scan target.com\nAssistant: I'll scan target.com for vulnerabilities...",
			Priority: 100,
		},

		// User prompts
		{
			ID:       "user-prefix",
			Position: prompt.PositionUserPrefix,
			Content:  "Current task:",
			Priority: 100,
		},
		{
			ID:       "user-main",
			Position: prompt.PositionUser,
			Content:  "{{.Mission.task}}",
			Priority: 100,
		},

		// Persona prompts
		{
			ID:       "persona:penetration-tester",
			Position: prompt.PositionSystem,
			Content:  "You specialize in penetration testing and security assessments.",
			Priority: 150,
		},
	}

	// Register all prompts
	for _, p := range prompts {
		if err := registry.Register(p); err != nil {
			fmt.Printf("Failed to register prompt %s: %v\n", p.ID, err)
			os.Exit(1)
		}
	}

	// Demo 1: Basic assembly with debug mode
	fmt.Println("=== DEMO 1: Debug Mode ===")
	demoDebugMode(assembler, registry)

	// Demo 2: Production mode
	fmt.Println("\n=== DEMO 2: Production Mode ===")
	demoProductionMode(assembler, registry)

	// Demo 3: With persona
	fmt.Println("\n=== DEMO 3: With Persona ===")
	demoWithPersona(assembler, registry)

	// Demo 4: Include/exclude optional positions
	fmt.Println("\n=== DEMO 4: Minimal Configuration ===")
	demoMinimalConfig(assembler, registry)

	// Demo 5: System override
	fmt.Println("\n=== DEMO 5: System Override ===")
	demoSystemOverride(assembler, registry)
}

func demoDebugMode(assembler prompt.Assembler, registry prompt.PromptRegistry) {
	ctx := prompt.NewRenderContext()
	ctx.Mission["objective"] = "security assessment"
	ctx.Mission["task"] = "Perform reconnaissance on target infrastructure"
	ctx.Agent["name"] = "Gibson"
	ctx.Agent["version"] = "1.0.0"
	ctx.Agent["debug"] = true

	result, err := assembler.Assemble(context.Background(), prompt.AssembleOptions{
		Registry:       registry,
		RenderContext:  ctx,
		IncludeTools:   true,
		IncludePlugins: true,
		IncludeAgents:  false,
	})

	if err != nil {
		fmt.Printf("Assembly failed: %v\n", err)
		return
	}

	printResult(result)
}

func demoProductionMode(assembler prompt.Assembler, registry prompt.PromptRegistry) {
	ctx := prompt.NewRenderContext()
	ctx.Mission["objective"] = "security assessment"
	ctx.Mission["task"] = "Perform reconnaissance on target infrastructure"
	ctx.Agent["name"] = "Gibson"
	ctx.Agent["version"] = "1.0.0"
	ctx.Agent["debug"] = false

	result, err := assembler.Assemble(context.Background(), prompt.AssembleOptions{
		Registry:       registry,
		RenderContext:  ctx,
		IncludeTools:   true,
		IncludePlugins: true,
	})

	if err != nil {
		fmt.Printf("Assembly failed: %v\n", err)
		return
	}

	printResult(result)
}

func demoWithPersona(assembler prompt.Assembler, registry prompt.PromptRegistry) {
	ctx := prompt.NewRenderContext()
	ctx.Mission["objective"] = "security assessment"
	ctx.Mission["task"] = "Perform penetration testing"
	ctx.Agent["name"] = "Gibson"
	ctx.Agent["version"] = "1.0.0"
	ctx.Agent["debug"] = false

	result, err := assembler.Assemble(context.Background(), prompt.AssembleOptions{
		Registry:       registry,
		RenderContext:  ctx,
		Persona:        "penetration-tester",
		IncludeTools:   true,
		IncludePlugins: true,
	})

	if err != nil {
		fmt.Printf("Assembly failed: %v\n", err)
		return
	}

	printResult(result)
}

func demoMinimalConfig(assembler prompt.Assembler, registry prompt.PromptRegistry) {
	ctx := prompt.NewRenderContext()
	ctx.Mission["objective"] = "basic task"
	ctx.Mission["task"] = "Simple operation"
	ctx.Agent["name"] = "Gibson"
	ctx.Agent["version"] = "1.0.0"
	ctx.Agent["debug"] = false

	result, err := assembler.Assemble(context.Background(), prompt.AssembleOptions{
		Registry:       registry,
		RenderContext:  ctx,
		IncludeTools:   false,
		IncludePlugins: false,
		IncludeAgents:  false,
	})

	if err != nil {
		fmt.Printf("Assembly failed: %v\n", err)
		return
	}

	printResult(result)
}

func demoSystemOverride(assembler prompt.Assembler, registry prompt.PromptRegistry) {
	ctx := prompt.NewRenderContext()
	ctx.Mission["task"] = "Test system override"

	result, err := assembler.Assemble(context.Background(), prompt.AssembleOptions{
		Registry:       registry,
		RenderContext:  ctx,
		SystemOverride: "CUSTOM SYSTEM PROMPT: All standard prompts are replaced by this override.",
	})

	if err != nil {
		fmt.Printf("Assembly failed: %v\n", err)
		return
	}

	printResult(result)
}

func printResult(result *prompt.AssembleResult) {
	fmt.Println("System Message:")
	fmt.Println("---")
	fmt.Println(result.System)
	fmt.Println("---")

	fmt.Println("\nUser Message:")
	fmt.Println("---")
	fmt.Println(result.User)
	fmt.Println("---")

	fmt.Printf("\nTotal prompts used: %d\n", len(result.Prompts))

	fmt.Println("\nMessages (JSON):")
	jsonBytes, _ := json.MarshalIndent(result.Messages, "", "  ")
	fmt.Println(string(jsonBytes))
}
