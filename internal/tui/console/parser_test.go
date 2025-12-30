package console

import (
	"testing"
)

func TestIsSlashCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid slash command",
			input:    "/help",
			expected: true,
		},
		{
			name:     "slash command with whitespace",
			input:    "  /agents  ",
			expected: true,
		},
		{
			name:     "not a slash command",
			input:    "help",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: false,
		},
		{
			name:     "slash at end",
			input:    "help/",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSlashCommand(tt.input)
			if result != tt.expected {
				t.Errorf("IsSlashCommand(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *ParsedCommand
		expectError bool
	}{
		{
			name:  "simple command",
			input: "/help",
			expected: &ParsedCommand{
				Name:       "help",
				Subcommand: "",
				Args:       []string{},
				Flags:      map[string]string{},
			},
			expectError: false,
		},
		{
			name:  "command with subcommand",
			input: "/agents list",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags:      map[string]string{},
			},
			expectError: false,
		},
		{
			name:  "command with subcommand and args",
			input: "/missions create TestMission",
			expected: &ParsedCommand{
				Name:       "missions",
				Subcommand: "create",
				Args:       []string{"TestMission"},
				Flags:      map[string]string{},
			},
			expectError: false,
		},
		{
			name:  "command with quoted arg containing spaces",
			input: `/missions create "Test Mission"`,
			expected: &ParsedCommand{
				Name:       "missions",
				Subcommand: "create",
				Args:       []string{"Test Mission"},
				Flags:      map[string]string{},
			},
			expectError: false,
		},
		{
			name:  "command with single-quoted arg",
			input: `/missions create 'Test Mission'`,
			expected: &ParsedCommand{
				Name:       "missions",
				Subcommand: "create",
				Args:       []string{"Test Mission"},
				Flags:      map[string]string{},
			},
			expectError: false,
		},
		{
			name:  "command with boolean flag",
			input: "/config --verbose",
			expected: &ParsedCommand{
				Name:       "config",
				Subcommand: "",
				Args:       []string{},
				Flags: map[string]string{
					"verbose": "true",
				},
			},
			expectError: false,
		},
		{
			name:  "command with flag using equals",
			input: "/agents list --format=json",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags: map[string]string{
					"format": "json",
				},
			},
			expectError: false,
		},
		{
			name:  "command with flag using space",
			input: "/agents list --format json",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags: map[string]string{
					"format": "json",
				},
			},
			expectError: false,
		},
		{
			name:  "command with multiple flags",
			input: "/agents list --format json --verbose",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags: map[string]string{
					"format":  "json",
					"verbose": "true",
				},
			},
			expectError: false,
		},
		{
			name:  "command with args and flags",
			input: `/missions create "Test Mission" --priority high`,
			expected: &ParsedCommand{
				Name:       "missions",
				Subcommand: "create",
				Args:       []string{"Test Mission"},
				Flags: map[string]string{
					"priority": "high",
				},
			},
			expectError: false,
		},
		{
			name:  "command with multiple args",
			input: "/exec ls -la /tmp",
			expected: &ParsedCommand{
				Name:       "exec",
				Subcommand: "ls",
				Args:       []string{"/tmp"},
				Flags: map[string]string{
					"la": "true",
				},
			},
			expectError: false,
		},
		{
			name:  "command with extra whitespace",
			input: "  /agents   list   --format   json  ",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags: map[string]string{
					"format": "json",
				},
			},
			expectError: false,
		},
		{
			name:        "empty input",
			input:       "",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "only whitespace",
			input:       "   ",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "no slash",
			input:       "help",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "only slash",
			input:       "/",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "slash with only whitespace",
			input:       "/   ",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "unclosed quote",
			input:       `/missions create "unclosed`,
			expected:    nil,
			expectError: true,
		},
		{
			name:  "short flag",
			input: "/agents -v",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "",
				Args:       []string{},
				Flags: map[string]string{
					"v": "true",
				},
			},
			expectError: false,
		},
		{
			name:  "mixed short and long flags",
			input: "/agents list -v --format json",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags: map[string]string{
					"v":      "true",
					"format": "json",
				},
			},
			expectError: false,
		},
		{
			name:  "flag with empty value",
			input: "/config --key=",
			expected: &ParsedCommand{
				Name:       "config",
				Subcommand: "",
				Args:       []string{},
				Flags: map[string]string{
					"key": "",
				},
			},
			expectError: false,
		},
		{
			name:  "consecutive flags",
			input: "/agents --verbose --debug",
			expected: &ParsedCommand{
				Name:       "agents",
				Subcommand: "",
				Args:       []string{},
				Flags: map[string]string{
					"verbose": "true",
					"debug":   "true",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("Parse(%q).Name = %q, want %q", tt.input, result.Name, tt.expected.Name)
			}

			if result.Subcommand != tt.expected.Subcommand {
				t.Errorf("Parse(%q).Subcommand = %q, want %q", tt.input, result.Subcommand, tt.expected.Subcommand)
			}

			if len(result.Args) != len(tt.expected.Args) {
				t.Errorf("Parse(%q).Args length = %d, want %d", tt.input, len(result.Args), len(tt.expected.Args))
			} else {
				for i, arg := range result.Args {
					if arg != tt.expected.Args[i] {
						t.Errorf("Parse(%q).Args[%d] = %q, want %q", tt.input, i, arg, tt.expected.Args[i])
					}
				}
			}

			if len(result.Flags) != len(tt.expected.Flags) {
				t.Errorf("Parse(%q).Flags length = %d, want %d", tt.input, len(result.Flags), len(tt.expected.Flags))
			} else {
				for key, value := range tt.expected.Flags {
					if result.Flags[key] != value {
						t.Errorf("Parse(%q).Flags[%q] = %q, want %q", tt.input, key, result.Flags[key], value)
					}
				}
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []string
		expectError bool
	}{
		{
			name:        "simple tokens",
			input:       "one two three",
			expected:    []string{"one", "two", "three"},
			expectError: false,
		},
		{
			name:        "double quoted string",
			input:       `one "two three" four`,
			expected:    []string{"one", "two three", "four"},
			expectError: false,
		},
		{
			name:        "single quoted string",
			input:       "one 'two three' four",
			expected:    []string{"one", "two three", "four"},
			expectError: false,
		},
		{
			name:        "mixed quotes",
			input:       `one "two 'three' four" five`,
			expected:    []string{"one", "two 'three' four", "five"},
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    []string{},
			expectError: false,
		},
		{
			name:        "only whitespace",
			input:       "   ",
			expected:    []string{},
			expectError: false,
		},
		{
			name:        "unclosed double quote",
			input:       `one "two three`,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "unclosed single quote",
			input:       "one 'two three",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "extra whitespace",
			input:       "  one   two  ",
			expected:    []string{"one", "two"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tokenize(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("tokenize(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("tokenize(%q) unexpected error: %v", tt.input, err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("tokenize(%q) length = %d, want %d", tt.input, len(result), len(tt.expected))
				return
			}

			for i, token := range result {
				if token != tt.expected[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, token, tt.expected[i])
				}
			}
		})
	}
}

func TestParsedCommand_String(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *ParsedCommand
		contains []string // Strings that should be present in output
	}{
		{
			name: "simple command",
			cmd: &ParsedCommand{
				Name:       "help",
				Subcommand: "",
				Args:       []string{},
				Flags:      map[string]string{},
			},
			contains: []string{"/help"},
		},
		{
			name: "command with subcommand",
			cmd: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags:      map[string]string{},
			},
			contains: []string{"/agents", "list"},
		},
		{
			name: "command with args",
			cmd: &ParsedCommand{
				Name:       "missions",
				Subcommand: "create",
				Args:       []string{"Test Mission"},
				Flags:      map[string]string{},
			},
			contains: []string{"/missions", "create", "Test Mission"},
		},
		{
			name: "command with flags",
			cmd: &ParsedCommand{
				Name:       "agents",
				Subcommand: "list",
				Args:       []string{},
				Flags: map[string]string{
					"format": "json",
				},
			},
			contains: []string{"/agents", "list", "--format"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cmd.String()
			for _, substr := range tt.contains {
				if !containsString(result, substr) {
					t.Errorf("String() = %q, should contain %q", result, substr)
				}
			}
		})
	}
}

func TestParsedCommand_HasFlag(t *testing.T) {
	cmd := &ParsedCommand{
		Name: "test",
		Flags: map[string]string{
			"verbose": "true",
			"format":  "json",
		},
	}

	if !cmd.HasFlag("verbose") {
		t.Error("HasFlag(verbose) should return true")
	}

	if !cmd.HasFlag("format") {
		t.Error("HasFlag(format) should return true")
	}

	if cmd.HasFlag("nonexistent") {
		t.Error("HasFlag(nonexistent) should return false")
	}
}

func TestParsedCommand_GetFlag(t *testing.T) {
	cmd := &ParsedCommand{
		Name: "test",
		Flags: map[string]string{
			"verbose": "true",
			"format":  "json",
		},
	}

	if value := cmd.GetFlag("format"); value != "json" {
		t.Errorf("GetFlag(format) = %q, want %q", value, "json")
	}

	if value := cmd.GetFlag("nonexistent"); value != "" {
		t.Errorf("GetFlag(nonexistent) = %q, want empty string", value)
	}
}

func TestParsedCommand_GetFlagWithDefault(t *testing.T) {
	cmd := &ParsedCommand{
		Name: "test",
		Flags: map[string]string{
			"format": "json",
		},
	}

	if value := cmd.GetFlagWithDefault("format", "xml"); value != "json" {
		t.Errorf("GetFlagWithDefault(format, xml) = %q, want %q", value, "json")
	}

	if value := cmd.GetFlagWithDefault("nonexistent", "default"); value != "default" {
		t.Errorf("GetFlagWithDefault(nonexistent, default) = %q, want %q", value, "default")
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexString(s, substr) >= 0)
}

func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
