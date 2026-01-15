package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/zero-day-ai/sdk/schema"
)

// BinaryScanner discovers tool binaries and retrieves their schemas.
// It provides the core functionality for tool discovery and introspection.
type BinaryScanner interface {
	// Scan walks the tools directory and discovers all executable tool binaries.
	// For each binary, it attempts to fetch the schema by invoking with --schema flag.
	// Returns information about all discovered tools, including any errors encountered.
	Scan(ctx context.Context, toolsDir string) ([]ToolBinaryInfo, error)

	// GetSchema invokes a tool binary with --schema flag to retrieve its schema.
	// Uses a timeout to prevent blocking on unresponsive tools.
	// Returns the tool's schema or an error if the schema cannot be retrieved.
	GetSchema(ctx context.Context, binaryPath string) (*ToolSchema, error)
}

// defaultBinaryScanner is the default implementation of BinaryScanner.
type defaultBinaryScanner struct {
	schemaTimeout time.Duration
}

// NewBinaryScanner creates a new BinaryScanner with default settings.
// The schema timeout is set to 5 seconds to prevent blocking on slow tools.
func NewBinaryScanner() BinaryScanner {
	return &defaultBinaryScanner{
		schemaTimeout: 5 * time.Second,
	}
}

// Scan walks the tools directory and discovers all executable tool binaries.
// It attempts to fetch schemas for each discovered binary, but continues
// scanning even if individual tools fail schema fetching.
func (s *defaultBinaryScanner) Scan(ctx context.Context, toolsDir string) ([]ToolBinaryInfo, error) {
	slog.Info("scanning tools directory", "path", toolsDir)

	// Expand home directory if needed
	if len(toolsDir) > 0 && toolsDir[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		toolsDir = filepath.Join(homeDir, toolsDir[1:])
	}

	// Check if directory exists
	info, err := os.Stat(toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("tools directory does not exist", "path", toolsDir)
			return []ToolBinaryInfo{}, nil
		}
		return nil, fmt.Errorf("failed to stat tools directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("tools path is not a directory: %s", toolsDir)
	}

	var tools []ToolBinaryInfo

	// Walk the directory to find executable files
	err = filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Warn("error accessing path during scan", "path", path, "error", err)
			return nil // Continue scanning despite errors
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is executable
		if !isExecutable(info) {
			slog.Debug("skipping non-executable file", "path", path)
			return nil
		}

		// Extract tool name from filename
		toolName := filepath.Base(path)
		slog.Debug("discovered tool binary", "name", toolName, "path", path)

		// Attempt to fetch schema
		toolInfo := ToolBinaryInfo{
			Name: toolName,
			Path: path,
		}

		schema, err := s.GetSchema(ctx, path)
		if err != nil {
			// Log warning but include tool with error status
			slog.Warn("failed to fetch schema for tool",
				"name", toolName,
				"path", path,
				"error", err)
			toolInfo.Error = err
		} else {
			// Populate tool info from schema
			toolInfo.InputSchema = schema.InputSchema
			toolInfo.OutputSchema = schema.OutputSchema
		}

		// Look for component.yaml in the tool's directory
		toolDir := filepath.Dir(path)
		componentPath := filepath.Join(toolDir, "component.yaml")

		if _, err := os.Stat(componentPath); err == nil {
			// component.yaml exists, try to parse it
			component, err := ParseComponentYAML(componentPath)
			if err != nil {
				slog.Warn("failed to parse component.yaml",
					"tool", toolName,
					"path", componentPath,
					"error", err)
			} else {
				// Extract timeout config
				timeoutCfg, err := component.TimeoutConfig()
				if err != nil {
					slog.Warn("invalid timeout config in component.yaml",
						"tool", toolName,
						"path", componentPath,
						"error", err)
				} else {
					toolInfo.Timeout = timeoutCfg
					slog.Debug("loaded timeout config for tool",
						"tool", toolName,
						"default", timeoutCfg.Default,
						"min", timeoutCfg.Min,
						"max", timeoutCfg.Max)
				}
			}
		}

		tools = append(tools, toolInfo)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk tools directory: %w", err)
	}

	slog.Info("tool scan completed", "total_tools", len(tools))
	return tools, nil
}

// GetSchema invokes a tool binary with --schema flag to retrieve its schema.
// Uses a 5 second timeout to prevent blocking on unresponsive tools.
func (s *defaultBinaryScanner) GetSchema(ctx context.Context, binaryPath string) (*ToolSchema, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, s.schemaTimeout)
	defer cancel()

	// Execute binary with --schema flag
	cmd := exec.CommandContext(ctx, binaryPath, "--schema")
	cmd.Env = append(os.Environ(), "GIBSON_TOOL_MODE=subprocess")

	slog.Debug("fetching schema from tool", "binary", binaryPath)

	// Capture stdout and stderr
	output, err := cmd.Output()
	if err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("schema fetch timeout after %v", s.schemaTimeout)
		}

		// Check for exit error to get stderr
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("schema fetch failed (exit code %d): %s",
				exitErr.ExitCode(), string(exitErr.Stderr))
		}

		return nil, fmt.Errorf("failed to execute binary: %w", err)
	}

	// Parse the JSON schema response
	var schemaResponse struct {
		Name         string      `json:"name"`
		Version      string      `json:"version"`
		Description  string      `json:"description"`
		Tags         []string    `json:"tags"`
		InputSchema  schema.JSON `json:"input_schema"`
		OutputSchema schema.JSON `json:"output_schema"`
	}

	if err := json.Unmarshal(output, &schemaResponse); err != nil {
		return nil, fmt.Errorf("failed to parse schema JSON: %w (output: %s)", err, string(output))
	}

	// Validate that we have at least input and output schemas
	if schemaResponse.InputSchema.Type == "" {
		return nil, fmt.Errorf("schema missing input_schema field")
	}
	if schemaResponse.OutputSchema.Type == "" {
		return nil, fmt.Errorf("schema missing output_schema field")
	}

	slog.Debug("successfully fetched schema",
		"binary", binaryPath,
		"name", schemaResponse.Name,
		"version", schemaResponse.Version)

	return &ToolSchema{
		InputSchema:  schemaResponse.InputSchema,
		OutputSchema: schemaResponse.OutputSchema,
	}, nil
}

// isExecutable checks if a file has executable permissions.
// On Unix-like systems, this checks the execute bits in the file mode.
func isExecutable(info os.FileInfo) bool {
	mode := info.Mode()
	// Check if any of the execute bits are set (owner, group, or other)
	return mode&0111 != 0
}
