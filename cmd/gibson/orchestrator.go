package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/llm/providers"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/plugin"
	"github.com/zero-day-ai/gibson/internal/registry"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/workflow"
	"go.opentelemetry.io/otel/trace"
)

// OrchestratorBundle contains the orchestrator and all associated resources
// that need to be cleaned up when done.
type OrchestratorBundle struct {
	// Orchestrator is the mission orchestrator for executing workflows
	Orchestrator mission.MissionOrchestrator

	// MissionStore provides access to mission persistence
	MissionStore mission.MissionStore

	// FindingStore provides access to finding persistence
	FindingStore finding.FindingStore

	// RegistryAdapter provides access to component discovery (agents, tools, plugins)
	RegistryAdapter registry.ComponentDiscovery

	// EventEmitter provides access to mission events for progress reporting
	EventEmitter mission.EventEmitter

	// Cleanup releases all resources associated with the orchestrator.
	// Must be called when done with the orchestrator.
	Cleanup func()
}

// OrchestratorOptions contains optional configuration for orchestrator creation.
type OrchestratorOptions struct {
	// Tracer is an optional OpenTelemetry tracer for distributed tracing
	Tracer trace.Tracer
}

// createOrchestrator initializes all dependencies and returns an OrchestratorBundle
// with a fully-configured MissionOrchestrator ready for workflow execution.
//
// The returned bundle includes:
// - MissionOrchestrator for executing workflows
// - MissionStore for mission persistence
// - FindingStore for finding persistence
// - EventEmitter for progress events
// - Cleanup function to release resources
//
// Caller must call bundle.Cleanup() when done to release database connections
// and other resources.
func createOrchestrator(ctx context.Context) (*OrchestratorBundle, error) {
	return createOrchestratorWithOptions(ctx, nil)
}

// createOrchestratorWithOptions creates an orchestrator with optional verbose logging support.
func createOrchestratorWithOptions(ctx context.Context, opts *OrchestratorOptions) (*OrchestratorBundle, error) {
	if opts == nil {
		opts = &OrchestratorOptions{}
	}
	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return nil, fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Track resources for cleanup
	cleanupFuncs := []func(){
		func() {
			if err := db.Close(); err != nil {
				slog.Warn("failed to close database", "error", err)
			}
		},
	}

	// Cleanup helper that runs all cleanup functions in reverse order
	cleanup := func() {
		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}

	// Step 1: Create stores
	missionStore := mission.NewDBMissionStore(db)
	findingStore := finding.NewDBFindingStore(db)

	// Step 2: Get registry manager from context and create adapter
	regManager := component.GetRegistryManager(ctx)
	if regManager == nil {
		cleanup()
		return nil, fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	// Create registry adapter for component discovery
	registryAdapter := registry.NewRegistryAdapter(regManager.Registry())

	// Step 3: Create legacy registries (tools and plugins still use legacy registries for now)
	toolRegistry := tool.NewToolRegistry()
	pluginRegistry := plugin.NewPluginRegistry()

	// Step 4: Create LLM components
	llmRegistry, slotManager, err := createLLMComponents()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to initialize LLM components: %w", err)
	}

	// Step 5: Create harness factory
	// Use provided tracer or create a no-op one
	tracer := opts.Tracer
	if tracer == nil {
		tracer = trace.NewNoopTracerProvider().Tracer("orchestrator")
	}

	harnessConfig := harness.HarnessConfig{
		LLMRegistry:     llmRegistry,
		SlotManager:     slotManager,
		ToolRegistry:    toolRegistry,
		PluginRegistry:  pluginRegistry,
		RegistryAdapter: registryAdapter,
		FindingStore:    nil, // Will be created per-harness if needed
		Logger:          slog.Default(),
		Tracer:          tracer,
	}

	harnessFactory, err := harness.NewDefaultHarnessFactory(harnessConfig)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create harness factory: %w", err)
	}

	// Step 6: Create workflow executor
	executorOpts := []workflow.ExecutorOption{
		workflow.WithLogger(slog.Default()),
		workflow.WithTracer(tracer),
	}

	workflowExecutor := workflow.NewWorkflowExecutor(executorOpts...)

	// Step 7: Create event emitter for progress reporting
	eventEmitter := mission.NewDefaultEventEmitter(mission.WithBufferSize(100))

	// Step 8: Create mission orchestrator
	orchestrator := mission.NewMissionOrchestrator(
		missionStore,
		mission.WithWorkflowExecutor(workflowExecutor),
		mission.WithHarnessFactory(harnessFactory),
		mission.WithEventEmitter(eventEmitter),
	)

	return &OrchestratorBundle{
		Orchestrator:    orchestrator,
		MissionStore:    missionStore,
		FindingStore:    findingStore,
		RegistryAdapter: registryAdapter,
		EventEmitter:    eventEmitter,
		Cleanup:         cleanup,
	}, nil
}

// createLLMComponents creates and configures LLM registry and slot manager.
// It automatically detects and registers available LLM providers based on
// environment variables:
// - ANTHROPIC_API_KEY for Claude models
// - OPENAI_API_KEY for GPT models
// - GOOGLE_API_KEY for Gemini models
// - OLLAMA_URL (or default localhost:11434) for local Ollama models
func createLLMComponents() (llm.LLMRegistry, llm.SlotManager, error) {
	// Create registry
	registry := llm.NewLLMRegistry()

	// Track number of providers successfully registered
	providersRegistered := 0

	// Check for Anthropic
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		cfg := llm.ProviderConfig{
			Type:         llm.ProviderAnthropic,
			APIKey:       apiKey,
			DefaultModel: os.Getenv("ANTHROPIC_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewAnthropicProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Anthropic provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register Anthropic provider", "error", err)
			} else {
				slog.Info("registered Anthropic LLM provider")
				providersRegistered++
			}
		}
	}

	// Check for OpenAI
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg := llm.ProviderConfig{
			Type:         llm.ProviderOpenAI,
			APIKey:       apiKey,
			DefaultModel: os.Getenv("OPENAI_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewOpenAIProvider(cfg)
		if err != nil {
			slog.Warn("failed to create OpenAI provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register OpenAI provider", "error", err)
			} else {
				slog.Info("registered OpenAI LLM provider")
				providersRegistered++
			}
		}
	}

	// Check for Google
	if apiKey := os.Getenv("GOOGLE_API_KEY"); apiKey != "" {
		cfg := llm.ProviderConfig{
			Type:         llm.ProviderGoogle,
			APIKey:       apiKey,
			DefaultModel: os.Getenv("GOOGLE_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewGoogleProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Google provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register Google provider", "error", err)
			} else {
				slog.Info("registered Google LLM provider")
				providersRegistered++
			}
		}
	}

	// Check for Ollama (local, no API key required)
	if ollamaURL := os.Getenv("OLLAMA_URL"); ollamaURL != "" {
		cfg := llm.ProviderConfig{
			Type:         "ollama",
			BaseURL:      ollamaURL,
			DefaultModel: os.Getenv("OLLAMA_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewOllamaProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Ollama provider", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Warn("failed to register Ollama provider", "error", err)
			} else {
				slog.Info("registered Ollama LLM provider", "url", ollamaURL)
				providersRegistered++
			}
		}
	} else {
		// Try default Ollama URL (localhost:11434)
		cfg := llm.ProviderConfig{
			Type:         "ollama",
			BaseURL:      "http://localhost:11434",
			DefaultModel: os.Getenv("OLLAMA_MODEL"), // Use env var, provider will use its default if empty
		}

		provider, err := providers.NewOllamaProvider(cfg)
		if err != nil {
			// Don't warn for default Ollama - it's optional
			slog.Debug("Ollama not available at default URL", "error", err)
		} else {
			if err := registry.RegisterProvider(provider); err != nil {
				slog.Debug("failed to register default Ollama provider", "error", err)
			} else {
				slog.Info("registered Ollama LLM provider at default URL")
				providersRegistered++
			}
		}
	}

	// Log warning if no providers are available
	if providersRegistered == 0 {
		slog.Warn("no LLM providers available - set ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY, or configure Ollama")
	} else {
		slog.Info("LLM initialization complete", "providers", providersRegistered)
	}

	// Create slot manager
	slotManager := llm.NewSlotManager(registry)

	return registry, slotManager, nil
}
