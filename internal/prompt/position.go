package prompt

// Position represents the placement position of a prompt in the message sequence.
// Positions are ordered and determine where prompt content appears in the final message.
type Position string

// Position constants define all valid prompt positions in their sequential order.
// The order matters: prompts are assembled from PositionSystemPrefix to PositionUserSuffix.
const (
	PositionSystemPrefix Position = "system_prefix"
	PositionSystem       Position = "system"
	PositionSystemSuffix Position = "system_suffix"
	PositionContext      Position = "context"
	PositionTools        Position = "tools"
	PositionPlugins      Position = "plugins"
	PositionAgents       Position = "agents"
	PositionConstraints  Position = "constraints"
	PositionExamples     Position = "examples"
	PositionUserPrefix   Position = "user_prefix"
	PositionUser         Position = "user"
	PositionUserSuffix   Position = "user_suffix"
)

// positionOrder maps each Position to its sequential order index (0-11).
var positionOrder = map[Position]int{
	PositionSystemPrefix: 0,
	PositionSystem:       1,
	PositionSystemSuffix: 2,
	PositionContext:      3,
	PositionTools:        4,
	PositionPlugins:      5,
	PositionAgents:       6,
	PositionConstraints:  7,
	PositionExamples:     8,
	PositionUserPrefix:   9,
	PositionUser:         10,
	PositionUserSuffix:   11,
}

// Order returns the sequential order index (0-11) for this position.
// Returns -1 if the position is not recognized.
func (p Position) Order() int {
	if order, exists := positionOrder[p]; exists {
		return order
	}
	return -1
}

// IsValid checks if this position is a recognized valid position.
// Returns true if the position exists in the position ordering.
func (p Position) IsValid() bool {
	_, exists := positionOrder[p]
	return exists
}

// AllPositions returns all valid positions in their sequential order.
// The returned slice contains positions from PositionSystemPrefix to PositionUserSuffix.
func AllPositions() []Position {
	return []Position{
		PositionSystemPrefix,
		PositionSystem,
		PositionSystemSuffix,
		PositionContext,
		PositionTools,
		PositionPlugins,
		PositionAgents,
		PositionConstraints,
		PositionExamples,
		PositionUserPrefix,
		PositionUser,
		PositionUserSuffix,
	}
}
