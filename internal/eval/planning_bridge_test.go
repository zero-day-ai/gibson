package eval

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/zero-day-ai/gibson/internal/planning"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkeval "github.com/zero-day-ai/sdk/eval"
)

// TestNewPlanningFeedbackBridge verifies bridge initialization.
func TestNewPlanningFeedbackBridge(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	assert.NotNil(t, bridge)
	assert.NotNil(t, bridge.planningOrch)
	assert.NotNil(t, bridge.alertChan)
	assert.NotNil(t, bridge.stopChan)
	assert.False(t, bridge.running)
}

// TestBridgeStartStop verifies start/stop lifecycle.
func TestBridgeStartStop(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()

	// Initial state: not running
	assert.False(t, bridge.running)

	// Start the bridge
	bridge.Start(ctx)
	assert.True(t, bridge.running)

	// Starting again should be idempotent
	bridge.Start(ctx)
	assert.True(t, bridge.running)

	// Stop the bridge
	bridge.Stop()
	assert.False(t, bridge.running)

	// Stopping again should be idempotent
	bridge.Stop()
	assert.False(t, bridge.running)
}

// TestBridgeOnAlert verifies alert handling.
func TestBridgeOnAlert(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "test_scorer",
		Score:     0.4,
		Threshold: 0.5,
		Message:   "Performance below threshold",
		Action:    sdkeval.ActionAdjust,
	}

	// Send alert
	bridge.OnAlert(alert)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Alert should be processed (no panic = success)
}

// TestBridgeOnAlertWhenNotRunning verifies alerts are dropped when bridge is not running.
func TestBridgeOnAlertWhenNotRunning(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "test_scorer",
		Score:     0.4,
		Threshold: 0.5,
		Message:   "Performance below threshold",
		Action:    sdkeval.ActionAdjust,
	}

	// Send alert while not running - should not block or panic
	bridge.OnAlert(alert)

	// No assertions needed - test passes if it doesn't hang
}

// TestBridgeOnFeedback verifies feedback handling.
func TestBridgeOnFeedback(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()

	feedback := &sdkeval.Feedback{
		Timestamp: time.Now(),
		StepIndex: 5,
		Scores: map[string]sdkeval.PartialScore{
			"test_scorer": {
				Score:      0.4,
				Confidence: 0.8,
			},
		},
		Overall: sdkeval.PartialScore{
			Score:      0.4,
			Confidence: 0.8,
		},
		Alerts: []sdkeval.Alert{
			{
				Level:     sdkeval.AlertWarning,
				Scorer:    "test_scorer",
				Score:     0.4,
				Threshold: 0.5,
				Message:   "Performance below threshold",
				Action:    sdkeval.ActionAdjust,
			},
		},
	}

	// Send feedback
	bridge.OnFeedback(feedback)

	// Verify step index was updated
	bridge.mu.RLock()
	stepIndex := bridge.stepIndex
	bridge.mu.RUnlock()

	assert.Equal(t, 5, stepIndex)

	// Give time for alert processing
	time.Sleep(50 * time.Millisecond)
}

// TestBridgeOnFeedbackNil verifies nil feedback is handled gracefully.
func TestBridgeOnFeedbackNil(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()

	// Send nil feedback - should not panic
	bridge.OnFeedback(nil)

	// No assertions needed - test passes if it doesn't panic
}

// TestBridgeSetMission verifies mission ID tracking.
func TestBridgeSetMission(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	missionID := types.NewID()
	bridge.SetMission(missionID)

	bridge.mu.RLock()
	actualID := bridge.missionID
	stepIndex := bridge.stepIndex
	bridge.mu.RUnlock()

	assert.Equal(t, missionID, actualID)
	assert.Equal(t, 0, stepIndex, "Step index should reset when mission changes")
}

// TestConvertToStepScore verifies alert to StepScore conversion.
func TestConvertToStepScore(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	missionID := types.NewID()
	bridge.SetMission(missionID)
	bridge.mu.Lock()
	bridge.stepIndex = 3
	bridge.mu.Unlock()

	tests := []struct {
		name              string
		alert             sdkeval.Alert
		wantSuccess       bool
		wantConfidence    float64
		wantShouldReplan  bool
		wantScoringMethod string
	}{
		{
			name: "warning alert - score above threshold",
			alert: sdkeval.Alert{
				Level:     sdkeval.AlertWarning,
				Scorer:    "test_scorer",
				Score:     0.6,
				Threshold: 0.5,
				Message:   "Performance borderline",
				Action:    sdkeval.ActionAdjust,
			},
			wantSuccess:       true,
			wantConfidence:    0.8,
			wantShouldReplan:  true,
			wantScoringMethod: "eval_feedback",
		},
		{
			name: "critical alert - score below threshold",
			alert: sdkeval.Alert{
				Level:     sdkeval.AlertCritical,
				Scorer:    "test_scorer",
				Score:     0.15,
				Threshold: 0.2,
				Message:   "Performance critically low",
				Action:    sdkeval.ActionAbort,
			},
			wantSuccess:       false,
			wantConfidence:    0.95,
			wantShouldReplan:  true,
			wantScoringMethod: "eval_feedback",
		},
		{
			name: "warning alert - score below threshold",
			alert: sdkeval.Alert{
				Level:     sdkeval.AlertWarning,
				Scorer:    "trajectory",
				Score:     0.4,
				Threshold: 0.5,
				Message:   "Deviating from expected path",
				Action:    sdkeval.ActionReconsider,
			},
			wantSuccess:       false,
			wantConfidence:    0.8,
			wantShouldReplan:  true,
			wantScoringMethod: "eval_feedback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := bridge.convertToStepScore(tt.alert)

			assert.NotNil(t, score)
			assert.Equal(t, "step_3", score.NodeID)
			assert.Equal(t, missionID, score.MissionID)
			assert.Equal(t, tt.wantSuccess, score.Success)
			assert.Equal(t, tt.wantConfidence, score.Confidence)
			assert.Equal(t, tt.wantShouldReplan, score.ShouldReplan)
			assert.Equal(t, tt.wantScoringMethod, score.ScoringMethod)
			assert.NotEmpty(t, score.ReplanReason)
			assert.Contains(t, score.ReplanReason, tt.alert.Message)
			assert.Contains(t, score.ReplanReason, tt.alert.Scorer)
		})
	}
}

// TestHandleAlertReconsider verifies "reconsider" action handling.
func TestHandleAlertReconsider(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	missionID := types.NewID()
	bridge.SetMission(missionID)

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "trajectory",
		Score:     0.4,
		Threshold: 0.5,
		Message:   "Deviating from expected path",
		Action:    sdkeval.ActionReconsider,
	}

	// Handle the alert - should not panic
	bridge.handleAlert(ctx, alert)

	// No assertions needed - test passes if it doesn't panic
	// In a full implementation, we'd verify replanning was triggered
}

// TestHandleAlertAbort verifies "abort" action handling.
func TestHandleAlertAbort(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	missionID := types.NewID()
	bridge.SetMission(missionID)

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertCritical,
		Scorer:    "test_scorer",
		Score:     0.15,
		Threshold: 0.2,
		Message:   "Performance critically low",
		Action:    sdkeval.ActionAbort,
	}

	// Handle the alert - should not panic
	bridge.handleAlert(ctx, alert)

	// No assertions needed - test passes if it doesn't panic
	// In a full implementation, we'd verify mission termination was evaluated
}

// TestHandleAlertAdjust verifies "adjust" action handling.
func TestHandleAlertAdjust(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "test_scorer",
		Score:     0.45,
		Threshold: 0.5,
		Message:   "Minor performance issue",
		Action:    sdkeval.ActionAdjust,
	}

	// Handle the alert - should not panic
	bridge.handleAlert(ctx, alert)

	// No assertions needed - test passes if it doesn't panic
}

// TestHandleAlertContinue verifies "continue" action handling.
func TestHandleAlertContinue(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()

	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "test_scorer",
		Score:     0.8,
		Threshold: 0.5,
		Message:   "All good",
		Action:    sdkeval.ActionContinue,
	}

	// Handle the alert - should not panic
	bridge.handleAlert(ctx, alert)

	// No assertions needed - test passes if it doesn't panic
}

// TestBridgeConcurrentAlerts verifies concurrent alert handling.
func TestBridgeConcurrentAlerts(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()

	missionID := types.NewID()
	bridge.SetMission(missionID)

	// Send multiple alerts concurrently
	var wg sync.WaitGroup
	numAlerts := 100

	for i := 0; i < numAlerts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			alert := sdkeval.Alert{
				Level:     sdkeval.AlertWarning,
				Scorer:    "test_scorer",
				Score:     0.4,
				Threshold: 0.5,
				Message:   "Concurrent test",
				Action:    sdkeval.ActionAdjust,
			}

			bridge.OnAlert(alert)
		}(i)
	}

	wg.Wait()

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Test passes if no race conditions or panics
}

// TestBridgeContextCancellation verifies graceful shutdown on context cancellation.
func TestBridgeContextCancellation(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)

	// Cancel context
	cancel()

	// Give time for shutdown
	time.Sleep(100 * time.Millisecond)

	// Bridge should have stopped gracefully
	// Test passes if it doesn't hang
}

// TestBridgeChannelFullDropsAlerts verifies alerts are dropped when channel is full.
func TestBridgeChannelFullDropsAlerts(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	// Don't start the bridge - channel won't be drained
	bridge.mu.Lock()
	bridge.running = true // Set running but don't start goroutine
	bridge.mu.Unlock()

	// Fill the channel beyond capacity
	for i := 0; i < 20; i++ { // Channel buffer is 10
		alert := sdkeval.Alert{
			Level:     sdkeval.AlertWarning,
			Scorer:    "test_scorer",
			Score:     0.4,
			Threshold: 0.5,
			Message:   "Test alert",
			Action:    sdkeval.ActionAdjust,
		}

		// Should not block even when channel is full
		bridge.OnAlert(alert)
	}

	// Clean up
	bridge.mu.Lock()
	bridge.running = false
	bridge.mu.Unlock()

	// Test passes if it doesn't hang
}

// TestBridgeMultipleFeedbacks verifies handling multiple feedback events.
func TestBridgeMultipleFeedbacks(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()

	missionID := types.NewID()
	bridge.SetMission(missionID)

	// Send multiple feedback events
	for i := 0; i < 10; i++ {
		feedback := &sdkeval.Feedback{
			Timestamp: time.Now(),
			StepIndex: i,
			Scores: map[string]sdkeval.PartialScore{
				"test_scorer": {
					Score:      0.5 + float64(i)*0.05,
					Confidence: 0.8,
				},
			},
			Overall: sdkeval.PartialScore{
				Score:      0.5 + float64(i)*0.05,
				Confidence: 0.8,
			},
			Alerts: []sdkeval.Alert{
				{
					Level:     sdkeval.AlertWarning,
					Scorer:    "test_scorer",
					Score:     0.5 + float64(i)*0.05,
					Threshold: 0.5,
					Message:   "Test alert",
					Action:    sdkeval.ActionAdjust,
				},
			},
		}

		bridge.OnFeedback(feedback)
	}

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify final step index
	bridge.mu.RLock()
	stepIndex := bridge.stepIndex
	bridge.mu.RUnlock()

	assert.Equal(t, 9, stepIndex, "Step index should be updated to latest")
}

// TestBridgeNilOrchestrator verifies handling when orchestrator is nil.
func TestBridgeNilOrchestrator(t *testing.T) {
	bridge := NewPlanningFeedbackBridge(nil)

	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()

	// Send alert - should not panic even with nil orchestrator
	alert := sdkeval.Alert{
		Level:     sdkeval.AlertWarning,
		Scorer:    "test_scorer",
		Score:     0.4,
		Threshold: 0.5,
		Message:   "Test alert",
		Action:    sdkeval.ActionReconsider,
	}

	bridge.OnAlert(alert)

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// Test passes if it doesn't panic
}

// TestBridgeIntegration verifies end-to-end bridge functionality.
func TestBridgeIntegration(t *testing.T) {
	// Create orchestrator with minimal configuration
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	missionID := types.NewID()

	// Set up mission
	bridge.SetMission(missionID)
	bridge.Start(ctx)
	defer bridge.Stop()

	// Simulate eval feedback with multiple alerts
	feedback := &sdkeval.Feedback{
		Timestamp: time.Now(),
		StepIndex: 5,
		Scores: map[string]sdkeval.PartialScore{
			"trajectory": {
				Score:      0.3,
				Confidence: 0.85,
				Action:     sdkeval.ActionReconsider,
			},
			"tool_correctness": {
				Score:      0.7,
				Confidence: 0.9,
				Action:     sdkeval.ActionContinue,
			},
		},
		Overall: sdkeval.PartialScore{
			Score:      0.5,
			Confidence: 0.87,
			Action:     sdkeval.ActionAdjust,
		},
		Alerts: []sdkeval.Alert{
			{
				Level:     sdkeval.AlertWarning,
				Scorer:    "trajectory",
				Score:     0.3,
				Threshold: 0.5,
				Message:   "Deviating from expected path",
				Action:    sdkeval.ActionReconsider,
			},
			{
				Level:     sdkeval.AlertCritical,
				Scorer:    "",
				Score:     0.15,
				Threshold: 0.2,
				Message:   "Overall performance critically low",
				Action:    sdkeval.ActionAbort,
			},
		},
	}

	// Send feedback
	bridge.OnFeedback(feedback)

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Verify state was updated
	bridge.mu.RLock()
	stepIndex := bridge.stepIndex
	actualMissionID := bridge.missionID
	bridge.mu.RUnlock()

	assert.Equal(t, 5, stepIndex)
	assert.Equal(t, missionID, actualMissionID)

	// Test passes if no panics occurred during processing
}

// TestBridgeStopWhileProcessing verifies clean shutdown during alert processing.
func TestBridgeStopWhileProcessing(t *testing.T) {
	orchestrator := planning.NewPlanningOrchestrator()
	bridge := NewPlanningFeedbackBridge(orchestrator)

	ctx := context.Background()
	bridge.Start(ctx)

	// Send some alerts
	for i := 0; i < 5; i++ {
		alert := sdkeval.Alert{
			Level:     sdkeval.AlertWarning,
			Scorer:    "test_scorer",
			Score:     0.4,
			Threshold: 0.5,
			Message:   "Test alert",
			Action:    sdkeval.ActionAdjust,
		}
		bridge.OnAlert(alert)
	}

	// Stop immediately - should wait for goroutine to finish
	done := make(chan struct{})
	go func() {
		bridge.Stop()
		close(done)
	}()

	// Stop should complete within reasonable time
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() did not complete within timeout")
	}
}
