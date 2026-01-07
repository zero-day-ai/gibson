package console

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNewEventRenderer(t *testing.T) {
	renderer := NewEventRenderer("test-agent")
	if renderer == nil {
		t.Fatal("expected non-nil renderer")
	}
	if renderer.agentName != "test-agent" {
		t.Errorf("agentName = %q, want %q", renderer.agentName, "test-agent")
	}
}

func TestEventRenderer_SetAgentName(t *testing.T) {
	renderer := NewEventRenderer("initial")
	renderer.SetAgentName("updated")
	if renderer.agentName != "updated" {
		t.Errorf("agentName = %q, want %q", renderer.agentName, "updated")
	}
}

func TestEventRenderer_RenderEvent_Nil(t *testing.T) {
	renderer := NewEventRenderer("test")
	lines := renderer.RenderEvent(nil)
	if lines != nil {
		t.Errorf("expected nil for nil event, got %v", lines)
	}
}

func TestEventRenderer_RenderOutput(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name      string
		content   OutputContent
		wantCount int
		wantText  string
		wantStyle OutputStyle
	}{
		{
			name:      "simple text",
			content:   OutputContent{Text: "Hello world", Complete: true},
			wantCount: 1,
			wantText:  "Hello world",
			wantStyle: StyleAgentOutput,
		},
		{
			name:      "multiline text",
			content:   OutputContent{Text: "Line 1\nLine 2\nLine 3", Complete: true},
			wantCount: 3,
			wantText:  "Line 1",
			wantStyle: StyleAgentOutput,
		},
		{
			name:      "empty text",
			content:   OutputContent{Text: "", Complete: true},
			wantCount: 0,
		},
		{
			name:      "incomplete text with trailing newline",
			content:   OutputContent{Text: "thinking...\n", Complete: false},
			wantCount: 1,
			wantText:  "thinking...",
			wantStyle: StyleAgentOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderOutput(contentJSON, ts)

			if len(lines) != tt.wantCount {
				t.Errorf("got %d lines, want %d", len(lines), tt.wantCount)
				return
			}

			if tt.wantCount > 0 {
				if lines[0].Text != tt.wantText {
					t.Errorf("first line text = %q, want %q", lines[0].Text, tt.wantText)
				}
				if lines[0].Style != tt.wantStyle {
					t.Errorf("style = %v, want %v", lines[0].Style, tt.wantStyle)
				}
				if lines[0].AgentName != "davinci" {
					t.Errorf("agent name = %q, want %q", lines[0].AgentName, "davinci")
				}
			}
		})
	}
}

func TestEventRenderer_RenderToolCall(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name         string
		content      ToolCallContent
		wantMinLines int
		wantHeader   string
	}{
		{
			name: "simple tool call",
			content: ToolCallContent{
				ToolName: "nmap",
				ToolID:   "tool-123",
			},
			wantMinLines: 1,
			wantHeader:   ">>> Calling tool: nmap",
		},
		{
			name: "tool call with arguments",
			content: ToolCallContent{
				ToolName:  "http_request",
				ToolID:    "tool-456",
				Arguments: map[string]any{"url": "https://example.com", "method": "GET"},
			},
			wantMinLines: 2, // Header + at least one arg line
			wantHeader:   ">>> Calling tool: http_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderToolCall(contentJSON, ts)

			if len(lines) < tt.wantMinLines {
				t.Errorf("got %d lines, want at least %d", len(lines), tt.wantMinLines)
				return
			}

			if lines[0].Text != tt.wantHeader {
				t.Errorf("header = %q, want %q", lines[0].Text, tt.wantHeader)
			}
			if lines[0].Style != StyleToolCall {
				t.Errorf("style = %v, want %v", lines[0].Style, StyleToolCall)
			}
		})
	}
}

func TestEventRenderer_RenderToolResult(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name         string
		content      ToolResultContent
		wantHeader   string
		wantStyle    OutputStyle
		wantMinLines int
	}{
		{
			name: "success result",
			content: ToolResultContent{
				ToolID:  "tool-123",
				Success: true,
				Output:  "Scan complete: 3 ports open",
			},
			wantHeader:   "<<< Tool result: success",
			wantStyle:    StyleToolResult,
			wantMinLines: 2,
		},
		{
			name: "success no output",
			content: ToolResultContent{
				ToolID:  "tool-123",
				Success: true,
			},
			wantHeader:   "<<< Tool result: success",
			wantStyle:    StyleToolResult,
			wantMinLines: 1,
		},
		{
			name: "failure result",
			content: ToolResultContent{
				ToolID:  "tool-123",
				Success: false,
				Error:   "Connection refused",
			},
			wantHeader:   "<<< Tool result: failed",
			wantStyle:    StyleError,
			wantMinLines: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderToolResult(contentJSON, ts)

			if len(lines) < tt.wantMinLines {
				t.Errorf("got %d lines, want at least %d", len(lines), tt.wantMinLines)
				return
			}

			if lines[0].Text != tt.wantHeader {
				t.Errorf("header = %q, want %q", lines[0].Text, tt.wantHeader)
			}
			if lines[0].Style != tt.wantStyle {
				t.Errorf("style = %v, want %v", lines[0].Style, tt.wantStyle)
			}
		})
	}
}

func TestEventRenderer_RenderFinding(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name      string
		content   FindingContent
		wantStyle OutputStyle
	}{
		{
			name: "critical finding",
			content: FindingContent{
				ID:          "finding-001",
				Title:       "SQL Injection Vulnerability",
				Severity:    "critical",
				Category:    "injection",
				Description: "Found SQL injection in login form",
			},
			wantStyle: StyleFindingCritical,
		},
		{
			name: "high finding",
			content: FindingContent{
				ID:       "finding-002",
				Title:    "XSS Vulnerability",
				Severity: "high",
			},
			wantStyle: StyleFindingHigh,
		},
		{
			name: "medium finding",
			content: FindingContent{
				ID:       "finding-003",
				Title:    "Missing CSRF Token",
				Severity: "medium",
			},
			wantStyle: StyleFindingMedium,
		},
		{
			name: "low finding",
			content: FindingContent{
				ID:       "finding-004",
				Title:    "Information Disclosure",
				Severity: "low",
			},
			wantStyle: StyleFindingLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderFinding(contentJSON, ts)

			if len(lines) < 2 {
				t.Errorf("got %d lines, want at least 2", len(lines))
				return
			}

			// Check header line style
			if lines[0].Style != tt.wantStyle {
				t.Errorf("header style = %v, want %v", lines[0].Style, tt.wantStyle)
			}

			// Check that finding ID hint is present
			var hasHint bool
			for _, line := range lines {
				if line.Style == StyleInfo && line.Source == SourceSystem {
					hasHint = true
					break
				}
			}
			if tt.content.ID != "" && !hasHint {
				t.Error("expected hint line for finding with ID")
			}
		})
	}
}

func TestEventRenderer_RenderStatus(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name         string
		content      StatusContent
		wantContains string
		wantStyle    OutputStyle
	}{
		{
			name:         "running status",
			content:      StatusContent{Status: "running"},
			wantContains: "started",
			wantStyle:    StyleStatus,
		},
		{
			name:         "paused status",
			content:      StatusContent{Status: "paused"},
			wantContains: "paused",
			wantStyle:    StyleStatus,
		},
		{
			name:         "completed status",
			content:      StatusContent{Status: "completed"},
			wantContains: "completed successfully",
			wantStyle:    StyleSuccess,
		},
		{
			name:         "failed status",
			content:      StatusContent{Status: "failed", Reason: "Out of memory"},
			wantContains: "failed",
			wantStyle:    StyleError,
		},
		{
			name:         "waiting for input",
			content:      StatusContent{Status: "waiting_for_input"},
			wantContains: "waiting for input",
			wantStyle:    StyleInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderStatus(contentJSON, ts)

			if len(lines) < 1 {
				t.Errorf("got %d lines, want at least 1", len(lines))
				return
			}

			if lines[0].Style != tt.wantStyle {
				t.Errorf("style = %v, want %v", lines[0].Style, tt.wantStyle)
			}

			found := false
			for _, line := range lines {
				if containsIgnoreCase(line.Text, tt.wantContains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected line containing %q", tt.wantContains)
			}
		})
	}
}

func TestEventRenderer_RenderSteeringAck(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name         string
		content      SteeringAckContent
		wantContains string
		wantStyle    OutputStyle
	}{
		{
			name:         "accepted",
			content:      SteeringAckContent{Sequence: 1, Accepted: true},
			wantContains: "accepted",
			wantStyle:    StyleSteeringAck,
		},
		{
			name:         "rejected",
			content:      SteeringAckContent{Sequence: 2, Accepted: false, Message: "Invalid mode"},
			wantContains: "rejected",
			wantStyle:    StyleError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderSteeringAck(contentJSON, ts)

			if len(lines) < 1 {
				t.Errorf("got %d lines, want at least 1", len(lines))
				return
			}

			if lines[0].Style != tt.wantStyle {
				t.Errorf("style = %v, want %v", lines[0].Style, tt.wantStyle)
			}

			found := false
			for _, line := range lines {
				if containsIgnoreCase(line.Text, tt.wantContains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected line containing %q", tt.wantContains)
			}
		})
	}
}

func TestEventRenderer_RenderError(t *testing.T) {
	renderer := NewEventRenderer("davinci")
	ts := time.Now()

	tests := []struct {
		name    string
		content ErrorContent
	}{
		{
			name:    "error with message only",
			content: ErrorContent{Message: "Connection timeout"},
		},
		{
			name:    "error with code",
			content: ErrorContent{Message: "Rate limited", Code: "RATE_LIMIT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}
			lines := renderer.RenderError(contentJSON, ts)

			if len(lines) < 1 {
				t.Errorf("got %d lines, want at least 1", len(lines))
				return
			}

			if lines[0].Style != StyleError {
				t.Errorf("style = %v, want %v", lines[0].Style, StyleError)
			}

			if !containsIgnoreCase(lines[0].Text, tt.content.Message) {
				t.Errorf("expected line containing message %q", tt.content.Message)
			}

			if tt.content.Code != "" && len(lines) < 2 {
				t.Error("expected code line")
			}
		})
	}
}

func TestEventRenderer_RenderEvent_AllTypes(t *testing.T) {
	renderer := NewEventRenderer("test-agent")
	ts := time.Now()

	tests := []struct {
		eventType   database.StreamEventType
		content     any
		wantNonNil  bool
		description string
	}{
		{
			eventType:   database.StreamEventOutput,
			content:     OutputContent{Text: "test output", Complete: true},
			wantNonNil:  true,
			description: "output event",
		},
		{
			eventType:   database.StreamEventToolCall,
			content:     ToolCallContent{ToolName: "test", ToolID: "123"},
			wantNonNil:  true,
			description: "tool call event",
		},
		{
			eventType:   database.StreamEventToolResult,
			content:     ToolResultContent{ToolID: "123", Success: true},
			wantNonNil:  true,
			description: "tool result event",
		},
		{
			eventType:   database.StreamEventFinding,
			content:     FindingContent{ID: "f1", Title: "Test", Severity: "low"},
			wantNonNil:  true,
			description: "finding event",
		},
		{
			eventType:   database.StreamEventStatus,
			content:     StatusContent{Status: "running"},
			wantNonNil:  true,
			description: "status event",
		},
		{
			eventType:   database.StreamEventSteeringAck,
			content:     SteeringAckContent{Sequence: 1, Accepted: true},
			wantNonNil:  true,
			description: "steering ack event",
		},
		{
			eventType:   database.StreamEventError,
			content:     ErrorContent{Message: "test error"},
			wantNonNil:  true,
			description: "error event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			contentJSON, err := json.Marshal(tt.content)
			if err != nil {
				t.Fatalf("failed to marshal content: %v", err)
			}

			event := &database.StreamEvent{
				ID:        types.NewID(),
				EventType: tt.eventType,
				Content:   contentJSON,
				Timestamp: ts,
			}

			lines := renderer.RenderEvent(event)

			if tt.wantNonNil && lines == nil {
				t.Error("expected non-nil lines")
			}
			if tt.wantNonNil && len(lines) == 0 {
				t.Error("expected at least one line")
			}
		})
	}
}

func TestEventRenderer_RenderUnknownEvent(t *testing.T) {
	renderer := NewEventRenderer("test-agent")
	ts := time.Now()

	event := &database.StreamEvent{
		ID:        types.NewID(),
		EventType: database.StreamEventType("unknown_type"),
		Content:   json.RawMessage(`{}`),
		Timestamp: ts,
	}

	lines := renderer.RenderEvent(event)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	if !containsIgnoreCase(lines[0].Text, "unknown_type") {
		t.Errorf("expected line to mention unknown event type")
	}
}

func TestEventRenderer_ParseError(t *testing.T) {
	renderer := NewEventRenderer("test-agent")
	ts := time.Now()

	// Invalid JSON
	invalidJSON := json.RawMessage(`{invalid json}`)

	lines := renderer.RenderOutput(invalidJSON, ts)
	if len(lines) != 2 {
		t.Fatalf("expected 2 error lines, got %d", len(lines))
	}

	if lines[0].Style != StyleError {
		t.Errorf("expected error style, got %v", lines[0].Style)
	}
	if lines[0].Source != SourceSystem {
		t.Errorf("expected system source, got %v", lines[0].Source)
	}
}

func TestSeverityToStyle(t *testing.T) {
	tests := []struct {
		severity string
		want     OutputStyle
	}{
		{"critical", StyleFindingCritical},
		{"CRITICAL", StyleFindingCritical},
		{"Critical", StyleFindingCritical},
		{"high", StyleFindingHigh},
		{"HIGH", StyleFindingHigh},
		{"medium", StyleFindingMedium},
		{"low", StyleFindingLow},
		{"info", StyleFinding},
		{"unknown", StyleFinding},
		{"", StyleFinding},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityToStyle(tt.severity)
			if got != tt.want {
				t.Errorf("severityToStyle(%q) = %v, want %v", tt.severity, got, tt.want)
			}
		})
	}
}

func TestSanitizeTerminalOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal text",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "ansi escape",
			input: "Hello \x1b[31mred\x1b[0m world",
			want:  "Hello [ESC[31mred[ESC[0m world",
		},
		{
			name:  "bell character",
			input: "Hello\x07world",
			want:  "Helloworld",
		},
		{
			name:  "backspace",
			input: "Hello\x08world",
			want:  "Helloworld",
		},
		{
			name:  "delete character",
			input: "Hello\x7fworld",
			want:  "Helloworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTerminalOutput(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTerminalOutput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"hello", 3, "hel"},
		{"hello", 4, "h..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatToolOutput(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "nil",
			input: nil,
			want:  "",
		},
		{
			name:  "map",
			input: map[string]any{"key": "value"},
			want:  "{\n  \"key\": \"value\"\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatToolOutput(tt.input)
			if got != tt.want {
				t.Errorf("formatToolOutput(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Helper function for case-insensitive contains check
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsIgnoreCaseImpl(s, substr)))
}

func containsIgnoreCaseImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
