package tool

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Tool represents an atomic, stateless operation that can be executed by the Gibson framework.
// Tools are the fundamental building blocks for agent capabilities, providing well-defined
// interfaces for input/output validation and health monitoring.
type Tool interface {
	// Name returns the unique identifier for this tool
	Name() string

	// Description returns a human-readable description of what this tool does
	Description() string

	// Version returns the semantic version of this tool (e.g., "1.0.0")
	Version() string

	// Tags returns a list of tags for categorization and discovery
	Tags() []string

	// InputSchema returns the JSON schema defining valid input parameters
	InputSchema() schema.JSONSchema

	// OutputSchema returns the JSON schema defining the output structure
	OutputSchema() schema.JSONSchema

	// Execute runs the tool with the given input and returns the result.
	// The input and output maps must conform to the InputSchema and OutputSchema respectively.
	// Returns an error if execution fails or input validation fails.
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)

	// Health returns the current health status of this tool
	Health(ctx context.Context) types.HealthStatus
}
