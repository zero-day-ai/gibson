package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// OutputFormat represents the output format type
type OutputFormat string

const (
	// FormatText is human-readable text output
	FormatText OutputFormat = "text"
	// FormatJSON is structured JSON output
	FormatJSON OutputFormat = "json"
)

// Formatter interface defines methods for formatting command output
type Formatter interface {
	// PrintSuccess prints a success message
	PrintSuccess(message string) error
	// PrintError prints an error message
	PrintError(message string) error
	// PrintTable prints a table with headers and rows
	PrintTable(headers []string, rows [][]string) error
	// PrintJSON prints arbitrary data as JSON
	PrintJSON(data interface{}) error
}

// TextFormatter implements Formatter for human-readable text output
type TextFormatter struct {
	writer io.Writer
}

// NewTextFormatter creates a new TextFormatter writing to the given writer
func NewTextFormatter(w io.Writer) *TextFormatter {
	if w == nil {
		w = os.Stdout
	}
	return &TextFormatter{writer: w}
}

// PrintSuccess prints a success message with a checkmark prefix
func (f *TextFormatter) PrintSuccess(message string) error {
	_, err := fmt.Fprintf(f.writer, "✓ %s\n", message)
	return err
}

// PrintError prints an error message with an X prefix
func (f *TextFormatter) PrintError(message string) error {
	_, err := fmt.Fprintf(f.writer, "✗ %s\n", message)
	return err
}

// PrintTable prints a table using text/tabwriter for aligned columns
func (f *TextFormatter) PrintTable(headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(f.writer, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	// Print headers in uppercase
	headerLine := make([]string, len(headers))
	for i, h := range headers {
		headerLine[i] = strings.ToUpper(h)
	}
	if _, err := fmt.Fprintln(tw, strings.Join(headerLine, "\t")); err != nil {
		return err
	}

	// Print separator
	separator := make([]string, len(headers))
	for i := range headers {
		separator[i] = strings.Repeat("-", len(headers[i]))
	}
	if _, err := fmt.Fprintln(tw, strings.Join(separator, "\t")); err != nil {
		return err
	}

	// Print rows
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return nil
}

// PrintJSON prints data as formatted JSON (for text output with JSON content)
func (f *TextFormatter) PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// JSONFormatter implements Formatter for structured JSON output
type JSONFormatter struct {
	writer io.Writer
}

// NewJSONFormatter creates a new JSONFormatter writing to the given writer
func NewJSONFormatter(w io.Writer) *JSONFormatter {
	if w == nil {
		w = os.Stdout
	}
	return &JSONFormatter{writer: w}
}

// PrintSuccess prints a success message as JSON
func (f *JSONFormatter) PrintSuccess(message string) error {
	return f.PrintJSON(map[string]interface{}{
		"status":  "success",
		"message": message,
	})
}

// PrintError prints an error message as JSON
func (f *JSONFormatter) PrintError(message string) error {
	return f.PrintJSON(map[string]interface{}{
		"status":  "error",
		"message": message,
	})
}

// PrintTable prints a table as JSON with headers and rows
func (f *JSONFormatter) PrintTable(headers []string, rows [][]string) error {
	// Convert rows to array of maps for better JSON structure
	data := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		rowMap := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				rowMap[header] = row[i]
			} else {
				rowMap[header] = ""
			}
		}
		data = append(data, rowMap)
	}

	return f.PrintJSON(map[string]interface{}{
		"headers": headers,
		"data":    data,
	})
}

// PrintJSON prints arbitrary data as formatted JSON
func (f *JSONFormatter) PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// NewFormatter creates a new Formatter based on the output format
func NewFormatter(format OutputFormat, w io.Writer) Formatter {
	if w == nil {
		w = os.Stdout
	}

	switch format {
	case FormatJSON:
		return NewJSONFormatter(w)
	case FormatText:
		fallthrough
	default:
		return NewTextFormatter(w)
	}
}
