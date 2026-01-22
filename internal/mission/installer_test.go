package mission

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MockGitOperations is a mock implementation of git.GitOperations
type MockGitOperations struct {
	mock.Mock
}

func (m *MockGitOperations) Clone(url, dest string, opts git.CloneOptions) error {
	args := m.Called(url, dest, opts)
	return args.Error(0)
}

func (m *MockGitOperations) Pull(dir string) error {
	args := m.Called(dir)
	return args.Error(0)
}

func (m *MockGitOperations) GetVersion(dir string) (string, error) {
	args := m.Called(dir)
	return args.String(0), args.Error(1)
}

func (m *MockGitOperations) ParseRepoURL(url string) (*git.RepoInfo, error) {
	args := m.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*git.RepoInfo), args.Error(1)
}

// MockMissionStore is a mock implementation of MissionStore
type MockMissionStore struct {
	mock.Mock
}

func (m *MockMissionStore) Save(ctx context.Context, mission *Mission) error {
	args := m.Called(ctx, mission)
	return args.Error(0)
}

func (m *MockMissionStore) Get(ctx context.Context, id types.ID) (*Mission, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Mission), args.Error(1)
}

func (m *MockMissionStore) GetByName(ctx context.Context, name string) (*Mission, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Mission), args.Error(1)
}

func (m *MockMissionStore) List(ctx context.Context, filter *MissionFilter) ([]*Mission, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Mission), args.Error(1)
}

func (m *MockMissionStore) Update(ctx context.Context, mission *Mission) error {
	args := m.Called(ctx, mission)
	return args.Error(0)
}

func (m *MockMissionStore) UpdateStatus(ctx context.Context, id types.ID, status MissionStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockMissionStore) UpdateProgress(ctx context.Context, id types.ID, progress float64) error {
	args := m.Called(ctx, id, progress)
	return args.Error(0)
}

func (m *MockMissionStore) Delete(ctx context.Context, id types.ID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMissionStore) GetByTarget(ctx context.Context, targetID types.ID) ([]*Mission, error) {
	args := m.Called(ctx, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Mission), args.Error(1)
}

func (m *MockMissionStore) GetActive(ctx context.Context) ([]*Mission, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Mission), args.Error(1)
}

func (m *MockMissionStore) SaveCheckpoint(ctx context.Context, missionID types.ID, checkpoint *MissionCheckpoint) error {
	args := m.Called(ctx, missionID, checkpoint)
	return args.Error(0)
}

func (m *MockMissionStore) Count(ctx context.Context, filter *MissionFilter) (int, error) {
	args := m.Called(ctx, filter)
	return args.Int(0), args.Error(1)
}

func (m *MockMissionStore) GetByNameAndStatus(ctx context.Context, name string, status MissionStatus) (*Mission, error) {
	args := m.Called(ctx, name, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Mission), args.Error(1)
}

func (m *MockMissionStore) ListByName(ctx context.Context, name string, limit int) ([]*Mission, error) {
	args := m.Called(ctx, name, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Mission), args.Error(1)
}

func (m *MockMissionStore) GetLatestByName(ctx context.Context, name string) (*Mission, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Mission), args.Error(1)
}

func (m *MockMissionStore) IncrementRunNumber(ctx context.Context, name string) (int, error) {
	args := m.Called(ctx, name)
	return args.Int(0), args.Error(1)
}

func (m *MockMissionStore) FindOrCreateByName(ctx context.Context, mission *Mission) (*Mission, bool, error) {
	args := m.Called(ctx, mission)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*Mission), args.Bool(1), args.Error(2)
}

func (m *MockMissionStore) CreateDefinition(ctx context.Context, def *MissionDefinition) error {
	args := m.Called(ctx, def)
	return args.Error(0)
}

func (m *MockMissionStore) GetDefinition(ctx context.Context, name string) (*MissionDefinition, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*MissionDefinition), args.Error(1)
}

func (m *MockMissionStore) ListDefinitions(ctx context.Context) ([]*MissionDefinition, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*MissionDefinition), args.Error(1)
}

func (m *MockMissionStore) UpdateDefinition(ctx context.Context, def *MissionDefinition) error {
	args := m.Called(ctx, def)
	return args.Error(0)
}

func (m *MockMissionStore) DeleteDefinition(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

// MockComponentStore is a mock implementation of ComponentStore
type MockComponentStore struct {
	mock.Mock
}

func (m *MockComponentStore) GetByName(ctx context.Context, kind ComponentKind, name string) (*Component, error) {
	args := m.Called(ctx, kind, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Component), args.Error(1)
}

// MockComponentInstaller is a mock implementation of ComponentInstaller
type MockComponentInstaller struct {
	mock.Mock
}

func (m *MockComponentInstaller) Install(ctx context.Context, url string, kind ComponentKind, opts ComponentInstallOptions) (*ComponentInstallResult, error) {
	args := m.Called(ctx, url, kind, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ComponentInstallResult), args.Error(1)
}

// createTestMissionYAML creates a temporary mission.yaml file for testing
func createTestMissionYAML(t *testing.T, dir string, name string) string {
	content := `name: ` + name + `
version: 1.0.0
description: Test mission
nodes:
  node1:
    id: node1
    type: agent
    agent_name: test-agent
`
	missionPath := filepath.Join(dir, MissionFileName)
	err := os.WriteFile(missionPath, []byte(content), 0644)
	require.NoError(t, err)
	return missionPath
}

func TestInstall_Success(t *testing.T) {
	// Setup
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil, // No component dependencies for this test
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Mock git clone - will be called on temp directory
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Run(func(args mock.Arguments) {
			// Create mission.yaml in the temp directory
			tempDir := args.String(1)
			createTestMissionYAML(t, tempDir, "test-mission")
		}).
		Return(nil)

	// Mock git version
	mockGit.On("GetVersion", mock.AnythingOfType("string")).Return("abc123", nil)

	// Execute
	result, err := installer.Install(ctx, testURL, InstallOptions{})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "test-mission", result.Name)
	assert.Equal(t, "abc123", result.Version)
	assert.NotEmpty(t, result.Path)

	// Verify mission.yaml was copied
	missionYAML := filepath.Join(result.Path, MissionFileName)
	assert.FileExists(t, missionYAML)

	// Verify metadata file was created
	metadataPath := filepath.Join(result.Path, MetadataFileName)
	assert.FileExists(t, metadataPath)

	// Verify metadata content
	metadataBytes, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	var metadata InstallMetadata
	err = json.Unmarshal(metadataBytes, &metadata)
	require.NoError(t, err)
	assert.Equal(t, "test-mission", metadata.Name)
	assert.Equal(t, "abc123", metadata.Version)
	assert.Equal(t, testURL, metadata.Source)

	mockGit.AssertExpectations(t)
}

func TestInstall_AlreadyExists(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Pre-create mission directory
	missionDir := filepath.Join(tempMissionsDir, "test-mission")
	err := os.MkdirAll(missionDir, 0755)
	require.NoError(t, err)

	// Mock git operations
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Run(func(args mock.Arguments) {
			tempDir := args.String(1)
			createTestMissionYAML(t, tempDir, "test-mission")
		}).
		Return(nil)

	// Execute without force flag
	_, err = installer.Install(ctx, testURL, InstallOptions{Force: false})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	mockGit.AssertNotCalled(t, "GetVersion")
}

func TestInstall_ForceOverwrite(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Pre-create mission directory with old content
	missionDir := filepath.Join(tempMissionsDir, "test-mission")
	err := os.MkdirAll(missionDir, 0755)
	require.NoError(t, err)
	oldFile := filepath.Join(missionDir, "old-file.txt")
	err = os.WriteFile(oldFile, []byte("old content"), 0644)
	require.NoError(t, err)

	// Mock git operations
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Run(func(args mock.Arguments) {
			tempDir := args.String(1)
			createTestMissionYAML(t, tempDir, "test-mission")
		}).
		Return(nil)

	mockGit.On("GetVersion", mock.AnythingOfType("string")).Return("abc123", nil)

	// Execute with force flag
	result, err := installer.Install(ctx, testURL, InstallOptions{Force: true})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "test-mission", result.Name)

	// Verify old file was removed
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))

	// Verify new mission.yaml exists
	missionYAML := filepath.Join(result.Path, MissionFileName)
	assert.FileExists(t, missionYAML)

	mockGit.AssertExpectations(t)
}

func TestInstall_GitCloneFails(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Mock git clone failure
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Return(errors.New("git clone failed: repository not found"))

	// Execute
	_, err := installer.Install(ctx, testURL, InstallOptions{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to clone repository")

	mockGit.AssertExpectations(t)
}

func TestInstall_MissionYAMLNotFound(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Mock git clone but don't create mission.yaml
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Return(nil)

	// Execute
	_, err := installer.Install(ctx, testURL, InstallOptions{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mission.yaml not found")

	mockGit.AssertExpectations(t)
}

func TestInstall_WithBranchAndTag(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Mock git clone with specific options
	expectedOpts := git.CloneOptions{
		Branch: "develop",
		Tag:    "v1.0.0",
	}

	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), expectedOpts).
		Run(func(args mock.Arguments) {
			tempDir := args.String(1)
			createTestMissionYAML(t, tempDir, "test-mission")
		}).
		Return(nil)

	mockGit.On("GetVersion", mock.AnythingOfType("string")).Return("abc123", nil)

	// Execute
	_, err := installer.Install(ctx, testURL, InstallOptions{
		Branch: "develop",
		Tag:    "v1.0.0",
	})

	// Assert
	require.NoError(t, err)
	mockGit.AssertExpectations(t)
}

func TestUninstall_Success(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()

	// Create mission directory
	missionDir := filepath.Join(tempMissionsDir, "test-mission")
	err := os.MkdirAll(missionDir, 0755)
	require.NoError(t, err)
	createTestMissionYAML(t, missionDir, "test-mission")

	// Execute
	err = installer.Uninstall(ctx, "test-mission", UninstallOptions{})

	// Assert
	require.NoError(t, err)

	// Verify directory was removed
	_, err = os.Stat(missionDir)
	assert.True(t, os.IsNotExist(err))
}

func TestUninstall_NotFound(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()

	// Execute
	err := installer.Uninstall(ctx, "nonexistent-mission", UninstallOptions{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdate_Success(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Create existing mission with metadata
	missionDir := filepath.Join(tempMissionsDir, "test-mission")
	err := os.MkdirAll(missionDir, 0755)
	require.NoError(t, err)
	createTestMissionYAML(t, missionDir, "test-mission")

	oldMetadata := InstallMetadata{
		Name:        "test-mission",
		Version:     "old123",
		Source:      testURL,
		InstalledAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-24 * time.Hour),
	}
	metadataJSON, err := json.Marshal(oldMetadata)
	require.NoError(t, err)
	metadataPath := filepath.Join(missionDir, MetadataFileName)
	err = os.WriteFile(metadataPath, metadataJSON, 0644)
	require.NoError(t, err)

	// Mock git operations
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Run(func(args mock.Arguments) {
			tempDir := args.String(1)
			createTestMissionYAML(t, tempDir, "test-mission")
		}).
		Return(nil)

	mockGit.On("GetVersion", mock.AnythingOfType("string")).Return("new456", nil)

	// Execute
	result, err := installer.Update(ctx, "test-mission", UpdateOptions{})

	// Assert
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.Equal(t, "old123", result.OldVersion)
	assert.Equal(t, "new456", result.NewVersion)

	// Verify metadata was updated
	metadataBytes, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	var updatedMetadata InstallMetadata
	err = json.Unmarshal(metadataBytes, &updatedMetadata)
	require.NoError(t, err)
	assert.Equal(t, "new456", updatedMetadata.Version)

	mockGit.AssertExpectations(t)
}

func TestUpdate_NoChanges(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Create existing mission with metadata
	missionDir := filepath.Join(tempMissionsDir, "test-mission")
	err := os.MkdirAll(missionDir, 0755)
	require.NoError(t, err)

	oldMetadata := InstallMetadata{
		Name:        "test-mission",
		Version:     "abc123",
		Source:      testURL,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	metadataJSON, err := json.Marshal(oldMetadata)
	require.NoError(t, err)
	metadataPath := filepath.Join(missionDir, MetadataFileName)
	err = os.WriteFile(metadataPath, metadataJSON, 0644)
	require.NoError(t, err)

	// Mock git operations - return same version
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Return(nil)

	mockGit.On("GetVersion", mock.AnythingOfType("string")).Return("abc123", nil)

	// Execute
	result, err := installer.Update(ctx, "test-mission", UpdateOptions{})

	// Assert
	require.NoError(t, err)
	assert.False(t, result.Updated)
	assert.Equal(t, "abc123", result.OldVersion)
	assert.Equal(t, "abc123", result.NewVersion)

	mockGit.AssertExpectations(t)
}

func TestUpdate_NotFound(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()

	// Execute
	_, err := installer.Update(ctx, "nonexistent-mission", UpdateOptions{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInstall_Rollback(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockStore := new(MockMissionStore)
	tempMissionsDir := t.TempDir()

	installer := NewDefaultMissionInstaller(
		mockGit,
		mockStore,
		tempMissionsDir,
		nil,
		nil,
	)

	ctx := context.Background()
	testURL := "https://github.com/test/mission"

	// Mock git clone succeeds, but we'll create invalid mission.yaml
	mockGit.On("Clone", testURL, mock.AnythingOfType("string"), mock.Anything).
		Run(func(args mock.Arguments) {
			// Create empty mission.yaml which will fail validation
			tempDir := args.String(1)
			missionPath := filepath.Join(tempDir, MissionFileName)
			err := os.WriteFile(missionPath, []byte(""), 0644)
			require.NoError(t, err)
		}).
		Return(nil)

	// Execute
	_, err := installer.Install(ctx, testURL, InstallOptions{})

	// Assert
	assert.Error(t, err)

	// Verify no mission directory was left behind
	missionDir := filepath.Join(tempMissionsDir, "test-mission")
	_, err = os.Stat(missionDir)
	// Directory should not exist (rollback cleaned it up)
	assert.True(t, os.IsNotExist(err))

	mockGit.AssertExpectations(t)
}

func TestCopyFile(t *testing.T) {
	// Create source file
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "source.txt")
	content := "test content"
	err := os.WriteFile(srcPath, []byte(content), 0644)
	require.NoError(t, err)

	// Copy file
	dstPath := filepath.Join(tempDir, "dest.txt")
	err = copyFile(srcPath, dstPath)
	require.NoError(t, err)

	// Verify content
	dstContent, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(dstContent))

	// Verify permissions
	srcInfo, err := os.Stat(srcPath)
	require.NoError(t, err)
	dstInfo, err := os.Stat(dstPath)
	require.NoError(t, err)
	assert.Equal(t, srcInfo.Mode(), dstInfo.Mode())
}

func TestInstallMetadata_JSON(t *testing.T) {
	metadata := InstallMetadata{
		Name:        "test-mission",
		Version:     "1.0.0",
		Source:      "https://github.com/test/mission",
		Branch:      "main",
		Tag:         "v1.0.0",
		Commit:      "abc123",
		InstalledAt: time.Now().Truncate(time.Second),
		UpdatedAt:   time.Now().Truncate(time.Second),
	}

	// JSON round-trip
	jsonData, err := json.Marshal(metadata)
	require.NoError(t, err)

	var decoded InstallMetadata
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, metadata.Name, decoded.Name)
	assert.Equal(t, metadata.Version, decoded.Version)
	assert.Equal(t, metadata.Source, decoded.Source)
	assert.Equal(t, metadata.Commit, decoded.Commit)
}
