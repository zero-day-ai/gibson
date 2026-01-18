package mission

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSourceType(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected SourceType
	}{
		{
			name:     "URL with https",
			source:   "https://github.com/user/repo",
			expected: SourceTypeURL,
		},
		{
			name:     "URL with git@",
			source:   "git@github.com:user/repo.git",
			expected: SourceTypeURL,
		},
		{
			name:     "Simple mission name",
			source:   "my-mission",
			expected: SourceTypeName,
		},
		{
			name:     "Relative file path",
			source:   "./mission.yaml",
			expected: SourceTypeFile,
		},
		{
			name:     "Absolute file path",
			source:   "/tmp/mission.yaml",
			expected: SourceTypeName, // Will be SourceTypeName if file doesn't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectSourceType(tt.source)
			if result != tt.expected {
				t.Errorf("detectSourceType(%q) = %v, want %v", tt.source, result, tt.expected)
			}
		})
	}
}

func TestDefaultMissionLoader_LoadByName(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a mission directory with mission.yaml
	missionDir := filepath.Join(tempDir, "test-mission")
	err := os.MkdirAll(missionDir, 0755)
	if err != nil {
		t.Fatalf("failed to create mission directory: %v", err)
	}

	// Write a simple mission.yaml
	missionYAML := `name: test-mission
version: 1.0.0
description: Test mission
nodes:
  - id: node1
    type: agent
    agent: test-agent
`
	err = os.WriteFile(filepath.Join(missionDir, "mission.yaml"), []byte(missionYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write mission.yaml: %v", err)
	}

	// Create loader with custom missions directory
	loader := &DefaultMissionLoader{
		missionsDir: tempDir,
	}

	// Test loading by name
	ctx := context.Background()
	def, err := loader.LoadByName(ctx, "test-mission")
	if err != nil {
		t.Fatalf("LoadByName failed: %v", err)
	}

	if def.Name != "test-mission" {
		t.Errorf("expected name 'test-mission', got %q", def.Name)
	}

	if def.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", def.Version)
	}
}

func TestDefaultMissionLoader_LoadByName_NotFound(t *testing.T) {
	tempDir := t.TempDir()

	loader := &DefaultMissionLoader{
		missionsDir: tempDir,
	}

	ctx := context.Background()
	_, err := loader.LoadByName(ctx, "nonexistent-mission")
	if err == nil {
		t.Fatal("expected error for nonexistent mission, got nil")
	}
}

func TestDefaultMissionLoader_LoadFromFile(t *testing.T) {
	// Create a temporary file with mission definition
	tempDir := t.TempDir()
	missionFile := filepath.Join(tempDir, "mission.yaml")

	missionYAML := `name: file-mission
version: 2.0.0
description: Mission loaded from file
nodes:
  - id: node1
    type: tool
    tool: test-tool
`
	err := os.WriteFile(missionFile, []byte(missionYAML), 0644)
	if err != nil {
		t.Fatalf("failed to write mission file: %v", err)
	}

	loader := &DefaultMissionLoader{
		missionsDir: tempDir,
	}

	ctx := context.Background()
	def, err := loader.LoadFromFile(ctx, missionFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if def.Name != "file-mission" {
		t.Errorf("expected name 'file-mission', got %q", def.Name)
	}

	if def.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", def.Version)
	}
}

func TestDefaultMissionLoader_LoadFromFile_NotFound(t *testing.T) {
	loader := &DefaultMissionLoader{
		missionsDir: "/tmp",
	}

	ctx := context.Background()
	_, err := loader.LoadFromFile(ctx, "/nonexistent/mission.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestDefaultMissionLoader_LoadFromURL_NoGitCloner(t *testing.T) {
	loader := &DefaultMissionLoader{
		missionsDir: "/tmp",
		gitCloner:   nil,
	}

	ctx := context.Background()
	_, err := loader.LoadFromURL(ctx, "https://github.com/user/repo")
	if err == nil {
		t.Fatal("expected error when gitCloner is nil, got nil")
	}
}

func TestNewMissionLoader(t *testing.T) {
	loader, err := NewMissionLoader()
	if err != nil {
		t.Fatalf("NewMissionLoader failed: %v", err)
	}

	if loader.missionsDir == "" {
		t.Error("expected missionsDir to be set, got empty string")
	}

	// Check that missions dir path contains ".gibson/missions"
	if !contains(loader.missionsDir, ".gibson") || !contains(loader.missionsDir, "missions") {
		t.Errorf("expected missionsDir to contain '.gibson/missions', got %q", loader.missionsDir)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
