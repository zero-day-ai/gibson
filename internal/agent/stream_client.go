package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc"

	proto "github.com/zero-day-ai/sdk/api/gen/proto"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// StreamClient manages a bidirectional gRPC stream to a single agent.
// It handles concurrent send/receive operations with proper synchronization
// and graceful shutdown without goroutine leaks.
type StreamClient struct {
	conn       *grpc.ClientConn
	stream     grpc.BidiStreamingClient[proto.ClientMessage, proto.AgentMessage]
	agentName  string
	sessionID  types.ID
	eventCh    chan *database.StreamEvent
	steeringCh chan *proto.ClientMessage
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	closed     bool
	wg         sync.WaitGroup
	recvErr    error // Last error from receive goroutine
}

// NewStreamClient creates a new stream client for the given agent.
// The returned client is ready to start streaming after calling Start().
// Note: This function panics if the gRPC stream cannot be created.
// This is intentional as stream creation failure is a programming error.
func NewStreamClient(ctx context.Context, conn *grpc.ClientConn, agentName string, sessionID types.ID) *StreamClient {
	clientCtx, cancel := context.WithCancel(ctx)

	// Create the bidirectional stream
	client := proto.NewAgentServiceClient(conn)
	stream, err := client.StreamExecute(clientCtx)
	if err != nil {
		cancel()
		panic(fmt.Sprintf("failed to create stream: %v", err))
	}

	c := &StreamClient{
		conn:       conn,
		stream:     stream,
		agentName:  agentName,
		sessionID:  sessionID,
		eventCh:    make(chan *database.StreamEvent, 100), // Buffered to prevent blocking
		steeringCh: make(chan *proto.ClientMessage, 10),
		ctx:        clientCtx,
		cancel:     cancel,
		closed:     false,
	}

	// Start the send and receive goroutines
	c.wg.Add(2)
	go c.sendLoop()
	go c.recvLoop()

	return c
}

// Start initiates agent execution with the given task and mode.
// This sends the initial StartExecutionRequest to the agent.
func (c *StreamClient) Start(taskJSON string, mode database.AgentMode) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("stream client is closed")
	}
	c.mu.Unlock()

	protoMode := proto.AgentMode_AGENT_MODE_AUTONOMOUS
	if mode == database.AgentModeInteractive {
		protoMode = proto.AgentMode_AGENT_MODE_INTERACTIVE
	}

	msg := &proto.ClientMessage{
		Payload: &proto.ClientMessage_Start{
			Start: &proto.StartExecutionRequest{
				TaskJson:    taskJSON,
				InitialMode: protoMode,
			},
		},
	}

	select {
	case c.steeringCh <- msg:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("stream closed: %w", c.ctx.Err())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending start request")
	}
}

// Events returns a read-only channel that receives stream events.
// The channel is closed when the stream terminates.
func (c *StreamClient) Events() <-chan *database.StreamEvent {
	return c.eventCh
}

// SendSteering sends a steering message to the agent.
// This method is thread-safe and can be called concurrently.
func (c *StreamClient) SendSteering(content string, metadata map[string]string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("stream client is closed")
	}
	c.mu.Unlock()

	clientMsg := &proto.ClientMessage{
		Payload: &proto.ClientMessage_Steering{
			Steering: &proto.SteeringMessage{
				Id:       types.NewID().String(),
				Content:  content,
				Metadata: metadata,
			},
		},
	}

	select {
	case c.steeringCh <- clientMsg:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("stream closed: %w", c.ctx.Err())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending steering message")
	}
}

// SendInterrupt sends an interrupt request to the agent.
func (c *StreamClient) SendInterrupt(reason string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("stream client is closed")
	}
	c.mu.Unlock()

	msg := &proto.ClientMessage{
		Payload: &proto.ClientMessage_Interrupt{
			Interrupt: &proto.InterruptRequest{
				Reason: reason,
			},
		},
	}

	select {
	case c.steeringCh <- msg:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("stream closed: %w", c.ctx.Err())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending interrupt")
	}
}

// SetMode changes the agent's execution mode.
func (c *StreamClient) SetMode(mode database.AgentMode) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("stream client is closed")
	}
	c.mu.Unlock()

	protoMode := proto.AgentMode_AGENT_MODE_AUTONOMOUS
	if mode == database.AgentModeInteractive {
		protoMode = proto.AgentMode_AGENT_MODE_INTERACTIVE
	}

	msg := &proto.ClientMessage{
		Payload: &proto.ClientMessage_SetMode{
			SetMode: &proto.SetModeRequest{
				Mode: protoMode,
			},
		},
	}

	select {
	case c.steeringCh <- msg:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("stream closed: %w", c.ctx.Err())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending set mode")
	}
}

// Resume sends a resume request to the agent with optional guidance.
func (c *StreamClient) Resume(guidance string) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("stream client is closed")
	}
	c.mu.Unlock()

	msg := &proto.ClientMessage{
		Payload: &proto.ClientMessage_Resume{
			Resume: &proto.ResumeRequest{
				Guidance: guidance,
			},
		},
	}

	select {
	case c.steeringCh <- msg:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("stream closed: %w", c.ctx.Err())
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending resume")
	}
}

// Close gracefully closes the stream and waits for goroutines to exit.
// This method is idempotent and safe to call multiple times.
func (c *StreamClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Cancel context to signal goroutines to exit
	c.cancel()

	// Close the steering channel to unblock sendLoop
	close(c.steeringCh)

	// Wait for both goroutines to exit
	c.wg.Wait()

	// Close the event channel
	close(c.eventCh)

	return c.recvErr
}

// sendLoop runs in a goroutine and sends messages to the agent.
// It exits when the steering channel is closed or context is cancelled.
func (c *StreamClient) sendLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			// Context cancelled, close send side of stream
			_ = c.stream.CloseSend()
			return

		case msg, ok := <-c.steeringCh:
			if !ok {
				// Channel closed, close send side of stream
				_ = c.stream.CloseSend()
				return
			}

			// Send the message to the agent
			if err := c.stream.Send(msg); err != nil {
				// Send failed, emit error event and exit
				c.emitErrorEvent(fmt.Errorf("failed to send message: %w", err))
				return
			}
		}
	}
}

// recvLoop runs in a goroutine and receives messages from the agent.
// It exits when the stream is closed or an error occurs.
func (c *StreamClient) recvLoop() {
	defer c.wg.Done()

	for {
		msg, err := c.stream.Recv()
		if err != nil {
			if err == io.EOF {
				// Stream closed normally
				c.recvErr = nil
			} else {
				// Stream closed with error
				c.recvErr = err
				c.emitErrorEvent(fmt.Errorf("stream receive error: %w", err))
			}
			return
		}

		// Convert proto message to StreamEvent
		event, err := c.protoToEvent(msg)
		if err != nil {
			c.emitErrorEvent(fmt.Errorf("failed to convert proto message: %w", err))
			continue
		}

		// Send event to consumers
		select {
		case c.eventCh <- event:
		case <-c.ctx.Done():
			return
		}
	}
}

// protoToEvent converts a proto AgentMessage to a database.StreamEvent
func (c *StreamClient) protoToEvent(msg *proto.AgentMessage) (*database.StreamEvent, error) {
	event := &database.StreamEvent{
		ID:        types.NewID(),
		SessionID: c.sessionID,
		Sequence:  msg.Sequence,
		TraceID:   msg.TraceId,
		SpanID:    msg.SpanId,
		Timestamp: time.UnixMilli(msg.TimestampMs),
	}

	// Handle the payload based on type
	switch payload := msg.Payload.(type) {
	case *proto.AgentMessage_Output:
		event.EventType = database.StreamEventOutput
		content, err := json.Marshal(map[string]any{
			"content":      payload.Output.Content,
			"is_reasoning": payload.Output.IsReasoning,
		})
		if err != nil {
			return event, fmt.Errorf("failed to marshal output: %w", err)
		}
		event.Content = content

	case *proto.AgentMessage_ToolCall:
		event.EventType = database.StreamEventToolCall
		content, err := json.Marshal(map[string]any{
			"tool_name":  payload.ToolCall.ToolName,
			"input_json": payload.ToolCall.InputJson,
			"call_id":    payload.ToolCall.CallId,
		})
		if err != nil {
			return event, fmt.Errorf("failed to marshal tool call: %w", err)
		}
		event.Content = content

	case *proto.AgentMessage_ToolResult:
		event.EventType = database.StreamEventToolResult
		content, err := json.Marshal(map[string]any{
			"call_id":     payload.ToolResult.CallId,
			"output_json": payload.ToolResult.OutputJson,
			"success":     payload.ToolResult.Success,
		})
		if err != nil {
			return event, fmt.Errorf("failed to marshal tool result: %w", err)
		}
		event.Content = content

	case *proto.AgentMessage_Finding:
		event.EventType = database.StreamEventFinding
		event.Content = json.RawMessage(payload.Finding.FindingJson)

	case *proto.AgentMessage_Status:
		event.EventType = database.StreamEventStatus
		content, err := json.Marshal(map[string]any{
			"status":  payload.Status.Status.String(),
			"message": payload.Status.Message,
		})
		if err != nil {
			return event, fmt.Errorf("failed to marshal status: %w", err)
		}
		event.Content = content

	case *proto.AgentMessage_SteeringAck:
		event.EventType = database.StreamEventSteeringAck
		content, err := json.Marshal(map[string]any{
			"message_id": payload.SteeringAck.MessageId,
			"response":   payload.SteeringAck.Response,
		})
		if err != nil {
			return event, fmt.Errorf("failed to marshal steering ack: %w", err)
		}
		event.Content = content

	case *proto.AgentMessage_Error:
		event.EventType = database.StreamEventError
		content, err := json.Marshal(map[string]any{
			"code":    payload.Error.Code,
			"message": payload.Error.Message,
			"fatal":   payload.Error.Fatal,
		})
		if err != nil {
			return event, fmt.Errorf("failed to marshal error: %w", err)
		}
		event.Content = content

	default:
		return event, fmt.Errorf("unknown message payload type: %T", payload)
	}

	return event, nil
}

// emitErrorEvent emits an error event to the event channel.
// This is used for internal errors that should be surfaced to consumers.
func (c *StreamClient) emitErrorEvent(err error) {
	content, _ := json.Marshal(map[string]any{
		"code":    "INTERNAL_ERROR",
		"message": err.Error(),
		"fatal":   false,
	})

	event := &database.StreamEvent{
		ID:        types.NewID(),
		SessionID: c.sessionID,
		EventType: database.StreamEventError,
		Content:   content,
		Timestamp: time.Now(),
	}

	select {
	case c.eventCh <- event:
	case <-c.ctx.Done():
	default:
		// Channel full, drop the event to avoid blocking
	}
}
