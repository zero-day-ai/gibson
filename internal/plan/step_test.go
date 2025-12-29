package plan

import (
	"testing"
)

func TestStepStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		status   StepStatus
		expected bool
	}{
		{
			name:     "pending is not terminal",
			status:   StepStatusPending,
			expected: false,
		},
		{
			name:     "approved is not terminal",
			status:   StepStatusApproved,
			expected: false,
		},
		{
			name:     "running is not terminal",
			status:   StepStatusRunning,
			expected: false,
		},
		{
			name:     "completed is terminal",
			status:   StepStatusCompleted,
			expected: true,
		},
		{
			name:     "failed is terminal",
			status:   StepStatusFailed,
			expected: true,
		},
		{
			name:     "skipped is terminal",
			status:   StepStatusSkipped,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsTerminal()
			if got != tt.expected {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRiskLevel_IsHighRisk(t *testing.T) {
	tests := []struct {
		name      string
		riskLevel RiskLevel
		expected  bool
	}{
		{
			name:      "low is not high risk",
			riskLevel: RiskLevelLow,
			expected:  false,
		},
		{
			name:      "medium is not high risk",
			riskLevel: RiskLevelMedium,
			expected:  false,
		},
		{
			name:      "high is high risk",
			riskLevel: RiskLevelHigh,
			expected:  true,
		},
		{
			name:      "critical is high risk",
			riskLevel: RiskLevelCritical,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.riskLevel.IsHighRisk()
			if got != tt.expected {
				t.Errorf("IsHighRisk() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStepType_Constants(t *testing.T) {
	// Verify all step type constants are defined correctly
	tests := []struct {
		name     string
		stepType StepType
		expected string
	}{
		{
			name:     "tool type",
			stepType: StepTypeTool,
			expected: "tool",
		},
		{
			name:     "plugin type",
			stepType: StepTypePlugin,
			expected: "plugin",
		},
		{
			name:     "agent type",
			stepType: StepTypeAgent,
			expected: "agent",
		},
		{
			name:     "condition type",
			stepType: StepTypeCondition,
			expected: "condition",
		},
		{
			name:     "parallel type",
			stepType: StepTypeParallel,
			expected: "parallel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.stepType) != tt.expected {
				t.Errorf("StepType = %v, want %v", tt.stepType, tt.expected)
			}
		})
	}
}

func TestStepStatus_Constants(t *testing.T) {
	// Verify all step status constants are defined correctly
	tests := []struct {
		name     string
		status   StepStatus
		expected string
	}{
		{
			name:     "pending status",
			status:   StepStatusPending,
			expected: "pending",
		},
		{
			name:     "approved status",
			status:   StepStatusApproved,
			expected: "approved",
		},
		{
			name:     "running status",
			status:   StepStatusRunning,
			expected: "running",
		},
		{
			name:     "completed status",
			status:   StepStatusCompleted,
			expected: "completed",
		},
		{
			name:     "failed status",
			status:   StepStatusFailed,
			expected: "failed",
		},
		{
			name:     "skipped status",
			status:   StepStatusSkipped,
			expected: "skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("StepStatus = %v, want %v", tt.status, tt.expected)
			}
		})
	}
}

func TestRiskLevel_Constants(t *testing.T) {
	// Verify all risk level constants are defined correctly
	tests := []struct {
		name      string
		riskLevel RiskLevel
		expected  string
	}{
		{
			name:      "low risk",
			riskLevel: RiskLevelLow,
			expected:  "low",
		},
		{
			name:      "medium risk",
			riskLevel: RiskLevelMedium,
			expected:  "medium",
		},
		{
			name:      "high risk",
			riskLevel: RiskLevelHigh,
			expected:  "high",
		},
		{
			name:      "critical risk",
			riskLevel: RiskLevelCritical,
			expected:  "critical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.riskLevel) != tt.expected {
				t.Errorf("RiskLevel = %v, want %v", tt.riskLevel, tt.expected)
			}
		})
	}
}
