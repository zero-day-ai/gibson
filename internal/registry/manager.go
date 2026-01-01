// Package registry provides service discovery and registration infrastructure for Gibson.
//
// This file implements the registry Manager which provides a unified entry point
// for creating and managing either embedded or external registry instances based
// on configuration.
package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/sdk/registry"
)

// Manager handles registry lifecycle for Gibson CLI.
//
// The manager provides a single entry point for initializing, starting, stopping,
// and querying the registry. It automatically selects between embedded and external
// registry implementations based on configuration.
//
// Example usage:
//
//	cfg := config.RegistryConfig{
//	    Type:      "embedded",
//	    DataDir:   "~/.gibson/etcd-data",
//	    Namespace: "gibson",
//	    TTL:       "30s",
//	}
//	mgr := NewManager(cfg)
//	defer mgr.Stop(context.Background())
//
//	if err := mgr.Start(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
//	reg := mgr.Registry()
//	// Use reg for service discovery operations
type Manager struct {
	config    config.RegistryConfig
	registry  registry.Registry
	mu        sync.RWMutex
	started   bool
	startedAt time.Time
}

// NewManager creates a registry manager based on the provided configuration.
//
// This does not start the registry - call Start() to initialize and launch
// the registry. The configuration determines which registry implementation
// will be used:
//
//   - Type="embedded" or Type="" (default): Creates an in-process etcd server
//   - Type="etcd": Connects to an external etcd cluster
//
// The manager is safe for concurrent use after Start() has been called.
func NewManager(cfg config.RegistryConfig) *Manager {
	return &Manager{
		config: cfg,
	}
}

// Start initializes and starts the registry.
//
// This method selects the appropriate registry implementation based on the
// configuration and starts it. For embedded mode, this launches an in-process
// etcd server. For external mode, this establishes a connection to the external
// etcd cluster.
//
// Start() is idempotent - if the registry is already started, this is a no-op.
//
// Returns an error if:
//   - The registry fails to start (embedded etcd startup failure)
//   - Cannot connect to external etcd cluster
//   - Invalid configuration (e.g., Type="etcd" but no endpoints)
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Idempotent check - no-op if already started
	if m.started {
		return nil
	}

	// Convert Gibson config to SDK registry config
	sdkCfg := m.toSDKConfig()

	var reg registry.Registry
	var err error

	// Default to embedded if Type not set or explicitly "embedded"
	registryType := m.config.Type
	if registryType == "" {
		registryType = "embedded"
	}

	// Create appropriate registry implementation
	switch registryType {
	case "embedded":
		reg, err = NewEmbeddedRegistry(sdkCfg)
		if err != nil {
			return fmt.Errorf("failed to create embedded registry: %w", err)
		}

	case "etcd":
		reg, err = NewExternalRegistry(sdkCfg)
		if err != nil {
			return fmt.Errorf("failed to create external registry: %w", err)
		}

	default:
		return fmt.Errorf("unsupported registry type: %s (must be 'embedded' or 'etcd')", registryType)
	}

	m.registry = reg
	m.started = true
	m.startedAt = time.Now()

	return nil
}

// Stop gracefully shuts down the registry.
//
// This method stops the registry and releases all associated resources. For
// embedded mode, this shuts down the in-process etcd server. For external mode,
// this closes the client connection.
//
// Stop() is idempotent - if the registry is not started, this is a no-op.
//
// After Stop() is called, the registry is no longer usable and Registry() will
// return nil. To use the registry again, call Start().
//
// Returns an error if the shutdown fails, though this is typically ignored
// during application shutdown.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Idempotent check - no-op if not started
	if !m.started {
		return nil
	}

	// Close the registry
	var err error
	if m.registry != nil {
		err = m.registry.Close()
		m.registry = nil
	}

	m.started = false

	if err != nil {
		return fmt.Errorf("failed to stop registry: %w", err)
	}

	return nil
}

// Registry returns the active registry for discovery operations.
//
// This method returns the underlying registry implementation that can be used
// to register, deregister, discover, and watch services.
//
// Returns nil if Start() has not been called or if Stop() has been called.
//
// The returned registry is safe for concurrent use.
func (m *Manager) Registry() registry.Registry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.registry
}

// Status returns registry health and connection information.
//
// This provides diagnostic information about the current state of the registry,
// including:
//   - Type: "embedded" or "etcd"
//   - Endpoint: The registry connection endpoint
//   - Healthy: Whether the registry is operational
//   - StartedAt: When the registry was started
//   - Services: Total count of registered services across all kinds
//
// If the registry is not started, Healthy will be false and Services will be 0.
func (m *Manager) Status() RegistryStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := RegistryStatus{
		Type:      m.getEffectiveType(),
		Endpoint:  m.getEndpoint(),
		Healthy:   m.started && m.registry != nil,
		StartedAt: m.startedAt,
		Services:  0,
	}

	// If the registry is running, query for service count
	if status.Healthy {
		// Count all services across all kinds
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Query each kind and sum the counts
		for _, kind := range []string{"agent", "tool", "plugin"} {
			services, err := m.registry.DiscoverAll(ctx, kind)
			if err == nil {
				status.Services += len(services)
			}
		}
	}

	return status
}

// RegistryStatus represents the current state of the registry.
type RegistryStatus struct {
	// Type is the registry mode: "embedded" or "etcd"
	Type string

	// Endpoint is the current registry endpoint (e.g., "localhost:2379")
	Endpoint string

	// Healthy indicates whether the registry is operational
	Healthy bool

	// StartedAt is the timestamp when the registry was started
	// Zero value if not yet started
	StartedAt time.Time

	// Services is the total number of registered service instances
	// across all kinds (agents, tools, plugins)
	Services int
}

// toSDKConfig converts Gibson's RegistryConfig to the SDK's registry.Config.
//
// This handles field mapping and format conversions, such as converting the
// TTL string to an integer.
func (m *Manager) toSDKConfig() registry.Config {
	cfg := registry.Config{
		Type:          m.config.Type,
		Endpoints:     m.config.Endpoints,
		Namespace:     m.config.Namespace,
		DataDir:       m.config.DataDir,
		ListenAddress: m.config.ListenAddress,
	}

	// Parse TTL string to seconds
	if m.config.TTL != "" {
		if duration, err := time.ParseDuration(m.config.TTL); err == nil {
			cfg.TTL = int(duration.Seconds())
		}
	}

	// Convert TLS configuration
	if m.config.TLS.Enabled {
		cfg.TLS = &registry.TLSConfig{
			Enabled:  m.config.TLS.Enabled,
			CertFile: m.config.TLS.CertFile,
			KeyFile:  m.config.TLS.KeyFile,
			CAFile:   m.config.TLS.CAFile,
		}
	}

	return cfg
}

// getEffectiveType returns the registry type, defaulting to "embedded" if not set.
func (m *Manager) getEffectiveType() string {
	if m.config.Type == "" {
		return "embedded"
	}
	return m.config.Type
}

// getEndpoint returns the appropriate endpoint based on registry type.
func (m *Manager) getEndpoint() string {
	registryType := m.getEffectiveType()

	switch registryType {
	case "embedded":
		if m.config.ListenAddress != "" {
			return m.config.ListenAddress
		}
		return "localhost:2379" // default

	case "etcd":
		if len(m.config.Endpoints) > 0 {
			return m.config.Endpoints[0] // return first endpoint
		}
		return "unknown"

	default:
		return "unknown"
	}
}
