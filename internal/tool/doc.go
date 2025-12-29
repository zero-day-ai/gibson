// Package tool provides the core tool abstraction for the Gibson Framework.
//
// Tools are atomic, stateless operations that serve as building blocks for agent
// capabilities. The package supports both internal (native Go) and external (gRPC)
// tool implementations, with built-in metrics tracking and health monitoring.
//
// # Core Concepts
//
// Tool: An interface representing an executable operation with well-defined schemas
// for input validation and output structure.
//
// ToolRegistry: A centralized registry managing tool lifecycle (registration,
// discovery, execution) with thread-safe operations and metrics collection.
//
// ToolMetrics: Execution statistics tracking for monitoring and observability,
// including success/failure rates and duration metrics.
//
// # Usage Example
//
//	// Create a registry
//	registry := tool.NewToolRegistry()
//
//	// Register an internal tool
//	myTool := &MyTool{} // implements Tool interface
//	if err := registry.RegisterInternal(myTool); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Execute the tool
//	ctx := context.Background()
//	input := map[string]any{"param": "value"}
//	output, err := registry.Execute(ctx, "my-tool", input)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Check metrics
//	metrics, _ := registry.Metrics("my-tool")
//	fmt.Printf("Success rate: %.2f%%\n", metrics.SuccessRate()*100)
//
// # Thread Safety
//
// All registry operations are thread-safe and can be called concurrently from
// multiple goroutines. Metrics are updated atomically during tool execution.
//
// # External Tools
//
// External tools are implemented via gRPC and can run as separate services.
// The GRPCToolClient provides transparent integration with the same Tool interface.
// Full gRPC implementation will be available after proto code generation.
package tool
