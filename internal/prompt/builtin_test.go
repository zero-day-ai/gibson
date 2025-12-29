package prompt

import (
	"strings"
	"testing"
)

func TestBuiltinPromptIDs(t *testing.T) {
	ids := BuiltinPromptIDs()

	// Should return all built-in prompts
	if len(ids) != 4 {
		t.Errorf("expected 4 built-in prompts, got %d", len(ids))
	}

	// All IDs should have the builtin: prefix
	for _, id := range ids {
		if !strings.HasPrefix(id, BuiltinPromptPrefix) {
			t.Errorf("built-in prompt ID %q does not have %q prefix", id, BuiltinPromptPrefix)
		}
	}

	// Should contain all expected IDs
	expectedIDs := map[string]bool{
		BuiltinSafetyID:              false,
		BuiltinOutputFormatID:        false,
		BuiltinPersonaProfessionalID: false,
		BuiltinPersonaTechnicalID:    false,
	}

	for _, id := range ids {
		if _, exists := expectedIDs[id]; exists {
			expectedIDs[id] = true
		} else {
			t.Errorf("unexpected built-in prompt ID: %s", id)
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("expected built-in prompt ID %s not found", id)
		}
	}
}

func TestGetBuiltinPrompts(t *testing.T) {
	prompts := GetBuiltinPrompts()

	// Should return all built-in prompts
	if len(prompts) != 4 {
		t.Errorf("expected 4 built-in prompts, got %d", len(prompts))
	}

	// All prompts should be valid
	for _, p := range prompts {
		if err := p.Validate(); err != nil {
			t.Errorf("built-in prompt %s failed validation: %v", p.ID, err)
		}

		// All IDs should have the builtin: prefix
		if !strings.HasPrefix(p.ID, BuiltinPromptPrefix) {
			t.Errorf("built-in prompt ID %q does not have %q prefix", p.ID, BuiltinPromptPrefix)
		}

		// Content should not be empty
		if p.Content == "" {
			t.Errorf("built-in prompt %s has empty content", p.ID)
		}

		// Position should be valid
		if !p.Position.IsValid() {
			t.Errorf("built-in prompt %s has invalid position: %s", p.ID, p.Position)
		}
	}
}

func TestGetBuiltinPrompt(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		expectErr bool
	}{
		{
			name:      "get safety prompt",
			id:        BuiltinSafetyID,
			expectErr: false,
		},
		{
			name:      "get output format prompt",
			id:        BuiltinOutputFormatID,
			expectErr: false,
		},
		{
			name:      "get professional persona",
			id:        BuiltinPersonaProfessionalID,
			expectErr: false,
		},
		{
			name:      "get technical persona",
			id:        BuiltinPersonaTechnicalID,
			expectErr: false,
		},
		{
			name:      "get non-existent prompt",
			id:        "builtin:nonexistent",
			expectErr: true,
		},
		{
			name:      "get non-builtin ID",
			id:        "custom:prompt",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := GetBuiltinPrompt(tt.id)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if p == nil {
					t.Error("expected prompt, got nil")
				}
				if p.ID != tt.id {
					t.Errorf("expected ID %s, got %s", tt.id, p.ID)
				}

				// Verify returned prompt is valid
				if err := p.Validate(); err != nil {
					t.Errorf("returned prompt failed validation: %v", err)
				}
			}
		})
	}
}

func TestGetBuiltinPrompt_ReturnsCopy(t *testing.T) {
	// Get the same prompt twice
	p1, err := GetBuiltinPrompt(BuiltinSafetyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p2, err := GetBuiltinPrompt(BuiltinSafetyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify one copy
	p1.Content = "modified content"

	// The other copy should be unchanged
	if p2.Content == "modified content" {
		t.Error("modifying returned prompt affected other copies - not returning independent copies")
	}
}

func TestSafetyPrompt_Configuration(t *testing.T) {
	p, err := GetBuiltinPrompt(BuiltinSafetyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Safety prompt should be at system_prefix
	if p.Position != PositionSystemPrefix {
		t.Errorf("expected position %s, got %s", PositionSystemPrefix, p.Position)
	}

	// Safety prompt should have priority 0 (highest)
	if p.Priority != 0 {
		t.Errorf("expected priority 0, got %d", p.Priority)
	}

	// Should have proper ID
	if p.ID != BuiltinSafetyID {
		t.Errorf("expected ID %s, got %s", BuiltinSafetyID, p.ID)
	}

	// Should have non-empty content
	if p.Content == "" {
		t.Error("safety prompt has empty content")
	}

	// Content should include key safety concepts
	requiredTerms := []string{"SAFETY", "CONSTRAINTS", "authorization", "ethical"}
	for _, term := range requiredTerms {
		if !strings.Contains(p.Content, term) {
			t.Errorf("safety prompt content missing expected term: %s", term)
		}
	}
}

func TestOutputFormatPrompt_Configuration(t *testing.T) {
	p, err := GetBuiltinPrompt(BuiltinOutputFormatID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Output format prompt should be at system_suffix
	if p.Position != PositionSystemSuffix {
		t.Errorf("expected position %s, got %s", PositionSystemSuffix, p.Position)
	}

	// Should have priority 100
	if p.Priority != 100 {
		t.Errorf("expected priority 100, got %d", p.Priority)
	}

	// Content should mention formatting
	if !strings.Contains(p.Content, "FORMAT") {
		t.Error("output format prompt should mention FORMAT")
	}
}

func TestPersonaPrompts_Configuration(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		position Position
		priority int
	}{
		{
			name:     "professional persona",
			id:       BuiltinPersonaProfessionalID,
			position: PositionSystem,
			priority: 50,
		},
		{
			name:     "technical persona",
			id:       BuiltinPersonaTechnicalID,
			position: PositionSystem,
			priority: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := GetBuiltinPrompt(tt.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if p.Position != tt.position {
				t.Errorf("expected position %s, got %s", tt.position, p.Position)
			}

			if p.Priority != tt.priority {
				t.Errorf("expected priority %d, got %d", tt.priority, p.Priority)
			}

			// Content should mention communication style
			if !strings.Contains(p.Content, "COMMUNICATION STYLE") {
				t.Error("persona prompt should mention COMMUNICATION STYLE")
			}
		})
	}
}

func TestRegisterBuiltins(t *testing.T) {
	registry := NewPromptRegistry()

	err := RegisterBuiltins(registry)
	if err != nil {
		t.Fatalf("unexpected error registering built-ins: %v", err)
	}

	// Should have registered all built-in prompts
	prompts := registry.List()
	if len(prompts) != 4 {
		t.Errorf("expected 4 prompts in registry, got %d", len(prompts))
	}

	// Each built-in should be retrievable
	builtinIDs := []string{
		BuiltinSafetyID,
		BuiltinOutputFormatID,
		BuiltinPersonaProfessionalID,
		BuiltinPersonaTechnicalID,
	}

	for _, id := range builtinIDs {
		p, err := registry.Get(id)
		if err != nil {
			t.Errorf("failed to get registered built-in %s: %v", id, err)
		}
		if p == nil {
			t.Errorf("got nil prompt for built-in %s", id)
		}
	}
}

func TestRegisterBuiltins_AlreadyExists(t *testing.T) {
	registry := NewPromptRegistry()

	// Register built-ins first time
	err := RegisterBuiltins(registry)
	if err != nil {
		t.Fatalf("unexpected error on first registration: %v", err)
	}

	// Try to register again - should fail
	err = RegisterBuiltins(registry)
	if err == nil {
		t.Error("expected error when registering built-ins twice, got nil")
	}
}

func TestRegisterBuiltins_PositionOrder(t *testing.T) {
	registry := NewPromptRegistry()

	err := RegisterBuiltins(registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get prompts at system_prefix - should have safety prompt first
	systemPrefixPrompts := registry.GetByPosition(PositionSystemPrefix)
	if len(systemPrefixPrompts) != 1 {
		t.Errorf("expected 1 prompt at system_prefix, got %d", len(systemPrefixPrompts))
	}
	if len(systemPrefixPrompts) > 0 && systemPrefixPrompts[0].ID != BuiltinSafetyID {
		t.Errorf("expected safety prompt at system_prefix, got %s", systemPrefixPrompts[0].ID)
	}

	// Get prompts at system - should have persona prompts
	systemPrompts := registry.GetByPosition(PositionSystem)
	if len(systemPrompts) != 2 {
		t.Errorf("expected 2 prompts at system position, got %d", len(systemPrompts))
	}

	// Both should have same priority (50)
	for _, p := range systemPrompts {
		if p.Priority != 50 {
			t.Errorf("expected persona prompt priority 50, got %d", p.Priority)
		}
	}

	// Get prompts at system_suffix - should have output format
	systemSuffixPrompts := registry.GetByPosition(PositionSystemSuffix)
	if len(systemSuffixPrompts) != 1 {
		t.Errorf("expected 1 prompt at system_suffix, got %d", len(systemSuffixPrompts))
	}
	if len(systemSuffixPrompts) > 0 && systemSuffixPrompts[0].ID != BuiltinOutputFormatID {
		t.Errorf("expected output format prompt at system_suffix, got %s", systemSuffixPrompts[0].ID)
	}
}

func TestIsBuiltinPrompt(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		expected bool
	}{
		{
			name:     "safety prompt",
			id:       BuiltinSafetyID,
			expected: true,
		},
		{
			name:     "output format prompt",
			id:       BuiltinOutputFormatID,
			expected: true,
		},
		{
			name:     "professional persona",
			id:       BuiltinPersonaProfessionalID,
			expected: true,
		},
		{
			name:     "technical persona",
			id:       BuiltinPersonaTechnicalID,
			expected: true,
		},
		{
			name:     "non-existent builtin",
			id:       "builtin:nonexistent",
			expected: false,
		},
		{
			name:     "custom prompt",
			id:       "custom:prompt",
			expected: false,
		},
		{
			name:     "empty string",
			id:       "",
			expected: false,
		},
		{
			name:     "just prefix",
			id:       "builtin:",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsBuiltinPrompt(tt.id)
			if result != tt.expected {
				t.Errorf("IsBuiltinPrompt(%q) = %v, expected %v", tt.id, result, tt.expected)
			}
		})
	}
}

func TestAllBuiltinPrompts_HaveCorrectPrefix(t *testing.T) {
	prompts := GetBuiltinPrompts()

	for _, p := range prompts {
		if !strings.HasPrefix(p.ID, BuiltinPromptPrefix) {
			t.Errorf("built-in prompt %s does not have correct prefix %s", p.ID, BuiltinPromptPrefix)
		}
	}
}

func TestAllBuiltinPrompts_AreValid(t *testing.T) {
	prompts := GetBuiltinPrompts()

	for _, p := range prompts {
		if err := p.Validate(); err != nil {
			t.Errorf("built-in prompt %s failed validation: %v", p.ID, err)
		}
	}
}

func TestBuiltinPrompts_HaveRequiredFields(t *testing.T) {
	prompts := GetBuiltinPrompts()

	for _, p := range prompts {
		// Check ID
		if p.ID == "" {
			t.Error("built-in prompt has empty ID")
		}

		// Check Name
		if p.Name == "" {
			t.Errorf("built-in prompt %s has empty Name", p.ID)
		}

		// Check Description
		if p.Description == "" {
			t.Errorf("built-in prompt %s has empty Description", p.ID)
		}

		// Check Content
		if p.Content == "" {
			t.Errorf("built-in prompt %s has empty Content", p.ID)
		}

		// Check Position is valid
		if !p.Position.IsValid() {
			t.Errorf("built-in prompt %s has invalid position: %s", p.ID, p.Position)
		}
	}
}

func TestBuiltinPrompts_UniqueIDs(t *testing.T) {
	prompts := GetBuiltinPrompts()
	seen := make(map[string]bool)

	for _, p := range prompts {
		if seen[p.ID] {
			t.Errorf("duplicate built-in prompt ID: %s", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestBuiltinPrompts_PriorityValues(t *testing.T) {
	// Test specific priority requirements
	safety, err := GetBuiltinPrompt(BuiltinSafetyID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if safety.Priority != 0 {
		t.Errorf("safety prompt should have priority 0 (highest), got %d", safety.Priority)
	}

	// All prompts should have non-negative priority
	prompts := GetBuiltinPrompts()
	for _, p := range prompts {
		if p.Priority < 0 {
			t.Errorf("built-in prompt %s has negative priority: %d", p.ID, p.Priority)
		}
	}
}
