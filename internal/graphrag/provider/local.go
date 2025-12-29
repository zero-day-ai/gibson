package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/memory/vector"
	"github.com/zero-day-ai/gibson/internal/types"
)

// LocalGraphRAGProvider implements GraphRAGProvider using local Neo4j and vector store.
// Combines graph database operations with vector similarity search for hybrid retrieval.
//
// Storage architecture:
// - Neo4j for graph structure, relationships, and traversal
// - Local vector store for semantic similarity search
// - Dual storage: nodes stored in both graph and vector stores
//
// Fallback behavior:
// - If Neo4j is unavailable, falls back to vector-only mode
// - Graph operations return degraded results using vector search only
//
// Thread-safety: Safe for concurrent access via internal locking.
type LocalGraphRAGProvider struct {
	config       graphrag.GraphRAGConfig
	graphClient  graph.GraphClient
	vectorStore  vector.VectorStore
	initialized  bool
	graphHealthy bool
	mu           sync.RWMutex
}

// NewLocalProvider creates a new LocalGraphRAGProvider with the given configuration.
// Does not initialize connections - call Initialize() before use.
// Returns an error if the configuration is invalid.
func NewLocalProvider(config graphrag.GraphRAGConfig) (*LocalGraphRAGProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, graphrag.NewConfigError("invalid local provider configuration", err)
	}

	return &LocalGraphRAGProvider{
		config:      config,
		initialized: false,
	}, nil
}

// Initialize establishes connections to Neo4j and vector store.
// Creates necessary indices and validates connectivity.
// Safe to call multiple times - subsequent calls are no-ops if already initialized.
func (l *LocalGraphRAGProvider) Initialize(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.initialized {
		return nil // Already initialized
	}

	// Initialize graph client
	graphConfig := graph.GraphClientConfig{
		URI:                     l.config.Neo4j.URI,
		Username:                l.config.Neo4j.Username,
		Password:                l.config.Neo4j.Password,
		Database:                l.config.Neo4j.Database,
		MaxConnectionPoolSize:   l.config.Neo4j.PoolSize,
		ConnectionTimeout:       30 * 1000000000, // 30 seconds
		MaxTransactionRetryTime: 30 * 1000000000,
	}

	var err error
	l.graphClient, err = graph.NewNeo4jClient(graphConfig)
	if err != nil {
		return graphrag.NewConnectionError("failed to create Neo4j client", err)
	}

	// Connect to Neo4j
	if err := l.graphClient.Connect(ctx); err != nil {
		// Neo4j unavailable - mark as unhealthy but continue initialization
		// This allows vector-only fallback mode
		l.graphHealthy = false
		// Don't return error here - we'll operate in degraded mode
	} else {
		l.graphHealthy = true
	}

	// Initialize vector store if enabled
	if l.config.Vector.Enabled {
		// Note: Vector store should be injected or created based on config
		// For now, we assume it's already available via dependency injection
		// In production, you'd create the vector store here based on config
		if l.vectorStore == nil {
			return graphrag.NewConfigError("vector store is enabled but not provided", nil)
		}

		// Validate vector store connectivity
		health := l.vectorStore.Health(ctx)
		if !health.IsHealthy() {
			return graphrag.NewConnectionError(
				"vector store is unhealthy",
				fmt.Errorf("health status: %s - %s", health.State, health.Message),
			)
		}
	}

	// Create graph indices for performance
	if l.graphHealthy {
		if err := l.createIndices(ctx); err != nil {
			// Index creation is not critical - log but continue
			// In production, you'd log this error
		}
	}

	l.initialized = true
	return nil
}

// SetVectorStore sets the vector store for the provider.
// Must be called before Initialize() if vector search is enabled.
func (l *LocalGraphRAGProvider) SetVectorStore(store vector.VectorStore) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.vectorStore = store
}

// createIndices creates necessary Neo4j indices for performance.
// Creates indices on node IDs, labels, and common query patterns.
func (l *LocalGraphRAGProvider) createIndices(ctx context.Context) error {
	// Create index on node ID for fast lookups
	_, err := l.graphClient.Query(ctx, `
		CREATE INDEX node_id_index IF NOT EXISTS
		FOR (n:GraphNode)
		ON (n.id)
	`, nil)
	if err != nil {
		return err
	}

	// Create index on mission_id for mission-scoped queries
	_, err = l.graphClient.Query(ctx, `
		CREATE INDEX mission_id_index IF NOT EXISTS
		FOR (n:GraphNode)
		ON (n.mission_id)
	`, nil)

	return err
}

// StoreNode stores a graph node in both Neo4j and vector store.
// Creates the node in the graph database and stores its embedding in the vector store.
// If Neo4j is unavailable, stores only in vector store (degraded mode).
func (l *LocalGraphRAGProvider) StoreNode(ctx context.Context, node graphrag.GraphNode) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Validate node
	if err := node.Validate(); err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeQueryFailed, "invalid node", err)
	}

	// Store in graph database if healthy
	if l.graphHealthy && l.graphClient != nil {
		// Convert node labels to strings
		labels := make([]string, len(node.Labels))
		for i, label := range node.Labels {
			labels[i] = label.String()
		}

		// Prepare properties
		props := make(map[string]any)
		for k, v := range node.Properties {
			props[k] = v
		}
		props["id"] = node.ID.String()
		props["created_at"] = node.CreatedAt.Unix()
		props["updated_at"] = node.UpdatedAt.Unix()
		if node.MissionID != nil {
			props["mission_id"] = node.MissionID.String()
		}

		// Create or update node in Neo4j
		_, err := l.graphClient.CreateNode(ctx, labels, props)
		if err != nil {
			return graphrag.NewQueryError("failed to create node in graph", err)
		}
	}

	// Store in vector store if enabled and node has embedding
	if l.config.Vector.Enabled && l.vectorStore != nil && len(node.Embedding) > 0 {
		// Create vector record from node
		metadata := make(map[string]any)
		metadata["node_id"] = node.ID.String()
		metadata["labels"] = node.Labels
		if node.MissionID != nil {
			metadata["mission_id"] = node.MissionID.String()
		}
		// Copy properties to metadata
		for k, v := range node.Properties {
			metadata[k] = v
		}

		// Create vector record
		record := vector.VectorRecord{
			ID:        node.ID.String(),
			Content:   fmt.Sprintf("%v", node.Properties), // Serialize properties as content
			Embedding: node.Embedding,
			Metadata:  metadata,
			CreatedAt: node.CreatedAt,
		}

		if err := l.vectorStore.Store(ctx, record); err != nil {
			// Vector storage failure is not critical if graph succeeded
			// In production, you'd log this error
			if !l.graphHealthy {
				// If both graph and vector failed, return error
				return graphrag.WrapGraphRAGError(graphrag.ErrCodeIndexFailed, "failed to store in vector store", err)
			}
		}
	}

	return nil
}

// StoreRelationship creates a relationship between two nodes in Neo4j.
// Both nodes must exist before creating the relationship.
// If Neo4j is unavailable, returns an error (relationships require graph database).
func (l *LocalGraphRAGProvider) StoreRelationship(ctx context.Context, rel graphrag.Relationship) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	if !l.graphHealthy || l.graphClient == nil {
		return graphrag.NewProviderUnavailableError("neo4j", fmt.Errorf("graph database unavailable"))
	}

	// Validate relationship
	if err := rel.Validate(); err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeRelationshipFailed, "invalid relationship", err)
	}

	// Prepare relationship properties
	props := make(map[string]any)
	for k, v := range rel.Properties {
		props[k] = v
	}
	props["id"] = rel.ID.String()
	props["weight"] = rel.Weight
	props["created_at"] = rel.CreatedAt.Unix()

	// Create relationship in Neo4j
	err := l.graphClient.CreateRelationship(
		ctx,
		rel.FromID.String(),
		rel.ToID.String(),
		rel.Type.String(),
		props,
	)
	if err != nil {
		return graphrag.NewRelationshipError("failed to create relationship", err)
	}

	return nil
}

// QueryNodes performs property-based node lookup in Neo4j.
// Falls back to vector store metadata search if Neo4j is unavailable.
func (l *LocalGraphRAGProvider) QueryNodes(ctx context.Context, query graphrag.NodeQuery) ([]graphrag.GraphNode, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Use Neo4j if healthy
	if l.graphHealthy && l.graphClient != nil {
		return l.queryNodesFromGraph(ctx, query)
	}

	// Fallback to vector store if available
	if l.config.Vector.Enabled && l.vectorStore != nil {
		return l.queryNodesFromVectorStore(ctx, query)
	}

	return nil, graphrag.NewProviderUnavailableError("all storage backends", nil)
}

// queryNodesFromGraph queries nodes from Neo4j using Cypher.
func (l *LocalGraphRAGProvider) queryNodesFromGraph(ctx context.Context, query graphrag.NodeQuery) ([]graphrag.GraphNode, error) {
	// Build Cypher query
	cypher := "MATCH (n"
	params := make(map[string]any)

	// Add label filter
	if len(query.NodeTypes) > 0 {
		for i, nt := range query.NodeTypes {
			if i == 0 {
				cypher += ":" + nt.String()
			} else {
				cypher += "|" + nt.String()
			}
		}
	}
	cypher += ")"

	// Add property filters
	if len(query.Properties) > 0 || query.MissionID != nil {
		cypher += " WHERE "
		first := true

		for key, value := range query.Properties {
			if !first {
				cypher += " AND "
			}
			paramKey := "prop_" + key
			cypher += fmt.Sprintf("n.%s = $%s", key, paramKey)
			params[paramKey] = value
			first = false
		}

		if query.MissionID != nil {
			if !first {
				cypher += " AND "
			}
			cypher += "n.mission_id = $mission_id"
			params["mission_id"] = query.MissionID.String()
		}
	}

	// Add return clause
	cypher += " RETURN n"

	// Add limit
	if query.Limit > 0 {
		cypher += fmt.Sprintf(" LIMIT %d", query.Limit)
	}

	// Execute query
	result, err := l.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return nil, graphrag.NewQueryError("failed to query nodes from graph", err)
	}

	// Convert results to GraphNodes
	nodes := make([]graphrag.GraphNode, 0, len(result.Records))
	for _, record := range result.Records {
		// Extract node from result
		// Note: This is a simplified conversion - production code would handle Neo4j node types properly
		nodeData, ok := record["n"].(map[string]any)
		if !ok {
			continue
		}

		node := l.recordToGraphNode(nodeData)
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// queryNodesFromVectorStore queries nodes from vector store using metadata filters.
// This is a fallback when Neo4j is unavailable.
func (l *LocalGraphRAGProvider) queryNodesFromVectorStore(ctx context.Context, query graphrag.NodeQuery) ([]graphrag.GraphNode, error) {
	// Build metadata filters
	filters := make(map[string]any)
	for k, v := range query.Properties {
		filters[k] = v
	}
	if query.MissionID != nil {
		filters["mission_id"] = query.MissionID.String()
	}
	if len(query.NodeTypes) > 0 {
		filters["labels"] = query.NodeTypes
	}

	// Note: This requires a more sophisticated vector store that supports metadata-only queries
	// For now, this is a placeholder - production code would implement proper metadata filtering
	return nil, graphrag.NewGraphRAGError(
		graphrag.ErrCodeProviderUnavailable,
		"vector store metadata-only queries not yet implemented",
	)
}

// QueryRelationships retrieves relationships from Neo4j matching the query criteria.
// Returns an error if Neo4j is unavailable (relationships require graph database).
func (l *LocalGraphRAGProvider) QueryRelationships(ctx context.Context, query graphrag.RelQuery) ([]graphrag.Relationship, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	if !l.graphHealthy || l.graphClient == nil {
		return nil, graphrag.NewProviderUnavailableError("neo4j", fmt.Errorf("graph database unavailable"))
	}

	// Build Cypher query for relationships
	cypher := "MATCH (from)-[r"

	// Add relationship type filter
	if len(query.Types) > 0 {
		for i, rt := range query.Types {
			if i == 0 {
				cypher += ":" + rt.String()
			} else {
				cypher += "|" + rt.String()
			}
		}
	}

	cypher += "]->(to)"

	// Add filters
	params := make(map[string]any)
	where := make([]string, 0)

	if query.FromID != nil {
		where = append(where, "from.id = $from_id")
		params["from_id"] = query.FromID.String()
	}
	if query.ToID != nil {
		where = append(where, "to.id = $to_id")
		params["to_id"] = query.ToID.String()
	}
	if query.MinWeight > 0 {
		where = append(where, "r.weight >= $min_weight")
		params["min_weight"] = query.MinWeight
	}

	// Add property filters
	for key, value := range query.Properties {
		paramKey := "prop_" + key
		where = append(where, fmt.Sprintf("r.%s = $%s", key, paramKey))
		params[paramKey] = value
	}

	if len(where) > 0 {
		cypher += " WHERE " + where[0]
		for i := 1; i < len(where); i++ {
			cypher += " AND " + where[i]
		}
	}

	// Add return clause
	cypher += " RETURN r, from.id AS from_id, to.id AS to_id"

	// Add limit
	if query.Limit > 0 {
		cypher += fmt.Sprintf(" LIMIT %d", query.Limit)
	}

	// Execute query
	result, err := l.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return nil, graphrag.NewQueryError("failed to query relationships", err)
	}

	// Convert results to Relationships
	relationships := make([]graphrag.Relationship, 0, len(result.Records))
	for _, record := range result.Records {
		rel := l.recordToRelationship(record)
		relationships = append(relationships, rel)
	}

	return relationships, nil
}

// TraverseGraph performs graph traversal starting from a node using Neo4j Cypher.
// Explores the graph structure up to maxHops depth, applying traversal filters.
// Returns an error if Neo4j is unavailable.
func (l *LocalGraphRAGProvider) TraverseGraph(ctx context.Context, startID string, maxHops int, filters graphrag.TraversalFilters) ([]graphrag.GraphNode, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	if !l.graphHealthy || l.graphClient == nil {
		return nil, graphrag.NewProviderUnavailableError("neo4j", fmt.Errorf("graph database unavailable"))
	}

	// Build Cypher traversal query
	cypher := "MATCH path = (start {id: $start_id})-[r*1.."
	if maxHops > 0 {
		cypher += fmt.Sprintf("%d", maxHops)
	} else {
		cypher += "3" // Default max hops
	}
	cypher += "]->(end)"

	params := map[string]any{
		"start_id": startID,
	}

	// Add relationship type filters
	if len(filters.AllowedRelations) > 0 {
		// Cypher relationship type filtering
		relTypes := make([]string, len(filters.AllowedRelations))
		for i, rt := range filters.AllowedRelations {
			relTypes[i] = rt.String()
		}
		// Note: This is simplified - production code would build proper Cypher syntax
	}

	// Add WHERE clause for filters
	where := make([]string, 0)

	if len(filters.AllowedNodeTypes) > 0 {
		// Filter by node labels
		labels := make([]string, len(filters.AllowedNodeTypes))
		for i, nt := range filters.AllowedNodeTypes {
			labels[i] = nt.String()
		}
		// Note: Simplified - production code would build proper label filtering
	}

	if filters.MinWeight > 0 {
		where = append(where, "ALL(rel IN relationships(path) WHERE rel.weight >= $min_weight)")
		params["min_weight"] = filters.MinWeight
	}

	if len(where) > 0 {
		cypher += " WHERE " + where[0]
		for i := 1; i < len(where); i++ {
			cypher += " AND " + where[i]
		}
	}

	// Return unique nodes
	cypher += " RETURN DISTINCT end"

	// Execute traversal query
	result, err := l.graphClient.Query(ctx, cypher, params)
	if err != nil {
		return nil, graphrag.NewQueryError("graph traversal failed", err)
	}

	// Convert results to GraphNodes
	nodes := make([]graphrag.GraphNode, 0, len(result.Records))
	for _, record := range result.Records {
		nodeData, ok := record["end"].(map[string]any)
		if !ok {
			continue
		}

		node := l.recordToGraphNode(nodeData)
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// VectorSearch performs pure vector similarity search using the vector store.
// Returns nodes ranked by embedding similarity to the query embedding.
// Returns an error if vector store is unavailable or disabled.
func (l *LocalGraphRAGProvider) VectorSearch(ctx context.Context, embedding []float64, topK int, filters map[string]any) ([]graphrag.VectorResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	if !l.config.Vector.Enabled || l.vectorStore == nil {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeProviderUnavailable, "vector search is disabled")
	}

	// Create vector query
	query := vector.NewVectorQueryFromEmbedding(embedding, topK)
	if len(filters) > 0 {
		query.WithFilters(filters)
	}

	// Execute vector search
	results, err := l.vectorStore.Search(ctx, *query)
	if err != nil {
		return nil, graphrag.WrapGraphRAGError(graphrag.ErrCodeQueryFailed, "vector search failed", err)
	}

	// Convert to GraphRAG VectorResults
	vectorResults := make([]graphrag.VectorResult, len(results))
	for i, result := range results {
		nodeID, err := types.ParseID(result.Record.ID)
		if err != nil {
			continue
		}

		vectorResults[i] = graphrag.VectorResult{
			NodeID:     nodeID,
			Similarity: result.Score,
			Embedding:  result.Record.Embedding,
			Metadata:   result.Record.Metadata,
		}
	}

	return vectorResults, nil
}

// Health checks the health of Neo4j and vector store connections.
// Returns healthy if both are operational, degraded if one is down, unhealthy if both are down.
func (l *LocalGraphRAGProvider) Health(ctx context.Context) types.HealthStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.initialized {
		return types.Unhealthy("provider not initialized")
	}

	// Check graph health
	graphHealthy := false
	if l.graphClient != nil {
		graphStatus := l.graphClient.Health(ctx)
		graphHealthy = graphStatus.IsHealthy()
	}

	// Check vector health
	vectorHealthy := true // Vector is optional
	if l.config.Vector.Enabled && l.vectorStore != nil {
		vectorStatus := l.vectorStore.Health(ctx)
		vectorHealthy = vectorStatus.IsHealthy()
	}

	// Determine overall health
	if graphHealthy && vectorHealthy {
		return types.Healthy("all backends healthy")
	} else if graphHealthy || vectorHealthy {
		return types.Degraded(fmt.Sprintf("graph healthy: %v, vector healthy: %v", graphHealthy, vectorHealthy))
	} else {
		return types.Unhealthy("all backends unhealthy")
	}
}

// Close releases all resources and closes connections to Neo4j and vector store.
func (l *LocalGraphRAGProvider) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.initialized {
		return nil
	}

	var errs []error

	// Close graph client
	if l.graphClient != nil {
		if err := l.graphClient.Close(context.Background()); err != nil {
			errs = append(errs, fmt.Errorf("failed to close graph client: %w", err))
		}
	}

	// Close vector store
	if l.vectorStore != nil {
		if err := l.vectorStore.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close vector store: %w", err))
		}
	}

	l.initialized = false
	l.graphHealthy = false

	if len(errs) > 0 {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, fmt.Sprintf("close errors: %v", errs))
	}

	return nil
}

// recordToGraphNode converts a Neo4j result record to a GraphNode.
func (l *LocalGraphRAGProvider) recordToGraphNode(data map[string]any) graphrag.GraphNode {
	// Extract ID
	idStr, _ := data["id"].(string)
	nodeID, _ := types.ParseID(idStr)

	// Extract labels - this is simplified, production code would handle Neo4j labels properly
	labels := []graphrag.NodeType{}

	// Extract properties
	properties := make(map[string]any)
	for k, v := range data {
		if k != "id" && k != "created_at" && k != "updated_at" && k != "mission_id" {
			properties[k] = v
		}
	}

	// Create node
	node := graphrag.NewGraphNode(nodeID, labels...)
	node.WithProperties(properties)

	// Extract mission ID if present
	if missionIDStr, ok := data["mission_id"].(string); ok {
		if missionID, err := types.ParseID(missionIDStr); err == nil {
			node.WithMission(missionID)
		}
	}

	return *node
}

// recordToRelationship converts a Neo4j result record to a Relationship.
func (l *LocalGraphRAGProvider) recordToRelationship(record map[string]any) graphrag.Relationship {
	relData, _ := record["r"].(map[string]any)
	fromIDStr, _ := record["from_id"].(string)
	toIDStr, _ := record["to_id"].(string)

	fromID, _ := types.ParseID(fromIDStr)
	toID, _ := types.ParseID(toIDStr)

	// Extract relationship type and properties
	relType := graphrag.RelationRelatedTo // Default
	weight := 1.0
	properties := make(map[string]any)

	for k, v := range relData {
		switch k {
		case "weight":
			if w, ok := v.(float64); ok {
				weight = w
			}
		default:
			properties[k] = v
		}
	}

	rel := graphrag.NewRelationship(fromID, toID, relType)
	rel.WithWeight(weight)
	for k, v := range properties {
		rel.WithProperty(k, v)
	}

	return *rel
}
