package console

import (
	"context"
	"strings"
	"testing"

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

func TestExecute_DeprecatedCommand(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	tests := []struct {
		name            string
		command         string
		wantErrorPrefix string
		wantSuggestion  string
	}{
		{
			name:            "mission-create deprecated",
			command:         "/mission-create",
			wantErrorPrefix: "Command '/mission-create' is deprecated.",
			wantSuggestion:  "Use '/mission run -f <file>' instead",
		},
		{
			name:            "agent-start deprecated",
			command:         "/agent-start",
			wantErrorPrefix: "Command '/agent-start' is deprecated.",
			wantSuggestion:  "Use '/agent start <name>' instead",
		},
		{
			name:            "tool-install deprecated",
			command:         "/tool-install",
			wantErrorPrefix: "Command '/tool-install' is deprecated.",
			wantSuggestion:  "Use '/tool install <source>' instead",
		},
		{
			name:            "plugin-stop deprecated",
			command:         "/plugin-stop",
			wantErrorPrefix: "Command '/plugin-stop' is deprecated.",
			wantSuggestion:  "Use '/plugin stop <name>' instead",
		},
		{
			name:            "agent-execute deprecated",
			command:         "/agent-execute",
			wantErrorPrefix: "Command '/agent-execute' is deprecated.",
			wantSuggestion:  "Use '/agent start <name>' then interact via steering commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(tt.command)
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}

			if !result.IsError {
				t.Error("expected error for deprecated command")
			}

			if result.Error == "" {
				t.Error("expected error message for deprecated command")
			}

			// Check that the error contains the deprecation message
			if !strings.Contains(result.Error, tt.wantErrorPrefix) {
				t.Errorf("error message %q does not contain expected prefix %q", result.Error, tt.wantErrorPrefix)
			}

			// Check that the error contains the suggestion
			if !strings.Contains(result.Error, tt.wantSuggestion) {
				t.Errorf("error message %q does not contain expected suggestion %q", result.Error, tt.wantSuggestion)
			}
		})
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

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	result, err := executor.Execute("/focus test-agent")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// Focus now works without validation
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Error)
	}

	// Focused agent should be set
	if executor.GetFocusedAgent() != "test-agent" {
		t.Errorf("focused agent = %q, want %q", executor.GetFocusedAgent(), "test-agent")
	}
}

// TestExecute_SubcommandRouting tests that subcommand-based commands have handlers registered
func TestExecute_SubcommandRouting(t *testing.T) {
	registry := NewCommandRegistry()
	RegisterDefaultCommands(registry)

	executor := NewExecutor(context.Background(), registry, ExecutorConfig{})
	executor.SetupHandlers()

	tests := []struct {
		name        string
		commandName string
		hasHandler  bool
	}{
		{
			name:        "mission command has handler",
			commandName: "mission",
			hasHandler:  true,
		},
		{
			name:        "agent command has handler",
			commandName: "agent",
			hasHandler:  true,
		},
		{
			name:        "tool command has handler",
			commandName: "tool",
			hasHandler:  true,
		},
		{
			name:        "plugin command has handler",
			commandName: "plugin",
			hasHandler:  true,
		},
		{
			name:        "mission alias (missions) resolves to same command",
			commandName: "missions",
			hasHandler:  true,
		},
		{
			name:        "agent alias (agents) resolves to same command",
			commandName: "agents",
			hasHandler:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := registry.Get(tt.commandName)
			if !ok {
				t.Fatalf("command %q not found in registry", tt.commandName)
			}

			if tt.hasHandler && cmd.Handler == nil {
				t.Errorf("command %q should have a handler but doesn't", tt.commandName)
			}

			if !tt.hasHandler && cmd.Handler != nil {
				t.Errorf("command %q should not have a handler but does", tt.commandName)
			}
		})
	}
}
