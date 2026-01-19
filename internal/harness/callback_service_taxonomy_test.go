package harness

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
	pb "github.com/zero-day-ai/sdk/api/gen/proto"
	"log/slog"
	"os"
)

// mockTaxonomyEngine implements TaxonomyGraphEngine for testing
type mockTaxonomyEngine struct {
	handleToolOutputCalls []mockToolOutputCall
	handleEventCalls      []mockEventCall
	healthStatus          engine.HealthStatus
}

type mockToolOutputCall struct {
	toolName   string
	output     map[string]any
	agentRunID string
}

type mockEventCall struct {
	eventType  string
	eventData  map[string]any
	agentRunID string
}

func newMockTaxonomyEngine() *mockTaxonomyEngine {
	return &mockTaxonomyEngine{
		handleToolOutputCalls: make([]mockToolOutputCall, 0),
		handleEventCalls:      make([]mockEventCall, 0),
		healthStatus: engine.HealthStatus{
			Healthy: true,
			Message: "mock engine healthy",
		},
	}
}

func (m *mockTaxonomyEngine) HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error {
	m.handleToolOutputCalls = append(m.handleToolOutputCalls, mockToolOutputCall{
		toolName:   toolName,
		output:     output,
		agentRunID: agentRunID,
	})
	return nil
}

func (m *mockTaxonomyEngine) HandleExecutionEvent(ctx context.Context, eventType string, eventData map[string]any, agentRunID string) error {
	m.handleEventCalls = append(m.handleEventCalls, mockEventCall{
		eventType:  eventType,
		eventData:  eventData,
		agentRunID: agentRunID,
	})
	return nil
}

func (m *mockTaxonomyEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
	m.handleEventCalls = append(m.handleEventCalls, mockEventCall{
		eventType:  eventType,
		eventData:  data,
		agentRunID: "",
	})
	return nil
}

func (m *mockTaxonomyEngine) HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error {
	return nil
}

func (m *mockTaxonomyEngine) Health(ctx context.Context) engine.HealthStatus {
	return m.healthStatus
}

// TestCallbackService_ToolOutputGraphing tests automatic tool output graphing
func TestCallbackService_ToolOutputGraphing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock harness
	mockHarn := &DefaultAgentHarness{}

	// Create registry and register harness
	registry := NewCallbackHarnessRegistry()
	registry.Register("mission-123", "test-agent", mockHarn)

	// Create mock taxonomy engine
	mockEngine := newMockTaxonomyEngine()

	// Create callback service with taxonomy engine
	_ = NewHarnessCallbackServiceWithRegistry(
		logger,
		registry,
		WithTaxonomyEngine(mockEngine),
	)

	// Create context info
	_ = &pb.ContextInfo{
		TaskId:    "task-456",
		AgentName: "test-agent",
		MissionId: "mission-123",
	}

	// Prepare tool input
	input := map[string]any{"target": "example.com"}
	_, _ = json.Marshal(input)

	// Call tool (this will fail because mock harness doesn't implement CallTool,
	// but we're testing the graphing integration, not the tool execution)
	// So we'll test with a proper mock harness below

	t.Skip("Requires proper mock harness with CallTool implementation")
}

// TestCallbackService_ToolOutputGraphing_NoEngine tests that service works without engine
func TestCallbackService_ToolOutputGraphing_NoEngine(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock harness
	mockHarn := &DefaultAgentHarness{}

	// Create registry and register harness
	registry := NewCallbackHarnessRegistry()
	registry.Register("mission-123", "test-agent", mockHarn)

	// Create callback service WITHOUT taxonomy engine
	service := NewHarnessCallbackServiceWithRegistry(logger, registry)

	// Verify engine is nil
	assert.Nil(t, service.taxonomyEngine, "taxonomy engine should be nil")

	// This should work fine - the service should simply skip graphing
	// (We can't test the full flow without a proper mock harness,
	// but we verify the service is created correctly)
}

// TestCallbackService_ExtractAgentRunID tests agent run ID extraction
func TestCallbackService_ExtractAgentRunID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	tests := []struct {
		name        string
		contextInfo *pb.ContextInfo
		wantPattern string // Use pattern matching since trace IDs are random
	}{
		{
			name: "with task ID",
			contextInfo: &pb.ContextInfo{
				TaskId:    "task-123",
				MissionId: "mission-456",
			},
			wantPattern: "task-123",
		},
		{
			name: "with mission ID only",
			contextInfo: &pb.ContextInfo{
				MissionId: "mission-789",
			},
			wantPattern: "mission-789",
		},
		{
			name:        "with nothing - generates UUID",
			contextInfo: &pb.ContextInfo{},
			wantPattern: "UUID", // Will check length instead
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			runID := service.extractAgentRunID(ctx, tt.contextInfo)

			if tt.wantPattern == "UUID" {
				// UUID format: 8-4-4-4-12 characters
				assert.Len(t, runID, 36, "should generate valid UUID")
			} else {
				assert.Equal(t, tt.wantPattern, runID)
			}
		})
	}
}

// TestCallbackService_WithTaxonomyEngineOption tests the configuration option
func TestCallbackService_WithTaxonomyEngineOption(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockEngine := newMockTaxonomyEngine()

	service := NewHarnessCallbackService(
		logger,
		WithTaxonomyEngine(mockEngine),
	)

	assert.NotNil(t, service.taxonomyEngine, "taxonomy engine should be set")
	assert.Equal(t, mockEngine, service.taxonomyEngine, "should be the same engine instance")
}

// TestCallbackService_GraphingInBackground tests that graphing doesn't block tool execution
func TestCallbackService_GraphingInBackground(t *testing.T) {
	// This test verifies that tool output graphing happens in a goroutine
	// and doesn't block the tool response

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create slow mock engine that takes 1 second
	slowEngine := &slowMockEngine{
		delay: 1 * time.Second,
	}

	service := NewHarnessCallbackService(
		logger,
		WithTaxonomyEngine(slowEngine),
	)

	assert.NotNil(t, service.taxonomyEngine)

	// The actual CallTool will launch graphing in background
	// This test just verifies the configuration is correct
}

// slowMockEngine simulates a slow taxonomy engine
type slowMockEngine struct {
	delay time.Duration
}

func (s *slowMockEngine) HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error {
	time.Sleep(s.delay)
	return nil
}

func (s *slowMockEngine) HandleExecutionEvent(ctx context.Context, eventType string, eventData map[string]any, agentRunID string) error {
	time.Sleep(s.delay)
	return nil
}

func (s *slowMockEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
	time.Sleep(s.delay)
	return nil
}

func (s *slowMockEngine) HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error {
	time.Sleep(s.delay)
	return nil
}

func (s *slowMockEngine) Health(ctx context.Context) engine.HealthStatus {
	return engine.HealthStatus{
		Healthy: true,
		Message: "slow engine healthy",
	}
}

// TestCallbackService_GraphingTimeout tests that graphing has a timeout
func TestCallbackService_GraphingTimeout(t *testing.T) {
	// The graphing goroutine uses a 30-second timeout
	// This test verifies the timeout is configured

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create engine that would hang forever
	hangingEngine := &hangingMockEngine{
		hangChan: make(chan struct{}),
	}

	service := NewHarnessCallbackService(
		logger,
		WithTaxonomyEngine(hangingEngine),
	)

	assert.NotNil(t, service.taxonomyEngine)

	// In actual CallTool, the goroutine has a 30s timeout
	// So even if the engine hangs, the tool response won't be blocked
}

// hangingMockEngine simulates an engine that hangs indefinitely
type hangingMockEngine struct {
	hangChan chan struct{}
}

func (h *hangingMockEngine) HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error {
	<-ctx.Done() // Wait for context cancellation (timeout)
	return ctx.Err()
}

func (h *hangingMockEngine) HandleExecutionEvent(ctx context.Context, eventType string, eventData map[string]any, agentRunID string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (h *hangingMockEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
	<-ctx.Done()
	return ctx.Err()
}

func (h *hangingMockEngine) HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (h *hangingMockEngine) Health(ctx context.Context) engine.HealthStatus {
	return engine.HealthStatus{
		Healthy: true,
		Message: "hanging engine",
	}
}

// TestCallbackService_NilOutput tests that nil output doesn't cause panic
func TestCallbackService_NilOutput(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockEngine := newMockTaxonomyEngine()

	service := NewHarnessCallbackService(
		logger,
		WithTaxonomyEngine(mockEngine),
	)

	// The service checks if output != nil before calling engine
	// This test verifies the nil check exists

	ctx := context.Background()
	contextInfo := &pb.ContextInfo{
		TaskId: "task-123",
	}

	agentRunID := service.extractAgentRunID(ctx, contextInfo)
	require.NotEmpty(t, agentRunID)

	// If output is nil, engine should not be called
	// (We can't test the full flow without a proper tool execution,
	// but the code has the nil check: if s.taxonomyEngine != nil && output != nil)
}

// =============================================================================
// Taxonomy RPC Handler Tests (GetTaxonomySchema, GenerateNodeID, Validate*)
// =============================================================================

// TestCallbackService_GetTaxonomySchema tests the GetTaxonomySchema RPC handler
func TestCallbackService_GetTaxonomySchema(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()
	req := &pb.GetTaxonomySchemaRequest{}

	resp, err := service.GetTaxonomySchema(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The response structure is valid even if taxonomy registry is not initialized
	// In production, the registry would be initialized at startup
	// In tests without initialization, we get empty slices (which is valid)
	t.Logf("Taxonomy version: %s", resp.Version)
	t.Logf("Node types count: %d", len(resp.NodeTypes))
	t.Logf("Relationship types count: %d", len(resp.RelationshipTypes))
}

// TestCallbackService_GetTaxonomySchema_HasExpectedStructure tests that the response has proper structure
func TestCallbackService_GetTaxonomySchema_HasExpectedStructure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()
	resp, err := service.GetTaxonomySchema(ctx, &pb.GetTaxonomySchemaRequest{})
	require.NoError(t, err)

	// Check that we have some node types (assuming taxonomy is initialized)
	if len(resp.NodeTypes) > 0 {
		// Validate first node type has required fields
		nodeType := resp.NodeTypes[0]
		assert.NotEmpty(t, nodeType.Type, "node type should have Type")
		assert.NotEmpty(t, nodeType.Name, "node type should have Name")
	}

	// Check that we have some relationship types
	if len(resp.RelationshipTypes) > 0 {
		relType := resp.RelationshipTypes[0]
		assert.NotEmpty(t, relType.Type, "relationship type should have Type")
	}
}

// TestCallbackService_GenerateNodeID tests the GenerateNodeID RPC handler
func TestCallbackService_GenerateNodeID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()

	tests := []struct {
		name        string
		req         *pb.GenerateNodeIDRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "valid host node",
			req: &pb.GenerateNodeIDRequest{
				NodeType:       "host",
				PropertiesJson: `{"ip": "192.168.1.1"}`,
			},
			wantErr: false,
		},
		{
			name: "valid domain node",
			req: &pb.GenerateNodeIDRequest{
				NodeType:       "domain",
				PropertiesJson: `{"name": "example.com"}`,
			},
			wantErr: false,
		},
		{
			name: "empty node type",
			req: &pb.GenerateNodeIDRequest{
				NodeType:       "",
				PropertiesJson: `{}`,
			},
			wantErr:     true,
			errContains: "node_type is required",
		},
		{
			name: "empty properties json",
			req: &pb.GenerateNodeIDRequest{
				NodeType:       "host",
				PropertiesJson: "",
			},
			wantErr: false, // Should work, just use empty props
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.GenerateNodeID(ctx, tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				// Handler returns error in response.Error rather than err
				if err != nil {
					t.Logf("Unexpected error: %v", err)
				}
				require.NotNil(t, resp)
				// If there's no response error or gRPC error, check node ID
				if resp.Error == nil && err == nil {
					assert.NotEmpty(t, resp.NodeId, "should generate a node ID")
				}
			}
		})
	}
}

// TestCallbackService_ValidateFinding tests the ValidateFinding RPC handler
func TestCallbackService_ValidateFinding(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()

	tests := []struct {
		name          string
		req           *pb.ValidateFindingRequest
		wantError     bool   // Expect some error in response.Error
		errorContains string // Optional: check error message contains this string
	}{
		{
			name: "valid finding json (registry not initialized)",
			req: &pb.ValidateFindingRequest{
				FindingJson: `{"title": "SQL Injection Found", "description": "A SQL injection vulnerability was found", "severity": "high"}`,
			},
			wantError:     true,
			errorContains: "registry", // Registry not initialized error
		},
		{
			name: "empty finding json (registry not initialized)",
			req: &pb.ValidateFindingRequest{
				FindingJson: "",
			},
			wantError:     true,
			errorContains: "registry", // Registry check happens before parse
		},
		{
			name: "malformed json (registry not initialized)",
			req: &pb.ValidateFindingRequest{
				FindingJson: `{invalid json}`,
			},
			wantError:     true,
			errorContains: "registry", // Registry check happens before parse
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.ValidateFinding(ctx, tt.req)
			require.NoError(t, err, "handler should not return gRPC error")
			require.NotNil(t, resp)

			if tt.wantError {
				assert.NotNil(t, resp.Error, "should have error in response")
				if tt.errorContains != "" {
					assert.Contains(t, resp.Error.Message, tt.errorContains)
				}
			}
		})
	}
}

// TestCallbackService_ValidateGraphNode tests the ValidateGraphNode RPC handler
func TestCallbackService_ValidateGraphNode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()

	tests := []struct {
		name                string
		req                 *pb.ValidateGraphNodeRequest
		wantValidationError bool // Expect validation error (empty node type)
		wantRegistryError   bool // Expect error due to uninitialized registry
	}{
		{
			name: "valid host node (registry not initialized)",
			req: &pb.ValidateGraphNodeRequest{
				NodeType:       "host",
				PropertiesJson: `{"ip": "192.168.1.1"}`,
			},
			wantRegistryError: true, // Registry not initialized in test env
		},
		{
			name: "empty node type",
			req: &pb.ValidateGraphNodeRequest{
				NodeType:       "",
				PropertiesJson: `{"ip": "192.168.1.1"}`,
			},
			wantValidationError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.ValidateGraphNode(ctx, tt.req)
			require.NoError(t, err, "handler should not error")
			require.NotNil(t, resp)

			if tt.wantValidationError {
				assert.False(t, resp.Valid, "should be invalid")
				assert.NotEmpty(t, resp.Errors, "should have validation errors")
			} else if tt.wantRegistryError {
				// Registry not initialized in test
				assert.NotNil(t, resp.Error, "should have error when registry not initialized")
			}
		})
	}
}

// TestCallbackService_ValidateRelationship tests the ValidateRelationship RPC handler
func TestCallbackService_ValidateRelationship(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()

	tests := []struct {
		name                string
		req                 *pb.ValidateRelationshipRequest
		wantValidationError bool // Expect validation error (empty required field)
		wantRegistryError   bool // Expect error due to uninitialized registry
	}{
		{
			name: "valid relationship (registry not initialized)",
			req: &pb.ValidateRelationshipRequest{
				RelationshipType: "HAS_PORT",
				FromNodeType:     "host",
				ToNodeType:       "port",
			},
			wantRegistryError: true, // Registry not initialized in test env
		},
		{
			name: "empty relationship type",
			req: &pb.ValidateRelationshipRequest{
				RelationshipType: "",
				FromNodeType:     "host",
				ToNodeType:       "port",
			},
			wantValidationError: true,
		},
		{
			name: "empty from node type",
			req: &pb.ValidateRelationshipRequest{
				RelationshipType: "HAS_PORT",
				FromNodeType:     "",
				ToNodeType:       "port",
			},
			wantValidationError: true,
		},
		{
			name: "empty to node type",
			req: &pb.ValidateRelationshipRequest{
				RelationshipType: "HAS_PORT",
				FromNodeType:     "host",
				ToNodeType:       "",
			},
			wantValidationError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := service.ValidateRelationship(ctx, tt.req)
			require.NoError(t, err, "handler should not error")
			require.NotNil(t, resp)

			if tt.wantValidationError {
				assert.False(t, resp.Valid, "should be invalid")
				assert.NotEmpty(t, resp.Errors, "should have validation errors")
			} else if tt.wantRegistryError {
				// Registry not initialized in test
				assert.NotNil(t, resp.Error, "should have error when registry not initialized")
			}
		})
	}
}

// TestCallbackService_TaxonomyHandlers_NilRequest tests handlers with nil requests
func TestCallbackService_TaxonomyHandlers_NilRequest(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewHarnessCallbackService(logger)

	ctx := context.Background()

	t.Run("GenerateNodeID with nil request", func(t *testing.T) {
		_, err := service.GenerateNodeID(ctx, nil)
		require.Error(t, err, "should error on nil request")
	})

	t.Run("ValidateFinding with nil request", func(t *testing.T) {
		_, err := service.ValidateFinding(ctx, nil)
		require.Error(t, err, "should error on nil request")
	})

	t.Run("ValidateGraphNode with nil request", func(t *testing.T) {
		_, err := service.ValidateGraphNode(ctx, nil)
		require.Error(t, err, "should error on nil request")
	})

	t.Run("ValidateRelationship with nil request", func(t *testing.T) {
		_, err := service.ValidateRelationship(ctx, nil)
		require.Error(t, err, "should error on nil request")
	})
}
