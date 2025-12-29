package component

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ComponentRegistry defines the interface for managing components.
// It provides thread-safe operations for registering, retrieving, and persisting components.
type ComponentRegistry interface {
	// Register adds a component to the registry.
	// Returns an error if the component already exists or validation fails.
	Register(component *Component) error

	// Unregister removes a component from the registry.
	// Returns an error if the component doesn't exist.
	Unregister(kind ComponentKind, name string) error

	// Get retrieves a component by kind and name.
	// Returns nil if the component doesn't exist.
	Get(kind ComponentKind, name string) *Component

	// List returns all components of the specified kind.
	// Returns an empty slice if no components of that kind exist.
	List(kind ComponentKind) []*Component

	// ListAll returns all components in the registry, grouped by kind.
	// Returns a map where keys are ComponentKind and values are slices of components.
	ListAll() map[ComponentKind][]*Component

	// LoadFromConfig loads components from a configuration file.
	// Returns an error if the file doesn't exist or is invalid.
	LoadFromConfig(path string) error

	// Save persists the registry to the default location (~/.gibson/registry.yaml).
	// Creates the directory if it doesn't exist.
	// Returns an error if writing fails.
	Save() error
}

// DefaultComponentRegistry is the default implementation of ComponentRegistry.
// It uses a sync.RWMutex for thread-safe concurrent access to the component store.
type DefaultComponentRegistry struct {
	mu         sync.RWMutex                        // Protects concurrent access to components
	components map[ComponentKind]map[string]*Component // Nested map: kind -> name -> component
}

// NewDefaultComponentRegistry creates a new DefaultComponentRegistry instance.
// The registry is initialized with empty component maps for each kind.
func NewDefaultComponentRegistry() *DefaultComponentRegistry {
	return &DefaultComponentRegistry{
		components: make(map[ComponentKind]map[string]*Component),
	}
}

// Register adds a component to the registry.
// It validates the component before adding and returns an error if:
// - The component validation fails
// - A component with the same kind and name already exists
// Thread-safe: Uses write lock to prevent concurrent modifications.
func (r *DefaultComponentRegistry) Register(component *Component) error {
	if component == nil {
		return NewComponentError(ErrCodeValidationFailed, "component cannot be nil")
	}

	// Validate component before registering
	if err := component.Validate(); err != nil {
		return NewValidationFailedError("component validation failed", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize kind map if it doesn't exist
	if r.components[component.Kind] == nil {
		r.components[component.Kind] = make(map[string]*Component)
	}

	// Check if component already exists
	if _, exists := r.components[component.Kind][component.Name]; exists {
		return NewComponentExistsError(component.Name)
	}

	// Set timestamps if not already set
	if component.CreatedAt.IsZero() {
		component.CreatedAt = time.Now()
	}
	if component.UpdatedAt.IsZero() {
		component.UpdatedAt = time.Now()
	}

	// Store the component
	r.components[component.Kind][component.Name] = component

	return nil
}

// Unregister removes a component from the registry.
// Returns an error if the component doesn't exist.
// Thread-safe: Uses write lock to prevent concurrent modifications.
func (r *DefaultComponentRegistry) Unregister(kind ComponentKind, name string) error {
	if !kind.IsValid() {
		return NewInvalidKindError(string(kind))
	}

	if name == "" {
		return NewComponentError(ErrCodeValidationFailed, "component name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if kind exists
	kindMap, exists := r.components[kind]
	if !exists || kindMap == nil {
		return NewComponentNotFoundError(name)
	}

	// Check if component exists
	if _, exists := kindMap[name]; !exists {
		return NewComponentNotFoundError(name)
	}

	// Remove the component
	delete(kindMap, name)

	// Clean up empty kind map
	if len(kindMap) == 0 {
		delete(r.components, kind)
	}

	return nil
}

// Get retrieves a component by kind and name.
// Returns nil if the component doesn't exist.
// Thread-safe: Uses read lock to allow concurrent reads.
func (r *DefaultComponentRegistry) Get(kind ComponentKind, name string) *Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kindMap, exists := r.components[kind]
	if !exists || kindMap == nil {
		return nil
	}

	// Return a copy to prevent external modifications
	component, exists := kindMap[name]
	if !exists {
		return nil
	}

	// Return the component pointer directly (caller should treat as read-only)
	return component
}

// List returns all components of the specified kind.
// Returns an empty slice if no components of that kind exist.
// Thread-safe: Uses read lock to allow concurrent reads.
func (r *DefaultComponentRegistry) List(kind ComponentKind) []*Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kindMap, exists := r.components[kind]
	if !exists || kindMap == nil {
		return []*Component{}
	}

	// Create a slice with capacity for efficiency
	components := make([]*Component, 0, len(kindMap))
	for _, component := range kindMap {
		components = append(components, component)
	}

	return components
}

// ListAll returns all components in the registry, grouped by kind.
// Returns a map where keys are ComponentKind and values are slices of components.
// Thread-safe: Uses read lock to allow concurrent reads.
func (r *DefaultComponentRegistry) ListAll() map[ComponentKind][]*Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[ComponentKind][]*Component)

	for kind, kindMap := range r.components {
		components := make([]*Component, 0, len(kindMap))
		for _, component := range kindMap {
			components = append(components, component)
		}
		result[kind] = components
	}

	return result
}

// LoadFromConfig loads components from a configuration file.
// The file must be in YAML format with a structure like:
//
//	agents:
//	  - kind: agent
//	    name: scanner
//	    ...
//	tools:
//	  - kind: tool
//	    name: nmap
//	    ...
//
// Returns an error if:
// - The file doesn't exist or can't be read
// - The YAML is invalid
// - Component validation fails
// Thread-safe: Uses write lock to prevent concurrent modifications.
func (r *DefaultComponentRegistry) LoadFromConfig(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return WrapComponentError(ErrCodeLoadFailed, fmt.Sprintf("config file not found: %s", path), err)
	}

	// Read file contents
	data, err := os.ReadFile(path)
	if err != nil {
		return WrapComponentError(ErrCodeLoadFailed, "failed to read config file", err)
	}

	// Parse YAML into a map structure
	var config struct {
		Agents  []*Component `yaml:"agents,omitempty"`
		Tools   []*Component `yaml:"tools,omitempty"`
		Plugins []*Component `yaml:"plugins,omitempty"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return WrapComponentError(ErrCodeLoadFailed, "failed to parse YAML config", err)
	}

	// Register components from each category
	var components []*Component
	components = append(components, config.Agents...)
	components = append(components, config.Tools...)
	components = append(components, config.Plugins...)

	// Register each component
	for _, component := range components {
		if component == nil {
			continue
		}

		if err := r.Register(component); err != nil {
			// If component already exists, skip it (allow idempotent loading)
			var compErr *ComponentError
			if err == compErr && compErr.Code == ErrCodeComponentExists {
				continue
			}
			return WrapComponentError(ErrCodeLoadFailed,
				fmt.Sprintf("failed to register component %s", component.Name), err)
		}
	}

	return nil
}

// Save persists the registry to the default location (~/.gibson/registry.yaml).
// The file structure groups components by kind:
//
//	agents:
//	  - kind: agent
//	    name: scanner
//	    ...
//	tools:
//	  - kind: tool
//	    name: nmap
//	    ...
//
// Creates the directory if it doesn't exist.
// Returns an error if:
// - The directory can't be created
// - YAML marshaling fails
// - Writing to file fails
// Thread-safe: Uses read lock to allow concurrent reads during save.
func (r *DefaultComponentRegistry) Save() error {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return WrapComponentError(ErrCodeLoadFailed, "failed to get home directory", err)
	}

	// Create .gibson directory if it doesn't exist
	gibsonDir := filepath.Join(homeDir, ".gibson")
	if err := os.MkdirAll(gibsonDir, 0755); err != nil {
		return WrapComponentError(ErrCodeLoadFailed, "failed to create .gibson directory", err)
	}

	// Build config structure
	r.mu.RLock()
	config := struct {
		Agents  []*Component `yaml:"agents,omitempty"`
		Tools   []*Component `yaml:"tools,omitempty"`
		Plugins []*Component `yaml:"plugins,omitempty"`
	}{
		Agents:  r.getComponentsForKind(ComponentKindAgent),
		Tools:   r.getComponentsForKind(ComponentKindTool),
		Plugins: r.getComponentsForKind(ComponentKindPlugin),
	}
	r.mu.RUnlock()

	// Marshal to YAML
	data, err := yaml.Marshal(&config)
	if err != nil {
		return WrapComponentError(ErrCodeLoadFailed, "failed to marshal registry to YAML", err)
	}

	// Write to file
	registryPath := filepath.Join(gibsonDir, "registry.yaml")
	if err := os.WriteFile(registryPath, data, 0644); err != nil {
		return WrapComponentError(ErrCodeLoadFailed, "failed to write registry file", err)
	}

	return nil
}

// getComponentsForKind is an internal helper that returns components for a specific kind.
// This method is NOT thread-safe and must be called while holding a lock.
func (r *DefaultComponentRegistry) getComponentsForKind(kind ComponentKind) []*Component {
	kindMap, exists := r.components[kind]
	if !exists || kindMap == nil {
		return nil
	}

	components := make([]*Component, 0, len(kindMap))
	for _, component := range kindMap {
		components = append(components, component)
	}

	return components
}

// Count returns the total number of components in the registry.
// Thread-safe: Uses read lock to allow concurrent reads.
func (r *DefaultComponentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, kindMap := range r.components {
		count += len(kindMap)
	}

	return count
}

// CountByKind returns the number of components for a specific kind.
// Thread-safe: Uses read lock to allow concurrent reads.
func (r *DefaultComponentRegistry) CountByKind(kind ComponentKind) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kindMap, exists := r.components[kind]
	if !exists || kindMap == nil {
		return 0
	}

	return len(kindMap)
}

// Clear removes all components from the registry.
// Thread-safe: Uses write lock to prevent concurrent modifications.
func (r *DefaultComponentRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.components = make(map[ComponentKind]map[string]*Component)
}

// Update updates an existing component in the registry.
// Returns an error if the component doesn't exist.
// Thread-safe: Uses write lock to prevent concurrent modifications.
func (r *DefaultComponentRegistry) Update(component *Component) error {
	if component == nil {
		return NewComponentError(ErrCodeValidationFailed, "component cannot be nil")
	}

	// Validate component before updating
	if err := component.Validate(); err != nil {
		return NewValidationFailedError("component validation failed", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if kind exists
	kindMap, exists := r.components[component.Kind]
	if !exists || kindMap == nil {
		return NewComponentNotFoundError(component.Name)
	}

	// Check if component exists
	if _, exists := kindMap[component.Name]; !exists {
		return NewComponentNotFoundError(component.Name)
	}

	// Update timestamp
	component.UpdatedAt = time.Now()

	// Update the component
	kindMap[component.Name] = component

	return nil
}
