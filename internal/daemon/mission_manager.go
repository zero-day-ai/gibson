package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/observability"
	"github.com/zero-day-ai/gibson/internal/orchestrator"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// targetStore is an interface for target lookup in mission manager
type targetStoreLookup interface {
	GetByName(ctx context.Context, name string) (*types.Target, error)
}

// missionManager implements the MissionManager interface for daemon operations.
// It orchestrates mission lifecycle including workflow loading, execution, tracking,
// and event emission.
type missionManager struct {
	config          *config.Config
	logger          *slog.Logger
	registry        registry.ComponentDiscovery
	missionStore    mission.MissionStore
	findingStore    finding.FindingStore
	llmRegistry     llm.LLMRegistry
	callbackManager *harness.CallbackManager
	harnessFactory  harness.HarnessFactoryInterface
	targetStore     targetStoreLookup
	runLinker       mission.MissionRunLinker
	infrastructure  *Infrastructure
	graphLoader     *workflow.GraphLoader // GraphLoader for storing workflows in Neo4j

	// Track active missions with their contexts and event channels
	mu             sync.RWMutex
	activeMissions map[string]*activeMission
	completedCount int
}

// activeMission tracks a running mission with its context and event channel
type activeMission struct {
	mission       *mission.Mission
	ctx           context.Context
	cancel        context.CancelFunc
	eventChan     chan api.MissionEventData
	workflowState *workflow.WorkflowState
	startTime     time.Time
}

// newMissionManager creates a new mission manager instance.
func newMissionManager(
	cfg *config.Config,
	logger *slog.Logger,
	reg registry.ComponentDiscovery,
	missionStore mission.MissionStore,
	findingStore finding.FindingStore,
	llmRegistry llm.LLMRegistry,
	callbackMgr *harness.CallbackManager,
	harnessFactory harness.HarnessFactoryInterface,
	targetStore targetStoreLookup,
	runLinker mission.MissionRunLinker,
	infrastructure *Infrastructure,
) *missionManager {
	// Create GraphLoader if Neo4j client is available
	var graphLoader *workflow.GraphLoader
	if infrastructure != nil && infrastructure.graphRAGClient != nil {
		graphLoader = workflow.NewGraphLoader(infrastructure.graphRAGClient)
		logger.Info("initialized GraphLoader for workflow persistence to Neo4j")
	}

	return &missionManager{
		config:          cfg,
		logger:          logger.With("component", "mission-manager"),
		registry:        reg,
		missionStore:    missionStore,
		findingStore:    findingStore,
		llmRegistry:     llmRegistry,
		callbackManager: callbackMgr,
		harnessFactory:  harnessFactory,
		targetStore:     targetStore,
		runLinker:       runLinker,
		infrastructure:  infrastructure,
		graphLoader:     graphLoader,
		activeMissions:  make(map[string]*activeMission),
	}
}

// Run starts a mission and returns an event channel for progress updates.
// This implements the core mission execution flow:
// 1. Load workflow from file
// 2. Create mission context and stores
// 3. Launch workflow executor in goroutine
// 4. Return event channel for streaming updates
func (m *missionManager) Run(ctx context.Context, workflowPath string, missionID string, variables map[string]string, memoryContinuity string) (<-chan api.MissionEventData, error) {
	m.logger.Info("starting mission",
		"workflow_path", workflowPath,
		"mission_id", missionID,
		"variables", len(variables),
		"memory_continuity", memoryContinuity,
	)

	// Load workflow from YAML file
	wf, err := workflow.ParseWorkflowFile(workflowPath)
	if err != nil {
		m.logger.Error("failed to load workflow", "error", err, "path", workflowPath)
		return nil, fmt.Errorf("failed to load workflow from %s: %w", workflowPath, err)
	}

	m.logger.Debug("workflow loaded",
		"workflow_name", wf.Name,
		"node_count", len(wf.Nodes),
	)

	// Generate mission ID if not provided
	if missionID == "" {
		missionID = types.NewID().String()
	}

	// IMPORTANT: Set workflow ID to match mission ID
	// This ensures the GraphLoader creates the Mission node with the same ID
	// that will be used in subsequent events (mission.started, agent.started, etc.)
	// Without this, we'd have two different Mission nodes with different IDs.
	missionIDTyped, err := types.ParseID(missionID)
	if err != nil {
		m.logger.Error("invalid mission ID format", "error", err, "mission_id", missionID)
		return nil, fmt.Errorf("invalid mission ID format: %w", err)
	}
	wf.ID = missionIDTyped

	// Store workflow in Neo4j GraphRAG for state tracking and cross-mission analysis
	if m.graphLoader != nil {
		graphMissionID, err := m.graphLoader.LoadWorkflow(ctx, wf)
		if err != nil {
			// Log warning but continue - graph storage is optional for execution
			m.logger.Warn("failed to store workflow in GraphRAG, continuing without graph persistence",
				"error", err,
				"mission_id", missionID,
				"workflow_name", wf.Name,
			)
		} else {
			m.logger.Info("workflow stored in GraphRAG",
				"graph_mission_id", graphMissionID,
				"mission_id", missionID,
				"workflow_name", wf.Name,
				"node_count", len(wf.Nodes),
				"edge_count", len(wf.Edges),
			)
		}
	}

	// Check if mission ID already exists
	m.mu.RLock()
	if _, exists := m.activeMissions[missionID]; exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("mission %s is already running", missionID)
	}
	m.mu.RUnlock()

	// Resolve target from workflow
	var targetID types.ID
	if wf.TargetRef != "" {
		if m.targetStore == nil {
			return nil, fmt.Errorf("target '%s' specified but target store not available", wf.TargetRef)
		}
		target, err := m.targetStore.GetByName(ctx, wf.TargetRef)
		if err != nil {
			m.logger.Error("failed to lookup target", "error", err, "target_ref", wf.TargetRef)
			return nil, fmt.Errorf("failed to lookup target '%s': %w", wf.TargetRef, err)
		}
		if target == nil {
			return nil, fmt.Errorf("target '%s' not found", wf.TargetRef)
		}
		targetID = target.ID
		m.logger.Debug("resolved target", "target_ref", wf.TargetRef, "target_id", targetID)
	} else {
		// No target specified - use a synthetic "discovery" target ID
		// This allows orchestration/discovery missions that don't target a specific system
		// Use a well-known UUID that signals this is a discovery mission
		targetID = types.ID("00000000-0000-0000-0000-d15c00e00000")
		m.logger.Debug("no target specified, using discovery target", "target_id", targetID)
	}

	// Serialize workflow to JSON for orchestrator
	workflowJSON, err := json.Marshal(wf)
	if err != nil {
		m.logger.Error("failed to serialize workflow", "error", err)
		return nil, fmt.Errorf("failed to serialize workflow: %w", err)
	}

	// Create mission record with Pending status (orchestrator will transition to Running)
	missionRecord := &mission.Mission{
		ID:               types.ID(missionID),
		Name:             wf.Name,
		Description:      wf.Description,
		Status:           mission.MissionStatusPending,
		WorkflowID:       wf.ID,
		WorkflowJSON:     string(workflowJSON),
		TargetID:         targetID,
		MemoryContinuity: memoryContinuity,
		CreatedAt:        time.Now(),
		FindingsCount:    0,
		Metrics: &mission.MissionMetrics{
			TotalNodes:     len(wf.Nodes),
			CompletedNodes: 0,
		},
		Metadata: make(map[string]any),
	}

	// Store variables in metadata
	if len(variables) > 0 {
		missionRecord.Metadata["variables"] = variables
	}

	// Save mission to store using run linker if available
	if m.runLinker != nil {
		if err := m.runLinker.CreateRun(ctx, wf.Name, missionRecord); err != nil {
			// Check if error is about active mission
			if activeErr, ok := err.(error); ok && activeErr != nil {
				errStr := activeErr.Error()
				if containsStr(errStr, "active run exists") {
					m.logger.Error("cannot start mission: active run already exists",
						"workflow_name", wf.Name,
						"error", err,
					)
					return nil, fmt.Errorf("cannot start mission '%s': %w. Use 'gibson mission list' to see active missions or 'gibson mission pause <id>' to pause the active mission", wf.Name, err)
				}
			}
			m.logger.Warn("failed to create mission run", "error", err)
			return nil, fmt.Errorf("failed to create mission run: %w", err)
		}
	} else if m.missionStore != nil {
		// Fallback to direct save if run linker not available
		if err := m.missionStore.Save(ctx, missionRecord); err != nil {
			m.logger.Warn("failed to save mission to store", "error", err)
			// Continue execution - this is not critical
		}
	}

	// Create event channel for mission updates
	eventChan := make(chan api.MissionEventData, 100)

	// Create mission context with cancellation
	missionCtx, cancel := context.WithCancel(context.Background())

	// Create active mission entry
	active := &activeMission{
		mission:   missionRecord,
		ctx:       missionCtx,
		cancel:    cancel,
		eventChan: eventChan,
		startTime: time.Now(),
	}

	// Register active mission
	m.mu.Lock()
	m.activeMissions[missionID] = active
	m.mu.Unlock()

	// Emit mission started event
	m.emitEvent(eventChan, api.MissionEventData{
		EventType: "mission.started",
		Timestamp: time.Now(),
		MissionID: missionID,
		Message:   fmt.Sprintf("Mission %s started", missionID),
	})

	// Launch mission executor in goroutine
	go m.executeMission(missionCtx, missionID, wf, eventChan)

	return eventChan, nil
}

// executeMission runs the workflow execution using the SOTA orchestrator.
// This handles the full mission lifecycle including setup, execution via
// the Observe → Think → Act loop, and cleanup.
func (m *missionManager) executeMission(ctx context.Context, missionID string, wf *workflow.Workflow, eventChan chan api.MissionEventData) {
	defer close(eventChan)
	defer m.cleanupMission(missionID)

	// Create mission execution span if tracing is enabled
	var span trace.Span
	if m.infrastructure != nil && m.infrastructure.tracerProvider != nil {
		tracer := m.infrastructure.tracerProvider.Tracer("gibson")
		ctx, span = tracer.Start(ctx, observability.SpanMissionExecute,
			trace.WithAttributes(
				attribute.String(observability.GibsonMissionID, missionID),
				attribute.String(observability.GibsonWorkflowName, wf.Name),
			),
		)
		defer span.End()
	}

	m.logger.Info("executing mission workflow with SOTA orchestrator", "mission_id", missionID)

	// Get active mission
	m.mu.RLock()
	active, exists := m.activeMissions[missionID]
	m.mu.RUnlock()

	if !exists {
		m.logger.Error("active mission not found", "mission_id", missionID)

		// Record error on span
		if span != nil {
			span.RecordError(fmt.Errorf("active mission not found"))
			span.SetStatus(codes.Error, "mission not found")
		}

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     "internal error: mission not found",
		})
		return
	}

	// Add mission name to span now that we have the active mission
	if span != nil {
		span.SetAttributes(attribute.String(observability.GibsonMissionName, active.mission.Name))
	}

	// Check if GraphRAG is available - required for SOTA orchestrator
	if m.infrastructure == nil || m.infrastructure.graphRAGClient == nil {
		m.logger.Error("GraphRAG not available - SOTA orchestrator requires Neo4j",
			"mission_id", missionID)

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     "GraphRAG (Neo4j) is required for mission execution but not configured",
		})
		return
	}

	// Create query handlers for graph operations
	graphClient := m.infrastructure.graphRAGClient
	missionQueries := queries.NewMissionQueries(graphClient)
	executionQueries := queries.NewExecutionQueries(graphClient)

	// Create mission context and target info for harness
	missionCtx := harness.NewMissionContext(active.mission.ID, active.mission.Name, "")

	// Load target entity to get connection details
	var targetInfo harness.TargetInfo
	if active.mission.TargetID == "00000000-0000-0000-0000-d15c00e00000" {
		// Synthetic target for discovery/orchestration missions
		targetInfo = harness.NewTargetInfo(active.mission.TargetID, "discovery-mission", "", "discovery")
	} else if ts, ok := m.targetStore.(mission.TargetStore); ok {
		target, err := ts.Get(ctx, active.mission.TargetID)
		if err != nil {
			m.logger.Error("failed to load target", "error", err, "target_id", active.mission.TargetID)
			m.emitEvent(eventChan, api.MissionEventData{
				EventType: "mission.failed",
				Timestamp: time.Now(),
				MissionID: missionID,
				Error:     fmt.Sprintf("failed to load target: %v", err),
			})
			return
		}
		targetInfo = harness.NewTargetInfoFull(
			target.ID,
			target.Name,
			target.URL,
			target.Type,
			target.Connection,
		)
	} else {
		targetInfo = harness.NewTargetInfo(active.mission.TargetID, "mission-target", "", "")
	}

	// Create harness for agent execution
	agentHarness, err := m.harnessFactory.Create("orchestrator", missionCtx, targetInfo)
	if err != nil {
		m.logger.Error("failed to create harness", "error", err)
		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     fmt.Sprintf("failed to create harness: %v", err),
		})
		return
	}

	// Create LLM client adapter for the Thinker
	llmClient := &llmClientAdapter{harness: agentHarness}

	// Create harness adapter for the Actor
	harnessAdapter := &orchestratorHarnessAdapter{harness: agentHarness}

	// Create SOTA orchestrator components
	observer := orchestrator.NewObserver(missionQueries, executionQueries)
	thinker := orchestrator.NewThinker(llmClient,
		orchestrator.WithMaxRetries(3),
		orchestrator.WithThinkerTemperature(0.2),
	)
	actor := orchestrator.NewActor(harnessAdapter, executionQueries, missionQueries, graphClient)

	// Create the SOTA orchestrator
	sotaOrchestrator := orchestrator.NewOrchestrator(observer, thinker, actor,
		orchestrator.WithMaxIterations(100),
		orchestrator.WithMaxConcurrent(10),
		orchestrator.WithLogger(m.logger.With("component", "sota-orchestrator")),
	)

	// Emit workflow execution started event
	m.emitEvent(eventChan, api.MissionEventData{
		EventType: "workflow.started",
		Timestamp: time.Now(),
		MissionID: missionID,
		Message:   fmt.Sprintf("Starting SOTA orchestrator for mission %s", missionID),
	})

	// Execute mission through SOTA orchestrator's Observe → Think → Act loop
	result, err := sotaOrchestrator.Run(ctx, missionID)

	// Calculate mission duration
	missionDuration := time.Since(active.startTime)

	// Determine final status based on execution result
	var finalStatus mission.MissionStatus
	var errorMsg string

	if err != nil {
		m.logger.Error("mission execution failed", "mission_id", missionID, "error", err)
		finalStatus = mission.MissionStatusFailed
		errorMsg = err.Error()

		// Record error on span
		if span != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, errorMsg)
			span.SetAttributes(
				attribute.Int("gibson.mission.duration_ms", int(missionDuration.Milliseconds())),
			)
		}

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     errorMsg,
		})
	} else if result != nil {
		// Map orchestrator status to mission status
		switch result.Status {
		case orchestrator.StatusCompleted:
			finalStatus = mission.MissionStatusCompleted
		case orchestrator.StatusFailed:
			finalStatus = mission.MissionStatusFailed
			errorMsg = "orchestrator reported failure"
			if result.Error != nil {
				errorMsg = result.Error.Error()
			}
		case orchestrator.StatusCancelled:
			finalStatus = mission.MissionStatusCancelled
			errorMsg = "mission was cancelled"
		case orchestrator.StatusMaxIterations:
			finalStatus = mission.MissionStatusFailed
			errorMsg = "max iterations reached"
		case orchestrator.StatusTimeout:
			finalStatus = mission.MissionStatusFailed
			errorMsg = "orchestrator timed out"
		case orchestrator.StatusBudgetExceeded:
			finalStatus = mission.MissionStatusFailed
			errorMsg = "token budget exceeded"
		default:
			finalStatus = mission.MissionStatusFailed
			errorMsg = fmt.Sprintf("unknown orchestrator status: %s", result.Status)
		}

		// Log orchestrator statistics
		m.logger.Info("SOTA orchestrator completed",
			"mission_id", missionID,
			"status", result.Status,
			"iterations", result.TotalIterations,
			"decisions", result.TotalDecisions,
			"tokens_used", result.TotalTokensUsed,
			"completed_nodes", result.CompletedNodes,
			"failed_nodes", result.FailedNodes,
			"duration", result.Duration,
			"stop_reason", result.StopReason,
		)

		if finalStatus == mission.MissionStatusFailed {
			// Record error on span
			if span != nil {
				span.SetStatus(codes.Error, errorMsg)
				span.SetAttributes(
					attribute.Int("gibson.mission.iterations", result.TotalIterations),
					attribute.Int("gibson.mission.decisions", result.TotalDecisions),
					attribute.Int("gibson.mission.tokens_used", result.TotalTokensUsed),
					attribute.Int("gibson.mission.duration_ms", int(missionDuration.Milliseconds())),
				)
			}
		} else {
			// Mission completed successfully
			if span != nil {
				span.SetStatus(codes.Ok, "mission completed")
				span.SetAttributes(
					attribute.Int("gibson.mission.iterations", result.TotalIterations),
					attribute.Int("gibson.mission.decisions", result.TotalDecisions),
					attribute.Int("gibson.mission.tokens_used", result.TotalTokensUsed),
					attribute.Int("gibson.mission.completed_nodes", result.CompletedNodes),
					attribute.Int("gibson.mission.failed_nodes", result.FailedNodes),
					attribute.Int("gibson.mission.duration_ms", int(missionDuration.Milliseconds())),
				)
			}
		}

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.completed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Message:   fmt.Sprintf("Mission completed with status: %s (iterations: %d, decisions: %d)", finalStatus, result.TotalIterations, result.TotalDecisions),
		})
	} else {
		// No result returned - treat as failed
		finalStatus = mission.MissionStatusFailed
		errorMsg = "no result returned from SOTA orchestrator"

		// Record error on span
		if span != nil {
			span.RecordError(fmt.Errorf("%s", errorMsg))
			span.SetStatus(codes.Error, errorMsg)
			span.SetAttributes(
				attribute.Int("gibson.mission.duration_ms", int(missionDuration.Milliseconds())),
			)
		}

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     errorMsg,
		})
	}

	// Update mission in store
	active.mission.Status = finalStatus
	active.mission.Error = errorMsg
	now := time.Now()
	active.mission.CompletedAt = &now

	if m.missionStore != nil {
		if saveErr := m.missionStore.Update(ctx, active.mission); saveErr != nil {
			m.logger.Warn("failed to update mission in store", "error", saveErr)
		}
	}

	m.logger.Info("mission execution completed",
		"mission_id", missionID,
		"status", finalStatus,
		"duration", time.Since(active.startTime),
	)
}

// Pause pauses a running mission at the next clean checkpoint.
func (m *missionManager) Pause(ctx context.Context, missionID string, force bool) error {
	m.logger.Info("pausing mission", "mission_id", missionID, "force", force)

	m.mu.RLock()
	active, exists := m.activeMissions[missionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("mission %s not found or not running", missionID)
	}

	// If force is true, immediately cancel the mission context
	// Otherwise, we let the mission detect the pause request gracefully
	if force {
		active.cancel()
	} else {
		// Cancel the mission context to signal pause request
		// The orchestrator should detect this and save a checkpoint before transitioning to paused
		active.cancel()
	}

	// Emit pause event
	m.emitEvent(active.eventChan, api.MissionEventData{
		EventType: "mission.pausing",
		Timestamp: time.Now(),
		MissionID: missionID,
		Message:   "Mission pause requested",
	})

	// Wait for mission to transition to paused state (with timeout)
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			m.logger.Warn("timeout waiting for mission to pause", "mission_id", missionID)
			// Update status to paused anyway
			active.mission.Status = mission.MissionStatusPaused
			now := time.Now()
			active.mission.CompletedAt = &now
			if m.missionStore != nil {
				if err := m.missionStore.Update(ctx, active.mission); err != nil {
					m.logger.Warn("failed to update mission status to paused", "error", err)
				}
			}
			return fmt.Errorf("timeout waiting for mission to pause")

		case <-ticker.C:
			// Check if mission is still active
			m.mu.RLock()
			_, stillActive := m.activeMissions[missionID]
			m.mu.RUnlock()

			// If mission is no longer active, it has completed or failed
			if !stillActive {
				// Fetch mission from store to get final status
				if m.missionStore != nil {
					finalMission, err := m.missionStore.Get(ctx, types.ID(missionID))
					if err == nil {
						if finalMission.Status == mission.MissionStatusPaused {
							m.logger.Info("mission paused successfully", "mission_id", missionID)
							return nil
						}
						return fmt.Errorf("mission transitioned to unexpected status: %s", finalMission.Status)
					}
				}
				// If we can't get the mission, assume it's paused
				return nil
			}
		}
	}
}

// Resume resumes a paused mission from its last checkpoint.
func (m *missionManager) Resume(ctx context.Context, missionID string) (<-chan api.MissionEventData, error) {
	m.logger.Info("resuming mission", "mission_id", missionID)

	// Check if mission is already running
	m.mu.RLock()
	if _, exists := m.activeMissions[missionID]; exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("mission %s is already running", missionID)
	}
	m.mu.RUnlock()

	// Get mission from store
	if m.missionStore == nil {
		return nil, fmt.Errorf("mission store not available")
	}

	missionRecord, err := m.missionStore.Get(ctx, types.ID(missionID))
	if err != nil {
		m.logger.Error("failed to get mission", "error", err, "mission_id", missionID)
		return nil, fmt.Errorf("failed to get mission %s: %w", missionID, err)
	}

	// Validate mission can be resumed
	if missionRecord.Status != mission.MissionStatusPaused {
		return nil, fmt.Errorf("cannot resume mission %s: status is %s (expected paused)", missionID, missionRecord.Status)
	}

	// Parse workflow from mission
	var wf *workflow.Workflow
	if missionRecord.WorkflowJSON != "" {
		wf, err = workflow.ParseWorkflow([]byte(missionRecord.WorkflowJSON))
		if err != nil {
			m.logger.Error("failed to parse workflow", "error", err)
			return nil, fmt.Errorf("failed to parse workflow: %w", err)
		}
	} else {
		return nil, fmt.Errorf("mission %s has no workflow definition", missionID)
	}

	// Create event channel for mission updates
	eventChan := make(chan api.MissionEventData, 100)

	// Create mission context with cancellation
	missionCtx, cancel := context.WithCancel(context.Background())

	// Update mission status to running
	missionRecord.Status = mission.MissionStatusRunning
	startedAt := time.Now()
	missionRecord.StartedAt = &startedAt

	if err := m.missionStore.Update(ctx, missionRecord); err != nil {
		m.logger.Warn("failed to update mission status", "error", err)
	}

	// Create active mission entry
	active := &activeMission{
		mission:   missionRecord,
		ctx:       missionCtx,
		cancel:    cancel,
		eventChan: eventChan,
		startTime: time.Now(),
	}

	// Register active mission
	m.mu.Lock()
	m.activeMissions[missionID] = active
	m.mu.Unlock()

	// Emit mission resumed event
	m.emitEvent(eventChan, api.MissionEventData{
		EventType: "mission.resumed",
		Timestamp: time.Now(),
		MissionID: missionID,
		Message:   fmt.Sprintf("Mission %s resumed from checkpoint", missionID),
	})

	// Launch mission executor in goroutine
	// Note: This will execute from the beginning - checkpoint restoration would be handled
	// by the orchestrator if ExecuteFromCheckpoint were implemented
	go m.executeMission(missionCtx, missionID, wf, eventChan)

	return eventChan, nil
}

// Stop stops a running mission with optional force flag.
func (m *missionManager) Stop(ctx context.Context, missionID string, force bool) error {
	m.logger.Info("stopping mission", "mission_id", missionID, "force", force)

	m.mu.RLock()
	active, exists := m.activeMissions[missionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("mission %s not found or not running", missionID)
	}

	// Cancel the mission context
	active.cancel()

	// Emit stop event
	m.emitEvent(active.eventChan, api.MissionEventData{
		EventType: "mission.stopped",
		Timestamp: time.Now(),
		MissionID: missionID,
		Message:   "Mission stopped by user",
	})

	// Update mission status
	active.mission.Status = mission.MissionStatusCancelled
	now := time.Now()
	active.mission.CompletedAt = &now

	if m.missionStore != nil {
		if err := m.missionStore.Update(ctx, active.mission); err != nil {
			m.logger.Warn("failed to update mission in store", "error", err)
		}
	}

	m.logger.Info("mission stopped", "mission_id", missionID)
	return nil
}

// List returns a list of missions with optional filtering.
func (m *missionManager) List(ctx context.Context, activeOnly bool, limit, offset int) ([]api.MissionData, int, error) {
	m.logger.Debug("listing missions", "active_only", activeOnly, "limit", limit, "offset", offset)

	var result []api.MissionData

	// Get active missions
	m.mu.RLock()
	activeMissions := make([]*mission.Mission, 0, len(m.activeMissions))
	for _, active := range m.activeMissions {
		activeMissions = append(activeMissions, active.mission)
	}
	m.mu.RUnlock()

	// Add active missions to result
	for _, m := range activeMissions {
		result = append(result, missionToData(m))
	}

	// If not active-only, also fetch from store
	if !activeOnly && m.missionStore != nil {
		filter := mission.NewMissionFilter()
		filter.Limit = 1000 // Get a reasonable number
		filter.Offset = 0

		stored, err := m.missionStore.List(ctx, filter)
		if err != nil {
			m.logger.Warn("failed to list missions from store", "error", err)
		} else {
			// Add completed missions that aren't already in active list
			for _, storedMission := range stored {
				// Check if already in active list
				isActive := false
				for _, active := range activeMissions {
					if active.ID == storedMission.ID {
						isActive = true
						break
					}
				}
				if !isActive && storedMission.Status.IsTerminal() {
					result = append(result, missionToData(storedMission))
				}
			}
		}
	}

	total := len(result)

	// Apply pagination
	if offset > 0 {
		if offset >= len(result) {
			result = []api.MissionData{}
		} else {
			result = result[offset:]
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	m.logger.Debug("listed missions", "total", total, "returned", len(result))
	return result, total, nil
}

// Get returns a specific mission by ID.
func (m *missionManager) Get(ctx context.Context, missionID string) (*api.MissionData, error) {
	m.logger.Debug("getting mission", "mission_id", missionID)

	// Check active missions first
	m.mu.RLock()
	active, exists := m.activeMissions[missionID]
	m.mu.RUnlock()

	if exists {
		data := missionToData(active.mission)
		return &data, nil
	}

	// Check mission store
	if m.missionStore != nil {
		missionRecord, err := m.missionStore.Get(ctx, types.ID(missionID))
		if err == nil {
			data := missionToData(missionRecord)
			return &data, nil
		}
	}

	return nil, fmt.Errorf("mission %s not found", missionID)
}

// cleanupMission removes a mission from active tracking after completion.
func (m *missionManager) cleanupMission(missionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.activeMissions, missionID)
	m.completedCount++

	m.logger.Debug("mission cleaned up", "mission_id", missionID)
}

// emitEvent safely sends an event to the event channel without blocking.
func (m *missionManager) emitEvent(eventChan chan api.MissionEventData, event api.MissionEventData) {
	select {
	case eventChan <- event:
		// Event sent successfully
	default:
		// Channel full or closed, log and skip
		m.logger.Warn("failed to emit mission event: channel full or closed",
			"event_type", event.EventType,
			"mission_id", event.MissionID,
		)
	}
}

// missionToData converts a mission.Mission to api.MissionData.
func missionToData(m *mission.Mission) api.MissionData {
	data := api.MissionData{
		ID:           m.ID.String(),
		WorkflowPath: "", // Not tracked in Mission struct
		Status:       string(m.Status),
		StartTime:    m.CreatedAt,
		FindingCount: int32(m.FindingsCount),
	}

	if m.CompletedAt != nil {
		data.EndTime = *m.CompletedAt
	}

	return data
}

// GetActiveMissionCount returns the number of currently active missions.
func (m *missionManager) GetActiveMissionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeMissions)
}

// GetTotalMissionCount returns the total number of missions (active + completed).
func (m *missionManager) GetTotalMissionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeMissions) + m.completedCount
}

// containsStr checks if a string contains a substring (case-sensitive).
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// llmClientAdapter adapts an AgentHarness to the orchestrator.LLMClient interface.
// This allows the SOTA orchestrator's Thinker to use the harness for LLM operations.
type llmClientAdapter struct {
	harness harness.AgentHarness
}

// Complete performs a synchronous LLM completion using the harness.
func (a *llmClientAdapter) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...orchestrator.CompletionOption) (*llm.CompletionResponse, error) {
	// Convert orchestrator options to harness options
	harnessOpts := make([]harness.CompletionOption, 0, len(opts))
	compOpts := &orchestrator.CompletionOptions{}
	for _, opt := range opts {
		opt(compOpts)
	}
	if compOpts.Temperature > 0 {
		harnessOpts = append(harnessOpts, harness.WithTemperature(compOpts.Temperature))
	}
	if compOpts.MaxTokens > 0 {
		harnessOpts = append(harnessOpts, harness.WithMaxTokens(compOpts.MaxTokens))
	}
	if compOpts.TopP > 0 {
		harnessOpts = append(harnessOpts, harness.WithTopP(compOpts.TopP))
	}

	return a.harness.Complete(ctx, slot, messages, harnessOpts...)
}

// CompleteStructuredAny performs a completion with provider-native structured output.
func (a *llmClientAdapter) CompleteStructuredAny(ctx context.Context, slot string, messages []llm.Message, schemaType any, opts ...orchestrator.CompletionOption) (any, error) {
	// Convert orchestrator options to harness options
	harnessOpts := make([]harness.CompletionOption, 0, len(opts))
	compOpts := &orchestrator.CompletionOptions{}
	for _, opt := range opts {
		opt(compOpts)
	}
	if compOpts.Temperature > 0 {
		harnessOpts = append(harnessOpts, harness.WithTemperature(compOpts.Temperature))
	}
	if compOpts.MaxTokens > 0 {
		harnessOpts = append(harnessOpts, harness.WithMaxTokens(compOpts.MaxTokens))
	}
	if compOpts.TopP > 0 {
		harnessOpts = append(harnessOpts, harness.WithTopP(compOpts.TopP))
	}

	return a.harness.CompleteStructuredAny(ctx, slot, messages, schemaType, harnessOpts...)
}

// orchestratorHarnessAdapter adapts an AgentHarness to the orchestrator.Harness interface.
// This allows the SOTA orchestrator's Actor to delegate to agents.
type orchestratorHarnessAdapter struct {
	harness harness.AgentHarness
}

// DelegateToAgent delegates a task to another agent via the harness.
func (a *orchestratorHarnessAdapter) DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error) {
	return a.harness.DelegateToAgent(ctx, agentName, task)
}

// CallTool executes a tool via the harness.
func (a *orchestratorHarnessAdapter) CallTool(ctx context.Context, toolName string, input map[string]interface{}) (interface{}, error) {
	return a.harness.CallTool(ctx, toolName, input)
}
