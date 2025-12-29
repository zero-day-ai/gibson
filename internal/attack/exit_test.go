package attack

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/finding"
)

func TestExitCodeFromResult_NilResult(t *testing.T) {
	exitCode := ExitCodeFromResult(nil)
	assert.Equal(t, ExitError, exitCode, "nil result should return ExitError")
}

func TestExitCodeFromResult_Success(t *testing.T) {
	result := &AttackResult{
		Status:   AttackStatusSuccess,
		Findings: []finding.EnhancedFinding{},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitSuccess, exitCode, "successful attack with no findings should return ExitSuccess")
}

func TestExitCodeFromResult_Cancelled(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusCancelled,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityCritical}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitCancelled, exitCode, "cancelled status should override findings")
}

func TestExitCodeFromResult_Timeout(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusTimeout,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityHigh}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitTimeout, exitCode, "timeout status should override findings")
}

func TestExitCodeFromResult_Failed(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFailed,
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitError, exitCode, "failed status should return ExitError")
}

func TestExitCodeFromResult_ErrorField(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusSuccess,
		Error:  errors.New("test error"),
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitError, exitCode, "result with error should return ExitError")
}

func TestExitCodeFromResult_CriticalFinding(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFindings,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityCritical, Title: "Critical vulnerability"}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitCriticalFindings, exitCode, "critical finding should return ExitCriticalFindings")
}

func TestExitCodeFromResult_HighFinding(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFindings,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityHigh, Title: "High severity vulnerability"}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitCriticalFindings, exitCode, "high severity finding should return ExitCriticalFindings")
}

func TestExitCodeFromResult_MediumFinding(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFindings,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityMedium, Title: "Medium severity vulnerability"}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitWithFindings, exitCode, "medium severity finding should return ExitWithFindings")
}

func TestExitCodeFromResult_LowFinding(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFindings,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityLow, Title: "Low severity vulnerability"}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitWithFindings, exitCode, "low severity finding should return ExitWithFindings")
}

func TestExitCodeFromResult_InfoFinding(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFindings,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityInfo, Title: "Information disclosure"}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitWithFindings, exitCode, "info severity finding should return ExitWithFindings")
}

func TestExitCodeFromResult_MixedFindings(t *testing.T) {
	result := &AttackResult{
		Status: AttackStatusFindings,
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityLow, Title: "Low issue"}},
			{Finding: agent.Finding{Severity: agent.SeverityMedium, Title: "Medium issue"}},
			{Finding: agent.Finding{Severity: agent.SeverityCritical, Title: "Critical issue"}},
		},
	}

	exitCode := ExitCodeFromResult(result)
	assert.Equal(t, ExitCriticalFindings, exitCode, "mixed findings with critical should return ExitCriticalFindings")
}

func TestExitCodeFromResult_UsingSeverityMap(t *testing.T) {
	tests := []struct {
		name               string
		findingsBySeverity map[string]int
		expectedExit       int
	}{
		{
			name: "critical in map",
			findingsBySeverity: map[string]int{
				string(agent.SeverityCritical): 1,
				string(agent.SeverityMedium):   2,
			},
			expectedExit: ExitCriticalFindings,
		},
		{
			name: "high in map",
			findingsBySeverity: map[string]int{
				string(agent.SeverityHigh):   3,
				string(agent.SeverityMedium): 1,
			},
			expectedExit: ExitCriticalFindings,
		},
		{
			name: "only medium and low in map",
			findingsBySeverity: map[string]int{
				string(agent.SeverityMedium): 2,
				string(agent.SeverityLow):    5,
			},
			expectedExit: ExitWithFindings,
		},
		{
			name: "empty map",
			findingsBySeverity: map[string]int{},
			expectedExit:       ExitSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &AttackResult{
				Status:             AttackStatusSuccess,
				FindingsBySeverity: tt.findingsBySeverity,
			}

			exitCode := ExitCodeFromResult(result)
			assert.Equal(t, tt.expectedExit, exitCode)
		})
	}
}

func TestExitCodeFromResult_StatusPriority(t *testing.T) {
	// Test that status codes have priority over findings
	tests := []struct {
		name         string
		status       AttackStatus
		hasCritical  bool
		expectedExit int
	}{
		{
			name:         "cancelled overrides critical findings",
			status:       AttackStatusCancelled,
			hasCritical:  true,
			expectedExit: ExitCancelled,
		},
		{
			name:         "timeout overrides critical findings",
			status:       AttackStatusTimeout,
			hasCritical:  true,
			expectedExit: ExitTimeout,
		},
		{
			name:         "failed overrides critical findings",
			status:       AttackStatusFailed,
			hasCritical:  true,
			expectedExit: ExitError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &AttackResult{
				Status: tt.status,
			}

			if tt.hasCritical {
				result.Findings = []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityCritical, Title: "Critical issue"}},
				}
			}

			exitCode := ExitCodeFromResult(result)
			assert.Equal(t, tt.expectedExit, exitCode)
		})
	}
}

func TestHasCriticalFindings_NilResult(t *testing.T) {
	hasCritical := hasCriticalFindings(nil)
	assert.False(t, hasCritical, "nil result should return false")
}

func TestHasCriticalFindings_EmptyFindings(t *testing.T) {
	result := &AttackResult{
		Findings: []finding.EnhancedFinding{},
	}

	hasCritical := hasCriticalFindings(result)
	assert.False(t, hasCritical, "empty findings should return false")
}

func TestHasCriticalFindings_OnlyCritical(t *testing.T) {
	result := &AttackResult{
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityCritical}},
		},
	}

	hasCritical := hasCriticalFindings(result)
	assert.True(t, hasCritical, "critical finding should return true")
}

func TestHasCriticalFindings_OnlyHigh(t *testing.T) {
	result := &AttackResult{
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityHigh}},
		},
	}

	hasCritical := hasCriticalFindings(result)
	assert.True(t, hasCritical, "high severity finding should return true")
}

func TestHasCriticalFindings_OnlyMedium(t *testing.T) {
	result := &AttackResult{
		Findings: []finding.EnhancedFinding{
			{Finding: agent.Finding{Severity: agent.SeverityMedium}},
		},
	}

	hasCritical := hasCriticalFindings(result)
	assert.False(t, hasCritical, "medium severity finding should return false")
}

func TestHasCriticalFindings_UsingSeverityMap(t *testing.T) {
	tests := []struct {
		name               string
		findingsBySeverity map[string]int
		expected           bool
	}{
		{
			name: "has critical",
			findingsBySeverity: map[string]int{
				string(agent.SeverityCritical): 1,
			},
			expected: true,
		},
		{
			name: "has high",
			findingsBySeverity: map[string]int{
				string(agent.SeverityHigh): 2,
			},
			expected: true,
		},
		{
			name: "has both critical and high",
			findingsBySeverity: map[string]int{
				string(agent.SeverityCritical): 1,
				string(agent.SeverityHigh):     2,
			},
			expected: true,
		},
		{
			name: "only lower severities",
			findingsBySeverity: map[string]int{
				string(agent.SeverityMedium): 3,
				string(agent.SeverityLow):    5,
			},
			expected: false,
		},
		{
			name: "zero counts",
			findingsBySeverity: map[string]int{
				string(agent.SeverityCritical): 0,
				string(agent.SeverityHigh):     0,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &AttackResult{
				FindingsBySeverity: tt.findingsBySeverity,
			}

			hasCritical := hasCriticalFindings(result)
			assert.Equal(t, tt.expected, hasCritical)
		})
	}
}

func TestExitCodeConstants(t *testing.T) {
	// Test that exit code constants have the expected values
	assert.Equal(t, 0, ExitSuccess, "ExitSuccess should be 0")
	assert.Equal(t, 1, ExitWithFindings, "ExitWithFindings should be 1")
	assert.Equal(t, 2, ExitCriticalFindings, "ExitCriticalFindings should be 2")
	assert.Equal(t, 3, ExitError, "ExitError should be 3")
	assert.Equal(t, 4, ExitTimeout, "ExitTimeout should be 4")
	assert.Equal(t, 5, ExitCancelled, "ExitCancelled should be 5")
	assert.Equal(t, 10, ExitConfigError, "ExitConfigError should be 10")
}

func TestExitCodeFromResult_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		result       *AttackResult
		expectedExit int
	}{
		{
			name:         "nil result",
			result:       nil,
			expectedExit: ExitError,
		},
		{
			name: "success no findings",
			result: &AttackResult{
				Status:   AttackStatusSuccess,
				Findings: []finding.EnhancedFinding{},
			},
			expectedExit: ExitSuccess,
		},
		{
			name: "success with info findings",
			result: &AttackResult{
				Status: AttackStatusSuccess,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityInfo}},
				},
			},
			expectedExit: ExitWithFindings,
		},
		{
			name: "cancelled with critical findings",
			result: &AttackResult{
				Status: AttackStatusCancelled,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityCritical}},
				},
			},
			expectedExit: ExitCancelled,
		},
		{
			name: "timeout with findings",
			result: &AttackResult{
				Status: AttackStatusTimeout,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityHigh}},
				},
			},
			expectedExit: ExitTimeout,
		},
		{
			name: "failed status",
			result: &AttackResult{
				Status: AttackStatusFailed,
			},
			expectedExit: ExitError,
		},
		{
			name: "error with success status",
			result: &AttackResult{
				Status: AttackStatusSuccess,
				Error:  errors.New("test error"),
			},
			expectedExit: ExitError,
		},
		{
			name: "critical finding",
			result: &AttackResult{
				Status: AttackStatusFindings,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityCritical}},
				},
			},
			expectedExit: ExitCriticalFindings,
		},
		{
			name: "high finding",
			result: &AttackResult{
				Status: AttackStatusFindings,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityHigh}},
				},
			},
			expectedExit: ExitCriticalFindings,
		},
		{
			name: "medium finding",
			result: &AttackResult{
				Status: AttackStatusFindings,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityMedium}},
				},
			},
			expectedExit: ExitWithFindings,
		},
		{
			name: "mixed findings prioritize critical",
			result: &AttackResult{
				Status: AttackStatusFindings,
				Findings: []finding.EnhancedFinding{
					{Finding: agent.Finding{Severity: agent.SeverityLow}},
					{Finding: agent.Finding{Severity: agent.SeverityCritical}},
					{Finding: agent.Finding{Severity: agent.SeverityMedium}},
				},
			},
			expectedExit: ExitCriticalFindings,
		},
		{
			name: "severity map with critical",
			result: &AttackResult{
				Status: AttackStatusSuccess,
				FindingsBySeverity: map[string]int{
					string(agent.SeverityCritical): 1,
					string(agent.SeverityLow):      3,
				},
			},
			expectedExit: ExitCriticalFindings,
		},
		{
			name: "severity map without critical/high",
			result: &AttackResult{
				Status: AttackStatusSuccess,
				FindingsBySeverity: map[string]int{
					string(agent.SeverityMedium): 2,
					string(agent.SeverityLow):    5,
				},
			},
			expectedExit: ExitWithFindings,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := ExitCodeFromResult(tt.result)
			assert.Equal(t, tt.expectedExit, exitCode)
		})
	}
}
