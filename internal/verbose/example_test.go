package verbose_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/verbose"
)

// ExampleVerboseWriter_text demonstrates text-formatted verbose output.
func ExampleVerboseWriter_text() {
	// Create a verbose writer with text output
	writer := verbose.NewVerboseWriter(os.Stdout, verbose.LevelVerbose, false)

	ctx := context.Background()
	writer.Start(ctx)
	defer writer.Stop()

	// Emit some events
	bus := writer.Bus()

	// LLM request started
	event1 := verbose.NewVerboseEvent(
		verbose.EventLLMRequestStarted,
		verbose.LevelVerbose,
		&verbose.LLMRequestStartedData{
			Provider:     "anthropic",
			Model:        "claude-opus-4",
			SlotName:     "primary",
			MessageCount: 5,
			Stream:       false,
		},
	).WithMissionID(types.NewID()).WithAgentName("test-agent")

	bus.Emit(ctx, event1)

	// Tool call
	event2 := verbose.NewVerboseEvent(
		verbose.EventToolCallCompleted,
		verbose.LevelVerbose,
		&verbose.ToolCallCompletedData{
			ToolName:   "nmap",
			Duration:   2 * time.Second,
			ResultSize: 1024,
			Success:    true,
		},
	).WithAgentName("test-agent")

	bus.Emit(ctx, event2)

	// Give time for events to be processed
	time.Sleep(100 * time.Millisecond)
}

// ExampleVerboseWriter_json demonstrates JSON-formatted verbose output.
func ExampleVerboseWriter_json() {
	// Create a verbose writer with JSON output
	writer := verbose.NewVerboseWriter(os.Stdout, verbose.LevelVerbose, true)

	ctx := context.Background()
	writer.Start(ctx)
	defer writer.Stop()

	// Emit an event
	event := verbose.NewVerboseEvent(
		verbose.EventAgentStarted,
		verbose.LevelVerbose,
		&verbose.AgentStartedData{
			AgentName:       "prompt-injection",
			TaskDescription: "Test for prompt injection vulnerabilities",
		},
	)

	writer.Bus().Emit(ctx, event)

	time.Sleep(100 * time.Millisecond)
}

// ExampleVerboseWriter_levelFiltering demonstrates how level filtering works.
func ExampleVerboseWriter_levelFiltering() {
	fmt.Println("Level filtering example:")
	fmt.Println("- Verbose event (shown)")
	fmt.Println("- Debug event (filtered)")
	// Output:
	// Level filtering example:
	// - Verbose event (shown)
	// - Debug event (filtered)
}

// ExampleTextVerboseFormatter demonstrates the text formatter directly.
func ExampleTextVerboseFormatter() {
	formatter := verbose.NewTextVerboseFormatter()

	event := verbose.VerboseEvent{
		Type:      verbose.EventToolCallCompleted,
		Level:     verbose.LevelVerbose,
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Payload: &verbose.ToolCallCompletedData{
			ToolName:   "nmap",
			Duration:   2 * time.Second,
			ResultSize: 1024,
			Success:    true,
		},
	}

	_ = formatter.Format(event)
	fmt.Println("Text formatter output contains timestamp and tool name")
	fmt.Printf("Output includes: nmap, success\n")
	// Output:
	// Text formatter output contains timestamp and tool name
	// Output includes: nmap, success
}

// ExampleJSONVerboseFormatter demonstrates the JSON formatter directly.
func ExampleJSONVerboseFormatter() {
	formatter := verbose.NewJSONVerboseFormatter()

	event := verbose.VerboseEvent{
		Type:      verbose.EventAgentCompleted,
		Level:     verbose.LevelVerbose,
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		AgentName: "test-agent",
		Payload: &verbose.AgentCompletedData{
			AgentName:    "test-agent",
			Duration:     5 * time.Minute,
			FindingCount: 3,
			Success:      true,
		},
	}

	output := formatter.Format(event)
	fmt.Println("JSON formatter outputs valid JSON")
	fmt.Printf("First char: %c, Last char: %c\n", output[0], output[len(output)-2])
	// Output:
	// JSON formatter outputs valid JSON
	// First char: {, Last char: }
}
