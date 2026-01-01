package console

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/zero-day-ai/gibson/internal/database"
)

// EventProcessor runs a background goroutine that reads from a StreamManager
// subscription channel and sends events to the TUI via tea.Program.Send().
// This bridges the gRPC streaming infrastructure with Bubble Tea's message loop.
type EventProcessor struct {
	// eventChan receives events from StreamManager subscription
	eventChan <-chan *database.StreamEvent
	// program is the Bubble Tea program to send messages to
	program *tea.Program
	// stopChan signals the processor to stop
	stopChan chan struct{}
	// agentName identifies which agent we're processing events for
	agentName string
	// running tracks whether the processor is actively processing
	running bool
}

// NewEventProcessor creates a new EventProcessor that will read events
// from the given channel and send them to the program.
func NewEventProcessor(eventChan <-chan *database.StreamEvent, program *tea.Program, agentName string) *EventProcessor {
	return &EventProcessor{
		eventChan: eventChan,
		program:   program,
		stopChan:  make(chan struct{}),
		agentName: agentName,
		running:   false,
	}
}

// Start begins processing events in a background goroutine.
// Events are read from the subscription channel and sent to the TUI.
// The goroutine runs until Stop() is called or the event channel is closed.
func (ep *EventProcessor) Start() {
	if ep.running {
		return
	}
	ep.running = true

	go ep.processLoop()
}

// Stop signals the processor to stop and waits for cleanup.
// It's safe to call Stop() multiple times.
func (ep *EventProcessor) Stop() {
	if !ep.running {
		return
	}

	// Signal stop - use select to avoid blocking if already closed
	select {
	case <-ep.stopChan:
		// Already closed
	default:
		close(ep.stopChan)
	}

	ep.running = false
}

// IsRunning returns whether the processor is currently active.
func (ep *EventProcessor) IsRunning() bool {
	return ep.running
}

// AgentName returns the name of the agent being processed.
func (ep *EventProcessor) AgentName() string {
	return ep.agentName
}

// processLoop is the main event processing loop that runs in a goroutine.
func (ep *EventProcessor) processLoop() {
	defer func() {
		ep.running = false
	}()

	for {
		select {
		case <-ep.stopChan:
			// Graceful shutdown requested
			return

		case event, ok := <-ep.eventChan:
			if !ok {
				// Channel closed - agent disconnected
				ep.sendDisconnected("stream closed")
				return
			}

			// Send event to TUI
			ep.sendEvent(event)
		}
	}
}

// sendEvent wraps the StreamEvent in an AgentEventMsg and sends to the program.
func (ep *EventProcessor) sendEvent(event *database.StreamEvent) {
	if ep.program == nil {
		return
	}

	msg := AgentEventMsg{
		AgentName: ep.agentName,
		Event:     event,
	}

	ep.program.Send(msg)
}

// sendDisconnected sends an AgentDisconnectedMsg to notify the TUI.
func (ep *EventProcessor) sendDisconnected(reason string) {
	if ep.program == nil {
		return
	}

	msg := AgentDisconnectedMsg{
		AgentName: ep.agentName,
		Reason:    reason,
	}

	ep.program.Send(msg)
}
