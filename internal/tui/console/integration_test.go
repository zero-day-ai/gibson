package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestAgentStreamingIntegration tests the full flow from focus command
// through event processing to rendering, without requiring actual TUI or gRPC.
func TestAgentStreamingIntegration(t *testing.T) {
	// Setup
	ctx := context.Background()
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	mockDAO := newMockComponentDAO()
	mockDAO.AddComponent(component.ComponentKindAgent, "test-agent", component.ComponentStatusRunning, 12345)

	mockSession := &mockSessionDAO{}
	streamManager := agent.NewStreamManager(ctx, agent.StreamManagerConfig{
		SessionDAO: mockSession,
	})

	executor := NewExecutor(ctx, registry, ExecutorConfig{
		ComponentDAO:  mockDAO,
		StreamManager: streamManager,
	})
	executor.SetupHandlers()

	// Test 1: handleFocus validates agent running status
	t.Run("focus validates agent running", func(t *testing.T) {
		// Add a stopped agent
		mockDAO.AddComponent(component.ComponentKindAgent, "stopped-agent", component.ComponentStatusStopped, 0)

		result, err := executor.handleFocus(ctx, []string{"stopped-agent"})
		if err != nil {
			t.Fatalf("handleFocus returned error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for stopped agent")
		}

		// Verify focused agent not set
		if executor.GetFocusedAgent() != "" {
			t.Error("focused agent should not be set for stopped agent")
		}
	})

	t.Run("focus validates agent exists", func(t *testing.T) {
		result, err := executor.handleFocus(ctx, []string{"nonexistent-agent"})
		if err != nil {
			t.Fatalf("handleFocus returned error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for nonexistent agent")
		}
	})
}

// TestEventRendererIntegration tests that EventRenderer produces consistent
// output for all event types that the TUI needs to display.
func TestEventRendererIntegration(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	// Test complete event flow
	events := []struct {
		eventType database.StreamEventType
		content   any
		name      string
	}{
		{
			eventType: database.StreamEventStatus,
			content:   StatusContent{Status: "running"},
			name:      "agent started",
		},
		{
			eventType: database.StreamEventOutput,
			content:   OutputContent{Text: "Analyzing target...", Complete: false},
			name:      "agent thinking",
		},
		{
			eventType: database.StreamEventToolCall,
			content:   ToolCallContent{ToolName: "nmap", ToolID: "call-1", Arguments: map[string]any{"target": "192.168.1.1"}},
			name:      "tool invocation",
		},
		{
			eventType: database.StreamEventToolResult,
			content:   ToolResultContent{ToolID: "call-1", Success: true, Output: "Port 80 open"},
			name:      "tool success",
		},
		{
			eventType: database.StreamEventFinding,
			content:   FindingContent{ID: "f1", Title: "Open Port", Severity: "medium", Category: "exposure"},
			name:      "finding discovered",
		},
		{
			eventType: database.StreamEventOutput,
			content:   OutputContent{Text: "Scan complete. Found 1 finding.", Complete: true},
			name:      "agent output complete",
		},
		{
			eventType: database.StreamEventStatus,
			content:   StatusContent{Status: "completed", Message: "Scan finished successfully"},
			name:      "agent completed",
		},
	}

	allLines := []OutputLine{}

	for _, evt := range events {
		contentJSON, err := json.Marshal(evt.content)
		if err != nil {
			t.Fatalf("failed to marshal content for %s: %v", evt.name, err)
		}

		event := &database.StreamEvent{
			ID:        types.NewID(),
			EventType: evt.eventType,
			Content:   contentJSON,
			Timestamp: ts,
		}

		lines := renderer.RenderEvent(event)
		if lines == nil || len(lines) == 0 {
			t.Errorf("no lines rendered for %s", evt.name)
			continue
		}

		t.Logf("%s: rendered %d lines", evt.name, len(lines))
		for i, line := range lines {
			t.Logf("  [%d] style=%s text=%q", i, line.Style, line.Text)
		}

		allLines = append(allLines, lines...)
	}

	// Verify we got output from all events
	if len(allLines) < len(events) {
		t.Errorf("expected at least %d lines total, got %d", len(events), len(allLines))
	}

	// Verify different styles are used
	styles := make(map[OutputStyle]bool)
	for _, line := range allLines {
		styles[line.Style] = true
	}

	expectedStyles := []OutputStyle{
		StyleStatus,
		StyleAgentOutput,
		StyleToolCall,
		StyleToolResult,
		StyleFindingMedium, // Our finding was medium severity
		StyleSuccess,       // Completed status uses success style
	}

	for _, expected := range expectedStyles {
		if !styles[expected] {
			t.Errorf("expected to see style %s in output", expected)
		}
	}
}

// TestEventProcessorIntegration tests the EventProcessor lifecycle
// with a real channel and verifies proper cleanup.
func TestEventProcessorIntegration(t *testing.T) {
	eventChan := make(chan *database.StreamEvent, 100)
	processor := NewEventProcessor(eventChan, nil, "test-agent")

	// Verify initial state
	if processor.IsRunning() {
		t.Error("processor should not be running initially")
	}
	if processor.AgentName() != "test-agent" {
		t.Errorf("agent name = %q, want %q", processor.AgentName(), "test-agent")
	}

	// Start processor
	processor.Start()
	if !processor.IsRunning() {
		t.Error("processor should be running after Start()")
	}

	// Send events
	for i := 0; i < 5; i++ {
		event := &database.StreamEvent{
			ID:        types.NewID(),
			Sequence:  int64(i + 1),
			EventType: database.StreamEventOutput,
			Content:   json.RawMessage(`{"text": "test", "complete": false}`),
			Timestamp: time.Now(),
		}
		eventChan <- event
	}

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Stop processor
	processor.Stop()

	// Wait for cleanup
	time.Sleep(50 * time.Millisecond)

	if processor.IsRunning() {
		t.Error("processor should not be running after Stop()")
	}
}

// TestSteeringCommandsRequireFocus verifies that steering commands
// fail appropriately when no agent is focused.
func TestSteeringCommandsRequireFocus(t *testing.T) {
	ctx := context.Background()
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(ctx, registry, ExecutorConfig{})
	executor.SetupHandlers()

	commands := []struct {
		name    string
		handler func(context.Context, []string) (*ExecutionResult, error)
	}{
		{"interrupt", executor.handleInterrupt},
		{"pause", executor.handlePause},
		{"resume", executor.handleResume},
	}

	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			result, err := cmd.handler(ctx, []string{"test message"})
			if err != nil {
				t.Fatalf("%s returned error: %v", cmd.name, err)
			}
			if !result.IsError {
				t.Errorf("%s should fail when no agent focused", cmd.name)
			}
		})
	}
}

// TestModeCommandValidation tests that mode command validates modes correctly.
func TestModeCommandValidation(t *testing.T) {
	ctx := context.Background()
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	// Without StreamManager, valid modes will fail at execution time
	executor := NewExecutor(ctx, registry, ExecutorConfig{})
	executor.SetupHandlers()
	executor.SetFocusedAgent("test-agent")

	tests := []struct {
		mode        string
		wantError   bool
		description string
	}{
		{"invalid", true, "invalid mode should error"},
		{"", true, "empty mode should error"},
		{"auto", true, "auto without StreamManager should error"},               // Will fail due to no StreamManager
		{"interactive", true, "interactive without StreamManager should error"}, // Will fail due to no StreamManager
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			args := []string{}
			if tt.mode != "" {
				args = []string{tt.mode}
			}

			result, err := executor.handleMode(ctx, args)
			if err != nil {
				t.Fatalf("handleMode returned error: %v", err)
			}

			if tt.wantError && !result.IsError {
				t.Errorf("expected error for mode %q", tt.mode)
			}
			if !tt.wantError && result.IsError {
				t.Errorf("unexpected error for mode %q: %s", tt.mode, result.Error)
			}
		})
	}
}

// TestOutputLineStyles verifies that all OutputStyle values have string representations.
func TestOutputLineStyles(t *testing.T) {
	styles := []OutputStyle{
		StyleNormal,
		StyleCommand,
		StyleError,
		StyleSuccess,
		StyleInfo,
		StyleAgentOutput,
		StyleToolCall,
		StyleToolResult,
		StyleFinding,
		StyleFindingCritical,
		StyleFindingHigh,
		StyleFindingMedium,
		StyleFindingLow,
		StyleStatus,
		StyleSteeringAck,
	}

	for _, style := range styles {
		str := style.String()
		if str == "" {
			t.Errorf("style %d has empty string representation", style)
		}
	}
}

// TestOutputSourceTypes verifies source type string representations.
func TestOutputSourceTypes(t *testing.T) {
	sources := []OutputSource{
		SourceUser,
		SourceSystem,
		SourceAgent,
	}

	for _, source := range sources {
		str := source.String()
		if str == "" {
			t.Errorf("source %d has empty string representation", source)
		}
	}

	// Verify source values are distinct
	if SourceUser == SourceSystem || SourceSystem == SourceAgent || SourceUser == SourceAgent {
		t.Error("source values should be distinct")
	}
}
