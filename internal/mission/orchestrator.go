package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// MissionOrchestrator coordinates mission execution.
type MissionOrchestrator interface {
	// Execute runs the mission workflow and manages all orchestration
	Execute(ctx context.Context, mission *Mission) (*MissionResult, error)
}

// DefaultMissionOrchestrator implements MissionOrchestrator.
type DefaultMissionOrchestrator struct {
	store            MissionStore
	emitter          EventEmitter
	workflowExecutor *workflow.WorkflowExecutor
	harnessFactory   harness.HarnessFactoryInterface
}

// OrchestratorOption is a functional option for configuring the orchestrator.
type OrchestratorOption func(*DefaultMissionOrchestrator)

// WithEventEmitter sets the event emitter.
func WithEventEmitter(emitter EventEmitter) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.emitter = emitter
	}
}

// WithWorkflowExecutor sets the workflow executor.
func WithWorkflowExecutor(executor *workflow.WorkflowExecutor) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.workflowExecutor = executor
	}
}

// WithHarnessFactory sets the harness factory.
func WithHarnessFactory(factory harness.HarnessFactoryInterface) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.harnessFactory = factory
	}
}

// NewMissionOrchestrator creates a new mission orchestrator.
func NewMissionOrchestrator(store MissionStore, opts ...OrchestratorOption) *DefaultMissionOrchestrator {
	o := &DefaultMissionOrchestrator{
		store:   store,
		emitter: NewDefaultEventEmitter(WithBufferSize(100)), // Default buffer size
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// Execute orchestrates mission execution.
func (o *DefaultMissionOrchestrator) Execute(ctx context.Context, mission *Mission) (*MissionResult, error) {
	// Validate mission can be executed
	if !mission.Status.CanTransitionTo(MissionStatusRunning) {
		return nil, NewInvalidStateError(mission.Status, MissionStatusRunning)
	}

	// Update mission status to running
	startedAt := time.Now()
	mission.Status = MissionStatusRunning
	mission.StartedAt = &startedAt

	// Initialize metrics
	mission.Metrics = &MissionMetrics{
		StartedAt:          startedAt,
		LastUpdateAt:       startedAt,
		FindingsBySeverity: make(map[string]int),
	}

	if err := o.store.Update(ctx, mission); err != nil {
		return nil, fmt.Errorf("failed to update mission status: %w", err)
	}

	// Emit mission started event
	o.emitter.Emit(ctx, MissionEvent{
		Type:      EventMissionStarted,
		MissionID: mission.ID,
		Timestamp: startedAt,
	})

	// Create result
	result := &MissionResult{
		MissionID:  mission.ID,
		Status:     MissionStatusCompleted,
		Metrics:    mission.Metrics,
		FindingIDs: []types.ID{},
	}

	// Execute workflow
	var workflowResult *workflow.WorkflowResult
	var workflowErr error

	// Check if workflow executor is configured
	if o.workflowExecutor == nil {
		// TODO: Handle approval workflows
		// TODO: Create checkpoints for pause/resume

		// For now, just simulate execution without workflow executor
		time.Sleep(100 * time.Millisecond)
	} else {
		// Parse workflow from mission's workflow definition
		// NOTE: This assumes mission has workflow JSON/YAML. If not available,
		// we need a WorkflowStore to load by WorkflowID
		var wf *workflow.Workflow
		if mission.WorkflowJSON != "" {
			// Check if the workflow content is JSON (starts with '{') or YAML
			workflowContent := strings.TrimSpace(mission.WorkflowJSON)
			if strings.HasPrefix(workflowContent, "{") {
				// JSON format - unmarshal directly
				wf = &workflow.Workflow{}
				workflowErr = json.Unmarshal([]byte(workflowContent), wf)
			} else {
				// YAML format - use ParseWorkflow
				wf, workflowErr = workflow.ParseWorkflow([]byte(mission.WorkflowJSON))
			}

			if workflowErr != nil {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = fmt.Sprintf("failed to parse workflow: %v", workflowErr)
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow parse error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to parse workflow: %w", workflowErr)
			}
		} else {
			// TODO: Load workflow from WorkflowStore using mission.WorkflowID
			// For now, return error if no workflow JSON is available
			result.Status = MissionStatusFailed
			mission.Status = MissionStatusFailed
			mission.Error = "workflow definition not available"
			completedAt := time.Now()
			mission.CompletedAt = &completedAt
			mission.Metrics.Duration = completedAt.Sub(startedAt)

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after workflow missing error: %w", updateErr)
			}

			return result, fmt.Errorf("workflow definition not available in mission")
		}

		// Create harness if factory is configured
		if o.harnessFactory == nil {
			result.Status = MissionStatusFailed
			mission.Status = MissionStatusFailed
			mission.Error = "harness factory not configured"
			completedAt := time.Now()
			mission.CompletedAt = &completedAt
			mission.Metrics.Duration = completedAt.Sub(startedAt)

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after harness factory error: %w", updateErr)
			}

			return result, fmt.Errorf("harness factory not configured")
		}

		// Create mission context and target info for harness
		missionCtx := harness.NewMissionContext(mission.ID, mission.Name, "")
		targetInfo := harness.NewTargetInfo(mission.TargetID, "mission-target", "", "")

		// Create harness for workflow execution
		// Use first agent in workflow or empty string if no agents
		agentName := ""
		if len(wf.Nodes) > 0 {
			for _, node := range wf.Nodes {
				if node.Type == workflow.NodeTypeAgent && node.AgentName != "" {
					agentName = node.AgentName
					break
				}
			}
		}

		agentHarness, err := o.harnessFactory.Create(agentName, missionCtx, targetInfo)
		if err != nil {
			result.Status = MissionStatusFailed
			mission.Status = MissionStatusFailed
			mission.Error = fmt.Sprintf("failed to create harness: %v", err)
			completedAt := time.Now()
			mission.CompletedAt = &completedAt
			mission.Metrics.Duration = completedAt.Sub(startedAt)

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after harness creation error: %w", updateErr)
			}

			return result, fmt.Errorf("failed to create harness: %w", err)
		}
		// Note: AgentHarness doesn't have a Close method, so no cleanup needed

		// Execute workflow with constraint checking
		workflowResult, workflowErr = o.executeWithConstraints(ctx, wf, agentHarness, mission)

		// Collect findings from workflow result
		if workflowResult != nil && len(workflowResult.Findings) > 0 {
			for _, finding := range workflowResult.Findings {
				result.FindingIDs = append(result.FindingIDs, finding.ID)
			}
		}

		// Check for workflow execution errors or failures
		if workflowErr != nil || (workflowResult != nil && workflowResult.Status == workflow.WorkflowStatusFailed) {
			result.Status = MissionStatusFailed
			if workflowErr != nil {
				mission.Error = fmt.Sprintf("workflow execution error: %v", workflowErr)
			} else if workflowResult.Error != nil {
				mission.Error = fmt.Sprintf("workflow failed: %v", workflowResult.Error.Message)
			}
		} else if workflowResult != nil && workflowResult.Status == workflow.WorkflowStatusCancelled {
			result.Status = MissionStatusCancelled
			mission.Error = "workflow execution was cancelled"
		}
	}

	// Update mission to completed
	completedAt := time.Now()
	mission.Status = MissionStatusCompleted
	mission.CompletedAt = &completedAt
	mission.Metrics.Duration = completedAt.Sub(startedAt)

	if err := o.store.Update(ctx, mission); err != nil {
		return nil, fmt.Errorf("failed to update mission completion: %w", err)
	}

	// Emit completion event
	o.emitter.Emit(ctx, MissionEvent{
		Type:      EventMissionCompleted,
		MissionID: mission.ID,
		Timestamp: completedAt,
		Payload: map[string]interface{}{
			"duration": mission.Metrics.Duration.String(),
		},
	})

	result.CompletedAt = completedAt
	return result, nil
}

// executeWithConstraints wraps workflow execution with periodic constraint checking.
// It runs checkConstraints every 5 seconds in a goroutine and cancels the workflow
// execution if any constraint is violated.
func (o *DefaultMissionOrchestrator) executeWithConstraints(
	ctx context.Context,
	wf *workflow.Workflow,
	harness harness.AgentHarness,
	mission *Mission,
) (*workflow.WorkflowResult, error) {
	// If no constraints are configured, execute normally
	if mission.Constraints == nil {
		return o.workflowExecutor.Execute(ctx, wf, harness)
	}

	// Create cancellable context for workflow execution
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel to receive workflow result
	type workflowOutcome struct {
		result *workflow.WorkflowResult
		err    error
	}
	resultChan := make(chan workflowOutcome, 1)

	// Start workflow execution in goroutine
	go func() {
		result, err := o.workflowExecutor.Execute(cancelCtx, wf, harness)
		resultChan <- workflowOutcome{result: result, err: err}
	}()

	// Start constraint checking goroutine
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Main select loop
	for {
		select {
		case <-ctx.Done():
			// Parent context cancelled - propagate cancellation
			cancel()
			// Wait for workflow to finish with cancelled context
			outcome := <-resultChan
			return outcome.result, ctx.Err()

		case <-ticker.C:
			// Periodic constraint check
			// Update metrics duration before checking
			if mission.Metrics != nil {
				mission.Metrics.Duration = time.Since(mission.Metrics.StartedAt)
			}

			violation, err := o.checkConstraints(ctx, mission)
			if err != nil {
				// Log error but don't fail the workflow
				o.emitter.Emit(ctx, MissionEvent{
					Type:      "constraint_check_error",
					MissionID: mission.ID,
					Timestamp: time.Now(),
					Payload: map[string]interface{}{
						"error": err.Error(),
					},
				})
				continue
			}

			if violation != nil {
				// Constraint violated - cancel workflow execution
				cancel()

				// Emit constraint violation event
				o.emitter.Emit(ctx, MissionEvent{
					Type:      "constraint_violated",
					MissionID: mission.ID,
					Timestamp: time.Now(),
					Payload: map[string]interface{}{
						"constraint": violation.Constraint,
						"message":    violation.Message,
						"action":     violation.Action.String(),
					},
				})

				// Wait for workflow to finish with cancelled context
				outcome := <-resultChan

				// Handle based on constraint action
				if violation.Action == ConstraintActionPause {
					// Update mission status to paused
					mission.Status = MissionStatusPaused
					mission.Error = fmt.Sprintf("constraint violation: %s", violation.Message)

					// Return partial results with findings collected so far
					if outcome.result != nil {
						return outcome.result, fmt.Errorf("mission paused due to constraint violation: %w", violation)
					}
					return nil, fmt.Errorf("mission paused due to constraint violation: %w", violation)
				} else {
					// ConstraintActionFail - update mission status to failed
					mission.Status = MissionStatusFailed
					mission.Error = fmt.Sprintf("constraint violation: %s", violation.Message)

					// Return partial results with findings collected so far
					if outcome.result != nil {
						return outcome.result, fmt.Errorf("mission failed due to constraint violation: %w", violation)
					}
					return nil, fmt.Errorf("mission failed due to constraint violation: %w", violation)
				}
			}

		case outcome := <-resultChan:
			// Workflow completed (successfully, failed, or cancelled) before constraints violated
			// Perform final constraint check
			if mission.Metrics != nil {
				mission.Metrics.Duration = time.Since(mission.Metrics.StartedAt)
			}

			finalViolation, err := o.checkConstraints(ctx, mission)
			if err != nil {
				// Log error but return workflow result
				o.emitter.Emit(ctx, MissionEvent{
					Type:      "constraint_check_error",
					MissionID: mission.ID,
					Timestamp: time.Now(),
					Payload: map[string]interface{}{
						"error": err.Error(),
					},
				})
			}

			// If we have a final violation, handle it
			if finalViolation != nil {
				if finalViolation.Action == ConstraintActionPause {
					mission.Status = MissionStatusPaused
					mission.Error = fmt.Sprintf("constraint violation: %s", finalViolation.Message)
				} else {
					mission.Status = MissionStatusFailed
					mission.Error = fmt.Sprintf("constraint violation: %s", finalViolation.Message)
				}

				return outcome.result, fmt.Errorf("constraint violation: %w", finalViolation)
			}

			// No violations - return normal workflow result
			return outcome.result, outcome.err
		}
	}
}

// checkConstraints checks if any constraints are violated.
func (o *DefaultMissionOrchestrator) checkConstraints(ctx context.Context, mission *Mission) (*ConstraintViolation, error) {
	if mission.Constraints == nil {
		return nil, nil
	}

	checker := NewDefaultConstraintChecker()
	return checker.Check(ctx, mission.Constraints, mission.Metrics)
}

// Ensure DefaultMissionOrchestrator implements MissionOrchestrator.
var _ MissionOrchestrator = (*DefaultMissionOrchestrator)(nil)
