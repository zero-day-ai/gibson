package tool_test

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// ExampleTool demonstrates implementing a custom tool with proto-based execution
type ExampleTool struct{}

func (t *ExampleTool) Name() string        { return "example-tool" }
func (t *ExampleTool) Description() string { return "An example tool for demonstration" }
func (t *ExampleTool) Version() string     { return "1.0.0" }
func (t *ExampleTool) Tags() []string      { return []string{"example", "demo"} }

func (t *ExampleTool) InputMessageType() string {
	return "google.protobuf.Struct"
}

func (t *ExampleTool) OutputMessageType() string {
	return "google.protobuf.Struct"
}

func (t *ExampleTool) ExecuteProto(ctx context.Context, input proto.Message) (proto.Message, error) {
	// Type assert to structpb.Struct
	structInput, ok := input.(*structpb.Struct)
	if !ok {
		return nil, fmt.Errorf("invalid input type: expected *structpb.Struct")
	}

	// Extract message field
	messageValue := structInput.Fields["message"]
	if messageValue == nil {
		return nil, fmt.Errorf("missing required field: message")
	}

	message := messageValue.GetStringValue()
	if message == "" {
		return nil, fmt.Errorf("invalid input: message must be a non-empty string")
	}

	result := fmt.Sprintf("Processed: %s", message)

	// Create output proto message
	output, err := structpb.NewStruct(map[string]any{"result": result})
	if err != nil {
		return nil, fmt.Errorf("failed to create output: %w", err)
	}

	return output, nil
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

	// Get the tool from registry
	t, err := registry.Get("example-tool")
	if err != nil {
		fmt.Printf("Failed to get tool: %v\n", err)
		return
	}

	// Execute the tool with proto input
	ctx := context.Background()
	input, _ := structpb.NewStruct(map[string]any{"message": "Hello, Gibson!"})
	outputProto, err := t.ExecuteProto(ctx, input)
	if err != nil {
		fmt.Printf("Execution failed: %v\n", err)
		return
	}

	// Extract result from proto output
	output := outputProto.(*structpb.Struct)
	result := output.Fields["result"].GetStringValue()
	fmt.Printf("Result: %s\n", result)

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
