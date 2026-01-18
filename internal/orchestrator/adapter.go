package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/queries"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

// SOTAMissionOrchestrator adapts the SOTA orchestrator to the mission.MissionOrchestrator interface.
// It provides backward compatibility with existing code that expects the mission.MissionOrchestrator interface
// while using the new SOTA (Observe → Think → Act) orchestrator internally.
type SOTAMissionOrchestrator struct {
	config SOTAOrchestratorConfig

	// Pause request handling for backward compatibility
	pauseRequestedMu sync.RWMutex
	pauseRequested   map[types.ID]bool
}

// MissionGraphLoader defines the interface for storing mission definitions in Neo4j.
// This is used to track mission execution state in the graph.
type MissionGraphLoader interface {
	// LoadMission stores a mission definition in the graph and returns the mission ID
	LoadMission(ctx context.Context, def *mission.MissionDefinition) (string, error)
}

// Execute implements mission.MissionOrchestrator interface.
// It converts the mission to the SOTA format, executes it using the SOTA orchestrator,
// and converts the result back to a mission.MissionResult.
func (s *SOTAMissionOrchestrator) Execute(ctx context.Context, m *mission.Mission) (*mission.MissionResult, error) {
	// Validate mission can be executed
	if !m.Status.CanTransitionTo(mission.MissionStatusRunning) {
		return nil, mission.NewInvalidStateError(m.Status, mission.MissionStatusRunning)
	}

	// Update mission status to running
	startedAt := time.Now()
	m.Status = mission.MissionStatusRunning
	m.StartedAt = &startedAt

	// Initialize metrics
	if m.Metrics == nil {
		m.Metrics = &mission.MissionMetrics{
			StartedAt:          startedAt,
			LastUpdateAt:       startedAt,
			FindingsBySeverity: make(map[string]int),
		}
	}

	// Parse mission definition from mission
	var def *mission.MissionDefinition
	var err error

	if m.WorkflowJSON != "" {
		// Parse mission definition from inline JSON
		def = &mission.MissionDefinition{}
		if err = json.Unmarshal([]byte(m.WorkflowJSON), def); err != nil {
			return nil, fmt.Errorf("failed to parse mission definition: %w", err)
		}
	} else if m.WorkflowID != "" {
		// For now, we need the definition JSON to be present
		// In a future enhancement, we could load from the mission definition store
		return nil, fmt.Errorf("mission definition loading from WorkflowID not yet implemented in SOTA adapter")
	} else {
		return nil, fmt.Errorf("no mission definition available (neither WorkflowID nor WorkflowJSON)")
	}

	// Store mission definition in Neo4j graph for state tracking
	if s.config.GraphLoader != nil {
		graphMissionID, err := s.config.GraphLoader.LoadMission(ctx, def)
		if err != nil {
			// Log warning but continue - graph storage is optional
			s.config.Logger.Warn("failed to store mission definition in GraphRAG",
				"error", err,
				"mission_id", m.ID,
				"definition_name", def.Name,
			)
		} else {
			s.config.Logger.Info("mission definition stored in GraphRAG",
				"graph_mission_id", graphMissionID,
				"mission_id", m.ID,
				"definition_name", def.Name,
			)
		}
	}

	// Create SOTA orchestrator for this mission execution
	orchestrator, err := s.createOrchestrator(ctx, m, def)
	if err != nil {
		return nil, fmt.Errorf("failed to create orchestrator: %w", err)
	}

	// Execute using SOTA orchestrator
	orchResult, err := orchestrator.Run(ctx, m.ID.String())
	if err != nil {
		// Convert error to mission result
		return s.convertErrorToResult(m, orchResult, err, startedAt), err
	}

	// Convert orchestrator result to mission result
	return s.convertResult(m, orchResult, startedAt), nil
}

// createOrchestrator creates a SOTA orchestrator instance for a specific mission execution.
// It creates the harness, adapters, and all orchestrator components (Observer, Thinker, Actor).
func (s *SOTAMissionOrchestrator) createOrchestrator(ctx context.Context, m *mission.Mission, def *mission.MissionDefinition) (*Orchestrator, error) {
	// Validate GraphRAG client
	if s.config.GraphRAGClient == nil {
		return nil, fmt.Errorf("GraphRAGClient not configured")
	}

	// Create query handlers
	missionQueries := queries.NewMissionQueries(s.config.GraphRAGClient)
	executionQueries := queries.NewExecutionQueries(s.config.GraphRAGClient)

	// Create Observer
	observer := NewObserver(missionQueries, executionQueries)

	// Create harness for this mission
	// Use the harness factory to create an appropriate harness
	missionCtx := harness.NewMissionContext(m.ID, m.Name, "")

	// Create target info
	// Note: In a full implementation, we would load the target entity here
	// For now, we use a simplified approach
	targetInfo := harness.NewTargetInfo(m.TargetID, "mission-target", "", "")

	// Get first agent name from definition
	agentName := "orchestrator" // Default agent name
	if len(def.Nodes) > 0 {
		for _, node := range def.Nodes {
			if node.Type == mission.NodeTypeAgent && node.AgentName != "" {
				agentName = node.AgentName
				break
			}
		}
	}

	// Create harness
	agentHarness, err := s.config.HarnessFactory.Create(agentName, missionCtx, targetInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create harness: %w", err)
	}

	// Create LLM client adapter
	llmClient := &llmClientAdapter{harness: agentHarness}

	// Create Thinker
	thinker := NewThinker(llmClient,
		WithMaxRetries(s.config.ThinkerMaxRetries),
		WithThinkerTemperature(s.config.ThinkerTemperature),
	)

	// Create harness adapter for Actor
	harnessAdapter := &orchestratorHarnessAdapter{harness: agentHarness}

	// Build component inventory for validation (if registry available)
	var inventory *ComponentInventory
	if s.config.Registry != nil {
		inventoryCtx, inventoryCancel := context.WithTimeout(ctx, 5*time.Second)
		defer inventoryCancel()

		inventoryBuilder := NewInventoryBuilder(s.config.Registry)
		var err error
		inventory, err = inventoryBuilder.Build(inventoryCtx)
		if err != nil {
			s.config.Logger.Warn("failed to build component inventory, validation will be skipped",
				"mission_id", m.ID,
				"error", err)
			inventory = nil // Continue without inventory
		}
	}

	// Create Actor
	actor := NewActor(harnessAdapter, executionQueries, missionQueries, s.config.GraphRAGClient, inventory)

	// Create the orchestrator
	orchOptions := []OrchestratorOption{
		WithMaxIterations(s.config.MaxIterations),
		WithMaxConcurrent(s.config.MaxConcurrent),
		WithBudget(s.config.Budget),
		WithTimeout(s.config.Timeout),
		WithLogger(s.config.Logger.With("component", "sota-orchestrator", "mission_id", m.ID)),
		WithTracer(s.config.Tracer),
		WithEventBus(s.config.EventBus),
		WithDecisionLogWriter(s.config.DecisionLogWriter),
	}

	// Add component discovery if registry available
	if s.config.Registry != nil {
		orchOptions = append(orchOptions, WithComponentDiscovery(s.config.Registry))
	}

	orchestrator := NewOrchestrator(observer, thinker, actor, orchOptions...)

	return orchestrator, nil
}

// convertResult converts an OrchestratorResult to a mission.MissionResult
func (s *SOTAMissionOrchestrator) convertResult(m *mission.Mission, orchResult *OrchestratorResult, startedAt time.Time) *mission.MissionResult {
	result := &mission.MissionResult{
		MissionID:  m.ID,
		Metrics:    m.Metrics,
		FindingIDs: []types.ID{},
	}

	// Map orchestrator status to mission status
	switch orchResult.Status {
	case StatusCompleted:
		result.Status = mission.MissionStatusCompleted
		m.Status = mission.MissionStatusCompleted
	case StatusFailed:
		result.Status = mission.MissionStatusFailed
		m.Status = mission.MissionStatusFailed
		if orchResult.Error != nil {
			result.Error = orchResult.Error.Error()
			m.Error = orchResult.Error.Error()
		}
	case StatusCancelled:
		result.Status = mission.MissionStatusCancelled
		m.Status = mission.MissionStatusCancelled
		result.Error = "orchestration cancelled"
		m.Error = "orchestration cancelled"
	case StatusMaxIterations:
		result.Status = mission.MissionStatusFailed
		m.Status = mission.MissionStatusFailed
		result.Error = "max iterations reached"
		m.Error = "max iterations reached"
	case StatusTimeout:
		result.Status = mission.MissionStatusFailed
		m.Status = mission.MissionStatusFailed
		result.Error = "orchestration timeout"
		m.Error = "orchestration timeout"
	case StatusBudgetExceeded:
		result.Status = mission.MissionStatusFailed
		m.Status = mission.MissionStatusFailed
		result.Error = "token budget exceeded"
		m.Error = "token budget exceeded"
	case StatusConcurrencyLimit:
		result.Status = mission.MissionStatusFailed
		m.Status = mission.MissionStatusFailed
		result.Error = "concurrency limit reached"
		m.Error = "concurrency limit reached"
	default:
		result.Status = mission.MissionStatusFailed
		m.Status = mission.MissionStatusFailed
		result.Error = fmt.Sprintf("unknown orchestrator status: %s", orchResult.Status)
		m.Error = fmt.Sprintf("unknown orchestrator status: %s", orchResult.Status)
	}

	// Update mission metrics
	completedAt := time.Now()
	m.CompletedAt = &completedAt
	m.Metrics.Duration = orchResult.Duration
	m.Metrics.CompletedNodes = orchResult.CompletedNodes
	m.Metrics.LastUpdateAt = completedAt

	// Set result completion time
	result.CompletedAt = completedAt

	// Convert orchestrator result to workflow result map
	if orchResult.FinalState != nil {
		workflowResultMap := make(map[string]any)
		workflowResultMap["status"] = string(orchResult.Status)
		workflowResultMap["total_iterations"] = orchResult.TotalIterations
		workflowResultMap["total_decisions"] = orchResult.TotalDecisions
		workflowResultMap["total_tokens"] = orchResult.TotalTokensUsed
		workflowResultMap["completed_nodes"] = orchResult.CompletedNodes
		workflowResultMap["failed_nodes"] = orchResult.FailedNodes
		workflowResultMap["duration"] = orchResult.Duration.String()

		if orchResult.StopReason != "" {
			workflowResultMap["stop_reason"] = orchResult.StopReason
		}

		result.WorkflowResult = workflowResultMap
	}

	return result
}

// convertErrorToResult creates a failed mission result from an error
func (s *SOTAMissionOrchestrator) convertErrorToResult(m *mission.Mission, orchResult *OrchestratorResult, err error, startedAt time.Time) *mission.MissionResult {
	result := &mission.MissionResult{
		MissionID:  m.ID,
		Status:     mission.MissionStatusFailed,
		Error:      err.Error(),
		Metrics:    m.Metrics,
		FindingIDs: []types.ID{},
	}

	// Update mission status
	m.Status = mission.MissionStatusFailed
	m.Error = err.Error()
	completedAt := time.Now()
	m.CompletedAt = &completedAt
	m.Metrics.Duration = completedAt.Sub(startedAt)
	result.CompletedAt = completedAt

	// If we have partial orchestrator results, include them
	if orchResult != nil {
		workflowResultMap := make(map[string]any)
		workflowResultMap["status"] = "failed"
		workflowResultMap["total_iterations"] = orchResult.TotalIterations
		workflowResultMap["total_decisions"] = orchResult.TotalDecisions
		workflowResultMap["total_tokens"] = orchResult.TotalTokensUsed
		workflowResultMap["completed_nodes"] = orchResult.CompletedNodes
		workflowResultMap["failed_nodes"] = orchResult.FailedNodes
		workflowResultMap["duration"] = orchResult.Duration.String()
		workflowResultMap["error"] = err.Error()
		result.WorkflowResult = workflowResultMap
	}

	return result
}

// RequestPause signals the orchestrator to pause at the next clean boundary.
// This implements the mission.MissionOrchestrator interface for backward compatibility.
func (s *SOTAMissionOrchestrator) RequestPause(ctx context.Context, missionID types.ID) error {
	s.pauseRequestedMu.Lock()
	defer s.pauseRequestedMu.Unlock()

	s.pauseRequested[missionID] = true
	// Note: The SOTA orchestrator doesn't currently support pause/resume
	// This is tracked for future implementation
	return nil
}

// IsPauseRequested checks if pause has been requested for a mission.
func (s *SOTAMissionOrchestrator) IsPauseRequested(missionID types.ID) bool {
	s.pauseRequestedMu.RLock()
	defer s.pauseRequestedMu.RUnlock()

	return s.pauseRequested[missionID]
}

// ClearPauseRequest clears the pause request flag for a mission.
func (s *SOTAMissionOrchestrator) ClearPauseRequest(missionID types.ID) {
	s.pauseRequestedMu.Lock()
	defer s.pauseRequestedMu.Unlock()

	delete(s.pauseRequested, missionID)
}

// ExecuteFromCheckpoint resumes execution from a saved checkpoint.
// This implements the mission.MissionOrchestrator interface for backward compatibility.
func (s *SOTAMissionOrchestrator) ExecuteFromCheckpoint(ctx context.Context, m *mission.Mission, checkpoint *mission.MissionCheckpoint) (*mission.MissionResult, error) {
	// For now, checkpoint recovery is not yet implemented in SOTA orchestrator
	// We'll execute from the beginning
	// TODO: Implement checkpoint recovery in SOTA orchestrator

	// Parse checkpoint state if available
	if checkpoint != nil && len(checkpoint.CompletedNodes) > 0 {
		// In a future enhancement, we would update the graph state to mark completed nodes
		// For now, we log a warning and execute normally
		_ = checkpoint // silence unused warning
	}

	// Execute normally
	return s.Execute(ctx, m)
}

// parseCheckpointState converts a checkpoint to graph updates
func (s *SOTAMissionOrchestrator) parseCheckpointState(checkpoint *mission.MissionCheckpoint) (map[string]any, error) {
	if checkpoint == nil {
		return nil, nil
	}

	state := make(map[string]any)

	// Extract workflow state
	if checkpoint.WorkflowState != nil {
		stateBytes, err := json.Marshal(checkpoint.WorkflowState)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal workflow state: %w", err)
		}
		if err := json.Unmarshal(stateBytes, &state); err != nil {
			return nil, fmt.Errorf("failed to unmarshal workflow state: %w", err)
		}
	}

	// Add completed and pending nodes
	state["completed_nodes"] = checkpoint.CompletedNodes
	state["pending_nodes"] = checkpoint.PendingNodes

	return state, nil
}

// Ensure SOTAMissionOrchestrator implements mission.MissionOrchestrator
var _ mission.MissionOrchestrator = (*SOTAMissionOrchestrator)(nil)
