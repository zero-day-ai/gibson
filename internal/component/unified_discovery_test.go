package component

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Mock implementations for testing

// mockLocalTracker is a mock implementation of LocalTracker for testing.
type mockLocalTracker struct {
	discoverFunc func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error)
}

func (m *mockLocalTracker) Start(ctx context.Context, comp *Component) error {
	return nil
}

func (m *mockLocalTracker) Stop(ctx context.Context, comp *Component) error {
	return nil
}

func (m *mockLocalTracker) IsRunning(ctx context.Context, kind ComponentKind, name string) (bool, error) {
	return false, nil
}

func (m *mockLocalTracker) Discover(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, kind)
	}
	return []LocalComponentState{}, nil
}

func (m *mockLocalTracker) Cleanup(ctx context.Context) error {
	return nil
}

// mockRemoteProber is a mock implementation of RemoteProber for testing.
type mockRemoteProber struct {
	probeFunc    func(ctx context.Context, address string) (RemoteComponentState, error)
	probeAllFunc func(ctx context.Context) ([]RemoteComponentState, error)
}

func (m *mockRemoteProber) Probe(ctx context.Context, address string) (RemoteComponentState, error) {
	if m.probeFunc != nil {
		return m.probeFunc(ctx, address)
	}
	return RemoteComponentState{}, nil
}

func (m *mockRemoteProber) ProbeAll(ctx context.Context) ([]RemoteComponentState, error) {
	if m.probeAllFunc != nil {
		return m.probeAllFunc(ctx)
	}
	return []RemoteComponentState{}, nil
}

func (m *mockRemoteProber) LoadConfig(agents, tools, plugins map[string]RemoteComponentConfig) error {
	return nil
}

// Note: mockLogger is already defined in config_test.go
// We extend it here with additional methods for the full Logger interface.
func (m *mockLogger) Debugf(format string, args ...interface{}) {}
func (m *mockLogger) Infof(format string, args ...interface{})  {}
func (m *mockLogger) Errorf(format string, args ...interface{}) {}

// Test: Basic construction of UnifiedDiscovery
func TestNewDefaultUnifiedDiscovery(t *testing.T) {
	localTracker := &mockLocalTracker{}
	remoteProber := &mockRemoteProber{}
	logger := &mockLogger{}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, logger)

	if discovery == nil {
		t.Fatal("NewDefaultUnifiedDiscovery returned nil")
	}

	if discovery.localTracker != localTracker {
		t.Error("localTracker not set correctly")
	}

	if discovery.remoteProber != remoteProber {
		t.Error("remoteProber not set correctly")
	}

	if discovery.logger != logger {
		t.Error("logger not set correctly")
	}
}

// Test: NewDefaultUnifiedDiscovery with nil logger
func TestNewDefaultUnifiedDiscovery_NilLogger(t *testing.T) {
	localTracker := &mockLocalTracker{}
	remoteProber := &mockRemoteProber{}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	if discovery == nil {
		t.Fatal("NewDefaultUnifiedDiscovery returned nil")
	}

	if discovery.logger != nil {
		t.Error("logger should be nil when passed as nil")
	}
}

// Test: DiscoverAll with local-only components
func TestDiscoverAll_LocalOnly(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			switch kind {
			case ComponentKindAgent:
				return []LocalComponentState{
					{
						Kind:    ComponentKindAgent,
						Name:    "local-agent",
						PID:     1234,
						Port:    50051,
						Socket:  "/tmp/agent.sock",
						Healthy: true,
					},
				}, nil
			case ComponentKindTool:
				return []LocalComponentState{
					{
						Kind:    ComponentKindTool,
						Name:    "local-tool",
						PID:     1235,
						Port:    50052,
						Socket:  "/tmp/tool.sock",
						Healthy: true,
					},
				}, nil
			case ComponentKindPlugin:
				return []LocalComponentState{}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}

	// Verify all are local
	for _, comp := range components {
		if comp.Source != DiscoverySourceLocal {
			t.Errorf("expected local source, got %s", comp.Source)
		}
		if !comp.Healthy {
			t.Errorf("expected healthy component, got unhealthy")
		}
	}
}

// Test: DiscoverAll with remote-only components
func TestDiscoverAll_RemoteOnly(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{
					Kind:         ComponentKindAgent,
					Name:         "remote-agent",
					Address:      "agent:50051",
					Healthy:      true,
					LastCheck:    time.Now(),
					ResponseTime: 10 * time.Millisecond,
				},
				{
					Kind:         ComponentKindTool,
					Name:         "remote-tool",
					Address:      "tool:50052",
					Healthy:      true,
					LastCheck:    time.Now(),
					ResponseTime: 15 * time.Millisecond,
				},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}

	// Verify all are remote
	for _, comp := range components {
		if comp.Source != DiscoverySourceRemote {
			t.Errorf("expected remote source, got %s", comp.Source)
		}
		if !comp.Healthy {
			t.Errorf("expected healthy component, got unhealthy")
		}
	}
}

// Test: DiscoverAll with mixed local+remote (local precedence)
func TestDiscoverAll_MixedWithPrecedence(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			switch kind {
			case ComponentKindAgent:
				return []LocalComponentState{
					{
						Kind:    ComponentKindAgent,
						Name:    "k8skiller", // Same name as remote
						PID:     1234,
						Port:    50051,
						Socket:  "/tmp/k8skiller.sock",
						Healthy: true,
					},
				}, nil
			case ComponentKindTool:
				return []LocalComponentState{
					{
						Kind:    ComponentKindTool,
						Name:    "nmap", // Unique local tool
						PID:     1235,
						Port:    50052,
						Socket:  "/tmp/nmap.sock",
						Healthy: true,
					},
				}, nil
			case ComponentKindPlugin:
				return []LocalComponentState{}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{
					Kind:         ComponentKindAgent,
					Name:         "k8skiller", // Same name as local - should be shadowed
					Address:      "k8skiller:50051",
					Healthy:      true,
					LastCheck:    time.Now(),
					ResponseTime: 10 * time.Millisecond,
				},
				{
					Kind:         ComponentKindAgent,
					Name:         "davinci", // Unique remote agent
					Address:      "davinci:50053",
					Healthy:      true,
					LastCheck:    time.Now(),
					ResponseTime: 12 * time.Millisecond,
				},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, logger)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	// Expected: local k8skiller, local nmap, remote davinci = 3 components
	if len(components) != 3 {
		t.Fatalf("expected 3 components, got %d", len(components))
	}

	// Verify k8skiller is local (not remote)
	k8sKiller := findComponent(components, ComponentKindAgent, "k8skiller")
	if k8sKiller == nil {
		t.Fatal("k8skiller not found in results")
	}
	if k8sKiller.Source != DiscoverySourceLocal {
		t.Errorf("expected k8skiller to be local (precedence), got %s", k8sKiller.Source)
	}
	if k8sKiller.Address != "/tmp/k8skiller.sock" {
		t.Errorf("expected local socket address, got %s", k8sKiller.Address)
	}

	// Verify davinci is remote
	davinci := findComponent(components, ComponentKindAgent, "davinci")
	if davinci == nil {
		t.Fatal("davinci not found in results")
	}
	if davinci.Source != DiscoverySourceRemote {
		t.Errorf("expected davinci to be remote, got %s", davinci.Source)
	}

	// Verify nmap is local
	nmap := findComponent(components, ComponentKindTool, "nmap")
	if nmap == nil {
		t.Fatal("nmap not found in results")
	}
	if nmap.Source != DiscoverySourceLocal {
		t.Errorf("expected nmap to be local, got %s", nmap.Source)
	}

	// Verify warning was logged about shadowing
	if len(logger.warnings) == 0 {
		t.Error("expected warning log about local shadowing remote, got none")
	}
	foundShadowWarning := false
	for _, msg := range logger.warnings {
		if contains(msg, "shadows") && contains(msg, "k8skiller") {
			foundShadowWarning = true
			break
		}
	}
	if !foundShadowWarning {
		t.Errorf("expected shadow warning in logs, got: %v", logger.warnings)
	}
}

// Test: DiscoverAll when both local and remote fail
func TestDiscoverAll_BothFail(t *testing.T) {
	ctx := context.Background()

	localErr := errors.New("local discovery failed")
	remoteErr := errors.New("remote probe failed")

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return nil, localErr
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return nil, remoteErr
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	components, err := discovery.DiscoverAll(ctx)
	if err == nil {
		t.Fatal("expected error when both local and remote fail, got nil")
	}

	if components != nil {
		t.Errorf("expected nil components when both fail, got %v", components)
	}

	// Error should mention both failures
	errMsg := err.Error()
	if !contains(errMsg, "local") || !contains(errMsg, "remote") {
		t.Errorf("error should mention both local and remote failures, got: %s", errMsg)
	}
}

// Test: DiscoverAll when only local fails (partial success)
func TestDiscoverAll_LocalFails(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	localErr := errors.New("local discovery failed")

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return nil, localErr
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{
					Kind:    ComponentKindAgent,
					Name:    "remote-agent",
					Address: "agent:50051",
					Healthy: true,
				},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, logger)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("expected partial success when only local fails, got error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 remote component, got %d", len(components))
	}

	// Verify warning was logged
	if len(logger.warnings) == 0 {
		t.Error("expected warning log about local failure, got none")
	}
}

// Test: DiscoverAll when only remote fails (partial success)
func TestDiscoverAll_RemoteFails(t *testing.T) {
	ctx := context.Background()
	logger := &mockLogger{}

	remoteErr := errors.New("remote probe failed")

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			switch kind {
			case ComponentKindAgent:
				return []LocalComponentState{
					{
						Kind:    ComponentKindAgent,
						Name:    "local-agent",
						PID:     1234,
						Port:    50051,
						Socket:  "/tmp/agent.sock",
						Healthy: true,
					},
				}, nil
			default:
				return []LocalComponentState{}, nil
			}
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return nil, remoteErr
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, logger)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("expected partial success when only remote fails, got error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 local component, got %d", len(components))
	}

	// Verify warning was logged
	if len(logger.warnings) == 0 {
		t.Error("expected warning log about remote failure, got none")
	}
}

// Test: DiscoverAll with empty results
func TestDiscoverAll_EmptyResults(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	if len(components) != 0 {
		t.Errorf("expected 0 components, got %d", len(components))
	}

	// Should return empty slice, not nil
	if components == nil {
		t.Error("expected empty slice, got nil")
	}
}

// Test: DiscoverAgents filters correctly
func TestDiscoverAgents_Filtering(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			switch kind {
			case ComponentKindAgent:
				return []LocalComponentState{
					{Kind: ComponentKindAgent, Name: "agent1", Healthy: true},
				}, nil
			case ComponentKindTool:
				return []LocalComponentState{
					{Kind: ComponentKindTool, Name: "tool1", Healthy: true},
				}, nil
			case ComponentKindPlugin:
				return []LocalComponentState{
					{Kind: ComponentKindPlugin, Name: "plugin1", Healthy: true},
				}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{Kind: ComponentKindAgent, Name: "agent2", Healthy: true, Address: "agent2:50051"},
				{Kind: ComponentKindTool, Name: "tool2", Healthy: true, Address: "tool2:50052"},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	agents, err := discovery.DiscoverAgents(ctx)
	if err != nil {
		t.Fatalf("DiscoverAgents failed: %v", err)
	}

	// Should only return agents (agent1 and agent2)
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	for _, agent := range agents {
		if agent.Kind != ComponentKindAgent {
			t.Errorf("expected only agents, got kind %s", agent.Kind)
		}
	}
}

// Test: DiscoverTools filters correctly
func TestDiscoverTools_Filtering(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			switch kind {
			case ComponentKindAgent:
				return []LocalComponentState{
					{Kind: ComponentKindAgent, Name: "agent1", Healthy: true},
				}, nil
			case ComponentKindTool:
				return []LocalComponentState{
					{Kind: ComponentKindTool, Name: "tool1", Healthy: true},
				}, nil
			case ComponentKindPlugin:
				return []LocalComponentState{}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{Kind: ComponentKindAgent, Name: "agent2", Healthy: true, Address: "agent2:50051"},
				{Kind: ComponentKindTool, Name: "tool2", Healthy: true, Address: "tool2:50052"},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	tools, err := discovery.DiscoverTools(ctx)
	if err != nil {
		t.Fatalf("DiscoverTools failed: %v", err)
	}

	// Should only return tools (tool1 and tool2)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	for _, tool := range tools {
		if tool.Kind != ComponentKindTool {
			t.Errorf("expected only tools, got kind %s", tool.Kind)
		}
	}
}

// Test: DiscoverPlugins filters correctly
func TestDiscoverPlugins_Filtering(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			switch kind {
			case ComponentKindAgent:
				return []LocalComponentState{}, nil
			case ComponentKindTool:
				return []LocalComponentState{}, nil
			case ComponentKindPlugin:
				return []LocalComponentState{
					{Kind: ComponentKindPlugin, Name: "plugin1", Healthy: true},
				}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{Kind: ComponentKindAgent, Name: "agent1", Healthy: true, Address: "agent1:50051"},
				{Kind: ComponentKindPlugin, Name: "plugin2", Healthy: true, Address: "plugin2:50053"},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	plugins, err := discovery.DiscoverPlugins(ctx)
	if err != nil {
		t.Fatalf("DiscoverPlugins failed: %v", err)
	}

	// Should only return plugins (plugin1 and plugin2)
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}

	for _, plugin := range plugins {
		if plugin.Kind != ComponentKindPlugin {
			t.Errorf("expected only plugins, got kind %s", plugin.Kind)
		}
	}
}

// Test: GetComponent finds local component
func TestGetComponent_LocalFound(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			if kind == ComponentKindAgent {
				return []LocalComponentState{
					{
						Kind:    ComponentKindAgent,
						Name:    "k8skiller",
						PID:     1234,
						Port:    50051,
						Socket:  "/tmp/k8skiller.sock",
						Healthy: true,
					},
				}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	component, err := discovery.GetComponent(ctx, ComponentKindAgent, "k8skiller")
	if err != nil {
		t.Fatalf("GetComponent failed: %v", err)
	}

	if component == nil {
		t.Fatal("expected component, got nil")
	}

	if component.Source != DiscoverySourceLocal {
		t.Errorf("expected local source, got %s", component.Source)
	}

	if component.Name != "k8skiller" {
		t.Errorf("expected name k8skiller, got %s", component.Name)
	}
}

// Test: GetComponent finds remote component when local not available
func TestGetComponent_RemoteFound(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{
					Kind:    ComponentKindAgent,
					Name:    "davinci",
					Address: "davinci:50051",
					Healthy: true,
				},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	component, err := discovery.GetComponent(ctx, ComponentKindAgent, "davinci")
	if err != nil {
		t.Fatalf("GetComponent failed: %v", err)
	}

	if component == nil {
		t.Fatal("expected component, got nil")
	}

	if component.Source != DiscoverySourceRemote {
		t.Errorf("expected remote source, got %s", component.Source)
	}

	if component.Name != "davinci" {
		t.Errorf("expected name davinci, got %s", component.Name)
	}
}

// Test: GetComponent prefers local over remote
func TestGetComponent_LocalPrecedence(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			if kind == ComponentKindAgent {
				return []LocalComponentState{
					{
						Kind:    ComponentKindAgent,
						Name:    "k8skiller",
						PID:     1234,
						Port:    50051,
						Socket:  "/tmp/k8skiller.sock",
						Healthy: true,
					},
				}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{
					Kind:    ComponentKindAgent,
					Name:    "k8skiller",
					Address: "k8skiller:50051",
					Healthy: true,
				},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	component, err := discovery.GetComponent(ctx, ComponentKindAgent, "k8skiller")
	if err != nil {
		t.Fatalf("GetComponent failed: %v", err)
	}

	if component == nil {
		t.Fatal("expected component, got nil")
	}

	if component.Source != DiscoverySourceLocal {
		t.Errorf("expected local precedence, got %s", component.Source)
	}

	if component.Address != "/tmp/k8skiller.sock" {
		t.Errorf("expected local socket address, got %s", component.Address)
	}
}

// Test: GetComponent returns nil when not found
func TestGetComponent_NotFound(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	component, err := discovery.GetComponent(ctx, ComponentKindAgent, "nonexistent")
	if err != nil {
		t.Fatalf("GetComponent failed: %v", err)
	}

	if component != nil {
		t.Errorf("expected nil for nonexistent component, got %v", component)
	}
}

// Test: GetComponent ignores unhealthy components
func TestGetComponent_IgnoresUnhealthy(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			if kind == ComponentKindAgent {
				return []LocalComponentState{
					{
						Kind:    ComponentKindAgent,
						Name:    "k8skiller",
						PID:     1234,
						Port:    50051,
						Socket:  "/tmp/k8skiller.sock",
						Healthy: false, // Unhealthy
					},
				}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{
					Kind:    ComponentKindAgent,
					Name:    "k8skiller",
					Address: "k8skiller:50051",
					Healthy: false, // Also unhealthy
				},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	component, err := discovery.GetComponent(ctx, ComponentKindAgent, "k8skiller")
	if err != nil {
		t.Fatalf("GetComponent failed: %v", err)
	}

	// Should return nil since both are unhealthy
	if component != nil {
		t.Errorf("expected nil for unhealthy component, got %v", component)
	}
}

// Test: Only healthy components are included in DiscoverAll
func TestDiscoverAll_OnlyHealthy(t *testing.T) {
	ctx := context.Background()

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			if kind == ComponentKindAgent {
				return []LocalComponentState{
					{Kind: ComponentKindAgent, Name: "healthy", Healthy: true},
					{Kind: ComponentKindAgent, Name: "unhealthy", Healthy: false},
				}, nil
			}
			return []LocalComponentState{}, nil
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return []RemoteComponentState{
				{Kind: ComponentKindAgent, Name: "remote-healthy", Healthy: true, Address: "agent:50051"},
				{Kind: ComponentKindAgent, Name: "remote-unhealthy", Healthy: false, Address: "agent:50052"},
			}, nil
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	components, err := discovery.DiscoverAll(ctx)
	if err != nil {
		t.Fatalf("DiscoverAll failed: %v", err)
	}

	// Only healthy components should be included
	if len(components) != 2 {
		t.Fatalf("expected 2 healthy components, got %d", len(components))
	}

	for _, comp := range components {
		if !comp.Healthy {
			t.Errorf("found unhealthy component in results: %s", comp.Name)
		}
	}
}

// Test: Port extraction from remote address
func TestRemoteStateToDiscovered_PortExtraction(t *testing.T) {
	discovery := NewDefaultUnifiedDiscovery(&mockLocalTracker{}, &mockRemoteProber{}, nil)

	tests := []struct {
		name            string
		address         string
		expectedPort    int
		expectedAddress string
	}{
		{
			name:            "standard host:port",
			address:         "localhost:50051",
			expectedPort:    50051,
			expectedAddress: "localhost:50051",
		},
		{
			name:            "IP:port",
			address:         "192.168.1.100:8080",
			expectedPort:    8080,
			expectedAddress: "192.168.1.100:8080",
		},
		{
			name:            "hostname without port",
			address:         "myhost",
			expectedPort:    0,
			expectedAddress: "myhost",
		},
		{
			name:            "IPv6 with port",
			address:         "[::1]:9000",
			expectedPort:    9000,
			expectedAddress: "[::1]:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := RemoteComponentState{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Address: tt.address,
				Healthy: true,
			}

			discovered := discovery.remoteStateToDiscovered(state)

			if discovered.Address != tt.expectedAddress {
				t.Errorf("expected address %s, got %s", tt.expectedAddress, discovered.Address)
			}

			if discovered.Port != tt.expectedPort {
				t.Errorf("expected port %d, got %d", tt.expectedPort, discovered.Port)
			}
		})
	}
}

// Test: DiscoveredComponent.Validate
func TestDiscoveredComponent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		comp    DiscoveredComponent
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid local component",
			comp: DiscoveredComponent{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Source:  DiscoverySourceLocal,
				Address: "/tmp/test.sock",
				Healthy: true,
				Port:    50051,
			},
			wantErr: false,
		},
		{
			name: "valid remote component",
			comp: DiscoveredComponent{
				Kind:    ComponentKindTool,
				Name:    "test",
				Source:  DiscoverySourceRemote,
				Address: "host:50051",
				Healthy: true,
				Port:    50051,
			},
			wantErr: false,
		},
		{
			name: "invalid kind",
			comp: DiscoveredComponent{
				Kind:    "invalid",
				Name:    "test",
				Source:  DiscoverySourceLocal,
				Address: "/tmp/test.sock",
				Healthy: true,
				Port:    50051,
			},
			wantErr: true,
			errMsg:  "invalid component kind",
		},
		{
			name: "empty name",
			comp: DiscoveredComponent{
				Kind:    ComponentKindAgent,
				Name:    "",
				Source:  DiscoverySourceLocal,
				Address: "/tmp/test.sock",
				Healthy: true,
				Port:    50051,
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "invalid source",
			comp: DiscoveredComponent{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Source:  "invalid",
				Address: "/tmp/test.sock",
				Healthy: true,
				Port:    50051,
			},
			wantErr: true,
			errMsg:  "invalid discovery source",
		},
		{
			name: "empty address",
			comp: DiscoveredComponent{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Source:  DiscoverySourceLocal,
				Address: "",
				Healthy: true,
				Port:    50051,
			},
			wantErr: true,
			errMsg:  "address is required",
		},
		{
			name: "invalid port for healthy component",
			comp: DiscoveredComponent{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Source:  DiscoverySourceLocal,
				Address: "/tmp/test.sock",
				Healthy: true,
				Port:    0,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "zero port for unhealthy component is ok",
			comp: DiscoveredComponent{
				Kind:    ComponentKindAgent,
				Name:    "test",
				Source:  DiscoverySourceLocal,
				Address: "/tmp/test.sock",
				Healthy: false,
				Port:    0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.comp.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// Test: DiscoveredComponent utility methods
func TestDiscoveredComponent_UtilityMethods(t *testing.T) {
	local := DiscoveredComponent{
		Source: DiscoverySourceLocal,
	}

	if !local.IsLocal() {
		t.Error("expected IsLocal() to return true for local component")
	}
	if local.IsRemote() {
		t.Error("expected IsRemote() to return false for local component")
	}

	remote := DiscoveredComponent{
		Source: DiscoverySourceRemote,
	}

	if remote.IsLocal() {
		t.Error("expected IsLocal() to return false for remote component")
	}
	if !remote.IsRemote() {
		t.Error("expected IsRemote() to return true for remote component")
	}
}

// Test: DiscoverySource utility methods
func TestDiscoverySource_UtilityMethods(t *testing.T) {
	// Test String()
	if DiscoverySourceLocal.String() != "local" {
		t.Errorf("expected 'local', got %s", DiscoverySourceLocal.String())
	}
	if DiscoverySourceRemote.String() != "remote" {
		t.Errorf("expected 'remote', got %s", DiscoverySourceRemote.String())
	}

	// Test IsValid()
	if !DiscoverySourceLocal.IsValid() {
		t.Error("DiscoverySourceLocal should be valid")
	}
	if !DiscoverySourceRemote.IsValid() {
		t.Error("DiscoverySourceRemote should be valid")
	}

	invalid := DiscoverySource("invalid")
	if invalid.IsValid() {
		t.Error("invalid source should not be valid")
	}
}

// Test: DiscoverAgents with error propagation
func TestDiscoverAgents_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	localErr := errors.New("local failed")
	remoteErr := errors.New("remote failed")

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return nil, localErr
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return nil, remoteErr
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	agents, err := discovery.DiscoverAgents(ctx)
	if err == nil {
		t.Fatal("expected error when both sources fail")
	}
	if agents != nil {
		t.Errorf("expected nil agents, got %v", agents)
	}
}

// Test: DiscoverTools with error propagation
func TestDiscoverTools_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	localErr := errors.New("local failed")
	remoteErr := errors.New("remote failed")

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return nil, localErr
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return nil, remoteErr
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	tools, err := discovery.DiscoverTools(ctx)
	if err == nil {
		t.Fatal("expected error when both sources fail")
	}
	if tools != nil {
		t.Errorf("expected nil tools, got %v", tools)
	}
}

// Test: DiscoverPlugins with error propagation
func TestDiscoverPlugins_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	localErr := errors.New("local failed")
	remoteErr := errors.New("remote failed")

	localTracker := &mockLocalTracker{
		discoverFunc: func(ctx context.Context, kind ComponentKind) ([]LocalComponentState, error) {
			return nil, localErr
		},
	}

	remoteProber := &mockRemoteProber{
		probeAllFunc: func(ctx context.Context) ([]RemoteComponentState, error) {
			return nil, remoteErr
		},
	}

	discovery := NewDefaultUnifiedDiscovery(localTracker, remoteProber, nil)

	plugins, err := discovery.DiscoverPlugins(ctx)
	if err == nil {
		t.Fatal("expected error when both sources fail")
	}
	if plugins != nil {
		t.Errorf("expected nil plugins, got %v", plugins)
	}
}

// Helper functions

// findComponent finds a component by kind and name in a slice.
func findComponent(components []DiscoveredComponent, kind ComponentKind, name string) *DiscoveredComponent {
	for i := range components {
		if components[i].Kind == kind && components[i].Name == name {
			return &components[i]
		}
	}
	return nil
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
