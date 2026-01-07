package verbose

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

func TestTextVerboseFormatter(t *testing.T) {
	formatter := NewTextVerboseFormatter()

	tests := []struct {
		name     string
		event    VerboseEvent
		contains []string
	}{
		{
			name: "LLM request started",
			event: VerboseEvent{
				Type:      EventLLMRequestStarted,
				Level:     LevelVerbose,
				Timestamp: time.Now(),
				MissionID: types.NewID(),
				AgentName: "test-agent",
				Payload: &LLMRequestStartedData{
					Provider:     "anthropic",
					Model:        "claude-opus-4",
					SlotName:     "primary",
					MessageCount: 5,
					Stream:       false,
				},
			},
			contains: []string{"[LLM]", "Request started", "anthropic", "claude-opus-4", "primary"},
		},
		{
			name: "Tool call completed",
			event: VerboseEvent{
				Type:      EventToolCallCompleted,
				Level:     LevelVerbose,
				Timestamp: time.Now(),
				Payload: &ToolCallCompletedData{
					ToolName:   "nmap",
					Duration:   2 * time.Second,
					ResultSize: 1024,
					Success:    true,
				},
			},
			contains: []string{"[TOOL]", "Tool call completed", "nmap", "success"},
		},
		{
			name: "Agent started",
			event: VerboseEvent{
				Type:      EventAgentStarted,
				Level:     LevelVerbose,
				Timestamp: time.Now(),
				Payload: &AgentStartedData{
					AgentName:       "prompt-injection",
					TaskDescription: "Test target for prompt injection vulnerabilities",
				},
			},
			contains: []string{"[AGENT]", "Agent started", "prompt-injection"},
		},
		{
			name: "Mission completed",
			event: VerboseEvent{
				Type:      EventMissionCompleted,
				Level:     LevelVerbose,
				Timestamp: time.Now(),
				Payload: &MissionCompletedData{
					MissionID:     types.NewID(),
					Duration:      5 * time.Minute,
					FindingCount:  3,
					NodesExecuted: 10,
					Success:       true,
				},
			},
			contains: []string{"[MISSION]", "Mission completed", "findings=3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.Format(tt.event)

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("Expected output to contain %q, got:\n%s", substr, result)
				}
			}

			// Verify timestamp format
			if !strings.Contains(result, ":") {
				t.Error("Expected output to contain timestamp")
			}
		})
	}
}

func TestJSONVerboseFormatter(t *testing.T) {
	formatter := NewJSONVerboseFormatter()

	event := VerboseEvent{
		Type:      EventLLMRequestStarted,
		Level:     LevelVerbose,
		Timestamp: time.Now(),
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Payload: &LLMRequestStartedData{
			Provider:     "anthropic",
			Model:        "claude-opus-4",
			SlotName:     "primary",
			MessageCount: 5,
			Stream:       false,
		},
	}

	result := formatter.Format(event)

	// Verify it's valid JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v\nOutput: %s", err, result)
	}

	// Verify required fields
	if decoded["type"] != string(EventLLMRequestStarted) {
		t.Errorf("Expected type %q, got %q", EventLLMRequestStarted, decoded["type"])
	}

	if decoded["agent_name"] != "test-agent" {
		t.Errorf("Expected agent_name %q, got %q", "test-agent", decoded["agent_name"])
	}

	// Verify ends with newline
	if !strings.HasSuffix(result, "\n") {
		t.Error("Expected JSON output to end with newline")
	}
}

func TestVerboseWriter(t *testing.T) {
	// Create a buffer to capture output
	buf := &bytes.Buffer{}

	// Create writer with text formatter
	writer := NewVerboseWriter(buf, LevelVerbose, false)

	// Start the writer
	ctx := context.Background()
	writer.Start(ctx)
	defer writer.Stop()

	// Emit some events
	event1 := NewVerboseEvent(EventLLMRequestStarted, LevelVerbose, &LLMRequestStartedData{
		Provider:     "anthropic",
		Model:        "claude-opus-4",
		SlotName:     "primary",
		MessageCount: 5,
	})

	event2 := NewVerboseEvent(EventToolCallCompleted, LevelVeryVerbose, &ToolCallCompletedData{
		ToolName:   "nmap",
		Duration:   2 * time.Second,
		ResultSize: 1024,
		Success:    true,
	})

	// Emit events
	if err := writer.Bus().Emit(ctx, event1); err != nil {
		t.Fatalf("Failed to emit event1: %v", err)
	}

	if err := writer.Bus().Emit(ctx, event2); err != nil {
		t.Fatalf("Failed to emit event2: %v", err)
	}

	// Give the writer time to process
	time.Sleep(100 * time.Millisecond)

	output := buf.String()

	// Should see event1 (LevelVerbose)
	if !strings.Contains(output, "anthropic") {
		t.Errorf("Expected to see event1 in output:\n%s", output)
	}

	// Should NOT see event2 (LevelVeryVerbose is filtered out since writer level is LevelVerbose)
	if strings.Contains(output, "nmap") {
		t.Errorf("Should not see event2 (filtered by level) in output:\n%s", output)
	}
}

func TestVerboseWriterLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		writerLevel VerboseLevel
		eventLevel  VerboseLevel
		shouldShow  bool
	}{
		{"Verbose writer shows verbose events", LevelVerbose, LevelVerbose, true},
		{"Verbose writer filters very-verbose events", LevelVerbose, LevelVeryVerbose, false},
		{"Very-verbose writer shows verbose events", LevelVeryVerbose, LevelVerbose, true},
		{"Very-verbose writer shows very-verbose events", LevelVeryVerbose, LevelVeryVerbose, true},
		{"Debug writer shows all events", LevelDebug, LevelVerbose, true},
		{"Debug writer shows debug events", LevelDebug, LevelDebug, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			writer := NewVerboseWriter(buf, tt.writerLevel, false)

			ctx := context.Background()
			writer.Start(ctx)
			defer writer.Stop()

			event := NewVerboseEvent(EventLLMRequestStarted, tt.eventLevel, &LLMRequestStartedData{
				Provider: "test-provider",
				Model:    "test-model",
			})

			if err := writer.Bus().Emit(ctx, event); err != nil {
				t.Fatalf("Failed to emit event: %v", err)
			}

			time.Sleep(100 * time.Millisecond)

			output := buf.String()
			hasOutput := strings.Contains(output, "test-provider")

			if hasOutput != tt.shouldShow {
				t.Errorf("Expected shouldShow=%v, got hasOutput=%v\nOutput: %s",
					tt.shouldShow, hasOutput, output)
			}
		})
	}
}

func TestVerboseWriterJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewVerboseWriter(buf, LevelVerbose, true) // JSON output

	ctx := context.Background()
	writer.Start(ctx)
	defer writer.Stop()

	event := NewVerboseEvent(EventToolCallStarted, LevelVerbose, &ToolCallStartedData{
		ToolName:      "sqlmap",
		ParameterSize: 256,
	})

	if err := writer.Bus().Emit(ctx, event); err != nil {
		t.Fatalf("Failed to emit event: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	output := buf.String()

	// Verify it's valid JSON
	var decoded map[string]interface{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatal("No output from writer")
	}

	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v\nOutput: %s", err, output)
	}

	if decoded["type"] != string(EventToolCallStarted) {
		t.Errorf("Expected type %q, got %q", EventToolCallStarted, decoded["type"])
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly ten!", 12, "exactly ten!"},
		{"this is a very long string that should be truncated", 20, "this is a very lo..."},
		{"abc", 3, "abc"},
		{"abc", 2, "..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	// This test just ensures the function doesn't panic
	result := isTerminal()
	t.Logf("isTerminal() = %v", result)

	// When running in test mode, stdout is usually not a terminal
	// But we can't assert this since it depends on how tests are run
}

func TestTextFormatterColorDetection(t *testing.T) {
	// Save original environment
	originalNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if originalNoColor != "" {
			os.Setenv("NO_COLOR", originalNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	// Test with NO_COLOR set
	os.Setenv("NO_COLOR", "1")
	formatter := NewTextVerboseFormatter()
	if formatter.useColors {
		t.Error("Expected colors to be disabled when NO_COLOR is set")
	}

	// Test with NO_COLOR unset (colors depend on TTY detection)
	os.Unsetenv("NO_COLOR")
	formatter = NewTextVerboseFormatter()
	// Can't assert value since it depends on TTY, just verify it doesn't panic
	t.Logf("Colors enabled (NO_COLOR unset): %v", formatter.useColors)
}
