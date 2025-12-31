package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// AgentStatus represents the current status of an agent session
type AgentStatus string

const (
	// AgentStatusRunning indicates the agent is currently executing
	AgentStatusRunning AgentStatus = "running"
	// AgentStatusPaused indicates the agent is paused
	AgentStatusPaused AgentStatus = "paused"
	// AgentStatusWaitingInput indicates the agent is waiting for operator input
	AgentStatusWaitingInput AgentStatus = "waiting_for_input"
	// AgentStatusInterrupted indicates the agent has been interrupted
	AgentStatusInterrupted AgentStatus = "interrupted"
	// AgentStatusCompleted indicates the agent has completed successfully
	AgentStatusCompleted AgentStatus = "completed"
	// AgentStatusFailed indicates the agent execution has failed
	AgentStatusFailed AgentStatus = "failed"
)

// AgentMode represents the operational mode of an agent
type AgentMode string

const (
	// AgentModeAutonomous indicates the agent operates autonomously
	AgentModeAutonomous AgentMode = "autonomous"
	// AgentModeInteractive indicates the agent requires operator interaction
	AgentModeInteractive AgentMode = "interactive"
)

// AgentSession represents a persisted agent execution session
type AgentSession struct {
	ID        types.ID        `db:"id" json:"id"`
	MissionID types.ID        `db:"mission_id" json:"mission_id"`
	AgentName string          `db:"agent_name" json:"agent_name"`
	Status    AgentStatus     `db:"status" json:"status"`
	Mode      AgentMode       `db:"mode" json:"mode"`
	StartedAt time.Time       `db:"started_at" json:"started_at"`
	EndedAt   *time.Time      `db:"ended_at" json:"ended_at,omitempty"`
	Metadata  json.RawMessage `db:"metadata" json:"metadata,omitempty"`
}

// StreamEventType represents the type of a stream event
type StreamEventType string

const (
	// StreamEventOutput represents agent output/reasoning
	StreamEventOutput StreamEventType = "output"
	// StreamEventToolCall represents a tool invocation
	StreamEventToolCall StreamEventType = "tool_call"
	// StreamEventToolResult represents a tool execution result
	StreamEventToolResult StreamEventType = "tool_result"
	// StreamEventFinding represents a security finding
	StreamEventFinding StreamEventType = "finding"
	// StreamEventStatus represents a status change
	StreamEventStatus StreamEventType = "status"
	// StreamEventSteeringAck represents acknowledgment of a steering message
	StreamEventSteeringAck StreamEventType = "steering_ack"
	// StreamEventError represents an error event
	StreamEventError StreamEventType = "error"
)

// StreamEvent represents a single event in the agent stream
type StreamEvent struct {
	ID        types.ID        `db:"id" json:"id"`
	SessionID types.ID        `db:"session_id" json:"session_id"`
	Sequence  int64           `db:"sequence" json:"sequence"`
	EventType StreamEventType `db:"event_type" json:"event_type"`
	Content   json.RawMessage `db:"content" json:"content"`
	Timestamp time.Time       `db:"timestamp" json:"timestamp"`
	TraceID   string          `db:"trace_id" json:"trace_id,omitempty"`
	SpanID    string          `db:"span_id" json:"span_id,omitempty"`
}

// SteeringType represents the type of steering message
type SteeringType string

const (
	// SteeringTypeSteer represents a guidance message
	SteeringTypeSteer SteeringType = "steer"
	// SteeringTypeInterrupt represents an interrupt command
	SteeringTypeInterrupt SteeringType = "interrupt"
	// SteeringTypeResume represents a resume command
	SteeringTypeResume SteeringType = "resume"
	// SteeringTypeCancel represents a cancellation command
	SteeringTypeCancel SteeringType = "cancel"
	// SteeringTypeSetMode represents a mode change command
	SteeringTypeSetMode SteeringType = "set_mode"
)

// SteeringMessage represents an operator steering message
type SteeringMessage struct {
	ID             types.ID        `db:"id" json:"id"`
	SessionID      types.ID        `db:"session_id" json:"session_id"`
	Sequence       int64           `db:"sequence" json:"sequence"`
	OperatorID     string          `db:"operator_id" json:"operator_id"`
	MessageType    SteeringType    `db:"message_type" json:"message_type"`
	Content        json.RawMessage `db:"content" json:"content"`
	Timestamp      time.Time       `db:"timestamp" json:"timestamp"`
	AcknowledgedAt *time.Time      `db:"acknowledged_at" json:"acknowledged_at,omitempty"`
	TraceID        string          `db:"trace_id" json:"trace_id,omitempty"`
}

// StreamEventFilter provides filtering options for stream events
type StreamEventFilter struct {
	EventTypes []StreamEventType
	FromSeq    int64
	ToSeq      int64
	FromTime   time.Time
	ToTime     time.Time
	Limit      int
}

// SessionDAO provides database operations for agent sessions and related entities
type SessionDAO interface {
	// Sessions
	CreateSession(ctx context.Context, session *AgentSession) error
	UpdateSession(ctx context.Context, session *AgentSession) error
	GetSession(ctx context.Context, id types.ID) (*AgentSession, error)
	ListSessionsByMission(ctx context.Context, missionID types.ID) ([]AgentSession, error)

	// Stream events
	InsertStreamEvent(ctx context.Context, event *StreamEvent) error
	InsertStreamEventBatch(ctx context.Context, events []StreamEvent) error
	GetStreamEvents(ctx context.Context, sessionID types.ID, filter StreamEventFilter) ([]StreamEvent, error)

	// Steering messages
	InsertSteeringMessage(ctx context.Context, msg *SteeringMessage) error
	AcknowledgeSteeringMessage(ctx context.Context, id types.ID) error
	GetSteeringMessages(ctx context.Context, sessionID types.ID) ([]SteeringMessage, error)
}

// sessionDAO implements SessionDAO
type sessionDAO struct {
	db *DB
}

// NewSessionDAO creates a new SessionDAO instance
func NewSessionDAO(db *DB) SessionDAO {
	return &sessionDAO{db: db}
}

// CreateSession creates a new agent session
func (d *sessionDAO) CreateSession(ctx context.Context, session *AgentSession) error {
	if session.ID.IsZero() {
		session.ID = types.NewID()
	}

	// Set default timestamps
	if session.StartedAt.IsZero() {
		session.StartedAt = time.Now()
	}

	// Serialize metadata to JSON
	var metadataJSON []byte
	var err error
	if len(session.Metadata) > 0 {
		metadataJSON = session.Metadata
	} else {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO agent_sessions (
			id, mission_id, agent_name, status, mode,
			started_at, ended_at, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = d.db.ExecContext(
		ctx, query,
		session.ID.String(),
		session.MissionID.String(),
		session.AgentName,
		session.Status,
		session.Mode,
		session.StartedAt,
		nullableTime(session.EndedAt),
		string(metadataJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to create agent session: %w", err)
	}

	return nil
}

// UpdateSession updates an existing agent session
func (d *sessionDAO) UpdateSession(ctx context.Context, session *AgentSession) error {
	// Serialize metadata to JSON
	var metadataJSON []byte
	if len(session.Metadata) > 0 {
		metadataJSON = session.Metadata
	} else {
		metadataJSON = []byte("{}")
	}

	query := `
		UPDATE agent_sessions
		SET status = ?, mode = ?, ended_at = ?, metadata = ?
		WHERE id = ?
	`

	result, err := d.db.ExecContext(
		ctx, query,
		session.Status,
		session.Mode,
		nullableTime(session.EndedAt),
		string(metadataJSON),
		session.ID.String(),
	)

	if err != nil {
		return fmt.Errorf("failed to update agent session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("agent session not found: %s", session.ID)
	}

	return nil
}

// GetSession retrieves an agent session by ID
func (d *sessionDAO) GetSession(ctx context.Context, id types.ID) (*AgentSession, error) {
	query := `
		SELECT
			id, mission_id, agent_name, status, mode,
			started_at, ended_at, metadata
		FROM agent_sessions
		WHERE id = ?
	`

	var session AgentSession
	var idStr, missionIDStr string
	var metadataJSON sql.NullString
	var endedAt sql.NullTime

	err := d.db.QueryRowContext(ctx, query, id.String()).Scan(
		&idStr,
		&missionIDStr,
		&session.AgentName,
		&session.Status,
		&session.Mode,
		&session.StartedAt,
		&endedAt,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent session not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent session: %w", err)
	}

	// Parse IDs
	session.ID, err = types.ParseID(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse session ID: %w", err)
	}

	session.MissionID, err = types.ParseID(missionIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mission ID: %w", err)
	}

	// Handle nullable fields
	if endedAt.Valid {
		session.EndedAt = &endedAt.Time
	}

	if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "{}" {
		session.Metadata = json.RawMessage(metadataJSON.String)
	}

	return &session, nil
}

// ListSessionsByMission retrieves all sessions for a given mission
func (d *sessionDAO) ListSessionsByMission(ctx context.Context, missionID types.ID) ([]AgentSession, error) {
	query := `
		SELECT
			id, mission_id, agent_name, status, mode,
			started_at, ended_at, metadata
		FROM agent_sessions
		WHERE mission_id = ?
		ORDER BY started_at DESC
	`

	rows, err := d.db.QueryContext(ctx, query, missionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list agent sessions: %w", err)
	}
	defer rows.Close()

	var sessions []AgentSession
	for rows.Next() {
		var session AgentSession
		var idStr, missionIDStr string
		var metadataJSON sql.NullString
		var endedAt sql.NullTime

		err := rows.Scan(
			&idStr,
			&missionIDStr,
			&session.AgentName,
			&session.Status,
			&session.Mode,
			&session.StartedAt,
			&endedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent session: %w", err)
		}

		// Parse IDs
		session.ID, err = types.ParseID(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse session ID: %w", err)
		}

		session.MissionID, err = types.ParseID(missionIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse mission ID: %w", err)
		}

		// Handle nullable fields
		if endedAt.Valid {
			session.EndedAt = &endedAt.Time
		}

		if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "{}" {
			session.Metadata = json.RawMessage(metadataJSON.String)
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating agent sessions: %w", err)
	}

	return sessions, nil
}

// InsertStreamEvent inserts a single stream event
func (d *sessionDAO) InsertStreamEvent(ctx context.Context, event *StreamEvent) error {
	if event.ID.IsZero() {
		event.ID = types.NewID()
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Serialize content to JSON
	var contentJSON []byte
	if len(event.Content) > 0 {
		contentJSON = event.Content
	} else {
		contentJSON = []byte("{}")
	}

	query := `
		INSERT INTO stream_events (
			id, session_id, sequence, event_type, content,
			timestamp, trace_id, span_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.ExecContext(
		ctx, query,
		event.ID.String(),
		event.SessionID.String(),
		event.Sequence,
		event.EventType,
		string(contentJSON),
		event.Timestamp,
		event.TraceID,
		event.SpanID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert stream event: %w", err)
	}

	return nil
}

// InsertStreamEventBatch inserts multiple stream events in a single transaction
func (d *sessionDAO) InsertStreamEventBatch(ctx context.Context, events []StreamEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Use transaction for batch insert
	return d.db.WithTx(ctx, func(tx *sql.Tx) error {
		query := `
			INSERT INTO stream_events (
				id, session_id, sequence, event_type, content,
				timestamp, trace_id, span_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`

		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for i := range events {
			event := &events[i]

			if event.ID.IsZero() {
				event.ID = types.NewID()
			}

			if event.Timestamp.IsZero() {
				event.Timestamp = time.Now()
			}

			// Serialize content to JSON
			var contentJSON []byte
			if len(event.Content) > 0 {
				contentJSON = event.Content
			} else {
				contentJSON = []byte("{}")
			}

			_, err := stmt.ExecContext(
				ctx,
				event.ID.String(),
				event.SessionID.String(),
				event.Sequence,
				event.EventType,
				string(contentJSON),
				event.Timestamp,
				event.TraceID,
				event.SpanID,
			)

			if err != nil {
				return fmt.Errorf("failed to insert stream event %d: %w", i, err)
			}
		}

		return nil
	})
}

// GetStreamEvents retrieves stream events for a session with optional filtering
func (d *sessionDAO) GetStreamEvents(ctx context.Context, sessionID types.ID, filter StreamEventFilter) ([]StreamEvent, error) {
	query := `
		SELECT
			id, session_id, sequence, event_type, content,
			timestamp, trace_id, span_id
		FROM stream_events
		WHERE session_id = ?
	`

	var args []interface{}
	args = append(args, sessionID.String())

	// Apply filters
	if len(filter.EventTypes) > 0 {
		placeholders := make([]string, len(filter.EventTypes))
		for i, eventType := range filter.EventTypes {
			placeholders[i] = "?"
			args = append(args, eventType)
		}
		query += fmt.Sprintf(" AND event_type IN (%s)", strings.Join(placeholders, ","))
	}

	if filter.FromSeq > 0 {
		query += " AND sequence >= ?"
		args = append(args, filter.FromSeq)
	}

	if filter.ToSeq > 0 {
		query += " AND sequence <= ?"
		args = append(args, filter.ToSeq)
	}

	if !filter.FromTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filter.FromTime)
	}

	if !filter.ToTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.ToTime)
	}

	// Order by sequence
	query += " ORDER BY sequence ASC"

	// Apply limit
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query stream events: %w", err)
	}
	defer rows.Close()

	var events []StreamEvent
	for rows.Next() {
		var event StreamEvent
		var idStr, sessionIDStr string
		var contentJSON sql.NullString
		var traceID, spanID sql.NullString

		err := rows.Scan(
			&idStr,
			&sessionIDStr,
			&event.Sequence,
			&event.EventType,
			&contentJSON,
			&event.Timestamp,
			&traceID,
			&spanID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stream event: %w", err)
		}

		// Parse IDs
		event.ID, err = types.ParseID(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event ID: %w", err)
		}

		event.SessionID, err = types.ParseID(sessionIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse session ID: %w", err)
		}

		// Handle nullable fields
		if contentJSON.Valid && contentJSON.String != "" && contentJSON.String != "{}" {
			event.Content = json.RawMessage(contentJSON.String)
		}

		if traceID.Valid {
			event.TraceID = traceID.String
		}

		if spanID.Valid {
			event.SpanID = spanID.String
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stream events: %w", err)
	}

	return events, nil
}

// InsertSteeringMessage inserts a steering message
func (d *sessionDAO) InsertSteeringMessage(ctx context.Context, msg *SteeringMessage) error {
	if msg.ID.IsZero() {
		msg.ID = types.NewID()
	}

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Serialize content to JSON
	var contentJSON []byte
	if len(msg.Content) > 0 {
		contentJSON = msg.Content
	} else {
		contentJSON = []byte("{}")
	}

	query := `
		INSERT INTO steering_messages (
			id, session_id, sequence, operator_id, message_type,
			content, timestamp, acknowledged_at, trace_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.ExecContext(
		ctx, query,
		msg.ID.String(),
		msg.SessionID.String(),
		msg.Sequence,
		msg.OperatorID,
		msg.MessageType,
		string(contentJSON),
		msg.Timestamp,
		nullableTime(msg.AcknowledgedAt),
		msg.TraceID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert steering message: %w", err)
	}

	return nil
}

// AcknowledgeSteeringMessage marks a steering message as acknowledged
func (d *sessionDAO) AcknowledgeSteeringMessage(ctx context.Context, id types.ID) error {
	query := `
		UPDATE steering_messages
		SET acknowledged_at = ?
		WHERE id = ? AND acknowledged_at IS NULL
	`

	now := time.Now()
	result, err := d.db.ExecContext(ctx, query, now, id.String())
	if err != nil {
		return fmt.Errorf("failed to acknowledge steering message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("steering message not found or already acknowledged: %s", id)
	}

	return nil
}

// GetSteeringMessages retrieves all steering messages for a session
func (d *sessionDAO) GetSteeringMessages(ctx context.Context, sessionID types.ID) ([]SteeringMessage, error) {
	query := `
		SELECT
			id, session_id, sequence, operator_id, message_type,
			content, timestamp, acknowledged_at, trace_id
		FROM steering_messages
		WHERE session_id = ?
		ORDER BY sequence ASC
	`

	rows, err := d.db.QueryContext(ctx, query, sessionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query steering messages: %w", err)
	}
	defer rows.Close()

	var messages []SteeringMessage
	for rows.Next() {
		var msg SteeringMessage
		var idStr, sessionIDStr string
		var contentJSON sql.NullString
		var acknowledgedAt sql.NullTime
		var traceID sql.NullString

		err := rows.Scan(
			&idStr,
			&sessionIDStr,
			&msg.Sequence,
			&msg.OperatorID,
			&msg.MessageType,
			&contentJSON,
			&msg.Timestamp,
			&acknowledgedAt,
			&traceID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan steering message: %w", err)
		}

		// Parse IDs
		msg.ID, err = types.ParseID(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse message ID: %w", err)
		}

		msg.SessionID, err = types.ParseID(sessionIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse session ID: %w", err)
		}

		// Handle nullable fields
		if contentJSON.Valid && contentJSON.String != "" && contentJSON.String != "{}" {
			msg.Content = json.RawMessage(contentJSON.String)
		}

		if acknowledgedAt.Valid {
			msg.AcknowledgedAt = &acknowledgedAt.Time
		}

		if traceID.Valid {
			msg.TraceID = traceID.String
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating steering messages: %w", err)
	}

	return messages, nil
}
