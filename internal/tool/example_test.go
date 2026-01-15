package tool_test

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/schema"
)

// ExampleTool demonstrates implementing a custom tool
type ExampleTool struct{}

func (t *ExampleTool) Name() string        { return "example-tool" }
func (t *ExampleTool) Description() string { return "An example tool for demonstration" }
func (t *ExampleTool) Version() string     { return "1.0.0" }
func (t *ExampleTool) Tags() []string      { return []string{"example", "demo"} }

func (t *ExampleTool) InputSchema() schema.JSON {
	minLen := 1
	return schema.Object(
		map[string]schema.JSON{
			"message": {Type: "string", Description: "Message to process", MinLength: &minLen},
		},
		"message",
	)
}

func (t *ExampleTool) OutputSchema() schema.JSON {
	return schema.Object(
		map[string]schema.JSON{
			"result": schema.StringWithDesc("Processed result"),
		},
		"result",
	)
}

func (t *ExampleTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	message, ok := input["message"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid input: message must be a string")
	}

	result := fmt.Sprintf("Processed: %s", message)
	return map[string]any{"result": result}, nil
}

func (t *ExampleTool) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("example tool is operational")
}

// Example demonstrates basic tool registry usage
func Example() {
	// Create a new registry
	registry := tool.NewToolRegistry()

	// Create and register a tool
	exampleTool := &ExampleTool{}
	if err := registry.RegisterInternal(exampleTool); err != nil {
		fmt.Printf("Failed to register tool: %v\n", err)
		return
	}

	// Execute the tool
	ctx := context.Background()
	input := map[string]any{"message": "Hello, Gibson!"}
	output, err := registry.Execute(ctx, "example-tool", input)
	if err != nil {
		fmt.Printf("Execution failed: %v\n", err)
		return
	}

	fmt.Printf("Result: %s\n", output["result"])

	// Check metrics
	metrics, _ := registry.Metrics("example-tool")
	fmt.Printf("Total calls: %d\n", metrics.TotalCalls)
	fmt.Printf("Success rate: %.0f%%\n", metrics.SuccessRate()*100)

	// Output:
	// Result: Processed: Hello, Gibson!
	// Total calls: 1
	// Success rate: 100%
}

// Example_listTools demonstrates tool discovery
func Example_listTools() {
	registry := tool.NewToolRegistry()

	// Register multiple tools
	_ = registry.RegisterInternal(&ExampleTool{})

	// List all tools
	tools := registry.List()
	fmt.Printf("Total tools: %d\n", len(tools))

	for _, desc := range tools {
		fmt.Printf("Tool: %s (v%s)\n", desc.Name, desc.Version)
	}

	// Output:
	// Total tools: 1
	// Tool: example-tool (v1.0.0)
}

// Example_healthCheck demonstrates health monitoring
func Example_healthCheck() {
	registry := tool.NewToolRegistry()
	_ = registry.RegisterInternal(&ExampleTool{})

	ctx := context.Background()

	// Check individual tool health
	health := registry.ToolHealth(ctx, "example-tool")
	fmt.Printf("Tool health: %s\n", health.State)

	// Check overall registry health
	overallHealth := registry.Health(ctx)
	fmt.Printf("Registry health: %s\n", overallHealth.State)

	// Output:
	// Tool health: healthy
	// Registry health: healthy
}

// Example_filterByTag demonstrates filtering tools by tags
func Example_filterByTag() {
	registry := tool.NewToolRegistry()
	_ = registry.RegisterInternal(&ExampleTool{})

	// Find all "example" tagged tools
	exampleTools := registry.ListByTag("example")
	fmt.Printf("Found %d example tools\n", len(exampleTools))

	for _, desc := range exampleTools {
		fmt.Printf("- %s: %s\n", desc.Name, desc.Description)
	}

	// Output:
	// Found 1 example tools
	// - example-tool: An example tool for demonstration
}
