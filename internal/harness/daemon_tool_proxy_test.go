package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/daemon/toolexec"
	"github.com/zero-day-ai/sdk/schema"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestDirectToolProxy_ExecuteProto verifies that DirectToolProxy correctly implements ExecuteProto
func TestDirectToolProxy_ExecuteProto(t *testing.T) {
	// Create a mock tool executor service
	mockService := &mockToolExecutorService{
		tools: map[string]mockToolResult{
			"ping": {
				output: map[string]any{
					"status":  "success",
					"latency": float64(42),
				},
				err: nil,
			},
		},
	}

	// Create a DirectToolProxy
	proxy := NewDirectToolProxy(
		mockService,
		toolexec.ToolDescriptor{
			Name:        "ping",
			Description: "Network ping tool",
			Version:     "1.0.0",
			Tags:        []string{"network"},
			Status:      "ready",
		},
		&toolexec.ToolSchema{
			InputSchema:  schema.JSON{},
			OutputSchema: schema.JSON{},
		},
		5*time.Minute,
	)

	// Verify proto types are set correctly
	assert.Equal(t, "google.protobuf.Struct", proxy.InputMessageType())
	assert.Equal(t, "google.protobuf.Struct", proxy.OutputMessageType())

	// Create input proto message
	inputMap := map[string]any{
		"target": "example.com",
		"port":   float64(80),
	}
	inputStruct, err := structpb.NewStruct(inputMap)
	require.NoError(t, err)

	// Execute tool using proto
	ctx := context.Background()
	outputProto, err := proxy.ExecuteProto(ctx, inputStruct)
	require.NoError(t, err)
	require.NotNil(t, outputProto)

	// Verify output is correct type
	outputStruct, ok := outputProto.(*structpb.Struct)
	require.True(t, ok, "output should be *structpb.Struct")

	// Verify output content
	outputMap := outputStruct.AsMap()
	assert.Equal(t, "success", outputMap["status"])
	assert.Equal(t, float64(42), outputMap["latency"])
}

// TestDirectToolProxy_ExecuteProto_InvalidInput verifies error handling for invalid input
func TestDirectToolProxy_ExecuteProto_InvalidInput(t *testing.T) {
	mockService := &mockToolExecutorService{
		tools: map[string]mockToolResult{},
	}

	proxy := NewDirectToolProxy(
		mockService,
		toolexec.ToolDescriptor{
			Name:   "ping",
			Status: "ready",
		},
		nil,
		5*time.Minute,
	)

	// Try to execute with wrong proto type (not *structpb.Struct)
	ctx := context.Background()
	wrongInput := &structpb.Value{
		Kind: &structpb.Value_StringValue{StringValue: "invalid"},
	}

	_, err := proxy.ExecuteProto(ctx, wrongInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected *structpb.Struct")
}

// TestDirectToolProxy_Execute verifies the Execute method works
func TestDirectToolProxy_Execute(t *testing.T) {
	mockService := &mockToolExecutorService{
		tools: map[string]mockToolResult{
			"scan": {
				output: map[string]any{
					"hosts_found": float64(5),
					"duration":    float64(120),
				},
				err: nil,
			},
		},
	}

	proxy := NewDirectToolProxy(
		mockService,
		toolexec.ToolDescriptor{
			Name:        "scan",
			Description: "Network scanner",
			Version:     "2.0.0",
			Tags:        []string{"network", "recon"},
			Status:      "ready",
		},
		nil,
		5*time.Minute,
	)

	input := map[string]any{
		"target":  "192.168.1.0/24",
		"timeout": float64(300),
	}

	ctx := context.Background()
	output, err := proxy.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.Equal(t, float64(5), output["hosts_found"])
	assert.Equal(t, float64(120), output["duration"])
}

// mockToolExecutorService is a mock implementation for testing
type mockToolExecutorService struct {
	tools map[string]mockToolResult
}

type mockToolResult struct {
	output map[string]any
	err    error
}

func (m *mockToolExecutorService) Execute(ctx context.Context, toolName string, input map[string]any, timeout time.Duration) (map[string]any, error) {
	result, ok := m.tools[toolName]
	if !ok {
		return nil, assert.AnError
	}
	return result.output, result.err
}

func (m *mockToolExecutorService) ListTools() []toolexec.ToolDescriptor {
	descriptors := make([]toolexec.ToolDescriptor, 0, len(m.tools))
	for name := range m.tools {
		descriptors = append(descriptors, toolexec.ToolDescriptor{
			Name:   name,
			Status: "ready",
		})
	}
	return descriptors
}

func (m *mockToolExecutorService) GetToolSchema(toolName string) (*toolexec.ToolSchema, error) {
	if _, ok := m.tools[toolName]; !ok {
		return nil, assert.AnError
	}
	return &toolexec.ToolSchema{}, nil
}

func (m *mockToolExecutorService) Start(ctx context.Context) error {
	return nil
}

func (m *mockToolExecutorService) Stop(ctx context.Context) error {
	return nil
}

func (m *mockToolExecutorService) RefreshTools(ctx context.Context) error {
	return nil
}
