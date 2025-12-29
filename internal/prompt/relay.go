package prompt

import (
	"encoding/json"
)

// RelayContext provides context for relay operations.
// It contains information about the delegation from a parent agent to a sub-agent.
type RelayContext struct {
	// SourceAgent is the name of the parent agent delegating the task
	SourceAgent string `json:"source_agent"`

	// TargetAgent is the name of the sub-agent receiving the delegated task
	TargetAgent string `json:"target_agent"`

	// Task is the description of the delegated task
	Task string `json:"task"`

	// Prompts are the original prompts being relayed
	Prompts []Prompt `json:"prompts"`

	// Memory stores arbitrary key-value pairs for context sharing
	Memory map[string]any `json:"memory,omitempty"`

	// Constraints are limitations or requirements for the sub-agent
	Constraints []string `json:"constraints,omitempty"`
}

// PromptRelay transforms prompts for sub-agent delegation.
// It applies a series of transformers to prompts while ensuring immutability.
type PromptRelay interface {
	// Relay transforms prompts through the given transformers.
	// Transformers are applied in sequence, each receiving the output of the previous.
	// Returns the transformed prompts or an error if any transformer fails.
	Relay(ctx *RelayContext, transformers ...PromptTransformer) ([]Prompt, error)
}

// DefaultPromptRelay implements PromptRelay with immutable transformations.
type DefaultPromptRelay struct{}

// NewPromptRelay creates a new PromptRelay instance.
func NewPromptRelay() PromptRelay {
	return &DefaultPromptRelay{}
}

// Relay transforms prompts through the given transformers while maintaining immutability.
// Each transformer receives a deep copy of the prompts to prevent mutation of originals.
func (r *DefaultPromptRelay) Relay(ctx *RelayContext, transformers ...PromptTransformer) ([]Prompt, error) {
	// Deep copy the input prompts to ensure immutability
	currentPrompts, err := deepCopyPrompts(ctx.Prompts)
	if err != nil {
		return nil, NewRelayFailedError("deep copy", err)
	}

	// Apply each transformer in sequence
	for _, transformer := range transformers {
		transformed, err := transformer.Transform(ctx, currentPrompts)
		if err != nil {
			return nil, NewRelayFailedError(transformer.Name(), err)
		}
		currentPrompts = transformed
	}

	return currentPrompts, nil
}

// deepCopyPrompts creates a deep copy of the prompts slice.
// This ensures that transformers cannot modify the original prompts.
func deepCopyPrompts(prompts []Prompt) ([]Prompt, error) {
	// Use JSON marshaling/unmarshaling for deep copy
	// This ensures all nested structures are copied
	data, err := json.Marshal(prompts)
	if err != nil {
		return nil, err
	}

	var copied []Prompt
	if err := json.Unmarshal(data, &copied); err != nil {
		return nil, err
	}

	return copied, nil
}
