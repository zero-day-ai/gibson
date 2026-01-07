package verbose

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestVerboseIntegration_TextFormatter verifies the full verbose logging flow with text formatter.
func TestVerboseIntegration_TextFormatter(t *testing.T) {
	t.Parallel()

	// Create output buffer
	var buf bytes.Buffer

	// Create writer with text formatter at verbose level
	writer := NewVerboseWriter(&buf, LevelVerbose, false)
	ctx := context.Background()

	// Start processing events
	writer.Start(ctx)
	defer writer.Stop()

	// Get the bus to emit events
	bus := writer.Bus()

	t.Run("LLM events", func(t *testing.T) {
		// Emit LLM request started event
		event := NewVerboseEvent(EventLLMRequestStarted, LevelVerbose, &LLMRequestStartedData{
			Provider:     "anthropic",
			Model:        "claude-opus-4",
			SlotName:     "primary",
			MessageCount: 5,
			MaxTokens:    4096,
			Temperature:  0.7,
			Stream:       false,
		})
		err := bus.Emit(ctx, event)
		require.NoError(t, err)

		// Give time for event to be processed
		time.Sleep(50 * time.Millisecond)

		// Verify output contains expected elements
		output := buf.String()
		assert.Contains(t, output, "[LLM]")
		assert.Contains(t, output, "Request started")
		assert.Contains(t, output, "anthropic/claude-opus-4")
		assert.Contains(t, output, "slot=primary")
		assert.Contains(t, output, "messages=5")

		buf.Reset()
	})

	t.Run("Tool events", func(t *testing.T) {
		// Emit tool call started
		startEvent := NewVerboseEvent(EventToolCallStarted, LevelVerbose, &ToolCallStartedData{
			ToolName: "nmap",
			Parameters: map[string]any{
				"target": "192.168.1.1",
				"ports":  "1-1000",
			},
			ParameterSize: 128,
		})
		err := bus.Emit(ctx, startEvent)
		require.NoError(t, err)

		// Emit tool call completed
		completedEvent := NewVerboseEvent(EventToolCallCompleted, LevelVerbose, &ToolCallCompletedData{
			ToolName:   "nmap",
			Duration:   2 * time.Second,
			ResultSize: 4096,
			Success:    true,
		})
		err = bus.Emit(ctx, completedEvent)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		output := buf.String()
		assert.Contains(t, output, "[TOOL]")
		assert.Contains(t, output, "Tool call started: nmap")
		assert.Contains(t, output, "Tool call completed: nmap")
		assert.Contains(t, output, "success")
		assert.Contains(t, output, "duration=2s")

		buf.Reset()
	})

	t.Run("Agent events", func(t *testing.T) {
		missionID := types.NewID()

		// Emit agent started
		startEvent := NewVerboseEvent(EventAgentStarted, LevelVerbose, &AgentStartedData{
			AgentName:       "prompt-injector",
			TaskDescription: "Test for prompt injection vulnerabilities",
		}).WithMissionID(missionID).WithAgentName("prompt-injector")

		err := bus.Emit(ctx, startEvent)
		require.NoError(t, err)

		// Emit finding submitted
		findingEvent := NewVerboseEvent(EventFindingSubmitted, LevelVerbose, &FindingSubmittedData{
			FindingID:    types.NewID(),
			Title:        "Prompt injection detected",
			Severity:     "high",
			AgentName:    "prompt-injector",
			TechniqueIDs: []string{"T1059", "T1071"},
		}).WithMissionID(missionID).WithAgentName("prompt-injector")

		err = bus.Emit(ctx, findingEvent)
		require.NoError(t, err)

		// Emit agent completed
		completedEvent := NewVerboseEvent(EventAgentCompleted, LevelVerbose, &AgentCompletedData{
			AgentName:    "prompt-injector",
			Duration:     5 * time.Second,
			FindingCount: 1,
			Success:      true,
		}).WithMissionID(missionID).WithAgentName("prompt-injector")

		err = bus.Emit(ctx, completedEvent)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		output := buf.String()
		assert.Contains(t, output, "[AGENT]")
		assert.Contains(t, output, "Agent started: prompt-injector")
		assert.Contains(t, output, "Test for prompt injection")
		assert.Contains(t, output, "Finding submitted")
		assert.Contains(t, output, "[HIGH]")
		assert.Contains(t, output, "Agent completed: prompt-injector")
		assert.Contains(t, output, "findings=1")

		buf.Reset()
	})

	t.Run("Mission events", func(t *testing.T) {
		missionID := types.NewID()

		// Emit mission started
		startEvent := NewVerboseEvent(EventMissionStarted, LevelVerbose, &MissionStartedData{
			MissionID:    missionID,
			WorkflowName: "test-workflow",
			NodeCount:    10,
		}).WithMissionID(missionID)

		err := bus.Emit(ctx, startEvent)
		require.NoError(t, err)

		// Emit progress
		progressEvent := NewVerboseEvent(EventMissionProgress, LevelVerbose, &MissionProgressData{
			MissionID:      missionID,
			CompletedNodes: 5,
			TotalNodes:     10,
			CurrentNode:    "agent-node-3",
			Message:        "Processing target",
		}).WithMissionID(missionID)

		err = bus.Emit(ctx, progressEvent)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		output := buf.String()
		assert.Contains(t, output, "[MISSION]")
		assert.Contains(t, output, "Mission started")
		assert.Contains(t, output, "workflow=test-workflow")
		assert.Contains(t, output, "Mission progress: 50%")
		assert.Contains(t, output, "(5/10 nodes)")

		buf.Reset()
	})
}

// TestVerboseIntegration_JSONFormatter verifies JSON output mode.
func TestVerboseIntegration_JSONFormatter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	// Create writer with JSON formatter
	writer := NewVerboseWriter(&buf, LevelVerbose, true)
	ctx := context.Background()

	writer.Start(ctx)
	defer writer.Stop()

	bus := writer.Bus()

	// Emit an LLM event
	event := NewVerboseEvent(EventLLMRequestStarted, LevelVerbose, &LLMRequestStartedData{
		Provider:     "openai",
		Model:        "gpt-4",
		SlotName:     "primary",
		MessageCount: 3,
		Stream:       true,
	})

	err := bus.Emit(ctx, event)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Verify output is valid JSON
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Greater(t, len(lines), 0, "should have at least one JSON line")

	// Parse the JSON
	var parsed VerboseEvent
	err = json.Unmarshal([]byte(lines[0]), &parsed)
	require.NoError(t, err, "output should be valid JSON")

	// Verify structure
	assert.Equal(t, EventLLMRequestStarted, parsed.Type)
	assert.Equal(t, LevelVerbose, parsed.Level)
	assert.NotZero(t, parsed.Timestamp)

	// Verify payload (will be map[string]interface{} after JSON unmarshal)
	payload, ok := parsed.Payload.(map[string]interface{})
	require.True(t, ok, "payload should be a map")
	assert.Equal(t, "openai", payload["provider"])
	assert.Equal(t, "gpt-4", payload["model"])
	assert.Equal(t, "primary", payload["slot_name"])
	assert.Equal(t, float64(3), payload["message_count"])
	assert.Equal(t, true, payload["stream"])
}

// TestVerboseIntegration_LevelFiltering verifies that events are filtered by level.
func TestVerboseIntegration_LevelFiltering(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		writerLevel  VerboseLevel
		eventLevel   VerboseLevel
		shouldAppear bool
	}{
		{
			name:         "Verbose writer shows verbose events",
			writerLevel:  LevelVerbose,
			eventLevel:   LevelVerbose,
			shouldAppear: true,
		},
		{
			name:         "Verbose writer filters very-verbose events",
			writerLevel:  LevelVerbose,
			eventLevel:   LevelVeryVerbose,
			shouldAppear: false,
		},
		{
			name:         "Verbose writer filters debug events",
			writerLevel:  LevelVerbose,
			eventLevel:   LevelDebug,
			shouldAppear: false,
		},
		{
			name:         "VeryVerbose writer shows verbose events",
			writerLevel:  LevelVeryVerbose,
			eventLevel:   LevelVerbose,
			shouldAppear: true,
		},
		{
			name:         "VeryVerbose writer shows very-verbose events",
			writerLevel:  LevelVeryVerbose,
			eventLevel:   LevelVeryVerbose,
			shouldAppear: true,
		},
		{
			name:         "VeryVerbose writer filters debug events",
			writerLevel:  LevelVeryVerbose,
			eventLevel:   LevelDebug,
			shouldAppear: false,
		},
		{
			name:         "Debug writer shows all events",
			writerLevel:  LevelDebug,
			eventLevel:   LevelDebug,
			shouldAppear: true,
		},
		{
			name:         "Debug writer shows verbose events",
			writerLevel:  LevelDebug,
			eventLevel:   LevelVerbose,
			shouldAppear: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			writer := NewVerboseWriter(&buf, tc.writerLevel, false)
			ctx := context.Background()

			writer.Start(ctx)
			defer writer.Stop()

			bus := writer.Bus()

			// Emit event with specific level
			event := NewVerboseEvent(EventToolCallStarted, tc.eventLevel, &ToolCallStartedData{
				ToolName:      "test-tool",
				ParameterSize: 100,
			})

			err := bus.Emit(ctx, event)
			require.NoError(t, err)

			time.Sleep(50 * time.Millisecond)

			output := buf.String()

			if tc.shouldAppear {
				assert.Contains(t, output, "test-tool", "event should appear in output")
			} else {
				assert.NotContains(t, output, "test-tool", "event should be filtered out")
			}
		})
	}
}

// TestVerboseIntegration_MultipleSubscribers verifies multiple subscribers work correctly.
func TestVerboseIntegration_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	// Create a shared event bus
	bus := NewDefaultVerboseEventBus()
	defer bus.Close()

	ctx := context.Background()

	// Create two subscribers with different level filters
	eventCh1, cleanup1 := bus.Subscribe(ctx)
	eventCh2, cleanup2 := bus.Subscribe(ctx)
	defer cleanup1()
	defer cleanup2()

	// Channel to collect filtered output
	output1 := make(chan string, 100)
	output2 := make(chan string, 100)

	formatter1 := NewTextVerboseFormatter()
	formatter2 := NewTextVerboseFormatter()

	// Start goroutines to process events with level filtering
	go func() {
		for event := range eventCh1 {
			// Filter: only LevelVerbose
			if event.Level <= LevelVerbose {
				output1 <- formatter1.Format(event)
			}
		}
	}()

	go func() {
		for event := range eventCh2 {
			// Filter: up to LevelDebug
			if event.Level <= LevelDebug {
				output2 <- formatter2.Format(event)
			}
		}
	}()

	// Emit a verbose event
	verboseEvent := NewVerboseEvent(EventToolCallStarted, LevelVerbose, &ToolCallStartedData{
		ToolName:      "verbose-tool",
		ParameterSize: 100,
	})
	err := bus.Emit(ctx, verboseEvent)
	require.NoError(t, err)

	// Emit a debug event
	debugEvent := NewVerboseEvent(EventMemoryGet, LevelDebug, &MemoryGetData{
		Tier:  "working",
		Key:   "test-key",
		Found: true,
	})
	err = bus.Emit(ctx, debugEvent)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Collect output from channels
	close(output1)
	close(output2)

	var collected1, collected2 strings.Builder
	for s := range output1 {
		collected1.WriteString(s)
	}
	for s := range output2 {
		collected2.WriteString(s)
	}

	// Subscriber 1 (LevelVerbose) should only show verbose event
	out1 := collected1.String()
	assert.Contains(t, out1, "verbose-tool")
	assert.NotContains(t, out1, "test-key")

	// Subscriber 2 (LevelDebug) should show both events
	out2 := collected2.String()
	assert.Contains(t, out2, "verbose-tool")
	assert.Contains(t, out2, "test-key")
}

// TestVerboseIntegration_SystemEvents verifies system events are logged correctly.
func TestVerboseIntegration_SystemEvents(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := NewVerboseWriter(&buf, LevelVerbose, false)
	ctx := context.Background()

	writer.Start(ctx)
	defer writer.Stop()

	bus := writer.Bus()

	// Component registered
	event1 := NewVerboseEvent(EventComponentRegistered, LevelVerbose, &ComponentRegisteredData{
		ComponentType: "agent",
		ComponentName: "prompt-injector",
		Version:       "1.0.0",
		Capabilities:  []string{"llm-testing", "prompt-analysis"},
	})
	err := bus.Emit(ctx, event1)
	require.NoError(t, err)

	// Component health
	event2 := NewVerboseEvent(EventComponentHealth, LevelVerbose, &ComponentHealthData{
		ComponentType: "tool",
		ComponentName: "nmap",
		Healthy:       true,
		Status:        "ready",
		ResponseTime:  10 * time.Millisecond,
	})
	err = bus.Emit(ctx, event2)
	require.NoError(t, err)

	// Daemon started
	event3 := NewVerboseEvent(EventDaemonStarted, LevelVerbose, &DaemonStartedData{
		Version:       "0.1.0",
		ConfigPath:    "/home/user/.gibson/config.yaml",
		DataDir:       "/home/user/.gibson",
		ListenAddress: "localhost:9090",
	})
	err = bus.Emit(ctx, event3)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, "[SYSTEM]")
	assert.Contains(t, output, "Component registered: agent/prompt-injector")
	assert.Contains(t, output, "version=1.0.0")
	assert.Contains(t, output, "capabilities: llm-testing, prompt-analysis")
	assert.Contains(t, output, "Component health: tool/nmap - healthy")
	assert.Contains(t, output, "Daemon started: version=0.1.0")
	assert.Contains(t, output, "address=localhost:9090")
}

// TestVerboseIntegration_MemoryEvents verifies memory operation events.
func TestVerboseIntegration_MemoryEvents(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := NewVerboseWriter(&buf, LevelDebug, false) // Debug level to see memory events
	ctx := context.Background()

	writer.Start(ctx)
	defer writer.Stop()

	bus := writer.Bus()

	// Memory set
	event1 := NewVerboseEvent(EventMemorySet, LevelDebug, &MemorySetData{
		Tier:      "mission",
		Key:       "conversation_history",
		ValueSize: 2048,
	})
	err := bus.Emit(ctx, event1)
	require.NoError(t, err)

	// Memory get (hit)
	event2 := NewVerboseEvent(EventMemoryGet, LevelDebug, &MemoryGetData{
		Tier:  "mission",
		Key:   "conversation_history",
		Found: true,
	})
	err = bus.Emit(ctx, event2)
	require.NoError(t, err)

	// Memory search
	event3 := NewVerboseEvent(EventMemorySearch, LevelDebug, &MemorySearchData{
		Tier:        "mission",
		Query:       "vulnerabilities",
		ResultCount: 5,
		Duration:    15 * time.Millisecond,
	})
	err = bus.Emit(ctx, event3)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	assert.Contains(t, output, "[MEMORY]")
	assert.Contains(t, output, "Memory set: tier=mission")
	assert.Contains(t, output, "key=conversation_history")
	assert.Contains(t, output, "value_size=2048 bytes")
	assert.Contains(t, output, "Memory get")
	assert.Contains(t, output, "result=hit")
	assert.Contains(t, output, "Memory search")
	assert.Contains(t, output, "results=5")
}

// TestVerboseIntegration_ConcurrentEmissions verifies thread-safety with concurrent emitters.
func TestVerboseIntegration_ConcurrentEmissions(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := NewVerboseWriter(&buf, LevelVerbose, false)
	ctx := context.Background()

	writer.Start(ctx)
	defer writer.Stop()

	bus := writer.Bus()

	// Emit events concurrently from multiple goroutines
	const numGoroutines = 10
	const eventsPerGoroutine = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < eventsPerGoroutine; j++ {
				event := NewVerboseEvent(EventToolCallStarted, LevelVerbose, &ToolCallStartedData{
					ToolName:      "concurrent-tool",
					ParameterSize: id*100 + j,
				})
				_ = bus.Emit(ctx, event)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	time.Sleep(100 * time.Millisecond)

	// Verify no panics occurred and some events were written
	output := buf.String()
	assert.Contains(t, output, "concurrent-tool")
	// We don't assert exact count because slow subscriber handling may drop some events
}

// TestVerboseIntegration_GracefulShutdown verifies writer stops cleanly.
func TestVerboseIntegration_GracefulShutdown(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := NewVerboseWriter(&buf, LevelVerbose, false)
	ctx := context.Background()

	writer.Start(ctx)
	bus := writer.Bus()

	// Emit an event
	event := NewVerboseEvent(EventAgentStarted, LevelVerbose, &AgentStartedData{
		AgentName: "test-agent",
	})
	err := bus.Emit(ctx, event)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Stop the writer
	writer.Stop()

	// Verify event was processed before shutdown
	output := buf.String()
	assert.Contains(t, output, "test-agent")

	// Emitting after stop should fail or be ignored
	event2 := NewVerboseEvent(EventAgentCompleted, LevelVerbose, &AgentCompletedData{
		AgentName: "test-agent",
		Success:   true,
	})
	_ = bus.Emit(ctx, event2)

	// The event should not appear since bus is closed
	finalOutput := buf.String()
	assert.Equal(t, output, finalOutput, "no new events should be processed after stop")
}

// TestVerboseIntegration_ContextCancellation verifies context cancellation stops processing.
func TestVerboseIntegration_ContextCancellation(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := NewVerboseWriter(&buf, LevelVerbose, false)

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	writer.Start(ctx)
	defer writer.Stop()

	bus := writer.Bus()

	// Emit an event
	event := NewVerboseEvent(EventToolCallStarted, LevelVerbose, &ToolCallStartedData{
		ToolName: "test-tool",
	})
	err := bus.Emit(ctx, event)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	time.Sleep(50 * time.Millisecond)

	// Verify first event was processed
	output := buf.String()
	assert.Contains(t, output, "test-tool")
}
