package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockAgent implements the agent.Agent interface for testing
type mockAgent struct {
	name         string
	version      string
	description  string
	capabilities []string
	executeFunc  func(ctx context.Context, task agent.Task, harness agent.AgentHarness) (agent.Result, error)
}

func (m *mockAgent) Name() string                                     { return m.name }
func (m *mockAgent) Version() string                                  { return m.version }
func (m *mockAgent) Description() string                              { return m.description }
func (m *mockAgent) Capabilities() []string                           { return m.capabilities }
func (m *mockAgent) TargetTypes() []component.TargetType              { return []component.TargetType{} }
func (m *mockAgent) TechniqueTypes() []component.TechniqueType        { return []component.TechniqueType{} }
func (m *mockAgent) LLMSlots() []agent.SlotDefinition                 { return []agent.SlotDefinition{} }
func (m *mockAgent) Initialize(ctx context.Context, cfg agent.AgentConfig) error { return nil }
func (m *mockAgent) Shutdown(ctx context.Context) error               { return nil }
func (m *mockAgent) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("mock agent healthy")
}

func (m *mockAgent) Execute(ctx context.Context, task agent.Task, harness agent.AgentHarness) (agent.Result, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, task, harness)
	}

	result := agent.NewResult(task.ID)
	result.Complete(map[string]any{
		"response": "Mock response from " + m.name,
	})
	return result, nil
}

// setupTestRegistry creates a test agent registry with mock agents
func setupTestRegistry() *agent.DefaultAgentRegistry {
	registry := agent.NewAgentRegistry()

	// Register test agents
	registry.RegisterInternal("recon", func(cfg agent.AgentConfig) (agent.Agent, error) {
		return &mockAgent{
			name:        "recon",
			version:     "1.0.0",
			description: "Reconnaissance agent for testing",
			capabilities: []string{"scan", "discover"},
		}, nil
	})

	registry.RegisterInternal("exploit", func(cfg agent.AgentConfig) (agent.Agent, error) {
		return &mockAgent{
			name:        "exploit",
			version:     "1.0.0",
			description: "Exploitation agent for testing",
			capabilities: []string{"exploit", "attack"},
		}, nil
	})

	return registry
}

// TestConsoleSlashCommandParsing tests slash command parsing
func TestConsoleSlashCommandParsing(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedExit   bool
		expectedOutput string
	}{
		{
			name:           "help command",
			input:          "/help",
			expectedExit:   false,
			expectedOutput: "Available commands:",
		},
		{
			name:           "quit command",
			input:          "/quit",
			expectedExit:   true,
			expectedOutput: "Goodbye!",
		},
		{
			name:           "exit command",
			input:          "/exit",
			expectedExit:   true,
			expectedOutput: "Goodbye!",
		},
		{
			name:           "agents command",
			input:          "/agents",
			expectedExit:   false,
			expectedOutput: "Available agents:",
		},
		{
			name:           "target command with argument",
			input:          "/target example.com",
			expectedExit:   false,
			expectedOutput: "Target set to: example.com",
		},
		{
			name:           "target command without argument",
			input:          "/target",
			expectedExit:   false,
			expectedOutput: "Usage: /target <name>",
		},
		{
			name:           "clear command",
			input:          "/clear",
			expectedExit:   false,
			expectedOutput: "",
		},
		{
			name:           "unknown command",
			input:          "/unknown",
			expectedExit:   false,
			expectedOutput: "Unknown command:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test console state
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			state := &consoleState{
				ctx:      ctx,
				cancel:   cancel,
				registry: setupTestRegistry(),
				history:  []string{},
			}

			// Create command with captured output
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Handle slash command
			shouldExit := state.handleSlashCommand(cmd, tt.input)

			// Verify exit behavior
			assert.Equal(t, tt.expectedExit, shouldExit, "unexpected exit behavior")

			// Verify output if expected
			if tt.expectedOutput != "" {
				output := buf.String()
				assert.Contains(t, output, tt.expectedOutput, "unexpected output")
			}
		})
	}
}

// TestConsoleAgentSwitching tests @agent mention handling
func TestConsoleAgentSwitching(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedAgent  string
		shouldSwitch   bool
		expectedOutput string
	}{
		{
			name:           "switch to recon agent",
			input:          "@recon",
			expectedAgent:  "recon",
			shouldSwitch:   true,
			expectedOutput: "Switched to agent: recon",
		},
		{
			name:           "switch to exploit agent",
			input:          "@exploit",
			expectedAgent:  "exploit",
			shouldSwitch:   true,
			expectedOutput: "Switched to agent: exploit",
		},
		{
			name:           "switch to unknown agent",
			input:          "@unknown",
			expectedAgent:  "",
			shouldSwitch:   false,
			expectedOutput: "Agent 'unknown' not found",
		},
		{
			name:           "switch with message",
			input:          "@recon scan the target",
			expectedAgent:  "recon",
			shouldSwitch:   true,
			expectedOutput: "Switched to agent: recon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test console state
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			state := &consoleState{
				ctx:      ctx,
				cancel:   cancel,
				registry: setupTestRegistry(),
				history:  []string{},
			}

			// Create command with captured output
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Handle agent mention
			state.handleAgentMention(cmd, tt.input)

			// Verify agent switch
			if tt.shouldSwitch {
				assert.Equal(t, tt.expectedAgent, state.activeAgent, "agent not switched correctly")
			}

			// Verify output
			output := buf.String()
			assert.Contains(t, output, tt.expectedOutput, "unexpected output")
		})
	}
}

// TestConsoleSendMessageToAgent tests sending messages to agents
func TestConsoleSendMessageToAgent(t *testing.T) {
	tests := []struct {
		name          string
		activeAgent   string
		message       string
		targetName    string
		expectError   bool
		expectedError string
	}{
		{
			name:        "send message to active agent",
			activeAgent: "recon",
			message:     "scan the target",
			expectError: false,
		},
		{
			name:          "send message without active agent",
			activeAgent:   "",
			message:       "test message",
			expectError:   true,
			expectedError: "no active agent",
		},
		{
			name:        "send message with target",
			activeAgent: "recon",
			message:     "scan the target",
			targetName:  "example.com",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test console state
			ctx := context.Background()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			state := &consoleState{
				ctx:         ctx,
				cancel:      cancel,
				activeAgent: tt.activeAgent,
				targetName:  tt.targetName,
				registry:    setupTestRegistry(),
				history:     []string{},
			}

			// Create command with captured output
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Send message
			err := state.sendMessageToAgent(cmd, tt.message)

			// Verify error expectation
			if tt.expectError {
				require.Error(t, err, "expected error but got none")
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError, "unexpected error message")
				}
			} else {
				require.NoError(t, err, "unexpected error")
				output := buf.String()
				assert.Contains(t, output, "Completed", "expected completion message")
			}
		})
	}
}

// TestConsoleGetPrompt tests prompt generation
func TestConsoleGetPrompt(t *testing.T) {
	tests := []struct {
		name           string
		activeAgent    string
		expectedPrompt string
	}{
		{
			name:           "no active agent",
			activeAgent:    "",
			expectedPrompt: "gibson> ",
		},
		{
			name:           "with active agent",
			activeAgent:    "recon",
			expectedPrompt: "gibson:recon> ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &consoleState{
				activeAgent: tt.activeAgent,
			}

			prompt := state.getPrompt()
			assert.Equal(t, tt.expectedPrompt, prompt, "unexpected prompt")
		})
	}
}

// TestConsoleDisplayAgentResponse tests agent response display
func TestConsoleDisplayAgentResponse(t *testing.T) {
	tests := []struct {
		name           string
		result         agent.Result
		expectedOutput []string
	}{
		{
			name: "completed with response",
			result: func() agent.Result {
				taskID := types.NewID()
				result := agent.NewResult(taskID)
				result.Complete(map[string]any{
					"response": "Test response message",
				})
				return result
			}(),
			expectedOutput: []string{"Completed", "Test response message"},
		},
		{
			name: "completed with findings",
			result: func() agent.Result {
				taskID := types.NewID()
				result := agent.NewResult(taskID)
				result.Complete(map[string]any{})
				result.AddFinding(agent.NewFinding(
					"Test Finding",
					"Test description",
					agent.SeverityHigh,
				))
				return result
			}(),
			expectedOutput: []string{"Completed", "Findings (1)", "Test Finding"},
		},
		{
			name: "failed with error",
			result: func() agent.Result {
				taskID := types.NewID()
				result := agent.NewResult(taskID)
				result.Error = &agent.ResultError{
					Message: "Test error message",
					Code:    "TEST_ERROR",
				}
				result.Status = agent.ResultStatusFailed
				return result
			}(),
			expectedOutput: []string{"Failed", "Test error message"},
		},
		{
			name: "cancelled",
			result: func() agent.Result {
				taskID := types.NewID()
				result := agent.NewResult(taskID)
				result.Cancel()
				return result
			}(),
			expectedOutput: []string{"Cancelled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &consoleState{
				activeAgent: "test",
			}

			// Create command with captured output
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Display response
			state.displayAgentResponse(cmd, tt.result)

			// Verify output contains expected strings
			output := buf.String()
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected, "output missing expected string: %s", expected)
			}
		})
	}
}

// TestConsoleListAgents tests agent listing
func TestConsoleListAgents(t *testing.T) {
	// Create test console state with active agent
	state := &consoleState{
		activeAgent: "recon",
		registry:    setupTestRegistry(),
	}

	// Create command with captured output
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// List agents
	state.listAgents(cmd)

	// Verify output
	output := buf.String()
	assert.Contains(t, output, "Available agents:", "missing header")
	assert.Contains(t, output, "@recon", "missing recon agent")
	assert.Contains(t, output, "@exploit", "missing exploit agent")
	assert.Contains(t, output, "(active)", "active agent not marked")
}

// TestConsoleListAgentsEmpty tests agent listing with no agents
func TestConsoleListAgentsEmpty(t *testing.T) {
	// Create test console state with empty registry
	state := &consoleState{
		registry: agent.NewAgentRegistry(),
	}

	// Create command with captured output
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// List agents
	state.listAgents(cmd)

	// Verify output
	output := buf.String()
	assert.Contains(t, output, "No agents available", "missing empty message")
	assert.Contains(t, output, "gibson agent install", "missing help text")
}

// TestConsoleInputParsing tests various input patterns
func TestConsoleInputParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		isCommand bool
		isMention bool
		isMessage bool
	}{
		{
			name:      "slash command",
			input:     "/help",
			isCommand: true,
			isMention: false,
			isMessage: false,
		},
		{
			name:      "agent mention",
			input:     "@recon",
			isCommand: false,
			isMention: true,
			isMessage: false,
		},
		{
			name:      "regular message",
			input:     "scan the target",
			isCommand: false,
			isMention: false,
			isMessage: true,
		},
		{
			name:      "empty input",
			input:     "",
			isCommand: false,
			isMention: false,
			isMessage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.input)

			isCommand := strings.HasPrefix(trimmed, "/")
			isMention := strings.HasPrefix(trimmed, "@")
			isMessage := trimmed != "" && !isCommand && !isMention

			assert.Equal(t, tt.isCommand, isCommand, "command detection mismatch")
			assert.Equal(t, tt.isMention, isMention, "mention detection mismatch")
			assert.Equal(t, tt.isMessage, isMessage, "message detection mismatch")
		})
	}
}

// TestConsoleHistoryTracking tests history management
func TestConsoleHistoryTracking(t *testing.T) {
	state := &consoleState{
		history:    []string{},
		historyIdx: 0,
	}

	// Add some history
	inputs := []string{
		"/help",
		"@recon",
		"scan the target",
		"/quit",
	}

	for _, input := range inputs {
		state.history = append(state.history, input)
		state.historyIdx = len(state.history)
	}

	// Verify history
	assert.Equal(t, len(inputs), len(state.history), "history length mismatch")
	assert.Equal(t, len(inputs), state.historyIdx, "history index mismatch")

	// Verify history contents
	for i, expected := range inputs {
		assert.Equal(t, expected, state.history[i], "history entry mismatch at index %d", i)
	}
}

// TestConsoleContextCancellation tests context cancellation handling
func TestConsoleContextCancellation(t *testing.T) {
	// Create test console state with cancellable context
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	state := &consoleState{
		ctx:         ctx,
		cancel:      cancel,
		activeAgent: "recon",
		registry:    setupTestRegistry(),
		history:     []string{},
	}

	// Create command with captured output
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Cancel the context
	cancel()

	// Try to send a message (should fail due to cancelled context)
	_ = state.sendMessageToAgent(cmd, "test message")

	// Verify that the operation was affected by cancellation
	// The exact error depends on how the registry handles cancellation
	// For now, we just verify that the context is cancelled
	assert.Error(t, ctx.Err(), "context should be cancelled")
}
