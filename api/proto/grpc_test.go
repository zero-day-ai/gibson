package proto_test

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/zero-day-ai/gibson/api/gen/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// Mock ToolService implementation
type mockToolService struct {
	pb.UnimplementedToolServiceServer
	executeFunc      func(context.Context, *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error)
	getDescriptorFunc func(context.Context, *pb.ToolGetDescriptorRequest) (*pb.ToolDescriptor, error)
	healthFunc       func(context.Context, *pb.ToolHealthRequest) (*pb.HealthStatus, error)
}

func (m *mockToolService) Execute(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req)
	}
	return &pb.ToolExecuteResponse{
		OutputJson: `{"result":"success"}`,
	}, nil
}

func (m *mockToolService) GetDescriptor(ctx context.Context, req *pb.ToolGetDescriptorRequest) (*pb.ToolDescriptor, error) {
	if m.getDescriptorFunc != nil {
		return m.getDescriptorFunc(ctx, req)
	}
	return &pb.ToolDescriptor{
		Name:        "test-tool",
		Description: "A test tool",
		Version:     "1.0.0",
		Tags:        []string{"test", "mock"},
		InputSchema: &pb.JSONSchema{
			Json: `{"type":"object"}`,
		},
		OutputSchema: &pb.JSONSchema{
			Json: `{"type":"object"}`,
		},
	}, nil
}

func (m *mockToolService) Health(ctx context.Context, req *pb.ToolHealthRequest) (*pb.HealthStatus, error) {
	if m.healthFunc != nil {
		return m.healthFunc(ctx, req)
	}
	return &pb.HealthStatus{
		State:     "healthy",
		Message:   "All systems operational",
		CheckedAt: time.Now().UnixMilli(),
	}, nil
}

// Mock PluginService implementation
type mockPluginService struct {
	pb.UnimplementedPluginServiceServer
	queryFunc       func(context.Context, *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error)
	initializeFunc  func(context.Context, *pb.PluginInitializeRequest) (*pb.PluginInitializeResponse, error)
	shutdownFunc    func(context.Context, *pb.PluginShutdownRequest) (*pb.PluginShutdownResponse, error)
	listMethodsFunc func(context.Context, *pb.PluginListMethodsRequest) (*pb.PluginListMethodsResponse, error)
	healthFunc      func(context.Context, *pb.PluginHealthRequest) (*pb.HealthStatus, error)
}

func (m *mockPluginService) Query(ctx context.Context, req *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, req)
	}
	return &pb.PluginQueryResponse{
		ResultJson: `{"data":"result"}`,
	}, nil
}

func (m *mockPluginService) Initialize(ctx context.Context, req *pb.PluginInitializeRequest) (*pb.PluginInitializeResponse, error) {
	if m.initializeFunc != nil {
		return m.initializeFunc(ctx, req)
	}
	return &pb.PluginInitializeResponse{}, nil
}

func (m *mockPluginService) Shutdown(ctx context.Context, req *pb.PluginShutdownRequest) (*pb.PluginShutdownResponse, error) {
	if m.shutdownFunc != nil {
		return m.shutdownFunc(ctx, req)
	}
	return &pb.PluginShutdownResponse{}, nil
}

func (m *mockPluginService) ListMethods(ctx context.Context, req *pb.PluginListMethodsRequest) (*pb.PluginListMethodsResponse, error) {
	if m.listMethodsFunc != nil {
		return m.listMethodsFunc(ctx, req)
	}
	return &pb.PluginListMethodsResponse{
		Methods: []*pb.PluginMethodDescriptor{
			{
				Name:        "testMethod",
				Description: "A test method",
				InputSchema: &pb.JSONSchema{
					Json: `{"type":"object"}`,
				},
				OutputSchema: &pb.JSONSchema{
					Json: `{"type":"object"}`,
				},
			},
		},
	}, nil
}

func (m *mockPluginService) Health(ctx context.Context, req *pb.PluginHealthRequest) (*pb.HealthStatus, error) {
	if m.healthFunc != nil {
		return m.healthFunc(ctx, req)
	}
	return &pb.HealthStatus{
		State:     "healthy",
		Message:   "Plugin operational",
		CheckedAt: time.Now().UnixMilli(),
	}, nil
}

// Mock AgentService implementation
type mockAgentService struct {
	pb.UnimplementedAgentServiceServer
	executeFunc         func(context.Context, *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error)
	getDescriptorFunc   func(context.Context, *pb.AgentGetDescriptorRequest) (*pb.AgentDescriptor, error)
	getSlotSchemaFunc   func(context.Context, *pb.AgentGetSlotSchemaRequest) (*pb.AgentGetSlotSchemaResponse, error)
	healthFunc          func(context.Context, *pb.AgentHealthRequest) (*pb.HealthStatus, error)
}

func (m *mockAgentService) Execute(ctx context.Context, req *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req)
	}
	return &pb.AgentExecuteResponse{
		ResultJson: `{"status":"completed"}`,
	}, nil
}

func (m *mockAgentService) GetDescriptor(ctx context.Context, req *pb.AgentGetDescriptorRequest) (*pb.AgentDescriptor, error) {
	if m.getDescriptorFunc != nil {
		return m.getDescriptorFunc(ctx, req)
	}
	return &pb.AgentDescriptor{
		Name:           "test-agent",
		Version:        "1.0.0",
		Description:    "A test agent",
		Capabilities:   []string{"execute", "analyze"},
		TargetTypes:    []string{"system", "network"},
		TechniqueTypes: []string{"recon", "exploit"},
	}, nil
}

func (m *mockAgentService) GetSlotSchema(ctx context.Context, req *pb.AgentGetSlotSchemaRequest) (*pb.AgentGetSlotSchemaResponse, error) {
	if m.getSlotSchemaFunc != nil {
		return m.getSlotSchemaFunc(ctx, req)
	}
	return &pb.AgentGetSlotSchemaResponse{
		Slots: []*pb.AgentSlotDefinition{
			{
				Name:        "primary",
				Description: "Primary slot",
				Required:    true,
				DefaultConfig: &pb.AgentSlotConfig{
					Provider:    "openai",
					Model:       "gpt-4",
					Temperature: 0.7,
					MaxTokens:   4096,
				},
				Constraints: &pb.AgentSlotConstraints{
					MinContextWindow:  8192,
					RequiredFeatures: []string{"function-calling"},
				},
			},
		},
	}, nil
}

func (m *mockAgentService) Health(ctx context.Context, req *pb.AgentHealthRequest) (*pb.HealthStatus, error) {
	if m.healthFunc != nil {
		return m.healthFunc(ctx, req)
	}
	return &pb.HealthStatus{
		State:     "healthy",
		Message:   "Agent ready",
		CheckedAt: time.Now().UnixMilli(),
	}, nil
}

// Helper function to create a bufconn gRPC server and client
func setupToolService(t *testing.T, srv pb.ToolServiceServer) (pb.ToolServiceClient, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterToolServiceServer(s, srv)

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		s.Stop()
		lis.Close()
	}

	return pb.NewToolServiceClient(conn), cleanup
}

func setupPluginService(t *testing.T, srv pb.PluginServiceServer) (pb.PluginServiceClient, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterPluginServiceServer(s, srv)

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		s.Stop()
		lis.Close()
	}

	return pb.NewPluginServiceClient(conn), cleanup
}

func setupAgentService(t *testing.T, srv pb.AgentServiceServer) (pb.AgentServiceClient, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterAgentServiceServer(s, srv)

	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		s.Stop()
		lis.Close()
	}

	return pb.NewAgentServiceClient(conn), cleanup
}

// ToolService Tests

func TestToolService_Execute_Success(t *testing.T) {
	mock := &mockToolService{}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ToolExecuteRequest{
		InputJson: `{"command":"test"}`,
		TimeoutMs: 5000,
	}

	resp, err := client.Execute(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, `{"result":"success"}`, resp.OutputJson)
	assert.Nil(t, resp.Error)
}

func TestToolService_Execute_WithError(t *testing.T) {
	mock := &mockToolService{
		executeFunc: func(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
			return &pb.ToolExecuteResponse{
				Error: &pb.Error{
					Code:      "EXECUTION_FAILED",
					Message:   "Tool execution failed",
					Retryable: true,
				},
			}, nil
		},
	}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ToolExecuteRequest{
		InputJson: `{"command":"fail"}`,
		TimeoutMs: 5000,
	}

	resp, err := client.Execute(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "EXECUTION_FAILED", resp.Error.Code)
	assert.Equal(t, "Tool execution failed", resp.Error.Message)
	assert.True(t, resp.Error.Retryable)
}

func TestToolService_Execute_GRPCError(t *testing.T) {
	mock := &mockToolService{
		executeFunc: func(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
			return nil, status.Error(codes.InvalidArgument, "invalid input JSON")
		},
	}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ToolExecuteRequest{
		InputJson: `invalid json`,
		TimeoutMs: 5000,
	}

	resp, err := client.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "invalid input JSON")
}

func TestToolService_Execute_ContextCancellation(t *testing.T) {
	mock := &mockToolService{
		executeFunc: func(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
			// Simulate long-running operation
			select {
			case <-ctx.Done():
				// Return the actual context error
				if ctx.Err() == context.Canceled {
					return nil, status.Error(codes.Canceled, "request canceled")
				}
				return nil, status.Error(codes.DeadlineExceeded, "deadline exceeded")
			case <-time.After(5 * time.Second):
				return &pb.ToolExecuteResponse{OutputJson: `{"result":"success"}`}, nil
			}
		},
	}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &pb.ToolExecuteRequest{
		InputJson: `{"command":"slow"}`,
		TimeoutMs: 10000,
	}

	resp, err := client.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	// Context timeout results in DeadlineExceeded, not Canceled
	assert.Equal(t, codes.DeadlineExceeded, st.Code())
}

func TestToolService_GetDescriptor_Success(t *testing.T) {
	mock := &mockToolService{}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ToolGetDescriptorRequest{}

	resp, err := client.GetDescriptor(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-tool", resp.Name)
	assert.Equal(t, "A test tool", resp.Description)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Equal(t, []string{"test", "mock"}, resp.Tags)
	assert.NotNil(t, resp.InputSchema)
	assert.NotNil(t, resp.OutputSchema)
}

func TestToolService_Health_Success(t *testing.T) {
	mock := &mockToolService{}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ToolHealthRequest{}

	resp, err := client.Health(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "healthy", resp.State)
	assert.Equal(t, "All systems operational", resp.Message)
	assert.Greater(t, resp.CheckedAt, int64(0))
}

func TestToolService_Health_Degraded(t *testing.T) {
	mock := &mockToolService{
		healthFunc: func(ctx context.Context, req *pb.ToolHealthRequest) (*pb.HealthStatus, error) {
			return &pb.HealthStatus{
				State:     "degraded",
				Message:   "High memory usage",
				CheckedAt: time.Now().UnixMilli(),
			}, nil
		},
	}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ToolHealthRequest{}

	resp, err := client.Health(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "degraded", resp.State)
	assert.Equal(t, "High memory usage", resp.Message)
}

// PluginService Tests

func TestPluginService_Query_Success(t *testing.T) {
	mock := &mockPluginService{}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginQueryRequest{
		Method:     "getData",
		ParamsJson: `{"id":"123"}`,
		TimeoutMs:  5000,
	}

	resp, err := client.Query(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, `{"data":"result"}`, resp.ResultJson)
	assert.Nil(t, resp.Error)
}

func TestPluginService_Query_WithError(t *testing.T) {
	mock := &mockPluginService{
		queryFunc: func(ctx context.Context, req *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error) {
			return &pb.PluginQueryResponse{
				Error: &pb.Error{
					Code:      "METHOD_NOT_FOUND",
					Message:   "Method not found: invalidMethod",
					Retryable: false,
				},
			}, nil
		},
	}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginQueryRequest{
		Method:     "invalidMethod",
		ParamsJson: `{}`,
		TimeoutMs:  5000,
	}

	resp, err := client.Query(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "METHOD_NOT_FOUND", resp.Error.Code)
	assert.Equal(t, "Method not found: invalidMethod", resp.Error.Message)
	assert.False(t, resp.Error.Retryable)
}

func TestPluginService_Query_GRPCError(t *testing.T) {
	mock := &mockPluginService{
		queryFunc: func(ctx context.Context, req *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error) {
			return nil, status.Error(codes.Unavailable, "plugin service unavailable")
		},
	}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginQueryRequest{
		Method:     "getData",
		ParamsJson: `{"id":"123"}`,
		TimeoutMs:  5000,
	}

	resp, err := client.Query(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Contains(t, st.Message(), "plugin service unavailable")
}

func TestPluginService_Initialize_Success(t *testing.T) {
	mock := &mockPluginService{}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginInitializeRequest{
		ConfigJson: `{"setting":"value"}`,
	}

	resp, err := client.Initialize(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Nil(t, resp.Error)
}

func TestPluginService_Initialize_WithError(t *testing.T) {
	mock := &mockPluginService{
		initializeFunc: func(ctx context.Context, req *pb.PluginInitializeRequest) (*pb.PluginInitializeResponse, error) {
			return &pb.PluginInitializeResponse{
				Error: &pb.Error{
					Code:      "INVALID_CONFIG",
					Message:   "Invalid configuration format",
					Retryable: false,
				},
			}, nil
		},
	}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginInitializeRequest{
		ConfigJson: `invalid config`,
	}

	resp, err := client.Initialize(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "INVALID_CONFIG", resp.Error.Code)
}

func TestPluginService_Shutdown_Success(t *testing.T) {
	mock := &mockPluginService{}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginShutdownRequest{}

	resp, err := client.Shutdown(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Nil(t, resp.Error)
}

func TestPluginService_ListMethods_Success(t *testing.T) {
	mock := &mockPluginService{}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginListMethodsRequest{}

	resp, err := client.ListMethods(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Methods, 1)
	assert.Equal(t, "testMethod", resp.Methods[0].Name)
	assert.Equal(t, "A test method", resp.Methods[0].Description)
	assert.NotNil(t, resp.Methods[0].InputSchema)
	assert.NotNil(t, resp.Methods[0].OutputSchema)
}

func TestPluginService_Health_Success(t *testing.T) {
	mock := &mockPluginService{}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.PluginHealthRequest{}

	resp, err := client.Health(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "healthy", resp.State)
	assert.Equal(t, "Plugin operational", resp.Message)
	assert.Greater(t, resp.CheckedAt, int64(0))
}

// AgentService Tests

func TestAgentService_Execute_Success(t *testing.T) {
	mock := &mockAgentService{}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentExecuteRequest{
		TaskJson:  `{"objective":"scan network"}`,
		TimeoutMs: 10000,
	}

	resp, err := client.Execute(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, `{"status":"completed"}`, resp.ResultJson)
	assert.Nil(t, resp.Error)
}

func TestAgentService_Execute_WithError(t *testing.T) {
	mock := &mockAgentService{
		executeFunc: func(ctx context.Context, req *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error) {
			return &pb.AgentExecuteResponse{
				Error: &pb.Error{
					Code:      "TASK_FAILED",
					Message:   "Task execution failed: insufficient permissions",
					Retryable: false,
				},
			}, nil
		},
	}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentExecuteRequest{
		TaskJson:  `{"objective":"restricted operation"}`,
		TimeoutMs: 10000,
	}

	resp, err := client.Execute(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "TASK_FAILED", resp.Error.Code)
	assert.Equal(t, "Task execution failed: insufficient permissions", resp.Error.Message)
	assert.False(t, resp.Error.Retryable)
}

func TestAgentService_Execute_GRPCError(t *testing.T) {
	mock := &mockAgentService{
		executeFunc: func(ctx context.Context, req *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error) {
			return nil, status.Error(codes.ResourceExhausted, "agent overloaded")
		},
	}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentExecuteRequest{
		TaskJson:  `{"objective":"test"}`,
		TimeoutMs: 10000,
	}

	resp, err := client.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Contains(t, st.Message(), "agent overloaded")
}

func TestAgentService_Execute_ContextDeadline(t *testing.T) {
	mock := &mockAgentService{
		executeFunc: func(ctx context.Context, req *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error) {
			// Simulate long-running task
			select {
			case <-ctx.Done():
				return nil, status.Error(codes.DeadlineExceeded, "execution timeout")
			case <-time.After(10 * time.Second):
				return &pb.AgentExecuteResponse{ResultJson: `{"status":"completed"}`}, nil
			}
		},
	}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := &pb.AgentExecuteRequest{
		TaskJson:  `{"objective":"long task"}`,
		TimeoutMs: 20000,
	}

	resp, err := client.Execute(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.DeadlineExceeded, st.Code())
}

func TestAgentService_GetDescriptor_Success(t *testing.T) {
	mock := &mockAgentService{}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentGetDescriptorRequest{}

	resp, err := client.GetDescriptor(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-agent", resp.Name)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Equal(t, "A test agent", resp.Description)
	assert.Equal(t, []string{"execute", "analyze"}, resp.Capabilities)
	assert.Equal(t, []string{"system", "network"}, resp.TargetTypes)
	assert.Equal(t, []string{"recon", "exploit"}, resp.TechniqueTypes)
}

func TestAgentService_GetSlotSchema_Success(t *testing.T) {
	mock := &mockAgentService{}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentGetSlotSchemaRequest{}

	resp, err := client.GetSlotSchema(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Slots, 1)

	slot := resp.Slots[0]
	assert.Equal(t, "primary", slot.Name)
	assert.Equal(t, "Primary slot", slot.Description)
	assert.True(t, slot.Required)
	assert.NotNil(t, slot.DefaultConfig)
	assert.Equal(t, "openai", slot.DefaultConfig.Provider)
	assert.Equal(t, "gpt-4", slot.DefaultConfig.Model)
	assert.Equal(t, 0.7, slot.DefaultConfig.Temperature)
	assert.Equal(t, int32(4096), slot.DefaultConfig.MaxTokens)
	assert.NotNil(t, slot.Constraints)
	assert.Equal(t, int32(8192), slot.Constraints.MinContextWindow)
	assert.Equal(t, []string{"function-calling"}, slot.Constraints.RequiredFeatures)
}

func TestAgentService_Health_Success(t *testing.T) {
	mock := &mockAgentService{}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentHealthRequest{}

	resp, err := client.Health(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "healthy", resp.State)
	assert.Equal(t, "Agent ready", resp.Message)
	assert.Greater(t, resp.CheckedAt, int64(0))
}

func TestAgentService_Health_Unhealthy(t *testing.T) {
	mock := &mockAgentService{
		healthFunc: func(ctx context.Context, req *pb.AgentHealthRequest) (*pb.HealthStatus, error) {
			return &pb.HealthStatus{
				State:     "unhealthy",
				Message:   "Connection to backend lost",
				CheckedAt: time.Now().UnixMilli(),
			}, nil
		},
	}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	ctx := context.Background()
	req := &pb.AgentHealthRequest{}

	resp, err := client.Health(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "unhealthy", resp.State)
	assert.Equal(t, "Connection to backend lost", resp.Message)
}

// Error Translation Tests

func TestErrorTranslation_InvalidArgument(t *testing.T) {
	tests := []struct {
		name         string
		setupService func() (interface{}, func())
		executeTest  func(interface{}) error
	}{
		{
			name: "ToolService Invalid Argument",
			setupService: func() (interface{}, func()) {
				mock := &mockToolService{
					executeFunc: func(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
						return nil, status.Error(codes.InvalidArgument, "invalid input")
					},
				}
				client, cleanup := setupToolService(t, mock)
				return client, cleanup
			},
			executeTest: func(client interface{}) error {
				_, err := client.(pb.ToolServiceClient).Execute(context.Background(), &pb.ToolExecuteRequest{})
				return err
			},
		},
		{
			name: "PluginService Invalid Argument",
			setupService: func() (interface{}, func()) {
				mock := &mockPluginService{
					queryFunc: func(ctx context.Context, req *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error) {
						return nil, status.Error(codes.InvalidArgument, "invalid method")
					},
				}
				client, cleanup := setupPluginService(t, mock)
				return client, cleanup
			},
			executeTest: func(client interface{}) error {
				_, err := client.(pb.PluginServiceClient).Query(context.Background(), &pb.PluginQueryRequest{})
				return err
			},
		},
		{
			name: "AgentService Invalid Argument",
			setupService: func() (interface{}, func()) {
				mock := &mockAgentService{
					executeFunc: func(ctx context.Context, req *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error) {
						return nil, status.Error(codes.InvalidArgument, "invalid task")
					},
				}
				client, cleanup := setupAgentService(t, mock)
				return client, cleanup
			},
			executeTest: func(client interface{}) error {
				_, err := client.(pb.AgentServiceClient).Execute(context.Background(), &pb.AgentExecuteRequest{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cleanup := tt.setupService()
			defer cleanup()

			err := tt.executeTest(client)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
		})
	}
}

func TestErrorTranslation_NotFound(t *testing.T) {
	mock := &mockToolService{
		getDescriptorFunc: func(ctx context.Context, req *pb.ToolGetDescriptorRequest) (*pb.ToolDescriptor, error) {
			return nil, status.Error(codes.NotFound, "tool not found")
		},
	}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	_, err := client.GetDescriptor(context.Background(), &pb.ToolGetDescriptorRequest{})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "tool not found")
}

func TestErrorTranslation_PermissionDenied(t *testing.T) {
	mock := &mockAgentService{
		executeFunc: func(ctx context.Context, req *pb.AgentExecuteRequest) (*pb.AgentExecuteResponse, error) {
			return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
		},
	}
	client, cleanup := setupAgentService(t, mock)
	defer cleanup()

	_, err := client.Execute(context.Background(), &pb.AgentExecuteRequest{})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
	assert.Contains(t, st.Message(), "insufficient permissions")
}

func TestErrorTranslation_Internal(t *testing.T) {
	mock := &mockPluginService{
		queryFunc: func(ctx context.Context, req *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error) {
			return nil, status.Error(codes.Internal, "internal server error")
		},
	}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	_, err := client.Query(context.Background(), &pb.PluginQueryRequest{})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "internal server error")
}

// Concurrent Access Tests

func TestToolService_ConcurrentExecute(t *testing.T) {
	mock := &mockToolService{
		executeFunc: func(ctx context.Context, req *pb.ToolExecuteRequest) (*pb.ToolExecuteResponse, error) {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			return &pb.ToolExecuteResponse{
				OutputJson: `{"result":"success"}`,
			}, nil
		},
	}
	client, cleanup := setupToolService(t, mock)
	defer cleanup()

	const numConcurrent = 10
	errChan := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func() {
			ctx := context.Background()
			req := &pb.ToolExecuteRequest{
				InputJson: `{"command":"test"}`,
				TimeoutMs: 5000,
			}

			resp, err := client.Execute(ctx, req)
			if err != nil {
				errChan <- err
				return
			}
			if resp.OutputJson != `{"result":"success"}` {
				errChan <- assert.AnError
				return
			}
			errChan <- nil
		}()
	}

	for i := 0; i < numConcurrent; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}
}

func TestPluginService_ConcurrentQuery(t *testing.T) {
	mock := &mockPluginService{
		queryFunc: func(ctx context.Context, req *pb.PluginQueryRequest) (*pb.PluginQueryResponse, error) {
			time.Sleep(5 * time.Millisecond)
			return &pb.PluginQueryResponse{
				ResultJson: `{"data":"result"}`,
			}, nil
		},
	}
	client, cleanup := setupPluginService(t, mock)
	defer cleanup()

	const numConcurrent = 5
	errChan := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func() {
			ctx := context.Background()
			req := &pb.PluginQueryRequest{
				Method:     "getData",
				ParamsJson: `{"id":"123"}`,
				TimeoutMs:  5000,
			}

			resp, err := client.Query(ctx, req)
			if err != nil {
				errChan <- err
				return
			}
			if resp.ResultJson != `{"data":"result"}` {
				errChan <- assert.AnError
				return
			}
			errChan <- nil
		}()
	}

	for i := 0; i < numConcurrent; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}
}
