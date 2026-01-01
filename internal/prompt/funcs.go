package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"
	"unicode"
)

// DefaultFuncMap returns the default template function map with all custom functions.
// These functions are available in all prompt templates for dynamic content generation.
func DefaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// String functions
		"toUpper":    toUpper,
		"toLower":    toLower,
		"title":      title,
		"trim":       trim,
		"trimPrefix": trimPrefix,
		"trimSuffix": trimSuffix,
		"replace":    replace,
		"contains":   contains,
		"hasPrefix":  hasPrefix,
		"hasSuffix":  hasSuffix,
		"split":      split,
		"join":       join,

		// Utility functions
		"default":  defaultFunc,
		"required": required,
		"coalesce": coalesce,
		"ternary":  ternary,

		// Date/time functions
		"now":        now,
		"formatDate": formatDate,

		// JSON functions
		"toJSON":       toJSON,
		"toPrettyJSON": toPrettyJSON,
		"fromJSON":     fromJSON,

		// Formatting functions
		"indent":  indent,
		"nindent": nindent,
		"quote":   quote,
		"squote":  squote,
	}
}

// String functions

// toUpper converts a string to uppercase.
func toUpper(s string) string {
	return strings.ToUpper(s)
}

// toLower converts a string to lowercase.
func toLower(s string) string {
	return strings.ToLower(s)
}

// title converts a string to title case (first letter of each word capitalized).
func title(s string) string {
	// Convert to title case by capitalizing first letter of each word
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			runes := []rune(word)
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

// trim removes leading and trailing whitespace from a string.
func trim(s string) string {
	return strings.TrimSpace(s)
}

// trimPrefix removes the specified prefix from a string if present.
func trimPrefix(prefix, s string) string {
	return strings.TrimPrefix(s, prefix)
}

// trimSuffix removes the specified suffix from a string if present.
func trimSuffix(suffix, s string) string {
	return strings.TrimSuffix(s, suffix)
}

// replace replaces all occurrences of old with new in the string s.
func replace(old, new, s string) string {
	return strings.ReplaceAll(s, old, new)
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// hasPrefix checks if a string has the specified prefix.
func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

// hasSuffix checks if a string has the specified suffix.
func hasSuffix(s, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

// split splits a string by the separator and returns a slice of strings.
func split(sep, s string) []string {
	return strings.Split(s, sep)
}

// join concatenates a slice of strings with the specified separator.
func join(sep string, items []string) string {
	return strings.Join(items, sep)
}

// Utility functions

// defaultFunc returns val if it's not empty, otherwise returns def.
// Empty values are: nil, "", 0, false, empty slices/maps.
func defaultFunc(def, val any) any {
	if isEmpty(val) {
		return def
	}
	return val
}

// required returns val if it's not nil/empty, otherwise returns an error.
// This is useful for enforcing required template variables.
func required(val any) (any, error) {
	if isEmpty(val) {
		return nil, fmt.Errorf("required value is missing or empty")
	}
	return val, nil
}

// coalesce returns the first non-empty value from the arguments.
// If all values are empty, returns nil.
func coalesce(vals ...any) any {
	for _, val := range vals {
		if !isEmpty(val) {
			return val
		}
	}
	return nil
}

// ternary returns trueVal if cond is true, otherwise returns falseVal.
// This provides conditional value selection in templates.
func ternary(cond bool, trueVal, falseVal any) any {
	if cond {
		return trueVal
	}
	return falseVal
}

// isEmpty checks if a value is empty according to Go semantics.
func isEmpty(val any) bool {
	if val == nil {
		return true
	}

	switch v := val.(type) {
	case string:
		return v == ""
	case bool:
		return !v
	case int, int8, int16, int32, int64:
		return v == 0
	case uint, uint8, uint16, uint32, uint64:
		return v == 0
	case float32, float64:
		return v == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		// For other types, consider them non-empty
		return false
	}
}

// Date/time functions

// now returns the current time.
func now() time.Time {
	return time.Now()
}

// formatDate formats a time value using the specified format string.
// The format string follows Go's time.Format conventions.
// Common formats:
//   - "2006-01-02" for YYYY-MM-DD
//   - "2006-01-02 15:04:05" for YYYY-MM-DD HH:MM:SS
//   - time.RFC3339 for RFC3339 format
func formatDate(format string, t time.Time) string {
	return t.Format(format)
}

// JSON functions

// toJSON marshals a value to JSON string.
// Returns an empty string if marshaling fails.
func toJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

// toPrettyJSON marshals a value to JSON string with indentation.
// Returns an empty string if marshaling fails.
func toPrettyJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

// fromJSON unmarshals a JSON string to a value.
// Returns nil if unmarshaling fails.
func fromJSON(s string) any {
	var result any
	err := json.Unmarshal([]byte(s), &result)
	if err != nil {
		return nil
	}
	return result
}

// Formatting functions

// indent indents each line of the string by the specified number of spaces.
func indent(spaces int, s string) string {
	if s == "" {
		return s
	}

	padding := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		if line != "" {
			lines[i] = padding + line
		}
	}

	return strings.Join(lines, "\n")
}

// nindent adds a newline and then indents each line by the specified number of spaces.
// This is useful for template composition where you want the indented content on a new line.
func nindent(spaces int, s string) string {
	if s == "" {
		return s
	}
	return "\n" + indent(spaces, s)
}

// quote wraps a string in double quotes and escapes internal quotes.
func quote(s string) string {
	return fmt.Sprintf("%q", s)
}

// squote wraps a string in single quotes.
// Note: This doesn't escape internal single quotes.
func squote(s string) string {
	return "'" + s + "'"
}
