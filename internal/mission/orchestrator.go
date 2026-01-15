package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// TargetStore provides access to target entities needed by the orchestrator.
// This interface allows the orchestrator to load full target details including
// connection parameters that agents need for testing.
type TargetStore interface {
	// Get retrieves a target by ID, returning the full target entity with connection details
	Get(ctx context.Context, id types.ID) (*types.Target, error)
}

// MissionOrchestrator coordinates mission execution.
type MissionOrchestrator interface {
	// Execute runs the mission workflow and manages all orchestration
	Execute(ctx context.Context, mission *Mission) (*MissionResult, error)
}

// EventBusPublisher is an interface for publishing daemon-wide events.
// This allows the orchestrator to publish to the daemon's event bus
// without creating a circular dependency.
type EventBusPublisher interface {
	Publish(ctx context.Context, event interface{}) error
}

// DefaultMissionOrchestrator implements MissionOrchestrator.
type DefaultMissionOrchestrator struct {
	store                MissionStore
	targetStore          TargetStore // For loading full target entities with connection details
	emitter              EventEmitter
	eventStore           EventStore
	eventBus             EventBusPublisher // For publishing to daemon event bus
	workflowExecutor     *workflow.WorkflowExecutor
	harnessFactory       harness.HarnessFactoryInterface
	missionService       MissionService // For loading workflows from store
	evalOptions          *eval.EvalOptions
	evalCollector        *eval.EvalResultCollector

	// Pause request handling
	pauseRequestedMu sync.RWMutex
	pauseRequested   map[types.ID]bool
}

// OrchestratorOption is a functional option for configuring the orchestrator.
type OrchestratorOption func(*DefaultMissionOrchestrator)

// WithEventEmitter sets the event emitter.
func WithEventEmitter(emitter EventEmitter) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.emitter = emitter
	}
}

// WithEventBus sets the event bus for publishing daemon-wide events.
func WithEventBus(eventBus EventBusPublisher) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.eventBus = eventBus
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

// WithMissionService sets the mission service for workflow loading.
func WithMissionService(service MissionService) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.missionService = service
	}
}


// WithEventStore sets the event store for event persistence.
func WithEventStore(store EventStore) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.eventStore = store
	}
}

// WithTargetStore sets the target store for loading target entities.
func WithTargetStore(store TargetStore) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.targetStore = store
	}
}

// NewMissionOrchestrator creates a new mission orchestrator.
func NewMissionOrchestrator(store MissionStore, opts ...OrchestratorOption) (*DefaultMissionOrchestrator, error) {
	o := &DefaultMissionOrchestrator{
		store:          store,
		emitter:        NewDefaultEventEmitter(WithBufferSize(100)), // Default buffer size
		pauseRequested: make(map[types.ID]bool),
	}

	for _, opt := range opts {
		opt(o)
	}

	// If eval options are set and harness factory exists, wrap it with eval capabilities
	if o.evalOptions != nil && o.harnessFactory != nil {
		wrappedFactory, collector, err := wrapFactoryWithEval(o.harnessFactory, o.evalOptions)
		if err != nil {
			// Log error but continue - eval is optional
			// In production, this would use proper logging
			// For now, we'll just skip eval wrapping on error
		} else if wrappedFactory != nil {
			o.harnessFactory = wrappedFactory
			o.evalCollector = collector
		}
	}

	return o, nil
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

	// Emit and persist mission started event
	startedPayload := make(map[string]interface{})
	if mission.WorkflowID != "" {
		startedPayload["workflow_id"] = mission.WorkflowID.String()
	}
	if mission.TargetID != "" {
		startedPayload["target_id"] = mission.TargetID.String()
	}

	o.emitAndPersist(ctx, MissionEvent{
		Type:      EventMissionStarted,
		MissionID: mission.ID,
		Timestamp: startedAt,
		Payload:   startedPayload,
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

	// Emit event that workflow execution is starting
	o.emitAndPersist(ctx, MissionEvent{
		Type:      "workflow_started",
		MissionID: mission.ID,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"message": "Mission workflow execution started",
		},
	})

	// Check if workflow executor is configured
	if o.workflowExecutor == nil {
		// No workflow executor configured - complete immediately
		// This is the minimal execution path for missions without workflow definitions
		time.Sleep(100 * time.Millisecond)
	} else {
		// Parse workflow from mission's workflow definition or load from store
		var wf *workflow.Workflow

		// First check if we have a WorkflowID and can load from store
		if mission.WorkflowID != "" && o.missionService != nil {
			// Load workflow from store using WorkflowID
			// Convert WorkflowID to reference string for the config
			workflowConfig := &MissionWorkflowConfig{
				Reference: mission.WorkflowID.String(),
			}

			loadedWorkflow, loadErr := o.missionService.LoadWorkflow(ctx, workflowConfig)
			if loadErr != nil {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = fmt.Sprintf("failed to load workflow from store: %v", loadErr)
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				// Emit and persist mission failed event
				o.emitAndPersist(ctx, NewFailedEvent(mission.ID, loadErr))

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow load error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to load workflow from store: %w", loadErr)
			}

			// Type assert to workflow.Workflow
			var ok bool
			wf, ok = loadedWorkflow.(*workflow.Workflow)
			if !ok {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = "loaded workflow has invalid type"
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				// Emit and persist mission failed event
				o.emitAndPersist(ctx, NewFailedEvent(mission.ID, fmt.Errorf("loaded workflow has invalid type")))

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow type error: %w", updateErr)
				}

				return result, fmt.Errorf("loaded workflow has invalid type")
			}
		} else if mission.WorkflowJSON != "" {
			// Fall back to inline workflow JSON/YAML
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

				// Emit and persist mission failed event
				o.emitAndPersist(ctx, NewFailedEvent(mission.ID, workflowErr))

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow parse error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to parse workflow: %w", workflowErr)
			}
		} else {
			// No workflow ID and no inline JSON - cannot proceed
			result.Status = MissionStatusFailed
			mission.Status = MissionStatusFailed
			mission.Error = "workflow definition not available (neither WorkflowID nor WorkflowJSON specified)"
			completedAt := time.Now()
			mission.CompletedAt = &completedAt
			mission.Metrics.Duration = completedAt.Sub(startedAt)

			// Emit and persist mission failed event
			o.emitAndPersist(ctx, NewFailedEvent(mission.ID, fmt.Errorf("workflow definition not available in mission")))

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

			// Emit and persist mission failed event
			o.emitAndPersist(ctx, NewFailedEvent(mission.ID, fmt.Errorf("harness factory not configured")))

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after harness factory error: %w", updateErr)
			}

			return result, fmt.Errorf("harness factory not configured")
		}

		// Create mission context and target info for harness
		missionCtx := harness.NewMissionContext(mission.ID, mission.Name, "")

		// Load target entity to get connection details
		var targetInfo harness.TargetInfo
		if mission.TargetID == "00000000-0000-0000-0000-d15c00e00000" {
			// Synthetic target for discovery/orchestration missions - no actual target to load
			targetInfo = harness.NewTargetInfo(mission.TargetID, "discovery-mission", "", "discovery")
		} else if o.targetStore != nil {
			target, err := o.targetStore.Get(ctx, mission.TargetID)
			if err != nil {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = fmt.Sprintf("failed to load target: %v", err)
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				// Emit and persist mission failed event
				o.emitAndPersist(ctx, NewFailedEvent(mission.ID, err))

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after target load error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to load target: %w", err)
			}

			// Create TargetInfo with full connection details from target entity
			targetInfo = harness.NewTargetInfoFull(
				target.ID,
				target.Name,
				target.URL,
				target.Type,
				target.Connection, // Connection contains schema-based connection parameters
			)
		} else {
			// Fallback if no target store configured (backward compatibility)
			targetInfo = harness.NewTargetInfo(mission.TargetID, "mission-target", "", "")
		}

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

			// Emit and persist mission failed event
			o.emitAndPersist(ctx, NewFailedEvent(mission.ID, err))

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after harness creation error: %w", updateErr)
			}

			return result, fmt.Errorf("failed to create harness: %w", err)
		}
		// Note: AgentHarness doesn't have a Close method, so no cleanup needed

		// Execute workflow with constraint checking
		workflowResult, workflowErr = o.executeWithConstraints(ctx, wf, agentHarness, mission)

		// Propagate workflow metrics to mission metrics
		if workflowResult != nil {
			mission.Metrics.CompletedNodes = workflowResult.NodesExecuted
			mission.Metrics.LastUpdateAt = time.Now()
		}

		// Collect findings from workflow result
		if workflowResult != nil && len(workflowResult.Findings) > 0 {
			for _, finding := range workflowResult.Findings {
				result.FindingIDs = append(result.FindingIDs, finding.ID)
			}
		}

		// Emit progress event with completed node information
		if workflowResult != nil {
			totalNodes := len(wf.Nodes)
			completedNodes := workflowResult.NodesExecuted
			percentComplete := float64(0)
			if totalNodes > 0 {
				percentComplete = (float64(completedNodes) / float64(totalNodes)) * 100
			}

			progress := &MissionProgress{
				MissionID:       mission.ID,
				Status:          mission.Status,
				PercentComplete: percentComplete,
				CompletedNodes:  completedNodes,
				TotalNodes:      totalNodes,
				FindingsCount:   len(result.FindingIDs),
			}

			o.emitAndPersist(ctx, NewProgressEvent(mission.ID, progress, fmt.Sprintf("Completed %d/%d nodes", completedNodes, totalNodes)))
		}

		// Check for workflow execution errors or failures
		if workflowErr != nil || (workflowResult != nil && workflowResult.Status == workflow.WorkflowStatusFailed) {
			result.Status = MissionStatusFailed
			if workflowErr != nil {
				mission.Error = fmt.Sprintf("workflow execution error: %v", workflowErr)
			} else if workflowResult.Error != nil {
				mission.Error = fmt.Sprintf("workflow failed: %v", workflowResult.Error.Message)
			}

			// Emit and persist mission failed event
			o.emitAndPersist(ctx, NewFailedEvent(mission.ID, fmt.Errorf("%s", mission.Error)))
		} else if workflowResult != nil && workflowResult.Status == workflow.WorkflowStatusCancelled {
			result.Status = MissionStatusCancelled
			mission.Error = "workflow execution was cancelled"

			// Emit and persist mission cancelled event
			o.emitAndPersist(ctx, NewCancelledEvent(mission.ID, mission.Error))
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

	// Emit and persist completion event
	completedPayload := map[string]interface{}{
		"duration":    mission.Metrics.Duration.Milliseconds(),
		"duration_ms": mission.Metrics.Duration.Milliseconds(),
		"status":      string(result.Status),
	}
	if workflowResult != nil {
		completedPayload["nodes_executed"] = workflowResult.NodesExecuted
		completedPayload["finding_count"] = len(result.FindingIDs)
	}

	o.emitAndPersist(ctx, MissionEvent{
		Type:      EventMissionCompleted,
		MissionID: mission.ID,
		Timestamp: completedAt,
		Payload:   completedPayload,
	})

	result.CompletedAt = completedAt

	// Populate WorkflowResult so attack runner can check for node failures
	if workflowResult != nil {
		// Convert workflow result to map[string]any for storage
		workflowResultMap := make(map[string]any)
		workflowResultMap["workflow_id"] = workflowResult.WorkflowID.String()
		workflowResultMap["status"] = string(workflowResult.Status)
		workflowResultMap["nodes_executed"] = workflowResult.NodesExecuted
		workflowResultMap["nodes_failed"] = workflowResult.NodesFailed
		workflowResultMap["nodes_skipped"] = workflowResult.NodesSkipped
		workflowResultMap["total_duration"] = workflowResult.TotalDuration.String()

		// Convert node results
		if workflowResult.NodeResults != nil {
			nodeResultsMap := make(map[string]any)
			for nodeID, nodeResult := range workflowResult.NodeResults {
				nodeMap := make(map[string]any)
				nodeMap["node_id"] = nodeResult.NodeID
				nodeMap["status"] = string(nodeResult.Status)
				nodeMap["duration"] = nodeResult.Duration.String()
				nodeMap["retry_count"] = nodeResult.RetryCount
				if nodeResult.Output != nil {
					nodeMap["output"] = nodeResult.Output
				}
				if nodeResult.Error != nil {
					nodeMap["error"] = map[string]any{
						"code":    nodeResult.Error.Code,
						"message": nodeResult.Error.Message,
					}
				}
				nodeResultsMap[nodeID] = nodeMap
			}
			workflowResultMap["node_results"] = nodeResultsMap
		}

		// Add error if present
		if workflowResult.Error != nil {
			workflowResultMap["error"] = map[string]any{
				"code":    string(workflowResult.Error.Code),
				"message": workflowResult.Error.Message,
			}
		}

		result.WorkflowResult = workflowResultMap
	}

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

			// Emit and persist mission cancelled event
			o.emitAndPersist(context.Background(), NewCancelledEvent(mission.ID, "context cancelled"))

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
				o.emitAndPersist(ctx, MissionEvent{
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

				// Emit and persist constraint violation event
				o.emitAndPersist(ctx, MissionEvent{
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
				o.emitAndPersist(ctx, MissionEvent{
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

// emitAndPersist emits an event and persists it to the event store if configured.
// It also publishes to the daemon event bus if configured.
// Emission to emitter happens regardless of persistence success.
// Persistence failures are logged but do not block execution.
func (o *DefaultMissionOrchestrator) emitAndPersist(ctx context.Context, event MissionEvent) {
	// Always emit to the event emitter for real-time subscribers
	o.emitter.Emit(ctx, event)

	// Publish to daemon event bus if configured
	if o.eventBus != nil {
		// The event bus takes interface{}, but we need to ensure it's the right type
		// Convert to the type expected by daemon event bus (which is the interface passed)
		o.publishToEventBus(ctx, event)
	}

	// Persist to event store if configured
	if o.eventStore != nil {
		if err := o.eventStore.Append(ctx, &event); err != nil {
			// Log error but don't fail execution
			// Event persistence is important but not critical enough to stop the mission
			// In production, this would use a proper logger
			// For now, we emit an error event
			o.emitter.Emit(ctx, MissionEvent{
				Type:      "event_persistence_error",
				MissionID: event.MissionID,
				Timestamp: time.Now(),
				Payload: map[string]interface{}{
					"error":      err.Error(),
					"event_type": event.Type,
				},
			})
		}
	}
}

// publishToEventBus converts a MissionEvent to daemon api.EventData and publishes to the event bus.
// This method handles the conversion and gracefully handles errors.
func (o *DefaultMissionOrchestrator) publishToEventBus(ctx context.Context, event MissionEvent) {
	// Note: We can't import daemon/api here due to circular dependency
	// So we create a compatible structure that will match api.EventData
	// The EventBusPublisher interface accepts interface{} to avoid circular dependency

	// Create event data structure that matches api.EventData
	eventData := struct {
		EventType    string
		Timestamp    time.Time
		Source       string
		Data         string
		Metadata     map[string]interface{}
		MissionEvent *struct {
			EventType string
			Timestamp time.Time
			MissionID string
			NodeID    string
			Message   string
			Data      string
			Error     string
			Result    interface{}
			Payload   map[string]interface{}
		}
		AttackEvent  interface{}
		AgentEvent   interface{}
		FindingEvent interface{}
	}{
		EventType: string(event.Type),
		Timestamp: event.Timestamp,
		Source:    "mission-orchestrator",
		Metadata:  make(map[string]interface{}),
		MissionEvent: &struct {
			EventType string
			Timestamp time.Time
			MissionID string
			NodeID    string
			Message   string
			Data      string
			Error     string
			Result    interface{}
			Payload   map[string]interface{}
		}{
			EventType: string(event.Type),
			Timestamp: event.Timestamp,
			MissionID: event.MissionID.String(),
			Payload:   convertPayload(event.Payload),
		},
	}

	// Extract trace context if available in context
	// TODO: Add OpenTelemetry trace extraction when available
	// For now, trace context will be added by the workflow executor

	// Publish to event bus
	if err := o.eventBus.Publish(ctx, eventData); err != nil {
		// Log error but don't fail mission execution
		// Event bus publishing is best-effort
		// In production, this would use proper logging
	}
}

// convertPayload converts an interface{} payload to map[string]interface{}.
// Returns empty map if payload is nil or not a map.
func convertPayload(payload interface{}) map[string]interface{} {
	if payload == nil {
		return make(map[string]interface{})
	}
	if m, ok := payload.(map[string]interface{}); ok {
		return m
	}
	return make(map[string]interface{})
}

// Ensure DefaultMissionOrchestrator implements MissionOrchestrator.
// RequestPause signals the orchestrator to pause at the next clean boundary.
func (o *DefaultMissionOrchestrator) RequestPause(ctx context.Context, missionID types.ID) error {
	o.pauseRequestedMu.Lock()
	defer o.pauseRequestedMu.Unlock()

	o.pauseRequested[missionID] = true
	return nil
}

// IsPauseRequested checks if pause has been requested for a mission.
func (o *DefaultMissionOrchestrator) IsPauseRequested(missionID types.ID) bool {
	o.pauseRequestedMu.RLock()
	defer o.pauseRequestedMu.RUnlock()

	return o.pauseRequested[missionID]
}

// ClearPauseRequest clears the pause request flag for a mission.
func (o *DefaultMissionOrchestrator) ClearPauseRequest(missionID types.ID) {
	o.pauseRequestedMu.Lock()
	defer o.pauseRequestedMu.Unlock()

	delete(o.pauseRequested, missionID)
}

// ExecuteFromCheckpoint resumes execution from a saved checkpoint.
func (o *DefaultMissionOrchestrator) ExecuteFromCheckpoint(ctx context.Context, mission *Mission, checkpoint *MissionCheckpoint) (*MissionResult, error) {
	// Validate mission can be resumed
	if !mission.Status.CanTransitionTo(MissionStatusRunning) {
		return nil, NewInvalidStateError(mission.Status, MissionStatusRunning)
	}

	// Update mission status to running
	startedAt := time.Now()
	mission.Status = MissionStatusRunning
	mission.StartedAt = &startedAt

	// Initialize or restore metrics
	if mission.Metrics == nil {
		mission.Metrics = &MissionMetrics{
			StartedAt:          startedAt,
			LastUpdateAt:       startedAt,
			FindingsBySeverity: make(map[string]int),
		}
	} else {
		mission.Metrics.LastUpdateAt = startedAt
	}

	if err := o.store.Update(ctx, mission); err != nil {
		return nil, fmt.Errorf("failed to update mission status: %w", err)
	}

	// Emit mission resumed event
	resumedEvent := NewResumedEvent(mission.ID)
	o.emitAndPersist(ctx, resumedEvent)

	// Create result
	result := &MissionResult{
		MissionID:  mission.ID,
		Status:     MissionStatusCompleted,
		Metrics:    mission.Metrics,
		FindingIDs: []types.ID{},
	}

	// Execute workflow from checkpoint
	var workflowResult *workflow.WorkflowResult
	var workflowErr error

	// Check if workflow executor is configured
	if o.workflowExecutor == nil {
		// For now, just simulate execution without workflow executor
		time.Sleep(100 * time.Millisecond)
	} else {
		// Parse workflow from mission's workflow definition or load from store
		var wf *workflow.Workflow

		// First check if we have a WorkflowID and can load from store
		if mission.WorkflowID != "" && o.missionService != nil {
			// Load workflow from store using WorkflowID
			workflowConfig := &MissionWorkflowConfig{
				Reference: mission.WorkflowID.String(),
			}

			loadedWorkflow, loadErr := o.missionService.LoadWorkflow(ctx, workflowConfig)
			if loadErr != nil {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = fmt.Sprintf("failed to load workflow from store: %v", loadErr)
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				failedEvent := NewFailedEvent(mission.ID, loadErr)
				o.emitAndPersist(ctx, failedEvent)

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow load error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to load workflow from store: %w", loadErr)
			}

			var ok bool
			wf, ok = loadedWorkflow.(*workflow.Workflow)
			if !ok {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = "loaded workflow has invalid type"
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				failedEvent := NewFailedEvent(mission.ID, fmt.Errorf("loaded workflow has invalid type"))
				o.emitAndPersist(ctx, failedEvent)

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow type error: %w", updateErr)
				}

				return result, fmt.Errorf("loaded workflow has invalid type")
			}
		} else if mission.WorkflowJSON != "" {
			// Fall back to inline workflow JSON/YAML
			workflowContent := strings.TrimSpace(mission.WorkflowJSON)
			if strings.HasPrefix(workflowContent, "{") {
				wf = &workflow.Workflow{}
				workflowErr = json.Unmarshal([]byte(workflowContent), wf)
			} else {
				wf, workflowErr = workflow.ParseWorkflow([]byte(mission.WorkflowJSON))
			}

			if workflowErr != nil {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = fmt.Sprintf("failed to parse workflow: %v", workflowErr)
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				failedEvent := NewFailedEvent(mission.ID, workflowErr)
				o.emitAndPersist(ctx, failedEvent)

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after workflow parse error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to parse workflow: %w", workflowErr)
			}
		} else {
			result.Status = MissionStatusFailed
			mission.Status = MissionStatusFailed
			mission.Error = "workflow definition not available (neither WorkflowID nor WorkflowJSON specified)"
			completedAt := time.Now()
			mission.CompletedAt = &completedAt
			mission.Metrics.Duration = completedAt.Sub(startedAt)

			failedEvent := NewFailedEvent(mission.ID, fmt.Errorf("workflow definition not available in mission"))
			o.emitAndPersist(ctx, failedEvent)

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

			failedEvent := NewFailedEvent(mission.ID, fmt.Errorf("harness factory not configured"))
			o.emitAndPersist(ctx, failedEvent)

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after harness factory error: %w", updateErr)
			}

			return result, fmt.Errorf("harness factory not configured")
		}

		// Create mission context and target info for harness
		missionCtx := harness.NewMissionContext(mission.ID, mission.Name, "")

		// Load target entity to get connection details
		var targetInfo harness.TargetInfo
		if mission.TargetID == "00000000-0000-0000-0000-d15c00e00000" {
			// Synthetic target for discovery/orchestration missions - no actual target to load
			targetInfo = harness.NewTargetInfo(mission.TargetID, "discovery-mission", "", "discovery")
		} else if o.targetStore != nil {
			target, err := o.targetStore.Get(ctx, mission.TargetID)
			if err != nil {
				result.Status = MissionStatusFailed
				mission.Status = MissionStatusFailed
				mission.Error = fmt.Sprintf("failed to load target: %v", err)
				completedAt := time.Now()
				mission.CompletedAt = &completedAt
				mission.Metrics.Duration = completedAt.Sub(startedAt)

				failedEvent := NewFailedEvent(mission.ID, err)
				o.emitAndPersist(ctx, failedEvent)

				if updateErr := o.store.Update(ctx, mission); updateErr != nil {
					return nil, fmt.Errorf("failed to update mission after target load error: %w", updateErr)
				}

				return result, fmt.Errorf("failed to load target: %w", err)
			}

			// Create TargetInfo with full connection details from target entity
			targetInfo = harness.NewTargetInfoFull(
				target.ID,
				target.Name,
				target.URL,
				target.Type,
				target.Connection, // Connection contains schema-based connection parameters
			)
		} else {
			// Fallback if no target store configured (backward compatibility)
			targetInfo = harness.NewTargetInfo(mission.TargetID, "mission-target", "", "")
		}

		// Create harness for workflow execution
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

			failedEvent := NewFailedEvent(mission.ID, err)
			o.emitAndPersist(ctx, failedEvent)

			if updateErr := o.store.Update(ctx, mission); updateErr != nil {
				return nil, fmt.Errorf("failed to update mission after harness creation error: %w", updateErr)
			}

			return result, fmt.Errorf("failed to create harness: %w", err)
		}

		// Execute workflow with constraint checking, resuming from checkpoint state
		// Note: The workflow executor would need to support resuming from checkpoint state
		// For now, we'll execute normally and trust that completed nodes won't be re-executed
		workflowResult, workflowErr = o.executeWithConstraints(ctx, wf, agentHarness, mission)

		// Propagate workflow metrics to mission metrics
		if workflowResult != nil {
			mission.Metrics.CompletedNodes = workflowResult.NodesExecuted
			mission.Metrics.LastUpdateAt = time.Now()
		}

		// Collect findings from workflow result
		if workflowResult != nil && len(workflowResult.Findings) > 0 {
			for _, finding := range workflowResult.Findings {
				result.FindingIDs = append(result.FindingIDs, finding.ID)
			}
		}

		// Emit progress event with completed node information
		if workflowResult != nil {
			totalNodes := len(wf.Nodes)
			completedNodes := workflowResult.NodesExecuted
			percentComplete := float64(0)
			if totalNodes > 0 {
				percentComplete = (float64(completedNodes) / float64(totalNodes)) * 100
			}

			progress := &MissionProgress{
				MissionID:       mission.ID,
				Status:          mission.Status,
				PercentComplete: percentComplete,
				CompletedNodes:  completedNodes,
				TotalNodes:      totalNodes,
				FindingsCount:   len(result.FindingIDs),
			}

			progressEvent := NewProgressEvent(mission.ID, progress, fmt.Sprintf("Completed %d/%d nodes", completedNodes, totalNodes))
			o.emitAndPersist(ctx, progressEvent)
		}

		// Check for workflow execution errors or failures
		if workflowErr != nil || (workflowResult != nil && workflowResult.Status == workflow.WorkflowStatusFailed) {
			result.Status = MissionStatusFailed
			if workflowErr != nil {
				mission.Error = fmt.Sprintf("workflow execution error: %v", workflowErr)
			} else if workflowResult.Error != nil {
				mission.Error = fmt.Sprintf("workflow failed: %v", workflowResult.Error.Message)
			}

			failedEvent := NewFailedEvent(mission.ID, fmt.Errorf("%s", mission.Error))
			o.emitAndPersist(ctx, failedEvent)
		} else if workflowResult != nil && workflowResult.Status == workflow.WorkflowStatusCancelled {
			result.Status = MissionStatusCancelled
			mission.Error = "workflow execution was cancelled"

			cancelledEvent := NewCancelledEvent(mission.ID, mission.Error)
			o.emitAndPersist(ctx, cancelledEvent)
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
	completionEvent := MissionEvent{
		Type:      EventMissionCompleted,
		MissionID: mission.ID,
		Timestamp: completedAt,
		Payload: map[string]interface{}{
			"duration": mission.Metrics.Duration.String(),
		},
	}
	o.emitAndPersist(ctx, completionEvent)

	result.CompletedAt = completedAt
	return result, nil
}

// Ensure DefaultMissionOrchestrator implements MissionOrchestrator.
var _ MissionOrchestrator = (*DefaultMissionOrchestrator)(nil)
