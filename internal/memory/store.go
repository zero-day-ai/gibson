package memory

// MemoryStore is the agent-facing interface that composes all three memory tiers.
// This interface is designed for use by AgentHarness in Stage 6, providing clean
// access to working memory, mission memory, and long-term memory through a single interface.
type MemoryStore interface {
	// Working returns the working memory instance for ephemeral key-value storage.
	Working() WorkingMemory

	// Mission returns the mission memory instance for persistent mission-scoped storage.
	Mission() MissionMemory

	// LongTerm returns the long-term memory instance for semantic search over historical data.
	LongTerm() LongTermMemory
}
