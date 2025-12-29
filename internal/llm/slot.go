package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// SlotManager manages LLM slot resolution and validation.
// It matches slot requirements against available providers and models,
// ensuring constraints are satisfied.
type SlotManager interface {
	// ResolveSlot resolves a slot definition to a specific provider and model
	// that satisfies the slot's constraints and configuration.
	// Returns ErrNoMatchingProvider if no provider/model combination meets the requirements.
	ResolveSlot(ctx context.Context, slot agent.SlotDefinition, override *agent.SlotConfig) (LLMProvider, ModelInfo, error)

	// ValidateSlot validates that a slot configuration can be satisfied by available providers
	ValidateSlot(ctx context.Context, slot agent.SlotDefinition) error
}

// DefaultSlotManager implements SlotManager with provider registry integration.
type DefaultSlotManager struct {
	registry LLMRegistry
}

// NewSlotManager creates a new DefaultSlotManager with the given registry
func NewSlotManager(registry LLMRegistry) *DefaultSlotManager {
	return &DefaultSlotManager{
		registry: registry,
	}
}

// ResolveSlot resolves a slot definition to a specific provider and model.
// It applies configuration overrides and validates that the selected provider/model
// meets all constraints defined in the slot.
//
// Resolution process:
// 1. Merge slot default config with any override
// 2. Get the specified provider from the registry
// 3. Retrieve model information from the provider
// 4. Validate the model against slot constraints
// 5. Return the provider and model info
//
// Returns ErrNoMatchingProvider if:
// - The specified provider is not registered
// - The specified model doesn't exist
// - The model doesn't meet MinContextWindow constraint
// - The model doesn't support all RequiredFeatures
func (m *DefaultSlotManager) ResolveSlot(ctx context.Context, slot agent.SlotDefinition, override *agent.SlotConfig) (LLMProvider, ModelInfo, error) {
	// Merge configuration
	config := slot.MergeConfig(override)

	// Validate merged config
	if config.Provider == "" {
		return nil, ModelInfo{}, types.NewError(
			ErrInvalidSlotConfig,
			"provider cannot be empty",
		)
	}
	if config.Model == "" {
		return nil, ModelInfo{}, types.NewError(
			ErrInvalidSlotConfig,
			"model cannot be empty",
		)
	}

	// Get provider from registry
	provider, err := m.registry.GetProvider(config.Provider)
	if err != nil {
		return nil, ModelInfo{}, types.WrapError(
			ErrNoMatchingProvider,
			fmt.Sprintf("provider %q not found for slot %q", config.Provider, slot.Name),
			err,
		)
	}

	// Get model information
	models, err := provider.Models(ctx)
	if err != nil {
		return nil, ModelInfo{}, types.WrapError(
			ErrNoMatchingProvider,
			fmt.Sprintf("failed to get models from provider %q", config.Provider),
			err,
		)
	}

	// Find the specified model
	var modelInfo ModelInfo
	found := false
	for _, model := range models {
		if model.Name == config.Model {
			modelInfo = model
			found = true
			break
		}
	}

	if !found {
		return nil, ModelInfo{}, types.NewError(
			ErrNoMatchingProvider,
			fmt.Sprintf("model %q not found in provider %q for slot %q", config.Model, config.Provider, slot.Name),
		)
	}

	// Validate constraints
	if err := m.validateConstraints(slot, modelInfo); err != nil {
		return nil, ModelInfo{}, types.WrapError(
			ErrNoMatchingProvider,
			fmt.Sprintf("model %q from provider %q does not meet constraints for slot %q",
				config.Model, config.Provider, slot.Name),
			err,
		)
	}

	return provider, modelInfo, nil
}

// ValidateSlot validates that a slot configuration can be satisfied by available providers.
// It attempts to resolve the slot with its default configuration to ensure viability.
func (m *DefaultSlotManager) ValidateSlot(ctx context.Context, slot agent.SlotDefinition) error {
	_, _, err := m.ResolveSlot(ctx, slot, nil)
	return err
}

// validateConstraints checks if a model meets the slot's constraints
func (m *DefaultSlotManager) validateConstraints(slot agent.SlotDefinition, model ModelInfo) error {
	constraints := slot.Constraints

	// Check MinContextWindow constraint
	if constraints.MinContextWindow > 0 {
		if model.ContextWindow < constraints.MinContextWindow {
			return types.NewError(
				ErrInvalidSlotConfig,
				fmt.Sprintf("model context window %d is less than required %d",
					model.ContextWindow, constraints.MinContextWindow),
			)
		}
	}

	// Check RequiredFeatures constraint
	if len(constraints.RequiredFeatures) > 0 {
		missingFeatures := []string{}
		for _, required := range constraints.RequiredFeatures {
			// Convert agent feature constants to model feature format
			modelFeature := convertAgentFeatureToModelFeature(required)
			if !model.SupportsFeature(modelFeature) {
				missingFeatures = append(missingFeatures, required)
			}
		}

		if len(missingFeatures) > 0 {
			return types.NewError(
				ErrInvalidSlotConfig,
				fmt.Sprintf("model missing required features: %s",
					strings.Join(missingFeatures, ", ")),
			)
		}
	}

	return nil
}

// convertAgentFeatureToModelFeature maps agent feature constants to model feature strings.
// This handles the conversion between agent.Feature* constants and ModelInfo.Features.
func convertAgentFeatureToModelFeature(agentFeature string) string {
	// Map agent feature constants to model features
	// agent.FeatureToolUse -> "tool_use" (already matches)
	// agent.FeatureVision -> "vision" (already matches)
	// agent.FeatureStreaming -> "streaming" (already matches)
	// agent.FeatureJSONMode -> "json_mode" (already matches)
	return agentFeature
}
