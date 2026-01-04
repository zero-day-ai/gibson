package console

import (
	"context"
	"strings"
	"testing"
)

func TestHandleHelp(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantContains   []string
		wantNotContain []string
		wantError      bool
	}{
		{
			name: "no args shows all commands",
			args: []string{},
			wantContains: []string{
				"Available Commands:",
				"agent",
				"mission",
				"tool",
				"plugin",
				"attack",
				"help",
				"Use /help <command>",
			},
			wantError: false,
		},
		{
			name: "help for mission command shows subcommands",
			args: []string{"mission"},
			wantContains: []string{
				"Mission Commands:",
				"list",
				"show",
				"run",
				"resume",
				"stop",
				"delete",
				"Use /help mission <subcommand>",
			},
			wantError: false,
		},
		{
			name: "help for agent command shows subcommands",
			args: []string{"agent"},
			wantContains: []string{
				"Agent Commands:",
				"list",
				"show",
				"install",
				"uninstall",
				"start",
				"stop",
				"status",
				"logs",
			},
			wantError: false,
		},
		{
			name: "help for agent start shows detailed help",
			args: []string{"agent", "start"},
			wantContains: []string{
				"Start an agent",
				"Usage: /agent start <name>",
				"--target",
				"--mode",
			},
			wantError: false,
		},
		{
			name: "help for mission run shows detailed help",
			args: []string{"mission", "run"},
			wantContains: []string{
				"Run mission from workflow",
				"Usage: /mission run -f <file>",
			},
			wantError: false,
		},
		{
			name: "help for unknown command returns error",
			args: []string{"unknown"},
			wantContains: []string{
				"Unknown command: unknown",
				"Use /help to see all available commands",
			},
			wantError: true,
		},
		{
			name: "help for unknown subcommand returns error",
			args: []string{"mission", "unknown"},
			wantContains: []string{
				"Unknown subcommand: mission unknown",
				"Use /help mission to see available subcommands",
			},
			wantError: true,
		},
		{
			name: "help for command without subcommands shows usage",
			args: []string{"attack"},
			wantContains: []string{
				"Launch a quick single-agent attack",
				"Usage: /attack",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create registry and setup commands
			registry := NewCommandRegistry()
			RegisterDefaultCommands(registry)

			// Create executor and setup handlers
			executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
			executor.SetupHandlers()

			// Execute help
			result, err := executor.handleHelp(context.Background(), tt.args)

			// Check error expectation
			if err != nil {
				t.Fatalf("handleHelp returned error: %v", err)
			}

			if result == nil {
				t.Fatal("handleHelp returned nil result")
			}

			if result.IsError != tt.wantError {
				t.Errorf("IsError = %v, want %v", result.IsError, tt.wantError)
			}

			// Check output contains expected strings
			output := result.Output
			if result.IsError {
				output = result.Error
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("Output does not contain %q\nOutput:\n%s", want, output)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(output, notWant) {
					t.Errorf("Output should not contain %q\nOutput:\n%s", notWant, output)
				}
			}
		})
	}
}

func TestExecuteWithHelpFlag(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		wantContains []string
		wantError    bool
	}{
		{
			name:    "mission --help shows mission subcommands",
			command: "/mission --help",
			wantContains: []string{
				"Mission Commands:",
				"list",
				"run",
			},
			wantError: false,
		},
		{
			name:    "mission -h shows mission subcommands",
			command: "/mission -h",
			wantContains: []string{
				"Mission Commands:",
			},
			wantError: false,
		},
		{
			name:    "agent start --help shows agent start help",
			command: "/agent start --help",
			wantContains: []string{
				"Start an agent",
				"Usage: /agent start",
			},
			wantError: false,
		},
		{
			name:    "agent start -h shows agent start help",
			command: "/agent start -h",
			wantContains: []string{
				"Start an agent",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create registry and setup commands
			registry := NewCommandRegistry()
			RegisterDefaultCommands(registry)

			// Create executor and setup handlers
			executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
			executor.SetupHandlers()

			// Execute command with help flag
			result, err := executor.Execute(tt.command)

			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}

			if result == nil {
				t.Fatal("Execute returned nil result")
			}

			if result.IsError != tt.wantError {
				t.Errorf("IsError = %v, want %v", result.IsError, tt.wantError)
			}

			// Check output contains expected strings
			output := result.Output
			if result.IsError {
				output = result.Error
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("Output does not contain %q\nOutput:\n%s", want, output)
				}
			}
		})
	}
}
