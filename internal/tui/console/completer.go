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
//   - Supports subcommand completion for commands that have subcommands
//   - Matches are case-insensitive
//   - Results are sorted alphabetically by command name
//
// Examples:
//   - GetSuggestions("") -> All commands with descriptions
//   - GetSuggestions("/") -> All commands with descriptions
//   - GetSuggestions("/a") -> [{Text: "/agents", Description: "..."}]
//   - GetSuggestions("/mission ") -> All mission subcommands
//   - GetSuggestions("/mission r") -> [{Text: "run", Description: "..."}, {Text: "resume", Description: "..."}]
func (c *Completer) GetSuggestions(input string) []Suggestion {
	// Trim only leading space to preserve trailing space detection
	input = strings.TrimLeft(input, " \t")

	// Check for trailing space after trimming leading whitespace
	hasTrailingSpace := input != "" && strings.HasSuffix(input, " ")

	// Trim trailing space for parsing
	input = strings.TrimRight(input, " \t")

	// Only complete input that starts with "/" or is empty
	if input != "" && !strings.HasPrefix(input, "/") {
		return nil
	}

	// Parse input into parts
	parts := strings.Fields(strings.TrimPrefix(input, "/"))

	// Case 1: Empty input or just "/" - return all top-level commands
	if len(parts) == 0 {
		return c.completeTopLevel("")
	}

	// Case 2: Single word - check if it's a complete command or needs completion
	if len(parts) == 1 {
		// Try to get the command to see if it exists
		cmd, exists := c.registry.Get(parts[0])

		// If command doesn't exist OR no trailing space, complete command names
		if !exists || !hasTrailingSpace {
			return c.completeTopLevel(parts[0])
		}

		// Command exists with trailing space - check for subcommands
		if cmd.Subcommands == nil || len(cmd.Subcommands) == 0 {
			// No subcommands available
			return nil
		}

		// Show all subcommands
		return c.completeSubcommand(cmd, "")
	}

	// Case 3+: Two or more words - attempting subcommand completion
	// Get the command (resolve aliases to primary name)
	cmd, ok := c.registry.Get(parts[0])
	if !ok || cmd.Subcommands == nil || len(cmd.Subcommands) == 0 {
		// Command doesn't exist or has no subcommands
		return nil
	}

	// Case 3: Typing subcommand - filter by prefix
	if len(parts) == 2 && !hasTrailingSpace {
		return c.completeSubcommand(cmd, parts[1])
	}

	// Case 4+: More than two parts or subcommand with trailing space - no completion for arguments
	return nil
}

// completeTopLevel returns suggestions for top-level commands matching the given prefix.
// If prefix is empty, returns all commands.
func (c *Completer) completeTopLevel(prefix string) []Suggestion {
	var suggestions []Suggestion
	commands := c.registry.List()

	prefix = strings.ToLower(prefix)

	for _, cmd := range commands {
		if prefix == "" || strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			suggestions = append(suggestions, Suggestion{
				Text:        "/" + cmd.Name,
				Description: cmd.Description,
			})
		}
	}

	// Sort suggestions alphabetically by command name
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})

	return suggestions
}

// completeSubcommand returns suggestions for subcommands matching the given prefix.
// If prefix is empty, returns all subcommands.
func (c *Completer) completeSubcommand(cmd *SlashCommand, prefix string) []Suggestion {
	var suggestions []Suggestion

	prefix = strings.ToLower(prefix)

	for _, subcmd := range cmd.Subcommands {
		if prefix == "" || strings.HasPrefix(strings.ToLower(subcmd.Name), prefix) {
			suggestions = append(suggestions, Suggestion{
				Text:        subcmd.Name,
				Description: subcmd.Description,
			})
		}
	}

	// Sort suggestions alphabetically by subcommand name
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})

	return suggestions
}
