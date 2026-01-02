package database

import (
	"context"
	"testing"

	"github.com/zero-day-ai/gibson/internal/component"
)

// TestComponentDAO_Create tests creating a new component
func TestComponentDAO_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create manifest
	manifest := &component.Manifest{
		Kind:        "agent",
		Name:        "test-agent",
		Version:     "1.0.0",
		Description: "Test agent for unit tests",
	}

	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "test-agent",
		Version:  "1.0.0",
		RepoPath: "/home/user/.gibson/agents/_repos/test-agent",
		BinPath:  "/home/user/.gibson/agents/bin/test-agent",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		Manifest: manifest,
		PID:      0,
		Port:     0,
	}

	err := dao.Create(ctx, comp)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was set
	if comp.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	// Retrieve component
	retrieved, err := dao.GetByID(ctx, comp.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Verify fields
	if retrieved.Kind != component.ComponentKindAgent {
		t.Errorf("expected Kind agent, got %s", retrieved.Kind)
	}
	if retrieved.Name != "test-agent" {
		t.Errorf("expected Name 'test-agent', got %s", retrieved.Name)
	}
	if retrieved.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %s", retrieved.Version)
	}
	if retrieved.RepoPath != comp.RepoPath {
		t.Errorf("expected RepoPath %s, got %s", comp.RepoPath, retrieved.RepoPath)
	}
	if retrieved.BinPath != comp.BinPath {
		t.Errorf("expected BinPath %s, got %s", comp.BinPath, retrieved.BinPath)
	}
	if retrieved.Source != component.ComponentSourceExternal {
		t.Errorf("expected Source external, got %s", retrieved.Source)
	}
	// Note: Status, PID, Port are no longer stored in database (migration 10)
	// They are tracked via process checking instead
	if retrieved.Manifest == nil {
		t.Fatal("expected Manifest to be set")
	}
	if retrieved.Manifest.Name != "test-agent" {
		t.Errorf("expected Manifest.Name 'test-agent', got %s", retrieved.Manifest.Name)
	}
}

// TestComponentDAO_Create_Duplicate tests that duplicate kind+name is rejected
func TestComponentDAO_Create_Duplicate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create first component
	comp1 := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "nmap",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo1",
		BinPath:  "/path/to/bin1",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := dao.Create(ctx, comp1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to create duplicate with same kind and name
	comp2 := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "nmap",
		Version:  "2.0.0", // Different version
		RepoPath: "/path/to/repo2",
		BinPath:  "/path/to/bin2",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err = dao.Create(ctx, comp2)
	if err == nil {
		t.Fatal("expected error creating duplicate component")
	}

	// Verify the error is a constraint violation
	// SQLite returns "UNIQUE constraint failed" error
	if !isConstraintError(err) {
		t.Errorf("expected constraint error, got: %v", err)
	}
}

// TestComponentDAO_GetByID tests retrieving a component by ID
func TestComponentDAO_GetByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindPlugin,
		Name:     "test-plugin",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve by ID
	retrieved, err := dao.GetByID(ctx, comp.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ID != comp.ID {
		t.Errorf("expected ID %d, got %d", comp.ID, retrieved.ID)
	}
	if retrieved.Name != "test-plugin" {
		t.Errorf("expected Name 'test-plugin', got %s", retrieved.Name)
	}
}

// TestComponentDAO_GetByID_NotFound tests that nil is returned for non-existent ID
func TestComponentDAO_GetByID_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Try to get non-existent component
	retrieved, err := dao.GetByID(ctx, 99999)
	if err != nil {
		t.Fatalf("GetByID should not return error for non-existent ID: %v", err)
	}

	if retrieved != nil {
		t.Error("expected nil component for non-existent ID")
	}
}

// TestComponentDAO_GetByName tests retrieving a component by kind and name
func TestComponentDAO_GetByName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "davinci",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve by kind and name
	retrieved, err := dao.GetByName(ctx, component.ComponentKindAgent, "davinci")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected component to be found")
	}
	if retrieved.Name != "davinci" {
		t.Errorf("expected Name 'davinci', got %s", retrieved.Name)
	}
	if retrieved.Kind != component.ComponentKindAgent {
		t.Errorf("expected Kind agent, got %s", retrieved.Kind)
	}
}

// TestComponentDAO_GetByName_NotFound tests that nil is returned when component doesn't exist
func TestComponentDAO_GetByName_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Try to get non-existent component
	retrieved, err := dao.GetByName(ctx, component.ComponentKindTool, "nonexistent")
	if err != nil {
		t.Fatalf("GetByName should not return error for non-existent component: %v", err)
	}

	if retrieved != nil {
		t.Error("expected nil component for non-existent name")
	}
}

// TestComponentDAO_List tests listing components by kind with ordering
func TestComponentDAO_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create multiple components of different kinds
	components := []*component.Component{
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-b",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-b",
			BinPath:  "/path/to/bin/agent-b",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-a",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-a",
			BinPath:  "/path/to/bin/agent-a",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindTool,
			Name:     "tool-x",
			Version:  "1.0.0",
			RepoPath: "/path/to/tool-x",
			BinPath:  "/path/to/bin/tool-x",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-c",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-c",
			BinPath:  "/path/to/bin/agent-c",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusRunning,
		},
	}

	for _, comp := range components {
		if err := dao.Create(ctx, comp); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List agents only
	agents, err := dao.List(ctx, component.ComponentKindAgent)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// Verify count
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	// Verify ordering (should be alphabetical by name)
	expectedOrder := []string{"agent-a", "agent-b", "agent-c"}
	for i, agent := range agents {
		if agent.Name != expectedOrder[i] {
			t.Errorf("expected agent[%d] name to be %s, got %s", i, expectedOrder[i], agent.Name)
		}
	}

	// List tools
	tools, err := dao.List(ctx, component.ComponentKindTool)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "tool-x" {
		t.Errorf("expected tool name 'tool-x', got %s", tools[0].Name)
	}

	// List plugins (empty)
	plugins, err := dao.List(ctx, component.ComponentKindPlugin)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

// TestComponentDAO_ListAll tests listing all components grouped by kind
func TestComponentDAO_ListAll(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create components of different kinds
	components := []*component.Component{
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-1",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-1",
			BinPath:  "/path/to/bin/agent-1",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindAgent,
			Name:     "agent-2",
			Version:  "1.0.0",
			RepoPath: "/path/to/agent-2",
			BinPath:  "/path/to/bin/agent-2",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindTool,
			Name:     "tool-1",
			Version:  "1.0.0",
			RepoPath: "/path/to/tool-1",
			BinPath:  "/path/to/bin/tool-1",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindPlugin,
			Name:     "plugin-1",
			Version:  "1.0.0",
			RepoPath: "/path/to/plugin-1",
			BinPath:  "/path/to/bin/plugin-1",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
	}

	for _, comp := range components {
		if err := dao.Create(ctx, comp); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List all components
	allComponents, err := dao.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}

	// Verify grouping by kind
	if len(allComponents[component.ComponentKindAgent]) != 2 {
		t.Errorf("expected 2 agents, got %d", len(allComponents[component.ComponentKindAgent]))
	}
	if len(allComponents[component.ComponentKindTool]) != 1 {
		t.Errorf("expected 1 tool, got %d", len(allComponents[component.ComponentKindTool]))
	}
	if len(allComponents[component.ComponentKindPlugin]) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(allComponents[component.ComponentKindPlugin]))
	}

	// Verify ordering within each kind
	agents := allComponents[component.ComponentKindAgent]
	if agents[0].Name != "agent-1" || agents[1].Name != "agent-2" {
		t.Error("agents not ordered correctly by name")
	}
}

// TestComponentDAO_ListByStatus is removed - status is no longer stored in database
// Runtime state is tracked via process checking starting from migration 10
// This test is disabled as ListByStatus method has been removed from ComponentDAO interface

// TestComponentDAO_Update tests updating a component's metadata
func TestComponentDAO_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create initial component
	comp := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "test-tool",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update component
	comp.Version = "2.0.0"
	comp.RepoPath = "/new/path/to/repo"
	comp.BinPath = "/new/path/to/bin"
	comp.Status = component.ComponentStatusStopped

	err := dao.Update(ctx, comp)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify updates
	updated, err := dao.GetByID(ctx, comp.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if updated.Version != "2.0.0" {
		t.Errorf("expected Version '2.0.0', got %s", updated.Version)
	}
	if updated.RepoPath != "/new/path/to/repo" {
		t.Errorf("expected RepoPath '/new/path/to/repo', got %s", updated.RepoPath)
	}
	if updated.BinPath != "/new/path/to/bin" {
		t.Errorf("expected BinPath '/new/path/to/bin', got %s", updated.BinPath)
	}
	// Note: Status is no longer stored in database (migration 10)
}

// TestComponentDAO_Update_NotFound tests updating non-existent component
func TestComponentDAO_Update_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Try to update non-existent component
	comp := &component.Component{
		ID:       99999,
		Kind:     component.ComponentKindAgent,
		Name:     "nonexistent",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	err := dao.Update(ctx, comp)
	if err == nil {
		t.Fatal("expected error updating non-existent component")
	}
}

// TestComponentDAO_UpdateStatus tests updating component status with pid and port
func TestComponentDAO_UpdateStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "test-agent",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// UpdateStatus should persist runtime state to database
	err := dao.UpdateStatus(ctx, comp.ID, component.ComponentStatusRunning, 1234, 50051)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify that status was persisted to database
	updated, err := dao.GetByID(ctx, comp.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Runtime state should be in database
	if updated.Status != component.ComponentStatusRunning {
		t.Errorf("expected Status running, got %s", updated.Status)
	}
	if updated.PID != 1234 {
		t.Errorf("expected PID 1234, got %d", updated.PID)
	}
	if updated.Port != 50051 {
		t.Errorf("expected Port 50051, got %d", updated.Port)
	}
	if updated.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
}

// TestComponentDAO_UpdateStatus_NotFound tests updating status of non-existent component
func TestComponentDAO_UpdateStatus_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// UpdateStatus on non-existent ID - SQL UPDATE on non-existent row is not an error
	err := dao.UpdateStatus(ctx, 99999, component.ComponentStatusRunning, 1234, 50051)
	if err != nil {
		t.Fatalf("UpdateStatus should not fail for non-existent ID: %v", err)
	}
}

// TestComponentDAO_Delete tests deleting a component
func TestComponentDAO_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create component
	comp := &component.Component{
		Kind:     component.ComponentKindPlugin,
		Name:     "test-plugin",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete component
	err := dao.Delete(ctx, component.ComponentKindPlugin, "test-plugin")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify component is deleted
	deleted, err := dao.GetByName(ctx, component.ComponentKindPlugin, "test-plugin")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if deleted != nil {
		t.Error("expected component to be deleted")
	}
}

// TestComponentDAO_Delete_NotFound tests deleting non-existent component
func TestComponentDAO_Delete_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Try to delete non-existent component
	err := dao.Delete(ctx, component.ComponentKindAgent, "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent component")
	}
}

// TestComponentDAO_ManifestSerialization tests that manifest is properly serialized/deserialized
func TestComponentDAO_ManifestSerialization(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create complex manifest
	manifest := &component.Manifest{
		Kind:        "agent",
		Name:        "complex-agent",
		Version:     "1.2.3",
		Description: "Agent with complex manifest",
		Build: &component.BuildConfig{
			Command:   "go build -o agent .",
			Artifacts: []string{"agent"},
		},
		Runtime: &component.RuntimeConfig{
			Type:       component.RuntimeTypeGo,
			Entrypoint: "./agent",
			Port:       0,
		},
		Dependencies: &component.ComponentDependencies{
			Gibson: ">=1.0.0",
			System: []string{"git", "make"},
		},
	}

	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "complex-agent",
		Version:  "1.2.3",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		Manifest: manifest,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve and verify manifest
	retrieved, err := dao.GetByName(ctx, component.ComponentKindAgent, "complex-agent")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if retrieved.Manifest == nil {
		t.Fatal("expected Manifest to be set")
	}

	// Verify manifest fields
	if retrieved.Manifest.Name != "complex-agent" {
		t.Errorf("expected Manifest.Name 'complex-agent', got %s", retrieved.Manifest.Name)
	}
	if retrieved.Manifest.Description != "Agent with complex manifest" {
		t.Errorf("expected Description 'Agent with complex manifest', got %s", retrieved.Manifest.Description)
	}
	if retrieved.Manifest.Build == nil {
		t.Fatal("expected Manifest.Build to be set")
	}
	if retrieved.Manifest.Build.Command != "go build -o agent ." {
		t.Errorf("expected Build.Command 'go build -o agent .', got %s", retrieved.Manifest.Build.Command)
	}
	if len(retrieved.Manifest.Build.Artifacts) != 1 || retrieved.Manifest.Build.Artifacts[0] != "agent" {
		t.Errorf("expected Build.Artifacts ['agent'], got %v", retrieved.Manifest.Build.Artifacts)
	}
	if retrieved.Manifest.Runtime == nil {
		t.Fatal("expected Manifest.Runtime to be set")
	}
	if retrieved.Manifest.Runtime.Type != component.RuntimeTypeGo {
		t.Errorf("expected Runtime.Type go, got %s", retrieved.Manifest.Runtime.Type)
	}
	if retrieved.Manifest.Dependencies == nil {
		t.Fatal("expected Manifest.Dependencies to be set")
	}
	if retrieved.Manifest.Dependencies.Gibson != ">=1.0.0" {
		t.Errorf("expected Dependencies.Gibson '>=1.0.0', got %s", retrieved.Manifest.Dependencies.Gibson)
	}
}

// TestComponentDAO_NullableFields tests handling of nullable fields (started_at, stopped_at)
func TestComponentDAO_NullableFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create component with null timestamps
	comp := &component.Component{
		Kind:     component.ComponentKindTool,
		Name:     "test-tool",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		// StartedAt and StoppedAt are nil
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve and verify null timestamps
	retrieved, err := dao.GetByID(ctx, comp.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.StartedAt != nil {
		t.Error("expected StartedAt to be nil")
	}
	if retrieved.StoppedAt != nil {
		t.Error("expected StoppedAt to be nil")
	}
}

// TestComponentDAO_EmptyManifest tests handling of component without manifest
func TestComponentDAO_EmptyManifest(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create component without manifest
	comp := &component.Component{
		Kind:     component.ComponentKindAgent,
		Name:     "no-manifest-agent",
		Version:  "1.0.0",
		RepoPath: "/path/to/repo",
		BinPath:  "/path/to/bin",
		Source:   component.ComponentSourceExternal,
		Status:   component.ComponentStatusAvailable,
		Manifest: nil,
	}

	if err := dao.Create(ctx, comp); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := dao.GetByName(ctx, component.ComponentKindAgent, "no-manifest-agent")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if retrieved.Manifest != nil {
		t.Error("expected Manifest to be nil")
	}
}

// TestComponentDAO_MultipleKindsSameName tests that same name is allowed for different kinds
func TestComponentDAO_MultipleKindsSameName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Apply migrations
	migrator := NewMigrator(db)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	ctx := context.Background()
	dao := NewComponentDAO(db)

	// Create components with same name but different kinds
	sameName := "duplicate-name"

	components := []*component.Component{
		{
			Kind:     component.ComponentKindAgent,
			Name:     sameName,
			Version:  "1.0.0",
			RepoPath: "/path/to/agent",
			BinPath:  "/path/to/bin/agent",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindTool,
			Name:     sameName,
			Version:  "1.0.0",
			RepoPath: "/path/to/tool",
			BinPath:  "/path/to/bin/tool",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
		{
			Kind:     component.ComponentKindPlugin,
			Name:     sameName,
			Version:  "1.0.0",
			RepoPath: "/path/to/plugin",
			BinPath:  "/path/to/bin/plugin",
			Source:   component.ComponentSourceExternal,
			Status:   component.ComponentStatusAvailable,
		},
	}

	// All should succeed (unique constraint is on kind+name combination)
	for _, comp := range components {
		if err := dao.Create(ctx, comp); err != nil {
			t.Fatalf("Create failed for %s: %v", comp.Kind, err)
		}
	}

	// Verify we can retrieve each by kind+name
	agent, err := dao.GetByName(ctx, component.ComponentKindAgent, sameName)
	if err != nil || agent == nil {
		t.Errorf("failed to get agent with name %s: %v", sameName, err)
	}

	tool, err := dao.GetByName(ctx, component.ComponentKindTool, sameName)
	if err != nil || tool == nil {
		t.Errorf("failed to get tool with name %s: %v", sameName, err)
	}

	plugin, err := dao.GetByName(ctx, component.ComponentKindPlugin, sameName)
	if err != nil || plugin == nil {
		t.Errorf("failed to get plugin with name %s: %v", sameName, err)
	}
}

// isConstraintError checks if an error is a SQLite constraint violation
func isConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// Check for SQLite UNIQUE constraint error
	errMsg := err.Error()
	return contains(errMsg, "UNIQUE constraint failed") || contains(errMsg, "constraint failed")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexString(s, substr) >= 0))
}

// indexString returns the index of substr in s, or -1
func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
