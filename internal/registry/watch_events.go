// Package registry provides event-based watching for component registration and deregistration.
//
// This file implements WatchEvents which emits individual events rather than full snapshots,
// enabling more granular monitoring of component lifecycle.
package registry

import (
	"context"

	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// EventType represents the type of registry event.
type EventType string

const (
	// EventTypeRegistered is emitted when a component registers
	EventTypeRegistered EventType = "registered"

	// EventTypeDeregistered is emitted when a component deregisters or its lease expires
	EventTypeDeregistered EventType = "deregistered"

	// EventTypeModified is emitted when a component's metadata changes (e.g., health status update)
	EventTypeModified EventType = "modified"
)

// RegistryEvent represents a change in component registration.
type RegistryEvent struct {
	// Type is the event type (registered, deregistered, modified)
	Type EventType

	// Service is the service that changed
	Service sdkregistry.ServiceInfo

	// Kind is the component kind (agent, tool, plugin)
	Kind string

	// Name is the component name
	Name string

	// Endpoint is the network endpoint
	Endpoint string
}

// WatchEvents returns a channel that emits individual events for registration changes.
//
// Unlike Watch() which sends full snapshots, WatchEvents sends individual events for:
//   - Component registration (EventTypeRegistered)
//   - Component deregistration (EventTypeDeregistered)
//   - Component metadata updates (EventTypeModified)
//
// The channel is closed when the context is canceled or the registry is closed.
//
// Example:
//
//	events, err := reg.WatchEvents(ctx, "agent", "davinci")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for event := range events {
//	    switch event.Type {
//	    case EventTypeRegistered:
//	        log.Printf("Agent registered: %s", event.Endpoint)
//	    case EventTypeDeregistered:
//	        log.Printf("Agent deregistered: %s", event.Endpoint)
//	    }
//	}
//
// Note: The kind and name parameters scope the watch to specific components.
// Pass empty strings to watch all components (not recommended for production).
func WatchEvents(ctx context.Context, reg sdkregistry.Registry, kind, name string) (<-chan RegistryEvent, error) {
	// Get the snapshot-based watch channel
	snapshotCh, err := reg.Watch(ctx, kind, name)
	if err != nil {
		return nil, err
	}

	// Create output channel for events
	eventCh := make(chan RegistryEvent, 10)

	// Start goroutine to convert snapshots to events
	go func() {
		defer close(eventCh)

		// Track previous state to detect changes
		previousState := make(map[string]sdkregistry.ServiceInfo)

		for services := range snapshotCh {
			// Build current state map
			currentState := make(map[string]sdkregistry.ServiceInfo)
			for _, svc := range services {
				currentState[svc.InstanceID] = svc
			}

			// Detect new registrations and modifications
			for instanceID, currentSvc := range currentState {
				if previousSvc, existed := previousState[instanceID]; existed {
					// Check if service was modified (metadata changed)
					if !serviceInfoEqual(previousSvc, currentSvc) {
						event := RegistryEvent{
							Type:     EventTypeModified,
							Service:  currentSvc,
							Kind:     currentSvc.Kind,
							Name:     currentSvc.Name,
							Endpoint: currentSvc.Endpoint,
						}
						select {
						case eventCh <- event:
						case <-ctx.Done():
							return
						}
					}
				} else {
					// New registration
					event := RegistryEvent{
						Type:     EventTypeRegistered,
						Service:  currentSvc,
						Kind:     currentSvc.Kind,
						Name:     currentSvc.Name,
						Endpoint: currentSvc.Endpoint,
					}
					select {
					case eventCh <- event:
					case <-ctx.Done():
						return
					}
				}
			}

			// Detect deregistrations
			for instanceID, previousSvc := range previousState {
				if _, exists := currentState[instanceID]; !exists {
					// Service was deregistered
					event := RegistryEvent{
						Type:     EventTypeDeregistered,
						Service:  previousSvc,
						Kind:     previousSvc.Kind,
						Name:     previousSvc.Name,
						Endpoint: previousSvc.Endpoint,
					}
					select {
					case eventCh <- event:
					case <-ctx.Done():
						return
					}
				}
			}

			// Update previous state for next iteration
			previousState = currentState
		}
	}()

	return eventCh, nil
}

// serviceInfoEqual compares two ServiceInfo instances for equality.
// Returns true if all fields match, false otherwise.
func serviceInfoEqual(a, b sdkregistry.ServiceInfo) bool {
	// Compare basic fields
	if a.Kind != b.Kind || a.Name != b.Name || a.Version != b.Version ||
		a.InstanceID != b.InstanceID || a.Endpoint != b.Endpoint {
		return false
	}

	// Compare metadata
	if len(a.Metadata) != len(b.Metadata) {
		return false
	}
	for key, valueA := range a.Metadata {
		valueB, exists := b.Metadata[key]
		if !exists || valueA != valueB {
			return false
		}
	}

	// StartedAt can be ignored for change detection as it won't change

	return true
}
