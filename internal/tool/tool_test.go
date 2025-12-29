package tool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MockTool implements Tool interface for testing
type MockTool struct {
	name         string
	description  string
	version      string
	tags         []string
	inputSchema  schema.JSONSchema
	outputSchema schema.JSONSchema
	executeFn    func(ctx context.Context, input map[string]any) (map[string]any, error)
	healthFn     func(ctx context.Context) types.HealthStatus
}

func NewMockTool(name string) *MockTool {
	return &MockTool{
		name:        name,
		description: fmt.Sprintf("Mock tool: %s", name),
		version:     "1.0.0",
		tags:        []string{"mock", "test"},
		inputSchema: schema.NewObjectSchema(map[string]schema.SchemaField{
			"input": schema.NewStringField("test input"),
		}, []string{"input"}),
		outputSchema: schema.NewObjectSchema(map[string]schema.SchemaField{
			"output": schema.NewStringField("test output"),
		}, []string{"output"}),
		executeFn: func(ctx context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"output": "success"}, nil
		},
		healthFn: func(ctx context.Context) types.HealthStatus {
			return types.Healthy("mock tool is healthy")
		},
	}
}

func (m *MockTool) Name() string                          { return m.name }
func (m *MockTool) Description() string                   { return m.description }
func (m *MockTool) Version() string                       { return m.version }
func (m *MockTool) Tags() []string                        { return m.tags }
func (m *MockTool) InputSchema() schema.JSONSchema        { return m.inputSchema }
func (m *MockTool) OutputSchema() schema.JSONSchema       { return m.outputSchema }
func (m *MockTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	return m.executeFn(ctx, input)
}
func (m *MockTool) Health(ctx context.Context) types.HealthStatus {
	return m.healthFn(ctx)
}

func (m *MockTool) WithTags(tags ...string) *MockTool {
	m.tags = tags
	return m
}

func (m *MockTool) WithExecuteFunc(fn func(ctx context.Context, input map[string]any) (map[string]any, error)) *MockTool {
	m.executeFn = fn
	return m
}

func (m *MockTool) WithHealthFunc(fn func(ctx context.Context) types.HealthStatus) *MockTool {
	m.healthFn = fn
	return m
}

// TestToolRegistry_RegisterInternal tests internal tool registration
func TestToolRegistry_RegisterInternal(t *testing.T) {
	tests := []struct {
		name      string
		tool      Tool
		wantError bool
		errorCode types.ErrorCode
	}{
		{
			name:      "successful registration",
			tool:      NewMockTool("test-tool"),
			wantError: false,
		},
		{
			name:      "nil tool",
			tool:      nil,
			wantError: true,
			errorCode: ErrToolInvalidInput,
		},
		{
			name:      "empty name",
			tool:      NewMockTool(""),
			wantError: true,
			errorCode: ErrToolInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewToolRegistry()
			err := registry.RegisterInternal(tt.tool)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				var gibsonErr *types.GibsonError
				if !errors.As(err, &gibsonErr) {
					t.Errorf("expected GibsonError but got %T", err)
					return
				}
				if gibsonErr.Code != tt.errorCode {
					t.Errorf("expected error code %q but got %q", tt.errorCode, gibsonErr.Code)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestToolRegistry_RegisterDuplicate tests duplicate registration error
func TestToolRegistry_RegisterDuplicate(t *testing.T) {
	registry := NewToolRegistry()
	tool := NewMockTool("duplicate-tool")

	// First registration should succeed
	err := registry.RegisterInternal(tool)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration should fail
	err = registry.RegisterInternal(tool)
	if err == nil {
		t.Fatal("expected error for duplicate registration but got nil")
	}

	var gibsonErr *types.GibsonError
	if !errors.As(err, &gibsonErr) {
		t.Fatalf("expected GibsonError but got %T", err)
	}

	if gibsonErr.Code != ErrToolAlreadyExists {
		t.Errorf("expected error code %q but got %q", ErrToolAlreadyExists, gibsonErr.Code)
	}
}

// TestToolRegistry_Unregister tests tool unregistration
func TestToolRegistry_Unregister(t *testing.T) {
	registry := NewToolRegistry()
	tool := NewMockTool("test-tool")

	// Register tool
	if err := registry.RegisterInternal(tool); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Unregister should succeed
	err := registry.Unregister("test-tool")
	if err != nil {
		t.Errorf("unregister failed: %v", err)
	}

	// Tool should no longer exist
	_, err = registry.Get("test-tool")
	if err == nil {
		t.Error("expected error when getting unregistered tool")
	}

	// Unregister non-existent tool should fail
	err = registry.Unregister("non-existent")
	if err == nil {
		t.Error("expected error when unregistering non-existent tool")
	}

	var gibsonErr *types.GibsonError
	if !errors.As(err, &gibsonErr) {
		t.Fatalf("expected GibsonError but got %T", err)
	}

	if gibsonErr.Code != ErrToolNotFound {
		t.Errorf("expected error code %q but got %q", ErrToolNotFound, gibsonErr.Code)
	}
}

// TestToolRegistry_Get tests tool retrieval
func TestToolRegistry_Get(t *testing.T) {
	registry := NewToolRegistry()
	tool := NewMockTool("test-tool")

	// Get non-existent tool should fail
	_, err := registry.Get("test-tool")
	if err == nil {
		t.Error("expected error when getting non-existent tool")
	}

	// Register tool
	if err := registry.RegisterInternal(tool); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Get existing tool should succeed
	retrieved, err := registry.Get("test-tool")
	if err != nil {
		t.Errorf("get failed: %v", err)
	}

	if retrieved.Name() != "test-tool" {
		t.Errorf("expected tool name %q but got %q", "test-tool", retrieved.Name())
	}
}

// TestToolRegistry_List tests listing all tools
func TestToolRegistry_List(t *testing.T) {
	registry := NewToolRegistry()

	// Empty registry
	descriptors := registry.List()
	if len(descriptors) != 0 {
		t.Errorf("expected 0 tools but got %d", len(descriptors))
	}

	// Register multiple tools
	tool1 := NewMockTool("tool1")
	tool2 := NewMockTool("tool2")
	tool3 := NewMockTool("tool3")

	if err := registry.RegisterInternal(tool1); err != nil {
		t.Fatalf("failed to register tool1: %v", err)
	}
	if err := registry.RegisterInternal(tool2); err != nil {
		t.Fatalf("failed to register tool2: %v", err)
	}
	if err := registry.RegisterInternal(tool3); err != nil {
		t.Fatalf("failed to register tool3: %v", err)
	}

	// List should return all 3 tools
	descriptors = registry.List()
	if len(descriptors) != 3 {
		t.Errorf("expected 3 tools but got %d", len(descriptors))
	}

	// Verify all tools are present
	toolNames := make(map[string]bool)
	for _, desc := range descriptors {
		toolNames[desc.Name] = true
	}

	expectedTools := []string{"tool1", "tool2", "tool3"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q in list but not found", name)
		}
	}
}

// TestToolRegistry_ListByTag tests filtering tools by tag
func TestToolRegistry_ListByTag(t *testing.T) {
	registry := NewToolRegistry()

	// Register tools with different tags
	tool1 := NewMockTool("tool1").WithTags("security", "scanner")
	tool2 := NewMockTool("tool2").WithTags("network", "scanner")
	tool3 := NewMockTool("tool3").WithTags("security", "exploit")

	if err := registry.RegisterInternal(tool1); err != nil {
		t.Fatalf("failed to register tool1: %v", err)
	}
	if err := registry.RegisterInternal(tool2); err != nil {
		t.Fatalf("failed to register tool2: %v", err)
	}
	if err := registry.RegisterInternal(tool3); err != nil {
		t.Fatalf("failed to register tool3: %v", err)
	}

	// Test filtering by "security" tag
	securityTools := registry.ListByTag("security")
	if len(securityTools) != 2 {
		t.Errorf("expected 2 security tools but got %d", len(securityTools))
	}

	// Test filtering by "scanner" tag
	scannerTools := registry.ListByTag("scanner")
	if len(scannerTools) != 2 {
		t.Errorf("expected 2 scanner tools but got %d", len(scannerTools))
	}

	// Test filtering by non-existent tag
	noTools := registry.ListByTag("non-existent")
	if len(noTools) != 0 {
		t.Errorf("expected 0 tools but got %d", len(noTools))
	}
}

// TestToolRegistry_Execute tests tool execution and error handling
func TestToolRegistry_Execute(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		input     map[string]any
		setupFn   func(*DefaultToolRegistry)
		wantError bool
		errorCode types.ErrorCode
	}{
		{
			name:     "successful execution",
			toolName: "test-tool",
			input:    map[string]any{"input": "test"},
			setupFn: func(r *DefaultToolRegistry) {
				tool := NewMockTool("test-tool")
				_ = r.RegisterInternal(tool)
			},
			wantError: false,
		},
		{
			name:      "tool not found",
			toolName:  "non-existent",
			input:     map[string]any{"input": "test"},
			setupFn:   func(r *DefaultToolRegistry) {},
			wantError: true,
			errorCode: ErrToolNotFound,
		},
		{
			name:     "execution failure",
			toolName: "failing-tool",
			input:    map[string]any{"input": "test"},
			setupFn: func(r *DefaultToolRegistry) {
				tool := NewMockTool("failing-tool").WithExecuteFunc(func(ctx context.Context, input map[string]any) (map[string]any, error) {
					return nil, errors.New("execution failed")
				})
				_ = r.RegisterInternal(tool)
			},
			wantError: true,
			errorCode: ErrToolExecutionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewToolRegistry()
			tt.setupFn(registry)

			ctx := context.Background()
			output, err := registry.Execute(ctx, tt.toolName, tt.input)

			if tt.wantError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				var gibsonErr *types.GibsonError
				if !errors.As(err, &gibsonErr) {
					t.Errorf("expected GibsonError but got %T", err)
					return
				}
				if gibsonErr.Code != tt.errorCode {
					t.Errorf("expected error code %q but got %q", tt.errorCode, gibsonErr.Code)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
				if output == nil {
					t.Error("expected output but got nil")
				}
			}
		})
	}
}

// TestToolRegistry_Metrics tests metrics tracking
func TestToolRegistry_Metrics(t *testing.T) {
	registry := NewToolRegistry()
	tool := NewMockTool("test-tool")

	if err := registry.RegisterInternal(tool); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	ctx := context.Background()
	input := map[string]any{"input": "test"}

	// Execute tool multiple times
	for i := 0; i < 5; i++ {
		_, err := registry.Execute(ctx, "test-tool", input)
		if err != nil {
			t.Fatalf("execution %d failed: %v", i, err)
		}
	}

	// Check metrics
	metrics, err := registry.Metrics("test-tool")
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}

	if metrics.TotalCalls != 5 {
		t.Errorf("expected 5 total calls but got %d", metrics.TotalCalls)
	}

	if metrics.SuccessCalls != 5 {
		t.Errorf("expected 5 success calls but got %d", metrics.SuccessCalls)
	}

	if metrics.FailedCalls != 0 {
		t.Errorf("expected 0 failed calls but got %d", metrics.FailedCalls)
	}

	if metrics.LastExecutedAt == nil {
		t.Error("expected LastExecutedAt to be set")
	}

	// Test failed execution
	failingTool := NewMockTool("failing-tool").WithExecuteFunc(func(ctx context.Context, input map[string]any) (map[string]any, error) {
		return nil, errors.New("execution failed")
	})

	if err := registry.RegisterInternal(failingTool); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	_, _ = registry.Execute(ctx, "failing-tool", input)

	metrics, err = registry.Metrics("failing-tool")
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}

	if metrics.TotalCalls != 1 {
		t.Errorf("expected 1 total call but got %d", metrics.TotalCalls)
	}

	if metrics.FailedCalls != 1 {
		t.Errorf("expected 1 failed call but got %d", metrics.FailedCalls)
	}
}

// TestToolRegistry_Health tests health status aggregation
func TestToolRegistry_Health(t *testing.T) {
	tests := []struct {
		name          string
		setupFn       func(*DefaultToolRegistry)
		expectedState types.HealthState
	}{
		{
			name: "all tools healthy",
			setupFn: func(r *DefaultToolRegistry) {
				tool1 := NewMockTool("tool1")
				tool2 := NewMockTool("tool2")
				_ = r.RegisterInternal(tool1)
				_ = r.RegisterInternal(tool2)
			},
			expectedState: types.HealthStateHealthy,
		},
		{
			name: "some tools unhealthy",
			setupFn: func(r *DefaultToolRegistry) {
				tool1 := NewMockTool("tool1")
				tool2 := NewMockTool("tool2").WithHealthFunc(func(ctx context.Context) types.HealthStatus {
					return types.Unhealthy("tool is down")
				})
				_ = r.RegisterInternal(tool1)
				_ = r.RegisterInternal(tool2)
			},
			expectedState: types.HealthStateDegraded,
		},
		{
			name: "all tools unhealthy",
			setupFn: func(r *DefaultToolRegistry) {
				tool1 := NewMockTool("tool1").WithHealthFunc(func(ctx context.Context) types.HealthStatus {
					return types.Unhealthy("tool1 is down")
				})
				tool2 := NewMockTool("tool2").WithHealthFunc(func(ctx context.Context) types.HealthStatus {
					return types.Unhealthy("tool2 is down")
				})
				_ = r.RegisterInternal(tool1)
				_ = r.RegisterInternal(tool2)
			},
			expectedState: types.HealthStateUnhealthy,
		},
		{
			name: "no tools registered",
			setupFn: func(r *DefaultToolRegistry) {
				// Empty registry
			},
			expectedState: types.HealthStateUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewToolRegistry()
			tt.setupFn(registry)

			ctx := context.Background()
			health := registry.Health(ctx)

			if health.State != tt.expectedState {
				t.Errorf("expected health state %q but got %q", tt.expectedState, health.State)
			}
		})
	}
}

// TestToolRegistry_ToolHealth tests individual tool health checks
func TestToolRegistry_ToolHealth(t *testing.T) {
	registry := NewToolRegistry()

	healthyTool := NewMockTool("healthy-tool")
	unhealthyTool := NewMockTool("unhealthy-tool").WithHealthFunc(func(ctx context.Context) types.HealthStatus {
		return types.Unhealthy("tool is down")
	})

	_ = registry.RegisterInternal(healthyTool)
	_ = registry.RegisterInternal(unhealthyTool)

	ctx := context.Background()

	// Check healthy tool
	health := registry.ToolHealth(ctx, "healthy-tool")
	if !health.IsHealthy() {
		t.Errorf("expected healthy tool to be healthy but got %q", health.State)
	}

	// Check unhealthy tool
	health = registry.ToolHealth(ctx, "unhealthy-tool")
	if !health.IsUnhealthy() {
		t.Errorf("expected unhealthy tool to be unhealthy but got %q", health.State)
	}

	// Check non-existent tool
	health = registry.ToolHealth(ctx, "non-existent")
	if !health.IsUnhealthy() {
		t.Errorf("expected non-existent tool to be unhealthy but got %q", health.State)
	}
}

// TestToolRegistry_ConcurrentAccess tests thread safety
func TestToolRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewToolRegistry()

	// Register initial tools
	for i := 0; i < 10; i++ {
		tool := NewMockTool(fmt.Sprintf("tool-%d", i))
		if err := registry.RegisterInternal(tool); err != nil {
			t.Fatalf("failed to register tool-%d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			toolName := fmt.Sprintf("tool-%d", idx%10)
			_, _ = registry.Get(toolName)
			_ = registry.List()
			_ = registry.ListByTag("mock")
			_, _ = registry.Metrics(toolName)
			_ = registry.ToolHealth(ctx, toolName)
		}(i)
	}

	// Concurrent writes (registrations and unregistrations)
	for i := 10; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			toolName := fmt.Sprintf("concurrent-tool-%d", idx)
			tool := NewMockTool(toolName)
			_ = registry.RegisterInternal(tool)
			time.Sleep(10 * time.Millisecond)
			_ = registry.Unregister(toolName)
		}(i)
	}

	// Concurrent executions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			toolName := fmt.Sprintf("tool-%d", idx%10)
			input := map[string]any{"input": fmt.Sprintf("test-%d", idx)}
			_, _ = registry.Execute(ctx, toolName, input)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify registry is still functional
	tools := registry.List()
	if len(tools) < 10 {
		t.Errorf("expected at least 10 tools after concurrent access, got %d", len(tools))
	}

	// Verify metrics are consistent
	for i := 0; i < 10; i++ {
		metrics, err := registry.Metrics(fmt.Sprintf("tool-%d", i))
		if err != nil {
			t.Errorf("failed to get metrics for tool-%d: %v", i, err)
			continue
		}
		if metrics.TotalCalls == 0 {
			t.Errorf("expected non-zero total calls for tool-%d", i)
		}
		if metrics.SuccessCalls+metrics.FailedCalls != metrics.TotalCalls {
			t.Errorf("metrics inconsistent for tool-%d: success(%d) + failed(%d) != total(%d)",
				i, metrics.SuccessCalls, metrics.FailedCalls, metrics.TotalCalls)
		}
	}
}

// TestToolMetrics_RecordSuccess tests success recording
func TestToolMetrics_RecordSuccess(t *testing.T) {
	metrics := NewToolMetrics()

	if metrics.TotalCalls != 0 {
		t.Errorf("expected 0 total calls but got %d", metrics.TotalCalls)
	}

	duration := 100 * time.Millisecond
	metrics.RecordSuccess(duration)

	if metrics.TotalCalls != 1 {
		t.Errorf("expected 1 total call but got %d", metrics.TotalCalls)
	}

	if metrics.SuccessCalls != 1 {
		t.Errorf("expected 1 success call but got %d", metrics.SuccessCalls)
	}

	if metrics.FailedCalls != 0 {
		t.Errorf("expected 0 failed calls but got %d", metrics.FailedCalls)
	}

	if metrics.TotalDuration != duration {
		t.Errorf("expected total duration %v but got %v", duration, metrics.TotalDuration)
	}

	if metrics.AvgDuration != duration {
		t.Errorf("expected avg duration %v but got %v", duration, metrics.AvgDuration)
	}

	if metrics.LastExecutedAt == nil {
		t.Error("expected LastExecutedAt to be set")
	}
}

// TestToolMetrics_RecordFailure tests failure recording
func TestToolMetrics_RecordFailure(t *testing.T) {
	metrics := NewToolMetrics()

	duration := 50 * time.Millisecond
	metrics.RecordFailure(duration)

	if metrics.TotalCalls != 1 {
		t.Errorf("expected 1 total call but got %d", metrics.TotalCalls)
	}

	if metrics.SuccessCalls != 0 {
		t.Errorf("expected 0 success calls but got %d", metrics.SuccessCalls)
	}

	if metrics.FailedCalls != 1 {
		t.Errorf("expected 1 failed call but got %d", metrics.FailedCalls)
	}
}

// TestToolMetrics_Rates tests success and failure rate calculations
func TestToolMetrics_Rates(t *testing.T) {
	metrics := NewToolMetrics()

	// No calls - rates should be 0
	if metrics.SuccessRate() != 0.0 {
		t.Errorf("expected 0.0 success rate but got %f", metrics.SuccessRate())
	}

	if metrics.FailureRate() != 0.0 {
		t.Errorf("expected 0.0 failure rate but got %f", metrics.FailureRate())
	}

	// 3 successes, 2 failures
	metrics.RecordSuccess(10 * time.Millisecond)
	metrics.RecordSuccess(10 * time.Millisecond)
	metrics.RecordSuccess(10 * time.Millisecond)
	metrics.RecordFailure(10 * time.Millisecond)
	metrics.RecordFailure(10 * time.Millisecond)

	expectedSuccessRate := 0.6 // 3/5
	if metrics.SuccessRate() != expectedSuccessRate {
		t.Errorf("expected success rate %f but got %f", expectedSuccessRate, metrics.SuccessRate())
	}

	expectedFailureRate := 0.4 // 2/5
	if metrics.FailureRate() != expectedFailureRate {
		t.Errorf("expected failure rate %f but got %f", expectedFailureRate, metrics.FailureRate())
	}
}

// TestNewToolDescriptor tests descriptor creation
func TestNewToolDescriptor(t *testing.T) {
	tool := NewMockTool("test-tool")
	descriptor := NewToolDescriptor(tool)

	if descriptor.Name != "test-tool" {
		t.Errorf("expected name %q but got %q", "test-tool", descriptor.Name)
	}

	if descriptor.IsExternal {
		t.Error("expected IsExternal to be false for internal tool")
	}

	if descriptor.Version != "1.0.0" {
		t.Errorf("expected version %q but got %q", "1.0.0", descriptor.Version)
	}

	// Test external descriptor
	externalDesc := NewExternalToolDescriptor(tool)
	if !externalDesc.IsExternal {
		t.Error("expected IsExternal to be true for external tool")
	}
}

// TestToolRegistry_ExternalTool tests external tool registration
func TestToolRegistry_ExternalTool(t *testing.T) {
	registry := NewToolRegistry()

	// Create an external tool client
	client := NewGRPCToolClient("external-tool")

	// Register external tool
	err := registry.RegisterExternal("external-tool", client)
	if err != nil {
		t.Fatalf("failed to register external tool: %v", err)
	}

	// Verify it can be retrieved
	tool, err := registry.Get("external-tool")
	if err != nil {
		t.Fatalf("failed to get external tool: %v", err)
	}

	if tool.Name() != "external-tool" {
		t.Errorf("expected name %q but got %q", "external-tool", tool.Name())
	}

	// Verify it appears in the list
	descriptors := registry.List()
	found := false
	for _, desc := range descriptors {
		if desc.Name == "external-tool" && desc.IsExternal {
			found = true
			break
		}
	}

	if !found {
		t.Error("external tool not found in list or not marked as external")
	}
}

// TestToolRegistry_MixedTools tests mixing internal and external tools
func TestToolRegistry_MixedTools(t *testing.T) {
	registry := NewToolRegistry()

	// Register internal tools
	internal1 := NewMockTool("internal-1")
	internal2 := NewMockTool("internal-2")

	if err := registry.RegisterInternal(internal1); err != nil {
		t.Fatalf("failed to register internal-1: %v", err)
	}
	if err := registry.RegisterInternal(internal2); err != nil {
		t.Fatalf("failed to register internal-2: %v", err)
	}

	// Register external tools
	external1 := NewGRPCToolClient("external-1")
	external2 := NewGRPCToolClient("external-2")

	if err := registry.RegisterExternal("external-1", external1); err != nil {
		t.Fatalf("failed to register external-1: %v", err)
	}
	if err := registry.RegisterExternal("external-2", external2); err != nil {
		t.Fatalf("failed to register external-2: %v", err)
	}

	// Verify list contains all tools
	descriptors := registry.List()
	if len(descriptors) != 4 {
		t.Errorf("expected 4 tools but got %d", len(descriptors))
	}

	// Count internal and external
	internalCount := 0
	externalCount := 0
	for _, desc := range descriptors {
		if desc.IsExternal {
			externalCount++
		} else {
			internalCount++
		}
	}

	if internalCount != 2 {
		t.Errorf("expected 2 internal tools but got %d", internalCount)
	}

	if externalCount != 2 {
		t.Errorf("expected 2 external tools but got %d", externalCount)
	}
}

// TestContainsTag tests the containsTag helper function
func TestContainsTag(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		tag      string
		expected bool
	}{
		{
			name:     "tag exists",
			tags:     []string{"security", "scanner", "network"},
			tag:      "scanner",
			expected: true,
		},
		{
			name:     "tag does not exist",
			tags:     []string{"security", "scanner", "network"},
			tag:      "exploit",
			expected: false,
		},
		{
			name:     "empty tags",
			tags:     []string{},
			tag:      "scanner",
			expected: false,
		},
		{
			name:     "nil tags",
			tags:     nil,
			tag:      "scanner",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsTag(tt.tags, tt.tag)
			if result != tt.expected {
				t.Errorf("expected %v but got %v", tt.expected, result)
			}
		})
	}
}
