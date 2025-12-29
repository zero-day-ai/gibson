package prompt

// Message represents a chat message for LLM consumption.
// Messages follow the standard chat format with role and content.
type Message struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // The message content
}

// AssembleResult contains the assembled prompts ready for LLM consumption.
// It provides multiple output formats to support different LLM APIs:
//   - System/User: Simple two-message format (legacy APIs)
//   - Messages: Structured message array (modern chat APIs)
//   - Prompts: Debug information showing all prompts used
type AssembleResult struct {
	// System contains the combined system message from all system positions
	System string `json:"system"`

	// User contains the combined user message from all user positions
	User string `json:"user"`

	// Messages is the structured message array for chat-based APIs
	// Typically contains [{"role": "system", "content": "..."}, {"role": "user", "content": "..."}]
	Messages []Message `json:"messages"`

	// Prompts contains all prompts that were used in assembly (for debugging)
	// This field is omitted if empty to reduce response size
	Prompts []Prompt `json:"prompts,omitempty"`
}
