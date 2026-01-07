package mission

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// EventStore provides persistence for mission events.
// Events are persisted for audit trail and replay capability.
type EventStore interface {
	// Append persists an event to the log.
	// This is synchronous and durable - the event is written to disk before returning.
	Append(ctx context.Context, event *MissionEvent) error

	// Query retrieves events matching filter criteria.
	// Results are ordered by created_at ascending.
	Query(ctx context.Context, filter *EventFilter) ([]*MissionEvent, error)

	// Stream returns a channel of events for a mission starting from a timestamp.
	// The channel is closed when all events are sent or context is cancelled.
	// This is useful for event replay during mission resume.
	Stream(ctx context.Context, missionID types.ID, fromTimestamp time.Time) (<-chan *MissionEvent, error)
}

// EventFilter provides filtering options for event queries.
type EventFilter struct {
	// MissionID filters events for a specific mission.
	MissionID *types.ID

	// EventTypes filters by event type (supports multiple types).
	EventTypes []MissionEventType

	// After filters events created after this time.
	After *time.Time

	// Before filters events created before this time.
	Before *time.Time

	// Limit limits the number of results.
	Limit int

	// Offset skips the first N results.
	Offset int
}

// NewEventFilter creates a new empty filter with default pagination.
func NewEventFilter() *EventFilter {
	return &EventFilter{
		Limit:  100,
		Offset: 0,
	}
}

// WithMissionID filters events for a specific mission.
func (f *EventFilter) WithMissionID(missionID types.ID) *EventFilter {
	f.MissionID = &missionID
	return f
}

// WithEventTypes filters by event types.
func (f *EventFilter) WithEventTypes(types ...MissionEventType) *EventFilter {
	f.EventTypes = types
	return f
}

// WithTimeRange filters by time range.
func (f *EventFilter) WithTimeRange(after, before time.Time) *EventFilter {
	f.After = &after
	f.Before = &before
	return f
}

// WithPagination sets pagination parameters.
func (f *EventFilter) WithPagination(limit, offset int) *EventFilter {
	f.Limit = limit
	f.Offset = offset
	return f
}

// DBEventStore implements EventStore using a SQL database.
type DBEventStore struct {
	db *database.DB
}

// NewDBEventStore creates a new database-backed event store.
func NewDBEventStore(db *database.DB) *DBEventStore {
	return &DBEventStore{db: db}
}

// Append persists an event to the log.
func (s *DBEventStore) Append(ctx context.Context, event *MissionEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Generate event ID if not set
	eventID := types.NewID()

	// Marshal payload to JSON
	var payloadJSON string
	if event.Payload != nil {
		data, err := json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal event payload: %w", err)
		}
		payloadJSON = string(data)
	}

	// Set timestamp if not set
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	query := `
		INSERT INTO mission_events (
			id, mission_id, event_type, payload, created_at
		) VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		eventID.String(),
		event.MissionID.String(),
		string(event.Type),
		payloadJSON,
		timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}

	return nil
}

// Query retrieves events matching filter criteria.
func (s *DBEventStore) Query(ctx context.Context, filter *EventFilter) ([]*MissionEvent, error) {
	if filter == nil {
		filter = NewEventFilter()
	}

	// Set default limit if not specified
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Build WHERE clause dynamically
	var conditions []string
	var args []interface{}

	if filter.MissionID != nil {
		conditions = append(conditions, "mission_id = ?")
		args = append(args, filter.MissionID.String())
	}

	if len(filter.EventTypes) > 0 {
		placeholders := make([]string, len(filter.EventTypes))
		for i, eventType := range filter.EventTypes {
			placeholders[i] = "?"
			args = append(args, string(eventType))
		}
		conditions = append(conditions, fmt.Sprintf("event_type IN (%s)", strings.Join(placeholders, ", ")))
	}

	if filter.After != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *filter.After)
	}

	if filter.Before != nil {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, *filter.Before)
	}

	// Build query
	query := `
		SELECT
			id, mission_id, event_type, payload, created_at
		FROM mission_events
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at ASC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// Stream returns a channel of events for a mission starting from a timestamp.
func (s *DBEventStore) Stream(ctx context.Context, missionID types.ID, fromTimestamp time.Time) (<-chan *MissionEvent, error) {
	// Create buffered channel to prevent blocking
	eventCh := make(chan *MissionEvent, 100)

	// Start goroutine to stream events
	go func() {
		defer close(eventCh)

		// Query events in batches
		const batchSize = 100
		offset := 0

		for {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Query batch of events
			filter := NewEventFilter().
				WithMissionID(missionID).
				WithTimeRange(fromTimestamp, time.Now()).
				WithPagination(batchSize, offset)

			events, err := s.Query(ctx, filter)
			if err != nil {
				// Log error but continue - we can't return error from goroutine
				// In production, this would use a logger
				return
			}

			// Send events to channel
			for _, event := range events {
				select {
				case eventCh <- event:
					// Event sent successfully
				case <-ctx.Done():
					return
				}
			}

			// If we got fewer events than batch size, we're done
			if len(events) < batchSize {
				return
			}

			offset += batchSize
		}
	}()

	return eventCh, nil
}

// scanEvents scans multiple events from query rows.
func (s *DBEventStore) scanEvents(rows *sql.Rows) ([]*MissionEvent, error) {
	events := make([]*MissionEvent, 0)

	for rows.Next() {
		event, err := s.scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return events, nil
}

// scanEvent scans a single event from a query row.
func (s *DBEventStore) scanEvent(scanner interface {
	Scan(dest ...interface{}) error
}) (*MissionEvent, error) {
	var (
		idStr        string
		missionIDStr string
		eventTypeStr string
		payloadStr   sql.NullString
		createdAt    time.Time
	)

	err := scanner.Scan(
		&idStr,
		&missionIDStr,
		&eventTypeStr,
		&payloadStr,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	// Parse mission ID
	missionID, err := types.ParseID(missionIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mission ID: %w", err)
	}

	// Create event
	event := &MissionEvent{
		Type:      MissionEventType(eventTypeStr),
		MissionID: missionID,
		Timestamp: createdAt,
	}

	// Unmarshal payload if present
	if payloadStr.Valid && payloadStr.String != "" {
		var payload interface{}
		if err := json.Unmarshal([]byte(payloadStr.String), &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event payload: %w", err)
		}
		event.Payload = payload
	}

	return event, nil
}

// Ensure DBEventStore implements EventStore at compile time.
var _ EventStore = (*DBEventStore)(nil)
