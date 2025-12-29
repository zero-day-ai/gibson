package prompt

import (
	"testing"
	"time"
)

// Test String Functions

func TestToUpper(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "hello", "HELLO"},
		{"mixed case", "Hello World", "HELLO WORLD"},
		{"already upper", "HELLO", "HELLO"},
		{"empty string", "", ""},
		{"with numbers", "hello123", "HELLO123"},
		{"special chars", "hello!@#", "HELLO!@#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toUpper(tt.input)
			if result != tt.expected {
				t.Errorf("toUpper(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToLower(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"uppercase", "HELLO", "hello"},
		{"mixed case", "Hello World", "hello world"},
		{"already lower", "hello", "hello"},
		{"empty string", "", ""},
		{"with numbers", "HELLO123", "hello123"},
		{"special chars", "HELLO!@#", "hello!@#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toLower(tt.input)
			if result != tt.expected {
				t.Errorf("toLower(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase words", "hello world", "Hello World"},
		{"single word", "hello", "Hello"},
		{"already titled", "Hello World", "Hello World"},
		{"all uppercase", "HELLO WORLD", "HELLO WORLD"},
		{"empty string", "", ""},
		{"multiple spaces", "hello  world", "Hello World"}, // Fields normalizes spaces
		{"with punctuation", "hello, world!", "Hello, World!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := title(tt.input)
			if result != tt.expected {
				t.Errorf("title(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"leading spaces", "  hello", "hello"},
		{"trailing spaces", "hello  ", "hello"},
		{"both sides", "  hello  ", "hello"},
		{"no spaces", "hello", "hello"},
		{"empty string", "", ""},
		{"only spaces", "   ", ""},
		{"tabs and newlines", "\t\nhello\n\t", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trim(tt.input)
			if result != tt.expected {
				t.Errorf("trim(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTrimPrefix(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		input    string
		expected string
	}{
		{"has prefix", "http://", "http://example.com", "example.com"},
		{"no prefix", "http://", "example.com", "example.com"},
		{"empty prefix", "", "hello", "hello"},
		{"empty input", "hello", "", ""},
		{"prefix equals input", "hello", "hello", ""},
		{"case sensitive", "Hello", "hello world", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimPrefix(tt.prefix, tt.input)
			if result != tt.expected {
				t.Errorf("trimPrefix(%q, %q) = %q, want %q", tt.prefix, tt.input, result, tt.expected)
			}
		})
	}
}

func TestTrimSuffix(t *testing.T) {
	tests := []struct {
		name     string
		suffix   string
		input    string
		expected string
	}{
		{"has suffix", ".txt", "file.txt", "file"},
		{"no suffix", ".txt", "file.doc", "file.doc"},
		{"empty suffix", "", "hello", "hello"},
		{"empty input", "hello", "", ""},
		{"suffix equals input", "hello", "hello", ""},
		{"case sensitive", "TXT", "file.txt", "file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimSuffix(tt.suffix, tt.input)
			if result != tt.expected {
				t.Errorf("trimSuffix(%q, %q) = %q, want %q", tt.suffix, tt.input, result, tt.expected)
			}
		})
	}
}

func TestReplace(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		new      string
		input    string
		expected string
	}{
		{"single replacement", "o", "0", "hello", "hell0"},
		{"multiple replacements", "l", "L", "hello", "heLLo"},
		{"no matches", "x", "y", "hello", "hello"},
		{"empty new", "l", "", "hello", "heo"},
		{"empty input", "o", "0", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replace(tt.old, tt.new, tt.input)
			if result != tt.expected {
				t.Errorf("replace(%q, %q, %q) = %q, want %q", tt.old, tt.new, tt.input, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"contains", "hello world", "world", true},
		{"not contains", "hello world", "goodbye", false},
		{"empty substring", "hello", "", true},
		{"empty string", "", "hello", false},
		{"both empty", "", "", true},
		{"case sensitive", "Hello", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		prefix   string
		expected bool
	}{
		{"has prefix", "hello world", "hello", true},
		{"no prefix", "hello world", "world", false},
		{"empty prefix", "hello", "", true},
		{"empty string", "", "hello", false},
		{"both empty", "", "", true},
		{"case sensitive", "Hello", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasPrefix(tt.s, tt.prefix)
			if result != tt.expected {
				t.Errorf("hasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestHasSuffix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		suffix   string
		expected bool
	}{
		{"has suffix", "hello world", "world", true},
		{"no suffix", "hello world", "hello", false},
		{"empty suffix", "hello", "", true},
		{"empty string", "", "hello", false},
		{"both empty", "", "", true},
		{"case sensitive", "World", "world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasSuffix(tt.s, tt.suffix)
			if result != tt.expected {
				t.Errorf("hasSuffix(%q, %q) = %v, want %v", tt.s, tt.suffix, result, tt.expected)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name     string
		sep      string
		s        string
		expected []string
	}{
		{"comma separated", ",", "a,b,c", []string{"a", "b", "c"}},
		{"space separated", " ", "hello world", []string{"hello", "world"}},
		{"no separator", ",", "hello", []string{"hello"}},
		{"empty string", ",", "", []string{""}},
		{"multiple consecutive seps", ",", "a,,b", []string{"a", "", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := split(tt.sep, tt.s)
			if len(result) != len(tt.expected) {
				t.Errorf("split(%q, %q) returned %d items, want %d", tt.sep, tt.s, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("split(%q, %q)[%d] = %q, want %q", tt.sep, tt.s, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		name     string
		sep      string
		items    []string
		expected string
	}{
		{"comma join", ",", []string{"a", "b", "c"}, "a,b,c"},
		{"space join", " ", []string{"hello", "world"}, "hello world"},
		{"empty separator", "", []string{"a", "b"}, "ab"},
		{"single item", ",", []string{"hello"}, "hello"},
		{"empty slice", ",", []string{}, ""},
		{"with empty strings", ",", []string{"a", "", "b"}, "a,,b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := join(tt.sep, tt.items)
			if result != tt.expected {
				t.Errorf("join(%q, %v) = %q, want %q", tt.sep, tt.items, result, tt.expected)
			}
		})
	}
}

// Test Utility Functions

func TestDefaultFunc(t *testing.T) {
	tests := []struct {
		name     string
		def      any
		val      any
		expected any
	}{
		{"non-empty string", "default", "value", "value"},
		{"empty string", "default", "", "default"},
		{"non-zero int", 10, 5, 5},
		{"zero int", 10, 0, 10},
		{"true bool", false, true, true},
		{"false bool", true, false, true},
		{"nil value", "default", nil, "default"},
		{"non-nil slice", "default", []any{1, 2}, []any{1, 2}},
		{"empty slice", "default", []any{}, "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaultFunc(tt.def, tt.val)
			// For slices, we need special comparison
			if resultSlice, ok := result.([]any); ok {
				expectedSlice := tt.expected.([]any)
				if len(resultSlice) != len(expectedSlice) {
					t.Errorf("defaultFunc(%v, %v) = %v, want %v", tt.def, tt.val, result, tt.expected)
				}
			} else if result != tt.expected {
				t.Errorf("defaultFunc(%v, %v) = %v, want %v", tt.def, tt.val, result, tt.expected)
			}
		})
	}
}

func TestRequired(t *testing.T) {
	tests := []struct {
		name      string
		val       any
		shouldErr bool
	}{
		{"non-empty string", "value", false},
		{"empty string", "", true},
		{"non-zero int", 5, false},
		{"zero int", 0, true},
		{"true bool", true, false},
		{"false bool", false, true},
		{"nil value", nil, true},
		{"non-empty slice", []any{1}, false},
		{"empty slice", []any{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := required(tt.val)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("required(%v) expected error but got none", tt.val)
				}
			} else {
				if err != nil {
					t.Errorf("required(%v) unexpected error: %v", tt.val, err)
				}
				// Don't compare slices directly as they are not comparable
				// Just verify the result is not nil for non-error cases
				if result == nil {
					t.Errorf("required(%v) returned nil but should return the value", tt.val)
				}
			}
		})
	}
}

func TestCoalesce(t *testing.T) {
	tests := []struct {
		name     string
		vals     []any
		expected any
	}{
		{"first non-empty", []any{"", "first", "second"}, "first"},
		{"all empty strings", []any{"", "", ""}, nil},
		{"mixed types", []any{0, false, "", "value"}, "value"},
		{"first is valid", []any{"first", "", ""}, "first"},
		{"no values", []any{}, nil},
		{"nil values", []any{nil, nil, "value"}, "value"},
		{"all nil", []any{nil, nil, nil}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := coalesce(tt.vals...)
			if result != tt.expected {
				t.Errorf("coalesce(%v) = %v, want %v", tt.vals, result, tt.expected)
			}
		})
	}
}

func TestTernary(t *testing.T) {
	tests := []struct {
		name     string
		cond     bool
		trueVal  any
		falseVal any
		expected any
	}{
		{"true condition", true, "yes", "no", "yes"},
		{"false condition", false, "yes", "no", "no"},
		{"with numbers", true, 1, 0, 1},
		{"with nil", false, "value", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ternary(tt.cond, tt.trueVal, tt.falseVal)
			if result != tt.expected {
				t.Errorf("ternary(%v, %v, %v) = %v, want %v", tt.cond, tt.trueVal, tt.falseVal, result, tt.expected)
			}
		})
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		val      any
		expected bool
	}{
		{"empty string", "", true},
		{"non-empty string", "hello", false},
		{"zero int", 0, true},
		{"non-zero int", 5, false},
		{"false bool", false, true},
		{"true bool", true, false},
		{"nil", nil, true},
		{"empty slice", []any{}, true},
		{"non-empty slice", []any{1}, false},
		{"empty map", map[string]any{}, true},
		{"non-empty map", map[string]any{"key": "value"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmpty(tt.val)
			if result != tt.expected {
				t.Errorf("isEmpty(%v) = %v, want %v", tt.val, result, tt.expected)
			}
		})
	}
}

// Test Date/Time Functions

func TestNow(t *testing.T) {
	before := time.Now()
	result := now()
	after := time.Now()

	if result.Before(before) || result.After(after) {
		t.Errorf("now() returned time outside expected range")
	}
}

func TestFormatDate(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{"date only", "2006-01-02", "2024-01-15"},
		{"datetime", "2006-01-02 15:04:05", "2024-01-15 14:30:45"},
		{"custom format", "Jan 2, 2006", "Jan 15, 2024"},
		{"time only", "15:04", "14:30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDate(tt.format, testTime)
			if result != tt.expected {
				t.Errorf("formatDate(%q, %v) = %q, want %q", tt.format, testTime, result, tt.expected)
			}
		})
	}
}

// Test JSON Functions

func TestToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"simple map", map[string]any{"key": "value"}, `{"key":"value"}`},
		{"number", 42, "42"},
		{"string", "hello", `"hello"`},
		{"boolean", true, "true"},
		{"slice", []int{1, 2, 3}, "[1,2,3]"},
		{"nil", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toJSON(tt.input)
			if result != tt.expected {
				t.Errorf("toJSON(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToPrettyJSON(t *testing.T) {
	input := map[string]any{
		"name": "Gibson",
		"age":  1,
	}

	result := toPrettyJSON(input)

	// Check that it contains newlines and indentation
	if !contains(result, "\n") || !contains(result, "  ") {
		t.Errorf("toPrettyJSON() should produce indented output, got: %q", result)
	}

	// Check that it contains the expected keys
	if !contains(result, "name") || !contains(result, "age") {
		t.Errorf("toPrettyJSON() missing expected keys, got: %q", result)
	}
}

func TestFromJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(any) bool
	}{
		{
			"simple object",
			`{"key":"value"}`,
			func(v any) bool {
				m, ok := v.(map[string]any)
				return ok && m["key"] == "value"
			},
		},
		{
			"number",
			"42",
			func(v any) bool {
				// JSON numbers are decoded as float64
				f, ok := v.(float64)
				return ok && f == 42
			},
		},
		{
			"array",
			"[1,2,3]",
			func(v any) bool {
				arr, ok := v.([]any)
				return ok && len(arr) == 3
			},
		},
		{
			"invalid json",
			"{invalid}",
			func(v any) bool {
				return v == nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromJSON(tt.input)
			if !tt.validate(result) {
				t.Errorf("fromJSON(%q) validation failed, got: %v", tt.input, result)
			}
		})
	}
}

// Test Formatting Functions

func TestIndent(t *testing.T) {
	tests := []struct {
		name     string
		spaces   int
		input    string
		expected string
	}{
		{"single line", 2, "hello", "  hello"},
		{"multiple lines", 2, "hello\nworld", "  hello\n  world"},
		{"empty string", 2, "", ""},
		{"zero spaces", 0, "hello", "hello"},
		{"empty lines preserved", 2, "hello\n\nworld", "  hello\n\n  world"},
		{"four spaces", 4, "code", "    code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := indent(tt.spaces, tt.input)
			if result != tt.expected {
				t.Errorf("indent(%d, %q) = %q, want %q", tt.spaces, tt.input, result, tt.expected)
			}
		})
	}
}

func TestNindent(t *testing.T) {
	tests := []struct {
		name     string
		spaces   int
		input    string
		expected string
	}{
		{"single line", 2, "hello", "\n  hello"},
		{"multiple lines", 2, "hello\nworld", "\n  hello\n  world"},
		{"empty string", 2, "", ""},
		{"zero spaces", 0, "hello", "\nhello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nindent(tt.spaces, tt.input)
			if result != tt.expected {
				t.Errorf("nindent(%d, %q) = %q, want %q", tt.spaces, tt.input, result, tt.expected)
			}
		})
	}
}

func TestQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple string", "hello", `"hello"`},
		{"with quotes", `say "hello"`, `"say \"hello\""`},
		{"empty string", "", `""`},
		{"with newline", "hello\nworld", `"hello\nworld"`},
		{"with backslash", `hello\world`, `"hello\\world"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := quote(tt.input)
			if result != tt.expected {
				t.Errorf("quote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSquote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple string", "hello", "'hello'"},
		{"with quotes", "say 'hello'", "'say 'hello''"},
		{"empty string", "", "''"},
		{"with double quotes", `say "hello"`, `'say "hello"'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := squote(tt.input)
			if result != tt.expected {
				t.Errorf("squote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Test DefaultFuncMap

func TestDefaultFuncMap(t *testing.T) {
	funcMap := DefaultFuncMap()

	// Check that all expected functions are present
	expectedFuncs := []string{
		// String functions
		"toUpper", "toLower", "title", "trim", "trimPrefix", "trimSuffix",
		"replace", "contains", "hasPrefix", "hasSuffix", "split", "join",
		// Utility functions
		"default", "required", "coalesce", "ternary",
		// Date/time functions
		"now", "formatDate",
		// JSON functions
		"toJSON", "toPrettyJSON", "fromJSON",
		// Formatting functions
		"indent", "nindent", "quote", "squote",
	}

	for _, funcName := range expectedFuncs {
		if _, exists := funcMap[funcName]; !exists {
			t.Errorf("DefaultFuncMap() missing expected function: %s", funcName)
		}
	}

	// Verify the count matches
	if len(funcMap) != len(expectedFuncs) {
		t.Errorf("DefaultFuncMap() has %d functions, expected %d", len(funcMap), len(expectedFuncs))
	}
}
