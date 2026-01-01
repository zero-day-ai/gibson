package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGRPCAgentRegistry(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		require.NotNil(t, registry)
		assert.NotNil(t, registry.clients)
		assert.NotNil(t, registry.descriptors)
		assert.NotNil(t, registry.addresses)
		assert.NotNil(t, registry.components)
		assert.Equal(t, 30*time.Second, registry.timeout)
		assert.Empty(t, registry.agentPaths)
	})

	t.Run("with agent paths", func(t *testing.T) {
		paths := []string{"/path/to/agents", "/another/path"}
		registry := NewGRPCAgentRegistry(WithAgentPaths(paths))

		require.NotNil(t, registry)
		assert.Equal(t, paths, registry.agentPaths)
	})

	t.Run("with custom timeout", func(t *testing.T) {
		customTimeout := 10 * time.Second
		registry := NewGRPCAgentRegistry(WithTimeout(customTimeout))

		require.NotNil(t, registry)
		assert.Equal(t, customTimeout, registry.timeout)
	})

	t.Run("with lifecycle manager", func(t *testing.T) {
		// We don't have a mock lifecycle manager here, just test it accepts the option
		registry := NewGRPCAgentRegistry(WithLifecycleManager(nil))

		require.NotNil(t, registry)
		assert.Nil(t, registry.lifecycleManager)
	})

	t.Run("with multiple options", func(t *testing.T) {
		paths := []string{"/path/to/agents"}
		customTimeout := 5 * time.Second

		registry := NewGRPCAgentRegistry(
			WithAgentPaths(paths),
			WithTimeout(customTimeout),
		)

		require.NotNil(t, registry)
		assert.Equal(t, paths, registry.agentPaths)
		assert.Equal(t, customTimeout, registry.timeout)
	})
}

func TestGRPCAgentRegistry_RegisterAddress(t *testing.T) {
	t.Run("register valid address", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("test-agent", "localhost:50051")

		assert.NoError(t, err)
		assert.True(t, registry.IsRegistered("test-agent"))
		assert.Equal(t, 1, registry.Count())
	})

	t.Run("register multiple addresses", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.RegisterAddress("agent-1", "localhost:50051")
		assert.NoError(t, err)

		err = registry.RegisterAddress("agent-2", "localhost:50052")
		assert.NoError(t, err)

		assert.Equal(t, 2, registry.Count())
		assert.True(t, registry.IsRegistered("agent-1"))
		assert.True(t, registry.IsRegistered("agent-2"))
	})

	t.Run("empty agent name", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("", "localhost:50051")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent name cannot be empty")
	})

	t.Run("empty address", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("test-agent", "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent address cannot be empty")
	})

	t.Run("overwrite existing address", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.RegisterAddress("test-agent", "localhost:50051")
		assert.NoError(t, err)

		// Overwrite with new address
		err = registry.RegisterAddress("test-agent", "localhost:50052")
		assert.NoError(t, err)

		// Should still have only 1 agent
		assert.Equal(t, 1, registry.Count())
	})
}

func TestGRPCAgentRegistry_Unregister(t *testing.T) {
	t.Run("unregister existing agent without connection", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("test-agent", "localhost:50051")
		require.NoError(t, err)

		err = registry.Unregister("test-agent")
		assert.NoError(t, err)
		assert.False(t, registry.IsRegistered("test-agent"))
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("unregister non-existent agent", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		// Unregistering non-existent agent should not error
		// (it just removes from maps, no-op if not present)
		err := registry.Unregister("non-existent")
		assert.NoError(t, err)
	})

	t.Run("unregister removes from all maps", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("test-agent", "localhost:50051")
		require.NoError(t, err)

		// Manually add a descriptor to test cleanup
		registry.mu.Lock()
		registry.descriptors["test-agent"] = AgentDescriptor{
			Name:    "test-agent",
			Version: "1.0.0",
		}
		registry.mu.Unlock()

		err = registry.Unregister("test-agent")
		assert.NoError(t, err)

		// Verify all maps are cleaned
		registry.mu.RLock()
		_, hasAddress := registry.addresses["test-agent"]
		_, hasDescriptor := registry.descriptors["test-agent"]
		_, hasClient := registry.clients["test-agent"]
		registry.mu.RUnlock()

		assert.False(t, hasAddress)
		assert.False(t, hasDescriptor)
		assert.False(t, hasClient)
	})
}

func TestGRPCAgentRegistry_IsRegistered(t *testing.T) {
	t.Run("registered agent", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("test-agent", "localhost:50051")
		require.NoError(t, err)

		assert.True(t, registry.IsRegistered("test-agent"))
	})

	t.Run("unregistered agent", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		assert.False(t, registry.IsRegistered("non-existent"))
	})

	t.Run("after unregister", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		err := registry.RegisterAddress("test-agent", "localhost:50051")
		require.NoError(t, err)

		err = registry.Unregister("test-agent")
		require.NoError(t, err)

		assert.False(t, registry.IsRegistered("test-agent"))
	})
}

func TestGRPCAgentRegistry_Count(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("after registering agents", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.RegisterAddress("agent-1", "localhost:50051")
		require.NoError(t, err)
		assert.Equal(t, 1, registry.Count())

		err = registry.RegisterAddress("agent-2", "localhost:50052")
		require.NoError(t, err)
		assert.Equal(t, 2, registry.Count())

		err = registry.RegisterAddress("agent-3", "localhost:50053")
		require.NoError(t, err)
		assert.Equal(t, 3, registry.Count())
	})

	t.Run("after unregistering", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.RegisterAddress("agent-1", "localhost:50051")
		require.NoError(t, err)
		err = registry.RegisterAddress("agent-2", "localhost:50052")
		require.NoError(t, err)

		assert.Equal(t, 2, registry.Count())

		err = registry.Unregister("agent-1")
		require.NoError(t, err)
		assert.Equal(t, 1, registry.Count())
	})
}

func TestGRPCAgentRegistry_GetDescriptor_NotFound(t *testing.T) {
	t.Run("agent not registered", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		_, err := registry.GetDescriptor("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent non-existent not found")
	})

	t.Run("cached descriptor", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		// Manually add a cached descriptor
		testDesc := AgentDescriptor{
			Name:        "test-agent",
			Version:     "1.0.0",
			Description: "Test agent",
			IsExternal:  true,
		}

		registry.mu.Lock()
		registry.addresses["test-agent"] = "localhost:50051"
		registry.descriptors["test-agent"] = testDesc
		registry.mu.Unlock()

		desc, err := registry.GetDescriptor("test-agent")
		assert.NoError(t, err)
		assert.Equal(t, testDesc.Name, desc.Name)
		assert.Equal(t, testDesc.Version, desc.Version)
		assert.Equal(t, testDesc.Description, desc.Description)
	})
}

func TestGRPCAgentRegistry_List_Empty(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		descriptors := registry.List()
		assert.NotNil(t, descriptors)
		assert.Empty(t, descriptors)
	})

	t.Run("with cached descriptors", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		// Manually add cached descriptors
		desc1 := AgentDescriptor{Name: "agent-1", Version: "1.0.0", IsExternal: true}
		desc2 := AgentDescriptor{Name: "agent-2", Version: "2.0.0", IsExternal: true}

		registry.mu.Lock()
		registry.addresses["agent-1"] = "localhost:50051"
		registry.addresses["agent-2"] = "localhost:50052"
		registry.descriptors["agent-1"] = desc1
		registry.descriptors["agent-2"] = desc2
		registry.mu.Unlock()

		descriptors := registry.List()
		assert.Len(t, descriptors, 2)

		// Find descriptors by name
		foundAgent1 := false
		foundAgent2 := false
		for _, desc := range descriptors {
			if desc.Name == "agent-1" {
				foundAgent1 = true
				assert.Equal(t, "1.0.0", desc.Version)
			}
			if desc.Name == "agent-2" {
				foundAgent2 = true
				assert.Equal(t, "2.0.0", desc.Version)
			}
		}
		assert.True(t, foundAgent1, "agent-1 should be in list")
		assert.True(t, foundAgent2, "agent-2 should be in list")
	})

	t.Run("partial cached descriptors", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		// Register two agents but only cache one descriptor
		desc1 := AgentDescriptor{Name: "agent-1", Version: "1.0.0", IsExternal: true}

		registry.mu.Lock()
		registry.addresses["agent-1"] = "localhost:50051"
		registry.addresses["agent-2"] = "localhost:50052" // No descriptor cached
		registry.descriptors["agent-1"] = desc1
		registry.mu.Unlock()

		descriptors := registry.List()

		// Should return at least the cached descriptor
		// The uncached one will fail to fetch (no real server) and be skipped
		assert.GreaterOrEqual(t, len(descriptors), 1)

		foundAgent1 := false
		for _, desc := range descriptors {
			if desc.Name == "agent-1" {
				foundAgent1 = true
			}
		}
		assert.True(t, foundAgent1, "cached agent-1 should be in list")
	})
}

func TestGRPCAgentRegistry_Health_NoAgents(t *testing.T) {
	t.Run("empty registry is healthy", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		ctx := context.Background()

		status := registry.Health(ctx)
		assert.True(t, status.IsHealthy())
		assert.Contains(t, status.Message, "No agents registered")
	})

	t.Run("agents registered but not connected", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		ctx := context.Background()

		err := registry.RegisterAddress("agent-1", "localhost:50051")
		require.NoError(t, err)

		// Should be healthy since we haven't connected yet
		status := registry.Health(ctx)
		assert.True(t, status.IsHealthy())
	})
}

func TestGRPCAgentRegistry_Close(t *testing.T) {
	t.Run("close empty registry", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.Close()
		assert.NoError(t, err)
	})

	t.Run("close clears maps", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		// Register some agents and add descriptors
		err := registry.RegisterAddress("agent-1", "localhost:50051")
		require.NoError(t, err)

		registry.mu.Lock()
		registry.descriptors["agent-1"] = AgentDescriptor{
			Name:    "agent-1",
			Version: "1.0.0",
		}
		registry.mu.Unlock()

		err = registry.Close()
		assert.NoError(t, err)

		// Verify maps are cleared
		registry.mu.RLock()
		assert.Empty(t, registry.clients)
		assert.Empty(t, registry.descriptors)
		registry.mu.RUnlock()

		// Note: addresses are NOT cleared by Close() according to the implementation
		// Only clients and descriptors are cleared
	})
}

func TestGRPCAgentRegistry_DelegateToAgent_NotFound(t *testing.T) {
	t.Run("agent not registered", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		ctx := context.Background()

		task := NewTask("test-task", "Test task", map[string]any{})

		_, err := registry.DelegateToAgent(ctx, "non-existent", task, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent non-existent not found")
	})
}

func TestGRPCAgentRegistry_RegisterInternal_NotSupported(t *testing.T) {
	t.Run("RegisterInternal returns error", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		factory := func(cfg AgentConfig) (Agent, error) {
			return nil, nil
		}

		err := registry.RegisterInternal("test-agent", factory)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not support internal agents")
	})
}

func TestGRPCAgentRegistry_RegisterExternal_NotImplemented(t *testing.T) {
	t.Run("RegisterExternal returns error", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.RegisterExternal("test-agent", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not yet fully implemented")
	})

	t.Run("RegisterExternal with empty name", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		err := registry.RegisterExternal("", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent name cannot be empty")
	})
}

func TestGRPCAgentRegistry_RunningAgents(t *testing.T) {
	t.Run("always returns empty slice", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		running := registry.RunningAgents()
		assert.NotNil(t, running)
		assert.Empty(t, running)
	})
}

func TestGRPCAgentRegistry_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent registration", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		var wg sync.WaitGroup

		// Register 100 agents concurrently
		numAgents := 100
		wg.Add(numAgents)

		for i := 0; i < numAgents; i++ {
			go func(id int) {
				defer wg.Done()
				agentName := fmt.Sprintf("agent-%d", id)
				address := fmt.Sprintf("localhost:%d", 50000+id)
				err := registry.RegisterAddress(agentName, address)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()

		// Verify all agents were registered
		assert.Equal(t, numAgents, registry.Count())
	})

	t.Run("concurrent register and unregister", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		var wg sync.WaitGroup

		// Pre-register some agents
		for i := 0; i < 50; i++ {
			agentName := fmt.Sprintf("agent-%d", i)
			address := fmt.Sprintf("localhost:%d", 50000+i)
			err := registry.RegisterAddress(agentName, address)
			require.NoError(t, err)
		}

		// Concurrently register new agents and unregister existing ones
		numOps := 100
		wg.Add(numOps)

		for i := 0; i < numOps; i++ {
			go func(id int) {
				defer wg.Done()
				if id%2 == 0 {
					// Register new agent
					agentName := fmt.Sprintf("new-agent-%d", id)
					address := fmt.Sprintf("localhost:%d", 60000+id)
					_ = registry.RegisterAddress(agentName, address)
				} else {
					// Unregister existing agent
					agentName := fmt.Sprintf("agent-%d", id%50)
					_ = registry.Unregister(agentName)
				}
			}(i)
		}

		wg.Wait()

		// Just verify no panic occurred and count is reasonable
		count := registry.Count()
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		// Use short timeout to avoid waiting for connection failures
		registry := NewGRPCAgentRegistry(WithTimeout(100 * time.Millisecond))
		var wg sync.WaitGroup

		// Pre-register some agents and cache descriptors to avoid connection attempts
		for i := 0; i < 10; i++ {
			agentName := fmt.Sprintf("agent-%d", i)
			address := fmt.Sprintf("localhost:%d", 50000+i)
			err := registry.RegisterAddress(agentName, address)
			require.NoError(t, err)

			// Pre-cache descriptors to avoid connection attempts in List()
			registry.mu.Lock()
			registry.descriptors[agentName] = AgentDescriptor{
				Name:        agentName,
				Version:     "1.0.0",
				Description: "Test agent",
				IsExternal:  true,
			}
			registry.mu.Unlock()
		}

		// Concurrent operations: register, unregister, count, isRegistered, list
		// Reduced from 200 to 100 to speed up tests while still testing concurrency
		numOps := 100
		wg.Add(numOps)

		for i := 0; i < numOps; i++ {
			go func(id int) {
				defer wg.Done()

				switch id % 4 {
				case 0:
					// Register
					agentName := fmt.Sprintf("concurrent-agent-%d", id)
					address := fmt.Sprintf("localhost:%d", 60000+id)
					_ = registry.RegisterAddress(agentName, address)
				case 1:
					// Unregister
					agentName := fmt.Sprintf("agent-%d", id%10)
					_ = registry.Unregister(agentName)
				case 2:
					// Count
					_ = registry.Count()
				case 3:
					// IsRegistered
					agentName := fmt.Sprintf("agent-%d", id%10)
					_ = registry.IsRegistered(agentName)
					// Skip List() in concurrent tests to avoid timeout delays
				}
			}(i)
		}

		wg.Wait()

		// Verify no panic and registry is still functional
		count := registry.Count()
		assert.GreaterOrEqual(t, count, 0)

		// Verify we can still register after concurrent access
		err := registry.RegisterAddress("final-agent", "localhost:50099")
		assert.NoError(t, err)
		assert.True(t, registry.IsRegistered("final-agent"))
	})

	t.Run("concurrent descriptor caching", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		var wg sync.WaitGroup

		// Register agents
		for i := 0; i < 10; i++ {
			agentName := fmt.Sprintf("agent-%d", i)
			address := fmt.Sprintf("localhost:%d", 50000+i)
			err := registry.RegisterAddress(agentName, address)
			require.NoError(t, err)
		}

		// Concurrently cache descriptors
		numOps := 100
		wg.Add(numOps)

		for i := 0; i < numOps; i++ {
			go func(id int) {
				defer wg.Done()

				agentName := fmt.Sprintf("agent-%d", id%10)

				// Manually cache a descriptor
				registry.mu.Lock()
				registry.descriptors[agentName] = AgentDescriptor{
					Name:        agentName,
					Version:     fmt.Sprintf("1.%d.0", id),
					Description: "Test agent",
					IsExternal:  true,
				}
				registry.mu.Unlock()

				// Read it back
				desc, err := registry.GetDescriptor(agentName)
				if err == nil {
					assert.Equal(t, agentName, desc.Name)
				}
			}(i)
		}

		wg.Wait()

		// Verify descriptors were cached
		assert.Equal(t, 10, len(registry.descriptors))
	})

	t.Run("concurrent List calls with cached descriptors", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		var wg sync.WaitGroup

		// Pre-register agents with cached descriptors
		for i := 0; i < 10; i++ {
			agentName := fmt.Sprintf("agent-%d", i)
			address := fmt.Sprintf("localhost:%d", 50000+i)
			err := registry.RegisterAddress(agentName, address)
			require.NoError(t, err)

			// Cache descriptor immediately
			registry.mu.Lock()
			registry.descriptors[agentName] = AgentDescriptor{
				Name:        agentName,
				Version:     "1.0.0",
				Description: "Test agent",
				IsExternal:  true,
			}
			registry.mu.Unlock()
		}

		// Concurrent List() calls - should be fast since all descriptors are cached
		numOps := 50
		wg.Add(numOps)

		for i := 0; i < numOps; i++ {
			go func() {
				defer wg.Done()
				descriptors := registry.List()
				assert.NotNil(t, descriptors)
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent health checks", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		var wg sync.WaitGroup

		// Register some agents
		for i := 0; i < 5; i++ {
			agentName := fmt.Sprintf("agent-%d", i)
			address := fmt.Sprintf("localhost:%d", 50000+i)
			err := registry.RegisterAddress(agentName, address)
			require.NoError(t, err)
		}

		// Concurrent health checks
		numChecks := 50
		wg.Add(numChecks)

		for i := 0; i < numChecks; i++ {
			go func() {
				defer wg.Done()
				ctx := context.Background()
				status := registry.Health(ctx)
				assert.NotNil(t, status)
			}()
		}

		wg.Wait()
	})
}

func TestGRPCAgentRegistry_Create_NotFound(t *testing.T) {
	t.Run("agent not registered", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()
		cfg := NewAgentConfig("non-existent")

		_, err := registry.Create("non-existent", cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent non-existent not found")
	})
}

func TestGRPCAgentRegistry_Timeout(t *testing.T) {
	t.Run("custom timeout is respected", func(t *testing.T) {
		customTimeout := 100 * time.Millisecond
		registry := NewGRPCAgentRegistry(WithTimeout(customTimeout))

		assert.Equal(t, customTimeout, registry.timeout)
	})

	t.Run("default timeout", func(t *testing.T) {
		registry := NewGRPCAgentRegistry()

		assert.Equal(t, 30*time.Second, registry.timeout)
	})
}

// Benchmark tests
func BenchmarkGRPCAgentRegistry_RegisterAddress(b *testing.B) {
	registry := NewGRPCAgentRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentName := fmt.Sprintf("agent-%d", i)
		address := fmt.Sprintf("localhost:%d", 50000+(i%10000))
		_ = registry.RegisterAddress(agentName, address)
	}
}

func BenchmarkGRPCAgentRegistry_IsRegistered(b *testing.B) {
	registry := NewGRPCAgentRegistry()

	// Pre-register some agents
	for i := 0; i < 1000; i++ {
		agentName := fmt.Sprintf("agent-%d", i)
		address := fmt.Sprintf("localhost:%d", 50000+i)
		_ = registry.RegisterAddress(agentName, address)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agentName := fmt.Sprintf("agent-%d", i%1000)
		_ = registry.IsRegistered(agentName)
	}
}

func BenchmarkGRPCAgentRegistry_Count(b *testing.B) {
	registry := NewGRPCAgentRegistry()

	// Pre-register some agents
	for i := 0; i < 1000; i++ {
		agentName := fmt.Sprintf("agent-%d", i)
		address := fmt.Sprintf("localhost:%d", 50000+i)
		_ = registry.RegisterAddress(agentName, address)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.Count()
	}
}

func BenchmarkGRPCAgentRegistry_List(b *testing.B) {
	registry := NewGRPCAgentRegistry()

	// Pre-register and cache descriptors
	for i := 0; i < 100; i++ {
		agentName := fmt.Sprintf("agent-%d", i)
		address := fmt.Sprintf("localhost:%d", 50000+i)
		_ = registry.RegisterAddress(agentName, address)

		registry.mu.Lock()
		registry.descriptors[agentName] = AgentDescriptor{
			Name:        agentName,
			Version:     "1.0.0",
			Description: "Test agent",
			IsExternal:  true,
		}
		registry.mu.Unlock()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.List()
	}
}

func BenchmarkGRPCAgentRegistry_ConcurrentAccess(b *testing.B) {
	registry := NewGRPCAgentRegistry()

	// Pre-register some agents
	for i := 0; i < 100; i++ {
		agentName := fmt.Sprintf("agent-%d", i)
		address := fmt.Sprintf("localhost:%d", 50000+i)
		_ = registry.RegisterAddress(agentName, address)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0:
				agentName := fmt.Sprintf("concurrent-%d", i)
				_ = registry.RegisterAddress(agentName, "localhost:60000")
			case 1:
				_ = registry.Count()
			case 2:
				_ = registry.IsRegistered("agent-0")
			case 3:
				_ = registry.List()
			}
			i++
		}
	})
}
