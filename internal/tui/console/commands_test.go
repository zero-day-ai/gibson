package console

import (
	"context"
	"reflect"
	"sort"
	"testing"
)

func TestNewCommandRegistry(t *testing.T) {
	registry := NewCommandRegistry()
	if registry == nil {
		t.Fatal("NewCommandRegistry returned nil")
	}
	if registry.commands == nil {
		t.Error("commands map is nil")
	}
	if len(registry.commands) != 0 {
		t.Errorf("new registry has %d commands, want 0", len(registry.commands))
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name            string
		command         *SlashCommand
		wantRegistered  bool
		wantCommandName string
		wantAliases     []string
	}{
		{
			name: "register command without aliases",
			command: &SlashCommand{
				Name:        "test",
				Description: "Test command",
				Usage:       "/test",
			},
			wantRegistered:  true,
			wantCommandName: "test",
			wantAliases:     []string{},
		},
		{
			name: "register command with aliases",
			command: &SlashCommand{
				Name:        "status",
				Aliases:     []string{"s", "stat"},
				Description: "Show status",
				Usage:       "/status",
			},
			wantRegistered:  true,
			wantCommandName: "status",
			wantAliases:     []string{"s", "stat"},
		},
		{
			name:           "register nil command",
			command:        nil,
			wantRegistered: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewCommandRegistry()
			registry.Register(tt.command)

			if !tt.wantRegistered {
				if len(registry.commands) != 0 {
					t.Errorf("expected no commands registered, got %d", len(registry.commands))
				}
				return
			}

			// Check primary name is registered
			cmd, ok := registry.Get(tt.wantCommandName)
			if !ok {
				t.Errorf("command %q not found", tt.wantCommandName)
			}
			if cmd != tt.command {
				t.Errorf("Get(%q) returned different command instance", tt.wantCommandName)
			}

			// Check all aliases are registered
			for _, alias := range tt.wantAliases {
				cmd, ok := registry.Get(alias)
				if !ok {
					t.Errorf("alias %q not found", alias)
				}
				if cmd != tt.command {
					t.Errorf("Get(%q) returned different command instance", alias)
				}
			}
		})
	}
}

func TestRegisterOverwrite(t *testing.T) {
	registry := NewCommandRegistry()

	// Register first command
	cmd1 := &SlashCommand{
		Name:        "test",
		Description: "First test command",
	}
	registry.Register(cmd1)

	// Register second command with same name
	cmd2 := &SlashCommand{
		Name:        "test",
		Description: "Second test command",
	}
	registry.Register(cmd2)

	// Should have the second command
	cmd, ok := registry.Get("test")
	if !ok {
		t.Fatal("command not found")
	}
	if cmd.Description != "Second test command" {
		t.Errorf("got description %q, want %q", cmd.Description, "Second test command")
	}
}

func TestGet(t *testing.T) {
	registry := NewCommandRegistry()
	testCmd := &SlashCommand{
		Name:        "agents",
		Aliases:     []string{"a", "ag"},
		Description: "Manage agents",
	}
	registry.Register(testCmd)

	tests := []struct {
		name    string
		lookup  string
		wantCmd *SlashCommand
		wantOk  bool
	}{
		{
			name:    "get by primary name",
			lookup:  "agents",
			wantCmd: testCmd,
			wantOk:  true,
		},
		{
			name:    "get by first alias",
			lookup:  "a",
			wantCmd: testCmd,
			wantOk:  true,
		},
		{
			name:    "get by second alias",
			lookup:  "ag",
			wantCmd: testCmd,
			wantOk:  true,
		},
		{
			name:    "get non-existent command",
			lookup:  "nonexistent",
			wantCmd: nil,
			wantOk:  false,
		},
		{
			name:    "get with empty string",
			lookup:  "",
			wantCmd: nil,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := registry.Get(tt.lookup)
			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v", tt.lookup, ok, tt.wantOk)
			}
			if cmd != tt.wantCmd {
				t.Errorf("Get(%q) returned wrong command", tt.lookup)
			}
		})
	}
}

func TestList(t *testing.T) {
	registry := NewCommandRegistry()

	// Register commands
	cmd1 := &SlashCommand{Name: "zebra", Description: "Last alphabetically"}
	cmd2 := &SlashCommand{Name: "alpha", Description: "First alphabetically"}
	cmd3 := &SlashCommand{Name: "beta", Aliases: []string{"b", "bet"}, Description: "Second alphabetically"}

	registry.Register(cmd1)
	registry.Register(cmd2)
	registry.Register(cmd3)

	commands := registry.List()

	// Should have 3 unique commands (aliases don't create duplicates)
	if len(commands) != 3 {
		t.Errorf("List() returned %d commands, want 3", len(commands))
	}

	// Should be sorted alphabetically
	expectedOrder := []string{"alpha", "beta", "zebra"}
	for i, want := range expectedOrder {
		if commands[i].Name != want {
			t.Errorf("commands[%d].Name = %q, want %q", i, commands[i].Name, want)
		}
	}

	// Verify each command is in the list exactly once
	seen := make(map[*SlashCommand]int)
	for _, cmd := range commands {
		seen[cmd]++
	}
	for cmd, count := range seen {
		if count != 1 {
			t.Errorf("command %q appears %d times, want 1", cmd.Name, count)
		}
	}
}

func TestListEmpty(t *testing.T) {
	registry := NewCommandRegistry()
	commands := registry.List()

	// List() may return nil or empty slice, both are acceptable
	if len(commands) != 0 {
		t.Errorf("List() returned %d commands, want 0", len(commands))
	}
}

func TestComplete(t *testing.T) {
	registry := NewCommandRegistry()

	// Register test commands
	registry.Register(&SlashCommand{Name: "agents", Aliases: []string{"a"}})
	registry.Register(&SlashCommand{Name: "alpha"})
	registry.Register(&SlashCommand{Name: "config"})
	registry.Register(&SlashCommand{Name: "clear"})
	registry.Register(&SlashCommand{Name: "status"})

	tests := []struct {
		name   string
		prefix string
		want   []string
	}{
		{
			name:   "prefix matches multiple commands",
			prefix: "a",
			want:   []string{"a", "agents", "alpha"},
		},
		{
			name:   "prefix matches single command",
			prefix: "ag",
			want:   []string{"agents"},
		},
		{
			name:   "prefix matches no commands",
			prefix: "xyz",
			want:   nil, // Complete returns nil for no matches
		},
		{
			name:   "prefix matches commands starting with c",
			prefix: "c",
			want:   []string{"clear", "config"},
		},
		{
			name:   "full command name",
			prefix: "status",
			want:   []string{"status"},
		},
		{
			name:   "prefix matches alias",
			prefix: "a",
			want:   []string{"a", "agents", "alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registry.Complete(tt.prefix)

			// Sort for consistent comparison (handle nil)
			if got != nil {
				sort.Strings(got)
			}
			if tt.want != nil {
				sort.Strings(tt.want)
			}

			// Handle nil vs empty slice equivalence
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Complete(%q) = %v, want %v", tt.prefix, got, tt.want)
			}
		})
	}
}

func TestRegisterDefaultCommands(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	// List of expected default commands (primary names, not aliases)
	expectedCommands := []string{
		"agent",
		"agent-start",
		"agent-execute",
		"agent-stop",
		"agent-install",
		"agent-uninstall",
		"agent-logs",
		"tool",
		"tool-start",
		"tool-stop",
		"tool-install",
		"tool-uninstall",
		"tool-logs",
		"plugin",
		"plugin-start",
		"plugin-stop",
		"plugin-install",
		"plugin-uninstall",
		"attack",
		"mission",
		"mission-start",
		"mission-stop",
		"mission-create",
		"mission-delete",
		"focus",
		"interrupt",
		"mode",
		"pause",
		"resume",
		"credentials",
		"findings",
		"targets",
		"status",
		"config",
		"help",
		"clear",
	}

	// Check that all expected commands are registered
	for _, name := range expectedCommands {
		cmd, ok := registry.Get(name)
		if !ok {
			t.Errorf("default command %q not found", name)
			continue
		}
		if cmd.Name != name {
			t.Errorf("command name mismatch: got %q, want %q", cmd.Name, name)
		}
		if cmd.Description == "" {
			t.Errorf("command %q has empty description", name)
		}
		if cmd.Usage == "" {
			t.Errorf("command %q has empty usage", name)
		}
		// Handler should be nil (to be set by executor)
		if cmd.Handler != nil {
			t.Errorf("command %q has non-nil handler", name)
		}
	}

	// Verify total count
	commands := registry.List()
	if len(commands) != len(expectedCommands) {
		t.Errorf("registered %d commands, want %d", len(commands), len(expectedCommands))
	}
}

func TestCommandRegistryWithHandlers(t *testing.T) {
	registry := NewCommandRegistry()

	called := false
	handler := func(ctx context.Context, args []string) (*ExecutionResult, error) {
		called = true
		return &ExecutionResult{
			Output: "test output",
		}, nil
	}

	cmd := &SlashCommand{
		Name:    "test",
		Handler: handler,
	}
	registry.Register(cmd)

	// Retrieve and execute the handler
	retrieved, ok := registry.Get("test")
	if !ok {
		t.Fatal("command not found")
	}

	if retrieved.Handler == nil {
		t.Fatal("handler is nil")
	}

	result, err := retrieved.Handler(context.Background(), []string{})
	if err != nil {
		t.Errorf("handler returned error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result.Output != "test output" {
		t.Errorf("handler output = %q, want %q", result.Output, "test output")
	}
}

func TestCommandRegistryMultipleAliases(t *testing.T) {
	registry := NewCommandRegistry()

	cmd := &SlashCommand{
		Name:    "status",
		Aliases: []string{"s", "stat", "st"},
	}
	registry.Register(cmd)

	// All aliases should point to the same command instance
	lookups := []string{"status", "s", "stat", "st"}
	for _, lookup := range lookups {
		retrieved, ok := registry.Get(lookup)
		if !ok {
			t.Errorf("lookup %q failed", lookup)
			continue
		}
		if retrieved != cmd {
			t.Errorf("lookup %q returned different command instance", lookup)
		}
	}

	// List should only return one command
	commands := registry.List()
	if len(commands) != 1 {
		t.Errorf("List() returned %d commands, want 1", len(commands))
	}
}

func TestCompleteCaseSensitive(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&SlashCommand{Name: "Status"})
	registry.Register(&SlashCommand{Name: "status"})
	registry.Register(&SlashCommand{Name: "STATUS"})

	// Should match all three (case-sensitive)
	matches := registry.Complete("s")
	if len(matches) != 1 {
		// Only "status" starts with lowercase "s"
		t.Errorf("Complete('s') matched %d commands, want 1", len(matches))
	}

	matches = registry.Complete("S")
	if len(matches) != 2 {
		// "Status" and "STATUS" start with uppercase "S"
		t.Errorf("Complete('S') matched %d commands, want 2", len(matches))
	}
}
