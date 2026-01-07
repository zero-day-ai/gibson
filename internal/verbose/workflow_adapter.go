package verbose

import (
	"context"
	"time"
)

// WorkflowVerboseAdapter adapts the VerboseEventBus to workflow.VerboseEventBus interface.
// This avoids import cycles between workflow and verbose packages.
//
// The workflow package defines its own minimal VerboseEvent interface (any type)
// to avoid circular dependencies. This adapter converts those events to proper
// VerboseEvent structs.
type WorkflowVerboseAdapter struct {
	bus VerboseEventBus
}

// NewWorkflowVerboseAdapter creates an adapter for workflow integration.
func NewWorkflowVerboseAdapter(bus VerboseEventBus) *WorkflowVerboseAdapter {
	return &WorkflowVerboseAdapter{bus: bus}
}

// Emit implements workflow.VerboseEventBus by converting workflow events to verbose events.
// The workflow package sends events as map[string]interface{} to avoid import cycles.
//
// Expected event structure:
//
//	{
//	  "node_id": "node-123",
//	  "node_type": "agent",
//	  "status": "started|completed|failed|skipped",
//	  "duration": time.Duration,
//	  "error": "error message" (optional)
//	}
func (a *WorkflowVerboseAdapter) Emit(ctx context.Context, event interface{}) error {
	// Type assert to get node event data
	// The workflow package sends a map with node event data
	nodeEvent, ok := event.(map[string]interface{})
	if !ok {
		// If not a map, try to convert to VerboseEvent directly
		if ve, ok := event.(VerboseEvent); ok {
			return a.bus.Emit(ctx, ve)
		}
		// Unknown type, skip silently
		return nil
	}

	// Extract fields from the event map
	nodeID, _ := nodeEvent["node_id"].(string)
	nodeType, _ := nodeEvent["node_type"].(string)
	status, _ := nodeEvent["status"].(string)
	duration, _ := nodeEvent["duration"].(time.Duration)
	errorMsg, _ := nodeEvent["error"].(string)

	// Create verbose event with mission node data
	nodeData := MissionNodeData{
		NodeID:   nodeID,
		NodeType: nodeType,
		Status:   status,
		Duration: duration,
		Error:    errorMsg,
	}

	verboseEvent := VerboseEvent{
		Type:      EventMissionNode,
		Level:     LevelVeryVerbose,
		Timestamp: time.Now(),
		Payload:   &nodeData,
	}

	return a.bus.Emit(ctx, verboseEvent)
}
