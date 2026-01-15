package toolexec

import (
	"context"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/schema"
	sdktypes "github.com/zero-day-ai/sdk/types"
)

// ToolExecutorService orchestrates tool discovery, caching, and subprocess-based execution.
// It provides a centralized service for managing external tool binaries without
// requiring individual gRPC servers per tool.
type ToolExecutorService interface {
	// Start initializes the service, scanning for available tool binaries
	// and populating the internal tool registry with their schemas.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the service, cleaning up any running
	// subprocesses and releasing resources.
	Stop(ctx context.Context) error

	// Execute runs a tool subprocess with the given input and timeout.
	// Input and output are marshaled as JSON via stdin/stdout.
	// Returns the tool's output or an error if execution fails or times out.
	Execute(ctx context.Context, name string, input map[string]any, timeout time.Duration) (map[string]any, error)

	// ListTools returns metadata for all discovered tools.
	// This provides information needed for tool selection and introspection.
	ListTools() []ToolDescriptor

	// GetToolSchema retrieves the input and output schemas for a specific tool.
	// Returns ErrToolNotFound if the tool is not registered.
	GetToolSchema(name string) (*ToolSchema, error)

	// RefreshTools rescans the tool directory and updates the internal registry.
	// This enables hot-reload of tools without restarting the daemon.
	RefreshTools(ctx context.Context) error
}

// ToolBinaryInfo contains information about a discovered tool binary.
// It includes metadata, schemas, and any errors encountered during discovery.
type ToolBinaryInfo struct {
	// Name is the tool's identifier, typically derived from the binary filename.
	Name string

	// Path is the absolute filesystem path to the tool binary.
	Path string

	// Version is the tool's version string, if provided by the tool.
	Version string

	// Description provides a human-readable explanation of what the tool does.
	Description string

	// Tags are optional labels for categorizing and filtering tools.
	Tags []string

	// InputSchema defines the expected structure of the tool's input.
	InputSchema schema.JSON

	// OutputSchema defines the structure of the tool's output.
	OutputSchema schema.JSON

	// Timeout defines the timeout configuration for this tool.
	// Zero value means no timeout constraints are configured.
	Timeout sdktypes.TimeoutConfig

	// Error captures any error that occurred during tool discovery or schema fetching.
	// A non-nil error indicates the tool may not be usable.
	Error error
}

// ToolSchema defines a tool's input and output schemas for validation
// and documentation purposes.
type ToolSchema struct {
	// InputSchema defines the expected structure of the tool's input.
	InputSchema schema.JSON `json:"input_schema"`

	// OutputSchema defines the structure of the tool's output.
	OutputSchema schema.JSON `json:"output_schema"`
}

// ExecuteRequest contains all information needed to execute a tool subprocess.
type ExecuteRequest struct {
	// BinaryPath is the absolute path to the tool binary to execute.
	BinaryPath string

	// Input is the JSON-serializable input data to pass to the tool via stdin.
	Input map[string]any

	// Timeout specifies the maximum duration for tool execution.
	// The subprocess will be killed if it exceeds this duration.
	Timeout time.Duration

	// Env contains environment variables to set for the subprocess.
	// Should include GIBSON_TOOL_MODE=subprocess.
	Env []string
}

// ExecuteResult contains the outcome of a tool subprocess execution.
type ExecuteResult struct {
	// Output is the JSON-parsed output from the tool's stdout.
	Output map[string]any

	// Duration is the actual execution time of the subprocess.
	Duration time.Duration

	// ExitCode is the process exit code. 0 indicates success.
	ExitCode int

	// Stderr captures any error output from the tool's stderr stream.
	Stderr string
}

// ToolDescriptor provides metadata about a tool for listing and introspection.
// It extends the base tool.ToolDescriptor with execution-specific information.
type ToolDescriptor struct {
	// Name is the tool's unique identifier.
	Name string `json:"name"`

	// Description explains what the tool does.
	Description string `json:"description"`

	// Version is the tool's version string.
	Version string `json:"version"`

	// Tags are labels for categorizing the tool.
	Tags []string `json:"tags"`

	// InputSchema defines the expected input structure.
	InputSchema schema.JSON `json:"input_schema"`

	// OutputSchema defines the output structure.
	OutputSchema schema.JSON `json:"output_schema"`

	// BinaryPath is the absolute path to the tool binary.
	BinaryPath string `json:"binary_path"`

	// Status indicates the tool's readiness state.
	// Values: "ready", "schema-unknown", "error"
	Status string `json:"status"`

	// ErrorMessage contains details if Status is "error".
	ErrorMessage string `json:"error_message,omitempty"`

	// Metrics tracks execution statistics for monitoring.
	Metrics *ToolMetrics `json:"metrics,omitempty"`

	// Timeout defines the timeout configuration for this tool.
	// Zero value means no timeout constraints are configured.
	Timeout sdktypes.TimeoutConfig `json:"timeout,omitempty"`
}

// ToolMetrics tracks execution statistics for a tool.
// Metrics are updated after each tool execution for observability.
type ToolMetrics struct {
	// TotalExecutions is the total number of execution attempts.
	TotalExecutions int64 `json:"total_executions"`

	// SuccessfulExecutions is the number of successful executions.
	SuccessfulExecutions int64 `json:"successful_executions"`

	// FailedExecutions is the number of failed executions.
	FailedExecutions int64 `json:"failed_executions"`

	// TotalDuration is the cumulative execution time across all calls.
	TotalDuration time.Duration `json:"total_duration"`

	// AverageDuration is the mean execution time.
	AverageDuration time.Duration `json:"average_duration"`

	// LastExecutedAt is the timestamp of the most recent execution.
	LastExecutedAt *time.Time `json:"last_executed_at,omitempty"`
}

// Tool execution error codes
const (
	ErrToolNotFound         types.ErrorCode = "TOOL_NOT_FOUND"
	ErrToolTimeout          types.ErrorCode = "TOOL_TIMEOUT"
	ErrToolExecutionFailed  types.ErrorCode = "TOOL_EXECUTION_FAILED"
	ErrInvalidToolOutput    types.ErrorCode = "TOOL_INVALID_OUTPUT"
	ErrToolSpawnFailed      types.ErrorCode = "TOOL_SPAWN_FAILED"
	ErrToolSchemaFetchError types.ErrorCode = "TOOL_SCHEMA_FETCH_ERROR"
	ErrInvalidTimeout       types.ErrorCode = "TOOL_INVALID_TIMEOUT"
)
