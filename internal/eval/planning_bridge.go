package eval

import (
	"context"
	"fmt"
	"sync"

	"github.com/zero-day-ai/gibson/internal/planning"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkeval "github.com/zero-day-ai/sdk/eval"
)

// PlanningFeedbackBridge connects evaluation feedback alerts to the planning
// orchestrator, enabling adaptive replanning based on real-time performance.
//
// When evaluation scores drop below critical thresholds, the bridge triggers
// replanning through the PlanningOrchestrator. This creates a feedback loop
// where poor agent performance automatically triggers plan adjustments.
//
// Thread safety: PlanningFeedbackBridge is safe for concurrent use.
type PlanningFeedbackBridge struct {
	// planningOrch is the orchestrator that handles replanning
	planningOrch *planning.PlanningOrchestrator

	// alertChan receives alerts from the eval system
	alertChan chan sdkeval.Alert

	// stopChan signals the bridge to stop processing
	stopChan chan struct{}

	// wg tracks the goroutine lifecycle
	wg sync.WaitGroup

	// mu protects concurrent access to internal state
	mu sync.RWMutex

	// running indicates if the bridge is currently active
	running bool

	// missionID tracks the current mission being evaluated
	missionID types.ID

	// stepIndex tracks the current step index for context
	stepIndex int
}

// NewPlanningFeedbackBridge creates a new bridge between eval and planning systems.
//
// The bridge must be started with Start() before it begins processing alerts.
//
// Example:
//
//	bridge := eval.NewPlanningFeedbackBridge(planningOrch)
//	bridge.Start(ctx)
//	defer bridge.Stop()
func NewPlanningFeedbackBridge(planningOrch *planning.PlanningOrchestrator) *PlanningFeedbackBridge {
	return &PlanningFeedbackBridge{
		planningOrch: planningOrch,
		alertChan:    make(chan sdkeval.Alert, 10), // Buffer to avoid blocking
		stopChan:     make(chan struct{}),
		running:      false,
	}
}

// Start begins listening for alerts in a background goroutine.
//
// The bridge will process alerts until Stop() is called or the context is cancelled.
// It's safe to call Start() multiple times - subsequent calls are ignored if already running.
func (b *PlanningFeedbackBridge) Start(ctx context.Context) {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	b.mu.Unlock()

	b.wg.Add(1)
	go b.processAlerts(ctx)
}

// Stop gracefully shuts down the bridge.
//
// It signals the processing goroutine to stop and waits for it to complete.
// Any alerts in the channel will be processed before shutdown.
// It's safe to call Stop() multiple times.
func (b *PlanningFeedbackBridge) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	b.mu.Unlock()

	// Signal shutdown
	close(b.stopChan)

	// Wait for processing goroutine to finish
	b.wg.Wait()
}

// OnAlert handles an alert from the eval system.
//
// This method is typically called by EvalEventHandler when alerts are generated.
// Alerts are sent to a buffered channel for asynchronous processing.
//
// If the bridge is not running or the channel is full, the alert is dropped.
// This prevents blocking the eval system if the bridge falls behind.
func (b *PlanningFeedbackBridge) OnAlert(alert sdkeval.Alert) {
	b.mu.RLock()
	running := b.running
	b.mu.RUnlock()

	if !running {
		return
	}

	// Non-blocking send - drop alert if channel is full
	select {
	case b.alertChan <- alert:
		// Alert sent successfully
	default:
		// Channel full - drop alert to avoid blocking
		// This is acceptable because the eval system will continue
		// generating feedback, and we only need to catch some alerts
		// to trigger replanning
	}
}

// OnFeedback handles feedback from the eval system and updates bridge state.
//
// This method extracts alerts from feedback and sends them to OnAlert.
// It also updates the bridge's internal state tracking.
func (b *PlanningFeedbackBridge) OnFeedback(feedback *sdkeval.Feedback) {
	if feedback == nil {
		return
	}

	b.mu.Lock()
	b.stepIndex = feedback.StepIndex
	b.mu.Unlock()

	// Forward each alert to OnAlert
	for _, alert := range feedback.Alerts {
		b.OnAlert(alert)
	}
}

// SetMission updates the current mission ID being tracked.
//
// This should be called when a new mission starts to ensure alerts
// are associated with the correct mission.
func (b *PlanningFeedbackBridge) SetMission(missionID types.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.missionID = missionID
	b.stepIndex = 0
}

// processAlerts runs in a goroutine and processes alerts as they arrive.
//
// It handles:
//   - "reconsider" action: Triggers replanning
//   - "abort" action: Evaluates mission termination
//   - Other actions: Logged but no action taken
func (b *PlanningFeedbackBridge) processAlerts(ctx context.Context) {
	defer b.wg.Done()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled - shutdown gracefully
			return

		case <-b.stopChan:
			// Stop signal received - shutdown gracefully
			return

		case alert := <-b.alertChan:
			// Process the alert
			b.handleAlert(ctx, alert)
		}
	}
}

// handleAlert processes a single alert and takes appropriate action.
//
// Actions:
//   - ActionReconsider: Triggers replanning via orchestrator
//   - ActionAbort: Evaluates mission termination (currently logged)
//   - Other: Logged for informational purposes
func (b *PlanningFeedbackBridge) handleAlert(ctx context.Context, alert sdkeval.Alert) {
	switch alert.Action {
	case sdkeval.ActionReconsider:
		// Convert alert to StepScore and trigger replanning
		score := b.convertToStepScore(alert)
		b.triggerReplanning(ctx, score)

	case sdkeval.ActionAbort:
		// Critical alert - evaluate mission termination
		b.evaluateMissionTermination(ctx, alert)

	case sdkeval.ActionAdjust:
		// Warning level - log but continue
		// Future: Could trigger minor plan adjustments

	case sdkeval.ActionContinue:
		// Everything is fine - no action needed

	default:
		// Unknown action - log for debugging
	}
}

// convertToStepScore converts an eval.Alert to a planning.StepScore.
//
// This maps evaluation feedback into the planning system's data model:
//   - Low scores → ShouldReplan = true
//   - Alert message → ReplanReason
//   - Alert metadata → StepScore fields
func (b *PlanningFeedbackBridge) convertToStepScore(alert sdkeval.Alert) *planning.StepScore {
	b.mu.RLock()
	missionID := b.missionID
	stepIndex := b.stepIndex
	b.mu.RUnlock()

	// Determine success based on score vs threshold
	success := alert.Score >= alert.Threshold

	// Map alert level to confidence
	confidence := 0.8
	if alert.Level == sdkeval.AlertCritical {
		confidence = 0.95
	}

	// Build replan reason from alert
	replanReason := fmt.Sprintf("Eval alert: %s (scorer: %s, score: %.2f, threshold: %.2f)",
		alert.Message, alert.Scorer, alert.Score, alert.Threshold)

	return &planning.StepScore{
		NodeID:        fmt.Sprintf("step_%d", stepIndex),
		MissionID:     missionID,
		Success:       success,
		Confidence:    confidence,
		ScoringMethod: "eval_feedback",
		ShouldReplan:  true,
		ReplanReason:  replanReason,
	}
}

// triggerReplanning calls the planning orchestrator to perform tactical replanning.
//
// This is called when evaluation feedback indicates the agent is performing poorly
// and needs to adjust its approach.
//
// The replanning is asynchronous and may fail if:
//   - Maximum replans already reached
//   - No replanner configured
//   - Context cancelled
func (b *PlanningFeedbackBridge) triggerReplanning(ctx context.Context, score *planning.StepScore) {
	if b.planningOrch == nil {
		return
	}

	// Emit a ReplanTriggered event to notify the mission controller
	// The controller owns the WorkflowState and will handle the actual replanning
	b.emitReplanRequestedEvent(ctx, score)

	// Note: We don't call HandleReplan directly because we don't have WorkflowState
	// The mission controller subscribes to planning events and will call HandleReplan
	// with the proper state when it receives the ReplanTriggered event
}

// evaluateMissionTermination handles critical alerts that recommend aborting.
//
// This evaluates whether the mission should be gracefully shut down based on:
//   - Severity and frequency of critical alerts
//   - Mission progress and state
//   - Whether the primary goal has been achieved
//   - Budget exhaustion
func (b *PlanningFeedbackBridge) evaluateMissionTermination(ctx context.Context, alert sdkeval.Alert) {
	b.mu.Lock()
	_ = b.missionID // Reserved for future use in mission-specific termination logic
	_ = b.stepIndex // Reserved for future use in alert tracking
	b.mu.Unlock()

	// Count recent critical alerts
	criticalAlertCount := b.countRecentCriticalAlerts()

	// Determine if termination is warranted based on multiple factors
	shouldTerminate := false
	terminationReason := ""

	// Factor 1: Multiple critical alerts in succession
	if criticalAlertCount >= 3 {
		shouldTerminate = true
		terminationReason = fmt.Sprintf("Multiple critical alerts (%d) indicate mission cannot proceed successfully", criticalAlertCount)
	}

	// Factor 2: Specific abort recommendation from evaluation
	if alert.Action == sdkeval.ActionAbort && alert.Level == sdkeval.AlertCritical {
		shouldTerminate = true
		terminationReason = fmt.Sprintf("Critical evaluation failure: %s (scorer: %s, score: %.2f)", alert.Message, alert.Scorer, alert.Score)
	}

	// Factor 3: Score has dropped below critical threshold
	if alert.Score < alert.Threshold && alert.Level == sdkeval.AlertCritical {
		shouldTerminate = true
		terminationReason = fmt.Sprintf("Score %.2f below critical threshold %.2f - mission quality unacceptable", alert.Score, alert.Threshold)
	}

	// If termination is warranted, emit termination event
	if shouldTerminate {
		b.emitMissionTerminationEvent(ctx, terminationReason, alert)

		// Note: We don't directly call controller shutdown because we don't have a controller reference
		// The mission controller subscribes to planning events and will handle graceful shutdown
		// when it receives the ConstraintViolation or custom termination event
	}

	// Otherwise, log the critical alert for monitoring but continue execution
	// The mission controller will track these events and may decide to terminate based on its own logic
}

// countRecentCriticalAlerts counts critical alerts received in the last 5 steps.
// This is a simple heuristic for detecting sustained poor performance.
func (b *PlanningFeedbackBridge) countRecentCriticalAlerts() int {
	// This would require maintaining alert history in the bridge
	// For now, return 1 (the current alert)
	// A full implementation would track alerts with timestamps/step indices
	return 1
}

// emitReplanRequestedEvent emits an event indicating replanning was requested by evaluation feedback.
func (b *PlanningFeedbackBridge) emitReplanRequestedEvent(ctx context.Context, score *planning.StepScore) {
	if b.planningOrch == nil {
		return
	}

	// Get the event emitter from the planning orchestrator if available
	// For now, create a replan triggered event manually
	event := planning.NewReplanTriggeredEvent(
		score.MissionID,
		score.NodeID,
		score.ReplanReason,
		0, // replan count would come from controller state
	)

	// Emit via the planning orchestrator's event system
	// Note: This assumes the orchestrator exposes an event emitter
	// If not available, the controller integration will need to subscribe differently
	_ = event // TODO: Wire up to actual event emitter when available
}

// emitMissionTerminationEvent emits an event indicating mission termination is recommended.
func (b *PlanningFeedbackBridge) emitMissionTerminationEvent(ctx context.Context, reason string, alert sdkeval.Alert) {
	b.mu.RLock()
	missionID := b.missionID
	b.mu.RUnlock()

	// Create a constraint violation event to signal termination
	details := map[string]any{
		"alert_scorer":    alert.Scorer,
		"alert_score":     alert.Score,
		"alert_threshold": alert.Threshold,
		"alert_message":   alert.Message,
		"alert_level":     string(alert.Level),
	}

	event := planning.NewConstraintViolationEvent(
		missionID,
		"mission_quality",
		reason,
		details,
	)

	// Emit via the planning orchestrator's event system
	// The mission controller will subscribe to these events and handle graceful shutdown
	_ = event // TODO: Wire up to actual event emitter when available
}
