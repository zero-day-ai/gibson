package plugin

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zero-day-ai/gibson/internal/events"
	"github.com/zero-day-ai/gibson/internal/types"
)

// PluginRegistry manages plugin lifecycle and queries
type PluginRegistry interface {
	// Register registers and initializes a native Go plugin
	Register(plugin Plugin, cfg PluginConfig) error

	// RegisterExternal registers and initializes an external gRPC plugin
	RegisterExternal(name string, client ExternalPluginClient, cfg PluginConfig) error

	// Unregister shuts down and removes a plugin from the registry
	Unregister(name string) error

	// Get retrieves a registered plugin by name
	Get(name string) (Plugin, error)

	// List returns descriptors for all registered plugins
	List() []PluginDescriptor

	// Methods returns the method descriptors for a specific plugin
	Methods(pluginName string) ([]MethodDescriptor, error)

	// Query executes a method on a plugin with the given parameters
	Query(ctx context.Context, pluginName, method string, params map[string]any) (any, error)

	// Shutdown gracefully shuts down all registered plugins in reverse registration order
	Shutdown(ctx context.Context) error

	// Health returns the aggregate health status of all plugins
	Health(ctx context.Context) types.HealthStatus
}

// ExternalPluginClient interface for gRPC plugin clients
type ExternalPluginClient interface {
	Plugin
}

// pluginEntry tracks a registered plugin
type pluginEntry struct {
	plugin       Plugin
	config       PluginConfig
	status       PluginStatus
	external     bool
	queryCount   atomic.Int64
	lastHealth   types.HealthStatus
	healthMu     sync.RWMutex
	initDuration time.Duration
}

// DefaultPluginRegistry implements PluginRegistry
type DefaultPluginRegistry struct {
	mu       sync.RWMutex
	plugins  map[string]*pluginEntry
	order    []string // Track registration order for shutdown
	eventBus events.EventBus
}

// NewPluginRegistry creates a new DefaultPluginRegistry
func NewPluginRegistry(eventBus events.EventBus) *DefaultPluginRegistry {
	return &DefaultPluginRegistry{
		plugins:  make(map[string]*pluginEntry),
		order:    make([]string, 0),
		eventBus: eventBus,
	}
}

// Register registers and initializes a native Go plugin
func (r *DefaultPluginRegistry) Register(plugin Plugin, cfg PluginConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Name()
	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	// Create entry with initializing status
	// Initialize lastHealth with current plugin health to avoid spurious event on first health check
	initialHealth := plugin.Health(context.Background())
	entry := &pluginEntry{
		plugin:     plugin,
		config:     cfg,
		status:     PluginStatusInitializing,
		external:   false,
		lastHealth: initialHealth,
	}
	r.plugins[name] = entry
	r.order = append(r.order, name)

	// Initialize the plugin (unlock during initialization to allow concurrent queries)
	r.mu.Unlock()
	ctx := context.Background()

	// Track initialization duration
	startTime := time.Now()
	err := plugin.Initialize(ctx, cfg)
	initDuration := time.Since(startTime)

	r.mu.Lock()

	if err != nil {
		entry.status = PluginStatusError

		// Emit plugin.initialization_failed event
		if r.eventBus != nil {
			r.mu.Unlock()
			_ = r.eventBus.Publish(ctx, events.Event{
				Type:      events.EventPluginInitializationFailed,
				Timestamp: time.Now(),
				Payload: events.PluginInitializationFailedPayload{
					PluginName: name,
					Error:      err.Error(),
				},
			})
			r.mu.Lock()
		}

		return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
	}

	entry.status = PluginStatusRunning
	entry.initDuration = initDuration

	// Get method names for the event
	methods := plugin.Methods()
	methodNames := make([]string, len(methods))
	for i, m := range methods {
		methodNames[i] = m.Name
	}

	// Emit plugin.initialized event
	if r.eventBus != nil {
		r.mu.Unlock()
		_ = r.eventBus.Publish(ctx, events.Event{
			Type:      events.EventPluginInitialized,
			Timestamp: time.Now(),
			Payload: events.PluginInitializedPayload{
				PluginName:             name,
				Version:                plugin.Version(),
				MethodsAvailable:       methodNames,
				InitializationDuration: initDuration,
			},
		})
		r.mu.Lock()
	}

	return nil
}

// RegisterExternal registers and initializes an external gRPC plugin
func (r *DefaultPluginRegistry) RegisterExternal(name string, client ExternalPluginClient, cfg PluginConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	// Create entry with initializing status
	// Initialize lastHealth with current plugin health to avoid spurious event on first health check
	initialHealth := client.Health(context.Background())
	entry := &pluginEntry{
		plugin:     client,
		config:     cfg,
		status:     PluginStatusInitializing,
		external:   true,
		lastHealth: initialHealth,
	}
	r.plugins[name] = entry
	r.order = append(r.order, name)

	// Initialize the plugin (unlock during initialization to allow concurrent queries)
	r.mu.Unlock()
	ctx := context.Background()

	// Track initialization duration
	startTime := time.Now()
	err := client.Initialize(ctx, cfg)
	initDuration := time.Since(startTime)

	r.mu.Lock()

	if err != nil {
		entry.status = PluginStatusError

		// Emit plugin.initialization_failed event
		if r.eventBus != nil {
			r.mu.Unlock()
			_ = r.eventBus.Publish(ctx, events.Event{
				Type:      events.EventPluginInitializationFailed,
				Timestamp: time.Now(),
				Payload: events.PluginInitializationFailedPayload{
					PluginName: name,
					Error:      err.Error(),
				},
			})
			r.mu.Lock()
		}

		return fmt.Errorf("failed to initialize external plugin %s: %w", name, err)
	}

	entry.status = PluginStatusRunning
	entry.initDuration = initDuration

	// Get method names for the event
	methods := client.Methods()
	methodNames := make([]string, len(methods))
	for i, m := range methods {
		methodNames[i] = m.Name
	}

	// Emit plugin.initialized event
	if r.eventBus != nil {
		r.mu.Unlock()
		_ = r.eventBus.Publish(ctx, events.Event{
			Type:      events.EventPluginInitialized,
			Timestamp: time.Now(),
			Payload: events.PluginInitializedPayload{
				PluginName:             name,
				Version:                client.Version(),
				MethodsAvailable:       methodNames,
				InitializationDuration: initDuration,
			},
		})
		r.mu.Lock()
	}

	return nil
}

// Unregister shuts down and removes a plugin from the registry
func (r *DefaultPluginRegistry) Unregister(name string) error {
	r.mu.Lock()
	entry, exists := r.plugins[name]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("plugin %s not found", name)
	}

	entry.status = PluginStatusStopping
	plugin := entry.plugin
	queryCount := entry.queryCount.Load()
	r.mu.Unlock()

	// Shutdown the plugin (unlock during shutdown to avoid deadlock)
	ctx := context.Background()
	if err := plugin.Shutdown(ctx); err != nil {
		r.mu.Lock()
		entry.status = PluginStatusError
		r.mu.Unlock()
		return fmt.Errorf("failed to shutdown plugin %s: %w", name, err)
	}

	// Emit plugin.shutdown event
	if r.eventBus != nil {
		_ = r.eventBus.Publish(ctx, events.Event{
			Type:      events.EventPluginShutdown,
			Timestamp: time.Now(),
			Payload: events.PluginShutdownPayload{
				PluginName:     name,
				ShutdownReason: "manual_unregister",
				QueriesServed:  queryCount,
			},
		})
	}

	// Remove from registry
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.plugins, name)

	// Remove from order slice
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}

	return nil
}

// Get retrieves a registered plugin by name
func (r *DefaultPluginRegistry) Get(name string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	if entry.status != PluginStatusRunning {
		return nil, fmt.Errorf("plugin %s is not running (status: %s)", name, entry.status)
	}

	return entry.plugin, nil
}

// List returns descriptors for all registered plugins
func (r *DefaultPluginRegistry) List() []PluginDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	descriptors := make([]PluginDescriptor, 0, len(r.plugins))
	for name, entry := range r.plugins {
		descriptors = append(descriptors, PluginDescriptor{
			Name:       name,
			Version:    entry.plugin.Version(),
			Methods:    entry.plugin.Methods(),
			IsExternal: entry.external,
			Status:     entry.status,
		})
	}

	return descriptors
}

// Methods returns the method descriptors for a specific plugin
func (r *DefaultPluginRegistry) Methods(pluginName string) ([]MethodDescriptor, error) {
	plugin, err := r.Get(pluginName)
	if err != nil {
		return nil, err
	}

	return plugin.Methods(), nil
}

// Query executes a method on a plugin with the given parameters
func (r *DefaultPluginRegistry) Query(ctx context.Context, pluginName, method string, params map[string]any) (any, error) {
	plugin, err := r.Get(pluginName)
	if err != nil {
		return nil, err
	}

	// Track query count
	r.mu.RLock()
	entry, exists := r.plugins[pluginName]
	r.mu.RUnlock()
	if exists {
		entry.queryCount.Add(1)
	}

	return plugin.Query(ctx, method, params)
}

// Shutdown gracefully shuts down all registered plugins in reverse registration order
func (r *DefaultPluginRegistry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Shutdown in reverse order
	var errors []error
	for i := len(r.order) - 1; i >= 0; i-- {
		name := r.order[i]
		entry, exists := r.plugins[name]
		if !exists {
			continue
		}

		entry.status = PluginStatusStopping
		queryCount := entry.queryCount.Load()

		// Unlock during shutdown to avoid deadlock
		r.mu.Unlock()
		if err := entry.plugin.Shutdown(ctx); err != nil {
			errors = append(errors, fmt.Errorf("plugin %s: %w", name, err))
		}

		// Emit plugin.shutdown event
		if r.eventBus != nil {
			_ = r.eventBus.Publish(ctx, events.Event{
				Type:      events.EventPluginShutdown,
				Timestamp: time.Now(),
				Payload: events.PluginShutdownPayload{
					PluginName:     name,
					ShutdownReason: "registry_shutdown",
					QueriesServed:  queryCount,
				},
			})
		}

		r.mu.Lock()

		entry.status = PluginStatusStopped
	}

	// Clear the registry
	r.plugins = make(map[string]*pluginEntry)
	r.order = make([]string, 0)

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// Health returns the aggregate health status of all plugins
func (r *DefaultPluginRegistry) Health(ctx context.Context) types.HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.plugins) == 0 {
		return types.Healthy("no plugins registered")
	}

	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for name, entry := range r.plugins {
		// Skip plugins not in running state
		if entry.status != PluginStatusRunning {
			degradedCount++
			continue
		}

		// Check plugin health (unlock during health check to avoid deadlock)
		r.mu.RUnlock()
		health := entry.plugin.Health(ctx)
		r.mu.RLock()

		// Track health changes
		entry.healthMu.RLock()
		previousHealth := entry.lastHealth
		entry.healthMu.RUnlock()

		if previousHealth.State != health.State {
			entry.healthMu.Lock()
			entry.lastHealth = health
			entry.healthMu.Unlock()

			// Emit plugin.health_changed event
			if r.eventBus != nil {
				r.mu.RUnlock()
				_ = r.eventBus.Publish(ctx, events.Event{
					Type:      events.EventPluginHealthChanged,
					Timestamp: time.Now(),
					Payload: events.PluginHealthChangedPayload{
						PluginName:     name,
						PreviousStatus: string(previousHealth.State),
						CurrentStatus:  string(health.State),
						HealthDetails:  health.Message,
					},
				})
				r.mu.RLock()
			}
		}

		switch health.State {
		case types.HealthStateHealthy:
			healthyCount++
		case types.HealthStateDegraded:
			degradedCount++
		case types.HealthStateUnhealthy:
			unhealthyCount++
		}

		// If any plugin is unhealthy, return unhealthy immediately
		if health.State == types.HealthStateUnhealthy {
			return types.Unhealthy(fmt.Sprintf("plugin %s is unhealthy: %s", name, health.Message))
		}
	}

	total := len(r.plugins)

	// If any plugin is degraded, overall status is degraded
	if degradedCount > 0 {
		return types.Degraded(fmt.Sprintf("%d/%d plugins degraded", degradedCount, total))
	}

	// All plugins are healthy
	return types.Healthy(fmt.Sprintf("%d/%d plugins healthy", healthyCount, total))
}
