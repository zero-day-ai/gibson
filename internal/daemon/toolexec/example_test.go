package toolexec_test

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/zero-day-ai/gibson/internal/daemon/toolexec"
)

// ExampleToolExecutorService demonstrates basic usage of the tool executor service.
func ExampleToolExecutorService() {
	// Create a logger
	logger := slog.Default()

	// Create the service with default tools directory and 5-minute timeout
	service := toolexec.NewToolExecutorService(
		"~/.gibson/tools/bin",
		5*time.Minute,
		logger,
	)

	// Start the service (scans for tools)
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		log.Fatalf("failed to start service: %v", err)
	}
	defer service.Stop(ctx)

	// List all discovered tools
	tools := service.ListTools()
	fmt.Printf("Discovered %d tools:\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("  - %s (status: %s)\n", tool.Name, tool.Status)
	}

	// Get schema for a specific tool
	schema, err := service.GetToolSchema("example-tool")
	if err != nil {
		fmt.Printf("Tool not found: %v\n", err)
	} else {
		fmt.Printf("Schema: input=%s, output=%s\n",
			schema.InputSchema.Type,
			schema.OutputSchema.Type)
	}

	// Execute a tool with custom input
	input := map[string]any{
		"message": "Hello, world!",
	}
	output, err := service.Execute(ctx, "example-tool", input, 30*time.Second)
	if err != nil {
		fmt.Printf("Execution failed: %v\n", err)
	} else {
		fmt.Printf("Tool output: %v\n", output)
	}

	// Hot-reload tools (rescans directory)
	if err := service.RefreshTools(ctx); err != nil {
		log.Fatalf("failed to refresh tools: %v", err)
	}
	fmt.Println("Tools refreshed successfully")
}

// ExampleToolExecutorService_metrics demonstrates accessing tool execution metrics.
func ExampleToolExecutorService_metrics() {
	logger := slog.Default()
	service := toolexec.NewToolExecutorService("~/.gibson/tools/bin", 5*time.Minute, logger)

	ctx := context.Background()
	service.Start(ctx)
	defer service.Stop(ctx)

	// Execute a tool multiple times
	for i := 0; i < 3; i++ {
		input := map[string]any{"iteration": i}
		_, _ = service.Execute(ctx, "example-tool", input, 30*time.Second)
	}

	// Check metrics for all tools
	tools := service.ListTools()
	for _, tool := range tools {
		if tool.Metrics != nil {
			fmt.Printf("Tool: %s\n", tool.Name)
			fmt.Printf("  Total executions: %d\n", tool.Metrics.TotalExecutions)
			fmt.Printf("  Successful: %d\n", tool.Metrics.SuccessfulExecutions)
			fmt.Printf("  Failed: %d\n", tool.Metrics.FailedExecutions)
			fmt.Printf("  Average duration: %v\n", tool.Metrics.AverageDuration)
			if tool.Metrics.LastExecutedAt != nil {
				fmt.Printf("  Last executed: %v\n", tool.Metrics.LastExecutedAt)
			}
		}
	}
}
