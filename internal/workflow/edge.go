package workflow

// WorkflowEdge represents a directed edge in the workflow DAG
type WorkflowEdge struct {
	// From is the source node ID
	From string `json:"from"`
	// To is the destination node ID
	To string `json:"to"`
	// Condition is an optional condition that must be satisfied for the edge to be traversed
	Condition string `json:"condition,omitempty"`
}
