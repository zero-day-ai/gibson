package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// StreamManager manages bidirectional gRPC streams to multiple agents.
// It provides thread-safe coordination, event fan-out to subscribers,
// and persistence of all stream events and steering messages.
//
// Key responsibilities:
// - Lifecycle management (connect, disconnect, reconnect)
// - Event subscription for TUI updates
// - Steering message delivery with acknowledgment tracking
// - Session state persistence via SessionDAO
// - OpenTelemetry tracing for all steering operations
//
// Note: Langfuse integration happens automatically through the OTel span export pipeline.
// When a LangfuseExporter is configured as a span exporter in the TracerProvider,
// all spans created by this StreamManager will be automatically exported to Langfuse.
// The session ID is set as a span attribute, which Langfuse uses for grouping.
type StreamManager struct {
	clients     map[string]*StreamClient                // agentName -> client
	subscribers map[string][]chan *database.StreamEvent // agentName -> subscriber channels
	sessionDAO  database.SessionDAO
	tracer      trace.Tracer
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// StreamManagerConfig provides configuration for StreamManager initialization
type StreamManagerConfig struct {
	SessionDAO database.SessionDAO
	Tracer     trace.Tracer
}

// NewStreamManager creates a new StreamManager for coordinating multiple agent streams.
//
// The manager handles:
// - Thread-safe access to multiple agent streams
// - Fan-out of events to multiple subscribers
// - Persistence of events and steering messages
// - OpenTelemetry tracing for observability
//
// Langfuse Integration:
// Langfuse observability is provided automatically through the OTel SDK. When a
// LangfuseExporter is configured in the TracerProvider, all spans created by this
// manager will be exported to Langfuse. The session ID is included as a span attribute
// for proper grouping in the Langfuse UI.
//
// Parameters:
//   - ctx: Parent context for lifecycle management
//   - cfg: Configuration including SessionDAO and tracer
//
// Returns:
//   - *StreamManager: Initialized stream manager ready for use
func NewStreamManager(ctx context.Context, cfg StreamManagerConfig) *StreamManager {
	ctx, cancel := context.WithCancel(ctx)

	return &StreamManager{
		clients:     make(map[string]*StreamClient),
		subscribers: make(map[string][]chan *database.StreamEvent),
		sessionDAO:  cfg.SessionDAO,
		tracer:      cfg.Tracer,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Connect establishes a bidirectional streaming connection to an agent.
// Creates a new StreamClient, starts the stream with the provided task,
// and persists the session to the database.
//
// This method:
// 1. Creates a new session in the database
// 2. Initializes a StreamClient for the agent
// 3. Starts background event processing
// 4. Returns the session ID for tracking
//
// Parameters:
//   - ctx: Request context with timeout/cancellation
//   - agentName: Unique identifier for the agent
//   - conn: gRPC connection to the agent service
//   - taskJSON: JSON-encoded task to execute
//   - mode: Initial agent mode (autonomous or interactive)
//
// Returns:
//   - string: Session ID for tracking
//   - error: Connection or initialization error
func (m *StreamManager) Connect(ctx context.Context, agentName string, conn *grpc.ClientConn, taskJSON string, mode database.AgentMode) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already connected
	if _, exists := m.clients[agentName]; exists {
		return "", fmt.Errorf("agent %s is already connected", agentName)
	}

	// Create session in database
	session := &database.AgentSession{
		ID:        types.NewID(),
		AgentName: agentName,
		Status:    database.AgentStatusRunning,
		Mode:      mode,
		StartedAt: time.Now(),
		Metadata:  json.RawMessage("{}"),
	}

	if err := m.sessionDAO.CreateSession(ctx, session); err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	// Create stream client (panics on error - intentional design decision from StreamClient)
	client := NewStreamClient(m.ctx, conn, agentName, session.ID)

	// Start the stream with the task
	if err := client.Start(taskJSON, mode); err != nil {
		// Clean up session on failure
		session.Status = database.AgentStatusFailed
		now := time.Now()
		session.EndedAt = &now
		_ = m.sessionDAO.UpdateSession(ctx, session)
		_ = client.Close()
		return "", fmt.Errorf("failed to start stream: %w", err)
	}

	// Store client
	m.clients[agentName] = client

	// Start event processing goroutine
	go m.processEvents(agentName, client)

	return session.ID.String(), nil
}

// processEvents handles incoming events from a stream client.
// Runs in a background goroutine, persisting events and fanning out to subscribers.
func (m *StreamManager) processEvents(agentName string, client *StreamClient) {
	eventCh := client.Events()

	for event := range eventCh {
		// Persist event to database
		if err := m.sessionDAO.InsertStreamEvent(m.ctx, event); err != nil {
			// Log error but continue processing
			// In production, consider retry logic or buffering
			continue
		}

		// Fan out to subscribers
		m.mu.RLock()
		subscribers := m.subscribers[agentName]
		m.mu.RUnlock()

		for _, ch := range subscribers {
			// Non-blocking send to avoid slow subscribers blocking the stream
			select {
			case ch <- event:
			default:
				// Subscriber channel full, skip this event
				// In production, consider logging or metrics
			}
		}

		// Handle status changes to update session
		if event.EventType == database.StreamEventStatus {
			m.handleStatusChange(event)
		}
	}

	// Stream closed, clean up
	m.mu.Lock()
	delete(m.clients, agentName)
	m.mu.Unlock()
}

// handleStatusChange updates the session status based on stream events
func (m *StreamManager) handleStatusChange(event *database.StreamEvent) {
	// Parse status from event content
	var statusData struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(event.Content, &statusData); err != nil {
		return
	}

	// Get session
	session, err := m.sessionDAO.GetSession(m.ctx, event.SessionID)
	if err != nil {
		return
	}

	// Map status string to AgentStatus
	switch statusData.Status {
	case "AGENT_STATUS_COMPLETED", "completed":
		session.Status = database.AgentStatusCompleted
		now := time.Now()
		session.EndedAt = &now
	case "AGENT_STATUS_FAILED", "failed":
		session.Status = database.AgentStatusFailed
		now := time.Now()
		session.EndedAt = &now
	case "AGENT_STATUS_PAUSED", "paused":
		session.Status = database.AgentStatusPaused
	case "AGENT_STATUS_INTERRUPTED", "interrupted":
		session.Status = database.AgentStatusInterrupted
	case "AGENT_STATUS_WAITING_FOR_INPUT", "waiting_for_input":
		session.Status = database.AgentStatusWaitingInput
	default:
		session.Status = database.AgentStatusRunning
	}

	_ = m.sessionDAO.UpdateSession(m.ctx, session)
}

// Disconnect closes the stream to an agent and cleans up resources.
// The session is marked as completed in the database.
//
// Parameters:
//   - agentName: Name of the agent to disconnect
//
// Returns:
//   - error: Disconnection error, if any
func (m *StreamManager) Disconnect(agentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, exists := m.clients[agentName]
	if !exists {
		return fmt.Errorf("agent %s is not connected", agentName)
	}

	// Close the stream
	if err := client.Close(); err != nil {
		return fmt.Errorf("failed to close stream: %w", err)
	}

	// Update session status
	session, err := m.sessionDAO.GetSession(m.ctx, client.sessionID)
	if err == nil {
		if session.Status == database.AgentStatusRunning {
			session.Status = database.AgentStatusCompleted
		}
		now := time.Now()
		session.EndedAt = &now
		_ = m.sessionDAO.UpdateSession(m.ctx, session)
	}

	// Remove from clients map (processEvents goroutine will also clean up)
	delete(m.clients, agentName)

	// Clean up subscribers
	delete(m.subscribers, agentName)

	return nil
}

// DisconnectAll closes all active streams and shuts down the manager.
// Called during graceful shutdown.
//
// Returns:
//   - error: First error encountered, if any
func (m *StreamManager) DisconnectAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error

	for _, client := range m.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}

		// Update session status
		session, err := m.sessionDAO.GetSession(m.ctx, client.sessionID)
		if err == nil {
			if session.Status == database.AgentStatusRunning {
				session.Status = database.AgentStatusCompleted
			}
			now := time.Now()
			session.EndedAt = &now
			_ = m.sessionDAO.UpdateSession(m.ctx, session)
		}
	}

	// Clear maps
	m.clients = make(map[string]*StreamClient)
	m.subscribers = make(map[string][]chan *database.StreamEvent)

	// Cancel context
	m.cancel()

	return firstErr
}

// Subscribe creates a new event subscription channel for an agent.
// The caller receives database.StreamEvents as they occur and must call Unsubscribe when done.
//
// Parameters:
//   - agentName: Name of the agent to subscribe to
//
// Returns:
//   - <-chan *database.StreamEvent: Read-only channel for receiving events
func (m *StreamManager) Subscribe(agentName string) <-chan *database.StreamEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create buffered channel to avoid blocking
	ch := make(chan *database.StreamEvent, 100)

	m.subscribers[agentName] = append(m.subscribers[agentName], ch)

	return ch
}

// Unsubscribe removes an event subscription and closes the channel.
//
// Parameters:
//   - agentName: Name of the agent to unsubscribe from
//   - ch: Channel returned by Subscribe
func (m *StreamManager) Unsubscribe(agentName string, ch <-chan *database.StreamEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscribers := m.subscribers[agentName]
	for i, sub := range subscribers {
		if sub == ch {
			// Remove from slice
			m.subscribers[agentName] = append(subscribers[:i], subscribers[i+1:]...)
			// Close channel
			close(sub)
			break
		}
	}
}

// SendSteering sends a steering message to an agent with OpenTelemetry tracing.
// The message is persisted to the database and sent to the agent via the stream.
//
// This creates a span named "gibson.steering.send" with attributes:
// - steering.message.id
// - steering.type
// - agent.name
//
// Parameters:
//   - agentName: Target agent name
//   - content: Free-form guidance text
//   - metadata: Optional metadata map
//
// Returns:
//   - error: Sending error, if any
func (m *StreamManager) SendSteering(agentName string, content string, metadata map[string]string) error {
	ctx, span := m.tracer.Start(m.ctx, "gibson.steering.send")
	defer span.End()

	m.mu.RLock()
	client, exists := m.clients[agentName]
	m.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("agent %s is not connected", agentName)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Create steering message
	msg := &database.SteeringMessage{
		ID:          types.NewID(),
		SessionID:   client.sessionID,
		MessageType: database.SteeringTypeSteer,
		Timestamp:   time.Now(),
	}

	// Serialize content and metadata
	msgData := map[string]any{
		"content":  content,
		"metadata": metadata,
	}
	msgJSON, err := json.Marshal(msgData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal message")
		return fmt.Errorf("failed to marshal steering message: %w", err)
	}
	msg.Content = msgJSON

	// Get trace context
	spanCtx := span.SpanContext()
	msg.TraceID = spanCtx.TraceID().String()

	// Add span attributes
	// The session ID is crucial for Langfuse to group related spans together
	span.SetAttributes(
		attribute.String("steering.message.id", msg.ID.String()),
		attribute.String("steering.type", string(msg.MessageType)),
		attribute.String("agent.name", agentName),
		attribute.String("agent.session.id", client.sessionID.String()),
		attribute.String("session.id", client.sessionID.String()), // For Langfuse grouping
	)

	// Get next sequence number
	sequence, err := m.getNextSequence(client.sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get sequence")
		return fmt.Errorf("failed to get sequence: %w", err)
	}
	msg.Sequence = sequence

	// Persist to database
	if err := m.sessionDAO.InsertSteeringMessage(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist message")
		return fmt.Errorf("failed to persist steering message: %w", err)
	}

	// Send via stream using StreamClient's SendSteering method
	if err := client.SendSteering(content, metadata); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send message")
		return fmt.Errorf("failed to send steering message: %w", err)
	}

	span.SetStatus(codes.Ok, "steering message sent")
	return nil
}

// SendInterrupt sends an interrupt command to an agent.
// This creates a span named "gibson.steering.interrupt".
//
// Parameters:
//   - agentName: Target agent name
//   - reason: Reason for interruption
//
// Returns:
//   - error: Sending error, if any
func (m *StreamManager) SendInterrupt(agentName string, reason string) error {
	ctx, span := m.tracer.Start(m.ctx, "gibson.steering.interrupt")
	defer span.End()

	m.mu.RLock()
	client, exists := m.clients[agentName]
	m.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("agent %s is not connected", agentName)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Create steering message
	msg := &database.SteeringMessage{
		ID:          types.NewID(),
		SessionID:   client.sessionID,
		MessageType: database.SteeringTypeInterrupt,
		Timestamp:   time.Now(),
	}

	// Serialize reason
	msgData := map[string]any{
		"reason": reason,
	}
	msgJSON, err := json.Marshal(msgData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal message")
		return fmt.Errorf("failed to marshal interrupt message: %w", err)
	}
	msg.Content = msgJSON

	// Get trace context
	spanCtx := span.SpanContext()
	msg.TraceID = spanCtx.TraceID().String()

	// Add span attributes
	// The session ID is crucial for Langfuse to group related spans together
	span.SetAttributes(
		attribute.String("steering.message.id", msg.ID.String()),
		attribute.String("steering.type", string(msg.MessageType)),
		attribute.String("agent.name", agentName),
		attribute.String("agent.session.id", client.sessionID.String()),
		attribute.String("session.id", client.sessionID.String()), // For Langfuse grouping
	)

	// Get next sequence number
	sequence, err := m.getNextSequence(client.sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get sequence")
		return fmt.Errorf("failed to get sequence: %w", err)
	}
	msg.Sequence = sequence

	// Persist to database
	if err := m.sessionDAO.InsertSteeringMessage(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist message")
		return fmt.Errorf("failed to persist interrupt message: %w", err)
	}

	// Send via stream
	if err := client.SendInterrupt(reason); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send interrupt")
		return fmt.Errorf("failed to send interrupt: %w", err)
	}

	span.SetStatus(codes.Ok, "interrupt sent")
	return nil
}

// SetMode changes the operational mode of an agent.
// This creates a span named "gibson.steering.set_mode".
//
// Parameters:
//   - agentName: Target agent name
//   - mode: New agent mode (autonomous or interactive)
//
// Returns:
//   - error: Operation error, if any
func (m *StreamManager) SetMode(agentName string, mode database.AgentMode) error {
	ctx, span := m.tracer.Start(m.ctx, "gibson.steering.set_mode")
	defer span.End()

	m.mu.RLock()
	client, exists := m.clients[agentName]
	m.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("agent %s is not connected", agentName)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Create steering message
	msg := &database.SteeringMessage{
		ID:          types.NewID(),
		SessionID:   client.sessionID,
		MessageType: database.SteeringTypeSetMode,
		Timestamp:   time.Now(),
	}

	// Serialize mode
	msgData := map[string]any{
		"mode": string(mode),
	}
	msgJSON, err := json.Marshal(msgData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal message")
		return fmt.Errorf("failed to marshal set_mode message: %w", err)
	}
	msg.Content = msgJSON

	// Get trace context
	spanCtx := span.SpanContext()
	msg.TraceID = spanCtx.TraceID().String()

	// Add span attributes
	// The session ID is crucial for Langfuse to group related spans together
	span.SetAttributes(
		attribute.String("steering.message.id", msg.ID.String()),
		attribute.String("steering.type", string(msg.MessageType)),
		attribute.String("agent.name", agentName),
		attribute.String("agent.session.id", client.sessionID.String()),
		attribute.String("session.id", client.sessionID.String()), // For Langfuse grouping
		attribute.String("agent.new_mode", string(mode)),
	)

	// Get next sequence number
	sequence, err := m.getNextSequence(client.sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get sequence")
		return fmt.Errorf("failed to get sequence: %w", err)
	}
	msg.Sequence = sequence

	// Persist to database
	if err := m.sessionDAO.InsertSteeringMessage(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist message")
		return fmt.Errorf("failed to persist set_mode message: %w", err)
	}

	// Send via stream (StreamClient already uses database.AgentMode)
	if err := client.SetMode(mode); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set mode")
		return fmt.Errorf("failed to set mode: %w", err)
	}

	// Update session mode
	session, err := m.sessionDAO.GetSession(ctx, client.sessionID)
	if err == nil {
		session.Mode = mode
		_ = m.sessionDAO.UpdateSession(ctx, session)
	}

	span.SetStatus(codes.Ok, "mode updated")
	return nil
}

// Resume sends a resume command to an interrupted agent.
// This creates a span named "gibson.steering.resume".
//
// Parameters:
//   - agentName: Target agent name
//   - guidance: Optional guidance text before resuming
//
// Returns:
//   - error: Operation error, if any
func (m *StreamManager) Resume(agentName string, guidance string) error {
	ctx, span := m.tracer.Start(m.ctx, "gibson.steering.resume")
	defer span.End()

	m.mu.RLock()
	client, exists := m.clients[agentName]
	m.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("agent %s is not connected", agentName)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Create steering message
	msg := &database.SteeringMessage{
		ID:          types.NewID(),
		SessionID:   client.sessionID,
		MessageType: database.SteeringTypeResume,
		Timestamp:   time.Now(),
	}

	// Serialize guidance
	msgData := map[string]any{
		"guidance": guidance,
	}
	msgJSON, err := json.Marshal(msgData)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal message")
		return fmt.Errorf("failed to marshal resume message: %w", err)
	}
	msg.Content = msgJSON

	// Get trace context
	spanCtx := span.SpanContext()
	msg.TraceID = spanCtx.TraceID().String()

	// Add span attributes
	// The session ID is crucial for Langfuse to group related spans together
	span.SetAttributes(
		attribute.String("steering.message.id", msg.ID.String()),
		attribute.String("steering.type", string(msg.MessageType)),
		attribute.String("agent.name", agentName),
		attribute.String("agent.session.id", client.sessionID.String()),
		attribute.String("session.id", client.sessionID.String()), // For Langfuse grouping
	)

	// Get next sequence number
	sequence, err := m.getNextSequence(client.sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get sequence")
		return fmt.Errorf("failed to get sequence: %w", err)
	}
	msg.Sequence = sequence

	// Persist to database
	if err := m.sessionDAO.InsertSteeringMessage(ctx, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to persist message")
		return fmt.Errorf("failed to persist resume message: %w", err)
	}

	// Send via stream
	if err := client.Resume(guidance); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to resume")
		return fmt.Errorf("failed to resume: %w", err)
	}

	span.SetStatus(codes.Ok, "resume sent")
	return nil
}

// GetSession retrieves the current session for an agent.
//
// Parameters:
//   - agentName: Name of the agent
//
// Returns:
//   - *database.AgentSession: Current session, if found
//   - error: Lookup error, if any
func (m *StreamManager) GetSession(agentName string) (*database.AgentSession, error) {
	m.mu.RLock()
	client, exists := m.clients[agentName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent %s is not connected", agentName)
	}

	return m.sessionDAO.GetSession(m.ctx, client.sessionID)
}

// ListActiveSessions returns all currently active agent sessions.
// An active session is one that has a connected StreamClient.
//
// Returns:
//   - []database.AgentSession: List of active sessions
func (m *StreamManager) ListActiveSessions() []database.AgentSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]database.AgentSession, 0, len(m.clients))

	for _, client := range m.clients {
		session, err := m.sessionDAO.GetSession(m.ctx, client.sessionID)
		if err == nil {
			sessions = append(sessions, *session)
		}
	}

	return sessions
}

// getNextSequence retrieves the next sequence number for a session.
// This is used to maintain ordered steering messages.
func (m *StreamManager) getNextSequence(sessionID types.ID) (int64, error) {
	messages, err := m.sessionDAO.GetSteeringMessages(m.ctx, sessionID)
	if err != nil {
		return 0, err
	}

	if len(messages) == 0 {
		return 1, nil
	}

	// Find max sequence
	maxSeq := int64(0)
	for _, msg := range messages {
		if msg.Sequence > maxSeq {
			maxSeq = msg.Sequence
		}
	}

	return maxSeq + 1, nil
}
