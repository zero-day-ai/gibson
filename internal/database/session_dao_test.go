package database

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// TestCreateSession tests creating a new agent session
func TestCreateSession(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	missionID := types.NewID()
	metadata := json.RawMessage(`{"key": "value", "test": true}`)

	session := &AgentSession{
		ID:        types.NewID(),
		MissionID: missionID,
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
		StartedAt: time.Now().UTC(),
		Metadata:  metadata,
	}

	err := dao.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify session was created
	retrieved, err := dao.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("expected ID %s, got %s", session.ID, retrieved.ID)
	}
	if retrieved.MissionID != missionID {
		t.Errorf("expected MissionID %s, got %s", missionID, retrieved.MissionID)
	}
	if retrieved.AgentName != "test-agent" {
		t.Errorf("expected AgentName 'test-agent', got %s", retrieved.AgentName)
	}
	if retrieved.Status != AgentStatusRunning {
		t.Errorf("expected Status running, got %s", retrieved.Status)
	}
	if retrieved.Mode != AgentModeAutonomous {
		t.Errorf("expected Mode autonomous, got %s", retrieved.Mode)
	}
	if string(retrieved.Metadata) != string(metadata) {
		t.Errorf("expected Metadata %s, got %s", metadata, retrieved.Metadata)
	}
}

// TestCreateSession_AutoGeneratesID tests that ID is auto-generated if not provided
func TestCreateSession_AutoGeneratesID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	session := &AgentSession{
		// ID not provided
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeInteractive,
	}

	err := dao.CreateSession(ctx, session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify ID was generated
	if session.ID.IsZero() {
		t.Error("expected ID to be auto-generated")
	}

	// Verify StartedAt was set
	if session.StartedAt.IsZero() {
		t.Error("expected StartedAt to be auto-set")
	}
}

// TestUpdateSession tests updating an existing session
func TestUpdateSession(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create initial session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
		Metadata:  json.RawMessage(`{"initial": "data"}`),
	}

	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Update session
	endTime := time.Now().UTC()
	session.Status = AgentStatusCompleted
	session.Mode = AgentModeInteractive
	session.EndedAt = &endTime
	session.Metadata = json.RawMessage(`{"updated": "metadata"}`)

	err := dao.UpdateSession(ctx, session)
	if err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	// Verify updates
	updated, err := dao.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if updated.Status != AgentStatusCompleted {
		t.Errorf("expected Status completed, got %s", updated.Status)
	}
	if updated.Mode != AgentModeInteractive {
		t.Errorf("expected Mode interactive, got %s", updated.Mode)
	}
	if updated.EndedAt == nil {
		t.Error("expected EndedAt to be set")
	}
	if string(updated.Metadata) != string(session.Metadata) {
		t.Errorf("expected Metadata %s, got %s", session.Metadata, updated.Metadata)
	}
}

// TestGetSession tests retrieving a session by ID
func TestGetSession(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}

	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Retrieve session
	retrieved, err := dao.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("expected ID %s, got %s", session.ID, retrieved.ID)
	}
}

// TestGetSession_NotFound tests error handling when session doesn't exist
func TestGetSession_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	nonExistentID := types.NewID()
	_, err := dao.GetSession(ctx, nonExistentID)
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

// TestListSessionsByMission tests listing all sessions for a mission
func TestListSessionsByMission(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	missionID := types.NewID()
	otherMissionID := types.NewID()

	// Create sessions for missionID
	sessions := []AgentSession{
		{
			MissionID: missionID,
			AgentName: "agent-1",
			Status:    AgentStatusRunning,
			Mode:      AgentModeAutonomous,
			StartedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
		{
			MissionID: missionID,
			AgentName: "agent-2",
			Status:    AgentStatusCompleted,
			Mode:      AgentModeInteractive,
			StartedAt: time.Now().UTC().Add(-1 * time.Hour),
		},
		{
			MissionID: otherMissionID,
			AgentName: "other-agent",
			Status:    AgentStatusRunning,
			Mode:      AgentModeAutonomous,
			StartedAt: time.Now().UTC(),
		},
	}

	for i := range sessions {
		if err := dao.CreateSession(ctx, &sessions[i]); err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
	}

	// List sessions for missionID
	retrieved, err := dao.ListSessionsByMission(ctx, missionID)
	if err != nil {
		t.Fatalf("ListSessionsByMission failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(retrieved))
	}

	// Verify ordering (DESC by started_at)
	if retrieved[0].StartedAt.Before(retrieved[1].StartedAt) {
		t.Error("expected sessions to be ordered by started_at DESC")
	}

	// Verify both belong to the mission
	for _, s := range retrieved {
		if s.MissionID != missionID {
			t.Errorf("expected MissionID %s, got %s", missionID, s.MissionID)
		}
	}
}

// TestInsertStreamEvent tests inserting a single stream event
func TestInsertStreamEvent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert stream event
	event := &StreamEvent{
		ID:        types.NewID(),
		SessionID: session.ID,
		Sequence:  1,
		EventType: StreamEventOutput,
		Content:   json.RawMessage(`{"text": "Agent output"}`),
		Timestamp: time.Now().UTC(),
		TraceID:   "trace-123",
		SpanID:    "span-456",
	}

	err := dao.InsertStreamEvent(ctx, event)
	if err != nil {
		t.Fatalf("InsertStreamEvent failed: %v", err)
	}

	// Verify event was inserted
	events, err := dao.GetStreamEvents(ctx, session.ID, StreamEventFilter{})
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].EventType != StreamEventOutput {
		t.Errorf("expected EventType output, got %s", events[0].EventType)
	}
	if events[0].Sequence != 1 {
		t.Errorf("expected Sequence 1, got %d", events[0].Sequence)
	}
	if events[0].TraceID != "trace-123" {
		t.Errorf("expected TraceID 'trace-123', got %s", events[0].TraceID)
	}
}

// TestInsertStreamEventBatch tests batch inserting stream events
func TestInsertStreamEventBatch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create batch of events
	events := []StreamEvent{
		{
			SessionID: session.ID,
			Sequence:  1,
			EventType: StreamEventOutput,
			Content:   json.RawMessage(`{"text": "First output"}`),
		},
		{
			SessionID: session.ID,
			Sequence:  2,
			EventType: StreamEventToolCall,
			Content:   json.RawMessage(`{"tool": "bash", "args": ["ls"]}`),
		},
		{
			SessionID: session.ID,
			Sequence:  3,
			EventType: StreamEventToolResult,
			Content:   json.RawMessage(`{"result": "file1.txt\nfile2.txt"}`),
		},
	}

	err := dao.InsertStreamEventBatch(ctx, events)
	if err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Verify all events were inserted
	retrieved, err := dao.GetStreamEvents(ctx, session.ID, StreamEventFilter{})
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Fatalf("expected 3 events, got %d", len(retrieved))
	}

	// Verify ordering by sequence
	for i, event := range retrieved {
		if event.Sequence != int64(i+1) {
			t.Errorf("expected Sequence %d, got %d", i+1, event.Sequence)
		}
	}
}

// TestGetStreamEvents_NoFilter tests retrieving all stream events
func TestGetStreamEvents_NoFilter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert events
	events := []StreamEvent{
		{SessionID: session.ID, Sequence: 1, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 2, EventType: StreamEventToolCall, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 3, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
	}

	if err := dao.InsertStreamEventBatch(ctx, events); err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Retrieve all events
	retrieved, err := dao.GetStreamEvents(ctx, session.ID, StreamEventFilter{})
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("expected 3 events, got %d", len(retrieved))
	}
}

// TestGetStreamEvents_FilterByType tests filtering events by type
func TestGetStreamEvents_FilterByType(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert events of different types
	events := []StreamEvent{
		{SessionID: session.ID, Sequence: 1, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 2, EventType: StreamEventToolCall, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 3, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 4, EventType: StreamEventFinding, Content: json.RawMessage(`{}`)},
	}

	if err := dao.InsertStreamEventBatch(ctx, events); err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Filter by type
	filter := StreamEventFilter{
		EventTypes: []StreamEventType{StreamEventOutput},
	}

	retrieved, err := dao.GetStreamEvents(ctx, session.ID, filter)
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("expected 2 output events, got %d", len(retrieved))
	}

	for _, event := range retrieved {
		if event.EventType != StreamEventOutput {
			t.Errorf("expected EventType output, got %s", event.EventType)
		}
	}
}

// TestGetStreamEvents_FilterBySequence tests filtering events by sequence range
func TestGetStreamEvents_FilterBySequence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert events with different sequences
	events := []StreamEvent{
		{SessionID: session.ID, Sequence: 1, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 2, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 3, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 4, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 5, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
	}

	if err := dao.InsertStreamEventBatch(ctx, events); err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Filter by sequence range
	filter := StreamEventFilter{
		FromSeq: 2,
		ToSeq:   4,
	}

	retrieved, err := dao.GetStreamEvents(ctx, session.ID, filter)
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Fatalf("expected 3 events (seq 2-4), got %d", len(retrieved))
	}

	// Verify sequence range
	for _, event := range retrieved {
		if event.Sequence < 2 || event.Sequence > 4 {
			t.Errorf("expected Sequence in range 2-4, got %d", event.Sequence)
		}
	}
}

// TestGetStreamEvents_FilterByTime tests filtering events by time range
func TestGetStreamEvents_FilterByTime(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert events with different timestamps
	baseTime := time.Now().UTC()
	events := []StreamEvent{
		{SessionID: session.ID, Sequence: 1, EventType: StreamEventOutput, Content: json.RawMessage(`{}`), Timestamp: baseTime.Add(-5 * time.Minute)},
		{SessionID: session.ID, Sequence: 2, EventType: StreamEventOutput, Content: json.RawMessage(`{}`), Timestamp: baseTime.Add(-3 * time.Minute)},
		{SessionID: session.ID, Sequence: 3, EventType: StreamEventOutput, Content: json.RawMessage(`{}`), Timestamp: baseTime.Add(-1 * time.Minute)},
	}

	if err := dao.InsertStreamEventBatch(ctx, events); err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Filter by time range
	filter := StreamEventFilter{
		FromTime: baseTime.Add(-4 * time.Minute),
		ToTime:   baseTime.Add(-2 * time.Minute),
	}

	retrieved, err := dao.GetStreamEvents(ctx, session.ID, filter)
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(retrieved) != 1 {
		t.Fatalf("expected 1 event in time range, got %d", len(retrieved))
	}

	if retrieved[0].Sequence != 2 {
		t.Errorf("expected Sequence 2, got %d", retrieved[0].Sequence)
	}
}

// TestInsertSteeringMessage tests inserting a steering message
func TestInsertSteeringMessage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeInteractive,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert steering message
	msg := &SteeringMessage{
		ID:          types.NewID(),
		SessionID:   session.ID,
		Sequence:    1,
		OperatorID:  "operator-123",
		MessageType: SteeringTypeSteer,
		Content:     json.RawMessage(`{"instruction": "Focus on XSS vulnerabilities"}`),
		Timestamp:   time.Now().UTC(),
		TraceID:     "trace-789",
	}

	err := dao.InsertSteeringMessage(ctx, msg)
	if err != nil {
		t.Fatalf("InsertSteeringMessage failed: %v", err)
	}

	// Verify message was inserted
	messages, err := dao.GetSteeringMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSteeringMessages failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].OperatorID != "operator-123" {
		t.Errorf("expected OperatorID 'operator-123', got %s", messages[0].OperatorID)
	}
	if messages[0].MessageType != SteeringTypeSteer {
		t.Errorf("expected MessageType steer, got %s", messages[0].MessageType)
	}
	if messages[0].AcknowledgedAt != nil {
		t.Error("expected AcknowledgedAt to be nil")
	}
}

// TestAcknowledgeSteeringMessage tests marking a steering message as acknowledged
func TestAcknowledgeSteeringMessage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeInteractive,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert steering message
	msg := &SteeringMessage{
		SessionID:   session.ID,
		Sequence:    1,
		OperatorID:  "operator-123",
		MessageType: SteeringTypeInterrupt,
		Content:     json.RawMessage(`{}`),
	}

	if err := dao.InsertSteeringMessage(ctx, msg); err != nil {
		t.Fatalf("InsertSteeringMessage failed: %v", err)
	}

	// Acknowledge the message
	beforeAck := time.Now().UTC()
	err := dao.AcknowledgeSteeringMessage(ctx, msg.ID)
	afterAck := time.Now().UTC()

	if err != nil {
		t.Fatalf("AcknowledgeSteeringMessage failed: %v", err)
	}

	// Verify acknowledgement
	messages, err := dao.GetSteeringMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSteeringMessages failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].AcknowledgedAt == nil {
		t.Fatal("expected AcknowledgedAt to be set")
	}

	ackTime := *messages[0].AcknowledgedAt
	if ackTime.Before(beforeAck) || ackTime.After(afterAck) {
		t.Errorf("acknowledgement time %v not within expected range %v - %v", ackTime, beforeAck, afterAck)
	}

	// Try to acknowledge again - should fail
	err = dao.AcknowledgeSteeringMessage(ctx, msg.ID)
	if err == nil {
		t.Error("expected error acknowledging already-acknowledged message")
	}
}

// TestGetSteeringMessages tests retrieving steering messages
func TestGetSteeringMessages(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeInteractive,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert multiple steering messages
	messages := []*SteeringMessage{
		{
			SessionID:   session.ID,
			Sequence:    1,
			OperatorID:  "operator-1",
			MessageType: SteeringTypeSteer,
			Content:     json.RawMessage(`{"instruction": "Focus on SQL injection"}`),
		},
		{
			SessionID:   session.ID,
			Sequence:    2,
			OperatorID:  "operator-1",
			MessageType: SteeringTypeInterrupt,
			Content:     json.RawMessage(`{}`),
		},
		{
			SessionID:   session.ID,
			Sequence:    3,
			OperatorID:  "operator-2",
			MessageType: SteeringTypeResume,
			Content:     json.RawMessage(`{}`),
		},
	}

	for _, msg := range messages {
		if err := dao.InsertSteeringMessage(ctx, msg); err != nil {
			t.Fatalf("InsertSteeringMessage failed: %v", err)
		}
	}

	// Retrieve all messages
	retrieved, err := dao.GetSteeringMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSteeringMessages failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(retrieved))
	}

	// Verify ordering by sequence
	for i, msg := range retrieved {
		expectedSeq := int64(i + 1)
		if msg.Sequence != expectedSeq {
			t.Errorf("expected Sequence %d, got %d", expectedSeq, msg.Sequence)
		}
	}

	// Verify message types
	if retrieved[0].MessageType != SteeringTypeSteer {
		t.Errorf("expected first message type steer, got %s", retrieved[0].MessageType)
	}
	if retrieved[1].MessageType != SteeringTypeInterrupt {
		t.Errorf("expected second message type interrupt, got %s", retrieved[1].MessageType)
	}
	if retrieved[2].MessageType != SteeringTypeResume {
		t.Errorf("expected third message type resume, got %s", retrieved[2].MessageType)
	}
}

// TestStreamEventFilter_Limit tests limiting the number of events returned
func TestStreamEventFilter_Limit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert many events
	var events []StreamEvent
	for i := 1; i <= 10; i++ {
		events = append(events, StreamEvent{
			SessionID: session.ID,
			Sequence:  int64(i),
			EventType: StreamEventOutput,
			Content:   json.RawMessage(`{}`),
		})
	}

	if err := dao.InsertStreamEventBatch(ctx, events); err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Retrieve with limit
	filter := StreamEventFilter{
		Limit: 5,
	}

	retrieved, err := dao.GetStreamEvents(ctx, session.ID, filter)
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	if len(retrieved) != 5 {
		t.Fatalf("expected 5 events (limit), got %d", len(retrieved))
	}
}

// TestStreamEventFilter_MultipleFilters tests combining multiple filters
func TestStreamEventFilter_MultipleFilters(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert events with different types and sequences
	events := []StreamEvent{
		{SessionID: session.ID, Sequence: 1, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 2, EventType: StreamEventToolCall, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 3, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 4, EventType: StreamEventToolResult, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 5, EventType: StreamEventOutput, Content: json.RawMessage(`{}`)},
		{SessionID: session.ID, Sequence: 6, EventType: StreamEventFinding, Content: json.RawMessage(`{}`)},
	}

	if err := dao.InsertStreamEventBatch(ctx, events); err != nil {
		t.Fatalf("InsertStreamEventBatch failed: %v", err)
	}

	// Combine filters: type = output, sequence 2-5, limit 2
	filter := StreamEventFilter{
		EventTypes: []StreamEventType{StreamEventOutput},
		FromSeq:    2,
		ToSeq:      5,
		Limit:      2,
	}

	retrieved, err := dao.GetStreamEvents(ctx, session.ID, filter)
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}

	// Should get sequences 3 and 5 (both output and in range), limited to 2
	if len(retrieved) != 2 {
		t.Fatalf("expected 2 events, got %d", len(retrieved))
	}

	for _, event := range retrieved {
		if event.EventType != StreamEventOutput {
			t.Errorf("expected EventType output, got %s", event.EventType)
		}
		if event.Sequence < 2 || event.Sequence > 5 {
			t.Errorf("expected Sequence in range 2-5, got %d", event.Sequence)
		}
	}
}

// TestSessionDAO_CascadeDelete tests that stream events are deleted when session is deleted
func TestSessionDAO_CascadeDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Create session
	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
	}
	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Insert stream events
	event := &StreamEvent{
		SessionID: session.ID,
		Sequence:  1,
		EventType: StreamEventOutput,
		Content:   json.RawMessage(`{}`),
	}
	if err := dao.InsertStreamEvent(ctx, event); err != nil {
		t.Fatalf("InsertStreamEvent failed: %v", err)
	}

	// Insert steering message
	msg := &SteeringMessage{
		SessionID:   session.ID,
		Sequence:    1,
		OperatorID:  "operator-1",
		MessageType: SteeringTypeSteer,
		Content:     json.RawMessage(`{}`),
	}
	if err := dao.InsertSteeringMessage(ctx, msg); err != nil {
		t.Fatalf("InsertSteeringMessage failed: %v", err)
	}

	// Delete session
	_, err := db.conn.ExecContext(ctx, "DELETE FROM agent_sessions WHERE id = ?", session.ID.String())
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify stream events were cascade deleted
	events, err := dao.GetStreamEvents(ctx, session.ID, StreamEventFilter{})
	if err != nil {
		t.Fatalf("GetStreamEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after cascade delete, got %d", len(events))
	}

	// Verify steering messages were cascade deleted
	messages, err := dao.GetSteeringMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSteeringMessages failed: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after cascade delete, got %d", len(messages))
	}
}

// TestUpdateSession_NotFound tests error handling when updating non-existent session
func TestUpdateSession_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Try to update non-existent session
	session := &AgentSession{
		ID:        types.NewID(),
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusCompleted,
		Mode:      AgentModeAutonomous,
	}

	err := dao.UpdateSession(ctx, session)
	if err == nil {
		t.Error("expected error updating non-existent session")
	}
}

// TestAcknowledgeSteeringMessage_NotFound tests error handling when acknowledging non-existent message
func TestAcknowledgeSteeringMessage_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Try to acknowledge non-existent message
	nonExistentID := types.NewID()
	err := dao.AcknowledgeSteeringMessage(ctx, nonExistentID)
	if err == nil {
		t.Error("expected error acknowledging non-existent message")
	}
}

// TestEmptyBatch tests inserting an empty batch of events
func TestEmptyBatch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Insert empty batch (should be no-op)
	err := dao.InsertStreamEventBatch(ctx, []StreamEvent{})
	if err != nil {
		t.Errorf("empty batch should not fail: %v", err)
	}
}

// TestSessionMetadataHandling tests that metadata is properly serialized/deserialized
func TestSessionMetadataHandling(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewSessionDAO(db)

	// Test with complex metadata
	metadata := json.RawMessage(`{
		"key1": "value1",
		"key2": 42,
		"key3": true,
		"nested": {
			"array": [1, 2, 3],
			"object": {"foo": "bar"}
		}
	}`)

	session := &AgentSession{
		MissionID: types.NewID(),
		AgentName: "test-agent",
		Status:    AgentStatusRunning,
		Mode:      AgentModeAutonomous,
		Metadata:  metadata,
	}

	if err := dao.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Retrieve and verify metadata
	retrieved, err := dao.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if string(retrieved.Metadata) != string(metadata) {
		t.Errorf("metadata mismatch:\nexpected: %s\ngot: %s", metadata, retrieved.Metadata)
	}
}
