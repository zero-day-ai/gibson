package console

import (
	"sort"
	"strings"
)

// Suggestion represents a command completion suggestion with descriptive text.
type Suggestion struct {
	Text        string // The completion text (command name with "/" prefix)
	Description string // Help text describing what the command does
}

// Completer provides tab completion functionality for console commands.
// It uses the CommandRegistry to generate completion suggestions based on
// partial user input.
type Completer struct {
	registry *CommandRegistry
}

// NewCompleter creates a new Completer that uses the provided CommandRegistry
// to generate command completions.
func NewCompleter(registry *CommandRegistry) *Completer {
	return &Completer{
		registry: registry,
	}
}

// Complete returns a sorted list of matching command names for tab completion.
// The returned command names include the "/" prefix.
//
// Behavior:
//   - If input is empty or just "/", returns all available commands
//   - If input starts with "/", matches against command names by prefix
//   - Matches are case-insensitive
//   - Results are sorted alphabetically
//
// Examples:
//   - Complete("") -> ["/agents", "/clear", "/config", ...]
//   - Complete("/") -> ["/agents", "/clear", "/config", ...]
//   - Complete("/a") -> ["/agents"]
//   - Complete("/c") -> ["/clear", "/config", "/credentials"]
//   - Complete("/miss") -> ["/missions"]
func (c *Completer) Complete(input string) []string {
	suggestions := c.GetSuggestions(input)
	result := make([]string, len(suggestions))
	for i, s := range suggestions {
		result[i] = s.Text
	}
	return result
}

// GetSuggestions returns a sorted list of matching command suggestions with descriptions.
// Each suggestion includes the command name and its description from the registry.
//
// Behavior:
//   - If input is empty or just "/", returns all available commands
//   - If input starts with "/", matches against command names by prefix
//   - Matches are case-insensitive
//   - Results are sorted alphabetically by command name
//
// Examples:
//   - GetSuggestions("") -> All commands with descriptions
//   - GetSuggestions("/") -> All commands with descriptions
//   - GetSuggestions("/a") -> [{Text: "/agents", Description: "..."}]
//   - GetSuggestions("/c") -> Multiple suggestions for commands starting with "c"
func (c *Completer) GetSuggestions(input string) []Suggestion {
	var suggestions []Suggestion

	// Normalize input
	input = strings.TrimSpace(input)

	// If input is empty or just "/", return all commands
	if input == "" || input == "/" {
		commands := c.registry.List()
		suggestions = make([]Suggestion, 0, len(commands))
		for _, cmd := range commands {
			suggestions = append(suggestions, Suggestion{
				Text:        "/" + cmd.Name,
				Description: cmd.Description,
			})
		}
	} else if strings.HasPrefix(input, "/") {
		// Match against command names
		prefix := strings.ToLower(input[1:]) // Remove "/" and convert to lowercase
		commands := c.registry.List()

		for _, cmd := range commands {
			if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
				suggestions = append(suggestions, Suggestion{
					Text:        "/" + cmd.Name,
					Description: cmd.Description,
				})
			}
		}
	}

	// Sort suggestions alphabetically by command name
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})

	return suggestions
}
