package mission

import (
	"context"
	"fmt"

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

// DefaultMissionService implements MissionService.
type DefaultMissionService struct {
	store MissionStore
	// Additional dependencies would go here (workflow store, finding store, etc.)
}

// NewMissionService creates a new mission service.
func NewMissionService(store MissionStore) *DefaultMissionService {
	return &DefaultMissionService{
		store: store,
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
	if err := mission.Validate(); err != nil {
		return err
	}

	// TODO: Validate target exists
	// TODO: Validate workflow exists
	// TODO: Validate constraints are reasonable

	return nil
}

// LoadWorkflow resolves workflow from store or inline definition.
func (s *DefaultMissionService) LoadWorkflow(ctx context.Context, config *MissionWorkflowConfig) (interface{}, error) {
	// TODO: Implement workflow resolution
	// This would either load from workflow store or parse inline definition
	return nil, fmt.Errorf("not implemented")
}

// AggregateFindings collects findings from all sources for a mission.
func (s *DefaultMissionService) AggregateFindings(ctx context.Context, missionID types.ID) ([]interface{}, error) {
	// TODO: Query finding store for all findings related to this mission
	return nil, fmt.Errorf("not implemented")
}

// GetSummary returns mission summary with finding counts.
func (s *DefaultMissionService) GetSummary(ctx context.Context, missionID types.ID) (*MissionSummary, error) {
	mission, err := s.store.Get(ctx, missionID)
	if err != nil {
		return nil, err
	}

	summary := &MissionSummary{
		Mission:         mission,
		FindingsCount:   0,
		FindingsByLevel: make(map[string]int),
		Progress:        mission.GetProgress(),
	}

	// TODO: Aggregate findings and populate counts

	return summary, nil
}

// Ensure DefaultMissionService implements MissionService.
var _ MissionService = (*DefaultMissionService)(nil)
