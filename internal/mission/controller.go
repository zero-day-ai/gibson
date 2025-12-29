package mission

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionController provides high-level mission control operations.
type MissionController interface {
	// Create creates a new mission from configuration
	Create(ctx context.Context, config *MissionConfig) (*Mission, error)

	// Start transitions mission to running and begins execution
	Start(ctx context.Context, missionID types.ID) error

	// Stop gracefully cancels mission execution
	Stop(ctx context.Context, missionID types.ID) error

	// Pause suspends mission at next checkpoint
	Pause(ctx context.Context, missionID types.ID) error

	// Resume continues mission from checkpoint
	Resume(ctx context.Context, missionID types.ID) error

	// Delete removes mission (only terminal states)
	Delete(ctx context.Context, missionID types.ID) error

	// Get retrieves a mission by ID
	Get(ctx context.Context, missionID types.ID) (*Mission, error)

	// List retrieves missions with filtering
	List(ctx context.Context, filter *MissionFilter) ([]*Mission, error)

	// GetProgress returns real-time progress for a mission
	GetProgress(ctx context.Context, missionID types.ID) (*MissionProgress, error)
}

// DefaultMissionController implements MissionController.
type DefaultMissionController struct {
	store        MissionStore
	service      MissionService
	orchestrator MissionOrchestrator

	// executionMu protects concurrent mission operations
	executionMu sync.RWMutex
	// activeMissions tracks currently executing missions
	activeMissions map[types.ID]context.CancelFunc
}

// ControllerOption is a functional option for configuring the controller.
type ControllerOption func(*DefaultMissionController)

// NewMissionController creates a new mission controller.
func NewMissionController(
	store MissionStore,
	service MissionService,
	orchestrator MissionOrchestrator,
	opts ...ControllerOption,
) *DefaultMissionController {
	c := &DefaultMissionController{
		store:          store,
		service:        service,
		orchestrator:   orchestrator,
		activeMissions: make(map[types.ID]context.CancelFunc),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Create creates a new mission from configuration.
func (c *DefaultMissionController) Create(ctx context.Context, config *MissionConfig) (*Mission, error) {
	return c.service.CreateFromConfig(ctx, config)
}

// Start transitions mission to running and begins execution.
func (c *DefaultMissionController) Start(ctx context.Context, missionID types.ID) error {
	c.executionMu.Lock()
	defer c.executionMu.Unlock()

	// Get mission
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		return err
	}

	// Validate state transition
	if !mission.Status.CanTransitionTo(MissionStatusRunning) {
		return NewInvalidStateError(mission.Status, MissionStatusRunning)
	}

	// Check if already running
	if _, exists := c.activeMissions[missionID]; exists {
		return fmt.Errorf("mission is already running")
	}

	// Create cancellable context for execution
	execCtx, cancel := context.WithCancel(context.Background())
	c.activeMissions[missionID] = cancel

	// Start mission execution in background
	go func() {
		defer func() {
			c.executionMu.Lock()
			delete(c.activeMissions, missionID)
			c.executionMu.Unlock()
		}()

		_, err := c.orchestrator.Execute(execCtx, mission)
		if err != nil {
			// Update mission with error
			mission.Status = MissionStatusFailed
			mission.Error = err.Error()
			completedAt := time.Now()
			mission.CompletedAt = &completedAt
			c.store.Update(context.Background(), mission)
		}
	}()

	return nil
}

// Stop gracefully cancels mission execution.
func (c *DefaultMissionController) Stop(ctx context.Context, missionID types.ID) error {
	c.executionMu.Lock()
	defer c.executionMu.Unlock()

	// Get mission
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		return err
	}

	// Check if mission is running
	cancel, exists := c.activeMissions[missionID]
	if !exists {
		return fmt.Errorf("mission is not running")
	}

	// Cancel execution
	cancel()

	// Update mission status
	mission.Status = MissionStatusCancelled
	completedAt := time.Now()
	mission.CompletedAt = &completedAt

	return c.store.Update(ctx, mission)
}

// Pause suspends mission at next checkpoint.
func (c *DefaultMissionController) Pause(ctx context.Context, missionID types.ID) error {
	// Get mission
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		return err
	}

	// Validate state transition
	if !mission.Status.CanTransitionTo(MissionStatusPaused) {
		return NewInvalidStateError(mission.Status, MissionStatusPaused)
	}

	// Update status
	mission.Status = MissionStatusPaused
	return c.store.Update(ctx, mission)
}

// Resume continues mission from checkpoint.
func (c *DefaultMissionController) Resume(ctx context.Context, missionID types.ID) error {
	// Get mission
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		return err
	}

	// Validate state transition
	if !mission.Status.CanTransitionTo(MissionStatusRunning) {
		return NewInvalidStateError(mission.Status, MissionStatusRunning)
	}

	// Start execution (which will resume from checkpoint)
	return c.Start(ctx, missionID)
}

// Delete removes mission (only terminal states).
func (c *DefaultMissionController) Delete(ctx context.Context, missionID types.ID) error {
	return c.store.Delete(ctx, missionID)
}

// Get retrieves a mission by ID.
func (c *DefaultMissionController) Get(ctx context.Context, missionID types.ID) (*Mission, error) {
	return c.store.Get(ctx, missionID)
}

// List retrieves missions with filtering.
func (c *DefaultMissionController) List(ctx context.Context, filter *MissionFilter) ([]*Mission, error) {
	return c.store.List(ctx, filter)
}

// GetProgress returns real-time progress for a mission.
func (c *DefaultMissionController) GetProgress(ctx context.Context, missionID types.ID) (*MissionProgress, error) {
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		return nil, err
	}

	return mission.GetProgress(), nil
}

// Ensure DefaultMissionController implements MissionController.
var _ MissionController = (*DefaultMissionController)(nil)
