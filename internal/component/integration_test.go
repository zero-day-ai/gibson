package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
)

// simpleGitOps is a simple git operations implementation for integration testing
type simpleGitOps struct {
	t *testing.T
}

func (s *simpleGitOps) Clone(url, dest string, opts git.CloneOptions) error {
	// Create directory structure
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	// Create manifest
	manifestPath := filepath.Join(dest, ManifestFileName)
	data := `kind: agent
name: scanner
version: 1.0.0
description: Test scanner agent
runtime:
  type: go
  entrypoint: ./bin/scanner
  port: 50000
  health_url: /health
`
	if err := os.WriteFile(manifestPath, []byte(data), 0644); err != nil {
		return err
	}
	// Create bin directory for build output
	binDir := filepath.Join(dest, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	// Create dummy binary
	binaryPath := filepath.Join(binDir, "scanner")
	return os.WriteFile(binaryPath, []byte("#!/bin/bash\necho test"), 0755)
}

func (s *simpleGitOps) Pull(dir string) error {
	return nil
}

func (s *simpleGitOps) GetVersion(dir string) (string, error) {
	return "abc123def456", nil
}

func (s *simpleGitOps) ParseRepoURL(url string) (*git.RepoInfo, error) {
	return &git.RepoInfo{
		Host:  "github.com",
		Owner: "test-org",
		Repo:  "gibson-agent-scanner",
		Kind:  "agent",
		Name:  "scanner",
	}, nil
}

// simpleBuildOps is a simple build executor for integration testing
type simpleBuildOps struct{}

func (s *simpleBuildOps) Build(ctx context.Context, config build.BuildConfig, name, version, gibsonVersion string) (*build.BuildResult, error) {
	return &build.BuildResult{
		Success:    true,
		OutputPath: filepath.Join(config.WorkDir, "bin", name),
		Duration:   time.Second * 5,
		Stdout:     "Build successful",
		Stderr:     "",
	}, nil
}

func (s *simpleBuildOps) Clean(ctx context.Context, workDir string) (*build.CleanResult, error) {
	return &build.CleanResult{
		Success:  true,
		Duration: time.Second,
		Output:   "Clean successful",
	}, nil
}

func (s *simpleBuildOps) Test(ctx context.Context, workDir string) (*build.TestResult, error) {
	return &build.TestResult{
		Success:  true,
		Passed:   10,
		Failed:   0,
		Duration: time.Second * 3,
		Output:   "All tests passed",
	}, nil
}

// TestFullInstallFlow tests the complete installation workflow
func TestFullInstallFlow(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "gibson-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create simple implementations for integration testing
	simpleGit := &simpleGitOps{t: t}
	simpleBuild := &simpleBuildOps{}
	registry := NewDefaultComponentRegistry()

	// Create installer with custom home directory
	installer := NewDefaultInstaller(simpleGit, simpleBuild, registry)
	installer.homeDir = tmpDir

	ctx := context.Background()
	repoURL := "https://github.com/test-org/gibson-agent-scanner"

	// Test installation
	opts := InstallOptions{
		Force:        false,
		SkipBuild:    false,
		SkipRegister: false,
		Timeout:      30 * time.Second,
	}

	result, err := installer.Install(ctx, repoURL, opts)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Installed)
	assert.NotNil(t, result.Component)
	assert.Equal(t, "scanner", result.Component.Name)
	assert.Equal(t, ComponentKindAgent, result.Component.Kind)
	assert.Equal(t, ComponentStatusAvailable, result.Component.Status)

	// Verify component is registered
	comp := registry.Get(ComponentKindAgent, "scanner")
	require.NotNil(t, comp)
	assert.Equal(t, "scanner", comp.Name)
	assert.Equal(t, ComponentKindAgent, comp.Kind)

	// Verify directory structure
	componentDir := filepath.Join(tmpDir, "agents", "scanner")
	assert.DirExists(t, componentDir)
	assert.FileExists(t, filepath.Join(componentDir, ManifestFileName))

	// Test that re-installing without Force flag fails
	_, err = installer.Install(ctx, repoURL, opts)
	assert.Error(t, err)
	var compErr *ComponentError
	assert.ErrorAs(t, err, &compErr)
	assert.Equal(t, ErrCodeComponentExists, compErr.Code)

	// Test re-installing with Force flag
	// Note: Must unregister from registry first since Force only handles filesystem
	err = registry.Unregister(ComponentKindAgent, "scanner")
	require.NoError(t, err)

	opts.Force = true
	result, err = installer.Install(ctx, repoURL, opts)
	require.NoError(t, err)
	assert.True(t, result.Installed)
}

// TestLifecycleFlow tests the component lifecycle: create component → start → health check → stop
func TestLifecycleFlow(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "gibson-lifecycle-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test component directory structure
	componentDir := filepath.Join(tmpDir, "agents", "test-agent")
	err = os.MkdirAll(componentDir, 0755)
	require.NoError(t, err)

	// Create manifest
	manifestPath := filepath.Join(componentDir, ManifestFileName)
	manifestData := `kind: agent
name: test-agent
version: 1.0.0
runtime:
  type: go
  entrypoint: ./bin/test-agent
  port: 50000
`
	err = os.WriteFile(manifestPath, []byte(manifestData), 0644)
	require.NoError(t, err)

	// Load manifest
	manifest, err := LoadManifest(manifestPath)
	require.NoError(t, err)

	// Create component instance
	comp := &Component{
		Kind:      ComponentKindAgent,
		Name:      "test-agent",
		Version:   "1.0.0",
		Path:      componentDir,
		Source:    ComponentSourceExternal,
		Status:    ComponentStatusAvailable,
		Manifest:  manifest,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Test component validation
	err = comp.Validate()
	require.NoError(t, err)

	// Test status transitions
	assert.True(t, comp.IsAvailable())
	assert.False(t, comp.IsRunning())
	assert.False(t, comp.IsStopped())

	// Simulate starting component
	comp.UpdateStatus(ComponentStatusRunning)
	comp.PID = 12345
	comp.Port = 50000

	assert.True(t, comp.IsRunning())
	assert.False(t, comp.IsAvailable())
	assert.NotNil(t, comp.StartedAt)

	// Simulate stopping component
	comp.UpdateStatus(ComponentStatusStopped)

	assert.True(t, comp.IsStopped())
	assert.False(t, comp.IsRunning())
	assert.NotNil(t, comp.StoppedAt)

	// Test error status
	comp.UpdateStatus(ComponentStatusError)
	assert.True(t, comp.HasError())
}

// TestConfigLoadingAndInitialization tests loading components from configuration
func TestConfigLoadingAndInitialization(t *testing.T) {
	// Create temporary config file
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "components.yaml")
	configContent := `
agents:
  - kind: agent
    name: scanner
    version: 1.0.0
    path: /tmp/scanner
    source: external
    status: available
    created_at: 2024-01-01T00:00:00Z
    updated_at: 2024-01-01T00:00:00Z
tools:
  - kind: tool
    name: nmap
    version: 2.0.0
    path: /tmp/nmap
    source: external
    status: available
    created_at: 2024-01-01T00:00:00Z
    updated_at: 2024-01-01T00:00:00Z
plugins:
  - kind: plugin
    name: vuln-db
    version: 1.5.0
    path: /tmp/vuln-db
    source: external
    status: available
    created_at: 2024-01-01T00:00:00Z
    updated_at: 2024-01-01T00:00:00Z
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create registry and load config
	registry := NewDefaultComponentRegistry()
	err = registry.LoadFromConfig(configPath)
	require.NoError(t, err)

	// Verify components were loaded
	assert.Equal(t, 3, registry.Count())
	assert.Equal(t, 1, registry.CountByKind(ComponentKindAgent))
	assert.Equal(t, 1, registry.CountByKind(ComponentKindTool))
	assert.Equal(t, 1, registry.CountByKind(ComponentKindPlugin))

	// Verify individual components
	scanner := registry.Get(ComponentKindAgent, "scanner")
	require.NotNil(t, scanner)
	assert.Equal(t, "scanner", scanner.Name)
	assert.Equal(t, "1.0.0", scanner.Version)

	nmap := registry.Get(ComponentKindTool, "nmap")
	require.NotNil(t, nmap)
	assert.Equal(t, "nmap", nmap.Name)
	assert.Equal(t, "2.0.0", nmap.Version)

	vulnDB := registry.Get(ComponentKindPlugin, "vuln-db")
	require.NotNil(t, vulnDB)
	assert.Equal(t, "vuln-db", vulnDB.Name)
	assert.Equal(t, "1.5.0", vulnDB.Version)
}

// TestRegistryPersistence tests saving and loading the registry
func TestRegistryPersistence(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "gibson-persist-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Override home directory for testing
	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	// Create registry and add components
	registry := NewDefaultComponentRegistry()

	comp1 := &Component{
		Kind:      ComponentKindAgent,
		Name:      "test-agent",
		Version:   "1.0.0",
		Path:      "/tmp/test-agent",
		Source:    ComponentSourceExternal,
		Status:    ComponentStatusAvailable,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	comp2 := &Component{
		Kind:      ComponentKindTool,
		Name:      "test-tool",
		Version:   "2.0.0",
		Path:      "/tmp/test-tool",
		Source:    ComponentSourceExternal,
		Status:    ComponentStatusAvailable,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = registry.Register(comp1)
	require.NoError(t, err)
	err = registry.Register(comp2)
	require.NoError(t, err)

	// Save registry
	err = registry.Save()
	require.NoError(t, err)

	// Verify file was created
	registryPath := filepath.Join(tmpDir, ".gibson", "registry.yaml")
	assert.FileExists(t, registryPath)

	// Create new registry and load from saved file
	newRegistry := NewDefaultComponentRegistry()
	err = newRegistry.LoadFromConfig(registryPath)
	require.NoError(t, err)

	// Verify components were loaded
	assert.Equal(t, 2, newRegistry.Count())

	loadedComp1 := newRegistry.Get(ComponentKindAgent, "test-agent")
	require.NotNil(t, loadedComp1)
	assert.Equal(t, comp1.Name, loadedComp1.Name)
	assert.Equal(t, comp1.Version, loadedComp1.Version)

	loadedComp2 := newRegistry.Get(ComponentKindTool, "test-tool")
	require.NotNil(t, loadedComp2)
	assert.Equal(t, comp2.Name, loadedComp2.Name)
	assert.Equal(t, comp2.Version, loadedComp2.Version)
}

// TestErrorRecoveryScenarios tests various error scenarios and recovery
func TestErrorRecoveryScenarios(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	t.Run("Register nil component", func(t *testing.T) {
		err := registry.Register(nil)
		assert.Error(t, err)
		var compErr *ComponentError
		assert.ErrorAs(t, err, &compErr)
		assert.Equal(t, ErrCodeValidationFailed, compErr.Code)
	})

	t.Run("Register invalid component", func(t *testing.T) {
		comp := &Component{
			Kind:    ComponentKindAgent,
			Name:    "", // Invalid: empty name
			Version: "1.0.0",
		}
		err := registry.Register(comp)
		assert.Error(t, err)
		var compErr *ComponentError
		assert.ErrorAs(t, err, &compErr)
		assert.Equal(t, ErrCodeValidationFailed, compErr.Code)
	})

	t.Run("Get non-existent component", func(t *testing.T) {
		comp := registry.Get(ComponentKindAgent, "non-existent")
		assert.Nil(t, comp)
	})

	t.Run("Unregister non-existent component", func(t *testing.T) {
		err := registry.Unregister(ComponentKindAgent, "non-existent")
		assert.Error(t, err)
		var compErr *ComponentError
		assert.ErrorAs(t, err, &compErr)
		assert.Equal(t, ErrCodeComponentNotFound, compErr.Code)
	})

	t.Run("Duplicate registration", func(t *testing.T) {
		comp := &Component{
			Kind:      ComponentKindAgent,
			Name:      "duplicate-test",
			Version:   "1.0.0",
			Path:      "/tmp/test",
			Source:    ComponentSourceExternal,
			Status:    ComponentStatusAvailable,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := registry.Register(comp)
		require.NoError(t, err)

		// Try to register again
		err = registry.Register(comp)
		assert.Error(t, err)
		var compErr *ComponentError
		assert.ErrorAs(t, err, &compErr)
		assert.Equal(t, ErrCodeComponentExists, compErr.Code)
	})

	t.Run("Update non-existent component", func(t *testing.T) {
		comp := &Component{
			Kind:      ComponentKindAgent,
			Name:      "non-existent",
			Version:   "1.0.0",
			Path:      "/tmp/test",
			Source:    ComponentSourceExternal,
			Status:    ComponentStatusAvailable,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := registry.Update(comp)
		assert.Error(t, err)
		var compErr *ComponentError
		assert.ErrorAs(t, err, &compErr)
		assert.Equal(t, ErrCodeComponentNotFound, compErr.Code)
	})
}

// TestComponentVariousStatuses tests components with different statuses
func TestComponentVariousStatuses(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	statuses := []ComponentStatus{
		ComponentStatusAvailable,
		ComponentStatusRunning,
		ComponentStatusStopped,
		ComponentStatusError,
	}

	for i, status := range statuses {
		comp := &Component{
			Kind:      ComponentKindAgent,
			Name:      fmt.Sprintf("agent-%d", i),
			Version:   "1.0.0",
			Path:      fmt.Sprintf("/tmp/agent-%d", i),
			Source:    ComponentSourceExternal,
			Status:    status,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// For running components, set required fields
		if status == ComponentStatusRunning {
			comp.PID = 12345 + i
			now := time.Now()
			comp.StartedAt = &now
		}

		err := registry.Register(comp)
		require.NoError(t, err, "Failed to register component with status %s", status)

		// Verify component was registered with correct status
		registered := registry.Get(ComponentKindAgent, comp.Name)
		require.NotNil(t, registered)
		assert.Equal(t, status, registered.Status)

		// Test status helper methods
		switch status {
		case ComponentStatusAvailable:
			assert.True(t, registered.IsAvailable())
			assert.False(t, registered.IsRunning())
		case ComponentStatusRunning:
			assert.True(t, registered.IsRunning())
			assert.False(t, registered.IsAvailable())
		case ComponentStatusStopped:
			assert.True(t, registered.IsStopped())
			assert.False(t, registered.IsRunning())
		case ComponentStatusError:
			assert.True(t, registered.HasError())
			assert.False(t, registered.IsRunning())
		}
	}

	// Verify all components were registered
	assert.Equal(t, len(statuses), registry.CountByKind(ComponentKindAgent))
}

// TestHealthMonitoringIntegration tests health monitoring integration
func TestHealthMonitoringIntegration(t *testing.T) {
	// Create health monitor
	monitor := NewHealthMonitor()

	// Register status change callback
	statusChanges := make([]struct {
		name      string
		oldStatus HealthStatus
		newStatus HealthStatus
	}, 0)

	monitor.OnStatusChange(func(name string, oldStatus, newStatus HealthStatus) {
		statusChanges = append(statusChanges, struct {
			name      string
			oldStatus HealthStatus
			newStatus HealthStatus
		}{name, oldStatus, newStatus})
	})

	// Register a component
	monitor.RegisterComponent("test-component", "http://localhost:9999/health")

	// Check initial status
	status := monitor.GetHealth("test-component")
	assert.Equal(t, HealthStatusUnknown, status)

	// Unregister component
	monitor.UnregisterComponent("test-component")

	// Verify component was unregistered
	status = monitor.GetHealth("test-component")
	assert.Equal(t, HealthStatusUnknown, status)

	// Test non-existent component
	status = monitor.GetHealth("non-existent")
	assert.Equal(t, HealthStatusUnknown, status)
}

// TestConcurrentRegistryOperations tests thread-safety of registry operations
func TestConcurrentRegistryOperations(t *testing.T) {
	registry := NewDefaultComponentRegistry()

	// Concurrently register components
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()

			comp := &Component{
				Kind:      ComponentKindAgent,
				Name:      fmt.Sprintf("agent-%d", idx),
				Version:   "1.0.0",
				Path:      fmt.Sprintf("/tmp/agent-%d", idx),
				Source:    ComponentSourceExternal,
				Status:    ComponentStatusAvailable,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			err := registry.Register(comp)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all components were registered
	assert.Equal(t, 10, registry.CountByKind(ComponentKindAgent))

	// Concurrently read components
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()

			comp := registry.Get(ComponentKindAgent, fmt.Sprintf("agent-%d", idx))
			assert.NotNil(t, comp)
		}(i)
	}

	// Wait for all read operations
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestUpdateWorkflow tests the update workflow
func TestUpdateWorkflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gibson-update-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create component directory
	componentDir := filepath.Join(tmpDir, "agents", "scanner")
	err = os.MkdirAll(componentDir, 0755)
	require.NoError(t, err)

	// Create manifest
	manifestPath := filepath.Join(componentDir, ManifestFileName)
	manifestData := `kind: agent
name: scanner
version: 1.0.0
runtime:
  type: go
  entrypoint: ./bin/scanner
`
	err = os.WriteFile(manifestPath, []byte(manifestData), 0644)
	require.NoError(t, err)

	updateGit := &updateTestGitOps{}
	simpleBuild := &simpleBuildOps{}
	registry := NewDefaultComponentRegistry()

	// Register initial component
	comp := &Component{
		Kind:      ComponentKindAgent,
		Name:      "scanner",
		Version:   "abc123def456",
		Path:      componentDir,
		Source:    ComponentSourceExternal,
		Status:    ComponentStatusAvailable,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = registry.Register(comp)
	require.NoError(t, err)

	installer := NewDefaultInstaller(updateGit, simpleBuild, registry)
	installer.homeDir = tmpDir

	ctx := context.Background()
	opts := UpdateOptions{
		Restart:   false,
		SkipBuild: true,
		Timeout:   30 * time.Second,
	}

	// Perform update
	result, err := installer.Update(ctx, ComponentKindAgent, "scanner", opts)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.Equal(t, "abc123def456", result.OldVersion)
	assert.Equal(t, "def789ghi012", result.NewVersion)

	// Verify registry was updated
	updated := registry.Get(ComponentKindAgent, "scanner")
	require.NotNil(t, updated)
	assert.Equal(t, "def789ghi012", updated.Version)
}

// updateTestGitOps is for testing update workflow
type updateTestGitOps struct{}

func (u *updateTestGitOps) Clone(url, dest string, opts git.CloneOptions) error {
	return nil
}

func (u *updateTestGitOps) Pull(dir string) error {
	return nil
}

func (u *updateTestGitOps) GetVersion(dir string) (string, error) {
	// Return different version to simulate update
	return "def789ghi012", nil
}

func (u *updateTestGitOps) ParseRepoURL(url string) (*git.RepoInfo, error) {
	return nil, nil
}
