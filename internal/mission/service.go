package mission

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MissionService provides business logic for mission operations.
type MissionService interface {
	// CreateFromConfig creates a mission from a YAML configuration
	CreateFromConfig(ctx context.Context, config *MissionConfig) (*Mission, error)

	// ValidateMission validates all mission fields and references
	ValidateMission(ctx context.Context, mission *Mission) error

	// LoadWorkflow resolves workflow from store or inline definition
	LoadWorkflow(ctx context.Context, config *MissionWorkflowConfig) (interface{}, error)

	// AggregateFindings collects findings from all sources for a mission
	AggregateFindings(ctx context.Context, missionID types.ID) ([]interface{}, error)

	// GetSummary returns mission summary with finding counts
	GetSummary(ctx context.Context, missionID types.ID) (*MissionSummary, error)
}

// MissionSummary provides a high-level overview of a mission.
type MissionSummary struct {
	Mission         *Mission         `json:"mission"`
	FindingsCount   int              `json:"findings_count"`
	FindingsByLevel map[string]int   `json:"findings_by_level"`
	Progress        *MissionProgress `json:"progress"`
}

// WorkflowStore provides access to workflow definitions.
type WorkflowStore interface {
	// Get retrieves a workflow by ID
	Get(ctx context.Context, id types.ID) (interface{}, error)
}

// FindingStore provides access to findings.
type FindingStore interface {
	// GetByMission retrieves all findings for a mission
	GetByMission(ctx context.Context, missionID types.ID) ([]interface{}, error)

	// CountBySeverity returns finding counts grouped by severity
	CountBySeverity(ctx context.Context, missionID types.ID) (map[string]int, error)
}

// DefaultMissionService implements MissionService.
type DefaultMissionService struct {
	store         MissionStore
	workflowStore WorkflowStore
	findingStore  FindingStore
}

// NewMissionService creates a new mission service.
func NewMissionService(store MissionStore, workflowStore WorkflowStore, findingStore FindingStore) *DefaultMissionService {
	return &DefaultMissionService{
		store:         store,
		workflowStore: workflowStore,
		findingStore:  findingStore,
	}
}

// CreateFromConfig creates a mission from a YAML configuration.
func (s *DefaultMissionService) CreateFromConfig(ctx context.Context, config *MissionConfig) (*Mission, error) {
	// TODO: Implement full logic to resolve target and workflow references
	// For now, create a basic mission structure

	mission := &Mission{
		ID:          types.NewID(),
		Name:        config.Name,
		Description: config.Description,
		Status:      MissionStatusPending,
		// TargetID and WorkflowID would be resolved from config
	}

	// Convert constraints if specified
	if config.Constraints != nil {
		constraints, err := config.Constraints.ToConstraints()
		if err != nil {
			return nil, fmt.Errorf("failed to convert constraints: %w", err)
		}
		mission.Constraints = constraints
	}

	// Validate the mission
	if err := s.ValidateMission(ctx, mission); err != nil {
		return nil, err
	}

	// Save to store
	if err := s.store.Save(ctx, mission); err != nil {
		return nil, fmt.Errorf("failed to save mission: %w", err)
	}

	return mission, nil
}

// ValidateMission validates all mission fields and references.
func (s *DefaultMissionService) ValidateMission(ctx context.Context, mission *Mission) error {
	// First validate basic mission fields
	if err := mission.Validate(); err != nil {
		return err
	}

	// Validate workflow exists if WorkflowID is set
	if !mission.WorkflowID.IsZero() && s.workflowStore != nil {
		_, err := s.workflowStore.Get(ctx, mission.WorkflowID)
		if err != nil {
			return fmt.Errorf("workflow validation failed: %w", err)
		}
	}

	// Validate constraints are reasonable if set
	if mission.Constraints != nil {
		if err := mission.Constraints.Validate(); err != nil {
			return fmt.Errorf("constraints validation failed: %w", err)
		}

		// Additional reasonableness checks
		if mission.Constraints.MaxDuration > 0 && mission.Constraints.MaxDuration < 1*time.Minute {
			return fmt.Errorf("max_duration too short: minimum 1 minute required")
		}

		// Note: MaxFindings == 0 is treated as "unlimited/not set"
		// Only validate if a positive value is specified but is invalid (which can't happen for int)

		if mission.Constraints.MaxCost > 0 && mission.Constraints.MaxCost < 0.01 {
			return fmt.Errorf("max_cost too low: minimum $0.01 required")
		}

		if mission.Constraints.MaxTokens > 0 && mission.Constraints.MaxTokens < 1000 {
			return fmt.Errorf("max_tokens too low: minimum 1000 tokens required")
		}
	}

	return nil
}

// LoadWorkflow resolves workflow from store or inline definition.
func (s *DefaultMissionService) LoadWorkflow(ctx context.Context, config *MissionWorkflowConfig) (interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("workflow config is required")
	}

	// If inline workflow is provided, return it directly (already validated in config)
	if config.Inline != nil {
		return config.Inline, nil
	}

	// If workflow reference is provided, load from store
	if config.Reference != "" {
		if s.workflowStore == nil {
			return nil, fmt.Errorf("workflow store not configured but workflow reference provided: %s", config.Reference)
		}

		// Try to parse reference as ID
		workflowID, err := types.ParseID(config.Reference)
		if err != nil {
			// If not a valid ID, treat as workflow name
			// For now, return error since we don't have name-based lookup
			return nil, fmt.Errorf("invalid workflow ID: %s", config.Reference)
		}

		workflow, err := s.workflowStore.Get(ctx, workflowID)
		if err != nil {
			return nil, fmt.Errorf("failed to load workflow %s: %w", config.Reference, err)
		}

		return workflow, nil
	}

	return nil, fmt.Errorf("workflow config must specify either 'reference' or 'inline'")
}

// AggregateFindings collects findings from all sources for a mission.
func (s *DefaultMissionService) AggregateFindings(ctx context.Context, missionID types.ID) ([]interface{}, error) {
	if missionID.IsZero() {
		return nil, fmt.Errorf("mission ID is required")
	}

	if s.findingStore == nil {
		return nil, fmt.Errorf("finding store not configured")
	}

	// Query finding store for all findings related to this mission
	findings, err := s.findingStore.GetByMission(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve findings for mission %s: %w", missionID, err)
	}

	return findings, nil
}

// GetSummary returns mission summary with finding counts.
func (s *DefaultMissionService) GetSummary(ctx context.Context, missionID types.ID) (*MissionSummary, error) {
	if missionID.IsZero() {
		return nil, fmt.Errorf("mission ID is required")
	}

	mission, err := s.store.Get(ctx, missionID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve mission: %w", err)
	}

	summary := &MissionSummary{
		Mission:         mission,
		FindingsCount:   0,
		FindingsByLevel: make(map[string]int),
		Progress:        mission.GetProgress(),
	}

	// If finding store is configured, get actual finding counts
	if s.findingStore != nil {
		// Get findings count by severity
		countsBySeverity, err := s.findingStore.CountBySeverity(ctx, missionID)
		if err != nil {
			// Log the error but don't fail the whole summary
			// Continue with zero counts
			summary.FindingsByLevel = make(map[string]int)
		} else {
			// Calculate total findings
			totalFindings := 0
			for severity, count := range countsBySeverity {
				summary.FindingsByLevel[severity] = count
				totalFindings += count
			}
			summary.FindingsCount = totalFindings
		}
	}

	return summary, nil
}

// Ensure DefaultMissionService implements MissionService.
var _ MissionService = (*DefaultMissionService)(nil)
