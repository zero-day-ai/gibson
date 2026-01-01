package console

import (
	"errors"
	"strings"
	"unicode"
)

var (
	// ErrEmptyInput is returned when the input string is empty or only whitespace
	ErrEmptyInput = errors.New("empty input")

	// ErrNotSlashCommand is returned when the input doesn't start with "/"
	ErrNotSlashCommand = errors.New("not a slash command")

	// ErrInvalidCommand is returned when the command structure is invalid
	ErrInvalidCommand = errors.New("invalid command structure")
)

// IsSlashCommand returns true if the input starts with a "/" character.
// Whitespace is trimmed before checking.
func IsSlashCommand(input string) bool {
	trimmed := strings.TrimSpace(input)
	return len(trimmed) > 0 && trimmed[0] == '/'
}

// Parse parses a slash command input string into a ParsedCommand structure.
//
// Parsing rules:
//   - Input must start with "/" (after trimming whitespace)
//   - First word after "/" becomes the command Name
//   - Second word becomes Subcommand (if it doesn't start with "-")
//   - Remaining words are parsed as Args or Flags
//   - Flags can be:
//   - --key=value (combined form)
//   - --key value (separated form, value consumed as flag value)
//   - --key (boolean flag, value set to "true")
//   - Quoted strings are treated as single arguments
//   - Both single and double quotes are supported
//
// Examples:
//   - "/agents" -> Name="agents", Subcommand="", Args=[], Flags={}
//   - "/agents list" -> Name="agents", Subcommand="list", Args=[], Flags={}
//   - "/missions create 'Test Mission'" -> Name="missions", Subcommand="create", Args=["Test Mission"], Flags={}
//   - "/config --verbose" -> Name="config", Subcommand="", Args=[], Flags={"verbose":"true"}
//   - "/agents list --format json" -> Name="agents", Subcommand="list", Args=[], Flags={"format":"json"}
func Parse(input string) (*ParsedCommand, error) {
	// Trim whitespace
	trimmed := strings.TrimSpace(input)

	// Check for empty input
	if len(trimmed) == 0 {
		return nil, ErrEmptyInput
	}

	// Check for slash command prefix
	if trimmed[0] != '/' {
		return nil, ErrNotSlashCommand
	}

	// Remove the leading slash
	commandText := strings.TrimSpace(trimmed[1:])
	if len(commandText) == 0 {
		return nil, ErrInvalidCommand
	}

	// Tokenize the input (handles quoted strings)
	tokens, err := tokenize(commandText)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, ErrInvalidCommand
	}

	cmd := &ParsedCommand{
		Name:  tokens[0],
		Args:  []string{},
		Flags: make(map[string]string),
	}

	// Parse remaining tokens
	i := 1
	foundSubcommand := false

	for i < len(tokens) {
		token := tokens[i]

		// Check if this is a flag
		if strings.HasPrefix(token, "--") {
			key, value, consumed := parseFlag(token, tokens, i)
			cmd.Flags[key] = value
			i += consumed
			continue
		}

		// Check if this is a short flag
		if strings.HasPrefix(token, "-") && len(token) > 1 {
			// Treat short flags as boolean flags
			key := strings.TrimPrefix(token, "-")
			cmd.Flags[key] = "true"
			i++
			continue
		}

		// If we haven't found a subcommand yet, this is it
		if !foundSubcommand {
			cmd.Subcommand = token
			foundSubcommand = true
			i++
			continue
		}

		// Otherwise, it's a positional argument
		cmd.Args = append(cmd.Args, token)
		i++
	}

	return cmd, nil
}

// tokenize splits the input into tokens, respecting quoted strings.
// Both single quotes (') and double quotes (") are supported.
func tokenize(input string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inQuotes := false
	quoteChar := rune(0)

	for i, r := range input {
		switch {
		case r == '"' || r == '\'':
			if !inQuotes {
				// Start of quoted string
				inQuotes = true
				quoteChar = r
			} else if r == quoteChar {
				// End of quoted string
				inQuotes = false
				quoteChar = 0
			} else {
				// Different quote inside quoted string
				current.WriteRune(r)
			}

		case unicode.IsSpace(r):
			if inQuotes {
				// Space inside quoted string
				current.WriteRune(r)
			} else if current.Len() > 0 {
				// End of token
				tokens = append(tokens, current.String())
				current.Reset()
			}

		default:
			current.WriteRune(r)
		}

		// Check for unclosed quotes at end of input
		if i == len(input)-1 && inQuotes {
			return nil, errors.New("unclosed quote")
		}
	}

	// Add final token if exists
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}

// parseFlag parses a flag from the token stream.
// Returns the flag key, value, and number of tokens consumed.
//
// Supports three formats:
//   - --key=value (returns key, value, 1)
//   - --key value (returns key, value, 2)
//   - --key (returns key, "true", 1)
func parseFlag(token string, tokens []string, index int) (key, value string, consumed int) {
	// Remove the "--" prefix
	flagText := strings.TrimPrefix(token, "--")

	// Check for --key=value format
	if strings.Contains(flagText, "=") {
		parts := strings.SplitN(flagText, "=", 2)
		return parts[0], parts[1], 1
	}

	// Check if there's a next token for --key value format
	if index+1 < len(tokens) {
		nextToken := tokens[index+1]
		// If next token is not a flag, use it as the value
		if !strings.HasPrefix(nextToken, "-") {
			return flagText, nextToken, 2
		}
	}

	// Boolean flag format: --key
	return flagText, "true", 1
}

// String returns a string representation of the parsed command.
func (c *ParsedCommand) String() string {
	var sb strings.Builder
	sb.WriteRune('/')
	sb.WriteString(c.Name)

	if c.Subcommand != "" {
		sb.WriteRune(' ')
		sb.WriteString(c.Subcommand)
	}

	for _, arg := range c.Args {
		sb.WriteRune(' ')
		// Quote args that contain spaces
		if strings.Contains(arg, " ") {
			sb.WriteRune('"')
			sb.WriteString(arg)
			sb.WriteRune('"')
		} else {
			sb.WriteString(arg)
		}
	}

	for key, value := range c.Flags {
		sb.WriteRune(' ')
		sb.WriteString("--")
		sb.WriteString(key)
		if value != "true" {
			sb.WriteRune('=')
			sb.WriteString(value)
		}
	}

	return sb.String()
}

// HasFlag returns true if the command has the specified flag.
func (c *ParsedCommand) HasFlag(name string) bool {
	_, ok := c.Flags[name]
	return ok
}

// GetFlag returns the value of a flag, or an empty string if not present.
func (c *ParsedCommand) GetFlag(name string) string {
	return c.Flags[name]
}

// GetFlagWithDefault returns the value of a flag, or the default value if not present.
func (c *ParsedCommand) GetFlagWithDefault(name, defaultValue string) string {
	if value, ok := c.Flags[name]; ok {
		return value
	}
	return defaultValue
}
