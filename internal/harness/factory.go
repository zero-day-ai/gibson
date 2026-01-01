package harness

import (
	"context"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
)

// HarnessFactory is a function type for creating child harnesses.
// This is used by concrete harness implementations for creating child harnesses during delegation.
// The function receives context, mission context, and target info to create a new harness.
type HarnessFactory func(ctx context.Context, missionCtx MissionContext, targetInfo TargetInfo) (AgentHarness, error)

// HarnessFactoryInterface creates configured AgentHarness instances.
// This factory interface provides a structured way to create harnesses with
// proper dependency injection and context propagation.
//
// The factory pattern enables:
//   - Consistent harness initialization across the framework
//   - Testability through dependency injection
//   - Support for hierarchical agent execution (parent-child relationships)
//   - Centralized configuration validation
type HarnessFactoryInterface interface {
	// Create creates a new AgentHarness for the given agent and mission context.
	//
	// Parameters:
	//   - agentName: Name of the agent this harness is for
	//   - missionCtx: Mission context providing mission-level metadata
	//   - target: Target information for the current mission
	//
	// Returns:
	//   - AgentHarness: Fully configured harness ready for agent execution
	//   - error: Non-nil if creation fails
	Create(agentName string, missionCtx MissionContext, target TargetInfo) (AgentHarness, error)

	// CreateChild creates a child harness from a parent for sub-agent delegation.
	//
	// Parameters:
	//   - parent: The parent harness that is delegating to a sub-agent
	//   - agentName: Name of the child agent this harness is for
	//
	// Returns:
	//   - AgentHarness: Child harness ready for sub-agent execution
	//   - error: Non-nil if creation fails
	//
	// The child harness shares certain state with the parent to enable coordination
	// while maintaining isolation for agent-specific concerns.
	CreateChild(parent AgentHarness, agentName string) (AgentHarness, error)
}

// DefaultHarnessFactory implements HarnessFactoryInterface using the HarnessConfig.
// It provides a production-ready implementation of the factory pattern for creating
// agent harnesses with proper dependency injection and state management.
type DefaultHarnessFactory struct {
	config HarnessConfig
}

// NewHarnessFactory creates a new DefaultHarnessFactory with the given configuration.
//
// The factory validates the configuration and applies defaults before storing it.
// This ensures that all harnesses created by this factory have consistent,
// valid configuration.
//
// Parameters:
//   - config: Harness configuration with registries and optional dependencies
//
// Returns:
//   - *DefaultHarnessFactory: Ready-to-use factory instance
//   - error: Non-nil if config validation fails
func NewHarnessFactory(config HarnessConfig) (*DefaultHarnessFactory, error) {
	// Apply defaults for optional fields
	config.ApplyDefaults()

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, types.WrapError(
			ErrHarnessInvalidConfig,
			"harness configuration validation failed",
			err,
		)
	}

	return &DefaultHarnessFactory{
		config: config,
	}, nil
}

// Create creates a new AgentHarness for the given agent and mission context.
//
// The harness is configured with:
//   - Fresh token usage tracker scoped to mission + agent
//   - Logger with "agent", "mission_id", and "mission_name" attributes
//   - All registries from factory configuration
//   - Memory manager from factory configuration
//   - Finding store from factory configuration
//   - Metrics recorder from factory configuration
//   - Tracer from factory configuration
func (f *DefaultHarnessFactory) Create(agentName string, missionCtx MissionContext, target TargetInfo) (AgentHarness, error) {
	// Validate agent name
	if agentName == "" {
		return nil, types.NewError(
			ErrHarnessInvalidConfig,
			"agent name cannot be empty",
		)
	}

	// Update mission context to reflect current agent
	updatedMissionCtx := missionCtx
	updatedMissionCtx.CurrentAgent = agentName

	// Create token usage tracker for this agent
	tokenTrackerPtr := llm.NewTokenTracker(nil)
	var tokenTracker llm.TokenTracker = tokenTrackerPtr

	// Create logger with agent context
	logger := f.config.Logger.With(
		slog.String("agent", agentName),
		slog.String("mission_id", missionCtx.ID.String()),
		slog.String("mission_name", missionCtx.Name),
	)

	// Create self-referential factory for child harness creation during delegation
	selfFactory := func(ctx context.Context, childMissionCtx MissionContext, childTarget TargetInfo) (AgentHarness, error) {
		childAgentName := childMissionCtx.CurrentAgent
		if childAgentName == "" {
			return nil, types.NewError(ErrHarnessInvalidConfig, "mission context missing CurrentAgent field")
		}
		return f.Create(childAgentName, childMissionCtx, childTarget)
	}

	// Create and return DefaultAgentHarness
	harness := &DefaultAgentHarness{
		slotManager:         f.config.SlotManager,
		llmRegistry:         f.config.LLMRegistry,
		toolRegistry:        f.config.ToolRegistry,
		pluginRegistry:      f.config.PluginRegistry,
		agentRegistry:       f.config.AgentRegistry,
		memoryStore:         f.config.MemoryManager,
		findingStore:        f.config.FindingStore,
		factory:             selfFactory,
		missionCtx:          updatedMissionCtx,
		targetInfo:          target,
		tracer:              f.config.Tracer,
		logger:              logger,
		metrics:             f.config.Metrics,
		tokenUsage:          tokenTracker,
		graphRAGBridge:      f.config.GraphRAGBridge,
		graphRAGQueryBridge: f.config.GraphRAGQueryBridge,
	}

	return harness, nil
}

// CreateChild creates a child harness from a parent for sub-agent delegation.
//
// The child harness will:
//   - Share memory store with parent (mission and long-term memory shared)
//   - Share finding store with parent
//   - Update MissionContext.CurrentAgent to the child agent name
//   - Have its own token usage tracker (scoped to child agent)
//   - Have logger with updated "agent" attribute for context
//   - Inherit all registries and configuration from parent
func (f *DefaultHarnessFactory) CreateChild(parent AgentHarness, agentName string) (AgentHarness, error) {
	// Validate inputs
	if parent == nil {
		return nil, types.NewError(
			ErrHarnessInvalidConfig,
			"parent harness cannot be nil",
		)
	}
	if agentName == "" {
		return nil, types.NewError(
			ErrHarnessInvalidConfig,
			"agent name cannot be empty",
		)
	}

	// Get parent's mission context and update the current agent
	parentMission := parent.Mission()
	childMission := parentMission
	childMission.CurrentAgent = agentName

	// Get parent's target info (shared with child)
	targetInfo := parent.Target()

	// Create the child harness with updated mission context
	// The child will share the parent's memory store (through the factory config)
	return f.Create(agentName, childMission, targetInfo)
}

// Config returns a copy of the factory's configuration.
// This is useful for inspection and debugging.
func (f *DefaultHarnessFactory) Config() HarnessConfig {
	return f.config
}

// Ensure DefaultHarnessFactory implements HarnessFactoryInterface
var _ HarnessFactoryInterface = (*DefaultHarnessFactory)(nil)

// ────────────────────────────────────────────────────────────────────────────
// Type Aliases for Spec Compatibility
// ────────────────────────────────────────────────────────────────────────────

// HarnessFactoryConfig is an alias for HarnessConfig for spec compatibility.
// This provides the naming convention used in the attack-orchestration-integration spec.
type HarnessFactoryConfig = HarnessConfig

// NewDefaultHarnessFactory is an alias for NewHarnessFactory for spec compatibility.
// This provides the naming convention used in the attack-orchestration-integration spec.
//
// The factory validates the configuration and applies defaults before storing it.
// This ensures that all harnesses created by this factory have consistent,
// valid configuration.
//
// Parameters:
//   - config: Harness configuration with registries and optional dependencies
//
// Returns:
//   - *DefaultHarnessFactory: Ready-to-use factory instance
//   - error: Non-nil if config validation fails
func NewDefaultHarnessFactory(config HarnessFactoryConfig) (*DefaultHarnessFactory, error) {
	return NewHarnessFactory(config)
}
