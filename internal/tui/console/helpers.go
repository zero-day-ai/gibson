package console

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadLastNLines reads the last N lines from a file.
// It returns the lines as a single string with newlines preserved.
// If the file has fewer than N lines, all lines are returned.
// Returns an error if the file cannot be read.
func ReadLastNLines(path string, n int) (string, error) {
	// Clean the file path
	cleanPath := filepath.Clean(path)

	// Open the file
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Read all lines
	scanner := bufio.NewScanner(file)
	var allLines []string

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Calculate start index for last N lines
	start := 0
	if len(allLines) > n {
		start = len(allLines) - n
	}

	// Join the last N lines with newlines
	var result strings.Builder
	for i, line := range allLines[start:] {
		result.WriteString(line)
		// Add newline for all lines except the last one
		if i < len(allLines[start:])-1 {
			result.WriteString("\n")
		}
	}

	return result.String(), nil
}

// ContainsFlag checks if a flag exists in the args slice.
// It supports both long (--flag) and short (-f) format.
// Example: ContainsFlag([]string{"--verbose", "value"}, "--verbose") returns true
func ContainsFlag(args []string, flag string) bool {
	for _, arg := range args {
		// Check exact match
		if arg == flag {
			return true
		}
		// Check if this is a --flag=value format
		if strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

// GetFlag gets the value after a flag in the args slice.
// It supports both "--flag value" and "--flag=value" formats.
// Returns empty string if the flag is not found or has no value.
// Example: GetFlag([]string{"--name", "test"}, "--name") returns "test"
// Example: GetFlag([]string{"--name=test"}, "--name") returns "test"
func GetFlag(args []string, flag string) string {
	for i, arg := range args {
		// Check for --flag=value format
		if strings.HasPrefix(arg, flag+"=") {
			return strings.TrimPrefix(arg, flag+"=")
		}
		// Check for --flag value format
		if arg == flag {
			// Make sure there's a next argument
			if i+1 < len(args) {
				nextArg := args[i+1]
				// Make sure the next arg is not another flag
				if !strings.HasPrefix(nextArg, "-") {
					return nextArg
				}
			}
			// Flag exists but has no value
			return ""
		}
	}
	return ""
}

// ErrorResult creates a standardized ExecutionResult for error cases.
// The result will have IsError=true and ExitCode=1.
// The error message is placed in the Error field.
func ErrorResult(err error) (*ExecutionResult, error) {
	return &ExecutionResult{
		Error:    err.Error(),
		IsError:  true,
		ExitCode: 1,
	}, nil
}

// UsageResult creates a standardized ExecutionResult for usage errors.
// The result will have IsError=true and ExitCode=1.
// The usage message is placed in the Output field.
func UsageResult(usage string) (*ExecutionResult, error) {
	return &ExecutionResult{
		Output:   usage,
		IsError:  true,
		ExitCode: 1,
	}, nil
}
