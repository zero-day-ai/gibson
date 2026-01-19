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
// GraphRAG is a required core component - this function will always attempt
// to create a real provider and return an error if initialization fails.
//
// The Provider field in config specifies the graph database type:
//   - "neo4j" - maps to local deployment
//   - "neptune", "memgraph" - maps to cloud deployment
//   - "local", "cloud", "hybrid" - explicit deployment types
func NewProvider(config graphrag.GraphRAGConfig) (graphrag.GraphRAGProvider, error) {
	// Validate configuration (GraphRAG is required)
	if err := config.Validate(); err != nil {
		return nil, graphrag.NewConfigError("invalid GraphRAG configuration", err)
	}

	// Determine provider type from configuration
	providerType := config.Provider
	if providerType == "" {
		return nil, graphrag.NewConfigError("provider is required", nil)
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
	default:
		return nil, graphrag.NewConfigError(
			fmt.Sprintf("unsupported provider type: %s (valid types: neo4j, neptune, memgraph, local, cloud, hybrid)", providerType),
			nil,
		)
	}
}
