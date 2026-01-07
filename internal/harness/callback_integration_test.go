//go:build integration

package harness

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/sdk/api/gen/proto"
	"github.com/zero-day-ai/sdk/serve"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// mockIntegrationHarness is a minimal implementation of AgentHarness for testing callbacks.
// It implements only the methods needed for the integration test (Memory access).
type mockIntegrationHarness struct {
	memory memory.MemoryStore
	logger *slog.Logger
	tracer trace.Tracer
}

// Memory returns the memory store
func (m *mockIntegrationHarness) Memory() memory.MemoryStore {
	return m.memory
}

// Logger returns the logger
func (m *mockIntegrationHarness) Logger() *slog.Logger {
	return m.logger
}

// Tracer returns the tracer
func (m *mockIntegrationHarness) Tracer() trace.Tracer {
	return m.tracer
}

// Unimplemented methods required by AgentHarness interface
func (m *mockIntegrationHarness) Complete(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	return nil, fmt.Errorf("Complete not implemented in mock harness")
}

func (m *mockIntegrationHarness) CompleteWithTools(ctx context.Context, slot string, messages []llm.Message, tools []llm.ToolDef, opts ...CompletionOption) (*llm.CompletionResponse, error) {
	return nil, fmt.Errorf("CompleteWithTools not implemented in mock harness")
}

func (m *mockIntegrationHarness) Stream(ctx context.Context, slot string, messages []llm.Message, opts ...CompletionOption) (<-chan llm.StreamChunk, error) {
	return nil, fmt.Errorf("Stream not implemented in mock harness")
}

func (m *mockIntegrationHarness) CallTool(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
	return nil, fmt.Errorf("CallTool not implemented in mock harness")
}

func (m *mockIntegrationHarness) ListTools() []ToolDescriptor {
	return nil
}

func (m *mockIntegrationHarness) QueryPlugin(ctx context.Context, pluginName string, method string, params map[string]any) (any, error) {
	return nil, fmt.Errorf("QueryPlugin not implemented in mock harness")
}

func (m *mockIntegrationHarness) ListPlugins() []PluginDescriptor {
	return nil
}

func (m *mockIntegrationHarness) DelegateToAgent(ctx context.Context, agentName string, task agent.Task) (agent.Result, error) {
	return agent.Result{}, fmt.Errorf("DelegateToAgent not implemented in mock harness")
}

func (m *mockIntegrationHarness) ListAgents() []AgentDescriptor {
	return nil
}

func (m *mockIntegrationHarness) SubmitFinding(ctx context.Context, finding agent.Finding) error {
	return fmt.Errorf("SubmitFinding not implemented in mock harness")
}

func (m *mockIntegrationHarness) GetFindings(ctx context.Context, filter FindingFilter) ([]agent.Finding, error) {
	return nil, fmt.Errorf("GetFindings not implemented in mock harness")
}

func (m *mockIntegrationHarness) Mission() MissionContext {
	return MissionContext{}
}

func (m *mockIntegrationHarness) Target() TargetInfo {
	return TargetInfo{}
}

func (m *mockIntegrationHarness) Metrics() MetricsRecorder {
	return nil
}

func (m *mockIntegrationHarness) TokenUsage() *llm.TokenTracker {
	return nil
}

// TestCallbackIntegration tests the full callback flow from agent to harness.
// It verifies that:
// 1. CallbackServer can start on a random available port
// 2. Harness registration works correctly
// 3. SDK CallbackClient can connect to the server
// 4. Memory operations (Set/Get) work through the callback mechanism
// 5. Server and client cleanup properly
func TestCallbackIntegration(t *testing.T) {
	// Skip if we can't bind to loopback (e.g., in restricted environments)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Cannot bind to loopback address: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create logger for testing
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Step 1: Start CallbackServer on the available port
	server := NewCallbackServer(logger, port)
	require.NotNil(t, server, "CallbackServer should be created")

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Ensure we stop the server at the end
	defer func() {
		cancel()
		server.Stop()
		// Wait for server to finish
		select {
		case <-serverErrCh:
		case <-time.After(2 * time.Second):
			t.Log("Server didn't stop gracefully within timeout")
		}
	}()

	// Step 2: Create mock harness with working memory
	workingMem := memory.NewWorkingMemory(10000) // 10k token budget
	mockMem := &mockMemoryStore{working: workingMem}
	harness := &mockIntegrationHarness{
		memory: mockMem,
		logger: logger,
		tracer: noop.NewTracerProvider().Tracer("test"),
	}

	// Step 3: Register harness with a task ID
	taskID := "integration-test-task-123"
	server.RegisterHarness(taskID, harness)
	defer server.UnregisterHarness(taskID)

	// Step 4: Create SDK CallbackClient and connect
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	client, err := serve.NewCallbackClient(endpoint)
	require.NoError(t, err, "CallbackClient should be created without error")
	require.NotNil(t, client, "CallbackClient should not be nil")

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer connectCancel()

	err = client.Connect(connectCtx)
	require.NoError(t, err, "CallbackClient should connect successfully")
	defer client.Close()

	// Step 5: Set task context on the callback client
	client.SetTaskContext(taskID, "test-agent", "trace-123", "span-456")

	// Step 6: Test MemorySet callback
	testKey := "test-key"
	testValue := "test-value-123"

	setCtx, setCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer setCancel()

	setReq := &proto.MemorySetRequest{
		Key:       testKey,
		ValueJson: fmt.Sprintf(`"%s"`, testValue), // JSON-encode the string
	}

	setResp, err := client.MemorySet(setCtx, setReq)
	require.NoError(t, err, "MemorySet callback should succeed")
	require.NotNil(t, setResp, "MemorySet response should not be nil")
	assert.Nil(t, setResp.Error, "MemorySet should not return an error")

	// Step 7: Verify the value was actually set in the mock harness
	storedValue, found := workingMem.Get(testKey)
	assert.True(t, found, "Value should be found in working memory")
	assert.Equal(t, testValue, storedValue, "Stored value should match what was set")

	// Step 8: Test MemoryGet callback
	getCtx, getCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer getCancel()

	getReq := &proto.MemoryGetRequest{
		Key: testKey,
	}

	getResp, err := client.MemoryGet(getCtx, getReq)
	require.NoError(t, err, "MemoryGet callback should succeed")
	require.NotNil(t, getResp, "MemoryGet response should not be nil")
	assert.True(t, getResp.Found, "MemoryGet should indicate value was found")
	assert.Equal(t, fmt.Sprintf(`"%s"`, testValue), getResp.ValueJson, "Retrieved value should match what was set")

	// Step 9: Test MemoryGet for non-existent key
	getNonExistentCtx, getNonExistentCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer getNonExistentCancel()

	getNonExistentReq := &proto.MemoryGetRequest{
		Key: "non-existent-key",
	}

	getNonExistentResp, err := client.MemoryGet(getNonExistentCtx, getNonExistentReq)
	require.NoError(t, err, "MemoryGet for non-existent key should not error")
	require.NotNil(t, getNonExistentResp, "Response should not be nil")
	assert.False(t, getNonExistentResp.Found, "Non-existent key should not be found")

	t.Log("Integration test completed successfully")
}

// mockMemoryStore implements the MemoryStore interface for testing
type mockMemoryStore struct {
	working  memory.WorkingMemory
	mission  memory.MissionMemory
	longTerm memory.LongTermMemory
}

func (m *mockMemoryStore) Working() memory.WorkingMemory {
	return m.working
}

func (m *mockMemoryStore) Mission() memory.MissionMemory {
	return m.mission
}

func (m *mockMemoryStore) LongTerm() memory.LongTermMemory {
	return m.longTerm
}
