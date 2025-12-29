package prompt

// PromptTransformer transforms prompts during relay operations.
// Transformers are applied in sequence during the relay process to modify
// prompts for delegation to sub-agents.
type PromptTransformer interface {
	// Name returns the transformer name for logging and debugging.
	Name() string

	// Transform applies the transformation to prompts.
	// Must return copies, not modify originals to ensure immutability.
	// Returns the transformed prompts or an error if transformation fails.
	Transform(ctx *RelayContext, prompts []Prompt) ([]Prompt, error)
}
