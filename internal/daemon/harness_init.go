package daemon

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/harness/middleware"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/trace"
)

// newHarnessFactory creates a new HarnessFactory with all required dependencies.
//
// The factory is configured with middleware for observability (tracing, logging, events)
// and all necessary registries for agent execution.
//
// Returns:
//   - harness.HarnessFactoryInterface: Configured factory ready to create harnesses
//   - error: Non-nil if factory creation fails
func (d *daemonImpl) newHarnessFactory(ctx context.Context) (harness.HarnessFactoryInterface, error) {
	d.logger.Debug("creating harness factory")

	// Get tracer from provider if available
	var tracer trace.Tracer
	if d.infrastructure != nil && d.infrastructure.tracerProvider != nil {
		tracer = d.infrastructure.tracerProvider.Tracer("gibson")
	}

	// Build middleware chain for harness operations
	var middlewareChain middleware.Middleware
	if tracer != nil {
		// Build middleware chain with tracing
		// Additional middleware (logging, events) can be added here
		middlewareChain = middleware.Chain(
			middleware.TracingMiddleware(tracer),
			// middleware.LoggingMiddleware(logger, middleware.LevelNormal),
			// middleware.EventMiddleware(eventBus, errorHandler),
		)
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

		// Component registries (will be defaulted by ApplyDefaults())
		ToolRegistry:   nil,
		PluginRegistry: nil,

		// Registry adapter for component discovery
		RegistryAdapter: d.registryAdapter,

		// Finding storage (in-memory for agent execution)
		FindingStore: harness.NewInMemoryFindingStore(),

		// MemoryFactory creates mission-scoped memory managers on demand
		MemoryManager: nil,
		MemoryFactory: func(missionID types.ID) (memory.MemoryManager, error) {
			return d.infrastructure.memoryManagerFactory.CreateForMission(context.Background(), missionID)
		},

		// Observability
		Logger:  d.logger.With("component", "harness"),
		Tracer:  tracer,
		Metrics: nil, // Defaulted to no-op

		// Middleware chain for cross-cutting concerns
		Middleware: middlewareChain,

		// Memory wrapper for tracing
		MemoryWrapper: memoryWrapper,

		// GraphRAG components
		GraphRAGBridge:      d.infrastructure.graphRAGBridge,
		GraphRAGQueryBridge: d.infrastructure.graphRAGQueryBridge,
	}

	// Create the factory
	factory, err := harness.NewHarnessFactory(config)
	if err != nil {
		return nil, err
	}

	d.logger.Info("harness factory created successfully")
	return factory, nil
}
