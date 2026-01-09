package attack

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNewAttackResult(t *testing.T) {
	result := NewAttackResult()

	assert.NotNil(t, result)
	assert.Equal(t, AttackStatusSuccess, result.Status)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotNil(t, result.Findings)
	assert.NotNil(t, result.FindingsBySeverity)
	assert.Len(t, result.Findings, 0)
	assert.Len(t, result.FindingsBySeverity, 0)
}

func TestAttackResult_WithError(t *testing.T) {
	result := NewAttackResult()
	err := errors.New("test error")

	result.WithError(err)

	assert.Equal(t, AttackStatusFailed, result.Status)
	assert.Equal(t, err, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

func TestAttackResult_WithAgentFailure(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		failedNodes []string
		wantStatus  AttackStatus
		wantExit    int
	}{
		{
			name:        "single failed node with output",
			output:      "Harness is nil - callback endpoint not received",
			failedNodes: []string{"attack-node-1"},
			wantStatus:  AttackStatusFailed,
			wantExit:    1,
		},
		{
			name:        "multiple failed nodes",
			output:      "Multiple agents failed",
			failedNodes: []string{"node-1", "node-2", "node-3"},
			wantStatus:  AttackStatusFailed,
			wantExit:    1,
		},
		{
			name:        "empty output",
			output:      "",
			failedNodes: []string{"node-1"},
			wantStatus:  AttackStatusFailed,
			wantExit:    1,
		},
		{
			name:        "empty failed nodes",
			output:      "Some error occurred",
			failedNodes: []string{},
			wantStatus:  AttackStatusFailed,
			wantExit:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewAttackResult()

			result.WithAgentFailure(tt.output, tt.failedNodes)

			assert.Equal(t, tt.wantStatus, result.Status, "status mismatch")
			assert.Equal(t, tt.wantExit, result.ExitCode, "exit code mismatch")
			assert.Equal(t, tt.output, result.AgentOutput, "agent output mismatch")
			assert.Equal(t, tt.failedNodes, result.FailedNodes, "failed nodes mismatch")
		})
	}
}

func TestAttackResult_WithAgentFailure_PreservesExitCode(t *testing.T) {
	result := NewAttackResult()
	result.ExitCode = 2 // Pre-set exit code

	result.WithAgentFailure("error", []string{"node-1"})

	assert.Equal(t, 2, result.ExitCode, "should preserve existing non-zero exit code")
}

func TestAttackResult_WithAgentFailure_Chaining(t *testing.T) {
	result := NewAttackResult()
	missionID := types.NewID()

	// Test method chaining
	result.
		WithMissionID(missionID).
		WithDuration(100 * time.Millisecond).
		WithAgentFailure("agent error", []string{"node-1"}).
		WithTurnsUsed(5)

	assert.Equal(t, AttackStatusFailed, result.Status)
	assert.Equal(t, "agent error", result.AgentOutput)
	assert.Equal(t, []string{"node-1"}, result.FailedNodes)
	assert.Equal(t, 1, result.ExitCode)
	assert.Equal(t, &missionID, result.MissionID)
	assert.Equal(t, 100*time.Millisecond, result.Duration)
	assert.Equal(t, 5, result.TurnsUsed)
}

func TestAttackResult_AddFinding(t *testing.T) {
	result := NewAttackResult()
	assert.Equal(t, AttackStatusSuccess, result.Status)

	f := finding.EnhancedFinding{
		Finding: agent.Finding{
			ID:       types.NewID(),
			Severity: "high",
		},
	}

	result.AddFinding(f)

	assert.Equal(t, AttackStatusFindings, result.Status, "status should change to findings")
	assert.Len(t, result.Findings, 1)
	assert.Equal(t, 1, result.FindingsBySeverity["high"])
}

func TestAttackResult_AddFindings(t *testing.T) {
	result := NewAttackResult()

	findings := []finding.EnhancedFinding{
		{
			Finding: agent.Finding{
				ID:       types.NewID(),
				Severity: "critical",
			},
		},
		{
			Finding: agent.Finding{
				ID:       types.NewID(),
				Severity: "high",
			},
		},
		{
			Finding: agent.Finding{
				ID:       types.NewID(),
				Severity: "high",
			},
		},
	}

	result.AddFindings(findings)

	assert.Equal(t, AttackStatusFindings, result.Status)
	assert.Len(t, result.Findings, 3)
	assert.Equal(t, 1, result.FindingsBySeverity["critical"])
	assert.Equal(t, 2, result.FindingsBySeverity["high"])
}

func TestAttackResult_StatusMethods(t *testing.T) {
	tests := []struct {
		name           string
		status         AttackStatus
		isSuccessful   bool
		isFailed       bool
		isTimeout      bool
		isCancelled    bool
		shouldExitNonZ bool
	}{
		{
			name:           "success",
			status:         AttackStatusSuccess,
			isSuccessful:   true,
			isFailed:       false,
			isTimeout:      false,
			isCancelled:    false,
			shouldExitNonZ: false,
		},
		{
			name:           "findings",
			status:         AttackStatusFindings,
			isSuccessful:   true,
			isFailed:       false,
			isTimeout:      false,
			isCancelled:    false,
			shouldExitNonZ: false,
		},
		{
			name:           "failed",
			status:         AttackStatusFailed,
			isSuccessful:   false,
			isFailed:       true,
			isTimeout:      false,
			isCancelled:    false,
			shouldExitNonZ: true,
		},
		{
			name:           "timeout",
			status:         AttackStatusTimeout,
			isSuccessful:   false,
			isFailed:       false,
			isTimeout:      true,
			isCancelled:    false,
			shouldExitNonZ: true,
		},
		{
			name:           "cancelled",
			status:         AttackStatusCancelled,
			isSuccessful:   false,
			isFailed:       false,
			isTimeout:      false,
			isCancelled:    true,
			shouldExitNonZ: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewAttackResult().WithStatus(tt.status)

			assert.Equal(t, tt.isSuccessful, result.IsSuccessful())
			assert.Equal(t, tt.isFailed, result.IsFailed())
			assert.Equal(t, tt.isTimeout, result.IsTimeout())
			assert.Equal(t, tt.isCancelled, result.IsCancelled())
			assert.Equal(t, tt.shouldExitNonZ, result.ShouldExitNonZero())
		})
	}
}

func TestAttackResult_FindingCountMethods(t *testing.T) {
	result := NewAttackResult()

	findings := []finding.EnhancedFinding{
		{Finding: agent.Finding{ID: types.NewID(), Severity: "critical"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "high"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "high"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "medium"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "medium"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "medium"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "low"}},
		{Finding: agent.Finding{ID: types.NewID(), Severity: "info"}},
	}

	result.AddFindings(findings)

	assert.Equal(t, 8, result.FindingCount())
	assert.Equal(t, 1, result.CriticalFindingCount())
	assert.Equal(t, 2, result.HighFindingCount())
	assert.Equal(t, 3, result.MediumFindingCount())
	assert.Equal(t, 1, result.LowFindingCount())
	assert.Equal(t, 1, result.InfoFindingCount())

	assert.True(t, result.HasFindings())
	assert.True(t, result.HasCriticalFindings())
	assert.True(t, result.HasHighFindings())
}
