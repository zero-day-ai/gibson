package component

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultHealthCheckTimeout is the default timeout for individual health checks
	DefaultHealthCheckTimeout = 5 * time.Second

	// DefaultHealthCheckInterval is the default interval between health checks
	DefaultHealthCheckInterval = 10 * time.Second

	// DefaultHealthCheckRetries is the default number of retries before marking unhealthy
	DefaultHealthCheckRetries = 3
)

// HealthStatus represents the health status of a component.
type HealthStatus string

const (
	// HealthStatusHealthy indicates the component is healthy
	HealthStatusHealthy HealthStatus = "healthy"

	// HealthStatusUnhealthy indicates the component is unhealthy
	HealthStatusUnhealthy HealthStatus = "unhealthy"

	// HealthStatusUnknown indicates the health status is unknown
	HealthStatusUnknown HealthStatus = "unknown"
)

// StatusChangeCallback is called when a component's health status changes.
type StatusChangeCallback func(componentName string, oldStatus, newStatus HealthStatus)

// HealthMonitor monitors the health of components.
type HealthMonitor interface {
	// Start begins the background health monitoring goroutine.
	Start(ctx context.Context) error

	// Stop halts the background health monitoring goroutine.
	Stop() error

	// CheckComponent performs a single health check on a component.
	// Returns nil if the component is healthy, error otherwise.
	CheckComponent(ctx context.Context, healthEndpoint string) error

	// OnStatusChange registers a callback to be invoked when health status changes.
	OnStatusChange(callback StatusChangeCallback)

	// GetHealth returns the current health status of a component.
	GetHealth(componentName string) HealthStatus

	// RegisterComponent adds a component to be monitored.
	RegisterComponent(name string, healthEndpoint string)

	// UnregisterComponent removes a component from monitoring.
	UnregisterComponent(name string)
}

// DefaultHealthMonitor is the default implementation of HealthMonitor.
type DefaultHealthMonitor struct {
	mu                sync.RWMutex
	client            *http.Client
	interval          time.Duration
	timeout           time.Duration
	retries           int
	components        map[string]*monitoredComponent
	callbacks         []StatusChangeCallback
	stopCh            chan struct{}
	stoppedCh         chan struct{}
	running           bool
}

// monitoredComponent represents a component being monitored.
type monitoredComponent struct {
	name           string
	healthEndpoint string
	currentStatus  HealthStatus
	failureCount   int
	lastCheck      time.Time
}

// NewHealthMonitor creates a new DefaultHealthMonitor with default settings.
func NewHealthMonitor() *DefaultHealthMonitor {
	return &DefaultHealthMonitor{
		client: &http.Client{
			Timeout: DefaultHealthCheckTimeout,
		},
		interval:   DefaultHealthCheckInterval,
		timeout:    DefaultHealthCheckTimeout,
		retries:    DefaultHealthCheckRetries,
		components: make(map[string]*monitoredComponent),
		callbacks:  make([]StatusChangeCallback, 0),
		stopCh:     make(chan struct{}),
		stoppedCh:  make(chan struct{}),
		running:    false,
	}
}

// NewHealthMonitorWithConfig creates a new DefaultHealthMonitor with custom configuration.
func NewHealthMonitorWithConfig(interval, timeout time.Duration, retries int) *DefaultHealthMonitor {
	return &DefaultHealthMonitor{
		client: &http.Client{
			Timeout: timeout,
		},
		interval:   interval,
		timeout:    timeout,
		retries:    retries,
		components: make(map[string]*monitoredComponent),
		callbacks:  make([]StatusChangeCallback, 0),
		stopCh:     make(chan struct{}),
		stoppedCh:  make(chan struct{}),
		running:    false,
	}
}

// Start begins the background health monitoring goroutine.
func (m *DefaultHealthMonitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("health monitor is already running")
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.stoppedCh = make(chan struct{})
	m.mu.Unlock()

	go m.monitorLoop(ctx)

	return nil
}

// Stop halts the background health monitoring goroutine.
func (m *DefaultHealthMonitor) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return fmt.Errorf("health monitor is not running")
	}
	m.mu.Unlock()

	close(m.stopCh)

	// Wait for monitor loop to finish
	select {
	case <-m.stoppedCh:
		// Stopped successfully
	case <-time.After(5 * time.Second):
		// Timeout waiting for stop
		return fmt.Errorf("timeout waiting for health monitor to stop")
	}

	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	return nil
}

// CheckComponent performs a single health check on a component.
func (m *DefaultHealthMonitor) CheckComponent(ctx context.Context, healthEndpoint string) error {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthEndpoint, nil)
	if err != nil {
		return NewConnectionFailedError("health-check", err)
	}

	// Perform request
	resp, err := m.client.Do(req)
	if err != nil {
		return NewConnectionFailedError("health-check", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewConnectionFailedError(
			"health-check",
			fmt.Errorf("health check failed with status code: %d", resp.StatusCode),
		)
	}

	return nil
}

// OnStatusChange registers a callback to be invoked when health status changes.
func (m *DefaultHealthMonitor) OnStatusChange(callback StatusChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// GetHealth returns the current health status of a component.
func (m *DefaultHealthMonitor) GetHealth(componentName string) HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	comp, exists := m.components[componentName]
	if !exists {
		return HealthStatusUnknown
	}

	return comp.currentStatus
}

// RegisterComponent adds a component to be monitored.
func (m *DefaultHealthMonitor) RegisterComponent(name string, healthEndpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.components[name] = &monitoredComponent{
		name:           name,
		healthEndpoint: healthEndpoint,
		currentStatus:  HealthStatusUnknown,
		failureCount:   0,
		lastCheck:      time.Time{},
	}
}

// UnregisterComponent removes a component from monitoring.
func (m *DefaultHealthMonitor) UnregisterComponent(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.components, name)
}

// monitorLoop is the main monitoring loop that runs in the background.
func (m *DefaultHealthMonitor) monitorLoop(ctx context.Context) {
	defer close(m.stoppedCh)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAllComponents(ctx)
		}
	}
}

// checkAllComponents performs health checks on all registered components.
func (m *DefaultHealthMonitor) checkAllComponents(ctx context.Context) {
	m.mu.RLock()
	components := make([]*monitoredComponent, 0, len(m.components))
	for _, comp := range m.components {
		components = append(components, comp)
	}
	m.mu.RUnlock()

	for _, comp := range components {
		m.checkComponentHealth(ctx, comp)
	}
}

// checkComponentHealth performs a health check on a single component.
func (m *DefaultHealthMonitor) checkComponentHealth(ctx context.Context, comp *monitoredComponent) {
	// Create timeout context for this check
	checkCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Perform health check
	err := m.CheckComponent(checkCtx, comp.healthEndpoint)

	m.mu.Lock()
	defer m.mu.Unlock()

	oldStatus := comp.currentStatus
	comp.lastCheck = time.Now()

	if err != nil {
		// Health check failed
		comp.failureCount++

		if comp.failureCount >= m.retries {
			// Component is unhealthy after max retries
			if comp.currentStatus != HealthStatusUnhealthy {
				comp.currentStatus = HealthStatusUnhealthy
				m.notifyStatusChange(comp.name, oldStatus, HealthStatusUnhealthy)
			}
		}
	} else {
		// Health check passed
		comp.failureCount = 0

		if comp.currentStatus != HealthStatusHealthy {
			comp.currentStatus = HealthStatusHealthy
			m.notifyStatusChange(comp.name, oldStatus, HealthStatusHealthy)
		}
	}
}

// notifyStatusChange invokes all registered callbacks with the status change.
// Must be called with lock held.
func (m *DefaultHealthMonitor) notifyStatusChange(name string, oldStatus, newStatus HealthStatus) {
	for _, callback := range m.callbacks {
		// Call callback in goroutine to avoid blocking
		go func(cb StatusChangeCallback) {
			cb(name, oldStatus, newStatus)
		}(callback)
	}
}
