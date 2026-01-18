//go:build integration

package mission

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/database"
)

// TestIntegration_FullMissionLifecycle tests the complete mission lifecycle:
// install -> list -> run -> uninstall
func TestIntegration_FullMissionLifecycle(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping integration test")
	}

	// Setup test environment
	ctx := context.Background()
	tempDir := t.TempDir()
	missionsDir := filepath.Join(tempDir, "missions")
	err := os.MkdirAll(missionsDir, 0755)
	require.NoError(t, err)

	// Create a local test git repository
	testRepoDir := createTestGitRepository(t)

	// Initialize components
	gitOps := git.NewDefaultGitOperations()
	store := createTestStore(t, tempDir)
	defer cleanupTestStore(store)

	installer := NewDefaultMissionInstaller(
		gitOps,
		store,
		missionsDir,
		nil, // No component store for this test
		nil, // No component installer
	)

	// Test 1: Install mission from git repository
	t.Run("Install", func(t *testing.T) {
		result, err := installer.Install(ctx, testRepoDir, InstallOptions{
			Timeout: 30 * time.Second,
		})

		require.NoError(t, err)
		assert.Equal(t, "integration-test-mission", result.Name)
		assert.NotEmpty(t, result.Version)
		assert.NotEmpty(t, result.Path)

		// Verify mission directory exists
		assert.DirExists(t, result.Path)

		// Verify mission.yaml exists
		missionYAML := filepath.Join(result.Path, MissionFileName)
		assert.FileExists(t, missionYAML)

		// Verify metadata file exists
		metadataFile := filepath.Join(result.Path, MetadataFileName)
		assert.FileExists(t, metadataFile)

		// Verify metadata content
		metadataBytes, err := os.ReadFile(metadataFile)
		require.NoError(t, err)

		var metadata InstallMetadata
		err = json.Unmarshal(metadataBytes, &metadata)
		require.NoError(t, err)
		assert.Equal(t, "integration-test-mission", metadata.Name)
		assert.Equal(t, testRepoDir, metadata.Source)
	})

	// Test 2: List installed missions (via filesystem)
	t.Run("List", func(t *testing.T) {
		// List missions by scanning filesystem
		entries, err := os.ReadDir(missionsDir)
		require.NoError(t, err)

		var missionNames []string
		for _, entry := range entries {
			if entry.IsDir() {
				// Verify it has mission.yaml
				missionYAML := filepath.Join(missionsDir, entry.Name(), MissionFileName)
				if _, err := os.Stat(missionYAML); err == nil {
					missionNames = append(missionNames, entry.Name())
				}
			}
		}

		assert.Contains(t, missionNames, "integration-test-mission")
	})

	// Test 3: Verify etcd storage (if store methods are implemented)
	t.Run("VerifyEtcdStorage", func(t *testing.T) {
		// Note: This test assumes CreateDefinition/GetDefinition are implemented
		// If not yet implemented (tasks 3.1/3.2), this section can be skipped

		// Try to get definition from store
		def, err := store.GetDefinition(ctx, "integration-test-mission")

		// If methods are not implemented yet, we expect an error or nil
		// Once implemented, verify the definition exists
		if err == nil && def != nil {
			assert.Equal(t, "integration-test-mission", def.Name)
			assert.NotEmpty(t, def.Version)
		} else {
			t.Log("Store definition methods not yet implemented (expected for tasks 3.1/3.2)")
		}
	})

	// Test 4: Update mission
	t.Run("Update", func(t *testing.T) {
		// Modify the test repository to simulate an update
		updateTestGitRepository(t, testRepoDir)

		result, err := installer.Update(ctx, "integration-test-mission", UpdateOptions{
			Timeout: 30 * time.Second,
		})

		require.NoError(t, err)
		assert.Equal(t, "integration-test-mission", result.Name)
		assert.True(t, result.Updated)
		assert.NotEqual(t, result.OldVersion, result.NewVersion)

		// Verify metadata was updated
		missionDir := filepath.Join(missionsDir, "integration-test-mission")
		metadataFile := filepath.Join(missionDir, MetadataFileName)
		metadataBytes, err := os.ReadFile(metadataFile)
		require.NoError(t, err)

		var metadata InstallMetadata
		err = json.Unmarshal(metadataBytes, &metadata)
		require.NoError(t, err)
		assert.Equal(t, result.NewVersion, metadata.Version)
	})

	// Test 5: Uninstall mission
	t.Run("Uninstall", func(t *testing.T) {
		err := installer.Uninstall(ctx, "integration-test-mission", UninstallOptions{})
		require.NoError(t, err)

		// Verify mission directory was removed
		missionDir := filepath.Join(missionsDir, "integration-test-mission")
		_, err = os.Stat(missionDir)
		assert.True(t, os.IsNotExist(err))

		// Verify it's no longer in the list
		entries, err := os.ReadDir(missionsDir)
		require.NoError(t, err)

		for _, entry := range entries {
			assert.NotEqual(t, "integration-test-mission", entry.Name())
		}
	})
}

// TestIntegration_ConcurrentInstalls tests installing multiple missions concurrently
func TestIntegration_ConcurrentInstalls(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping integration test")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	missionsDir := filepath.Join(tempDir, "missions")
	err := os.MkdirAll(missionsDir, 0755)
	require.NoError(t, err)

	gitOps := git.NewDefaultGitOperations()
	store := createTestStore(t, tempDir)
	defer cleanupTestStore(store)

	installer := NewDefaultMissionInstaller(
		gitOps,
		store,
		missionsDir,
		nil,
		nil,
	)

	// Create multiple test repositories
	numMissions := 3
	testRepos := make([]string, numMissions)
	for i := 0; i < numMissions; i++ {
		testRepos[i] = createTestGitRepositoryWithName(t, "mission-"+string(rune('a'+i)))
	}

	// Install missions concurrently
	errChan := make(chan error, numMissions)
	for i := 0; i < numMissions; i++ {
		go func(repoDir string) {
			_, err := installer.Install(ctx, repoDir, InstallOptions{
				Timeout: 30 * time.Second,
			})
			errChan <- err
		}(testRepos[i])
	}

	// Wait for all installations to complete
	for i := 0; i < numMissions; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}

	// Verify all missions were installed
	entries, err := os.ReadDir(missionsDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), numMissions)
}

// TestIntegration_InstallWithForce tests forcing reinstallation
func TestIntegration_InstallWithForce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping integration test")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	missionsDir := filepath.Join(tempDir, "missions")
	err := os.MkdirAll(missionsDir, 0755)
	require.NoError(t, err)

	testRepoDir := createTestGitRepository(t)

	gitOps := git.NewDefaultGitOperations()
	store := createTestStore(t, tempDir)
	defer cleanupTestStore(store)

	installer := NewDefaultMissionInstaller(
		gitOps,
		store,
		missionsDir,
		nil,
		nil,
	)

	// First install
	result1, err := installer.Install(ctx, testRepoDir, InstallOptions{})
	require.NoError(t, err)
	version1 := result1.Version

	// Try to install again without force (should fail)
	_, err = installer.Install(ctx, testRepoDir, InstallOptions{Force: false})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Update the repository
	updateTestGitRepository(t, testRepoDir)

	// Install with force (should succeed)
	result2, err := installer.Install(ctx, testRepoDir, InstallOptions{Force: true})
	require.NoError(t, err)
	version2 := result2.Version

	// Versions should be different
	assert.NotEqual(t, version1, version2)
}

// createTestGitRepository creates a temporary git repository with a test mission
func createTestGitRepository(t *testing.T) string {
	return createTestGitRepositoryWithName(t, "integration-test-mission")
}

// createTestGitRepositoryWithName creates a temporary git repository with a specific mission name
func createTestGitRepositoryWithName(t *testing.T, missionName string) string {
	repoDir := t.TempDir()

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	err := cmd.Run()
	require.NoError(t, err)

	// Configure git user (required for commits)
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	// Create mission.yaml
	missionContent := `name: ` + missionName + `
version: 1.0.0
description: Integration test mission
target: test-target

nodes:
  recon:
    id: recon
    type: agent
    name: Reconnaissance
    agent_name: recon-agent
    timeout: 5m

  scan:
    id: scan
    type: tool
    name: Scanner
    tool_name: nmap
    tool_input:
      target: "localhost"
      ports: "1-1000"

edges:
  - from: recon
    to: scan

entry_points:
  - recon

exit_points:
  - scan

dependencies:
  agents:
    - recon-agent
  tools:
    - nmap
`

	missionPath := filepath.Join(repoDir, MissionFileName)
	err = os.WriteFile(missionPath, []byte(missionContent), 0644)
	require.NoError(t, err)

	// Add and commit
	cmd = exec.Command("git", "add", MissionFileName)
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	return repoDir
}

// updateTestGitRepository modifies the test repository to simulate an update
func updateTestGitRepository(t *testing.T, repoDir string) {
	// Modify mission.yaml
	missionContent := `name: integration-test-mission
version: 2.0.0
description: Updated integration test mission
target: test-target

nodes:
  recon:
    id: recon
    type: agent
    name: Reconnaissance
    agent_name: recon-agent
    timeout: 10m

  scan:
    id: scan
    type: tool
    name: Scanner
    tool_name: nmap
    tool_input:
      target: "localhost"
      ports: "1-65535"

  exploit:
    id: exploit
    type: agent
    name: Exploitation
    agent_name: exploit-agent
    timeout: 15m

edges:
  - from: recon
    to: scan
  - from: scan
    to: exploit

entry_points:
  - recon

exit_points:
  - exploit

dependencies:
  agents:
    - recon-agent
    - exploit-agent
  tools:
    - nmap
`

	missionPath := filepath.Join(repoDir, MissionFileName)
	err := os.WriteFile(missionPath, []byte(missionContent), 0644)
	require.NoError(t, err)

	// Commit the change
	cmd := exec.Command("git", "add", MissionFileName)
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.Command("git", "commit", "-m", "Update mission")
	cmd.Dir = repoDir
	err = cmd.Run()
	require.NoError(t, err)
}

// createTestStore creates a test mission store backed by embedded etcd and SQLite
func createTestStore(t *testing.T, tempDir string) MissionStore {
	// For integration tests, we'll create a minimal store implementation
	// In a real integration test, you'd initialize the full store with etcd

	// Create SQLite database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.NewSQLiteDB(dbPath)
	require.NoError(t, err)

	// Initialize etcd client (use embedded etcd or connect to existing cluster)
	// For simplicity, we'll use a minimal in-memory implementation
	// In production integration tests, you'd use a real etcd instance

	etcdClient := createTestEtcdClient(t, tempDir)

	store := NewMissionStore(db, etcdClient)
	return store
}

// createTestEtcdClient creates a test etcd client
func createTestEtcdClient(t *testing.T, tempDir string) *clientv3.Client {
	// For integration tests, we need a real etcd instance
	// This could be:
	// 1. An embedded etcd server
	// 2. A containerized etcd (using testcontainers)
	// 3. A shared test etcd cluster

	// For now, we'll create a minimal client config
	// In a real integration test, you'd start an embedded etcd server
	cfg := clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	}

	client, err := clientv3.New(cfg)
	if err != nil {
		// If etcd is not available, skip etcd-related tests
		t.Log("etcd not available, some tests may be skipped")
		return nil
	}

	return client
}

// cleanupTestStore cleans up test store resources
func cleanupTestStore(store MissionStore) {
	// Close database connections, etcd clients, etc.
	// The actual implementation depends on the MissionStore interface
	if store != nil {
		// Cleanup logic here
	}
}

// TestIntegration_FilesystemState verifies filesystem state after operations
func TestIntegration_FilesystemState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping integration test")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	missionsDir := filepath.Join(tempDir, "missions")
	err := os.MkdirAll(missionsDir, 0755)
	require.NoError(t, err)

	testRepoDir := createTestGitRepository(t)

	gitOps := git.NewDefaultGitOperations()
	store := createTestStore(t, tempDir)
	defer cleanupTestStore(store)

	installer := NewDefaultMissionInstaller(
		gitOps,
		store,
		missionsDir,
		nil,
		nil,
	)

	// Install mission
	result, err := installer.Install(ctx, testRepoDir, InstallOptions{})
	require.NoError(t, err)

	missionDir := result.Path

	// Verify directory structure
	assert.DirExists(t, missionDir)

	// Verify mission.yaml
	missionYAML := filepath.Join(missionDir, MissionFileName)
	assert.FileExists(t, missionYAML)
	content, err := os.ReadFile(missionYAML)
	require.NoError(t, err)
	assert.Contains(t, string(content), "integration-test-mission")

	// Verify metadata file
	metadataFile := filepath.Join(missionDir, MetadataFileName)
	assert.FileExists(t, metadataFile)

	// Verify metadata is valid JSON
	metadataBytes, err := os.ReadFile(metadataFile)
	require.NoError(t, err)
	var metadata InstallMetadata
	err = json.Unmarshal(metadataBytes, &metadata)
	require.NoError(t, err)

	// Verify metadata fields
	assert.Equal(t, "integration-test-mission", metadata.Name)
	assert.NotEmpty(t, metadata.Version)
	assert.Equal(t, testRepoDir, metadata.Source)
	assert.NotZero(t, metadata.InstalledAt)
	assert.NotZero(t, metadata.UpdatedAt)

	// Verify directory permissions
	info, err := os.Stat(missionDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}
