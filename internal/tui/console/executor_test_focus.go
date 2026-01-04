package console

import (
	"context"
	"strings"
	"testing"
)

// TestHandleFocus tests the simplified focus handler that doesn't validate agents
func TestHandleFocus(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	tests := []struct {
		name            string
		args            []string
		initialFocus    string
		wantFocus       string
		wantOutputMatch string
	}{
		{
			name:            "no args shows current focus when none set",
			args:            []string{},
			initialFocus:    "",
			wantFocus:       "",
			wantOutputMatch: "No agent currently focused",
		},
		{
			name:            "no args shows current focus when set",
			args:            []string{},
			initialFocus:    "test-agent",
			wantFocus:       "test-agent",
			wantOutputMatch: "Currently focused on agent: test-agent",
		},
		{
			name:            "focus on agent sets focus",
			args:            []string{"davinci"},
			initialFocus:    "",
			wantFocus:       "davinci",
			wantOutputMatch: "Focused on agent: davinci",
		},
		{
			name:            "focus changes from one agent to another",
			args:            []string{"new-agent"},
			initialFocus:    "old-agent",
			wantFocus:       "new-agent",
			wantOutputMatch: "Focused on agent: new-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
			executor.SetupHandlers()
			executor.SetFocusedAgent(tt.initialFocus)

			result, err := executor.handleFocus(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("handleFocus returned error: %v", err)
			}

			if result.IsError {
				t.Errorf("unexpected error: %s", result.Error)
			}

			if executor.GetFocusedAgent() != tt.wantFocus {
				t.Errorf("focused agent = %q, want %q", executor.GetFocusedAgent(), tt.wantFocus)
			}

			if tt.wantOutputMatch != "" && !strings.Contains(result.Output, tt.wantOutputMatch) {
				t.Errorf("expected output containing %q, got %q", tt.wantOutputMatch, result.Output)
			}
		})
	}
}
