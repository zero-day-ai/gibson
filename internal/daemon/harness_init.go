package daemon

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/observability"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// newHarnessFactory creates a new HarnessFactory with all required dependencies.
//
// This method initializes the HarnessFactory that will be used to create
// AgentHarness instances during agent execution. The factory is configured with:
//   - LLMRegistry from infrastructure (provides access to LLM providers)
//   - SlotManager from infrastructure (resolves slot names to providers)
//   - ToolRegistry via RegistryAdapter (accesses registered tools)
//   - PluginRegistry via RegistryAdapter (accesses registered plugins)
//   - FindingStore from infrastructure (stores security findings)
//   - RegistryAdapter for component discovery (agent delegation)
//   - Logger with daemon context
//
// The method assumes that infrastructure has been initialized with all required
// components (LLMRegistry, SlotManager, FindingStore) and that the daemon's
// registryAdapter has been created.
//
// Returns:
//   - harness.HarnessFactoryInterface: Configured factory ready to create harnesses
//   - error: Non-nil if factory creation fails (e.g., invalid configuration)
func (d *daemonImpl) newHarnessFactory(ctx context.Context) (harness.HarnessFactoryInterface, error) {
	d.logger.Debug("creating harness factory")

	// Get tracer from provider if available
	var tracer trace.Tracer
	if d.infrastructure != nil && d.infrastructure.tracerProvider != nil {
		tracer = d.infrastructure.tracerProvider.Tracer("gibson")
	}

	// Create harness wrapper if tracer is available
	var harnessWrapper func(harness.AgentHarness) harness.AgentHarness
	if tracer != nil {
		harnessWrapper = func(h harness.AgentHarness) harness.AgentHarness {
			return observability.NewTracedAgentHarness(h, observability.WithTracer(tracer))
		}
	}

	// Create memory wrapper if tracer is available
	var memoryWrapper func(memory.MemoryManager) memory.MemoryManager
	if tracer != nil {
		memoryWrapper = func(mm memory.MemoryManager) memory.MemoryManager {
			return memory.NewTracedMemoryManager(mm, tracer)
		}
	}

	// Build HarnessConfig with all required dependencies
	config := harness.HarnessConfig{
		// LLM components
		LLMRegistry: d.infrastructure.llmRegistry,
		SlotManager: d.infrastructure.slotManager,

		// Component registries
		// Note: ToolRegistry and PluginRegistry are not directly used since we have RegistryAdapter.
		// The harness will use RegistryAdapter for tool and plugin discovery.
		// We still need to provide empty registries to satisfy the config validation.
		ToolRegistry:   nil, // Will be defaulted by ApplyDefaults()
		PluginRegistry: nil, // Will be defaulted by ApplyDefaults()

		// Registry adapter for component discovery (agents, tools, plugins)
		RegistryAdapter: d.registryAdapter,

		// Finding storage
		// Note: We use harness.NewInMemoryFindingStore() here because the harness package
		// has its own FindingStore interface that is different from finding.FindingStore.
		// The daemon's finding.FindingStore is for database persistence (EnhancedFinding),
		// while harness.FindingStore is for in-memory storage during agent execution (agent.Finding).
		// Findings submitted via harness are stored in memory and can be persisted separately
		// to the database by the mission manager after agent execution completes.
		FindingStore: harness.NewInMemoryFindingStore(),

		// MemoryManager is nil - we use MemoryFactory instead for per-mission memory
		MemoryManager: nil,

		// MemoryFactory creates mission-scoped memory managers on demand.
		// This is called during harness creation with the mission ID from MissionContext.
		// Each mission gets its own isolated memory manager.
		MemoryFactory: func(missionID types.ID) (memory.MemoryManager, error) {
			return d.infrastructure.memoryManagerFactory.CreateForMission(context.Background(), missionID)
		},

		// Observability components
		Logger:  d.logger.With("component", "harness"),
		Tracer:  tracer,                     // May be nil, will be defaulted by ApplyDefaults() to no-op tracer
		Metrics: nil,                        // Will be defaulted by ApplyDefaults() to no-op metrics

		// Harness wrapper for adding observability (TracedAgentHarness)
		HarnessWrapper: harnessWrapper, // May be nil (no wrapping)

		// Memory wrapper for adding observability (TracedMemoryManager)
		MemoryWrapper: memoryWrapper, // May be nil (no wrapping)

		// GraphRAG components (optional, will be defaulted to no-op if nil)
		GraphRAGBridge:      d.infrastructure.graphRAGBridge,      // May be nil, will be defaulted by ApplyDefaults()
		GraphRAGQueryBridge: d.infrastructure.graphRAGQueryBridge, // May be nil, will be defaulted by ApplyDefaults()
	}

	// Create the factory with validation
	factory, err := harness.NewHarnessFactory(config)
	if err != nil {
		return nil, err
	}

	d.logger.Info("harness factory created successfully")
	return factory, nil
}
