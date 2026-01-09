package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	status_grpc "google.golang.org/grpc/status"
)

// DaemonServer implements the DaemonServiceServer interface.
//
// This server exposes all daemon functionality via gRPC including mission
// execution, agent management, attack operations, and real-time event streaming.
// It acts as a facade that delegates to the daemon's internal services.
type DaemonServer struct {
	UnimplementedDaemonServiceServer

	// daemon is the daemon instance this server exposes
	daemon DaemonInterface

	// logger is the structured logger
	logger *slog.Logger

	// sessionCounter generates unique session IDs
	sessionCounter int64
}

// DaemonInterface defines the interface that the daemon must implement
// for the gRPC server to delegate operations.
//
// This abstraction allows the server to work with the daemon without
// creating circular dependencies.
type DaemonInterface interface {
	// Status returns the current daemon status
	Status() (DaemonStatus, error)

	// ListAgents returns all registered agents
	ListAgents(ctx context.Context, kind string) ([]AgentInfoInternal, error)

	// GetAgentStatus returns status for a specific agent
	GetAgentStatus(ctx context.Context, agentID string) (AgentStatusInternal, error)

	// ListTools returns all registered tools
	ListTools(ctx context.Context) ([]ToolInfoInternal, error)

	// ListPlugins returns all registered plugins
	ListPlugins(ctx context.Context) ([]PluginInfoInternal, error)

	// RunMission starts a mission and returns an event channel
	RunMission(ctx context.Context, workflowPath string, missionID string, variables map[string]string, memoryContinuity string) (<-chan MissionEventData, error)

	// StopMission stops a running mission
	StopMission(ctx context.Context, missionID string, force bool) error

	// ListMissions returns mission list
	ListMissions(ctx context.Context, activeOnly bool, statusFilter, namePattern string, limit, offset int) ([]MissionData, int, error)

	// RunAttack executes an attack and returns an event channel
	RunAttack(ctx context.Context, req AttackRequest) (<-chan AttackEventData, error)

	// Subscribe establishes an event stream
	Subscribe(ctx context.Context, eventTypes []string, missionID string) (<-chan EventData, error)

	// StartComponent starts a component by kind and name
	StartComponent(ctx context.Context, kind string, name string) (StartComponentResult, error)

	// StopComponent stops a component by kind and name
	StopComponent(ctx context.Context, kind string, name string, force bool) (StopComponentResult, error)

	// PauseMission pauses a running mission at the next clean checkpoint boundary
	PauseMission(ctx context.Context, missionID string, force bool) error

	// ResumeMission resumes a paused mission from its last checkpoint
	ResumeMission(ctx context.Context, missionID string) (<-chan MissionEventData, error)

	// GetMissionHistory returns all runs for a mission name
	GetMissionHistory(ctx context.Context, name string, limit int, offset int) ([]MissionRunData, int, error)

	// GetMissionCheckpoints returns all checkpoints for a mission
	GetMissionCheckpoints(ctx context.Context, missionID string) ([]CheckpointData, error)

	// ExecuteTool executes a tool via the Tool Executor Service
	ExecuteTool(ctx context.Context, name string, inputJSON string, timeoutMs int64) (ExecuteToolResult, error)

	// GetAvailableTools returns all available tools from the Tool Executor Service
	GetAvailableTools(ctx context.Context) ([]AvailableToolData, error)
}

// DaemonStatus represents daemon status information.
type DaemonStatus struct {
	Running            bool
	PID                int32
	StartTime          time.Time
	Uptime             string
	GRPCAddress        string
	RegistryType       string
	RegistryAddr       string
	CallbackAddr       string
	AgentCount         int32
	MissionCount       int32
	ActiveMissionCount int32
}

// AgentInfoInternal represents agent information for internal daemon operations.
// This is separate from the proto-generated AgentInfo to avoid naming conflicts.
type AgentInfoInternal struct {
	ID           string
	Name         string
	Kind         string
	Version      string
	Endpoint     string
	Capabilities []string
	Health       string
	LastSeen     time.Time
}

// AgentStatusInternal represents detailed agent status for internal daemon operations.
// This is separate from the proto-generated types to avoid naming conflicts.
type AgentStatusInternal struct {
	Agent         AgentInfoInternal
	Active        bool
	CurrentTask   string
	TaskStartTime time.Time
}

// ToolInfoInternal represents tool information for internal daemon operations.
// This is separate from the proto-generated ToolInfo to avoid naming conflicts.
type ToolInfoInternal struct {
	ID          string
	Name        string
	Version     string
	Endpoint    string
	Description string
	Health      string
	LastSeen    time.Time
}

// PluginInfoInternal represents plugin information for internal daemon operations.
// This is separate from the proto-generated PluginInfo to avoid naming conflicts.
type PluginInfoInternal struct {
	ID          string
	Name        string
	Version     string
	Endpoint    string
	Description string
	Health      string
	LastSeen    time.Time
}

// MissionData represents mission information.
type MissionData struct {
	ID           string
	WorkflowPath string
	Status       string
	StartTime    time.Time
	EndTime      time.Time
	FindingCount int32
}

// MissionEventData represents mission event data from the daemon.
type MissionEventData struct {
	EventType string
	Timestamp time.Time
	MissionID string
	NodeID    string
	Message   string
	Data      string
	Error     string
	Result    *OperationResult
}

// AttackRequest represents an attack request.
type AttackRequest struct {
	Target        string
	TargetName    string
	AttackType    string
	AgentID       string
	PayloadFilter string
	Options       map[string]string
	Goal          string
}

// AttackEventData represents attack event data from the daemon.
type AttackEventData struct {
	EventType string
	Timestamp time.Time
	AttackID  string
	Message   string
	Data      string
	Error     string
	Finding   *FindingData
	Result    *OperationResult
}

// FindingData represents finding information.
type FindingData struct {
	ID          string
	Title       string
	Severity    string
	Category    string
	Description string
	Technique   string
	Evidence    string
	Timestamp   time.Time
}

// EventData represents a generic event from the daemon.
type EventData struct {
	EventType    string
	Timestamp    time.Time
	Source       string
	Data         string
	MissionEvent *MissionEventData
	AttackEvent  *AttackEventData
	AgentEvent   *AgentEventData
	FindingEvent *FindingEventData
}

// AgentEventData represents agent event data.
type AgentEventData struct {
	EventType string
	Timestamp time.Time
	AgentID   string
	AgentName string
	Message   string
	Data      string
}

// FindingEventData represents finding event data.
type FindingEventData struct {
	EventType string
	Timestamp time.Time
	Finding   FindingData
	MissionID string
}

// StartComponentResult represents the result of starting a component.
type StartComponentResult struct {
	PID     int
	Port    int
	LogPath string
}

// StopComponentResult represents the result of stopping a component.
type StopComponentResult struct {
	StoppedCount int
	TotalCount   int
}

// MissionRunData represents a single execution instance of a mission.
type MissionRunData struct {
	MissionID     string
	RunNumber     int
	Status        string
	CreatedAt     int64
	CompletedAt   int64
	FindingsCount int
	PreviousRunID string
}

// CheckpointData provides metadata about a mission checkpoint.
type CheckpointData struct {
	CheckpointID   string
	CreatedAt      int64
	CompletedNodes int
	TotalNodes     int
	FindingsCount  int
	Version        int
}

// ExecuteToolResult represents the result of tool execution.
type ExecuteToolResult struct {
	Success    bool
	OutputJSON string
	Error      string
	DurationMs int64
}

// AvailableToolData describes a tool's capabilities and execution metrics.
type AvailableToolData struct {
	Name             string
	Version          string
	Description      string
	Tags             []string
	InputSchemaJSON  string
	OutputSchemaJSON string
	Status           string
	ErrorMessage     string
	Metrics          *ToolMetricsData
}

// ToolMetricsData tracks execution statistics for a tool.
type ToolMetricsData struct {
	TotalCalls     int64
	SuccessCalls   int64
	FailedCalls    int64
	AvgDurationMs  int64
	LastExecutedAt int64
}

// NewDaemonServer creates a new gRPC server that exposes daemon functionality.
//
// Parameters:
//   - daemon: The daemon instance to expose via gRPC
//   - logger: Structured logger for request logging
//
// Returns:
//   - *DaemonServer: A new gRPC server ready to be registered
func NewDaemonServer(daemon DaemonInterface, logger *slog.Logger) *DaemonServer {
	if logger == nil {
		logger = slog.Default().With("component", "daemon-grpc")
	}

	return &DaemonServer{
		daemon:         daemon,
		logger:         logger.With("component", "daemon-grpc"),
		sessionCounter: 0,
	}
}

// Connect establishes a client connection to the daemon.
func (s *DaemonServer) Connect(ctx context.Context, req *ConnectRequest) (*ConnectResponse, error) {
	s.logger.Info("client connecting",
		"client_version", req.ClientVersion,
		"client_id", req.ClientId,
	)

	// Generate unique session ID
	s.sessionCounter++
	sessionID := fmt.Sprintf("session-%d-%d", time.Now().Unix(), s.sessionCounter)

	// Get daemon status to include address
	status, err := s.daemon.Status()
	if err != nil {
		s.logger.Error("failed to get daemon status", "error", err)
		return nil, status_grpc.Errorf(codes.Internal, "failed to get daemon status: %v", err)
	}

	return &ConnectResponse{
		DaemonVersion: "0.1.0", // TODO: Get from version package
		SessionId:     sessionID,
		GrpcAddress:   status.GRPCAddress,
	}, nil
}

// Ping checks if the daemon is responsive.
func (s *DaemonServer) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	return &PingResponse{
		Timestamp: time.Now().Unix(),
	}, nil
}

// Status returns the current daemon status.
func (s *DaemonServer) Status(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
	s.logger.Debug("status request received")

	status, err := s.daemon.Status()
	if err != nil {
		s.logger.Error("failed to get daemon status", "error", err)
		return nil, status_grpc.Errorf(codes.Internal, "failed to get daemon status: %v", err)
	}

	return &StatusResponse{
		Running:            status.Running,
		Pid:                status.PID,
		StartTime:          status.StartTime.Unix(),
		Uptime:             status.Uptime,
		GrpcAddress:        status.GRPCAddress,
		RegistryType:       status.RegistryType,
		RegistryAddr:       status.RegistryAddr,
		CallbackAddr:       status.CallbackAddr,
		AgentCount:         status.AgentCount,
		MissionCount:       status.MissionCount,
		ActiveMissionCount: status.ActiveMissionCount,
	}, nil
}

// RunMission starts a mission and streams execution events.
func (s *DaemonServer) RunMission(req *RunMissionRequest, stream grpc.ServerStreamingServer[MissionEvent]) error {
	s.logger.Info("mission run request received",
		"workflow_path", req.WorkflowPath,
		"mission_id", req.MissionId,
		"memory_continuity", req.MemoryContinuity,
	)

	// Start mission and get event channel
	eventChan, err := s.daemon.RunMission(stream.Context(), req.WorkflowPath, req.MissionId, req.Variables, req.MemoryContinuity)
	if err != nil {
		s.logger.Error("failed to start mission", "error", err)
		return status_grpc.Errorf(codes.Internal, "failed to start mission: %v", err)
	}

	// Stream events to client
	for {
		select {
		case <-stream.Context().Done():
			s.logger.Info("mission stream cancelled", "mission_id", req.MissionId)
			return stream.Context().Err()

		case event, ok := <-eventChan:
			if !ok {
				// Event channel closed, mission completed
				s.logger.Info("mission completed", "mission_id", req.MissionId)
				return nil
			}

			// Convert event to proto message
			protoEvent := &MissionEvent{
				EventType: event.EventType,
				Timestamp: event.Timestamp.Unix(),
				MissionId: event.MissionID,
				NodeId:    event.NodeID,
				Message:   event.Message,
				Data:      event.Data,
				Error:     event.Error,
			}

			// Send event to client
			if err := stream.Send(protoEvent); err != nil {
				s.logger.Error("failed to send mission event", "error", err)
				return status_grpc.Errorf(codes.Internal, "failed to send event: %v", err)
			}
		}
	}
}

// StopMission gracefully stops a running mission.
func (s *DaemonServer) StopMission(ctx context.Context, req *StopMissionRequest) (*StopMissionResponse, error) {
	s.logger.Info("mission stop request received",
		"mission_id", req.MissionId,
		"force", req.Force,
	)

	err := s.daemon.StopMission(ctx, req.MissionId, req.Force)
	if err != nil {
		s.logger.Error("failed to stop mission", "error", err, "mission_id", req.MissionId)
		return &StopMissionResponse{
			Success: false,
			Message: fmt.Sprintf("failed to stop mission: %v", err),
		}, nil
	}

	return &StopMissionResponse{
		Success: true,
		Message: "Mission stop requested",
	}, nil
}

// ListMissions returns all missions (past and active).
func (s *DaemonServer) ListMissions(ctx context.Context, req *ListMissionsRequest) (*ListMissionsResponse, error) {
	s.logger.Debug("mission list request received",
		"active_only", req.ActiveOnly,
		"status_filter", req.StatusFilter,
		"name_pattern", req.NamePattern,
		"limit", req.Limit,
		"offset", req.Offset,
	)

	missions, total, err := s.daemon.ListMissions(ctx, req.ActiveOnly, req.StatusFilter, req.NamePattern, int(req.Limit), int(req.Offset))
	if err != nil {
		s.logger.Error("failed to list missions", "error", err)
		return nil, status_grpc.Errorf(codes.Internal, "failed to list missions: %v", err)
	}

	// Convert to proto messages
	protoMissions := make([]*MissionInfo, len(missions))
	for i, m := range missions {
		protoMissions[i] = &MissionInfo{
			Id:           m.ID,
			WorkflowPath: m.WorkflowPath,
			Status:       m.Status,
			StartTime:    m.StartTime.Unix(),
			EndTime:      m.EndTime.Unix(),
			FindingCount: m.FindingCount,
		}
	}

	return &ListMissionsResponse{
		Missions: protoMissions,
		Total:    int32(total),
	}, nil
}

// ListAgents returns all registered agents from the etcd registry.
func (s *DaemonServer) ListAgents(ctx context.Context, req *ListAgentsRequest) (*ListAgentsResponse, error) {
	s.logger.Debug("agent list request received", "kind", req.Kind)

	agents, err := s.daemon.ListAgents(ctx, req.Kind)
	if err != nil {
		s.logger.Error("failed to list agents", "error", err)
		return nil, status_grpc.Errorf(codes.Internal, "failed to list agents: %v", err)
	}

	// Convert to proto messages
	protoAgents := make([]*AgentInfo, len(agents))
	for i, a := range agents {
		protoAgents[i] = &AgentInfo{
			Id:           a.ID,
			Name:         a.Name,
			Kind:         a.Kind,
			Version:      a.Version,
			Endpoint:     a.Endpoint,
			Capabilities: a.Capabilities,
			Health:       a.Health,
			LastSeen:     a.LastSeen.Unix(),
		}
	}

	return &ListAgentsResponse{
		Agents: protoAgents,
	}, nil
}

// GetAgentStatus returns the current status of a specific agent.
func (s *DaemonServer) GetAgentStatus(ctx context.Context, req *GetAgentStatusRequest) (*AgentStatusResponse, error) {
	s.logger.Debug("agent status request received", "agent_id", req.AgentId)

	agentStatus, err := s.daemon.GetAgentStatus(ctx, req.AgentId)
	if err != nil {
		s.logger.Error("failed to get agent status", "error", err, "agent_id", req.AgentId)
		return nil, status_grpc.Errorf(codes.Internal, "failed to get agent status: %v", err)
	}

	return &AgentStatusResponse{
		Agent: &AgentInfo{
			Id:           agentStatus.Agent.ID,
			Name:         agentStatus.Agent.Name,
			Kind:         agentStatus.Agent.Kind,
			Version:      agentStatus.Agent.Version,
			Endpoint:     agentStatus.Agent.Endpoint,
			Capabilities: agentStatus.Agent.Capabilities,
			Health:       agentStatus.Agent.Health,
			LastSeen:     agentStatus.Agent.LastSeen.Unix(),
		},
		Active:        agentStatus.Active,
		CurrentTask:   agentStatus.CurrentTask,
		TaskStartTime: agentStatus.TaskStartTime.Unix(),
	}, nil
}

// RunAttack executes an attack and streams progress events.
func (s *DaemonServer) RunAttack(req *RunAttackRequest, stream grpc.ServerStreamingServer[AttackEvent]) error {
	s.logger.Info("attack run request received",
		"target", req.Target,
		"attack_type", req.AttackType,
		"agent_id", req.AgentId,
	)

	// Convert proto request to internal request
	attackReq := AttackRequest{
		Target:        req.Target,
		TargetName:    req.TargetName,
		AttackType:    req.AttackType,
		AgentID:       req.AgentId,
		PayloadFilter: req.PayloadFilter,
		Options:       req.Options,
		Goal:          req.Goal,
	}

	// Start attack and get event channel
	eventChan, err := s.daemon.RunAttack(stream.Context(), attackReq)
	if err != nil {
		s.logger.Error("failed to start attack", "error", err)
		return status_grpc.Errorf(codes.Internal, "failed to start attack: %v", err)
	}

	// Stream events to client
	for {
		select {
		case <-stream.Context().Done():
			s.logger.Info("attack stream cancelled", "target", req.Target)
			return stream.Context().Err()

		case event, ok := <-eventChan:
			if !ok {
				// Event channel closed, attack completed
				s.logger.Info("attack completed", "target", req.Target)
				return nil
			}

			// Convert event to proto message
			protoEvent := &AttackEvent{
				EventType: event.EventType,
				Timestamp: event.Timestamp.Unix(),
				AttackId:  event.AttackID,
				Message:   event.Message,
				Data:      event.Data,
				Error:     event.Error,
			}

			// Add operation result if present
			if event.Result != nil {
				protoEvent.Result = event.Result
			}

			// Add finding if present
			if event.Finding != nil {
				protoEvent.Finding = &FindingInfo{
					Id:          event.Finding.ID,
					Title:       event.Finding.Title,
					Severity:    event.Finding.Severity,
					Category:    event.Finding.Category,
					Description: event.Finding.Description,
					Technique:   event.Finding.Technique,
					Evidence:    event.Finding.Evidence,
					Timestamp:   event.Finding.Timestamp.Unix(),
				}
			}

			// Send event to client
			if err := stream.Send(protoEvent); err != nil {
				s.logger.Error("failed to send attack event", "error", err)
				return status_grpc.Errorf(codes.Internal, "failed to send event: %v", err)
			}
		}
	}
}

// Subscribe establishes an event stream for TUI real-time updates.
func (s *DaemonServer) Subscribe(req *SubscribeRequest, stream grpc.ServerStreamingServer[Event]) error {
	s.logger.Info("event subscription request received",
		"event_types", req.EventTypes,
		"mission_id", req.MissionId,
	)

	// Subscribe to daemon events
	eventChan, err := s.daemon.Subscribe(stream.Context(), req.EventTypes, req.MissionId)
	if err != nil {
		s.logger.Error("failed to subscribe to events", "error", err)
		return status_grpc.Errorf(codes.Internal, "failed to subscribe: %v", err)
	}

	// Stream events to client
	for {
		select {
		case <-stream.Context().Done():
			s.logger.Info("event subscription cancelled")
			return stream.Context().Err()

		case event, ok := <-eventChan:
			if !ok {
				// Event channel closed
				s.logger.Info("event subscription closed")
				return nil
			}

			// Convert event to proto message
			protoEvent := &Event{
				EventType: event.EventType,
				Timestamp: event.Timestamp.Unix(),
				Source:    event.Source,
				Data:      event.Data,
			}

			// Add specific event type if present
			if event.MissionEvent != nil {
				protoEvent.Event = &Event_MissionEvent{
					MissionEvent: &MissionEvent{
						EventType: event.MissionEvent.EventType,
						Timestamp: event.MissionEvent.Timestamp.Unix(),
						MissionId: event.MissionEvent.MissionID,
						NodeId:    event.MissionEvent.NodeID,
						Message:   event.MissionEvent.Message,
						Data:      event.MissionEvent.Data,
						Error:     event.MissionEvent.Error,
					},
				}
			} else if event.AttackEvent != nil {
				var finding *FindingInfo
				if event.AttackEvent.Finding != nil {
					finding = &FindingInfo{
						Id:          event.AttackEvent.Finding.ID,
						Title:       event.AttackEvent.Finding.Title,
						Severity:    event.AttackEvent.Finding.Severity,
						Category:    event.AttackEvent.Finding.Category,
						Description: event.AttackEvent.Finding.Description,
						Technique:   event.AttackEvent.Finding.Technique,
						Evidence:    event.AttackEvent.Finding.Evidence,
						Timestamp:   event.AttackEvent.Finding.Timestamp.Unix(),
					}
				}
				protoEvent.Event = &Event_AttackEvent{
					AttackEvent: &AttackEvent{
						EventType: event.AttackEvent.EventType,
						Timestamp: event.AttackEvent.Timestamp.Unix(),
						AttackId:  event.AttackEvent.AttackID,
						Message:   event.AttackEvent.Message,
						Data:      event.AttackEvent.Data,
						Error:     event.AttackEvent.Error,
						Finding:   finding,
					},
				}
			} else if event.AgentEvent != nil {
				protoEvent.Event = &Event_AgentEvent{
					AgentEvent: &AgentEvent{
						EventType: event.AgentEvent.EventType,
						Timestamp: event.AgentEvent.Timestamp.Unix(),
						AgentId:   event.AgentEvent.AgentID,
						AgentName: event.AgentEvent.AgentName,
						Message:   event.AgentEvent.Message,
						Data:      event.AgentEvent.Data,
					},
				}
			} else if event.FindingEvent != nil {
				protoEvent.Event = &Event_FindingEvent{
					FindingEvent: &FindingEvent{
						EventType: event.FindingEvent.EventType,
						Timestamp: event.FindingEvent.Timestamp.Unix(),
						Finding: &FindingInfo{
							Id:          event.FindingEvent.Finding.ID,
							Title:       event.FindingEvent.Finding.Title,
							Severity:    event.FindingEvent.Finding.Severity,
							Category:    event.FindingEvent.Finding.Category,
							Description: event.FindingEvent.Finding.Description,
							Technique:   event.FindingEvent.Finding.Technique,
							Evidence:    event.FindingEvent.Finding.Evidence,
							Timestamp:   event.FindingEvent.Finding.Timestamp.Unix(),
						},
						MissionId: event.FindingEvent.MissionID,
					},
				}
			}

			// Send event to client
			if err := stream.Send(protoEvent); err != nil {
				s.logger.Error("failed to send event", "error", err)
				return status_grpc.Errorf(codes.Internal, "failed to send event: %v", err)
			}
		}
	}
}

// StartComponent starts a component (agent, tool, or plugin) by kind and name.
func (s *DaemonServer) StartComponent(ctx context.Context, req *StartComponentRequest) (*StartComponentResponse, error) {
	s.logger.Info("start component request received",
		"kind", req.Kind,
		"name", req.Name,
	)

	// Validate request
	if req.Kind == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "component kind is required")
	}
	if req.Name == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "component name is required")
	}

	// Validate kind is one of the supported types
	if req.Kind != "agent" && req.Kind != "tool" && req.Kind != "plugin" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "invalid component kind: %s (must be agent, tool, or plugin)", req.Kind)
	}

	// Call daemon implementation
	result, err := s.daemon.StartComponent(ctx, req.Kind, req.Name)
	if err != nil {
		s.logger.Error("failed to start component", "error", err, "kind", req.Kind, "name", req.Name)

		// Map errors to appropriate gRPC codes
		if strings.Contains(err.Error(), "not found") {
			return nil, status_grpc.Errorf(codes.NotFound, "component '%s' not found", req.Name)
		}
		if strings.Contains(err.Error(), "already running") {
			return nil, status_grpc.Errorf(codes.AlreadyExists, "component '%s' is already running", req.Name)
		}

		return nil, status_grpc.Errorf(codes.Internal, "failed to start component: %v", err)
	}

	s.logger.Info("component started successfully",
		"kind", req.Kind,
		"name", req.Name,
		"pid", result.PID,
		"port", result.Port,
	)

	return &StartComponentResponse{
		Success: true,
		Pid:     int32(result.PID),
		Port:    int32(result.Port),
		Message: fmt.Sprintf("Component '%s' started successfully", req.Name),
		LogPath: result.LogPath,
	}, nil
}

// StopComponent stops a running component (agent, tool, or plugin) by kind and name.
func (s *DaemonServer) StopComponent(ctx context.Context, req *StopComponentRequest) (*StopComponentResponse, error) {
	s.logger.Info("stop component request received",
		"kind", req.Kind,
		"name", req.Name,
		"force", req.Force,
	)

	// Validate request
	if req.Kind == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "component kind is required")
	}
	if req.Name == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "component name is required")
	}

	// Validate kind is one of the supported types
	if req.Kind != "agent" && req.Kind != "tool" && req.Kind != "plugin" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "invalid component kind: %s (must be agent, tool, or plugin)", req.Kind)
	}

	// Call daemon implementation
	result, err := s.daemon.StopComponent(ctx, req.Kind, req.Name, req.Force)
	if err != nil {
		s.logger.Error("failed to stop component", "error", err, "kind", req.Kind, "name", req.Name)

		// Map errors to appropriate gRPC codes
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not running") {
			return nil, status_grpc.Errorf(codes.NotFound, "component '%s' is not running", req.Name)
		}

		return nil, status_grpc.Errorf(codes.Internal, "failed to stop component: %v", err)
	}

	s.logger.Info("component stopped successfully",
		"kind", req.Kind,
		"name", req.Name,
		"stopped_count", result.StoppedCount,
		"total_count", result.TotalCount,
	)

	return &StopComponentResponse{
		Success:      true,
		StoppedCount: int32(result.StoppedCount),
		TotalCount:   int32(result.TotalCount),
		Message:      fmt.Sprintf("Stopped %d/%d instances of component '%s'", result.StoppedCount, result.TotalCount, req.Name),
	}, nil
}

// PauseMission pauses a running mission at the next clean checkpoint boundary.
func (s *DaemonServer) PauseMission(ctx context.Context, req *PauseMissionRequest) (*PauseMissionResponse, error) {
	s.logger.Info("mission pause request received",
		"mission_id", req.MissionId,
		"force", req.Force,
	)

	// Validate mission ID
	if req.MissionId == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "mission ID is required")
	}

	// Call daemon implementation
	err := s.daemon.PauseMission(ctx, req.MissionId, req.Force)
	if err != nil {
		s.logger.Error("failed to pause mission", "error", err, "mission_id", req.MissionId)

		// Map errors to appropriate gRPC codes
		if strings.Contains(err.Error(), "not found") {
			return nil, status_grpc.Errorf(codes.NotFound, "mission not found: %s", req.MissionId)
		}
		if strings.Contains(err.Error(), "not running") {
			return nil, status_grpc.Errorf(codes.FailedPrecondition, "mission is not running: %s", req.MissionId)
		}

		return nil, status_grpc.Errorf(codes.Internal, "failed to pause mission: %v", err)
	}

	s.logger.Info("mission paused successfully", "mission_id", req.MissionId)

	return &PauseMissionResponse{
		Success:      true,
		CheckpointId: "", // Will be populated when checkpoint system is fully integrated
		Message:      fmt.Sprintf("Mission %s paused successfully", req.MissionId),
	}, nil
}

// ResumeMission resumes a paused mission from its last checkpoint and streams execution events.
func (s *DaemonServer) ResumeMission(req *ResumeMissionRequest, stream grpc.ServerStreamingServer[MissionEvent]) error {
	s.logger.Info("mission resume request received",
		"mission_id", req.MissionId,
		"checkpoint_id", req.CheckpointId,
	)

	// Validate mission ID
	if req.MissionId == "" {
		return status_grpc.Errorf(codes.InvalidArgument, "mission ID is required")
	}

	// Call daemon implementation to resume the mission
	eventChan, err := s.daemon.ResumeMission(stream.Context(), req.MissionId)
	if err != nil {
		s.logger.Error("failed to resume mission", "error", err, "mission_id", req.MissionId)

		// Map errors to appropriate gRPC codes
		if strings.Contains(err.Error(), "not found") {
			return status_grpc.Errorf(codes.NotFound, "mission not found: %s", req.MissionId)
		}
		if strings.Contains(err.Error(), "not paused") {
			return status_grpc.Errorf(codes.FailedPrecondition, "mission is not paused: %s", req.MissionId)
		}
		if strings.Contains(err.Error(), "no checkpoint") {
			return status_grpc.Errorf(codes.FailedPrecondition, "no checkpoint available for mission: %s", req.MissionId)
		}

		return status_grpc.Errorf(codes.Internal, "failed to resume mission: %v", err)
	}

	// Stream events to client (similar to RunMission)
	for {
		select {
		case <-stream.Context().Done():
			s.logger.Info("mission resume stream cancelled", "mission_id", req.MissionId)
			return stream.Context().Err()

		case event, ok := <-eventChan:
			if !ok {
				// Event channel closed, mission completed
				s.logger.Info("mission resumed and completed", "mission_id", req.MissionId)
				return nil
			}

			// Convert event to proto message
			protoEvent := &MissionEvent{
				EventType: event.EventType,
				Timestamp: event.Timestamp.Unix(),
				MissionId: event.MissionID,
				NodeId:    event.NodeID,
				Message:   event.Message,
				Data:      event.Data,
				Error:     event.Error,
			}

			// Add operation result if present
			if event.Result != nil {
				protoEvent.Result = event.Result
			}

			// Send event to client
			if err := stream.Send(protoEvent); err != nil {
				s.logger.Error("failed to send mission event", "error", err)
				return status_grpc.Errorf(codes.Internal, "failed to send event: %v", err)
			}
		}
	}
}

// GetMissionHistory returns all runs for a mission name.
func (s *DaemonServer) GetMissionHistory(ctx context.Context, req *GetMissionHistoryRequest) (*GetMissionHistoryResponse, error) {
	s.logger.Debug("mission history request received",
		"name", req.Name,
		"limit", req.Limit,
		"offset", req.Offset,
	)

	// Validate request
	if req.Name == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "mission name is required")
	}

	// Set defaults for pagination
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	// Call daemon implementation
	runs, total, err := s.daemon.GetMissionHistory(ctx, req.Name, limit, offset)
	if err != nil {
		s.logger.Error("failed to get mission history", "error", err, "name", req.Name)
		return nil, status_grpc.Errorf(codes.Internal, "failed to get mission history: %v", err)
	}

	// Convert internal types to proto types
	protoRuns := make([]*MissionRun, len(runs))
	for i, run := range runs {
		protoRuns[i] = &MissionRun{
			MissionId:     run.MissionID,
			RunNumber:     int32(run.RunNumber),
			Status:        run.Status,
			CreatedAt:     run.CreatedAt,
			CompletedAt:   run.CompletedAt,
			FindingsCount: int32(run.FindingsCount),
			PreviousRunId: run.PreviousRunID,
		}
	}

	s.logger.Debug("mission history retrieved", "name", req.Name, "count", len(runs), "total", total)

	return &GetMissionHistoryResponse{
		Runs:  protoRuns,
		Total: int32(total),
	}, nil
}

// GetMissionCheckpoints returns all checkpoints for a specific mission.
func (s *DaemonServer) GetMissionCheckpoints(ctx context.Context, req *GetMissionCheckpointsRequest) (*GetMissionCheckpointsResponse, error) {
	s.logger.Debug("mission checkpoints request received",
		"mission_id", req.MissionId,
	)

	// Validate request
	if req.MissionId == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "mission ID is required")
	}

	// Call daemon implementation
	checkpoints, err := s.daemon.GetMissionCheckpoints(ctx, req.MissionId)
	if err != nil {
		s.logger.Error("failed to get mission checkpoints", "error", err, "mission_id", req.MissionId)

		// Map errors to appropriate gRPC codes
		if strings.Contains(err.Error(), "not found") {
			return nil, status_grpc.Errorf(codes.NotFound, "mission not found: %s", req.MissionId)
		}

		return nil, status_grpc.Errorf(codes.Internal, "failed to get mission checkpoints: %v", err)
	}

	// Convert internal types to proto types
	protoCheckpoints := make([]*CheckpointInfo, len(checkpoints))
	for i, cp := range checkpoints {
		protoCheckpoints[i] = &CheckpointInfo{
			CheckpointId:   cp.CheckpointID,
			CreatedAt:      cp.CreatedAt,
			CompletedNodes: int32(cp.CompletedNodes),
			TotalNodes:     int32(cp.TotalNodes),
			FindingsCount:  int32(cp.FindingsCount),
			Version:        int32(cp.Version),
		}
	}

	s.logger.Debug("mission checkpoints retrieved", "mission_id", req.MissionId, "count", len(checkpoints))

	return &GetMissionCheckpointsResponse{
		Checkpoints: protoCheckpoints,
	}, nil
}

// ExecuteTool executes a tool via the Tool Executor Service.
func (s *DaemonServer) ExecuteTool(ctx context.Context, req *ExecuteToolRequest) (*ExecuteToolResponse, error) {
	s.logger.Debug("tool execution request received",
		"name", req.Name,
		"timeout_ms", req.TimeoutMs,
	)

	// Validate request
	if req.Name == "" {
		return nil, status_grpc.Errorf(codes.InvalidArgument, "tool name is required")
	}

	// Call daemon implementation
	result, err := s.daemon.ExecuteTool(ctx, req.Name, req.InputJson, req.TimeoutMs)
	if err != nil {
		s.logger.Error("failed to execute tool", "error", err, "name", req.Name)
		return nil, status_grpc.Errorf(codes.Internal, "failed to execute tool: %v", err)
	}

	s.logger.Debug("tool execution completed",
		"name", req.Name,
		"success", result.Success,
		"duration_ms", result.DurationMs,
	)

	return &ExecuteToolResponse{
		Success:    result.Success,
		OutputJson: result.OutputJSON,
		Error:      result.Error,
		DurationMs: result.DurationMs,
	}, nil
}

// GetAvailableTools returns all available tools from the Tool Executor Service.
func (s *DaemonServer) GetAvailableTools(ctx context.Context, req *GetAvailableToolsRequest) (*GetAvailableToolsResponse, error) {
	s.logger.Debug("get available tools request received")

	// Call daemon implementation
	tools, err := s.daemon.GetAvailableTools(ctx)
	if err != nil {
		s.logger.Error("failed to get available tools", "error", err)
		return nil, status_grpc.Errorf(codes.Internal, "failed to get available tools: %v", err)
	}

	// Convert internal types to proto types
	protoTools := make([]*AvailableToolInfo, len(tools))
	for i, tool := range tools {
		var protoMetrics *ToolExecutionMetrics
		if tool.Metrics != nil {
			protoMetrics = &ToolExecutionMetrics{
				TotalCalls:     tool.Metrics.TotalCalls,
				SuccessCalls:   tool.Metrics.SuccessCalls,
				FailedCalls:    tool.Metrics.FailedCalls,
				AvgDurationMs:  tool.Metrics.AvgDurationMs,
				LastExecutedAt: tool.Metrics.LastExecutedAt,
			}
		}

		protoTools[i] = &AvailableToolInfo{
			Name:             tool.Name,
			Version:          tool.Version,
			Description:      tool.Description,
			Tags:             tool.Tags,
			InputSchemaJson:  tool.InputSchemaJSON,
			OutputSchemaJson: tool.OutputSchemaJSON,
			Status:           tool.Status,
			ErrorMessage:     tool.ErrorMessage,
			Metrics:          protoMetrics,
		}
	}

	s.logger.Debug("available tools retrieved", "count", len(protoTools))

	return &GetAvailableToolsResponse{
		Tools: protoTools,
	}, nil
}
