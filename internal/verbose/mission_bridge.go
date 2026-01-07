package verbose

import (
	"context"
	"sync"

	"github.com/zero-day-ai/gibson/internal/mission"
)

// MissionEventBridge subscribes to mission.EventEmitter and forwards events to VerboseEventBus.
// This bridge translates mission lifecycle events into verbose events for unified logging.
type MissionEventBridge struct {
	missionEmitter mission.EventEmitter
	verboseBus     VerboseEventBus
	cleanup        func()
	started        bool
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewMissionEventBridge creates a new bridge between mission events and verbose events.
func NewMissionEventBridge(emitter mission.EventEmitter, bus VerboseEventBus) *MissionEventBridge {
	return &MissionEventBridge{
		missionEmitter: emitter,
		verboseBus:     bus,
		started:        false,
	}
}

// Start begins forwarding mission events to the verbose event bus.
// This spawns a goroutine that listens for mission events and translates them.
// Returns an error if the bridge is already started.
func (b *MissionEventBridge) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return nil // Already started, idempotent
	}

	// Create cancellable context for the bridge
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Subscribe to mission events
	eventChan, cleanup := b.missionEmitter.Subscribe(b.ctx)
	b.cleanup = cleanup
	b.started = true

	// Start forwarding goroutine
	go b.forwardEvents(eventChan)

	return nil
}

// Stop stops the bridge and cleans up the subscription.
func (b *MissionEventBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.started {
		return
	}

	// Cancel context to stop forwarding goroutine
	if b.cancel != nil {
		b.cancel()
	}

	// Clean up mission event subscription
	if b.cleanup != nil {
		b.cleanup()
	}

	b.started = false
}

// forwardEvents listens for mission events and translates them to verbose events.
func (b *MissionEventBridge) forwardEvents(eventChan <-chan mission.MissionEvent) {
	for {
		select {
		case <-b.ctx.Done():
			return
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed, stop forwarding
				return
			}
			b.translateAndEmit(event)
		}
	}
}

// translateAndEmit translates a mission event to a verbose event and emits it.
func (b *MissionEventBridge) translateAndEmit(event mission.MissionEvent) {
	var verboseEvent VerboseEvent

	switch event.Type {
	case mission.EventMissionStarted:
		verboseEvent = b.translateMissionStarted(event)

	case mission.EventMissionProgress:
		verboseEvent = b.translateMissionProgress(event)

	case mission.EventMissionCompleted:
		verboseEvent = b.translateMissionCompleted(event)

	case mission.EventMissionFailed:
		verboseEvent = b.translateMissionFailed(event)

	case mission.EventMissionFinding:
		verboseEvent = b.translateFindingSubmitted(event)

	case mission.EventMissionCheckpoint:
		verboseEvent = b.translateMissionNode(event)

	default:
		// Unknown event type, skip
		return
	}

	// Emit the translated verbose event
	// Use background context to avoid blocking on parent cancellation
	b.verboseBus.Emit(context.Background(), verboseEvent)
}

// translateMissionStarted translates EventMissionStarted to verbose event.
func (b *MissionEventBridge) translateMissionStarted(event mission.MissionEvent) VerboseEvent {
	// Extract MissionStartedData from payload if available
	var nodeCount int
	var workflowName string
	var targetID = event.MissionID // Default to mission ID

	// Check if payload exists and try to extract information
	// The mission started event may not have a typed payload
	if payload, ok := event.Payload.(map[string]interface{}); ok {
		if nc, exists := payload["node_count"]; exists {
			if nodeCountInt, ok := nc.(int); ok {
				nodeCount = nodeCountInt
			}
		}
		if wn, exists := payload["workflow_name"]; exists {
			if workflowNameStr, ok := wn.(string); ok {
				workflowName = workflowNameStr
			}
		}
	}

	return VerboseEvent{
		Type:      EventMissionStarted,
		Level:     LevelVerbose,
		Timestamp: event.Timestamp,
		MissionID: event.MissionID,
		Payload: &MissionStartedData{
			MissionID:    event.MissionID,
			WorkflowName: workflowName,
			TargetID:     targetID,
			NodeCount:    nodeCount,
		},
	}
}

// translateMissionProgress translates EventMissionProgress to verbose event.
func (b *MissionEventBridge) translateMissionProgress(event mission.MissionEvent) VerboseEvent {
	var progressData MissionProgressData

	// Extract progress information from payload
	if payload, ok := event.Payload.(*mission.ProgressPayload); ok {
		if payload.Progress != nil {
			progressData = MissionProgressData{
				MissionID:      event.MissionID,
				CompletedNodes: payload.Progress.CompletedNodes,
				TotalNodes:     payload.Progress.TotalNodes,
				Message:        payload.Message,
			}
			// Set current node from running nodes if available
			if len(payload.Progress.RunningNodes) > 0 {
				progressData.CurrentNode = payload.Progress.RunningNodes[0]
			}
		}
	} else if payload, ok := event.Payload.(map[string]interface{}); ok {
		// Handle map payload (fallback)
		progressData.MissionID = event.MissionID
		if msg, exists := payload["message"]; exists {
			if msgStr, ok := msg.(string); ok {
				progressData.Message = msgStr
			}
		}
	}

	return VerboseEvent{
		Type:      EventMissionProgress,
		Level:     LevelVerbose,
		Timestamp: event.Timestamp,
		MissionID: event.MissionID,
		Payload:   &progressData,
	}
}

// translateMissionCompleted translates EventMissionCompleted to verbose event.
func (b *MissionEventBridge) translateMissionCompleted(event mission.MissionEvent) VerboseEvent {
	var completedData MissionCompletedData
	completedData.MissionID = event.MissionID
	completedData.Success = true

	// Extract result information from payload
	if result, ok := event.Payload.(*mission.MissionResult); ok {
		if result.Metrics != nil {
			completedData.Duration = result.Metrics.Duration
		}
		completedData.FindingCount = len(result.FindingIDs)
		// NodesExecuted would need to come from workflow result
	} else if payload, ok := event.Payload.(map[string]interface{}); ok {
		// Handle map payload (fallback)
		if duration, exists := payload["duration"]; exists {
			if _, ok := duration.(string); ok {
				// Duration is already a string representation
				// We could parse it but for now we'll skip it
			}
		}
	}

	return VerboseEvent{
		Type:      EventMissionCompleted,
		Level:     LevelVerbose,
		Timestamp: event.Timestamp,
		MissionID: event.MissionID,
		Payload:   &completedData,
	}
}

// translateMissionFailed translates EventMissionFailed to verbose event.
func (b *MissionEventBridge) translateMissionFailed(event mission.MissionEvent) VerboseEvent {
	var failedData MissionFailedData
	failedData.MissionID = event.MissionID

	// Extract error information from payload
	if payload, ok := event.Payload.(map[string]interface{}); ok {
		if errMsg, exists := payload["error"]; exists {
			if errStr, ok := errMsg.(string); ok {
				failedData.Error = errStr
			}
		}
	}

	return VerboseEvent{
		Type:      EventMissionFailed,
		Level:     LevelVerbose,
		Timestamp: event.Timestamp,
		MissionID: event.MissionID,
		Payload:   &failedData,
	}
}

// translateFindingSubmitted translates EventMissionFinding to verbose event.
func (b *MissionEventBridge) translateFindingSubmitted(event mission.MissionEvent) VerboseEvent {
	var findingData FindingSubmittedData

	// Extract finding information from payload
	if payload, ok := event.Payload.(*mission.FindingPayload); ok {
		findingData = FindingSubmittedData{
			FindingID: payload.FindingID,
			Title:     payload.Title,
			Severity:  payload.Severity,
			AgentName: payload.AgentName,
		}
	}

	return VerboseEvent{
		Type:      EventFindingSubmitted,
		Level:     LevelVerbose,
		Timestamp: event.Timestamp,
		MissionID: event.MissionID,
		Payload:   &findingData,
	}
}

// translateMissionNode translates EventMissionCheckpoint to verbose event.
func (b *MissionEventBridge) translateMissionNode(event mission.MissionEvent) VerboseEvent {
	var nodeData MissionNodeData

	// Extract checkpoint information from payload
	if payload, ok := event.Payload.(*mission.CheckpointPayload); ok {
		nodeData = MissionNodeData{
			NodeID:   payload.CheckpointID,
			NodeType: "checkpoint",
			Status:   "completed",
		}
	}

	return VerboseEvent{
		Type:      EventMissionNode,
		Level:     LevelVeryVerbose,
		Timestamp: event.Timestamp,
		MissionID: event.MissionID,
		Payload:   &nodeData,
	}
}
