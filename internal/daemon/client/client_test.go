package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zero-day-ai/gibson/internal/daemon"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
)

// TestConvertProtoStatus tests the convertProtoStatus function.
func TestConvertProtoStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    *api.StatusResponse
		expected *daemon.DaemonStatus
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name: "complete status",
			input: &api.StatusResponse{
				Running:            true,
				Pid:                12345,
				StartTime:          1640000000,
				Uptime:             "2h30m15s",
				GrpcAddress:        "localhost:50002",
				RegistryType:       "embedded",
				RegistryAddr:       "embedded://localhost:2379",
				CallbackAddr:       "localhost:50001",
				AgentCount:         5,
				MissionCount:       10,
				ActiveMissionCount: 2,
			},
			expected: &daemon.DaemonStatus{
				Running:      true,
				PID:          12345,
				StartTime:    time.Unix(1640000000, 0),
				Uptime:       "2h30m15s",
				GRPCAddress:  "localhost:50002",
				RegistryType: "embedded",
				RegistryAddr: "embedded://localhost:2379",
				CallbackAddr: "localhost:50001",
				AgentCount:   5,
			},
		},
		{
			name: "zero values",
			input: &api.StatusResponse{
				Running:      false,
				Pid:          0,
				StartTime:    0,
				Uptime:       "",
				GrpcAddress:  "",
				RegistryType: "",
				RegistryAddr: "",
				CallbackAddr: "",
				AgentCount:   0,
			},
			expected: &daemon.DaemonStatus{
				Running:      false,
				PID:          0,
				StartTime:    time.Unix(0, 0),
				Uptime:       "",
				GRPCAddress:  "",
				RegistryType: "",
				RegistryAddr: "",
				CallbackAddr: "",
				AgentCount:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertProtoAgents tests the convertProtoAgents function.
func TestConvertProtoAgents(t *testing.T) {
	tests := []struct {
		name     string
		input    []*api.AgentInfo
		expected []AgentInfo
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []AgentInfo{},
		},
		{
			name:     "empty slice",
			input:    []*api.AgentInfo{},
			expected: []AgentInfo{},
		},
		{
			name: "single agent",
			input: []*api.AgentInfo{
				{
					Id:           "agent-1",
					Name:         "prompt-injection",
					Kind:         "agent",
					Version:      "1.0.0",
					Endpoint:     "localhost:50100",
					Capabilities: []string{"llm", "web"},
					Health:       "healthy",
					LastSeen:     1640000000,
				},
			},
			expected: []AgentInfo{
				{
					Name:        "prompt-injection",
					Version:     "1.0.0",
					Description: "",
					Address:     "localhost:50100",
					Status:      "healthy",
				},
			},
		},
		{
			name: "multiple agents",
			input: []*api.AgentInfo{
				{
					Name:     "agent-1",
					Version:  "1.0.0",
					Endpoint: "localhost:50100",
					Health:   "healthy",
				},
				{
					Name:     "agent-2",
					Version:  "2.0.0",
					Endpoint: "localhost:50101",
					Health:   "degraded",
				},
			},
			expected: []AgentInfo{
				{
					Name:        "agent-1",
					Version:     "1.0.0",
					Description: "",
					Address:     "localhost:50100",
					Status:      "healthy",
				},
				{
					Name:        "agent-2",
					Version:     "2.0.0",
					Description: "",
					Address:     "localhost:50101",
					Status:      "degraded",
				},
			},
		},
		{
			name: "nil elements are skipped",
			input: []*api.AgentInfo{
				{
					Name:     "agent-1",
					Version:  "1.0.0",
					Endpoint: "localhost:50100",
					Health:   "healthy",
				},
				nil,
				{
					Name:     "agent-2",
					Version:  "2.0.0",
					Endpoint: "localhost:50101",
					Health:   "healthy",
				},
			},
			expected: []AgentInfo{
				{
					Name:        "agent-1",
					Version:     "1.0.0",
					Description: "",
					Address:     "localhost:50100",
					Status:      "healthy",
				},
				{
					Name:        "agent-2",
					Version:     "2.0.0",
					Description: "",
					Address:     "localhost:50101",
					Status:      "healthy",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoAgents(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertProtoTools tests the convertProtoTools function.
func TestConvertProtoTools(t *testing.T) {
	tests := []struct {
		name     string
		input    []*api.ToolInfo
		expected []ToolInfo
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []ToolInfo{},
		},
		{
			name:     "empty slice",
			input:    []*api.ToolInfo{},
			expected: []ToolInfo{},
		},
		{
			name: "single tool",
			input: []*api.ToolInfo{
				{
					Id:          "tool-1",
					Name:        "nmap",
					Version:     "7.92",
					Endpoint:    "localhost:50200",
					Description: "Network scanner",
					Health:      "healthy",
					LastSeen:    1640000000,
				},
			},
			expected: []ToolInfo{
				{
					Name:        "nmap",
					Version:     "7.92",
					Description: "Network scanner",
					Address:     "localhost:50200",
					Status:      "healthy",
				},
			},
		},
		{
			name: "multiple tools with nil element",
			input: []*api.ToolInfo{
				{
					Name:        "nmap",
					Version:     "7.92",
					Description: "Network scanner",
					Endpoint:    "localhost:50200",
					Health:      "healthy",
				},
				nil,
				{
					Name:        "sqlmap",
					Version:     "1.5",
					Description: "SQL injection tool",
					Endpoint:    "localhost:50201",
					Health:      "healthy",
				},
			},
			expected: []ToolInfo{
				{
					Name:        "nmap",
					Version:     "7.92",
					Description: "Network scanner",
					Address:     "localhost:50200",
					Status:      "healthy",
				},
				{
					Name:        "sqlmap",
					Version:     "1.5",
					Description: "SQL injection tool",
					Address:     "localhost:50201",
					Status:      "healthy",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoTools(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertProtoPlugins tests the convertProtoPlugins function.
func TestConvertProtoPlugins(t *testing.T) {
	tests := []struct {
		name     string
		input    []*api.PluginInfo
		expected []PluginInfo
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []PluginInfo{},
		},
		{
			name:     "empty slice",
			input:    []*api.PluginInfo{},
			expected: []PluginInfo{},
		},
		{
			name: "single plugin",
			input: []*api.PluginInfo{
				{
					Id:          "plugin-1",
					Name:        "mitre-lookup",
					Version:     "1.0.0",
					Endpoint:    "localhost:50300",
					Description: "MITRE ATT&CK lookup plugin",
					Health:      "healthy",
					LastSeen:    1640000000,
				},
			},
			expected: []PluginInfo{
				{
					Name:        "mitre-lookup",
					Version:     "1.0.0",
					Description: "MITRE ATT&CK lookup plugin",
					Address:     "localhost:50300",
					Status:      "healthy",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoPlugins(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertProtoMissionEvent tests the convertProtoMissionEvent function.
func TestConvertProtoMissionEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    *api.MissionEvent
		expected MissionEvent
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: MissionEvent{},
		},
		{
			name: "complete mission event",
			input: &api.MissionEvent{
				EventType: "mission_started",
				Timestamp: 1640000000,
				MissionId: "mission-1",
				NodeId:    "node-1",
				Message:   "Mission started successfully",
				Data:      `{"workflow":"attack.yaml"}`,
				Error:     "",
			},
			expected: MissionEvent{
				Type:      "mission_started",
				Timestamp: time.Unix(1640000000, 0),
				Message:   "Mission started successfully",
				Data: map[string]interface{}{
					"workflow": "attack.yaml",
				},
			},
		},
		{
			name: "mission event with no data",
			input: &api.MissionEvent{
				EventType: "mission_completed",
				Timestamp: 1640000100,
				Message:   "Mission completed",
				Data:      "",
			},
			expected: MissionEvent{
				Type:      "mission_completed",
				Timestamp: time.Unix(1640000100, 0),
				Message:   "Mission completed",
				Data:      nil,
			},
		},
		{
			name: "mission event with invalid JSON data",
			input: &api.MissionEvent{
				EventType: "mission.finding",
				Timestamp: 1640000200,
				Message:   "Found vulnerability",
				Data:      "invalid-json{",
			},
			expected: MissionEvent{
				Type:      "mission.finding",
				Timestamp: time.Unix(1640000200, 0),
				Message:   "Found vulnerability",
				Data:      nil, // Invalid JSON results in nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoMissionEvent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertProtoAttackEvent tests the convertProtoAttackEvent function.
func TestConvertProtoAttackEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    *api.AttackEvent
		expected AttackEvent
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: AttackEvent{},
		},
		{
			name: "complete attack event",
			input: &api.AttackEvent{
				EventType: "attack.started",
				Timestamp: 1640000000,
				AttackId:  "attack-1",
				Message:   "Attack started",
				Data:      `{"target":"http://example.com"}`,
				Error:     "",
			},
			expected: AttackEvent{
				Type:      "attack.started",
				Timestamp: time.Unix(1640000000, 0),
				Message:   "Attack started",
				Severity:  "", // Not in proto
				Data: map[string]interface{}{
					"target": "http://example.com",
				},
			},
		},
		{
			name: "attack event with no data",
			input: &api.AttackEvent{
				EventType: "attack.completed",
				Timestamp: 1640000100,
				Message:   "Attack completed",
			},
			expected: AttackEvent{
				Type:      "attack.completed",
				Timestamp: time.Unix(1640000100, 0),
				Message:   "Attack completed",
				Severity:  "",
				Data:      nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoAttackEvent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertProtoEvent tests the convertProtoEvent function.
func TestConvertProtoEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    *api.Event
		expected Event
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: Event{},
		},
		{
			name: "complete event",
			input: &api.Event{
				EventType: "agent_registered",
				Timestamp: 1640000000,
				Source:    "registry",
				Data:      `{"agent":"test-agent"}`,
			},
			expected: Event{
				Type:      "agent_registered",
				Source:    "registry",
				Timestamp: time.Unix(1640000000, 0),
				Data: map[string]interface{}{
					"agent": "test-agent",
				},
			},
		},
		{
			name: "event with no data",
			input: &api.Event{
				EventType: "system_ready",
				Timestamp: 1640000100,
				Source:    "daemon",
				Data:      "",
			},
			expected: Event{
				Type:      "system_ready",
				Source:    "daemon",
				Timestamp: time.Unix(1640000100, 0),
				Data:      nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertProtoEvent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Mock DaemonServiceClient for testing client methods
type mockDaemonServiceClient struct {
	pingResponse        *api.PingResponse
	pingError           error
	statusResponse      *api.StatusResponse
	statusError         error
	listAgentsResponse  *api.ListAgentsResponse
	listAgentsError     error
	listToolsResponse   *api.ListToolsResponse
	listToolsError      error
	listPluginsResponse *api.ListPluginsResponse
	listPluginsError    error
}

func (m *mockDaemonServiceClient) Ping(ctx context.Context, req *api.PingRequest, opts ...grpc.CallOption) (*api.PingResponse, error) {
	return m.pingResponse, m.pingError
}

func (m *mockDaemonServiceClient) Status(ctx context.Context, req *api.StatusRequest, opts ...grpc.CallOption) (*api.StatusResponse, error) {
	return m.statusResponse, m.statusError
}

func (m *mockDaemonServiceClient) ListAgents(ctx context.Context, req *api.ListAgentsRequest, opts ...grpc.CallOption) (*api.ListAgentsResponse, error) {
	return m.listAgentsResponse, m.listAgentsError
}

func (m *mockDaemonServiceClient) ListTools(ctx context.Context, req *api.ListToolsRequest, opts ...grpc.CallOption) (*api.ListToolsResponse, error) {
	return m.listToolsResponse, m.listToolsError
}

func (m *mockDaemonServiceClient) ListPlugins(ctx context.Context, req *api.ListPluginsRequest, opts ...grpc.CallOption) (*api.ListPluginsResponse, error) {
	return m.listPluginsResponse, m.listPluginsError
}

// Stub implementations for other methods (not tested in this task)
func (m *mockDaemonServiceClient) Connect(ctx context.Context, req *api.ConnectRequest, opts ...grpc.CallOption) (*api.ConnectResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) RunMission(ctx context.Context, req *api.RunMissionRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.MissionEvent], error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) StopMission(ctx context.Context, req *api.StopMissionRequest, opts ...grpc.CallOption) (*api.StopMissionResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) ListMissions(ctx context.Context, req *api.ListMissionsRequest, opts ...grpc.CallOption) (*api.ListMissionsResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) GetAgentStatus(ctx context.Context, req *api.GetAgentStatusRequest, opts ...grpc.CallOption) (*api.AgentStatusResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) RunAttack(ctx context.Context, req *api.RunAttackRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.AttackEvent], error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) Subscribe(ctx context.Context, req *api.SubscribeRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.Event], error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) StartComponent(ctx context.Context, req *api.StartComponentRequest, opts ...grpc.CallOption) (*api.StartComponentResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) StopComponent(ctx context.Context, req *api.StopComponentRequest, opts ...grpc.CallOption) (*api.StopComponentResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) PauseMission(ctx context.Context, req *api.PauseMissionRequest, opts ...grpc.CallOption) (*api.PauseMissionResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) ResumeMission(ctx context.Context, req *api.ResumeMissionRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[api.MissionEvent], error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) GetMissionHistory(ctx context.Context, req *api.GetMissionHistoryRequest, opts ...grpc.CallOption) (*api.GetMissionHistoryResponse, error) {
	return nil, nil
}
func (m *mockDaemonServiceClient) GetMissionCheckpoints(ctx context.Context, req *api.GetMissionCheckpointsRequest, opts ...grpc.CallOption) (*api.GetMissionCheckpointsResponse, error) {
	return nil, nil
}

// TestClient_Ping tests the Ping method with mock client.
func TestClient_Ping(t *testing.T) {
	tests := []struct {
		name          string
		mockResponse  *api.PingResponse
		mockError     error
		expectedError string
	}{
		{
			name: "successful ping",
			mockResponse: &api.PingResponse{
				Timestamp: time.Now().Unix(),
			},
			mockError:     nil,
			expectedError: "",
		},
		{
			name:          "unavailable error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Unavailable, "connection refused"),
			expectedError: "daemon not responding (connection unavailable)",
		},
		{
			name:          "deadline exceeded error",
			mockResponse:  nil,
			mockError:     status.Error(codes.DeadlineExceeded, "timeout"),
			expectedError: "daemon ping timeout",
		},
		{
			name:          "internal error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Internal, "server panic"),
			expectedError: "daemon ping failed: server panic",
		},
		{
			name:          "generic error",
			mockResponse:  nil,
			mockError:     assert.AnError,
			expectedError: "daemon ping failed:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDaemonServiceClient{
				pingResponse: tt.mockResponse,
				pingError:    tt.mockError,
			}

			client := &Client{
				daemon: mock,
			}

			ctx := context.Background()
			err := client.Ping(ctx)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestClient_Status tests the Status method with mock client.
func TestClient_Status(t *testing.T) {
	tests := []struct {
		name          string
		mockResponse  *api.StatusResponse
		mockError     error
		expectedError string
	}{
		{
			name: "successful status",
			mockResponse: &api.StatusResponse{
				Running:      true,
				Pid:          12345,
				StartTime:    time.Now().Unix(),
				Uptime:       "1h30m",
				GrpcAddress:  "localhost:50002",
				RegistryType: "embedded",
				AgentCount:   5,
			},
			mockError:     nil,
			expectedError: "",
		},
		{
			name:          "unavailable error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Unavailable, "connection refused"),
			expectedError: "daemon not responding (is it running?)",
		},
		{
			name:          "deadline exceeded error",
			mockResponse:  nil,
			mockError:     status.Error(codes.DeadlineExceeded, "timeout"),
			expectedError: "daemon status request timeout",
		},
		{
			name:          "internal error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Internal, "database error"),
			expectedError: "failed to get daemon status: database error",
		},
		{
			name:          "generic error",
			mockResponse:  nil,
			mockError:     assert.AnError,
			expectedError: "failed to get daemon status:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDaemonServiceClient{
				statusResponse: tt.mockResponse,
				statusError:    tt.mockError,
			}

			client := &Client{
				daemon: mock,
			}

			ctx := context.Background()
			result, err := client.Status(ctx)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.mockResponse.Running, result.Running)
				assert.Equal(t, int(tt.mockResponse.Pid), result.PID)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestClient_ListAgents tests the ListAgents method with mock client.
func TestClient_ListAgents(t *testing.T) {
	tests := []struct {
		name          string
		mockResponse  *api.ListAgentsResponse
		mockError     error
		expectedCount int
		expectedError string
	}{
		{
			name: "successful list with agents",
			mockResponse: &api.ListAgentsResponse{
				Agents: []*api.AgentInfo{
					{
						Name:     "agent-1",
						Version:  "1.0.0",
						Endpoint: "localhost:50100",
						Health:   "healthy",
					},
					{
						Name:     "agent-2",
						Version:  "2.0.0",
						Endpoint: "localhost:50101",
						Health:   "healthy",
					},
				},
			},
			mockError:     nil,
			expectedCount: 2,
			expectedError: "",
		},
		{
			name: "successful list with empty results",
			mockResponse: &api.ListAgentsResponse{
				Agents: []*api.AgentInfo{},
			},
			mockError:     nil,
			expectedCount: 0,
			expectedError: "",
		},
		{
			name:          "unavailable error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Unavailable, "connection refused"),
			expectedCount: 0,
			expectedError: "daemon not responding (is it running?)",
		},
		{
			name:          "deadline exceeded error",
			mockResponse:  nil,
			mockError:     status.Error(codes.DeadlineExceeded, "timeout"),
			expectedCount: 0,
			expectedError: "daemon request timeout while listing agents",
		},
		{
			name:          "internal error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Internal, "registry error"),
			expectedCount: 0,
			expectedError: "failed to list agents: registry error",
		},
		{
			name:          "not found error",
			mockResponse:  nil,
			mockError:     status.Error(codes.NotFound, "no agents found"),
			expectedCount: 0,
			expectedError: "failed to list agents: no agents found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDaemonServiceClient{
				listAgentsResponse: tt.mockResponse,
				listAgentsError:    tt.mockError,
			}

			client := &Client{
				daemon: mock,
			}

			ctx := context.Background()
			result, err := client.ListAgents(ctx)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, tt.expectedCount)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestClient_ListTools tests the ListTools method with mock client.
func TestClient_ListTools(t *testing.T) {
	tests := []struct {
		name          string
		mockResponse  *api.ListToolsResponse
		mockError     error
		expectedCount int
		expectedError string
	}{
		{
			name: "successful list with tools",
			mockResponse: &api.ListToolsResponse{
				Tools: []*api.ToolInfo{
					{
						Name:        "nmap",
						Version:     "7.92",
						Description: "Network scanner",
						Endpoint:    "localhost:50200",
						Health:      "healthy",
					},
				},
			},
			mockError:     nil,
			expectedCount: 1,
			expectedError: "",
		},
		{
			name: "empty tools list",
			mockResponse: &api.ListToolsResponse{
				Tools: []*api.ToolInfo{},
			},
			mockError:     nil,
			expectedCount: 0,
			expectedError: "",
		},
		{
			name:          "unavailable error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Unavailable, "connection refused"),
			expectedCount: 0,
			expectedError: "daemon not responding (is it running?)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDaemonServiceClient{
				listToolsResponse: tt.mockResponse,
				listToolsError:    tt.mockError,
			}

			client := &Client{
				daemon: mock,
			}

			ctx := context.Background()
			result, err := client.ListTools(ctx)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, tt.expectedCount)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// TestClient_ListPlugins tests the ListPlugins method with mock client.
func TestClient_ListPlugins(t *testing.T) {
	tests := []struct {
		name          string
		mockResponse  *api.ListPluginsResponse
		mockError     error
		expectedCount int
		expectedError string
	}{
		{
			name: "successful list with plugins",
			mockResponse: &api.ListPluginsResponse{
				Plugins: []*api.PluginInfo{
					{
						Name:        "mitre-lookup",
						Version:     "1.0.0",
						Description: "MITRE ATT&CK lookup",
						Endpoint:    "localhost:50300",
						Health:      "healthy",
					},
				},
			},
			mockError:     nil,
			expectedCount: 1,
			expectedError: "",
		},
		{
			name: "empty plugins list",
			mockResponse: &api.ListPluginsResponse{
				Plugins: []*api.PluginInfo{},
			},
			mockError:     nil,
			expectedCount: 0,
			expectedError: "",
		},
		{
			name:          "unavailable error",
			mockResponse:  nil,
			mockError:     status.Error(codes.Unavailable, "connection refused"),
			expectedCount: 0,
			expectedError: "daemon not responding (is it running?)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDaemonServiceClient{
				listPluginsResponse: tt.mockResponse,
				listPluginsError:    tt.mockError,
			}

			client := &Client{
				daemon: mock,
			}

			ctx := context.Background()
			result, err := client.ListPlugins(ctx)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, tt.expectedCount)
			} else {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

// Keep existing tests for Connect and ConnectFromInfo
func TestConnect_InvalidAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Test empty address
	_, err := Connect(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestConnect_UnixSocketFormat(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		wantErr  bool
		errMatch string
	}{
		{
			name:     "unix scheme with absolute path",
			address:  "unix:///nonexistent/socket",
			wantErr:  true, // Connection will fail since socket doesn't exist
			errMatch: "failed to connect",
		},
		{
			name:     "absolute path without scheme",
			address:  "/nonexistent/socket",
			wantErr:  true,
			errMatch: "failed to connect",
		},
		{
			name:     "tcp address",
			address:  "localhost:50002",
			wantErr:  true, // No server listening
			errMatch: "failed to connect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			client, err := Connect(ctx, tt.address)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMatch != "" {
					assert.Contains(t, err.Error(), tt.errMatch)
				}
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				if client != nil {
					client.Close()
				}
			}
		})
	}
}

func TestConnectFromInfo_FileNotFound(t *testing.T) {
	ctx := context.Background()

	// Test with non-existent file
	_, err := ConnectFromInfo(ctx, "/nonexistent/daemon.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read daemon info")
}

func TestConnectFromInfo_InvalidPath(t *testing.T) {
	ctx := context.Background()

	// Test with empty path
	_, err := ConnectFromInfo(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestConnectFromInfo_ValidFileButNoServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Create temporary daemon info file
	tempDir := t.TempDir()
	infoPath := filepath.Join(tempDir, "daemon.json")

	info := &daemon.DaemonInfo{
		PID:         12345,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59999", // Non-existent server
		Version:     "1.0.0",
	}

	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	// Attempt connection - should fail because server doesn't exist
	_, err = ConnectFromInfo(ctx, infoPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

func TestClient_Close_Nil(t *testing.T) {
	client := &Client{conn: nil}
	err := client.Close()
	assert.NoError(t, err, "Close on nil connection should not error")
}

func TestConnectOrFail_NoDaemonInfo(t *testing.T) {
	// Set GIBSON_HOME to a temporary directory without daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	ctx := context.Background()
	client, err := ConnectOrFail(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "not running")
	assert.Contains(t, err.Error(), "gibson daemon start")
}

func TestConnectOrFail_DaemonInfoExistsButNotRunning(t *testing.T) {
	// Create temporary directory with daemon.json
	tempDir := t.TempDir()
	os.Setenv("GIBSON_HOME", tempDir)
	defer os.Unsetenv("GIBSON_HOME")

	// Write daemon info for a non-existent server
	infoPath := filepath.Join(tempDir, "daemon.json")
	info := &daemon.DaemonInfo{
		PID:         99999,
		StartTime:   time.Now(),
		GRPCAddress: "localhost:59998",
		Version:     "1.0.0",
	}
	err := daemon.WriteDaemonInfo(infoPath, info)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := ConnectOrFail(ctx)

	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "failed to connect")
	assert.Contains(t, err.Error(), "crashed or is not responding")
	assert.Contains(t, err.Error(), "Troubleshooting")
}

func TestGetGibsonHome(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		wantErr bool
	}{
		{
			name:    "with GIBSON_HOME set",
			envVal:  "/custom/gibson/home",
			wantErr: false,
		},
		{
			name:    "without GIBSON_HOME (uses default)",
			envVal:  "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set/unset environment variable
			if tt.envVal != "" {
				os.Setenv("GIBSON_HOME", tt.envVal)
				defer os.Unsetenv("GIBSON_HOME")
			} else {
				os.Unsetenv("GIBSON_HOME")
			}

			home, err := getGibsonHome()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, home)

				if tt.envVal != "" {
					assert.Equal(t, tt.envVal, home)
				} else {
					// Should return default path
					assert.Contains(t, home, ".gibson")
				}
			}
		})
	}
}
