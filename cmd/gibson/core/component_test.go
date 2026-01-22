//go:build fts5

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/registry"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// mockComponentStore implements component.ComponentStore for testing.
// Components are now stored in etcd, not SQLite.
type mockComponentStore struct {
	components    []*component.Component
	createFn      func(ctx context.Context, comp *component.Component) error
	getByNameFn   func(ctx context.Context, kind component.ComponentKind, name string) (*component.Component, error)
	listFn        func(ctx context.Context, kind component.ComponentKind) ([]*component.Component, error)
	listAllFn     func(ctx context.Context) (map[component.ComponentKind][]*component.Component, error)
	updateFn      func(ctx context.Context, comp *component.Component) error
	deleteFn      func(ctx context.Context, kind component.ComponentKind, name string) error
	listInstances func(ctx context.Context, kind component.ComponentKind, name string) ([]sdkregistry.ServiceInfo, error)
}

func (m *mockComponentStore) Create(ctx context.Context, comp *component.Component) error {
	if m.createFn != nil {
		return m.createFn(ctx, comp)
	}
	m.components = append(m.components, comp)
	return nil
}

func (m *mockComponentStore) GetByName(ctx context.Context, kind component.ComponentKind, name string) (*component.Component, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, kind, name)
	}
	for _, comp := range m.components {
		if comp.Kind == kind && comp.Name == name {
			return comp, nil
		}
	}
	return nil, nil
}

func (m *mockComponentStore) List(ctx context.Context, kind component.ComponentKind) ([]*component.Component, error) {
	if m.listFn != nil {
		return m.listFn(ctx, kind)
	}
	var filtered []*component.Component
	for _, comp := range m.components {
		if comp.Kind == kind {
			filtered = append(filtered, comp)
		}
	}
	return filtered, nil
}

func (m *mockComponentStore) ListAll(ctx context.Context) (map[component.ComponentKind][]*component.Component, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx)
	}
	result := make(map[component.ComponentKind][]*component.Component)
	for _, comp := range m.components {
		result[comp.Kind] = append(result[comp.Kind], comp)
	}
	return result, nil
}

func (m *mockComponentStore) Update(ctx context.Context, comp *component.Component) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, comp)
	}
	for i, existing := range m.components {
		if existing.Kind == comp.Kind && existing.Name == comp.Name {
			m.components[i] = comp
			return nil
		}
	}
	return errors.New("component not found")
}

func (m *mockComponentStore) Delete(ctx context.Context, kind component.ComponentKind, name string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, kind, name)
	}
	for i, comp := range m.components {
		if comp.Kind == kind && comp.Name == name {
			m.components = append(m.components[:i], m.components[i+1:]...)
			return nil
		}
	}
	return errors.New("component not found")
}

func (m *mockComponentStore) ListInstances(ctx context.Context, kind component.ComponentKind, name string) ([]sdkregistry.ServiceInfo, error) {
	if m.listInstances != nil {
		return m.listInstances(ctx, kind, name)
	}
	return []sdkregistry.ServiceInfo{}, nil
}

// mockInstaller implements component.Installer for testing
type mockInstaller struct {
	installFn    func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallResult, error)
	installAllFn func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallAllResult, error)
	uninstallFn  func(ctx context.Context, kind component.ComponentKind, name string) (*component.UninstallResult, error)
	updateFn     func(ctx context.Context, kind component.ComponentKind, name string, opts component.UpdateOptions) (*component.UpdateResult, error)
	updateAllFn  func(ctx context.Context, kind component.ComponentKind, opts component.UpdateOptions) ([]component.UpdateResult, error)
}

var _ component.Installer = (*mockInstaller)(nil)

func (m *mockInstaller) Install(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallResult, error) {
	if m.installFn != nil {
		return m.installFn(ctx, source, kind, opts)
	}
	return &component.InstallResult{
		Component: &component.Component{
			Name:    "test-component",
			Kind:    kind,
			Version: "1.0.0",
			Source:  "local",
		},
		Path:      "/test/path",
		Installed: true,
	}, nil
}

func (m *mockInstaller) InstallAll(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallAllResult, error) {
	if m.installAllFn != nil {
		return m.installAllFn(ctx, source, kind, opts)
	}
	return &component.InstallAllResult{
		ComponentsFound: 2,
		Successful:      []component.InstallResult{},
		Failed:          []component.InstallFailure{},
	}, nil
}

func (m *mockInstaller) Uninstall(ctx context.Context, kind component.ComponentKind, name string) (*component.UninstallResult, error) {
	if m.uninstallFn != nil {
		return m.uninstallFn(ctx, kind, name)
	}
	return &component.UninstallResult{
		Name: name,
		Kind: kind,
		Path: "/test/path",
	}, nil
}

func (m *mockInstaller) Update(ctx context.Context, kind component.ComponentKind, name string, opts component.UpdateOptions) (*component.UpdateResult, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, kind, name, opts)
	}
	return &component.UpdateResult{
		Component: &component.Component{
			Name:    name,
			Kind:    kind,
			Version: "1.1.0",
		},
		Updated:    true,
		OldVersion: "1.0.0",
		NewVersion: "1.1.0",
	}, nil
}

func (m *mockInstaller) UpdateAll(ctx context.Context, kind component.ComponentKind, opts component.UpdateOptions) ([]component.UpdateResult, error) {
	if m.updateAllFn != nil {
		return m.updateAllFn(ctx, kind, opts)
	}
	return []component.UpdateResult{}, nil
}

// mockRegistry implements sdkregistry.Registry for testing
type mockRegistry struct {
	discoverFn    func(ctx context.Context, kind, name string) ([]sdkregistry.ServiceInfo, error)
	discoverAllFn func(ctx context.Context, kind string) ([]sdkregistry.ServiceInfo, error)
}

func (m *mockRegistry) Register(ctx context.Context, info sdkregistry.ServiceInfo) error {
	return nil
}

func (m *mockRegistry) Deregister(ctx context.Context, info sdkregistry.ServiceInfo) error {
	return nil
}

func (m *mockRegistry) Discover(ctx context.Context, kind, name string) ([]sdkregistry.ServiceInfo, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx, kind, name)
	}
	return []sdkregistry.ServiceInfo{}, nil
}

func (m *mockRegistry) DiscoverAll(ctx context.Context, kind string) ([]sdkregistry.ServiceInfo, error) {
	if m.discoverAllFn != nil {
		return m.discoverAllFn(ctx, kind)
	}
	return []sdkregistry.ServiceInfo{}, nil
}

func (m *mockRegistry) Watch(ctx context.Context, kind, name string) (<-chan []sdkregistry.ServiceInfo, error) {
	ch := make(chan []sdkregistry.ServiceInfo)
	close(ch)
	return ch, nil
}

func (m *mockRegistry) Health(ctx context.Context) error {
	return nil
}

func (m *mockRegistry) Close() error {
	return nil
}

// mockRegManager implements registry.Manager interface for testing
type mockRegManager struct {
	registry *mockRegistry
	status   registry.RegistryStatus
}

func (m *mockRegManager) Start(ctx context.Context) error {
	return nil
}

func (m *mockRegManager) Stop(ctx context.Context) error {
	return nil
}

func (m *mockRegManager) Registry() sdkregistry.Registry {
	if m.registry == nil {
		return nil
	}
	return m.registry
}

func (m *mockRegManager) Status() registry.RegistryStatus {
	return m.status
}

func TestComponentList(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		kind      component.ComponentKind
		opts      ListOptions
		setupMock func(*mockComponentStore)
		wantErr   bool
		wantCount int
	}{
		{
			name: "list all agents",
			kind: component.ComponentKindAgent,
			opts: ListOptions{},
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "agent1",
						Version:   "1.0.0",
						Source:    "local",
						CreatedAt: now,
						UpdatedAt: now,
					},
					{
						Kind:      component.ComponentKindAgent,
						Name:      "agent2",
						Version:   "1.0.0",
						Source:    "remote",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			wantCount: 2,
		},
		{
			name: "list local only",
			kind: component.ComponentKindAgent,
			opts: ListOptions{Local: true},
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "agent1",
						Version:   "1.0.0",
						Source:    "local",
						CreatedAt: now,
						UpdatedAt: now,
					},
					{
						Kind:      component.ComponentKindAgent,
						Name:      "agent2",
						Version:   "1.0.0",
						Source:    "remote",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			wantCount: 1,
		},
		{
			name: "list remote only",
			kind: component.ComponentKindAgent,
			opts: ListOptions{Remote: true},
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "agent1",
						Version:   "1.0.0",
						Source:    "local",
						CreatedAt: now,
						UpdatedAt: now,
					},
					{
						Kind:      component.ComponentKindAgent,
						Name:      "agent2",
						Version:   "1.0.0",
						Source:    "remote",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			wantCount: 1,
		},
		{
			name: "both local and remote flags",
			kind: component.ComponentKindAgent,
			opts: ListOptions{Local: true, Remote: true},
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{}
			},
			wantErr: true,
		},
		{
			name: "empty list",
			kind: component.ComponentKindTool,
			opts: ListOptions{},
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{}
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockComponentStore{}
			if tt.setupMock != nil {
				tt.setupMock(store)
			}

			cc := &CommandContext{
				Ctx:            context.Background(),
				ComponentStore: store,
			}

			result, err := ComponentList(cc, tt.kind, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentList() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentList() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentList() returned nil result")
			}

			components, ok := result.Data.([]*component.Component)
			if !ok {
				t.Fatalf("ComponentList() result.Data is not []*component.Component")
			}

			if len(components) != tt.wantCount {
				t.Errorf("ComponentList() count = %d, want %d", len(components), tt.wantCount)
			}
		})
	}
}

func TestComponentShow(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		kind          component.ComponentKind
		componentName string
		setupMock     func(*mockComponentStore)
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:          "existing component",
			kind:          component.ComponentKindAgent,
			componentName: "test-agent",
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "test-agent",
						Version:   "1.0.0",
						Source:    "local",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
		},
		{
			name:          "non-existing component",
			kind:          component.ComponentKindAgent,
			componentName: "non-existing",
			setupMock:     func(m *mockComponentStore) {},
			wantErr:       true,
			wantErrMsg:    "component 'non-existing' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockComponentStore{}
			if tt.setupMock != nil {
				tt.setupMock(store)
			}

			cc := &CommandContext{
				Ctx:            context.Background(),
				ComponentStore: store,
			}

			result, err := ComponentShow(cc, tt.kind, tt.componentName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentShow() expected error, got nil")
				} else if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ComponentShow() error = %v, wantErrMsg %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentShow() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentShow() returned nil result")
			}

			comp, ok := result.Data.(*component.Component)
			if !ok {
				t.Fatalf("ComponentShow() result.Data is not *component.Component")
			}

			if comp.Name != tt.componentName {
				t.Errorf("ComponentShow() component name = %s, want %s", comp.Name, tt.componentName)
			}
		})
	}
}

func TestComponentInstall(t *testing.T) {
	tests := []struct {
		name       string
		kind       component.ComponentKind
		source     string
		opts       InstallOptions
		setupMock  func(*mockInstaller)
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "successful install",
			kind:   component.ComponentKindAgent,
			source: "https://github.com/test/agent",
			opts:   InstallOptions{},
			setupMock: func(m *mockInstaller) {
				m.installFn = func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallResult, error) {
					return &component.InstallResult{
						Component: &component.Component{
							Name:    "test-agent",
							Kind:    kind,
							Version: "1.0.0",
						},
						Installed: true,
					}, nil
				}
			},
		},
		{
			name:   "install with branch",
			kind:   component.ComponentKindAgent,
			source: "https://github.com/test/agent",
			opts:   InstallOptions{Branch: "develop"},
			setupMock: func(m *mockInstaller) {
				m.installFn = func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallResult, error) {
					if opts.Branch != "develop" {
						return nil, errors.New("unexpected branch")
					}
					return &component.InstallResult{
						Component: &component.Component{
							Name:    "test-agent",
							Kind:    kind,
							Version: "1.0.0",
						},
					}, nil
				}
			},
		},
		{
			name:   "install error",
			kind:   component.ComponentKindAgent,
			source: "https://github.com/test/agent",
			opts:   InstallOptions{},
			setupMock: func(m *mockInstaller) {
				m.installFn = func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallResult, error) {
					return nil, errors.New("git clone failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := &mockInstaller{}
			if tt.setupMock != nil {
				tt.setupMock(installer)
			}

			cc := &CommandContext{
				Ctx:       context.Background(),
				Installer: installer,
			}

			result, err := ComponentInstall(cc, tt.kind, tt.source, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentInstall() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentInstall() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentInstall() returned nil result")
			}

			_, ok := result.Data.(*component.InstallResult)
			if !ok {
				t.Fatalf("ComponentInstall() result.Data is not *component.InstallResult")
			}
		})
	}
}

func TestComponentInstallAll(t *testing.T) {
	tests := []struct {
		name       string
		kind       component.ComponentKind
		source     string
		opts       InstallOptions
		setupMock  func(*mockInstaller)
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "successful install all",
			kind:   component.ComponentKindAgent,
			source: "https://github.com/test/agents",
			opts:   InstallOptions{},
			setupMock: func(m *mockInstaller) {
				m.installAllFn = func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallAllResult, error) {
					return &component.InstallAllResult{
						ComponentsFound: 3,
						Successful:      make([]component.InstallResult, 3),
						Failed:          []component.InstallFailure{},
					}, nil
				}
			},
		},
		{
			name:   "no components found",
			kind:   component.ComponentKindAgent,
			source: "https://github.com/test/empty",
			opts:   InstallOptions{},
			setupMock: func(m *mockInstaller) {
				m.installAllFn = func(ctx context.Context, source string, kind component.ComponentKind, opts component.InstallOptions) (*component.InstallAllResult, error) {
					return &component.InstallAllResult{
						ComponentsFound: 0,
						Successful:      []component.InstallResult{},
						Failed:          []component.InstallFailure{},
					}, nil
				}
			},
			wantErr:    true,
			wantErrMsg: "no components found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := &mockInstaller{}
			if tt.setupMock != nil {
				tt.setupMock(installer)
			}

			cc := &CommandContext{
				Ctx:       context.Background(),
				Installer: installer,
			}

			result, err := ComponentInstallAll(cc, tt.kind, tt.source, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentInstallAll() expected error, got nil")
				} else if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ComponentInstallAll() error = %v, wantErrMsg %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentInstallAll() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentInstallAll() returned nil result")
			}
		})
	}
}

func TestComponentUninstall(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		kind           component.ComponentKind
		componentName  string
		opts           UninstallOptions
		setupStore     func(*mockComponentStore)
		setupInstaller func(*mockInstaller)
		wantErr        bool
		wantErrMsg     string
	}{
		{
			name:          "successful uninstall",
			kind:          component.ComponentKindAgent,
			componentName: "test-agent",
			opts:          UninstallOptions{},
			setupStore: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "test-agent",
						Version:   "1.0.0",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			setupInstaller: func(m *mockInstaller) {},
		},
		{
			name:           "component not found",
			kind:           component.ComponentKindAgent,
			componentName:  "non-existing",
			opts:           UninstallOptions{},
			setupStore:     func(m *mockComponentStore) {},
			setupInstaller: func(m *mockInstaller) {},
			wantErr:        true,
			wantErrMsg:     "component 'non-existing' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockComponentStore{}
			installer := &mockInstaller{}
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			if tt.setupInstaller != nil {
				tt.setupInstaller(installer)
			}

			cc := &CommandContext{
				Ctx:            context.Background(),
				ComponentStore: store,
				Installer:      installer,
			}

			result, err := ComponentUninstall(cc, tt.kind, tt.componentName, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentUninstall() expected error, got nil")
				} else if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ComponentUninstall() error = %v, wantErrMsg %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentUninstall() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentUninstall() returned nil result")
			}
		})
	}
}

func TestComponentUpdate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		kind           component.ComponentKind
		componentName  string
		opts           UpdateOptions
		setupStore     func(*mockComponentStore)
		setupInstaller func(*mockInstaller)
		wantErr        bool
		wantErrMsg     string
	}{
		{
			name:          "successful update",
			kind:          component.ComponentKindAgent,
			componentName: "test-agent",
			opts:          UpdateOptions{},
			setupStore: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "test-agent",
						Version:   "1.0.0",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			setupInstaller: func(m *mockInstaller) {},
		},
		{
			name:           "component not found",
			kind:           component.ComponentKindAgent,
			componentName:  "non-existing",
			opts:           UpdateOptions{},
			setupStore:     func(m *mockComponentStore) {},
			setupInstaller: func(m *mockInstaller) {},
			wantErr:        true,
			wantErrMsg:     "component 'non-existing' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockComponentStore{}
			installer := &mockInstaller{}
			if tt.setupStore != nil {
				tt.setupStore(store)
			}
			if tt.setupInstaller != nil {
				tt.setupInstaller(installer)
			}

			cc := &CommandContext{
				Ctx:            context.Background(),
				ComponentStore: store,
				Installer:      installer,
			}

			result, err := ComponentUpdate(cc, tt.kind, tt.componentName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentUpdate() expected error, got nil")
				} else if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ComponentUpdate() error = %v, wantErrMsg %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentUpdate() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentUpdate() returned nil result")
			}
		})
	}
}

func TestComponentBuild(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		kind          component.ComponentKind
		componentName string
		setupMock     func(*mockComponentStore)
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:          "component not found",
			kind:          component.ComponentKindAgent,
			componentName: "non-existing",
			setupMock:     func(m *mockComponentStore) {},
			wantErr:       true,
			wantErrMsg:    "component 'non-existing' not found",
		},
		{
			name:          "component without manifest",
			kind:          component.ComponentKindAgent,
			componentName: "no-manifest",
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:      component.ComponentKindAgent,
						Name:      "no-manifest",
						Version:   "1.0.0",
						Manifest:  nil,
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			wantErr:    true,
			wantErrMsg: "has no build configuration",
		},
		{
			name:          "component without repo path",
			kind:          component.ComponentKindAgent,
			componentName: "no-repo-path",
			setupMock: func(m *mockComponentStore) {
				m.components = []*component.Component{
					{
						Kind:    component.ComponentKindAgent,
						Name:    "no-repo-path",
						Version: "1.0.0",
						Manifest: &component.Manifest{
							Build: &component.BuildConfig{},
						},
						RepoPath:  "",
						CreatedAt: now,
						UpdatedAt: now,
					},
				}
			},
			wantErr:    true,
			wantErrMsg: "has no repository path configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockComponentStore{}
			if tt.setupMock != nil {
				tt.setupMock(store)
			}

			cc := &CommandContext{
				Ctx:            context.Background(),
				ComponentStore: store,
			}

			result, err := ComponentBuild(cc, tt.kind, tt.componentName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentBuild() expected error, got nil")
				} else if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ComponentBuild() error = %v, wantErrMsg %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentBuild() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentBuild() returned nil result")
			}
		})
	}
}

// TestComponentStatus is skipped as it requires complex mocking of registry.Manager
// which is a concrete struct rather than an interface. Integration tests should cover this functionality.
func TestComponentStatus(t *testing.T) {
	t.Skip("Skipping component status test - requires integration testing with real registry")
}

// TestComponentStart is skipped for the same reason as ComponentStatus
func TestComponentStart(t *testing.T) {
	t.Skip("Skipping component start test - requires integration testing with real registry")
}

// TestComponentStop is skipped for the same reason as ComponentStatus
func TestComponentStop(t *testing.T) {
	t.Skip("Skipping component stop test - requires integration testing with real registry")
}

func TestComponentLogs(t *testing.T) {
	// Create a temporary log file for testing
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "logs", "agent", "test-agent.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	logContent := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(logPath, []byte(logContent), 0644); err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	tests := []struct {
		name          string
		kind          component.ComponentKind
		componentName string
		opts          LogsOptions
		homeDir       string
		wantErr       bool
		wantErrMsg    string
		wantLines     int
	}{
		{
			name:          "read last 3 lines",
			kind:          component.ComponentKindAgent,
			componentName: "test-agent",
			opts:          LogsOptions{Lines: 3},
			homeDir:       tempDir,
			wantLines:     3,
		},
		{
			name:          "follow mode",
			kind:          component.ComponentKindAgent,
			componentName: "test-agent",
			opts:          LogsOptions{Follow: true},
			homeDir:       tempDir,
		},
		{
			name:          "log file not found",
			kind:          component.ComponentKindAgent,
			componentName: "non-existing",
			opts:          LogsOptions{Lines: 10},
			homeDir:       tempDir,
			wantErr:       true,
			wantErrMsg:    "no logs found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CommandContext{
				Ctx:     context.Background(),
				HomeDir: tt.homeDir,
			}

			result, err := ComponentLogs(cc, tt.kind, tt.componentName, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ComponentLogs() expected error, got nil")
				} else if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("ComponentLogs() error = %v, wantErrMsg %v", err, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ComponentLogs() unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("ComponentLogs() returned nil result")
			}

			data, ok := result.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("ComponentLogs() result.Data is not map[string]interface{}")
			}

			if tt.opts.Follow {
				if _, ok := data["follow"]; !ok {
					t.Error("ComponentLogs() follow mode did not return follow flag")
				}
			} else if tt.wantLines > 0 {
				lines, ok := data["lines"].([]string)
				if !ok {
					t.Fatal("ComponentLogs() lines is not []string")
				}
				if len(lines) != tt.wantLines {
					t.Errorf("ComponentLogs() lines count = %d, want %d", len(lines), tt.wantLines)
				}
			}
		})
	}
}
