package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// mockRegistry implements registry.Registry for testing
type mockToolRegistry struct {
	sdkregistry.Registry

	discoverFunc    func(ctx context.Context, componentType, name string) ([]sdkregistry.ServiceInfo, error)
	discoverAllFunc func(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error)
}

func (m *mockToolRegistry) Discover(ctx context.Context, componentType, name string) ([]sdkregistry.ServiceInfo, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, componentType, name)
	}
	return []sdkregistry.ServiceInfo{}, nil
}

func (m *mockToolRegistry) DiscoverAll(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error) {
	if m.discoverAllFunc != nil {
		return m.discoverAllFunc(ctx, componentType)
	}
	return []sdkregistry.ServiceInfo{}, nil
}

func TestRegistryAdapter_DiscoverTool_NotFound(t *testing.T) {
	mockReg := &mockToolRegistry{
		discoverFunc: func(ctx context.Context, componentType, name string) ([]sdkregistry.ServiceInfo, error) {
			// Return empty list when tool not found
			return []sdkregistry.ServiceInfo{}, nil
		},
		discoverAllFunc: func(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error) {
			// Return list of available tools
			return []sdkregistry.ServiceInfo{
				{Name: "nmap", Version: "1.0.0"},
				{Name: "sqlmap", Version: "1.0.0"},
			}, nil
		},
	}

	adapter := NewRegistryAdapter(mockReg)
	ctx := context.Background()

	tool, err := adapter.DiscoverTool(ctx, "nonexistent")

	assert.Nil(t, tool)
	assert.Error(t, err)

	var notFoundErr *ToolNotFoundError
	require.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, "nonexistent", notFoundErr.Name)
	assert.Contains(t, notFoundErr.Available, "nmap")
	assert.Contains(t, notFoundErr.Available, "sqlmap")
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "nmap")
}

func TestRegistryAdapter_DiscoverTool_NotFoundNoTools(t *testing.T) {
	mockReg := &mockToolRegistry{
		discoverFunc: func(ctx context.Context, componentType, name string) ([]sdkregistry.ServiceInfo, error) {
			return []sdkregistry.ServiceInfo{}, nil
		},
		discoverAllFunc: func(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error) {
			return []sdkregistry.ServiceInfo{}, nil
		},
	}

	adapter := NewRegistryAdapter(mockReg)
	ctx := context.Background()

	tool, err := adapter.DiscoverTool(ctx, "nmap")

	assert.Nil(t, tool)
	assert.Error(t, err)

	var notFoundErr *ToolNotFoundError
	require.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, "nmap", notFoundErr.Name)
	assert.Empty(t, notFoundErr.Available)
	assert.Contains(t, err.Error(), "no tools registered")
}

func TestRegistryAdapter_DiscoverTool_RegistryUnavailable(t *testing.T) {
	mockReg := &mockToolRegistry{
		discoverFunc: func(ctx context.Context, componentType, name string) ([]sdkregistry.ServiceInfo, error) {
			return nil, errors.New("etcd connection failed")
		},
	}

	adapter := NewRegistryAdapter(mockReg)
	ctx := context.Background()

	tool, err := adapter.DiscoverTool(ctx, "nmap")

	assert.Nil(t, tool)
	assert.Error(t, err)

	var unavailableErr *RegistryUnavailableError
	require.True(t, errors.As(err, &unavailableErr))
	assert.Contains(t, err.Error(), "registry unavailable")
	assert.Contains(t, err.Error(), "etcd connection failed")
}

func TestRegistryAdapter_DiscoverTool_ListAvailableFails(t *testing.T) {
	mockReg := &mockToolRegistry{
		discoverFunc: func(ctx context.Context, componentType, name string) ([]sdkregistry.ServiceInfo, error) {
			// Tool not found
			return []sdkregistry.ServiceInfo{}, nil
		},
		discoverAllFunc: func(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error) {
			// But listing available tools also fails
			return nil, errors.New("registry error")
		},
	}

	adapter := NewRegistryAdapter(mockReg)
	ctx := context.Background()

	tool, err := adapter.DiscoverTool(ctx, "nmap")

	assert.Nil(t, tool)
	assert.Error(t, err)

	// Should still get ToolNotFoundError, just with empty available list
	var notFoundErr *ToolNotFoundError
	require.True(t, errors.As(err, &notFoundErr))
	assert.Equal(t, "nmap", notFoundErr.Name)
	assert.Empty(t, notFoundErr.Available)
}

func TestToolNotFoundError_Error(t *testing.T) {
	tests := []struct {
		name      string
		err       *ToolNotFoundError
		wantMsg   string
		wantParts []string
	}{
		{
			name: "with available tools",
			err: &ToolNotFoundError{
				Name:      "hydra",
				Available: []string{"nmap", "sqlmap"},
			},
			wantMsg: "tool 'hydra' not found (available: nmap, sqlmap)",
		},
		{
			name: "no available tools",
			err: &ToolNotFoundError{
				Name:      "hydra",
				Available: []string{},
			},
			wantMsg: "tool 'hydra' not found (no tools registered)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			assert.Equal(t, tt.wantMsg, msg)
		})
	}
}

func TestRegistryAdapter_GetAvailableToolNames(t *testing.T) {
	mockReg := &mockToolRegistry{
		discoverAllFunc: func(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error) {
			assert.Equal(t, "tool", componentType)
			return []sdkregistry.ServiceInfo{
				{Name: "nmap", Version: "1.0.0"},
				{Name: "sqlmap", Version: "1.0.0"},
				{Name: "nmap", Version: "2.0.0"}, // Duplicate name, should be deduplicated
			}, nil
		},
	}

	adapter := NewRegistryAdapter(mockReg)
	ctx := context.Background()

	names, err := adapter.getAvailableToolNames(ctx)

	require.NoError(t, err)
	assert.Len(t, names, 2)
	assert.Contains(t, names, "nmap")
	assert.Contains(t, names, "sqlmap")
}

func TestRegistryAdapter_GetAvailableToolNames_Error(t *testing.T) {
	mockReg := &mockToolRegistry{
		discoverAllFunc: func(ctx context.Context, componentType string) ([]sdkregistry.ServiceInfo, error) {
			return nil, errors.New("registry error")
		},
	}

	adapter := NewRegistryAdapter(mockReg)
	ctx := context.Background()

	names, err := adapter.getAvailableToolNames(ctx)

	assert.Error(t, err)
	assert.Empty(t, names)
}
