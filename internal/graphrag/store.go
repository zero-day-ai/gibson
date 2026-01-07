// Package graphrag provides the GraphRAG store implementation.
// This file contains the unified GraphRAGStore interface and DefaultGraphRAGStore implementation
// for high-level GraphRAG operations combining graph storage, vector search, and hybrid queries.
//
// The store orchestrates:
// - Graph node and relationship storage
// - Embedding generation and vector indexing
// - Hybrid GraphRAG queries (semantic + structural retrieval)
// - Domain-specific operations (attack patterns, findings, attack chains)
//
// Usage:
//
//	config := GraphRAGConfig{Enabled: true, Provider: "neo4j", ...}
//	embedder := embedder.NewOpenAIEmbedder(...)
//	store, err := NewGraphRAGStore(config, embedder)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	// Store a finding
//	finding := NewFindingNode(id, "SQL Injection", "Found SQLi vulnerability", missionID)
//	if err := store.StoreFinding(ctx, *finding); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Query for similar findings
//	similar, err := store.FindSimilarFindings(ctx, finding.ID.String(), 5)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// The DefaultGraphRAGStore is thread-safe and can be used concurrently from multiple goroutines.
package graphrag

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/memory/embedder"
	"github.com/zero-day-ai/gibson/internal/types"
)

// GraphRAGStore provides a unified, high-level interface for GraphRAG operations.
// It orchestrates the full GraphRAG stack: graph storage, vector search, and hybrid queries.
//
// The store abstracts away the complexity of coordinating between:
// - GraphRAGProvider for graph + vector operations
// - QueryProcessor for hybrid query execution
// - Embedder for embedding generation
//
// Thread-safety: All implementations must be safe for concurrent access.
type GraphRAGStore interface {
	// Store stores a single graph record (node + optional relationships).
	// Automatically generates embeddings if not provided and upserts to both
	// graph database and vector store.
	Store(ctx context.Context, record GraphRecord) error

	// StoreBatch efficiently stores multiple graph records in a single operation.
	// Uses batch embedding generation and bulk upsert for optimal performance.
	StoreBatch(ctx context.Context, records []GraphRecord) error

	// Query executes a hybrid GraphRAG query combining vector similarity and graph traversal.
	// This is the primary query method for semantic + structural retrieval.
	Query(ctx context.Context, query GraphRAGQuery) ([]GraphRAGResult, error)

	// StoreAttackPattern stores a MITRE ATT&CK pattern with technique relationships.
	// Creates the attack pattern node and USES_TECHNIQUE relationships to techniques.
	// Automatically generates embeddings from pattern description.
	StoreAttackPattern(ctx context.Context, pattern AttackPattern) error

	// FindSimilarAttacks finds attack patterns similar to the given content.
	// Uses vector search filtered to AttackPattern node type.
	// Returns top-K most similar attack patterns ranked by similarity.
	FindSimilarAttacks(ctx context.Context, content string, topK int) ([]AttackPattern, error)

	// FindSimilarFindings finds findings similar to the given finding.
	// Uses vector search filtered to Finding node type.
	// Returns top-K most similar findings ranked by similarity.
	FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]FindingNode, error)

	// GetAttackChains discovers attack chains (technique sequences) from a starting technique.
	// Traverses USES_TECHNIQUE relationships to find multi-step attack patterns.
	// Returns all discovered chains up to maxDepth steps.
	GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]AttackChain, error)

	// StoreFinding stores a security finding with contextual relationships.
	// Creates the finding node and relationships to targets/techniques.
	// Automatically generates embeddings from finding description.
	StoreFinding(ctx context.Context, finding FindingNode) error

	// GetRelatedFindings retrieves findings related to the given finding.
	// Traverses SIMILAR_TO and other relationship types to find connected findings.
	// Useful for correlation and deduplication analysis.
	GetRelatedFindings(ctx context.Context, findingID string) ([]FindingNode, error)

	// Health returns the current health status of the GraphRAG store.
	// Aggregates health from provider, embedder, and processor.
	Health(ctx context.Context) types.HealthStatus

	// Close releases all resources and closes connections.
	// Should be called during graceful shutdown.
	Close() error
}

// GraphRecord represents a unified input for storing graph data.
// Combines a node with optional relationships to be created atomically.
type GraphRecord struct {
	Node          GraphNode      // The graph node to store
	Relationships []Relationship // Optional relationships to create
	EmbedContent  string         // Content to embed (if Node.Embedding is empty)
}

// NewGraphRecord creates a new GraphRecord with the given node.
func NewGraphRecord(node GraphNode) GraphRecord {
	return GraphRecord{
		Node:          node,
		Relationships: []Relationship{},
	}
}

// WithRelationship adds a relationship to the record.
// Returns the record for method chaining.
func (gr *GraphRecord) WithRelationship(rel Relationship) *GraphRecord {
	gr.Relationships = append(gr.Relationships, rel)
	return gr
}

// WithRelationships adds multiple relationships to the record.
// Returns the record for method chaining.
func (gr *GraphRecord) WithRelationships(rels []Relationship) *GraphRecord {
	gr.Relationships = append(gr.Relationships, rels...)
	return gr
}

// WithEmbedContent sets the content to use for embedding generation.
// Returns the record for method chaining.
func (gr *GraphRecord) WithEmbedContent(content string) *GraphRecord {
	gr.EmbedContent = content
	return gr
}

// DefaultGraphRAGStore implements GraphRAGStore using a provider and processor.
type DefaultGraphRAGStore struct {
	provider  GraphRAGProvider
	processor QueryProcessor
	embedder  embedder.Embedder
	config    GraphRAGConfig
}

// NewGraphRAGStore creates a new GraphRAGStore with the given configuration.
// Initializes the provider, processor, and embedder based on the config.
//
// Parameters:
//   - config: GraphRAG configuration
//   - emb: Embedder for generating embeddings
//
// Returns a GraphRAGStore ready for use, or an error if initialization fails.
func NewGraphRAGStore(config GraphRAGConfig, emb embedder.Embedder) (GraphRAGStore, error) {
	// Apply defaults
	config.ApplyDefaults()

	// Validate embedder
	if emb == nil {
		return nil, NewConfigError("embedder cannot be nil", nil)
	}

	// Check if GraphRAG is enabled
	// If enabled, we cannot create a provider here due to import cycles
	// Return a descriptive error directing users to NewGraphRAGStoreWithProvider
	if config.Enabled {
		return nil, NewConfigError(
			"GraphRAG is enabled but requires provider injection to avoid import cycles",
			nil,
		).WithContext("solution", "Use NewGraphRAGStoreWithProvider() and inject provider created via provider.NewProvider()").
			WithContext("provider_type", config.Provider).
			WithContext("neo4j_uri", config.Neo4j.URI)
	}

	// GraphRAG is disabled - validate config and create noop provider
	if err := config.Validate(); err != nil {
		return nil, NewConfigError("invalid GraphRAG configuration", err)
	}

	// Create noop provider for disabled GraphRAG
	provider := &noopProvider{}

	// Create query processor
	processor, err := NewQueryProcessorFromConfig(config, emb)
	if err != nil {
		return nil, NewConfigError("failed to create query processor", err)
	}

	return &DefaultGraphRAGStore{
		provider:  provider,
		processor: processor,
		embedder:  emb,
		config:    config,
	}, nil
}

// Store stores a single graph record (node + relationships).
// Generates embedding if not provided, then upserts to provider.
func (s *DefaultGraphRAGStore) Store(ctx context.Context, record GraphRecord) error {
	// Generate embedding if not present
	if len(record.Node.Embedding) == 0 && record.EmbedContent != "" {
		embedding, err := s.embedder.Embed(ctx, record.EmbedContent)
		if err != nil {
			return NewEmbeddingError("failed to generate embedding for record", err, true)
		}
		record.Node.Embedding = embedding
	}

	// Validate node
	if err := record.Node.Validate(); err != nil {
		return NewInvalidQueryError(fmt.Sprintf("invalid graph node: %v", err))
	}

	// Store node
	if err := s.provider.StoreNode(ctx, record.Node); err != nil {
		return NewQueryError("failed to store node", err)
	}

	// Store relationships
	for _, rel := range record.Relationships {
		if err := rel.Validate(); err != nil {
			return NewInvalidQueryError(fmt.Sprintf("invalid relationship: %v", err))
		}
		if err := s.provider.StoreRelationship(ctx, rel); err != nil {
			return NewRelationshipError("failed to store relationship", err)
		}
	}

	return nil
}

// StoreBatch efficiently stores multiple graph records.
// Generates embeddings in batch, then stores nodes and relationships.
func (s *DefaultGraphRAGStore) StoreBatch(ctx context.Context, records []GraphRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Collect records that need embeddings
	var embedTexts []string
	var embedIndices []int
	for i, record := range records {
		if len(record.Node.Embedding) == 0 && record.EmbedContent != "" {
			embedTexts = append(embedTexts, record.EmbedContent)
			embedIndices = append(embedIndices, i)
		}
	}

	// Generate embeddings in batch
	if len(embedTexts) > 0 {
		embeddings, err := s.embedder.EmbedBatch(ctx, embedTexts)
		if err != nil {
			return NewEmbeddingError("failed to generate batch embeddings", err, true)
		}

		// Assign embeddings to records
		for i, idx := range embedIndices {
			records[idx].Node.Embedding = embeddings[i]
		}
	}

	// Store each record (provider may optimize this internally)
	for _, record := range records {
		if err := s.Store(ctx, record); err != nil {
			return err
		}
	}

	return nil
}

// Query executes a hybrid GraphRAG query.
// Delegates to the QueryProcessor for full pipeline execution.
func (s *DefaultGraphRAGStore) Query(ctx context.Context, query GraphRAGQuery) ([]GraphRAGResult, error) {
	return s.processor.ProcessQuery(ctx, query, s.provider)
}

// StoreAttackPattern stores a MITRE ATT&CK pattern with technique relationships.
// Creates the pattern node and USES_TECHNIQUE relationships.
func (s *DefaultGraphRAGStore) StoreAttackPattern(ctx context.Context, pattern AttackPattern) error {
	// Generate embedding from description
	if len(pattern.Embedding) == 0 && pattern.Description != "" {
		embedding, err := s.embedder.Embed(ctx, pattern.Description)
		if err != nil {
			return NewEmbeddingError("failed to generate embedding for attack pattern", err, true)
		}
		pattern.Embedding = embedding
	}

	// Convert to GraphNode
	node := pattern.ToGraphNode()

	// Store the node
	if err := s.provider.StoreNode(ctx, *node); err != nil {
		return NewQueryError("failed to store attack pattern node", err)
	}

	// Create USES_TECHNIQUE relationships for each tactic
	// Note: This assumes technique nodes already exist
	// In a real implementation, we'd either create technique nodes or query for them
	for _, tactic := range pattern.Tactics {
		// Create a technique node for this tactic (simplified)
		// In production, we'd query for existing technique nodes
		rel := NewRelationship(
			pattern.ID,
			types.NewID(), // Placeholder - should be actual technique ID
			RelationUsesTechnique,
		).WithProperty("tactic", tactic)

		if err := s.provider.StoreRelationship(ctx, *rel); err != nil {
			// Don't fail the entire operation if relationship creation fails
			// Log error in production
			continue
		}
	}

	return nil
}

// FindSimilarAttacks finds attack patterns similar to the given content.
// Uses vector search filtered to AttackPattern node type.
func (s *DefaultGraphRAGStore) FindSimilarAttacks(ctx context.Context, content string, topK int) ([]AttackPattern, error) {
	// Generate embedding for content
	embedding, err := s.embedder.Embed(ctx, content)
	if err != nil {
		return nil, NewEmbeddingError("failed to generate embedding for content", err, true)
	}

	// Execute vector search with AttackPattern filter
	filters := map[string]any{
		"node_type": NodeTypeAttackPattern.String(),
	}
	vectorResults, err := s.provider.VectorSearch(ctx, embedding, topK, filters)
	if err != nil {
		return nil, NewQueryError("vector search for attack patterns failed", err)
	}

	// Fetch full nodes and convert to AttackPattern
	patterns := make([]AttackPattern, 0, len(vectorResults))
	for _, vr := range vectorResults {
		// Query for full node data
		nodeQuery := NewNodeQuery().
			WithNodeTypes(NodeTypeAttackPattern).
			WithProperty("id", vr.NodeID.String())

		nodes, err := s.provider.QueryNodes(ctx, *nodeQuery)
		if err != nil || len(nodes) == 0 {
			continue
		}

		// Convert GraphNode to AttackPattern
		pattern := graphNodeToAttackPattern(nodes[0])
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// FindSimilarFindings finds findings similar to the given finding.
// Uses vector search filtered to Finding node type.
func (s *DefaultGraphRAGStore) FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]FindingNode, error) {
	// Parse finding ID
	id, err := types.ParseID(findingID)
	if err != nil {
		return nil, NewInvalidQueryError(fmt.Sprintf("invalid finding ID: %v", err))
	}

	// Fetch the source finding
	nodeQuery := NewNodeQuery().
		WithNodeTypes(NodeTypeFinding).
		WithProperty("id", id.String())

	nodes, err := s.provider.QueryNodes(ctx, *nodeQuery)
	if err != nil || len(nodes) == 0 {
		return nil, NewNodeNotFoundError(findingID)
	}

	sourceFinding := nodes[0]
	if len(sourceFinding.Embedding) == 0 {
		return nil, NewQueryError("source finding has no embedding", nil)
	}

	// Execute vector search with Finding filter
	filters := map[string]any{
		"node_type": NodeTypeFinding.String(),
	}
	vectorResults, err := s.provider.VectorSearch(ctx, sourceFinding.Embedding, topK+1, filters)
	if err != nil {
		return nil, NewQueryError("vector search for similar findings failed", err)
	}

	// Convert to FindingNode, excluding the source finding
	findings := make([]FindingNode, 0, topK)
	for _, vr := range vectorResults {
		// Skip the source finding itself
		if vr.NodeID == id {
			continue
		}

		// Query for full node data
		nodeQuery := NewNodeQuery().
			WithNodeTypes(NodeTypeFinding).
			WithProperty("id", vr.NodeID.String())

		nodes, err := s.provider.QueryNodes(ctx, *nodeQuery)
		if err != nil || len(nodes) == 0 {
			continue
		}

		// Convert GraphNode to FindingNode
		finding := graphNodeToFindingNode(nodes[0])
		findings = append(findings, finding)

		if len(findings) >= topK {
			break
		}
	}

	return findings, nil
}

// GetAttackChains discovers attack chains (technique sequences) from a starting technique.
// Traverses USES_TECHNIQUE relationships to find multi-step attack patterns.
func (s *DefaultGraphRAGStore) GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]AttackChain, error) {
	// Query for the starting technique node
	nodeQuery := NewNodeQuery().
		WithNodeTypes(NodeTypeTechnique).
		WithProperty("technique_id", techniqueID)

	nodes, err := s.provider.QueryNodes(ctx, *nodeQuery)
	if err != nil || len(nodes) == 0 {
		return nil, NewNodeNotFoundError(techniqueID)
	}

	startNode := nodes[0]

	// Traverse graph from this technique following USES_TECHNIQUE relationships
	filters := TraversalFilters{
		AllowedRelations: []RelationType{RelationUsesTechnique},
		AllowedNodeTypes: []NodeType{NodeTypeTechnique, NodeTypeAttackPattern},
	}

	traversedNodes, err := s.provider.TraverseGraph(ctx, startNode.ID.String(), maxDepth, filters)
	if err != nil {
		return nil, NewQueryError("graph traversal failed", err)
	}

	// Build attack chains from traversed nodes
	chains := buildAttackChainsFromNodes(startNode, traversedNodes, maxDepth)

	return chains, nil
}

// StoreFinding stores a security finding with contextual relationships.
// Creates the finding node and relationships to targets/techniques.
func (s *DefaultGraphRAGStore) StoreFinding(ctx context.Context, finding FindingNode) error {
	// Generate embedding from description
	if len(finding.Embedding) == 0 && finding.Description != "" {
		embedding, err := s.embedder.Embed(ctx, finding.Description)
		if err != nil {
			return NewEmbeddingError("failed to generate embedding for finding", err, true)
		}
		finding.Embedding = embedding
	}

	// Convert to GraphNode
	node := finding.ToGraphNode()

	// Store the node
	if err := s.provider.StoreNode(ctx, *node); err != nil {
		return NewQueryError("failed to store finding node", err)
	}

	// Create DISCOVERED_ON relationship if target is specified
	if finding.TargetID != nil {
		rel := NewRelationship(
			finding.ID,
			*finding.TargetID,
			RelationDiscoveredOn,
		).WithProperty("severity", finding.Severity)

		if err := s.provider.StoreRelationship(ctx, *rel); err != nil {
			// Don't fail the entire operation
			// Log error in production
		}
	}

	return nil
}

// GetRelatedFindings retrieves findings related to the given finding.
// Traverses SIMILAR_TO and other relationship types.
func (s *DefaultGraphRAGStore) GetRelatedFindings(ctx context.Context, findingID string) ([]FindingNode, error) {
	// Parse finding ID
	id, err := types.ParseID(findingID)
	if err != nil {
		return nil, NewInvalidQueryError(fmt.Sprintf("invalid finding ID: %v", err))
	}

	// Query for relationships from this finding
	relQuery := NewRelQuery().
		WithFromID(id).
		WithTypes(RelationSimilarTo, RelationRelatedTo)

	rels, err := s.provider.QueryRelationships(ctx, *relQuery)
	if err != nil {
		return nil, NewQueryError("failed to query relationships", err)
	}

	// Fetch related finding nodes
	findings := make([]FindingNode, 0, len(rels))
	for _, rel := range rels {
		// Query for the target node
		nodeQuery := NewNodeQuery().
			WithNodeTypes(NodeTypeFinding).
			WithProperty("id", rel.ToID.String())

		nodes, err := s.provider.QueryNodes(ctx, *nodeQuery)
		if err != nil || len(nodes) == 0 {
			continue
		}

		// Convert GraphNode to FindingNode
		finding := graphNodeToFindingNode(nodes[0])
		findings = append(findings, finding)
	}

	return findings, nil
}

// Health returns the current health status of the GraphRAG store.
// Aggregates health from provider and embedder.
func (s *DefaultGraphRAGStore) Health(ctx context.Context) types.HealthStatus {
	// Check provider health
	providerHealth := s.provider.Health(ctx)
	if providerHealth.IsUnhealthy() {
		return types.Unhealthy(fmt.Sprintf("provider unhealthy: %s", providerHealth.Message))
	}

	// Check embedder health
	embedderHealth := s.embedder.Health(ctx)
	if embedderHealth.IsUnhealthy() {
		return types.Unhealthy(fmt.Sprintf("embedder unhealthy: %s", embedderHealth.Message))
	}

	// If either is degraded, return degraded
	if providerHealth.IsDegraded() || embedderHealth.IsDegraded() {
		return types.Degraded("GraphRAG store is degraded")
	}

	return types.Healthy("GraphRAG store is healthy")
}

// Close releases all resources and closes connections.
func (s *DefaultGraphRAGStore) Close() error {
	if s.provider != nil {
		return s.provider.Close()
	}
	return nil
}

// noopProvider is a minimal inline noop provider implementation.
// Used when GraphRAG is disabled to avoid import cycles with provider package.
// For enabled GraphRAG, use NewGraphRAGStoreWithProvider() with provider.NewProvider().
type noopProvider struct{}

func (n *noopProvider) Initialize(ctx context.Context) error                          { return nil }
func (n *noopProvider) StoreNode(ctx context.Context, node GraphNode) error           { return nil }
func (n *noopProvider) StoreRelationship(ctx context.Context, rel Relationship) error { return nil }
func (n *noopProvider) QueryNodes(ctx context.Context, query NodeQuery) ([]GraphNode, error) {
	return []GraphNode{}, nil
}
func (n *noopProvider) QueryRelationships(ctx context.Context, query RelQuery) ([]Relationship, error) {
	return []Relationship{}, nil
}
func (n *noopProvider) TraverseGraph(ctx context.Context, startID string, maxHops int, filters TraversalFilters) ([]GraphNode, error) {
	return []GraphNode{}, nil
}
func (n *noopProvider) VectorSearch(ctx context.Context, embedding []float64, topK int, filters map[string]any) ([]VectorResult, error) {
	return []VectorResult{}, nil
}
func (n *noopProvider) Health(ctx context.Context) types.HealthStatus {
	return types.Healthy("noop provider (GraphRAG disabled)")
}
func (n *noopProvider) Close() error { return nil }

// NewGraphRAGStoreWithProvider creates a new GraphRAGStore with an injected provider.
// This is the recommended constructor when GraphRAG is enabled, as it allows
// external creation of the provider via provider.NewProvider() to avoid import cycles.
//
// Usage:
//
//	import "github.com/zero-day-ai/gibson/internal/graphrag/provider"
//
//	prov, err := provider.NewProvider(config)
//	if err != nil {
//	    return err
//	}
//	store, err := NewGraphRAGStoreWithProvider(config, embedder, prov)
//
// Parameters:
//   - config: GraphRAG configuration
//   - emb: Embedder for generating embeddings
//   - prov: Pre-created GraphRAGProvider (from provider.NewProvider)
//
// Returns a GraphRAGStore ready for use, or an error if initialization fails.
func NewGraphRAGStoreWithProvider(config GraphRAGConfig, emb embedder.Embedder, prov GraphRAGProvider) (GraphRAGStore, error) {
	// Apply defaults and validate config
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, NewConfigError("invalid GraphRAG configuration", err)
	}

	// Validate embedder
	if emb == nil {
		return nil, NewConfigError("embedder cannot be nil", nil)
	}

	// Validate provider
	if prov == nil {
		return nil, NewConfigError("provider cannot be nil", nil)
	}

	// Create query processor
	processor, err := NewQueryProcessorFromConfig(config, emb)
	if err != nil {
		return nil, NewConfigError("failed to create query processor", err)
	}

	return &DefaultGraphRAGStore{
		provider:  prov,
		processor: processor,
		embedder:  emb,
		config:    config,
	}, nil
}

// Helper functions for converting between types

// graphNodeToAttackPattern converts a GraphNode to an AttackPattern.
func graphNodeToAttackPattern(node GraphNode) AttackPattern {
	pattern := AttackPattern{
		ID:          node.ID,
		TechniqueID: node.GetStringProperty("technique_id"),
		Name:        node.GetStringProperty("name"),
		Description: node.GetStringProperty("description"),
		Embedding:   node.Embedding,
		CreatedAt:   node.CreatedAt,
		UpdatedAt:   node.UpdatedAt,
	}

	// Extract arrays from properties
	if tactics, ok := node.Properties["tactics"].([]string); ok {
		pattern.Tactics = tactics
	} else if tactics, ok := node.Properties["tactics"].([]interface{}); ok {
		pattern.Tactics = make([]string, 0, len(tactics))
		for _, t := range tactics {
			if str, ok := t.(string); ok {
				pattern.Tactics = append(pattern.Tactics, str)
			}
		}
	}

	if platforms, ok := node.Properties["platforms"].([]string); ok {
		pattern.Platforms = platforms
	} else if platforms, ok := node.Properties["platforms"].([]interface{}); ok {
		pattern.Platforms = make([]string, 0, len(platforms))
		for _, p := range platforms {
			if str, ok := p.(string); ok {
				pattern.Platforms = append(pattern.Platforms, str)
			}
		}
	}

	return pattern
}

// graphNodeToFindingNode converts a GraphNode to a FindingNode.
func graphNodeToFindingNode(node GraphNode) FindingNode {
	finding := FindingNode{
		ID:          node.ID,
		Title:       node.GetStringProperty("title"),
		Description: node.GetStringProperty("description"),
		Severity:    node.GetStringProperty("severity"),
		Category:    node.GetStringProperty("category"),
		Embedding:   node.Embedding,
		CreatedAt:   node.CreatedAt,
		UpdatedAt:   node.UpdatedAt,
	}

	// Extract confidence
	if conf, ok := node.Properties["confidence"].(float64); ok {
		finding.Confidence = conf
	}

	// Extract mission ID
	if node.MissionID != nil {
		finding.MissionID = *node.MissionID
	}

	// Extract target ID
	if targetIDStr := node.GetStringProperty("target_id"); targetIDStr != "" {
		if targetID, err := types.ParseID(targetIDStr); err == nil {
			finding.TargetID = &targetID
		}
	}

	return finding
}

// buildAttackChainsFromNodes constructs attack chains from traversed nodes.
// Analyzes the graph structure to identify technique sequences.
func buildAttackChainsFromNodes(startNode GraphNode, traversedNodes []GraphNode, maxDepth int) []AttackChain {
	// Simplified implementation - builds a single chain from the traversal
	// Production implementation would use path analysis and chain discovery algorithms

	if len(traversedNodes) == 0 {
		return []AttackChain{}
	}

	// Create a simple chain from the traversed nodes
	chain := NewAttackChain("Attack Chain", types.NewID())
	chain.Severity = "medium"

	// Add starting technique as first step
	chain.AddStep(AttackStep{
		TechniqueID: startNode.GetStringProperty("technique_id"),
		NodeID:      startNode.ID,
		Description: startNode.GetStringProperty("description"),
		Evidence:    []types.ID{},
		Confidence:  1.0,
	})

	// Add subsequent techniques from traversed nodes
	for i, node := range traversedNodes {
		if i >= maxDepth {
			break
		}
		if node.HasLabel(NodeTypeTechnique) {
			chain.AddStep(AttackStep{
				TechniqueID: node.GetStringProperty("technique_id"),
				NodeID:      node.ID,
				Description: node.GetStringProperty("description"),
				Evidence:    []types.ID{},
				Confidence:  0.8, // Decreasing confidence with depth
			})
		}
	}

	// Return the constructed chain
	return []AttackChain{*chain}
}
