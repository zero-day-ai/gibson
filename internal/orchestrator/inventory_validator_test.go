package orchestrator

import (
	"errors"
	"testing"
)

func TestNewInventoryValidator(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "agent1"},
			{Name: "agent2"},
		},
	}

	validator := NewInventoryValidator(inv)
	if validator == nil {
		t.Fatal("NewInventoryValidator returned nil")
	}
	if validator.inventory != inv {
		t.Error("validator inventory not set correctly")
	}
}

func TestValidateSpawnAgent_ValidAgent(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "k8skiller"},
		},
	}

	validator := NewInventoryValidator(inv)

	decision := &Decision{
		Action: ActionSpawnAgent,
		SpawnConfig: &SpawnNodeConfig{
			AgentName:   "davinci",
			Description: "Test agent",
			TaskConfig:  map[string]interface{}{},
			DependsOn:   []string{},
		},
	}

	err := validator.ValidateSpawnAgent(decision)
	if err != nil {
		t.Errorf("ValidateSpawnAgent failed for valid agent: %v", err)
	}
}

func TestValidateSpawnAgent_InvalidAgent(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "k8skiller"},
		},
	}

	validator := NewInventoryValidator(inv)

	decision := &Decision{
		Action: ActionSpawnAgent,
		SpawnConfig: &SpawnNodeConfig{
			AgentName:   "nonexistent",
			Description: "Test agent",
			TaskConfig:  map[string]interface{}{},
			DependsOn:   []string{},
		},
	}

	err := validator.ValidateSpawnAgent(decision)
	if err == nil {
		t.Fatal("ValidateSpawnAgent should fail for invalid agent")
	}

	var validationErr *ComponentValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Expected ComponentValidationError, got %T", err)
	}

	if validationErr.ComponentType != "agent" {
		t.Errorf("Expected component type 'agent', got %q", validationErr.ComponentType)
	}
	if validationErr.RequestedName != "nonexistent" {
		t.Errorf("Expected requested name 'nonexistent', got %q", validationErr.RequestedName)
	}
	if len(validationErr.Available) != 2 {
		t.Errorf("Expected 2 available agents, got %d", len(validationErr.Available))
	}
}

func TestValidateSpawnAgent_NilInventory(t *testing.T) {
	validator := NewInventoryValidator(nil)

	decision := &Decision{
		Action: ActionSpawnAgent,
		SpawnConfig: &SpawnNodeConfig{
			AgentName:   "nonexistent",
			Description: "Test agent",
			TaskConfig:  map[string]interface{}{},
			DependsOn:   []string{},
		},
	}

	// Should not fail when inventory is nil
	err := validator.ValidateSpawnAgent(decision)
	if err != nil {
		t.Errorf("ValidateSpawnAgent should skip validation when inventory is nil, got error: %v", err)
	}
}

func TestValidateSpawnAgent_NonSpawnAction(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
		},
	}

	validator := NewInventoryValidator(inv)

	decision := &Decision{
		Action:       ActionExecuteAgent,
		TargetNodeID: "node1",
	}

	err := validator.ValidateSpawnAgent(decision)
	if err != nil {
		t.Errorf("ValidateSpawnAgent should return nil for non-spawn actions, got: %v", err)
	}
}

func TestValidateDecision_SpawnAgent(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
		},
	}

	validator := NewInventoryValidator(inv)

	tests := []struct {
		name      string
		decision  *Decision
		expectErr bool
	}{
		{
			name: "valid spawn agent",
			decision: &Decision{
				Action: ActionSpawnAgent,
				SpawnConfig: &SpawnNodeConfig{
					AgentName:   "davinci",
					Description: "Test",
					TaskConfig:  map[string]interface{}{},
					DependsOn:   []string{},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid spawn agent",
			decision: &Decision{
				Action: ActionSpawnAgent,
				SpawnConfig: &SpawnNodeConfig{
					AgentName:   "invalid",
					Description: "Test",
					TaskConfig:  map[string]interface{}{},
					DependsOn:   []string{},
				},
			},
			expectErr: true,
		},
		{
			name: "execute agent - no validation",
			decision: &Decision{
				Action:       ActionExecuteAgent,
				TargetNodeID: "node1",
			},
			expectErr: false,
		},
		{
			name: "complete action - no validation",
			decision: &Decision{
				Action:     ActionComplete,
				StopReason: "Done",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateDecision(tt.decision)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateDecision_NilDecision(t *testing.T) {
	validator := NewInventoryValidator(&ComponentInventory{})

	err := validator.ValidateDecision(nil)
	if err == nil {
		t.Error("ValidateDecision should return error for nil decision")
	}
}

func TestSuggestAlternatives_ExactSubstring(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "davinci-pro"},
			{Name: "k8skiller"},
		},
	}

	validator := NewInventoryValidator(inv)

	suggestions := validator.SuggestAlternatives("dav", "agent")
	if len(suggestions) == 0 {
		t.Fatal("Expected suggestions for 'dav'")
	}

	// Should match davinci and davinci-pro (both contain 'dav')
	foundDavinci := false
	for _, s := range suggestions {
		if s == "davinci" || s == "davinci-pro" {
			foundDavinci = true
			break
		}
	}
	if !foundDavinci {
		t.Errorf("Expected 'davinci' or 'davinci-pro' in suggestions, got: %v", suggestions)
	}
}

func TestSuggestAlternatives_Typo(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "k8skiller"},
		},
	}

	validator := NewInventoryValidator(inv)

	// "davinci" with typo
	suggestions := validator.SuggestAlternatives("davinki", "agent")
	if len(suggestions) == 0 {
		t.Fatal("Expected suggestions for typo 'davinki'")
	}

	found := false
	for _, s := range suggestions {
		if s == "davinci" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'davinci' in suggestions for typo, got: %v", suggestions)
	}
}

func TestSuggestAlternatives_NoMatches(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "davinci"},
			{Name: "k8skiller"},
		},
	}

	validator := NewInventoryValidator(inv)

	// Very different name
	suggestions := validator.SuggestAlternatives("completely-different-name", "agent")
	// Should return empty or very few suggestions
	// (depends on Levenshtein distance threshold)
	if len(suggestions) > 3 {
		t.Errorf("Expected at most 3 suggestions, got: %d", len(suggestions))
	}
}

func TestSuggestAlternatives_CaseInsensitive(t *testing.T) {
	inv := &ComponentInventory{
		Agents: []AgentSummary{
			{Name: "DaVinci"},
			{Name: "K8sKiller"},
		},
	}

	validator := NewInventoryValidator(inv)

	suggestions := validator.SuggestAlternatives("davinci", "agent")
	if len(suggestions) == 0 {
		t.Fatal("Expected suggestions for case-insensitive match")
	}

	found := false
	for _, s := range suggestions {
		if s == "DaVinci" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'DaVinci' in suggestions, got: %v", suggestions)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"sitting", "kitten", 3},
		{"saturday", "sunday", 3},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := levenshteinDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("levenshteinDistance(%q, %q) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
