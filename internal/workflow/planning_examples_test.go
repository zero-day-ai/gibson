package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlanningExampleWorkflows verifies that example planning workflow files parse correctly
// and have the planning configuration populated.
func TestPlanningExampleWorkflows(t *testing.T) {
	testCases := []struct {
		name string
		path string
	}{
		{
			name: "workflow-with-planning.yaml",
			path: "../../examples/workflow-with-planning.yaml",
		},
		{
			name: "planning-basic.yaml",
			path: "../../examples/workflows/planning-basic.yaml",
		},
		{
			name: "planning-constrained.yaml",
			path: "../../examples/workflows/planning-constrained.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the workflow file
			workflow, err := ParseWorkflowFile(tc.path)
			require.NoError(t, err, "Failed to parse %s", tc.path)
			require.NotNil(t, workflow, "Workflow should not be nil")

			// Verify planning config is loaded (Planning field is not nil)
			assert.NotNil(t, workflow.Planning, "Planning field should not be nil for %s", tc.name)
		})
	}
}
