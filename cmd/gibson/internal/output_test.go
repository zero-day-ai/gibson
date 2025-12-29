package internal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name           string
		format         OutputFormat
		expectedType   string
		expectText     bool
		expectJSON     bool
	}{
		{
			name:         "text format",
			format:       FormatText,
			expectedType: "*internal.TextFormatter",
			expectText:   true,
			expectJSON:   false,
		},
		{
			name:         "json format",
			format:       FormatJSON,
			expectedType: "*internal.JSONFormatter",
			expectText:   false,
			expectJSON:   true,
		},
		{
			name:         "unknown format defaults to text",
			format:       "unknown",
			expectedType: "*internal.TextFormatter",
			expectText:   true,
			expectJSON:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			formatter := NewFormatter(tt.format, buf)

			if formatter == nil {
				t.Fatal("NewFormatter returned nil")
			}

			_, isText := formatter.(*TextFormatter)
			_, isJSON := formatter.(*JSONFormatter)

			if isText != tt.expectText {
				t.Errorf("expected text formatter=%v, got=%v", tt.expectText, isText)
			}
			if isJSON != tt.expectJSON {
				t.Errorf("expected JSON formatter=%v, got=%v", tt.expectJSON, isJSON)
			}
		})
	}
}

func TestTextFormatter_PrintSuccess(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "simple success message",
			message:  "Operation completed",
			expected: "✓ Operation completed\n",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "✓ \n",
		},
		{
			name:     "message with special characters",
			message:  "User \"admin\" created successfully",
			expected: "✓ User \"admin\" created successfully\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			formatter := NewTextFormatter(buf)

			err := formatter.PrintSuccess(tt.message)
			if err != nil {
				t.Fatalf("PrintSuccess returned error: %v", err)
			}

			if buf.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, buf.String())
			}
		})
	}
}

func TestTextFormatter_PrintError(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "simple error message",
			message:  "Operation failed",
			expected: "✗ Operation failed\n",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "✗ \n",
		},
		{
			name:     "message with newlines",
			message:  "Multiple\nlines\nof\nerror",
			expected: "✗ Multiple\nlines\nof\nerror\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			formatter := NewTextFormatter(buf)

			err := formatter.PrintError(tt.message)
			if err != nil {
				t.Fatalf("PrintError returned error: %v", err)
			}

			if buf.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, buf.String())
			}
		})
	}
}

func TestTextFormatter_PrintTable(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		rows    [][]string
		check   func(t *testing.T, output string)
	}{
		{
			name:    "simple table",
			headers: []string{"Name", "Status", "Port"},
			rows: [][]string{
				{"agent1", "running", "8080"},
				{"agent2", "stopped", "8081"},
			},
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "NAME") {
					t.Error("expected uppercase headers")
				}
				if !strings.Contains(output, "agent1") || !strings.Contains(output, "agent2") {
					t.Error("expected row data in output")
				}
				if !strings.Contains(output, "running") || !strings.Contains(output, "stopped") {
					t.Error("expected status values in output")
				}
			},
		},
		{
			name:    "empty table",
			headers: []string{"Col1", "Col2"},
			rows:    [][]string{},
			check: func(t *testing.T, output string) {
				if !strings.Contains(output, "COL1") || !strings.Contains(output, "COL2") {
					t.Error("expected headers even with empty rows")
				}
			},
		},
		{
			name:    "table with varying row lengths",
			headers: []string{"A", "B", "C"},
			rows: [][]string{
				{"1", "2", "3"},
				{"4", "5"},
				{"6"},
			},
			check: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) < 3 {
					t.Errorf("expected at least 3 lines (headers, separator, rows), got %d", len(lines))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			formatter := NewTextFormatter(buf)

			err := formatter.PrintTable(tt.headers, tt.rows)
			if err != nil {
				t.Fatalf("PrintTable returned error: %v", err)
			}

			tt.check(t, buf.String())
		})
	}
}

func TestTextFormatter_PrintJSON(t *testing.T) {
	tests := []struct {
		name  string
		data  interface{}
		check func(t *testing.T, output string)
	}{
		{
			name: "simple object",
			data: map[string]string{"key": "value"},
			check: func(t *testing.T, output string) {
				var result map[string]string
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to parse JSON: %v", err)
				}
				if result["key"] != "value" {
					t.Errorf("expected key=value, got key=%s", result["key"])
				}
			},
		},
		{
			name: "nested object",
			data: map[string]interface{}{
				"parent": map[string]string{"child": "value"},
			},
			check: func(t *testing.T, output string) {
				var result map[string]interface{}
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to parse JSON: %v", err)
				}
				parent, ok := result["parent"].(map[string]interface{})
				if !ok {
					t.Fatal("expected nested object")
				}
				if parent["child"] != "value" {
					t.Error("expected nested value")
				}
			},
		},
		{
			name: "array",
			data: []string{"a", "b", "c"},
			check: func(t *testing.T, output string) {
				var result []string
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to parse JSON: %v", err)
				}
				if len(result) != 3 {
					t.Errorf("expected 3 elements, got %d", len(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			formatter := NewTextFormatter(buf)

			err := formatter.PrintJSON(tt.data)
			if err != nil {
				t.Fatalf("PrintJSON returned error: %v", err)
			}

			tt.check(t, buf.String())
		})
	}
}

func TestJSONFormatter_PrintSuccess(t *testing.T) {
	buf := &bytes.Buffer{}
	formatter := NewJSONFormatter(buf)

	err := formatter.PrintSuccess("operation successful")
	if err != nil {
		t.Fatalf("PrintSuccess returned error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if result["status"] != "success" {
		t.Errorf("expected status=success, got status=%v", result["status"])
	}
	if result["message"] != "operation successful" {
		t.Errorf("expected message=operation successful, got message=%v", result["message"])
	}
}

func TestJSONFormatter_PrintError(t *testing.T) {
	buf := &bytes.Buffer{}
	formatter := NewJSONFormatter(buf)

	err := formatter.PrintError("operation failed")
	if err != nil {
		t.Fatalf("PrintError returned error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if result["status"] != "error" {
		t.Errorf("expected status=error, got status=%v", result["status"])
	}
	if result["message"] != "operation failed" {
		t.Errorf("expected message=operation failed, got message=%v", result["message"])
	}
}

func TestJSONFormatter_PrintTable(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		rows    [][]string
		check   func(t *testing.T, result map[string]interface{})
	}{
		{
			name:    "simple table",
			headers: []string{"Name", "Status"},
			rows: [][]string{
				{"agent1", "running"},
				{"agent2", "stopped"},
			},
			check: func(t *testing.T, result map[string]interface{}) {
				data, ok := result["data"].([]interface{})
				if !ok {
					t.Fatal("expected data to be array")
				}
				if len(data) != 2 {
					t.Errorf("expected 2 rows, got %d", len(data))
				}

				row1, ok := data[0].(map[string]interface{})
				if !ok {
					t.Fatal("expected row to be object")
				}
				if row1["Name"] != "agent1" {
					t.Errorf("expected Name=agent1, got %v", row1["Name"])
				}
				if row1["Status"] != "running" {
					t.Errorf("expected Status=running, got %v", row1["Status"])
				}
			},
		},
		{
			name:    "empty table",
			headers: []string{"Col1", "Col2"},
			rows:    [][]string{},
			check: func(t *testing.T, result map[string]interface{}) {
				data, ok := result["data"].([]interface{})
				if !ok {
					t.Fatal("expected data to be array")
				}
				if len(data) != 0 {
					t.Errorf("expected 0 rows, got %d", len(data))
				}
			},
		},
		{
			name:    "table with short row",
			headers: []string{"A", "B", "C"},
			rows: [][]string{
				{"1", "2"},
			},
			check: func(t *testing.T, result map[string]interface{}) {
				data, ok := result["data"].([]interface{})
				if !ok {
					t.Fatal("expected data to be array")
				}
				row, ok := data[0].(map[string]interface{})
				if !ok {
					t.Fatal("expected row to be object")
				}
				if row["C"] != "" {
					t.Errorf("expected empty string for missing column, got %v", row["C"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			formatter := NewJSONFormatter(buf)

			err := formatter.PrintTable(tt.headers, tt.rows)
			if err != nil {
				t.Fatalf("PrintTable returned error: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
				t.Fatalf("failed to parse JSON: %v", err)
			}

			tt.check(t, result)
		})
	}
}

func TestJSONFormatter_PrintJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	formatter := NewJSONFormatter(buf)

	data := map[string]interface{}{
		"name":   "test",
		"value":  42,
		"active": true,
	}

	err := formatter.PrintJSON(data)
	if err != nil {
		t.Fatalf("PrintJSON returned error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if result["name"] != "test" {
		t.Errorf("expected name=test, got name=%v", result["name"])
	}
	if result["value"] != float64(42) {
		t.Errorf("expected value=42, got value=%v", result["value"])
	}
	if result["active"] != true {
		t.Errorf("expected active=true, got active=%v", result["active"])
	}
}

func TestFormatter_NilWriter(t *testing.T) {
	// Test that formatters handle nil writer gracefully by using stdout
	textFormatter := NewTextFormatter(nil)
	if textFormatter == nil {
		t.Error("NewTextFormatter with nil writer returned nil")
	}

	jsonFormatter := NewJSONFormatter(nil)
	if jsonFormatter == nil {
		t.Error("NewJSONFormatter with nil writer returned nil")
	}

	formatter := NewFormatter(FormatText, nil)
	if formatter == nil {
		t.Error("NewFormatter with nil writer returned nil")
	}
}
