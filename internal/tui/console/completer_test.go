package console

import (
	"reflect"
	"sort"
	"testing"
)

func TestNewCompleter(t *testing.T) {
	registry := NewCommandRegistry()
	completer := NewCompleter(registry)

	if completer == nil {
		t.Fatal("NewCompleter returned nil")
	}
	if completer.registry != registry {
		t.Error("completer registry not set correctly")
	}
}

func TestCompleterComplete(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&SlashCommand{Name: "agents", Description: "Manage agents"})
	registry.Register(&SlashCommand{Name: "alpha", Description: "Alpha command"})
	registry.Register(&SlashCommand{Name: "config", Description: "Show config"})
	registry.Register(&SlashCommand{Name: "clear", Description: "Clear console"})
	registry.Register(&SlashCommand{Name: "status", Description: "Show status"})

	completer := NewCompleter(registry)

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty input returns all commands",
			input: "",
			want:  []string{"/agents", "/alpha", "/clear", "/config", "/status"},
		},
		{
			name:  "just slash returns all commands",
			input: "/",
			want:  []string{"/agents", "/alpha", "/clear", "/config", "/status"},
		},
		{
			name:  "prefix matches multiple commands",
			input: "/a",
			want:  []string{"/agents", "/alpha"},
		},
		{
			name:  "prefix matches single command",
			input: "/ag",
			want:  []string{"/agents"},
		},
		{
			name:  "prefix matches no commands",
			input: "/xyz",
			want:  []string{},
		},
		{
			name:  "prefix matches commands starting with c",
			input: "/c",
			want:  []string{"/clear", "/config"},
		},
		{
			name:  "full command name",
			input: "/status",
			want:  []string{"/status"},
		},
		{
			name:  "input with whitespace",
			input: "  /a  ",
			want:  []string{"/agents", "/alpha"},
		},
		{
			name:  "case insensitive matching",
			input: "/A",
			want:  []string{"/agents", "/alpha"},
		},
		{
			name:  "non-slash input returns nothing",
			input: "agents",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completer.Complete(tt.input)

			// Sort for consistent comparison
			sort.Strings(got)
			sort.Strings(tt.want)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Complete(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompleterGetSuggestions(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&SlashCommand{
		Name:        "agents",
		Description: "Manage agents",
	})
	registry.Register(&SlashCommand{
		Name:        "alpha",
		Description: "Alpha command",
	})
	registry.Register(&SlashCommand{
		Name:        "config",
		Description: "Show config",
	})
	registry.Register(&SlashCommand{
		Name:        "clear",
		Description: "Clear console",
	})

	completer := NewCompleter(registry)

	tests := []struct {
		name  string
		input string
		want  []Suggestion
	}{
		{
			name:  "empty input returns all suggestions",
			input: "",
			want: []Suggestion{
				{Text: "/agents", Description: "Manage agents"},
				{Text: "/alpha", Description: "Alpha command"},
				{Text: "/clear", Description: "Clear console"},
				{Text: "/config", Description: "Show config"},
			},
		},
		{
			name:  "just slash returns all suggestions",
			input: "/",
			want: []Suggestion{
				{Text: "/agents", Description: "Manage agents"},
				{Text: "/alpha", Description: "Alpha command"},
				{Text: "/clear", Description: "Clear console"},
				{Text: "/config", Description: "Show config"},
			},
		},
		{
			name:  "prefix matches multiple suggestions",
			input: "/a",
			want: []Suggestion{
				{Text: "/agents", Description: "Manage agents"},
				{Text: "/alpha", Description: "Alpha command"},
			},
		},
		{
			name:  "prefix matches single suggestion",
			input: "/ag",
			want: []Suggestion{
				{Text: "/agents", Description: "Manage agents"},
			},
		},
		{
			name:  "prefix matches no suggestions",
			input: "/xyz",
			want:  nil, // GetSuggestions may return nil for no matches
		},
		{
			name:  "prefix matches commands starting with c",
			input: "/c",
			want: []Suggestion{
				{Text: "/clear", Description: "Clear console"},
				{Text: "/config", Description: "Show config"},
			},
		},
		{
			name:  "full command name",
			input: "/clear",
			want: []Suggestion{
				{Text: "/clear", Description: "Clear console"},
			},
		},
		{
			name:  "input with whitespace",
			input: "  /a  ",
			want: []Suggestion{
				{Text: "/agents", Description: "Manage agents"},
				{Text: "/alpha", Description: "Alpha command"},
			},
		},
		{
			name:  "case insensitive matching",
			input: "/A",
			want: []Suggestion{
				{Text: "/agents", Description: "Manage agents"},
				{Text: "/alpha", Description: "Alpha command"},
			},
		},
		{
			name:  "non-slash input returns empty",
			input: "agents",
			want:  nil, // GetSuggestions may return nil for no matches
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completer.GetSuggestions(tt.input)

			// Handle nil vs empty slice equivalence
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}

			// Sort for consistent comparison
			if got != nil {
				sortSuggestions(got)
			}
			if tt.want != nil {
				sortSuggestions(tt.want)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSuggestions(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompleterWithAliases(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&SlashCommand{
		Name:        "agents",
		Aliases:     []string{"a", "ag"},
		Description: "Manage agents",
	})
	registry.Register(&SlashCommand{
		Name:        "status",
		Aliases:     []string{"s", "stat"},
		Description: "Show status",
	})

	completer := NewCompleter(registry)

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "prefix matches command and aliases",
			input: "/a",
			want:  []string{"/agents"},
		},
		{
			name:  "prefix matches alias",
			input: "/s",
			want:  []string{"/status"},
		},
		{
			name:  "all commands including primary names only",
			input: "/",
			want:  []string{"/agents", "/status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := completer.Complete(tt.input)

			// Sort for consistent comparison
			sort.Strings(got)
			sort.Strings(tt.want)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Complete(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompleterEmptyRegistry(t *testing.T) {
	registry := NewCommandRegistry()
	completer := NewCompleter(registry)

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty input", input: ""},
		{name: "slash input", input: "/"},
		{name: "prefix input", input: "/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := completer.Complete(tt.input)
			if len(results) != 0 {
				t.Errorf("Complete(%q) on empty registry = %v, want []", tt.input, results)
			}

			suggestions := completer.GetSuggestions(tt.input)
			if len(suggestions) != 0 {
				t.Errorf("GetSuggestions(%q) on empty registry = %v, want []", tt.input, suggestions)
			}
		})
	}
}

func TestCompleterSorted(t *testing.T) {
	registry := NewCommandRegistry()
	// Register in non-alphabetical order
	registry.Register(&SlashCommand{Name: "zebra", Description: "Z command"})
	registry.Register(&SlashCommand{Name: "alpha", Description: "A command"})
	registry.Register(&SlashCommand{Name: "beta", Description: "B command"})
	registry.Register(&SlashCommand{Name: "gamma", Description: "G command"})

	completer := NewCompleter(registry)

	results := completer.Complete("/")
	expected := []string{"/alpha", "/beta", "/gamma", "/zebra"}

	if !reflect.DeepEqual(results, expected) {
		t.Errorf("Complete('/') = %v, want %v (sorted)", results, expected)
	}

	suggestions := completer.GetSuggestions("/")
	for i := 1; i < len(suggestions); i++ {
		if suggestions[i-1].Text > suggestions[i].Text {
			t.Errorf("suggestions not sorted: %q > %q", suggestions[i-1].Text, suggestions[i].Text)
		}
	}
}

func TestCompleterPartialMatch(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&SlashCommand{Name: "configuration", Description: "Full config"})
	registry.Register(&SlashCommand{Name: "config", Description: "Short config"})
	registry.Register(&SlashCommand{Name: "conf", Description: "Shorter config"})

	completer := NewCompleter(registry)

	tests := []struct {
		input string
		want  int
	}{
		{input: "/con", want: 3}, // All three match
		{input: "/conf", want: 3}, // conf, config, and configuration
		{input: "/confi", want: 2}, // config and configuration
		{input: "/config", want: 2}, // config and configuration
		{input: "/configu", want: 1}, // only configuration
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			results := completer.Complete(tt.input)
			if len(results) != tt.want {
				t.Errorf("Complete(%q) matched %d commands, want %d", tt.input, len(results), tt.want)
			}
		})
	}
}

func TestCompleterCaseInsensitive(t *testing.T) {
	registry := NewCommandRegistry()
	registry.Register(&SlashCommand{Name: "agents", Description: "Manage agents"})
	registry.Register(&SlashCommand{Name: "Alpha", Description: "Alpha command"})

	completer := NewCompleter(registry)

	tests := []struct {
		input string
		want  []string
	}{
		{
			input: "/a",
			want:  []string{"/Alpha", "/agents"},
		},
		{
			input: "/A",
			want:  []string{"/Alpha", "/agents"},
		},
		{
			input: "/ag",
			want:  []string{"/agents"},
		},
		{
			input: "/AG",
			want:  []string{"/agents"},
		},
		{
			input: "/al",
			want:  []string{"/Alpha"},
		},
		{
			input: "/AL",
			want:  []string{"/Alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := completer.Complete(tt.input)
			sort.Strings(got)
			sort.Strings(tt.want)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Complete(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompleterWithDefaultCommands(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)
	completer := NewCompleter(registry)

	// Test that we can complete default commands
	allCommands := completer.Complete("/")
	if len(allCommands) == 0 {
		t.Error("no commands returned for default registry")
	}

	// Test specific completions
	agentCompletions := completer.Complete("/ag")
	if len(agentCompletions) != 1 || agentCompletions[0] != "/agents" {
		t.Errorf("Complete('/ag') = %v, want ['/agents']", agentCompletions)
	}

	// Test that descriptions are present
	suggestions := completer.GetSuggestions("/agents")
	if len(suggestions) != 1 {
		t.Fatalf("GetSuggestions('/agents') returned %d suggestions, want 1", len(suggestions))
	}
	if suggestions[0].Description == "" {
		t.Error("agent command has empty description")
	}
}

// Helper function to sort suggestions for comparison
func sortSuggestions(suggestions []Suggestion) {
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})
}
