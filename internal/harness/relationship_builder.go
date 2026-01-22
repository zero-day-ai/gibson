package harness

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TaxonomyRegistry provides taxonomy lookups for relationship building.
// This interface will be implemented by the internal/graphrag/taxonomy_registry.go
// component in a parallel task.
type TaxonomyRegistry interface {
	// GetParentRelationship returns the relationship type for a parent-child node pair.
	// Returns (relationshipType, true) if a parent relationship exists in the taxonomy,
	// or ("", false) if no relationship is defined.
	//
	// Example: GetParentRelationship("port", "host") returns ("HAS_PORT", true)
	GetParentRelationship(childType, parentType string) (string, bool)

	// IsAssetType returns true if the given node type is classified as an asset
	// in the taxonomy. Asset types should have DISCOVERED relationships created.
	//
	// Asset types include: host, port, service, endpoint, domain, subdomain, etc.
	// Non-asset types include: finding, mission, agent_run, tool_execution, etc.
	IsAssetType(nodeType string) bool
}

// NodeStore provides access to graph nodes for parent lookup.
// This interface allows the RelationshipBuilder to verify parent nodes exist
// before creating relationships.
type NodeStore interface {
	// GetNode retrieves a single node by ID from the graph.
	// Returns the node if found, or an error if not found or query fails.
	GetNode(ctx context.Context, nodeID types.ID) (*graphrag.GraphNode, error)
}

// RelationshipBuilder creates taxonomy-defined relationships when nodes are stored.
// It implements two key relationship patterns:
//  1. DISCOVERED relationships: (AgentRun)-[:DISCOVERED]->(AssetNode)
//  2. Parent relationships: (ParentNode)-[:HAS_PORT|RUNS_SERVICE|etc.]->(ChildNode)
//
// The builder uses the TaxonomyRegistry to determine appropriate relationship types
// based on node types, and validates parent existence via NodeStore.
type RelationshipBuilder interface {
	// BuildRelationships creates relationships for a node based on taxonomy rules.
	// Returns a slice of Relationship objects to be created in the graph.
	//
	// Relationship creation rules:
	//  1. If node is an asset type, create (AgentRun)-[:DISCOVERED]->(node)
	//  2. If node has a parent reference, create parent relationship from taxonomy
	//  3. Log warnings for missing parents, don't return errors
	//
	// The relationships are returned as graphrag.Relationship structs that the caller
	// must store in the graph database.
	BuildRelationships(ctx context.Context, node *graphrag.GraphNode) ([]*graphrag.Relationship, error)
}

// relationshipBuilder implements RelationshipBuilder with taxonomy-based logic.
type relationshipBuilder struct {
	registry TaxonomyRegistry
	store    NodeStore
	logger   *slog.Logger
}

// NewRelationshipBuilder creates a new RelationshipBuilder with the given dependencies.
// The TaxonomyRegistry is used for relationship type lookups, and NodeStore for
// parent node validation.
//
// If logger is nil, slog.Default() is used.
func NewRelationshipBuilder(registry TaxonomyRegistry, store NodeStore, logger *slog.Logger) RelationshipBuilder {
	if logger == nil {
		logger = slog.Default()
	}
	return &relationshipBuilder{
		registry: registry,
		store:    store,
		logger:   logger,
	}
}

// BuildRelationships implements the RelationshipBuilder interface.
// It creates relationships based on taxonomy rules and node properties.
func (rb *relationshipBuilder) BuildRelationships(ctx context.Context, node *graphrag.GraphNode) ([]*graphrag.Relationship, error) {
	var relationships []*graphrag.Relationship

	// Validate node has at least one label
	if len(node.Labels) == 0 {
		return relationships, fmt.Errorf("node has no labels")
	}

	// Use the first label as the primary node type
	nodeType := node.Labels[0].String()

	// Rule 1: Create DISCOVERED relationship from AgentRun to asset nodes
	if rb.registry.IsAssetType(nodeType) {
		discoveredRel, err := rb.createDiscoveredRelationship(ctx, node)
		if err != nil {
			// Log warning but don't fail - DISCOVERED relationship is optional
			rb.logger.WarnContext(ctx, "Failed to create DISCOVERED relationship",
				"node_id", node.ID.String(),
				"node_type", nodeType,
				"error", err)
		} else if discoveredRel != nil {
			relationships = append(relationships, discoveredRel)
		}
	}

	// Rule 2: Create parent relationship if node has a parent reference
	// Check if node has a parent_id or parent_ref property
	parentNodeID, hasParent := rb.extractParentID(node)
	if hasParent && parentNodeID != "" {
		parentRel, err := rb.createParentRelationship(ctx, node, nodeType, parentNodeID)
		if err != nil {
			// Log warning but don't fail - parent relationship is optional
			rb.logger.WarnContext(ctx, "Failed to create parent relationship",
				"node_id", node.ID.String(),
				"node_type", nodeType,
				"parent_id", parentNodeID,
				"error", err)
		} else if parentRel != nil {
			relationships = append(relationships, parentRel)
		}
	}

	return relationships, nil
}

// createDiscoveredRelationship creates a DISCOVERED relationship from AgentRun to node.
func (rb *relationshipBuilder) createDiscoveredRelationship(ctx context.Context, node *graphrag.GraphNode) (*graphrag.Relationship, error) {
	// Get AgentRunID from context
	agentRunID := AgentRunIDFromContext(ctx)
	if agentRunID == "" {
		return nil, fmt.Errorf("agent_run_id not found in context")
	}

	// Parse AgentRunID as types.ID
	agentRunTypeID, err := types.ParseID(agentRunID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_run_id format: %w", err)
	}

	// Create DISCOVERED relationship: (AgentRun)-[:DISCOVERED]->(Node)
	rel := graphrag.NewRelationship(
		agentRunTypeID,
		node.ID,
		graphrag.RelationType(graphrag.RelTypeDiscovered),
	)

	return rel, nil
}

// createParentRelationship creates a parent relationship based on taxonomy rules.
func (rb *relationshipBuilder) createParentRelationship(ctx context.Context, node *graphrag.GraphNode, nodeType, parentNodeID string) (*graphrag.Relationship, error) {
	// Parse parent node ID
	parentID, err := types.ParseID(parentNodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid parent node ID: %w", err)
	}

	// Lookup parent node to determine its type
	parentNode, err := rb.store.GetNode(ctx, parentID)
	if err != nil {
		// Parent not found - log warning but don't fail
		// This allows for eventual consistency: child can be stored before parent
		rb.logger.WarnContext(ctx, "Parent node not found in graph - skipping parent relationship",
			"child_id", node.ID.String(),
			"child_type", nodeType,
			"parent_id", parentNodeID)
		return nil, nil
	}

	// Get parent node type (use first label)
	if len(parentNode.Labels) == 0 {
		return nil, fmt.Errorf("parent node has no labels")
	}
	parentType := parentNode.Labels[0].String()

	// Lookup relationship type from taxonomy
	relationshipType, found := rb.registry.GetParentRelationship(nodeType, parentType)
	if !found {
		// No relationship defined in taxonomy - log warning
		rb.logger.WarnContext(ctx, "No parent relationship defined in taxonomy",
			"child_type", nodeType,
			"parent_type", parentType,
			"child_id", node.ID.String(),
			"parent_id", parentNodeID)
		return nil, nil
	}

	// Create parent relationship: (Parent)-[:HAS_PORT|RUNS_SERVICE|etc.]->(Child)
	rel := graphrag.NewRelationship(
		parentID,
		node.ID,
		graphrag.RelationType(relationshipType),
	)

	return rel, nil
}

// extractParentID extracts the parent node ID from node properties.
// This supports multiple property naming conventions:
//   - parent_id (standard)
//   - parent_ref (legacy)
//   - {type}_id (e.g., host_id for port nodes)
//
// Returns (parentID, true) if found, or ("", false) if not found.
func (rb *relationshipBuilder) extractParentID(node *graphrag.GraphNode) (string, bool) {
	// Try standard parent_id property
	if parentID, ok := node.Properties["parent_id"].(string); ok && parentID != "" {
		return parentID, true
	}

	// Try parent_ref property (legacy)
	if parentRef, ok := node.Properties["parent_ref"].(string); ok && parentRef != "" {
		return parentRef, true
	}

	// Try type-specific parent ID (e.g., host_id for port)
	// This supports the legacy pattern: port.HostID sets host_id property
	nodeType := node.Labels[0].String()
	switch nodeType {
	case "port":
		if hostID, ok := node.Properties["host_id"].(string); ok && hostID != "" {
			return hostID, true
		}
	case "service":
		if portID, ok := node.Properties["port_id"].(string); ok && portID != "" {
			return portID, true
		}
	case "endpoint":
		if serviceID, ok := node.Properties["service_id"].(string); ok && serviceID != "" {
			return serviceID, true
		}
	case "subdomain":
		if domainID, ok := node.Properties["domain_id"].(string); ok && domainID != "" {
			return domainID, true
		}
	case "certificate":
		if hostID, ok := node.Properties["host_id"].(string); ok && hostID != "" {
			return hostID, true
		}
	}

	// No parent found
	return "", false
}
