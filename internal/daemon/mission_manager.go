package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

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
) *missionManager {
	return &missionManager{
		config:          cfg,
		logger:          logger.With("component", "mission-manager"),
		registry:        reg,
		missionStore:    missionStore,
		findingStore:    findingStore,
		llmRegistry:     llmRegistry,
		callbackManager: callbackMgr,
		harnessFactory:  harnessFactory,
		activeMissions:  make(map[string]*activeMission),
	}
}

// Run starts a mission and returns an event channel for progress updates.
// This implements the core mission execution flow:
// 1. Load workflow from file
// 2. Create mission context and stores
// 3. Launch workflow executor in goroutine
// 4. Return event channel for streaming updates
func (m *missionManager) Run(ctx context.Context, workflowPath string, missionID string, variables map[string]string) (<-chan api.MissionEventData, error) {
	m.logger.Info("starting mission",
		"workflow_path", workflowPath,
		"mission_id", missionID,
		"variables", len(variables),
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
		missionID = fmt.Sprintf("mission-%d", time.Now().Unix())
	}

	// Check if mission ID already exists
	m.mu.RLock()
	if _, exists := m.activeMissions[missionID]; exists {
		m.mu.RUnlock()
		return nil, fmt.Errorf("mission %s is already running", missionID)
	}
	m.mu.RUnlock()

	// Create mission record
	missionRecord := &mission.Mission{
		ID:            types.ID(missionID),
		Name:          wf.Name,
		Description:   wf.Description,
		Status:        mission.MissionStatusRunning,
		WorkflowID:    wf.ID,
		CreatedAt:     time.Now(),
		StartedAt:     &[]time.Time{time.Now()}[0],
		FindingsCount: 0,
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

	// Save mission to store
	if m.missionStore != nil {
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

// executeMission runs the workflow execution in a goroutine.
// This handles the full mission lifecycle including setup, execution, and cleanup.
func (m *missionManager) executeMission(ctx context.Context, missionID string, wf *workflow.Workflow, eventChan chan api.MissionEventData) {
	defer close(eventChan)
	defer m.cleanupMission(missionID)

	m.logger.Info("executing mission workflow", "mission_id", missionID)

	// Get active mission
	m.mu.RLock()
	active, exists := m.activeMissions[missionID]
	m.mu.RUnlock()

	if !exists {
		m.logger.Error("active mission not found", "mission_id", missionID)
		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     "internal error: mission not found",
		})
		return
	}

	// Create workflow executor
	workflowExecutor := workflow.NewWorkflowExecutor(
		workflow.WithLogger(m.logger.With("component", "workflow-executor")),
		workflow.WithMaxParallel(10),
	)

	// Create mission orchestrator with harness factory
	orchestrator := mission.NewMissionOrchestrator(
		m.missionStore,
		mission.WithHarnessFactory(m.harnessFactory),
		mission.WithWorkflowExecutor(workflowExecutor),
		mission.WithEventEmitter(mission.NewDefaultEventEmitter(mission.WithBufferSize(100))),
	)

	// Emit workflow execution started event
	m.emitEvent(eventChan, api.MissionEventData{
		EventType: "workflow.started",
		Timestamp: time.Now(),
		MissionID: missionID,
		Message:   fmt.Sprintf("Starting workflow execution for mission %s", missionID),
	})

	// Execute mission through orchestrator
	result, err := orchestrator.Execute(ctx, active.mission)

	// Determine final status based on execution result
	var finalStatus mission.MissionStatus
	var errorMsg string

	if err != nil {
		m.logger.Error("mission execution failed", "mission_id", missionID, "error", err)
		finalStatus = mission.MissionStatusFailed
		errorMsg = err.Error()

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.failed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Error:     errorMsg,
		})
	} else if result != nil {
		// Use the status from the result
		finalStatus = result.Status
		if finalStatus == mission.MissionStatusFailed {
			errorMsg = "workflow execution failed"
		}

		m.emitEvent(eventChan, api.MissionEventData{
			EventType: "mission.completed",
			Timestamp: time.Now(),
			MissionID: missionID,
			Message:   fmt.Sprintf("Mission completed with status: %s", finalStatus),
		})
	} else {
		// No result returned - treat as failed
		finalStatus = mission.MissionStatusFailed
		errorMsg = "no result returned from orchestrator"

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
		if saveErr := m.missionStore.Save(ctx, active.mission); saveErr != nil {
			m.logger.Warn("failed to update mission in store", "error", saveErr)
		}
	}

	m.logger.Info("mission execution completed",
		"mission_id", missionID,
		"status", finalStatus,
		"duration", time.Since(active.startTime),
	)
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
		if err := m.missionStore.Save(ctx, active.mission); err != nil {
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
