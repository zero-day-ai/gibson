package console

import (
	"context"
	"testing"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// mockComponentDAO is a simple mock for testing
type mockComponentDAO struct {
	components map[string]*component.Component
}

func newMockComponentDAO() *mockComponentDAO {
	return &mockComponentDAO{
		components: make(map[string]*component.Component),
	}
}

func (m *mockComponentDAO) Create(ctx context.Context, comp *component.Component) error {
	return nil
}

func (m *mockComponentDAO) GetByID(ctx context.Context, id int64) (*component.Component, error) {
	return nil, nil
}

func (m *mockComponentDAO) GetByName(ctx context.Context, kind component.ComponentKind, name string) (*component.Component, error) {
	key := string(kind) + ":" + name
	if comp, ok := m.components[key]; ok {
		return comp, nil
	}
	return nil, nil
}

func (m *mockComponentDAO) List(ctx context.Context, kind component.ComponentKind) ([]*component.Component, error) {
	return nil, nil
}

func (m *mockComponentDAO) ListAll(ctx context.Context) (map[component.ComponentKind][]*component.Component, error) {
	return nil, nil
}

func (m *mockComponentDAO) ListByStatus(ctx context.Context, kind component.ComponentKind, status component.ComponentStatus) ([]*component.Component, error) {
	return nil, nil
}

func (m *mockComponentDAO) Update(ctx context.Context, comp *component.Component) error {
	return nil
}

func (m *mockComponentDAO) UpdateStatus(ctx context.Context, id int64, status component.ComponentStatus, pid, port int) error {
	return nil
}

func (m *mockComponentDAO) Delete(ctx context.Context, kind component.ComponentKind, name string) error {
	return nil
}

func (m *mockComponentDAO) AddComponent(kind component.ComponentKind, name string, status component.ComponentStatus, pid int) {
	key := string(kind) + ":" + name
	m.components[key] = &component.Component{
		Kind:   kind,
		Name:   name,
		Status: status,
		PID:    pid,
	}
}

func TestHandleFocus(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	tests := []struct {
		name        string
		args        []string
		wantError   bool
		errorSubstr string
	}{
		{
			name:        "no agent name",
			args:        []string{},
			wantError:   true,
			errorSubstr: "Usage",
		},
		{
			name:        "focus without ComponentDAO",
			args:        []string{"davinci"},
			wantError:   true,
			errorSubstr: "component DAO not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create executor without ComponentDAO
			executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
			executor.SetupHandlers()

			result, err := executor.handleFocus(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("handleFocus returned error: %v", err)
			}

			if tt.wantError {
				if !result.IsError {
					t.Error("expected error result, got success")
				}
				if tt.errorSubstr != "" && result.Error == "" {
					t.Errorf("expected error containing %q, got empty", tt.errorSubstr)
				}
			} else {
				if result.IsError {
					t.Errorf("unexpected error: %s", result.Error)
				}
			}
		})
	}
}

func TestHandleFocus_AgentNotFound(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	mockDAO := newMockComponentDAO()
	// Don't add any agents - simulating agent not found

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{
		ComponentDAO: mockDAO,
	})
	executor.SetupHandlers()

	result, err := executor.handleFocus(context.Background(), []string{"nonexistent"})
	if err != nil {
		t.Fatalf("handleFocus returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when agent not found")
	}
	if result.Error == "" {
		t.Error("expected error message about agent not found")
	}
}

func TestHandleFocus_AgentNotRunning(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	mockDAO := newMockComponentDAO()
	// Add agent that is stopped (not running)
	mockDAO.AddComponent(component.ComponentKindAgent, "davinci", component.ComponentStatusStopped, 0)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{
		ComponentDAO: mockDAO,
	})
	executor.SetupHandlers()

	result, err := executor.handleFocus(context.Background(), []string{"davinci"})
	if err != nil {
		t.Fatalf("handleFocus returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when agent not running")
	}
	if result.Error == "" {
		t.Error("expected error message about agent not running")
	}
}

func TestHandleFocus_NoStreamManager(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	mockDAO := newMockComponentDAO()
	// Add a running agent
	mockDAO.AddComponent(component.ComponentKindAgent, "davinci", component.ComponentStatusRunning, 12345)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{
		ComponentDAO: mockDAO,
		// No StreamManager
	})
	executor.SetupHandlers()

	result, err := executor.handleFocus(context.Background(), []string{"davinci"})
	if err != nil {
		t.Fatalf("handleFocus returned error: %v", err)
	}

	// Should fail because StreamManager is nil
	if !result.IsError {
		t.Error("expected error when StreamManager is nil")
	}
}

func TestHandleFocus_WithStreamManager(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	mockDAO := newMockComponentDAO()
	// Add a running agent
	mockDAO.AddComponent(component.ComponentKindAgent, "davinci", component.ComponentStatusRunning, 12345)

	// Create a minimal stream manager
	ctx := context.Background()
	sessionDAO := &mockSessionDAO{}
	streamManager := agent.NewStreamManager(ctx, agent.StreamManagerConfig{
		SessionDAO: sessionDAO,
	})

	executor := NewExecutor(ctx, registry, ExecutorConfig{
		ComponentDAO:  mockDAO,
		StreamManager: streamManager,
	})
	executor.SetupHandlers()

	result, err := executor.handleFocus(ctx, []string{"davinci"})
	if err != nil {
		t.Fatalf("handleFocus returned error: %v", err)
	}

	// Should fail because there's no active streaming session
	if !result.IsError {
		t.Error("expected error when no streaming session")
	}
}

// mockSessionDAO is a simple mock for testing that implements database.SessionDAO
type mockSessionDAO struct{}

func (m *mockSessionDAO) CreateSession(ctx context.Context, session *database.AgentSession) error {
	return nil
}

func (m *mockSessionDAO) UpdateSession(ctx context.Context, session *database.AgentSession) error {
	return nil
}

func (m *mockSessionDAO) GetSession(ctx context.Context, id types.ID) (*database.AgentSession, error) {
	return nil, nil
}

func (m *mockSessionDAO) ListSessionsByMission(ctx context.Context, missionID types.ID) ([]database.AgentSession, error) {
	return nil, nil
}

func (m *mockSessionDAO) InsertStreamEvent(ctx context.Context, event *database.StreamEvent) error {
	return nil
}

func (m *mockSessionDAO) InsertStreamEventBatch(ctx context.Context, events []database.StreamEvent) error {
	return nil
}

func (m *mockSessionDAO) GetStreamEvents(ctx context.Context, sessionID types.ID, filter database.StreamEventFilter) ([]database.StreamEvent, error) {
	return nil, nil
}

func (m *mockSessionDAO) InsertSteeringMessage(ctx context.Context, msg *database.SteeringMessage) error {
	return nil
}

func (m *mockSessionDAO) AcknowledgeSteeringMessage(ctx context.Context, id types.ID) error {
	return nil
}

func (m *mockSessionDAO) GetSteeringMessages(ctx context.Context, sessionID types.ID) ([]database.SteeringMessage, error) {
	return nil, nil
}

func TestHandleInterrupt_NoFocusedAgent(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.handleInterrupt(context.Background(), []string{})
	if err != nil {
		t.Fatalf("handleInterrupt returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when no agent focused")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestHandleInterrupt_NoStreamManager(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()
	executor.SetFocusedAgent("test-agent")

	result, err := executor.handleInterrupt(context.Background(), []string{})
	if err != nil {
		t.Fatalf("handleInterrupt returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when StreamManager is nil")
	}
}

func TestHandleMode_InvalidMode(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()
	executor.SetFocusedAgent("test-agent")

	result, err := executor.handleMode(context.Background(), []string{"invalid"})
	if err != nil {
		t.Fatalf("handleMode returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid mode")
	}
}

func TestHandleMode_NoArgs(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.handleMode(context.Background(), []string{})
	if err != nil {
		t.Fatalf("handleMode returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when no mode provided")
	}
}

func TestHandlePause_NoFocusedAgent(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.handlePause(context.Background(), []string{})
	if err != nil {
		t.Fatalf("handlePause returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when no agent focused")
	}
}

func TestHandleResume_NoFocusedAgent(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.handleResume(context.Background(), []string{})
	if err != nil {
		t.Fatalf("handleResume returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error when no agent focused")
	}
}

func TestGetSetFocusedAgent(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})

	// Initially empty
	if got := executor.GetFocusedAgent(); got != "" {
		t.Errorf("initial focused agent = %q, want empty", got)
	}

	// Set and get
	executor.SetFocusedAgent("test-agent")
	if got := executor.GetFocusedAgent(); got != "test-agent" {
		t.Errorf("focused agent = %q, want %q", got, "test-agent")
	}

	// Set to different value
	executor.SetFocusedAgent("another-agent")
	if got := executor.GetFocusedAgent(); got != "another-agent" {
		t.Errorf("focused agent = %q, want %q", got, "another-agent")
	}

	// Clear
	executor.SetFocusedAgent("")
	if got := executor.GetFocusedAgent(); got != "" {
		t.Errorf("focused agent = %q, want empty", got)
	}
}

func TestSetupHandlers_AllCommandsHaveHandlers(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	commands := registry.List()
	nilHandlers := []string{}

	for _, cmd := range commands {
		if cmd.Handler == nil {
			nilHandlers = append(nilHandlers, cmd.Name)
		}
	}

	if len(nilHandlers) > 0 {
		t.Errorf("commands with nil handlers: %v", nilHandlers)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.Execute("/nonexistent")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for unknown command")
	}
}

func TestExecute_HelpCommand(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.Execute("/help")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.IsError {
		t.Errorf("help command failed: %s", result.Error)
	}

	if result.Output == "" {
		t.Error("help command returned empty output")
	}
}

func TestExecute_ClearCommand(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.Execute("/clear")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.IsError {
		t.Errorf("clear command failed: %s", result.Error)
	}
}

func TestExecute_FocusCommand(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	// Create executor without ComponentDAO - focus should fail
	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.Execute("/focus test-agent")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Expected to fail because no ComponentDAO
	if !result.IsError {
		t.Error("expected focus to fail without ComponentDAO")
	}

	// Focused agent should not be set
	if executor.GetFocusedAgent() != "" {
		t.Errorf("focused agent = %q, want empty (focus should have failed)", executor.GetFocusedAgent())
	}
}
