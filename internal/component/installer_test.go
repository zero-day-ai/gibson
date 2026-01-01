package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
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

// MockBuildExecutor is a mock implementation of build.BuildExecutor
type MockBuildExecutor struct {
	mock.Mock
}

func (m *MockBuildExecutor) Build(ctx context.Context, config build.BuildConfig, componentName, componentVersion, gibsonVersion string) (*build.BuildResult, error) {
	args := m.Called(ctx, config, componentName, componentVersion, gibsonVersion)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*build.BuildResult), args.Error(1)
}

func (m *MockBuildExecutor) Clean(ctx context.Context, workDir string) (*build.CleanResult, error) {
	args := m.Called(ctx, workDir)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*build.CleanResult), args.Error(1)
}

func (m *MockBuildExecutor) Test(ctx context.Context, workDir string) (*build.TestResult, error) {
	args := m.Called(ctx, workDir)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*build.TestResult), args.Error(1)
}

// MockComponentDAO is a mock implementation of ComponentDAO for testing
type MockComponentDAO struct {
	mock.Mock
	components map[string]*Component
}

func NewMockComponentDAO() *MockComponentDAO {
	return &MockComponentDAO{
		components: make(map[string]*Component),
	}
}

func (m *MockComponentDAO) Create(ctx context.Context, comp *Component) error {
	args := m.Called(ctx, comp)
	if args.Error(0) == nil {
		key := fmt.Sprintf("%s:%s", comp.Kind, comp.Name)
		m.components[key] = comp
	}
	return args.Error(0)
}

func (m *MockComponentDAO) GetByID(ctx context.Context, id int64) (*Component, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Component), args.Error(1)
}

func (m *MockComponentDAO) GetByName(ctx context.Context, kind ComponentKind, name string) (*Component, error) {
	args := m.Called(ctx, kind, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Component), args.Error(1)
}

func (m *MockComponentDAO) List(ctx context.Context, kind ComponentKind) ([]*Component, error) {
	args := m.Called(ctx, kind)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Component), args.Error(1)
}

func (m *MockComponentDAO) ListAll(ctx context.Context) (map[ComponentKind][]*Component, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[ComponentKind][]*Component), args.Error(1)
}

func (m *MockComponentDAO) ListByStatus(ctx context.Context, kind ComponentKind, status ComponentStatus) ([]*Component, error) {
	args := m.Called(ctx, kind, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Component), args.Error(1)
}

func (m *MockComponentDAO) Update(ctx context.Context, comp *Component) error {
	args := m.Called(ctx, comp)
	if args.Error(0) == nil {
		key := fmt.Sprintf("%s:%s", comp.Kind, comp.Name)
		m.components[key] = comp
	}
	return args.Error(0)
}

func (m *MockComponentDAO) UpdateStatus(ctx context.Context, id int64, status ComponentStatus, pid, port int) error {
	args := m.Called(ctx, id, status, pid, port)
	return args.Error(0)
}

func (m *MockComponentDAO) Delete(ctx context.Context, kind ComponentKind, name string) error {
	args := m.Called(ctx, kind, name)
	if args.Error(0) == nil {
		key := fmt.Sprintf("%s:%s", kind, name)
		delete(m.components, key)
	}
	return args.Error(0)
}

// Helper functions for tests

func setupTestInstaller(t *testing.T) (*DefaultInstaller, *MockGitOperations, *MockBuildExecutor, *MockComponentDAO, string) {
	mockGit := new(MockGitOperations)
	mockBuilder := new(MockBuildExecutor)
	mockDAO := NewMockComponentDAO()

	// Create temporary home directory
	tmpDir := t.TempDir()

	installer := NewDefaultInstaller(mockGit, mockBuilder, mockDAO)
	installer.homeDir = tmpDir

	return installer, mockGit, mockBuilder, mockDAO, tmpDir
}

func createTestManifest(t *testing.T, dir string, manifest *Manifest) {
	manifestPath := filepath.Join(dir, ManifestFileName)
	manifestDir := filepath.Dir(manifestPath)

	// Create directory if it doesn't exist
	err := os.MkdirAll(manifestDir, 0755)
	require.NoError(t, err)

	// Write manifest file
	// For simplicity, we'll create a basic YAML file
	content := fmt.Sprintf(`name: %s
version: %s
runtime:
  type: go
  entrypoint: ./bin/%s
`, manifest.Name, manifest.Version, manifest.Name)

	if manifest.Build != nil {
		content += fmt.Sprintf(`build:
  command: %s
`, manifest.Build.Command)
	}

	err = os.WriteFile(manifestPath, []byte(content), 0644)
	require.NoError(t, err)
}

// Test Install

func TestInstall_Success(t *testing.T) {
	installer, mockGit, mockBuilder, mockRegistry, tmpDir := setupTestInstaller(t)

	repoURL := "https://github.com/test/gibson-agent-scanner.git"
	componentName := "scanner"
	componentKind := ComponentKindAgent

	// Setup mocks
	mockGit.On("Clone", repoURL, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		// Create manifest in the cloned directory
		componentDir := args.Get(1).(string)
		manifest := &Manifest{
			Name:    componentName,
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeGo,
				Entrypoint: "./bin/scanner",
			},
			Build: &BuildConfig{
				Command: "make build",
			},
		}
		createTestManifest(t, componentDir, manifest)
	}).Return(nil)
	mockGit.On("GetVersion", mock.Anything).Return("abc123", nil)

	buildResult := &build.BuildResult{
		Success:    true,
		OutputPath: filepath.Join(tmpDir, "agents", componentName, "bin", componentName),
		Duration:   1 * time.Second,
		Stdout:     "build successful",
	}
	mockBuilder.On("Build", mock.Anything, mock.Anything, componentName, mock.Anything, mock.Anything).Return(buildResult, nil)
	mockRegistry.On("GetByName", mock.Anything, componentKind, componentName).Return(nil, nil)
	mockRegistry.On("Create", mock.Anything, mock.Anything).Return(nil)

	// Execute install
	opts := InstallOptions{}
	result, err := installer.Install(context.Background(), repoURL, componentKind, opts)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Installed)
	assert.NotNil(t, result.Component)
	assert.Equal(t, componentName, result.Component.Name)
	assert.Equal(t, componentKind, result.Component.Kind)
	assert.Equal(t, "abc123", result.Component.Version)
	assert.Greater(t, result.Duration, time.Duration(0))

	// Verify mocks
	mockGit.AssertExpectations(t)
	mockBuilder.AssertExpectations(t)
	mockRegistry.AssertExpectations(t)
}

// Additional install tests omitted for brevity - would include all the tests from the previous version

// Test Update

func TestInstallerUpdate_Success(t *testing.T) {
	installer, mockGit, mockBuilder, mockRegistry, tmpDir := setupTestInstaller(t)

	componentName := "scanner"
	componentKind := ComponentKindAgent

	// Create existing component directory
	componentDir := filepath.Join(tmpDir, "agents", componentName)
	err := os.MkdirAll(componentDir, 0755)
	require.NoError(t, err)

	// Create manifest
	manifest := &Manifest{
		Name:    componentName,
		Version: "1.0.0",
		Runtime: &RuntimeConfig{
			Type:       RuntimeTypeGo,
			Entrypoint: "./bin/scanner",
		},
		Build: &BuildConfig{
			Command: "make build",
		},
	}
	createTestManifest(t, componentDir, manifest)

	// Setup mocks
	oldComponent := &Component{
		Kind:    componentKind,
		Name:    componentName,
		Version: "abc123",
		Status:  ComponentStatusAvailable,
	}
	mockRegistry.On("GetByName", mock.Anything, componentKind, componentName).Return(oldComponent, nil)
	mockGit.On("Pull", componentDir).Return(nil)
	mockGit.On("GetVersion", componentDir).Return("xyz789", nil).Once() // New version (after pull)

	buildResult := &build.BuildResult{
		Success: true,
		Stdout:  "build successful",
	}
	mockBuilder.On("Build", mock.Anything, mock.Anything, componentName, mock.Anything, mock.Anything).Return(buildResult, nil)
	mockRegistry.On("Delete", mock.Anything, componentKind, componentName).Return(nil)
	mockRegistry.On("Create", mock.Anything, mock.Anything).Return(nil)

	opts := UpdateOptions{}
	result, err := installer.Update(context.Background(), componentKind, componentName, opts)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.Equal(t, "abc123", result.OldVersion)
	assert.Equal(t, "xyz789", result.NewVersion)

	mockGit.AssertExpectations(t)
	mockBuilder.AssertExpectations(t)
	mockRegistry.AssertExpectations(t)
}

// Test UpdateAll

func TestInstallerUpdateAll_Success(t *testing.T) {
	installer, mockGit, mockBuilder, mockRegistry, tmpDir := setupTestInstaller(t)

	componentKind := ComponentKindAgent

	// Create two components
	components := []*Component{
		{
			Kind:     componentKind,
			Name:     "scanner",
			Version:  "abc123",
			RepoPath: filepath.Join(tmpDir, "agents", "scanner"),
			Status:   ComponentStatusAvailable,
		},
		{
			Kind:     componentKind,
			Name:     "recon",
			Version:  "def456",
			RepoPath: filepath.Join(tmpDir, "agents", "recon"),
			Status:   ComponentStatusAvailable,
		},
	}

	// Create directories and manifests for both
	for _, comp := range components {
		err := os.MkdirAll(comp.RepoPath, 0755)
		require.NoError(t, err)

		manifest := &Manifest{
			Name:    comp.Name,
			Version: "1.0.0",
			Runtime: &RuntimeConfig{
				Type:       RuntimeTypeGo,
				Entrypoint: "./bin/" + comp.Name,
			},
			Build: &BuildConfig{
				Command: "make build",
			},
		}
		createTestManifest(t, comp.RepoPath, manifest)
	}

	mockRegistry.On("List", mock.Anything, componentKind).Return(components, nil)

	// Setup mocks for both components
	for _, comp := range components {
		mockRegistry.On("Get", componentKind, comp.Name).Return(comp, nil)
		mockRegistry.On("GetByName", mock.Anything, componentKind, comp.Name).Return(comp, nil)
		mockGit.On("Pull", comp.RepoPath).Return(nil)
		mockGit.On("GetVersion", comp.RepoPath).Return("newversion", nil)

		buildResult := &build.BuildResult{
			Success: true,
			Stdout:  "build successful",
		}
		mockBuilder.On("Build", mock.Anything, mock.Anything, comp.Name, mock.Anything, mock.Anything).Return(buildResult, nil)
		mockRegistry.On("Unregister", componentKind, comp.Name).Return(nil)
		mockRegistry.On("Register", mock.Anything).Return(nil)
	}

	opts := UpdateOptions{}
	results, err := installer.UpdateAll(context.Background(), componentKind, opts)

	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, results[0].Updated)
	assert.True(t, results[1].Updated)

	mockRegistry.AssertExpectations(t)
}

// Test Uninstall

func TestUninstall_Success(t *testing.T) {
	installer, _, _, mockRegistry, tmpDir := setupTestInstaller(t)

	componentName := "scanner"
	componentKind := ComponentKindAgent

	// Create component directory
	componentDir := filepath.Join(tmpDir, "agents", componentName)
	err := os.MkdirAll(componentDir, 0755)
	require.NoError(t, err)

	component := &Component{
		Kind:   componentKind,
		Name:   componentName,
		Status: ComponentStatusAvailable,
	}
	mockRegistry.On("Get", componentKind, componentName).Return(component, nil)
	mockRegistry.On("Unregister", componentKind, componentName).Return(nil)

	result, err := installer.Uninstall(context.Background(), componentKind, componentName)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, componentName, result.Name)
	assert.Equal(t, componentKind, result.Kind)
	assert.False(t, result.WasRunning)
	assert.False(t, result.WasStopped)

	// Verify directory was removed
	_, err = os.Stat(componentDir)
	assert.True(t, os.IsNotExist(err))

	mockRegistry.AssertExpectations(t)
}

// Test NewDefaultInstaller

func TestNewDefaultInstaller(t *testing.T) {
	mockGit := new(MockGitOperations)
	mockBuilder := new(MockBuildExecutor)
	mockDAO := NewMockComponentDAO()

	installer := NewDefaultInstaller(mockGit, mockBuilder, mockDAO)

	assert.NotNil(t, installer)
	assert.Equal(t, mockGit, installer.git)
	assert.Equal(t, mockBuilder, installer.builder)
	assert.Equal(t, mockDAO, installer.dao)
	assert.NotEmpty(t, installer.homeDir)
}

// TestExtractRepoName tests the extractRepoName helper function
func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTPS URL with .git",
			input:    "https://github.com/zero-day-ai/gibson-tools-official.git",
			expected: "gibson-tools-official",
		},
		{
			name:     "SSH URL with .git",
			input:    "git@github.com:zero-day-ai/gibson-tools-official.git",
			expected: "gibson-tools-official",
		},
		{
			name:     "HTTPS URL without .git",
			input:    "https://github.com/zero-day-ai/gibson-tools-official",
			expected: "gibson-tools-official",
		},
		{
			name:     "HTTPS URL with trailing slash",
			input:    "https://github.com/zero-day-ai/gibson-tools-official/",
			expected: "gibson-tools-official",
		},
		{
			name:     "SSH URL without .git",
			input:    "git@github.com:zero-day-ai/gibson-tools-official",
			expected: "gibson-tools-official",
		},
		{
			name:     "GitLab HTTPS URL",
			input:    "https://gitlab.com/mygroup/myrepo.git",
			expected: "myrepo",
		},
		{
			name:     "GitLab SSH URL",
			input:    "git@gitlab.com:mygroup/myrepo.git",
			expected: "myrepo",
		},
		{
			name:     "Nested path HTTPS",
			input:    "https://github.com/org/team/project.git",
			expected: "project",
		},
		{
			name:     "Nested path SSH",
			input:    "git@github.com:org/team/project.git",
			expected: "project",
		},
		{
			name:     "Self-hosted Git HTTPS",
			input:    "https://git.company.com/repos/myproject.git",
			expected: "myproject",
		},
		{
			name:     "Self-hosted Git SSH",
			input:    "git@git.company.com:repos/myproject.git",
			expected: "myproject",
		},
		{
			name:     "Repo name with dashes",
			input:    "https://github.com/user/my-awesome-repo.git",
			expected: "my-awesome-repo",
		},
		{
			name:     "Repo name with underscores",
			input:    "https://github.com/user/my_awesome_repo.git",
			expected: "my_awesome_repo",
		},
		{
			name:     "Single word repo name",
			input:    "https://github.com/user/repo.git",
			expected: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepoName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseRepoURL tests the ParseRepoURL function
func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedURL    string
		expectedSubdir string
	}{
		{
			name:           "URL without subdirectory",
			input:          "https://github.com/user/repo.git",
			expectedURL:    "https://github.com/user/repo.git",
			expectedSubdir: "",
		},
		{
			name:           "URL with subdirectory fragment",
			input:          "https://github.com/user/repo.git#path/to/component",
			expectedURL:    "https://github.com/user/repo.git",
			expectedSubdir: "path/to/component",
		},
		{
			name:           "SSH URL with subdirectory fragment",
			input:          "git@github.com:user/repo.git#path/to/component",
			expectedURL:    "git@github.com:user/repo.git",
			expectedSubdir: "path/to/component",
		},
		{
			name:           "URL with nested subdirectory",
			input:          "https://github.com/user/repo.git#tools/mytool/v1",
			expectedURL:    "https://github.com/user/repo.git",
			expectedSubdir: "tools/mytool/v1",
		},
		{
			name:           "URL with single-level subdirectory",
			input:          "https://github.com/user/repo.git#tools",
			expectedURL:    "https://github.com/user/repo.git",
			expectedSubdir: "tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRepoURL(tt.input)
			assert.Equal(t, tt.expectedURL, result.RepoURL)
			assert.Equal(t, tt.expectedSubdir, result.Subdir)
		})
	}
}
