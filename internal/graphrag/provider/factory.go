package provider

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/graphrag"
)

// NewProvider creates a new GraphRAGProvider based on the configuration.
// Returns the appropriate provider implementation for the configured provider type.
// Returns an error if the provider type is invalid or initialization fails.
//
// This is the main entry point for creating GraphRAG providers.
//
// The Provider field in config has two interpretations:
//   - Graph database types: "neo4j", "neptune", "memgraph" - these map to deployment types
//   - Deployment types: "local", "cloud", "hybrid", "noop"
//
// Mapping: neo4j -> local, neptune/memgraph -> cloud
func NewProvider(config graphrag.GraphRAGConfig) (graphrag.GraphRAGProvider, error) {
	if !config.Enabled {
		return NewNoopProvider(), nil
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, graphrag.NewConfigError("invalid GraphRAG configuration", err)
	}

	// Determine provider type from configuration
	providerType := config.Provider
	if providerType == "" {
		providerType = "local" // Default to local
	}

	// Map graph database types to deployment types
	switch providerType {
	case "neo4j":
		providerType = "local" // Neo4j uses local provider
	case "neptune", "memgraph":
		providerType = "cloud" // Cloud databases use cloud provider
	}

	// Create provider based on type
	switch providerType {
	case "local":
		return NewLocalProvider(config)
	case "cloud":
		return NewCloudProvider(config)
	case "hybrid":
		return NewHybridProvider(config)
	case "noop":
		return NewNoopProvider(), nil
	default:
		return nil, graphrag.NewConfigError(
			fmt.Sprintf("unsupported provider type: %s", providerType),
			nil,
		)
	}
}
