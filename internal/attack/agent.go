package attack

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
)

// AgentSelector selects and validates agents for attacks.
// It provides methods to look up agents by name and list available agents.
type AgentSelector interface {
	// Select retrieves an agent by name from the registry.
	// Returns ErrAgentRequired if agentName is empty.
	// Returns ErrAgentNotFound if the agent doesn't exist.
	Select(ctx context.Context, agentName string) (agent.Agent, error)

	// ListAvailable returns information about all registered agents.
	ListAvailable(ctx context.Context) ([]AgentInfo, error)
}

// AgentInfo contains display information about an available agent.
// This is used for listing agents and providing helpful error messages.
type AgentInfo struct {
	Name           string                    `json:"name"`
	Description    string                    `json:"description"`
	Capabilities   []string                  `json:"capabilities"`
	TargetTypes    []component.TargetType    `json:"target_types"`
	TechniqueTypes []component.TechniqueType `json:"technique_types"`
	Version        string                    `json:"version"`
	IsExternal     bool                      `json:"is_external"`
}

// DefaultAgentSelector implements AgentSelector using the agent registry.
type DefaultAgentSelector struct {
	registry agent.AgentRegistry
}

// NewAgentSelector creates a new agent selector backed by the given registry.
func NewAgentSelector(registry agent.AgentRegistry) *DefaultAgentSelector {
	return &DefaultAgentSelector{
		registry: registry,
	}
}

// Select retrieves an agent by name from the registry.
// Returns ErrAgentRequired if agentName is empty.
// Returns ErrAgentNotFound if the agent doesn't exist.
func (s *DefaultAgentSelector) Select(ctx context.Context, agentName string) (agent.Agent, error) {
	// Validate agent name is provided
	if agentName == "" {
		// Get available agents for error message
		availableAgents, listErr := s.getAvailableAgentNames(ctx)
		if listErr != nil {
			// Fallback to generic error if we can't list agents
			return nil, NewAgentRequiredError([]string{})
		}
		return nil, NewAgentRequiredError(availableAgents)
	}

	// Create agent configuration
	cfg := agent.NewAgentConfig(agentName)

	// Attempt to create the agent
	agentInstance, err := s.registry.Create(agentName, cfg)
	if err != nil {
		// Get available agents for helpful error message
		availableAgents, listErr := s.getAvailableAgentNames(ctx)
		if listErr != nil {
			// Fallback to basic error if we can't list agents
			return nil, NewAgentNotFoundError(agentName, []string{})
		}
		return nil, NewAgentNotFoundError(agentName, availableAgents)
	}

	return agentInstance, nil
}

// ListAvailable returns information about all registered agents.
func (s *DefaultAgentSelector) ListAvailable(ctx context.Context) ([]AgentInfo, error) {
	descriptors := s.registry.List()

	// Convert agent descriptors to AgentInfo
	infos := make([]AgentInfo, 0, len(descriptors))
	for _, desc := range descriptors {
		info := AgentInfo{
			Name:           desc.Name,
			Description:    desc.Description,
			Capabilities:   desc.Capabilities,
			TargetTypes:    desc.TargetTypes,
			TechniqueTypes: desc.TechniqueTypes,
			Version:        desc.Version,
			IsExternal:     desc.IsExternal,
		}
		infos = append(infos, info)
	}

	// Sort by name for consistent ordering
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos, nil
}

// getAvailableAgentNames returns a sorted list of available agent names.
// This is a helper method for generating error messages.
func (s *DefaultAgentSelector) getAvailableAgentNames(ctx context.Context) ([]string, error) {
	infos, err := s.ListAvailable(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
	}

	return names, nil
}

// ValidateAgentName checks if an agent name is valid and exists.
// This is a convenience function that combines validation checks.
func ValidateAgentName(ctx context.Context, selector AgentSelector, agentName string) error {
	if agentName == "" {
		availableAgents, err := selector.ListAvailable(ctx)
		if err != nil {
			return NewAgentRequiredError([]string{})
		}

		names := make([]string, 0, len(availableAgents))
		for _, agent := range availableAgents {
			names = append(names, agent.Name)
		}
		return NewAgentRequiredError(names)
	}

	// Check if agent exists by attempting to select it
	_, err := selector.Select(ctx, agentName)
	return err
}

// FormatAgentList formats a list of agent names for display in error messages.
// Returns a human-readable string like "agent1, agent2, agent3"
func FormatAgentList(agents []string) string {
	if len(agents) == 0 {
		return "(no agents available)"
	}

	// Sort for consistent display
	sorted := make([]string, len(agents))
	copy(sorted, agents)
	sort.Strings(sorted)

	return strings.Join(sorted, ", ")
}

// FormatAgentInfoList formats a list of AgentInfo for display.
// Returns a multi-line string with agent details.
func FormatAgentInfoList(infos []AgentInfo) string {
	if len(infos) == 0 {
		return "No agents available"
	}

	var builder strings.Builder
	builder.WriteString("Available agents:\n")

	for _, info := range infos {
		fmt.Fprintf(&builder, "  - %s (%s): %s\n",
			info.Name,
			info.Version,
			info.Description)
	}

	return builder.String()
}
