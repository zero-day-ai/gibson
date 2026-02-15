package plugin

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/sdk/schema"

	"github.com/zero-day-ai/gibson/internal/events"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestPluginLifecycleEvents tests that plugin lifecycle events are emitted correctly
func TestPluginLifecycleEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("Initialized Event", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		// Subscribe to plugin.initialized events
		eventsChan, cleanup := eventBus.Subscribe(ctx, events.Filter{
			Types: []events.EventType{events.EventPluginInitialized},
		}, 10)
		defer cleanup()

		// Create registry with event bus
		registry := NewPluginRegistry(eventBus)
		plugin := NewMockPlugin("test-plugin", "1.0.0")
		plugin.AddMethod("method1", "Test method", schema.JSON{}, schema.JSON{})

		// Register plugin
		err := registry.Register(plugin, PluginConfig{})
		require.NoError(t, err)

		// Wait for event
		select {
		case event := <-eventsChan:
			assert.Equal(t, events.EventPluginInitialized, event.Type)

			payload, ok := event.Payload.(events.PluginInitializedPayload)
			require.True(t, ok, "Expected PluginInitializedPayload")

			assert.Equal(t, "test-plugin", payload.PluginName)
			assert.Equal(t, "1.0.0", payload.Version)
			assert.Contains(t, payload.MethodsAvailable, "method1")
			assert.Greater(t, payload.InitializationDuration, time.Duration(0))
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for plugin.initialized event")
		}
	})

	t.Run("Initialization Failed Event", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		// Subscribe to plugin.initialization_failed events
		eventsChan, cleanup := eventBus.Subscribe(ctx, events.Filter{
			Types: []events.EventType{events.EventPluginInitializationFailed},
		}, 10)
		defer cleanup()

		// Create registry with event bus
		registry := NewPluginRegistry(eventBus)
		plugin := NewMockPlugin("failing-plugin", "1.0.0")
		plugin.SetInitError(assert.AnError)

		// Register plugin (should fail)
		err := registry.Register(plugin, PluginConfig{})
		require.Error(t, err)

		// Wait for event
		select {
		case event := <-eventsChan:
			assert.Equal(t, events.EventPluginInitializationFailed, event.Type)

			payload, ok := event.Payload.(events.PluginInitializationFailedPayload)
			require.True(t, ok, "Expected PluginInitializationFailedPayload")

			assert.Equal(t, "failing-plugin", payload.PluginName)
			assert.Contains(t, payload.Error, "assert.AnError")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for plugin.initialization_failed event")
		}
	})

	t.Run("Shutdown Event", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		// Subscribe to plugin.shutdown events
		eventsChan, cleanup := eventBus.Subscribe(ctx, events.Filter{
			Types: []events.EventType{events.EventPluginShutdown},
		}, 10)
		defer cleanup()

		// Create registry with event bus
		registry := NewPluginRegistry(eventBus)
		plugin := NewMockPlugin("test-plugin", "1.0.0")

		// Register and then unregister plugin
		err := registry.Register(plugin, PluginConfig{})
		require.NoError(t, err)

		// Make some queries to track count
		_, _ = registry.Query(ctx, "test-plugin", "search", nil)
		_, _ = registry.Query(ctx, "test-plugin", "search", nil)

		// Unregister plugin
		err = registry.Unregister("test-plugin")
		require.NoError(t, err)

		// Wait for event
		select {
		case event := <-eventsChan:
			assert.Equal(t, events.EventPluginShutdown, event.Type)

			payload, ok := event.Payload.(events.PluginShutdownPayload)
			require.True(t, ok, "Expected PluginShutdownPayload")

			assert.Equal(t, "test-plugin", payload.PluginName)
			assert.Equal(t, "manual_unregister", payload.ShutdownReason)
			assert.Equal(t, int64(2), payload.QueriesServed)
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for plugin.shutdown event")
		}
	})

	t.Run("Shutdown Event On Registry Shutdown", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		// Subscribe to plugin.shutdown events
		eventsChan, cleanup := eventBus.Subscribe(ctx, events.Filter{
			Types: []events.EventType{events.EventPluginShutdown},
		}, 10)
		defer cleanup()

		// Create registry with event bus
		registry := NewPluginRegistry(eventBus)
		plugin1 := NewMockPlugin("plugin1", "1.0.0")
		plugin2 := NewMockPlugin("plugin2", "1.0.0")

		// Register plugins
		err := registry.Register(plugin1, PluginConfig{})
		require.NoError(t, err)
		err = registry.Register(plugin2, PluginConfig{})
		require.NoError(t, err)

		// Shutdown registry
		err = registry.Shutdown(ctx)
		require.NoError(t, err)

		// Wait for shutdown events for both plugins
		shutdownEvents := make(map[string]events.PluginShutdownPayload)
		timeout := time.After(2 * time.Second)

		for i := 0; i < 2; i++ {
			select {
			case event := <-eventsChan:
				payload, ok := event.Payload.(events.PluginShutdownPayload)
				require.True(t, ok, "Expected PluginShutdownPayload")
				shutdownEvents[payload.PluginName] = payload
			case <-timeout:
				t.Fatalf("Timeout waiting for shutdown event %d/2", i+1)
			}
		}

		// Verify both plugins emitted shutdown events
		assert.Contains(t, shutdownEvents, "plugin1")
		assert.Contains(t, shutdownEvents, "plugin2")
		assert.Equal(t, "registry_shutdown", shutdownEvents["plugin1"].ShutdownReason)
		assert.Equal(t, "registry_shutdown", shutdownEvents["plugin2"].ShutdownReason)
	})

	t.Run("Health Changed Event", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		// Subscribe to plugin.health_changed events
		eventsChan, cleanup := eventBus.Subscribe(ctx, events.Filter{
			Types: []events.EventType{events.EventPluginHealthChanged},
		}, 10)
		defer cleanup()

		// Create registry with event bus
		registry := NewPluginRegistry(eventBus)
		plugin := NewMockPlugin("test-plugin", "1.0.0")
		plugin.SetHealthStatus(types.Healthy("all good"))

		// Register plugin
		err := registry.Register(plugin, PluginConfig{})
		require.NoError(t, err)

		// First health check - establishes baseline
		_ = registry.Health(ctx)

		// Change health status
		plugin.SetHealthStatus(types.Degraded("experiencing issues"))

		// Second health check - should detect change
		_ = registry.Health(ctx)

		// Wait for health changed event
		select {
		case event := <-eventsChan:
			assert.Equal(t, events.EventPluginHealthChanged, event.Type)

			payload, ok := event.Payload.(events.PluginHealthChangedPayload)
			require.True(t, ok, "Expected PluginHealthChangedPayload")

			assert.Equal(t, "test-plugin", payload.PluginName)
			assert.Equal(t, string(types.HealthStateHealthy), payload.PreviousStatus)
			assert.Equal(t, string(types.HealthStateDegraded), payload.CurrentStatus)
			assert.Contains(t, payload.HealthDetails, "experiencing issues")
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for plugin.health_changed event")
		}
	})

	t.Run("Query Count Tracking", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		registry := NewPluginRegistry(eventBus)
		plugin := NewMockPlugin("test-plugin", "1.0.0")
		plugin.SetQueryResult("search", "result")

		err := registry.Register(plugin, PluginConfig{})
		require.NoError(t, err)

		// Make multiple queries
		for i := 0; i < 5; i++ {
			_, _ = registry.Query(ctx, "test-plugin", "search", nil)
		}

		// Verify query count is tracked
		entry := registry.plugins["test-plugin"]
		assert.Equal(t, int64(5), entry.queryCount.Load())
	})

	t.Run("No Events When EventBus Is Nil", func(t *testing.T) {
		// Create registry without event bus
		registry := NewPluginRegistry(nil)
		plugin := NewMockPlugin("test-plugin", "1.0.0")

		// Register plugin - should not panic
		err := registry.Register(plugin, PluginConfig{})
		require.NoError(t, err)

		// Unregister plugin - should not panic
		err = registry.Unregister("test-plugin")
		require.NoError(t, err)
	})

	t.Run("Concurrent Event Emissions", func(t *testing.T) {
		eventBus := events.NewEventBus()
		defer eventBus.Close()

		// Subscribe to all plugin events
		eventsChan, cleanup := eventBus.Subscribe(ctx, events.Filter{
			Types: []events.EventType{
				events.EventPluginInitialized,
				events.EventPluginShutdown,
			},
		}, 100)
		defer cleanup()

		registry := NewPluginRegistry(eventBus)

		// Register multiple plugins concurrently
		var wg sync.WaitGroup
		pluginCount := 10

		for i := 0; i < pluginCount; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				plugin := NewMockPlugin("plugin-"+string(rune('0'+idx)), "1.0.0")
				_ = registry.Register(plugin, PluginConfig{})
			}(i)
		}

		wg.Wait()

		// Shutdown registry
		_ = registry.Shutdown(ctx)

		// Count events
		initCount := 0
		shutdownCount := 0
		timeout := time.After(3 * time.Second)

	eventLoop:
		for {
			select {
			case event := <-eventsChan:
				switch event.Type {
				case events.EventPluginInitialized:
					initCount++
				case events.EventPluginShutdown:
					shutdownCount++
				}
				if initCount >= pluginCount && shutdownCount >= pluginCount {
					break eventLoop
				}
			case <-timeout:
				break eventLoop
			}
		}

		// Verify we got events for all plugins
		assert.GreaterOrEqual(t, initCount, pluginCount, "Should have initialization events for all plugins")
		assert.GreaterOrEqual(t, shutdownCount, pluginCount, "Should have shutdown events for all plugins")
	})
}
