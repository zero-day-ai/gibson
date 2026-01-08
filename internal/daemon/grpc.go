package daemon

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/attack"
	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/daemon/api"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
	"google.golang.org/grpc"
)

// startGRPCServer creates and starts the gRPC server.
//
// This method creates a gRPC server, registers the daemon service,
// and starts listening on the configured address in a goroutine.
func (d *daemonImpl) startGRPCServer(ctx context.Context) error {
	// Create listener
	listener, err := net.Listen("tcp", d.grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", d.grpcAddr, err)
	}

	// Create gRPC server
	srv := grpc.NewServer()
	d.grpcServer = srv

	// Create and register daemon service
	daemonSvc := api.NewDaemonServer(d, d.logger)
	api.RegisterDaemonServiceServer(srv, daemonSvc)

	// Start serving in goroutine
	go func() {
		d.logger.Info("gRPC server listening", "address", d.grpcAddr)
		if err := srv.Serve(listener); err != nil {
			d.logger.Error("gRPC server error", "error", err)
		}
	}()

	return nil
}

// Implementation of api.DaemonInterface for delegation from gRPC server.
// These methods delegate to the daemon's internal services.

// Status implements the api.DaemonInterface.Status method.
// It returns the daemon status in the format expected by the gRPC API.
func (d *daemonImpl) Status() (api.DaemonStatus, error) {
	// Get the internal status
	internalStatus, err := d.status()
	if err != nil {
		return api.DaemonStatus{}, err
	}

	// Convert to API status format
	return api.DaemonStatus{
		Running:            internalStatus.Running,
		PID:                int32(internalStatus.PID),
		StartTime:          internalStatus.StartTime,
		Uptime:             internalStatus.Uptime,
		GRPCAddress:        internalStatus.GRPCAddress,
		RegistryType:       internalStatus.RegistryType,
		RegistryAddr:       internalStatus.RegistryAddr,
		CallbackAddr:       internalStatus.CallbackAddr,
		AgentCount:         int32(internalStatus.AgentCount),
		MissionCount:       int32(internalStatus.MissionCount),
		ActiveMissionCount: int32(internalStatus.ActiveCount),
	}, nil
}

// ListAgents returns all registered agents from the registry.
func (d *daemonImpl) ListAgents(ctx context.Context, kind string) ([]api.AgentInfoInternal, error) {
	d.logger.Debug("ListAgents called", "kind", kind)

	// Query registry for all agents
	agents, err := d.registryAdapter.ListAgents(ctx)
	if err != nil {
		d.logger.Error("failed to list agents from registry", "error", err)
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// Convert registry.AgentInfo to api.AgentInfoInternal
	result := make([]api.AgentInfoInternal, len(agents))
	for i, agent := range agents {
		// Use first endpoint if available, empty string otherwise
		endpoint := ""
		if len(agent.Endpoints) > 0 {
			endpoint = agent.Endpoints[0]
		}

		// Determine health status - if agent has instances, it's healthy
		health := "healthy"
		if agent.Instances == 0 {
			health = "unknown"
		}

		result[i] = api.AgentInfoInternal{
			ID:           agent.Name, // Use name as ID
			Name:         agent.Name,
			Kind:         "agent", // Default kind, could be enhanced with metadata
			Version:      agent.Version,
			Endpoint:     endpoint,
			Capabilities: agent.Capabilities,
			Health:       health,
			LastSeen:     time.Now(), // TODO: Track actual last seen time in registry
		}
	}

	d.logger.Debug("listed agents from registry", "count", len(result))
	return result, nil
}

// GetAgentStatus returns status for a specific agent.
func (d *daemonImpl) GetAgentStatus(ctx context.Context, agentID string) (api.AgentStatusInternal, error) {
	d.logger.Debug("GetAgentStatus called", "agent_id", agentID)

	// Query registry for all agents
	agents, err := d.registryAdapter.ListAgents(ctx)
	if err != nil {
		d.logger.Error("failed to query registry for agent status", "error", err, "agent_id", agentID)
		return api.AgentStatusInternal{}, fmt.Errorf("failed to query registry: %w", err)
	}

	// Find the specific agent by ID (using name as ID)
	for _, agent := range agents {
		if agent.Name == agentID {
			// Use first endpoint if available
			endpoint := ""
			if len(agent.Endpoints) > 0 {
				endpoint = agent.Endpoints[0]
			}

			// Determine health status
			health := "healthy"
			if agent.Instances == 0 {
				health = "unknown"
			}

			// Build agent info
			agentInfo := api.AgentInfoInternal{
				ID:           agent.Name,
				Name:         agent.Name,
				Kind:         "agent",
				Version:      agent.Version,
				Endpoint:     endpoint,
				Capabilities: agent.Capabilities,
				Health:       health,
				LastSeen:     time.Now(), // TODO: Track actual last seen time
			}

			// Build agent status
			status := api.AgentStatusInternal{
				Agent:         agentInfo,
				Active:        agent.Instances > 0,
				CurrentTask:   "", // TODO: Track current task when mission execution is implemented
				TaskStartTime: time.Time{},
			}

			d.logger.Debug("found agent status", "agent_id", agentID, "instances", agent.Instances)
			return status, nil
		}
	}

	// Agent not found
	d.logger.Debug("agent not found in registry", "agent_id", agentID)
	return api.AgentStatusInternal{}, fmt.Errorf("agent not found: %s", agentID)
}

// ListTools returns all registered tools from the registry.
func (d *daemonImpl) ListTools(ctx context.Context) ([]api.ToolInfoInternal, error) {
	d.logger.Debug("ListTools called")

	// Query registry for all tools
	tools, err := d.registryAdapter.ListTools(ctx)
	if err != nil {
		d.logger.Error("failed to list tools from registry", "error", err)
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	// Convert registry.ToolInfo to api.ToolInfoInternal
	result := make([]api.ToolInfoInternal, len(tools))
	for i, tool := range tools {
		// Use first endpoint if available
		endpoint := ""
		if len(tool.Endpoints) > 0 {
			endpoint = tool.Endpoints[0]
		}

		// Determine health status
		health := "healthy"
		if tool.Instances == 0 {
			health = "unknown"
		}

		result[i] = api.ToolInfoInternal{
			ID:          tool.Name, // Use name as ID
			Name:        tool.Name,
			Version:     tool.Version,
			Endpoint:    endpoint,
			Description: tool.Description,
			Health:      health,
			LastSeen:    time.Now(), // TODO: Track actual last seen time
		}
	}

	d.logger.Debug("listed tools from registry", "count", len(result))
	return result, nil
}

// ListPlugins returns all registered plugins from the registry.
func (d *daemonImpl) ListPlugins(ctx context.Context) ([]api.PluginInfoInternal, error) {
	d.logger.Debug("ListPlugins called")

	// Query registry for all plugins
	plugins, err := d.registryAdapter.ListPlugins(ctx)
	if err != nil {
		d.logger.Error("failed to list plugins from registry", "error", err)
		return nil, fmt.Errorf("failed to list plugins: %w", err)
	}

	// Convert registry.PluginInfo to api.PluginInfoInternal
	result := make([]api.PluginInfoInternal, len(plugins))
	for i, plugin := range plugins {
		// Use first endpoint if available
		endpoint := ""
		if len(plugin.Endpoints) > 0 {
			endpoint = plugin.Endpoints[0]
		}

		// Determine health status
		health := "healthy"
		if plugin.Instances == 0 {
			health = "unknown"
		}

		result[i] = api.PluginInfoInternal{
			ID:          plugin.Name, // Use name as ID
			Name:        plugin.Name,
			Version:     plugin.Version,
			Endpoint:    endpoint,
			Description: plugin.Description,
			Health:      health,
			LastSeen:    time.Now(), // TODO: Track actual last seen time
		}
	}

	d.logger.Debug("listed plugins from registry", "count", len(result))
	return result, nil
}

// RunMission starts a mission and returns an event channel.
func (d *daemonImpl) RunMission(ctx context.Context, workflowPath string, missionID string, variables map[string]string, memoryContinuity string) (<-chan api.MissionEventData, error) {
	return d.RunMissionWithManager(ctx, workflowPath, missionID, variables, memoryContinuity)
}

// StopMission stops a running mission.
func (d *daemonImpl) StopMission(ctx context.Context, missionID string, force bool) error {
	d.logger.Info("StopMission called", "mission_id", missionID, "force", force)

	// Validate mission ID
	if missionID == "" {
		return fmt.Errorf("mission ID cannot be empty")
	}

	// Lock the missions map to check if mission is running
	d.missionsMu.Lock()
	cancelFunc, exists := d.activeMissions[missionID]
	if !exists {
		d.missionsMu.Unlock()
		// Mission is not running - check if it exists in the store
		_, err := d.missionStore.Get(ctx, types.ID(missionID))
		if err != nil {
			// Mission not found in store either
			d.logger.Warn("mission not found", "mission_id", missionID)
			return fmt.Errorf("mission not found: %s", missionID)
		}
		// Mission exists but is not running
		d.logger.Info("mission is not currently running", "mission_id", missionID)
		return fmt.Errorf("mission is not currently running: %s", missionID)
	}

	// Remove from active missions immediately to prevent duplicate stop requests
	delete(d.activeMissions, missionID)
	d.missionsMu.Unlock()

	// Cancel the mission context to trigger graceful shutdown
	d.logger.Info("cancelling mission execution", "mission_id", missionID, "force", force)
	cancelFunc()

	// Update mission status in the store
	missionObj, err := d.missionStore.Get(ctx, types.ID(missionID))
	if err != nil {
		d.logger.Error("failed to get mission for status update", "error", err, "mission_id", missionID)
		// Continue anyway - the cancellation was successful
	} else {
		// Update mission status to cancelled
		missionObj.Status = mission.MissionStatusCancelled
		completedAt := time.Now()
		missionObj.CompletedAt = &completedAt
		if missionObj.Metrics != nil {
			missionObj.Metrics.Duration = completedAt.Sub(missionObj.Metrics.StartedAt)
		}

		if err := d.missionStore.Update(ctx, missionObj); err != nil {
			d.logger.Error("failed to update mission status", "error", err, "mission_id", missionID)
		}
	}

	// Emit mission stopped event if event bus is available
	if d.eventBus != nil {
		event := api.EventData{
			EventType: "mission_stopped",
			Timestamp: time.Now(),
			Source:    "daemon",
			MissionEvent: &api.MissionEventData{
				EventType: "mission_stopped",
				Timestamp: time.Now(),
				MissionID: missionID,
				Message:   fmt.Sprintf("Mission %s stopped (force=%t)", missionID, force),
			},
		}
		if err := d.eventBus.Publish(ctx, event); err != nil {
			d.logger.Warn("failed to publish mission stopped event", "error", err)
		}
	}

	d.logger.Info("mission stopped successfully", "mission_id", missionID)
	return nil
}

// ListMissions returns mission list.

// RunAttack executes an attack and returns an event channel.
func (d *daemonImpl) RunAttack(ctx context.Context, req api.AttackRequest) (<-chan api.AttackEventData, error) {
	d.logger.Info("RunAttack called",
		"target", req.Target,
		"attack_type", req.AttackType,
		"agent_id", req.AgentID)

	// Validate request
	if err := d.validateAttackRequest(req); err != nil {
		d.logger.Error("invalid attack request", "error", err)
		return nil, fmt.Errorf("invalid attack request: %w", err)
	}

	// Check if attack runner is available
	if d.attackRunner == nil {
		d.logger.Error("attack runner not initialized")
		return nil, fmt.Errorf("attack execution not available: runner not initialized")
	}

	// Convert API request to attack options
	attackOpts, err := d.buildAttackOptions(req)
	if err != nil {
		d.logger.Error("failed to build attack options", "error", err)
		return nil, fmt.Errorf("failed to build attack options: %w", err)
	}

	// Create event channel for streaming attack progress
	eventChan := make(chan api.AttackEventData, 100)

	// Execute attack in goroutine
	go func() {
		defer close(eventChan)

		// Generate unique attack ID
		attackID := types.NewID().String()

		// Send attack started event with resolved target URL
		eventChan <- api.AttackEventData{
			EventType: "attack.started",
			Timestamp: time.Now(),
			AttackID:  attackID,
			Message:   fmt.Sprintf("Starting attack on %s with agent %s", attackOpts.TargetURL, req.AgentID),
		}

		d.logger.Info("executing attack",
			"attack_id", attackID,
			"target_url", attackOpts.TargetURL,
			"target_name", attackOpts.TargetName,
			"agent", attackOpts.AgentName)

		// Execute attack through runner
		result, err := d.attackRunner.Run(ctx, attackOpts)
		if err != nil {
			d.logger.Error("attack execution failed", "error", err, "attack_id", attackID)
			eventChan <- api.AttackEventData{
				EventType: "attack.failed",
				Timestamp: time.Now(),
				AttackID:  attackID,
				Message:   "Attack execution failed",
				Error:     err.Error(),
			}
			return
		}

		// Send progress events for findings
		for _, f := range result.Findings {
			eventChan <- api.AttackEventData{
				EventType: "attack.finding",
				Timestamp: time.Now(),
				AttackID:  attackID,
				Message:   fmt.Sprintf("Found %s severity finding: %s", f.Severity, f.Title),
				Finding: &api.FindingData{
					ID:          f.ID.String(),
					Title:       f.Title,
					Severity:    string(f.Severity),
					Category:    f.Category,
					Description: f.Description,
					Technique:   "", // Not available in EnhancedFinding
					Evidence:    formatEvidence(f.Evidence),
					Timestamp:   f.CreatedAt,
				},
			}
		}

		// Send attack completed event with typed OperationResult
		now := time.Now()
		startTime := now.Add(-result.Duration)

		// Create typed operation result
		operationResult := &api.OperationResult{
			Status:        string(result.Status),
			DurationMs:    result.Duration.Milliseconds(),
			StartedAt:     startTime.UnixMilli(),
			CompletedAt:   now.UnixMilli(),
			TurnsUsed:     int32(result.TurnsUsed),
			TokensUsed:    result.TokensUsed,
			FindingsCount: int32(len(result.Findings)),
		}

		// Populate severity counts from FindingsBySeverity map
		if count, ok := result.FindingsBySeverity["critical"]; ok {
			operationResult.CriticalCount = int32(count)
		}
		if count, ok := result.FindingsBySeverity["high"]; ok {
			operationResult.HighCount = int32(count)
		}
		if count, ok := result.FindingsBySeverity["medium"]; ok {
			operationResult.MediumCount = int32(count)
		}
		if count, ok := result.FindingsBySeverity["low"]; ok {
			operationResult.LowCount = int32(count)
		}

		// Add error information if present
		if result.Error != nil {
			operationResult.ErrorMessage = result.Error.Error()
		}

		eventChan <- api.AttackEventData{
			EventType: "attack.completed",
			Timestamp: now,
			AttackID:  attackID,
			Message:   fmt.Sprintf("Attack completed: %d findings discovered", len(result.Findings)),
			Data:      "", // Empty - using typed Result now
			Result:    operationResult,
		}

		d.logger.Info("attack completed",
			"attack_id", attackID,
			"status", result.Status,
			"findings", len(result.Findings),
			"duration", result.Duration)
	}()

	return eventChan, nil
}

// validateAttackRequest validates the attack request parameters.
func (d *daemonImpl) validateAttackRequest(req api.AttackRequest) error {
	// Require either target or target_name
	if req.Target == "" && req.TargetName == "" {
		return fmt.Errorf("either target or target_name is required")
	}

	// Don't allow both to be set (user should choose one approach)
	if req.Target != "" && req.TargetName != "" {
		return fmt.Errorf("cannot specify both target and target_name")
	}

	if req.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	return nil
}

// buildAttackOptions converts API AttackRequest to internal AttackOptions.
func (d *daemonImpl) buildAttackOptions(req api.AttackRequest) (*attack.AttackOptions, error) {
	opts := attack.NewAttackOptions()

	// Handle target resolution: prefer target_name lookup, fall back to inline target
	if req.TargetName != "" {
		// Look up target from database by name
		target, err := d.targetStore.GetByName(context.Background(), req.TargetName)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup target '%s': %w", req.TargetName, err)
		}

		// Extract URL from connection JSON
		targetURL := target.GetURL()
		if targetURL == "" {
			return nil, fmt.Errorf("target '%s' has no URL configured", req.TargetName)
		}

		// Set target options from stored target
		// Only set TargetURL - the name was used for lookup, URL is the resolved value
		opts.TargetURL = targetURL
		opts.TargetType = types.TargetType(target.Type)

		// Set credential if target has one configured
		if target.CredentialID != nil {
			opts.Credential = target.CredentialID.String()
		}
	} else {
		// Use inline target URL (backward compatibility)
		opts.TargetURL = req.Target
	}

	opts.AgentName = req.AgentID

	// Map attack_type to agent configuration
	// For now, we use the agent_id directly, but attack_type could be used
	// to select a default agent or configure the attack strategy
	if req.AttackType != "" {
		opts.Goal = fmt.Sprintf("Execute %s attack", req.AttackType)
	}

	// Apply payload filter if specified
	if req.PayloadFilter != "" {
		opts.PayloadCategory = req.PayloadFilter
	}

	// Apply additional options from the options map
	if req.Options != nil {
		if maxTurns, ok := req.Options["max_turns"]; ok {
			var turns int
			fmt.Sscanf(maxTurns, "%d", &turns)
			opts.MaxTurns = turns
		}

		if timeout, ok := req.Options["timeout"]; ok {
			var duration time.Duration
			duration, err := time.ParseDuration(timeout)
			if err == nil {
				opts.Timeout = duration
			}
		}

		if verbose, ok := req.Options["verbose"]; ok && verbose == "true" {
			opts.Verbose = true
		}

		if dryRun, ok := req.Options["dry_run"]; ok && dryRun == "true" {
			opts.DryRun = true
		}
	}

	return opts, nil
}

// Subscribe establishes an event stream.
func (d *daemonImpl) Subscribe(ctx context.Context, eventTypes []string, missionID string) (<-chan api.EventData, error) {
	d.logger.Info("Subscribe called", "event_types", eventTypes, "mission_id", missionID)

	// Subscribe to events from the event bus
	eventChan, cleanup := d.eventBus.Subscribe(ctx, eventTypes, missionID)

	// Start a goroutine to handle cleanup when context is cancelled
	go func() {
		<-ctx.Done()
		cleanup()
		d.logger.Info("subscription cleanup completed", "mission_id", missionID)
	}()

	return eventChan, nil
}

// formatEvidence converts a slice of Evidence to a string representation.
func formatEvidence(evidence []agent.Evidence) string {
	if len(evidence) == 0 {
		return ""
	}
	var parts []string
	for _, e := range evidence {
		parts = append(parts, fmt.Sprintf("[%s] %s", e.Type, e.Description))
	}
	return strings.Join(parts, "; ")
}

// StartComponent starts a component by kind and name.
func (d *daemonImpl) StartComponent(ctx context.Context, kind string, name string) (api.StartComponentResult, error) {
	d.logger.Info("StartComponent called", "kind", kind, "name", name)

	// Validate kind
	var componentKind component.ComponentKind
	switch kind {
	case "agent":
		componentKind = component.ComponentKindAgent
	case "tool":
		componentKind = component.ComponentKindTool
	case "plugin":
		componentKind = component.ComponentKindPlugin
	default:
		return api.StartComponentResult{}, fmt.Errorf("invalid component kind: %s", kind)
	}

	// Get component from database
	dao := database.NewComponentDAO(d.db)
	comp, err := dao.GetByName(ctx, componentKind, name)
	if err != nil {
		d.logger.Error("failed to get component from database", "error", err, "kind", kind, "name", name)
		return api.StartComponentResult{}, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		d.logger.Warn("component not found in database", "kind", kind, "name", name)
		return api.StartComponentResult{}, fmt.Errorf("component '%s' not found", name)
	}

	// Get registry from registry manager
	reg := d.registry.Registry()
	if reg == nil {
		d.logger.Error("registry not available")
		return api.StartComponentResult{}, fmt.Errorf("registry not started")
	}

	// Check if already running by querying registry
	instances, err := reg.Discover(ctx, string(componentKind), name)
	if err != nil {
		d.logger.Error("failed to query registry", "error", err, "kind", kind, "name", name)
		return api.StartComponentResult{}, fmt.Errorf("failed to query registry: %w", err)
	}
	if len(instances) > 0 {
		d.logger.Warn("component already running", "kind", kind, "name", name, "instances", len(instances))
		return api.StartComponentResult{}, fmt.Errorf("component '%s' is already running (%d instance(s) found in registry)", name, len(instances))
	}

	// Get registry endpoint from registry manager
	registryEndpoint := d.registry.Status().Endpoint

	// Get home directory from config
	homeDir := d.config.Core.HomeDir

	// Start component process
	port, pid, logPath, err := startComponentProcess(ctx, comp, reg, registryEndpoint, homeDir)
	if err != nil {
		d.logger.Error("failed to start component process", "error", err, "kind", kind, "name", name)
		return api.StartComponentResult{}, fmt.Errorf("failed to start component: %w", err)
	}

	// Update component status in database
	comp.PID = pid
	comp.Port = port
	comp.UpdateStatus(component.ComponentStatusRunning)
	if err := dao.UpdateStatus(ctx, comp.ID, comp.Status, comp.PID, comp.Port); err != nil {
		// Log warning but don't fail - component is running
		d.logger.Warn("failed to update component status in database", "error", err, "kind", kind, "name", name)
	}

	d.logger.Info("component started successfully", "kind", kind, "name", name, "pid", pid, "port", port)

	return api.StartComponentResult{
		PID:     pid,
		Port:    port,
		LogPath: logPath,
	}, nil
}

// StopComponent stops a component by kind and name.
func (d *daemonImpl) StopComponent(ctx context.Context, kind string, name string, force bool) (api.StopComponentResult, error) {
	d.logger.Info("StopComponent called", "kind", kind, "name", name, "force", force)

	// Validate kind
	var componentKind component.ComponentKind
	switch kind {
	case "agent":
		componentKind = component.ComponentKindAgent
	case "tool":
		componentKind = component.ComponentKindTool
	case "plugin":
		componentKind = component.ComponentKindPlugin
	default:
		return api.StopComponentResult{}, fmt.Errorf("invalid component kind: %s", kind)
	}

	// Get component from database
	dao := database.NewComponentDAO(d.db)
	comp, err := dao.GetByName(ctx, componentKind, name)
	if err != nil {
		d.logger.Error("failed to get component from database", "error", err, "kind", kind, "name", name)
		return api.StopComponentResult{}, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		d.logger.Warn("component not found in database", "kind", kind, "name", name)
		return api.StopComponentResult{}, fmt.Errorf("component '%s' not found", name)
	}

	// Get registry from registry manager
	reg := d.registry.Registry()
	if reg == nil {
		d.logger.Error("registry not available")
		return api.StopComponentResult{}, fmt.Errorf("registry not started")
	}

	// Query registry for running instances
	instances, err := reg.Discover(ctx, string(componentKind), name)
	if err != nil {
		d.logger.Error("failed to query registry", "error", err, "kind", kind, "name", name)
		return api.StopComponentResult{}, fmt.Errorf("failed to query registry: %w", err)
	}
	if len(instances) == 0 {
		d.logger.Warn("component not running", "kind", kind, "name", name)
		return api.StopComponentResult{}, fmt.Errorf("component '%s' is not running (no instances found in registry)", name)
	}

	// Stop all instances
	var lastErr error
	stoppedCount := 0
	for _, instance := range instances {
		if err := stopComponentProcess(ctx, instance, reg, force); err != nil {
			d.logger.Warn("failed to stop instance", "error", err, "instance_id", instance.InstanceID)
			lastErr = err
		} else {
			stoppedCount++
		}
	}

	if stoppedCount == 0 && lastErr != nil {
		d.logger.Error("failed to stop any instances", "error", lastErr, "kind", kind, "name", name)
		return api.StopComponentResult{}, fmt.Errorf("failed to stop any instances: %w", lastErr)
	}

	// Update component status in database
	comp.UpdateStatus(component.ComponentStatusStopped)
	comp.PID = 0
	comp.Port = 0
	if err := dao.UpdateStatus(ctx, comp.ID, comp.Status, comp.PID, comp.Port); err != nil {
		// Log warning but don't fail - component is stopped
		d.logger.Warn("failed to update component status in database", "error", err, "kind", kind, "name", name)
	}

	d.logger.Info("component stopped successfully", "kind", kind, "name", name, "stopped", stoppedCount, "total", len(instances))

	return api.StopComponentResult{
		StoppedCount: stoppedCount,
		TotalCount:   len(instances),
	}, nil
}

// PauseMission pauses a running mission at the next clean checkpoint boundary.
func (d *daemonImpl) PauseMission(ctx context.Context, missionID string, force bool) error {
	d.logger.Info("PauseMission called", "mission_id", missionID, "force", force)

	// Validate mission ID
	if missionID == "" {
		return fmt.Errorf("mission ID cannot be empty")
	}

	// Check if mission manager is available
	if d.missionManager == nil {
		d.logger.Error("mission manager not initialized")
		return fmt.Errorf("mission manager not initialized")
	}

	// Call mission manager's pause method
	if err := d.missionManager.Pause(ctx, missionID, force); err != nil {
		d.logger.Error("failed to pause mission", "error", err, "mission_id", missionID)
		return fmt.Errorf("failed to pause mission: %w", err)
	}

	d.logger.Info("mission paused successfully", "mission_id", missionID)
	return nil
}

// ResumeMission resumes a paused mission from its last checkpoint.
func (d *daemonImpl) ResumeMission(ctx context.Context, missionID string) (<-chan api.MissionEventData, error) {
	d.logger.Info("ResumeMission called", "mission_id", missionID)

	// Validate mission ID
	if missionID == "" {
		return nil, fmt.Errorf("mission ID cannot be empty")
	}

	// Check if mission manager is available
	if d.missionManager == nil {
		d.logger.Error("mission manager not initialized")
		return nil, fmt.Errorf("mission manager not initialized")
	}

	// Call mission manager's resume method
	eventChan, err := d.missionManager.Resume(ctx, missionID)
	if err != nil {
		d.logger.Error("failed to resume mission", "error", err, "mission_id", missionID)
		return nil, fmt.Errorf("failed to resume mission: %w", err)
	}

	d.logger.Info("mission resume started", "mission_id", missionID)
	return eventChan, nil
}

// GetMissionHistory returns all runs for a mission name.
func (d *daemonImpl) GetMissionHistory(ctx context.Context, name string, limit int, offset int) ([]api.MissionRunData, int, error) {
	d.logger.Debug("GetMissionHistory called", "name", name, "limit", limit, "offset", offset)

	// Validate name
	if name == "" {
		return nil, 0, fmt.Errorf("mission name cannot be empty")
	}

	// TODO: Implement when mission run linker is added (Task 13)
	// This will query the mission store for all missions with the given name
	// For now, return empty results as the store doesn't support name filtering
	// This will be properly implemented once the run linker is integrated
	missions := []*mission.Mission{}
	total := 0

	// TODO: Once run linker is properly wired, use:
	// missions, err := d.infrastructure.runLinker.GetRunHistory(ctx, name)
	// if err != nil {
	//     return nil, 0, err
	// }

	// Convert missions to MissionRunData
	runs := make([]api.MissionRunData, len(missions))
	for i, m := range missions {
		completedAt := int64(0)
		if m.CompletedAt != nil {
			completedAt = m.CompletedAt.Unix()
		}

		runs[i] = api.MissionRunData{
			MissionID:     m.ID.String(),
			RunNumber:     1, // TODO: Will be populated when run_number field is added
			Status:        string(m.Status),
			CreatedAt:     m.CreatedAt.Unix(),
			CompletedAt:   completedAt,
			FindingsCount: m.FindingsCount,
			PreviousRunID: "", // TODO: Will be populated when previous_run_id field is added
		}
	}

	d.logger.Debug("mission history retrieved", "name", name, "count", len(runs), "total", total)
	return runs, total, nil
}

// GetMissionCheckpoints returns all checkpoints for a mission.
func (d *daemonImpl) GetMissionCheckpoints(ctx context.Context, missionID string) ([]api.CheckpointData, error) {
	d.logger.Debug("GetMissionCheckpoints called", "mission_id", missionID)

	// Validate mission ID
	if missionID == "" {
		return nil, fmt.Errorf("mission ID cannot be empty")
	}

	// Get the mission from the store
	m, err := d.missionStore.Get(ctx, types.ID(missionID))
	if err != nil {
		d.logger.Error("failed to get mission", "error", err, "mission_id", missionID)
		return nil, fmt.Errorf("failed to get mission: %w", err)
	}

	// Check if mission has a checkpoint
	if m.Checkpoint == nil {
		d.logger.Debug("no checkpoints found for mission", "mission_id", missionID)
		return []api.CheckpointData{}, nil
	}

	// Convert checkpoint to CheckpointData
	// Calculate total nodes from metrics if available
	totalNodes := 0
	findingsCount := 0
	if m.Metrics != nil {
		totalNodes = m.Metrics.TotalNodes
		findingsCount = m.Metrics.TotalFindings
	}

	checkpoint := api.CheckpointData{
		CheckpointID:   m.Checkpoint.ID.String(),
		CreatedAt:      m.Checkpoint.CheckpointedAt.Unix(),
		CompletedNodes: len(m.Checkpoint.CompletedNodes),
		TotalNodes:     totalNodes,
		FindingsCount:  findingsCount,
		Version:        m.Checkpoint.Version,
	}

	d.logger.Debug("mission checkpoints retrieved", "mission_id", missionID, "count", 1)
	return []api.CheckpointData{checkpoint}, nil
}
