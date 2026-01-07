package workflow

import (
	"fmt"
	"time"
)

// PlanningConfig holds configuration for the bounded planning system.
// These settings can be defined in workflow.yaml or mission config.
type PlanningConfig struct {
	// BudgetAllocation overrides the default budget distribution.
	// Values are percentages that must sum to 100.
	BudgetAllocation *BudgetAllocationConfig `yaml:"budget_allocation" json:"budget_allocation"`

	// ReplanLimit sets the maximum number of replanning events allowed.
	// Default is 3. Set to 0 to disable replanning entirely.
	ReplanLimit int `yaml:"replan_limit" json:"replan_limit"`

	// MemoryQuery enables querying long-term memory for planning context.
	// Default is true.
	MemoryQuery bool `yaml:"memory_query" json:"memory_query"`

	// PlanningTimeout is the maximum time for strategic planning.
	// Default is 30 seconds.
	PlanningTimeout string `yaml:"planning_timeout" json:"planning_timeout"`
}

// BudgetAllocationConfig defines the budget distribution across planning phases.
// All values are percentages and must sum to 100.
type BudgetAllocationConfig struct {
	Planning   int `yaml:"planning" json:"planning"`     // Default 10
	Scoring    int `yaml:"scoring" json:"scoring"`       // Default 5
	Replanning int `yaml:"replanning" json:"replanning"` // Default 5
	Execution  int `yaml:"execution" json:"execution"`   // Default 80
}

// Default percentages for budget allocation.
const (
	DefaultStrategicPlanningPercent = 10
	DefaultScoringPercent           = 5
	DefaultReplanningPercent        = 5
	DefaultExecutionPercent         = 80
)

// DefaultPlanningConfig returns the default planning configuration.
func DefaultPlanningConfig() *PlanningConfig {
	return &PlanningConfig{
		BudgetAllocation: &BudgetAllocationConfig{
			Planning:   DefaultStrategicPlanningPercent,
			Scoring:    DefaultScoringPercent,
			Replanning: DefaultReplanningPercent,
			Execution:  DefaultExecutionPercent,
		},
		ReplanLimit:     3,
		MemoryQuery:     true,
		PlanningTimeout: "30s",
	}
}

// Validate validates the planning configuration.
// Returns an error if:
//   - Budget percentages don't sum to 100
//   - ReplanLimit is negative
//   - PlanningTimeout is invalid or negative
func (c *PlanningConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("planning config cannot be nil")
	}

	// Validate budget allocation if present
	if c.BudgetAllocation != nil {
		if err := c.BudgetAllocation.Validate(); err != nil {
			return fmt.Errorf("invalid budget allocation: %w", err)
		}
	}

	// Validate replan limit
	if c.ReplanLimit < 0 {
		return fmt.Errorf("replan_limit must be non-negative, got %d", c.ReplanLimit)
	}

	// Validate planning timeout
	if c.PlanningTimeout != "" {
		timeout, err := time.ParseDuration(c.PlanningTimeout)
		if err != nil {
			return fmt.Errorf("invalid planning_timeout format %q: %w", c.PlanningTimeout, err)
		}
		if timeout < 0 {
			return fmt.Errorf("planning_timeout must be positive, got %s", c.PlanningTimeout)
		}
	}

	return nil
}

// Validate validates the budget allocation configuration.
// Returns an error if percentages don't sum to 100 or any value is negative.
func (c *BudgetAllocationConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("budget allocation config cannot be nil")
	}

	// Check for negative values
	if c.Planning < 0 {
		return fmt.Errorf("planning percentage must be non-negative, got %d", c.Planning)
	}
	if c.Scoring < 0 {
		return fmt.Errorf("scoring percentage must be non-negative, got %d", c.Scoring)
	}
	if c.Replanning < 0 {
		return fmt.Errorf("replanning percentage must be non-negative, got %d", c.Replanning)
	}
	if c.Execution < 0 {
		return fmt.Errorf("execution percentage must be non-negative, got %d", c.Execution)
	}

	// Validate sum equals 100
	sum := c.Planning + c.Scoring + c.Replanning + c.Execution
	if sum != 100 {
		return fmt.Errorf("budget percentages must sum to 100, got %d (planning=%d, scoring=%d, replanning=%d, execution=%d)",
			sum, c.Planning, c.Scoring, c.Replanning, c.Execution)
	}

	return nil
}

// ToBudgetAllocation converts the config to BudgetAllocation percentages.
// If the BudgetAllocationConfig is nil, returns nil.
func (c *PlanningConfig) ToBudgetAllocation() *BudgetAllocationConfig {
	if c == nil || c.BudgetAllocation == nil {
		return nil
	}
	return c.BudgetAllocation
}

// GetPlanningTimeoutDuration returns the planning timeout as a time.Duration.
// Returns the default timeout (30s) if the timeout is not set or invalid.
func (c *PlanningConfig) GetPlanningTimeoutDuration() time.Duration {
	if c == nil || c.PlanningTimeout == "" {
		return 30 * time.Second
	}

	timeout, err := time.ParseDuration(c.PlanningTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return timeout
}

// ApplyDefaults fills in missing configuration values with defaults.
func (c *PlanningConfig) ApplyDefaults() {
	if c == nil {
		return
	}

	// Apply default budget allocation if not specified
	if c.BudgetAllocation == nil {
		c.BudgetAllocation = &BudgetAllocationConfig{
			Planning:   DefaultStrategicPlanningPercent,
			Scoring:    DefaultScoringPercent,
			Replanning: DefaultReplanningPercent,
			Execution:  DefaultExecutionPercent,
		}
	}

	// Apply default replan limit if not specified (0 is valid - disables replanning)
	if c.ReplanLimit == 0 {
		c.ReplanLimit = 3
	}

	// Apply default planning timeout if not specified
	if c.PlanningTimeout == "" {
		c.PlanningTimeout = "30s"
	}
}
