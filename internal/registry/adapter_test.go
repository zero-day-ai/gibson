package registry

import (
	"context"
	"testing"
	"time"

	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// Note: mockRegistry is defined in load_balancer_test.go

func TestNewRegistryAdapter(t *testing.T) {
	reg := newMockRegistry()
	adapter := NewRegistryAdapter(reg)

	if adapter == nil {
		t.Fatal("NewRegistryAdapter returned nil")
	}

	if adapter.registry != reg {
		t.Error("adapter.registry does not match provided registry")
	}

	if adapter.loadBalancer == nil {
		t.Error("adapter.loadBalancer is nil")
	}

	if adapter.pool == nil {
		t.Error("adapter.pool is nil")
	}
}

func TestRegistryAdapter_ListAgents_Empty(t *testing.T) {
	reg := newMockRegistry()
	adapter := NewRegistryAdapter(reg)
	defer adapter.Close()

	ctx := context.Background()
	agents, err := adapter.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	if len(agents) != 0 {
		t.Errorf("Expected 0 agents, got %d", len(agents))
	}
}

func TestRegistryAdapter_ListAgents_WithAgents(t *testing.T) {
	reg := newMockRegistry()
	adapter := NewRegistryAdapter(reg)
	defer adapter.Close()

	ctx := context.Background()

	// Register some test agents
	agent1 := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "davinci",
		Version:    "1.0.0",
		InstanceID: "instance-1",
		Endpoint:   "localhost:50051",
		Metadata: map[string]string{
			"description":     "Jailbreak testing agent",
			"capabilities":    "jailbreak,prompt_injection",
			"target_types":    "llm_chat,llm_api",
			"technique_types": "prompt_injection",
		},
		StartedAt: time.Now(),
	}

	agent2 := sdkregistry.ServiceInfo{
		Kind:       "agent",
		Name:       "davinci",
		Version:    "1.0.0",
		InstanceID: "instance-2",
		Endpoint:   "localhost:50052",
		Metadata: map[string]string{
			"description":     "Jailbreak testing agent",
			"capabilities":    "jailbreak,prompt_injection",
			"target_types":    "llm_chat,llm_api",
			"technique_types": "prompt_injection",
		},
		StartedAt: time.Now(),
	}

	if err := reg.Register(ctx, agent1); err != nil {
		t.Fatalf("Failed to register agent1: %v", err)
	}

	if err := reg.Register(ctx, agent2); err != nil {
		t.Fatalf("Failed to register agent2: %v", err)
	}

	// List agents
	agents, err := adapter.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("Expected 1 agent (aggregated), got %d", len(agents))
	}

	if agents[0].Name != "davinci" {
		t.Errorf("Expected agent name 'davinci', got '%s'", agents[0].Name)
	}

	if agents[0].Instances != 2 {
		t.Errorf("Expected 2 instances, got %d", agents[0].Instances)
	}

	if len(agents[0].Endpoints) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(agents[0].Endpoints))
	}

	if len(agents[0].Capabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(agents[0].Capabilities))
	}
}

func TestRegistryAdapter_DiscoverAgent_NotFound(t *testing.T) {
	reg := newMockRegistry()
	adapter := NewRegistryAdapter(reg)
	defer adapter.Close()

	ctx := context.Background()

	// Try to discover non-existent agent
	_, err := adapter.DiscoverAgent(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent agent, got nil")
	}

	// Check that it's the right error type
	var notFoundErr *AgentNotFoundError
	if !isAgentNotFoundError(err, &notFoundErr) {
		t.Errorf("Expected AgentNotFoundError, got %T: %v", err, err)
	}

	if notFoundErr != nil && notFoundErr.Name != "nonexistent" {
		t.Errorf("Expected error name 'nonexistent', got '%s'", notFoundErr.Name)
	}
}

func TestAgentNotFoundError_Message(t *testing.T) {
	tests := []struct {
		name      string
		err       *AgentNotFoundError
		wantMatch string
	}{
		{
			name: "no available agents",
			err: &AgentNotFoundError{
				Name:      "test",
				Available: []string{},
			},
			wantMatch: "agent 'test' not found (no agents registered)",
		},
		{
			name: "with available agents",
			err: &AgentNotFoundError{
				Name:      "test",
				Available: []string{"davinci", "k8skiller"},
			},
			wantMatch: "agent 'test' not found (available: davinci, k8skiller)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMatch {
				t.Errorf("Error() = %q, want %q", got, tt.wantMatch)
			}
		})
	}
}

func TestRegistryUnavailableError_Unwrap(t *testing.T) {
	originalErr := context.DeadlineExceeded
	err := &RegistryUnavailableError{Cause: originalErr}

	if err.Unwrap() != originalErr {
		t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), originalErr)
	}
}

// Note: TestParseCommaSeparated is defined in grpc_agent_client_test.go

// Helper to check error type without importing errors package
func isAgentNotFoundError(err error, target **AgentNotFoundError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*AgentNotFoundError); ok {
		*target = e
		return true
	}
	return false
}
