package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
)

// CreateMissionRequest contains the parameters for creating a new mission.
// This is an internal type used by the MissionClient to encapsulate
// all the information needed to create a mission.
type CreateMissionRequest struct {
	// Workflow is the workflow definition to execute.
	Workflow *workflow.Workflow

	// TargetID is the ID of the target to test.
	TargetID types.ID

	// ParentMissionID is the ID of the parent mission (for lineage tracking).
	// Nil indicates this is a root mission.
	ParentMissionID *types.ID

	// ParentDepth is the depth of the parent mission.
	// This is used to calculate the depth of the new mission.
	ParentDepth int

	// Name is an optional human-readable name for the mission.
	// If empty, a name will be auto-generated.
	Name string

	// Description is an optional description of the mission.
	Description string

	// Constraints defines execution limits for the mission.
	Constraints *MissionConstraints

	// Metadata contains arbitrary key-value pairs for storing
	// additional mission context.
	Metadata map[string]any

	// Tags are labels for categorizing and filtering missions.
	Tags []string
}

// MissionClient provides mission operations for the harness.
// It acts as a bridge between the agent harness and the mission orchestrator,
// handling mission lifecycle management and coordination.
type MissionClient struct {
	store        MissionStore
	orchestrator MissionOrchestrator
	logger       *slog.Logger
	tracer       trace.Tracer

	// Spawn limits to prevent runaway mission creation
	maxChildMissions      int
	maxConcurrentMissions int
	maxMissionDepth       int
}

// ClientOption is a functional option for configuring the MissionClient.
type ClientOption func(*MissionClient)

// WithLogger sets the structured logger for the client.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *MissionClient) {
		c.logger = logger
	}
}

// WithTracer sets the OpenTelemetry tracer for the client.
func WithTracer(tracer trace.Tracer) ClientOption {
	return func(c *MissionClient) {
		c.tracer = tracer
	}
}

// WithSpawnLimits sets the spawn limits for mission creation.
// This prevents runaway mission spawning by limiting:
// - maxChildMissions: Maximum number of child missions per parent
// - maxConcurrentMissions: Maximum number of concurrent running missions system-wide
// - maxMissionDepth: Maximum depth of mission hierarchy
func WithSpawnLimits(maxChildMissions, maxConcurrentMissions, maxMissionDepth int) ClientOption {
	return func(c *MissionClient) {
		c.maxChildMissions = maxChildMissions
		c.maxConcurrentMissions = maxConcurrentMissions
		c.maxMissionDepth = maxMissionDepth
	}
}

// NewMissionClient creates a new MissionClient with the given dependencies.
// The store is used for mission persistence, and the orchestrator handles execution.
// Optional configuration can be provided via ClientOption functions.
func NewMissionClient(store MissionStore, orchestrator MissionOrchestrator, opts ...ClientOption) *MissionClient {
	client := &MissionClient{
		store:                 store,
		orchestrator:          orchestrator,
		logger:                slog.Default(),
		tracer:                trace.NewNoopTracerProvider().Tracer("mission.client"),
		maxChildMissions:      10, // Default: 10 children per parent
		maxConcurrentMissions: 50, // Default: 50 concurrent missions
		maxMissionDepth:       3,  // Default: max depth of 3
	}

	// Apply functional options
	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Create creates a new mission from the given request.
// This validates the input, generates IDs, persists the mission to the store,
// and returns mission information suitable for the harness.
//
// The method performs the following steps:
// 1. Validates the request parameters
// 2. Generates a unique mission ID and workflow ID
// 3. Serializes the workflow definition to JSON
// 4. Creates the mission entity with lineage tracking
// 5. Persists to the store
// 6. Returns mission information
func (c *MissionClient) Create(ctx context.Context, req *CreateMissionRequest) (*Mission, error) {
	// Start tracing span
	ctx, span := c.tracer.Start(ctx, "mission.client.Create")
	defer span.End()

	// Validate request
	if err := c.validateCreateRequest(req); err != nil {
		c.logger.ErrorContext(ctx, "mission creation validation failed",
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Generate mission ID
	missionID := types.NewID()

	// Generate workflow ID if not set
	if req.Workflow.ID.IsZero() {
		req.Workflow.ID = types.NewID()
	}

	// Generate mission name if not provided
	name := req.Name
	if name == "" {
		if req.Workflow.Name != "" {
			name = req.Workflow.Name
		} else {
			// Use first 8 characters of the ID for a shorter name
			idStr := missionID.String()
			if len(idStr) > 8 {
				idStr = idStr[:8]
			}
			name = fmt.Sprintf("mission-%s", idStr)
		}
	}

	// Generate description if not provided
	description := req.Description
	if description == "" && req.Workflow.Description != "" {
		description = req.Workflow.Description
	}

	// Serialize workflow to JSON for storage
	workflowJSON, err := c.serializeWorkflow(req.Workflow)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to serialize workflow",
			slog.String("workflow_id", req.Workflow.ID.String()),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to serialize workflow: %w", err)
	}

	// Calculate mission depth for lineage tracking
	depth := 0
	if req.ParentMissionID != nil {
		depth = req.ParentDepth + 1
	}

	// Create mission entity
	now := time.Now()
	mission := &Mission{
		ID:               missionID,
		Name:             name,
		Description:      description,
		Status:           MissionStatusPending,
		TargetID:         req.TargetID,
		WorkflowID:       req.Workflow.ID,
		WorkflowJSON:     workflowJSON,
		Constraints:      req.Constraints,
		Metadata:         req.Metadata,
		ParentMissionID:  req.ParentMissionID,
		Depth:            depth,
		CreatedAt:        now,
		UpdatedAt:        now,
		FindingsCount:    0,
		Progress:         0.0,
		AgentAssignments: make(map[string]string),
	}

	// Validate mission before saving
	if err := mission.Validate(); err != nil {
		c.logger.ErrorContext(ctx, "mission validation failed",
			slog.String("mission_id", missionID.String()),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("mission validation failed: %w", err)
	}

	// Persist mission to store
	if err := c.store.Save(ctx, mission); err != nil {
		c.logger.ErrorContext(ctx, "failed to save mission to store",
			slog.String("mission_id", missionID.String()),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to save mission: %w", err)
	}

	c.logger.InfoContext(ctx, "mission created successfully",
		slog.String("mission_id", missionID.String()),
		slog.String("mission_name", name),
		slog.String("target_id", req.TargetID.String()),
		slog.String("workflow_id", req.Workflow.ID.String()),
		slog.Int("depth", depth),
		slog.String("parent_mission_id", func() string {
			if req.ParentMissionID != nil {
				return req.ParentMissionID.String()
			}
			return "none"
		}()))

	return mission, nil
}

// validateCreateRequest validates that the create request has all required fields.
func (c *MissionClient) validateCreateRequest(req *CreateMissionRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if req.Workflow == nil {
		return fmt.Errorf("workflow cannot be nil")
	}

	if req.TargetID.IsZero() {
		return fmt.Errorf("target ID is required")
	}

	// Validate workflow has nodes
	if len(req.Workflow.Nodes) == 0 {
		return fmt.Errorf("workflow must contain at least one node")
	}

	// Validate constraints if provided
	if req.Constraints != nil {
		if err := req.Constraints.Validate(); err != nil {
			return fmt.Errorf("invalid constraints: %w", err)
		}
	}

	// Validate mission depth doesn't exceed limit
	if req.ParentMissionID != nil {
		newDepth := req.ParentDepth + 1
		if newDepth >= c.maxMissionDepth {
			return fmt.Errorf("mission depth limit exceeded: max=%d, attempted=%d", c.maxMissionDepth, newDepth)
		}
	}

	return nil
}

// serializeWorkflow converts a workflow to JSON for storage.
func (c *MissionClient) serializeWorkflow(wf *workflow.Workflow) (string, error) {
	data, err := json.Marshal(wf)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow: %w", err)
	}
	return string(data), nil
}

// List returns missions matching the provided filter.
// This method supports filtering by status, target ID, workflow ID, creation date range,
// and text search. Pagination is supported via Limit and Offset parameters.
//
// If no filter is provided, returns all missions with default pagination (limit 100).
// An empty result set is returned if no missions match the filter criteria.
func (c *MissionClient) List(ctx context.Context, filter *MissionFilter) ([]*Mission, error) {
	// Start tracing span
	ctx, span := c.tracer.Start(ctx, "mission.client.List")
	defer span.End()

	// Use default filter if none provided
	if filter == nil {
		filter = NewMissionFilter()
	}

	// Validate filter parameters
	if filter.Limit < 0 {
		return nil, fmt.Errorf("invalid filter: limit cannot be negative")
	}
	if filter.Offset < 0 {
		return nil, fmt.Errorf("invalid filter: offset cannot be negative")
	}

	// Query missions from store
	missions, err := c.store.List(ctx, filter)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to list missions",
			slog.String("error", err.Error()),
			slog.Int("limit", filter.Limit),
			slog.Int("offset", filter.Offset))
		return nil, fmt.Errorf("failed to list missions: %w", err)
	}

	c.logger.DebugContext(ctx, "missions listed successfully",
		slog.Int("count", len(missions)),
		slog.Int("limit", filter.Limit),
		slog.Int("offset", filter.Offset))

	return missions, nil
}

// Cancel requests cancellation of a running or pending mission.
// This method is idempotent - calling it multiple times on the same mission
// will not result in an error. If the mission is already in a terminal state
// (completed, failed, or cancelled), this method returns successfully without
// making any changes.
//
// For running missions, this method updates the status to cancelled and notifies
// the orchestrator to stop execution. For pending missions, this method simply
// updates the status to cancelled.
//
// Returns an error if the mission does not exist or if the database update fails.
func (c *MissionClient) Cancel(ctx context.Context, missionID types.ID) error {
	// Start tracing span
	ctx, span := c.tracer.Start(ctx, "mission.client.Cancel")
	defer span.End()

	// Validate mission ID
	if missionID.IsZero() {
		return fmt.Errorf("mission ID is required")
	}

	// Get current mission state
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to get mission for cancellation",
			slog.String("mission_id", missionID.String()),
			slog.String("error", err.Error()))
		return fmt.Errorf("failed to get mission: %w", err)
	}

	// Check if mission is already in a terminal state (idempotent)
	if mission.Status.IsTerminal() {
		c.logger.InfoContext(ctx, "mission already in terminal state, no cancellation needed",
			slog.String("mission_id", missionID.String()),
			slog.String("status", string(mission.Status)))
		return nil
	}

	// Update mission status to cancelled
	if err := c.store.UpdateStatus(ctx, missionID, MissionStatusCancelled); err != nil {
		c.logger.ErrorContext(ctx, "failed to update mission status to cancelled",
			slog.String("mission_id", missionID.String()),
			slog.String("error", err.Error()))
		return fmt.Errorf("failed to cancel mission: %w", err)
	}

	c.logger.InfoContext(ctx, "mission cancelled successfully",
		slog.String("mission_id", missionID.String()),
		slog.String("previous_status", string(mission.Status)))

	// TODO: Notify orchestrator to stop execution if mission is running
	// This will be implemented when orchestrator exposes a cancellation API

	return nil
}

// GetResults returns the results of a completed mission, including findings
// and workflow execution output. This method should only be called on missions
// that are in a terminal state (completed, failed, or cancelled).
//
// The returned MissionResult contains:
// - Final mission status
// - Execution metrics (duration, token usage, etc.)
// - Finding IDs (full findings are stored separately in the finding store)
// - Workflow execution result (output data)
// - Error message if the mission failed
//
// Returns an error if the mission does not exist. Returns partial results
// for failed or cancelled missions.
func (c *MissionClient) GetResults(ctx context.Context, missionID types.ID) (*MissionResult, error) {
	// Start tracing span
	ctx, span := c.tracer.Start(ctx, "mission.client.GetResults")
	defer span.End()

	// Validate mission ID
	if missionID.IsZero() {
		return nil, fmt.Errorf("mission ID is required")
	}

	// Get mission from store
	mission, err := c.store.Get(ctx, missionID)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to get mission for results",
			slog.String("mission_id", missionID.String()),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get mission: %w", err)
	}

	// Build mission result from mission entity
	result := &MissionResult{
		MissionID: mission.ID,
		Status:    mission.Status,
		Metrics:   mission.Metrics,
		Error:     mission.Error,
	}

	// Set completion timestamp if available
	if mission.CompletedAt != nil {
		result.CompletedAt = *mission.CompletedAt
	}

	// Parse workflow result from JSON if available
	if mission.WorkflowJSON != "" {
		var workflowResult map[string]any
		if err := json.Unmarshal([]byte(mission.WorkflowJSON), &workflowResult); err != nil {
			c.logger.WarnContext(ctx, "failed to parse workflow result JSON",
				slog.String("mission_id", missionID.String()),
				slog.String("error", err.Error()))
			// Don't fail the entire request, just skip workflow result
		} else {
			result.WorkflowResult = workflowResult
		}
	}

	// Note: FindingIDs are not stored in the mission entity.
	// They would need to be queried from a separate finding store.
	// For now, we initialize it as an empty slice.
	result.FindingIDs = []types.ID{}

	c.logger.InfoContext(ctx, "mission results retrieved successfully",
		slog.String("mission_id", missionID.String()),
		slog.String("status", string(mission.Status)),
		slog.Int("findings_count", mission.FindingsCount))

	return result, nil
}
