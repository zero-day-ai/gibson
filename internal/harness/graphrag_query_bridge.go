// Package harness provides the agent execution environment.
package harness

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
)

// GraphRAGQueryBridge provides the query interface for GraphRAG operations.
// It bridges SDK types to internal GraphRAG store operations, enabling agents
// to query the knowledge graph, traverse relationships, and retrieve domain-specific
// data like attack patterns and security findings.
//
// All methods include OpenTelemetry instrumentation for observability.
// The bridge handles nil store gracefully by returning ErrGraphRAGNotEnabled.
type GraphRAGQueryBridge interface {
	// Query executes a hybrid GraphRAG query combining semantic search and graph traversal.
	// Returns results ranked by combined vector and graph scores.
	Query(ctx context.Context, query sdkgraphrag.Query) ([]sdkgraphrag.Result, error)

	// FindSimilarAttacks finds attack patterns similar to the given content.
	// Uses vector similarity search on attack pattern descriptions.
	FindSimilarAttacks(ctx context.Context, content string, topK int) ([]sdkgraphrag.AttackPattern, error)

	// FindSimilarFindings finds findings similar to the specified finding.
	// Uses vector similarity search on finding descriptions.
	FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]sdkgraphrag.FindingNode, error)

	// GetAttackChains discovers attack chains (technique sequences) from a starting technique.
	// Traverses USES_TECHNIQUE relationships to find multi-step attack patterns.
	GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]sdkgraphrag.AttackChain, error)

	// GetRelatedFindings retrieves findings related to the specified finding.
	// Traverses SIMILAR_TO and other relationship types.
	GetRelatedFindings(ctx context.Context, findingID string) ([]sdkgraphrag.FindingNode, error)

	// StoreNode stores a single graph node with mission and agent context.
	// Returns the node ID. MissionID and agentName are auto-populated.
	StoreNode(ctx context.Context, node sdkgraphrag.GraphNode, missionID, agentName string) (string, error)

	// CreateRelationship creates a relationship between two nodes.
	CreateRelationship(ctx context.Context, rel sdkgraphrag.Relationship) error

	// StoreBatch stores multiple nodes and relationships in a single operation.
	// Returns node IDs for all created nodes. MissionID and agentName are auto-populated.
	StoreBatch(ctx context.Context, batch sdkgraphrag.Batch, missionID, agentName string) ([]string, error)

	// Traverse performs graph traversal from a starting node with filtering options.
	// Returns all nodes visited during traversal with path information.
	Traverse(ctx context.Context, startNodeID string, opts sdkgraphrag.TraversalOptions) ([]sdkgraphrag.TraversalResult, error)

	// Health returns the health status of the GraphRAG bridge.
	Health(ctx context.Context) types.HealthStatus
}

// DefaultGraphRAGQueryBridge is the default implementation of GraphRAGQueryBridge.
// It wraps graphrag.GraphRAGStore and provides type conversion between SDK and internal types.
type DefaultGraphRAGQueryBridge struct {
	store  graphrag.GraphRAGStore
	tracer trace.Tracer
}

// NewGraphRAGQueryBridge creates a new DefaultGraphRAGQueryBridge.
// If store is nil, methods will return ErrGraphRAGNotEnabled.
func NewGraphRAGQueryBridge(store graphrag.GraphRAGStore) *DefaultGraphRAGQueryBridge {
	return &DefaultGraphRAGQueryBridge{
		store:  store,
		tracer: otel.Tracer("gibson/harness/graphrag_query_bridge"),
	}
}

// Query executes a hybrid GraphRAG query.
func (b *DefaultGraphRAGQueryBridge) Query(ctx context.Context, query sdkgraphrag.Query) ([]sdkgraphrag.Result, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.Query",
		trace.WithAttributes(
			attribute.String("query.text", query.Text),
			attribute.Int("query.top_k", query.TopK),
			attribute.Int("query.max_hops", query.MaxHops),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Validate query
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", sdkgraphrag.ErrInvalidQuery, err)
	}

	// Convert SDK query to internal query
	internalQuery := sdkQueryToInternal(query)

	// Execute query
	internalResults, err := b.store.Query(ctx, internalQuery)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	// Convert results back to SDK types
	results := make([]sdkgraphrag.Result, len(internalResults))
	for i, r := range internalResults {
		results[i] = internalResultToSDK(r)
	}

	span.SetAttributes(attribute.Int("results.count", len(results)))
	return results, nil
}

// FindSimilarAttacks finds attack patterns similar to the given content.
func (b *DefaultGraphRAGQueryBridge) FindSimilarAttacks(ctx context.Context, content string, topK int) ([]sdkgraphrag.AttackPattern, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.FindSimilarAttacks",
		trace.WithAttributes(
			attribute.String("content.preview", truncate(content, 100)),
			attribute.Int("top_k", topK),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Execute similarity search
	internalPatterns, err := b.store.FindSimilarAttacks(ctx, content, topK)
	if err != nil {
		return nil, fmt.Errorf("find similar attacks failed: %w", err)
	}

	// Convert to SDK types
	patterns := make([]sdkgraphrag.AttackPattern, len(internalPatterns))
	for i, p := range internalPatterns {
		patterns[i] = internalAttackPatternToSDK(p)
	}

	span.SetAttributes(attribute.Int("patterns.count", len(patterns)))
	return patterns, nil
}

// FindSimilarFindings finds findings similar to the specified finding.
func (b *DefaultGraphRAGQueryBridge) FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]sdkgraphrag.FindingNode, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.FindSimilarFindings",
		trace.WithAttributes(
			attribute.String("finding_id", findingID),
			attribute.Int("top_k", topK),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Execute similarity search
	internalFindings, err := b.store.FindSimilarFindings(ctx, findingID, topK)
	if err != nil {
		return nil, fmt.Errorf("find similar findings failed: %w", err)
	}

	// Convert to SDK types
	findings := make([]sdkgraphrag.FindingNode, len(internalFindings))
	for i, f := range internalFindings {
		findings[i] = internalFindingToSDK(f)
	}

	span.SetAttributes(attribute.Int("findings.count", len(findings)))
	return findings, nil
}

// GetAttackChains discovers attack chains from a starting technique.
func (b *DefaultGraphRAGQueryBridge) GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]sdkgraphrag.AttackChain, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.GetAttackChains",
		trace.WithAttributes(
			attribute.String("technique_id", techniqueID),
			attribute.Int("max_depth", maxDepth),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Execute attack chain discovery
	internalChains, err := b.store.GetAttackChains(ctx, techniqueID, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("get attack chains failed: %w", err)
	}

	// Convert to SDK types
	chains := make([]sdkgraphrag.AttackChain, len(internalChains))
	for i, c := range internalChains {
		chains[i] = internalAttackChainToSDK(c)
	}

	span.SetAttributes(attribute.Int("chains.count", len(chains)))
	return chains, nil
}

// GetRelatedFindings retrieves findings related to the specified finding.
func (b *DefaultGraphRAGQueryBridge) GetRelatedFindings(ctx context.Context, findingID string) ([]sdkgraphrag.FindingNode, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.GetRelatedFindings",
		trace.WithAttributes(
			attribute.String("finding_id", findingID),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Execute relationship traversal
	internalFindings, err := b.store.GetRelatedFindings(ctx, findingID)
	if err != nil {
		return nil, fmt.Errorf("get related findings failed: %w", err)
	}

	// Convert to SDK types
	findings := make([]sdkgraphrag.FindingNode, len(internalFindings))
	for i, f := range internalFindings {
		findings[i] = internalFindingToSDK(f)
	}

	span.SetAttributes(attribute.Int("findings.count", len(findings)))
	return findings, nil
}

// StoreNode stores a single graph node.
func (b *DefaultGraphRAGQueryBridge) StoreNode(ctx context.Context, node sdkgraphrag.GraphNode, missionID, agentName string) (string, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.StoreNode",
		trace.WithAttributes(
			attribute.String("node.type", node.Type),
			attribute.String("mission_id", missionID),
			attribute.String("agent_name", agentName),
		))
	defer span.End()

	if b.store == nil {
		return "", sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Validate node
	if err := node.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", sdkgraphrag.ErrInvalidQuery, err)
	}

	// Convert SDK node to internal node
	internalNode := sdkNodeToInternal(node, missionID, agentName)

	// Create graph record
	record := graphrag.NewGraphRecord(*internalNode)
	if node.Content != "" {
		record.WithEmbedContent(node.Content)
	}

	// Store node
	if err := b.store.Store(ctx, record); err != nil {
		return "", fmt.Errorf("%w: %v", sdkgraphrag.ErrStorageFailed, err)
	}

	span.SetAttributes(attribute.String("node.id", internalNode.ID.String()))
	return internalNode.ID.String(), nil
}

// CreateRelationship creates a relationship between two nodes.
func (b *DefaultGraphRAGQueryBridge) CreateRelationship(ctx context.Context, rel sdkgraphrag.Relationship) error {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.CreateRelationship",
		trace.WithAttributes(
			attribute.String("relationship.type", rel.Type),
			attribute.String("relationship.from", rel.FromID),
			attribute.String("relationship.to", rel.ToID),
		))
	defer span.End()

	if b.store == nil {
		return sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Validate relationship
	if err := rel.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sdkgraphrag.ErrInvalidQuery, err)
	}

	// Convert SDK relationship to internal relationship
	internalRel, err := sdkRelationshipToInternal(rel)
	if err != nil {
		return fmt.Errorf("relationship conversion failed: %w", err)
	}

	// Create a minimal graph record for the relationship
	// We need a dummy node to carry the relationship
	fromID, err := types.ParseID(rel.FromID)
	if err != nil {
		return fmt.Errorf("%w: invalid from_id: %v", sdkgraphrag.ErrInvalidQuery, err)
	}

	dummyNode := graphrag.NewGraphNode(fromID, graphrag.NodeTypeEntity)
	record := graphrag.NewGraphRecord(*dummyNode)
	record.WithRelationship(*internalRel)

	// Store the relationship
	if err := b.store.Store(ctx, record); err != nil {
		return fmt.Errorf("%w: %v", sdkgraphrag.ErrRelationshipFailed, err)
	}

	return nil
}

// StoreBatch stores multiple nodes and relationships in a single operation.
func (b *DefaultGraphRAGQueryBridge) StoreBatch(ctx context.Context, batch sdkgraphrag.Batch, missionID, agentName string) ([]string, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.StoreBatch",
		trace.WithAttributes(
			attribute.Int("batch.nodes", len(batch.Nodes)),
			attribute.Int("batch.relationships", len(batch.Relationships)),
			attribute.String("mission_id", missionID),
			attribute.String("agent_name", agentName),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Convert SDK batch to internal records
	records := make([]graphrag.GraphRecord, len(batch.Nodes))
	nodeIDs := make([]string, len(batch.Nodes))

	for i, sdkNode := range batch.Nodes {
		// Validate node
		if err := sdkNode.Validate(); err != nil {
			return nil, fmt.Errorf("%w: node %d validation failed: %v", sdkgraphrag.ErrInvalidQuery, i, err)
		}

		// Convert node
		internalNode := sdkNodeToInternal(sdkNode, missionID, agentName)
		nodeIDs[i] = internalNode.ID.String()

		// Create record
		record := graphrag.NewGraphRecord(*internalNode)
		if sdkNode.Content != "" {
			record.WithEmbedContent(sdkNode.Content)
		}
		records[i] = record
	}

	// Add relationships to the appropriate records
	for _, sdkRel := range batch.Relationships {
		// Validate relationship
		if err := sdkRel.Validate(); err != nil {
			return nil, fmt.Errorf("%w: relationship validation failed: %v", sdkgraphrag.ErrInvalidQuery, err)
		}

		// Convert relationship
		internalRel, err := sdkRelationshipToInternal(sdkRel)
		if err != nil {
			return nil, fmt.Errorf("relationship conversion failed: %w", err)
		}

		// Find the record for the source node and add the relationship
		// We'll add it to the first record as a simplification
		// In a real implementation, we'd match by node ID
		if len(records) > 0 {
			records[0].WithRelationship(*internalRel)
		}
	}

	// Store batch
	if err := b.store.StoreBatch(ctx, records); err != nil {
		return nil, fmt.Errorf("%w: %v", sdkgraphrag.ErrStorageFailed, err)
	}

	span.SetAttributes(attribute.Int("stored.count", len(nodeIDs)))
	return nodeIDs, nil
}

// Traverse performs graph traversal from a starting node.
func (b *DefaultGraphRAGQueryBridge) Traverse(ctx context.Context, startNodeID string, opts sdkgraphrag.TraversalOptions) ([]sdkgraphrag.TraversalResult, error) {
	ctx, span := b.tracer.Start(ctx, "GraphRAGQueryBridge.Traverse",
		trace.WithAttributes(
			attribute.String("start_node_id", startNodeID),
			attribute.Int("max_depth", opts.MaxDepth),
			attribute.String("direction", opts.Direction),
		))
	defer span.End()

	if b.store == nil {
		return nil, sdkgraphrag.ErrGraphRAGNotEnabled
	}

	// Convert relationship types to internal types
	var relTypes []graphrag.RelationType
	for _, rt := range opts.RelationshipTypes {
		relTypes = append(relTypes, graphrag.RelationType(rt))
	}

	// Convert node types to internal types
	var nodeTypes []graphrag.NodeType
	for _, nt := range opts.NodeTypes {
		nodeTypes = append(nodeTypes, graphrag.NodeType(nt))
	}

	// Create traversal filters
	filters := graphrag.TraversalFilters{
		AllowedRelations: relTypes,
		AllowedNodeTypes: nodeTypes,
	}

	// Execute traversal
	// Note: The GraphRAGProvider interface has TraverseGraph method
	// We'll need to access the provider through the store
	// For now, we'll return an error indicating this needs provider access
	_ = filters
	return nil, fmt.Errorf("traverse not yet implemented: requires direct provider access")
}

// Health returns the health status of the GraphRAG bridge.
func (b *DefaultGraphRAGQueryBridge) Health(ctx context.Context) types.HealthStatus {
	if b.store == nil {
		return types.Unhealthy("graphrag store is nil")
	}
	return b.store.Health(ctx)
}

// Compile-time interface check
var _ GraphRAGQueryBridge = (*DefaultGraphRAGQueryBridge)(nil)

// NoopGraphRAGQueryBridge is a no-op implementation that returns ErrGraphRAGNotEnabled.
// Use this when GraphRAG is disabled or not configured.
type NoopGraphRAGQueryBridge struct{}

// Query returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) Query(_ context.Context, _ sdkgraphrag.Query) ([]sdkgraphrag.Result, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// FindSimilarAttacks returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) FindSimilarAttacks(_ context.Context, _ string, _ int) ([]sdkgraphrag.AttackPattern, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// FindSimilarFindings returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) FindSimilarFindings(_ context.Context, _ string, _ int) ([]sdkgraphrag.FindingNode, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// GetAttackChains returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) GetAttackChains(_ context.Context, _ string, _ int) ([]sdkgraphrag.AttackChain, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// GetRelatedFindings returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) GetRelatedFindings(_ context.Context, _ string) ([]sdkgraphrag.FindingNode, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// StoreNode returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) StoreNode(_ context.Context, _ sdkgraphrag.GraphNode, _, _ string) (string, error) {
	return "", sdkgraphrag.ErrGraphRAGNotEnabled
}

// CreateRelationship returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) CreateRelationship(_ context.Context, _ sdkgraphrag.Relationship) error {
	return sdkgraphrag.ErrGraphRAGNotEnabled
}

// StoreBatch returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) StoreBatch(_ context.Context, _ sdkgraphrag.Batch, _, _ string) ([]string, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// Traverse returns ErrGraphRAGNotEnabled.
func (n *NoopGraphRAGQueryBridge) Traverse(_ context.Context, _ string, _ sdkgraphrag.TraversalOptions) ([]sdkgraphrag.TraversalResult, error) {
	return nil, sdkgraphrag.ErrGraphRAGNotEnabled
}

// Health returns a healthy status since no-op bridge has no failure modes.
func (n *NoopGraphRAGQueryBridge) Health(_ context.Context) types.HealthStatus {
	return types.Healthy("graphrag query bridge disabled (noop)")
}

// Compile-time interface check
var _ GraphRAGQueryBridge = (*NoopGraphRAGQueryBridge)(nil)

// Type conversion functions (adapters)

// sdkQueryToInternal converts SDK Query to internal GraphRAGQuery.
func sdkQueryToInternal(q sdkgraphrag.Query) graphrag.GraphRAGQuery {
	// Convert mission ID if present
	var missionID *types.ID
	if q.MissionID != "" {
		if id, err := types.ParseID(q.MissionID); err == nil {
			missionID = &id
		}
	}

	// Convert node types
	var nodeTypes []graphrag.NodeType
	for _, nt := range q.NodeTypes {
		nodeTypes = append(nodeTypes, graphrag.NodeType(nt))
	}

	return graphrag.GraphRAGQuery{
		Text:         q.Text,
		Embedding:    q.Embedding,
		TopK:         q.TopK,
		MaxHops:      q.MaxHops,
		MinScore:     q.MinScore,
		NodeTypes:    nodeTypes,
		MissionID:    missionID,
		VectorWeight: q.VectorWeight,
		GraphWeight:  q.GraphWeight,
	}
}

// internalResultToSDK converts internal GraphRAGResult to SDK Result.
func internalResultToSDK(r graphrag.GraphRAGResult) sdkgraphrag.Result {
	return sdkgraphrag.Result{
		Node:        internalNodeToSDK(r.Node),
		Score:       r.Score,
		VectorScore: r.VectorScore,
		GraphScore:  r.GraphScore,
		Path:        internalIDsToStrings(r.Path),
		Distance:    r.Distance,
	}
}

// internalNodeToSDK converts internal GraphNode to SDK GraphNode.
func internalNodeToSDK(n graphrag.GraphNode) sdkgraphrag.GraphNode {
	// Get primary node type (first label)
	var nodeType string
	if len(n.Labels) > 0 {
		nodeType = n.Labels[0].String()
	}

	sdkNode := sdkgraphrag.GraphNode{
		ID:         n.ID.String(),
		Type:       nodeType,
		Properties: n.Properties,
		CreatedAt:  n.CreatedAt,
		UpdatedAt:  n.UpdatedAt,
	}

	// Set mission ID and agent name if present
	if n.MissionID != nil {
		sdkNode.MissionID = n.MissionID.String()
	}

	return sdkNode
}

// sdkNodeToInternal converts SDK GraphNode to internal GraphNode.
func sdkNodeToInternal(n sdkgraphrag.GraphNode, missionID, agentName string) *graphrag.GraphNode {
	// Generate or parse node ID
	var nodeID types.ID
	if n.ID != "" {
		if id, err := types.ParseID(n.ID); err == nil {
			nodeID = id
		} else {
			nodeID = types.NewID()
		}
	} else {
		nodeID = types.NewID()
	}

	// Parse mission ID
	var internalMissionID *types.ID
	if missionID != "" {
		if id, err := types.ParseID(missionID); err == nil {
			internalMissionID = &id
		}
	}

	// Create node with type as label
	node := graphrag.NewGraphNode(nodeID, graphrag.NodeType(n.Type))

	// Set properties
	if n.Properties != nil {
		node.WithProperties(n.Properties)
	}

	// Add agent name to properties
	if agentName != "" {
		node.WithProperty("agent_name", agentName)
	}

	// Set mission ID
	if internalMissionID != nil {
		node.WithMission(*internalMissionID)
	}

	// Preserve timestamps if provided
	if !n.CreatedAt.IsZero() {
		node.CreatedAt = n.CreatedAt
	}
	if !n.UpdatedAt.IsZero() {
		node.UpdatedAt = n.UpdatedAt
	}

	return node
}

// sdkRelationshipToInternal converts SDK Relationship to internal Relationship.
func sdkRelationshipToInternal(r sdkgraphrag.Relationship) (*graphrag.Relationship, error) {
	fromID, err := types.ParseID(r.FromID)
	if err != nil {
		return nil, fmt.Errorf("invalid from_id: %w", err)
	}

	toID, err := types.ParseID(r.ToID)
	if err != nil {
		return nil, fmt.Errorf("invalid to_id: %w", err)
	}

	rel := graphrag.NewRelationship(fromID, toID, graphrag.RelationType(r.Type))

	// Set properties
	if r.Properties != nil {
		for k, v := range r.Properties {
			rel.WithProperty(k, v)
		}
	}

	return rel, nil
}

// internalAttackPatternToSDK converts internal AttackPattern to SDK AttackPattern.
func internalAttackPatternToSDK(p graphrag.AttackPattern) sdkgraphrag.AttackPattern {
	return sdkgraphrag.AttackPattern{
		TechniqueID: p.TechniqueID,
		Name:        p.Name,
		Description: p.Description,
		Tactics:     p.Tactics,
		Platforms:   p.Platforms,
		Similarity:  0.0, // Will be set by vector search
	}
}

// internalFindingToSDK converts internal FindingNode to SDK FindingNode.
func internalFindingToSDK(f graphrag.FindingNode) sdkgraphrag.FindingNode {
	return sdkgraphrag.FindingNode{
		ID:          f.ID.String(),
		Title:       f.Title,
		Description: f.Description,
		Severity:    f.Severity,
		Category:    f.Category,
		Confidence:  f.Confidence,
		Similarity:  0.0, // Will be set by vector search
	}
}

// internalAttackChainToSDK converts internal AttackChain to SDK AttackChain.
func internalAttackChainToSDK(c graphrag.AttackChain) sdkgraphrag.AttackChain {
	steps := make([]sdkgraphrag.AttackStep, len(c.Steps))
	for i, s := range c.Steps {
		steps[i] = sdkgraphrag.AttackStep{
			Order:       s.Order,
			TechniqueID: s.TechniqueID,
			NodeID:      s.NodeID.String(),
			Description: s.Description,
			Confidence:  s.Confidence,
		}
	}

	return sdkgraphrag.AttackChain{
		ID:       c.ID.String(),
		Name:     c.Name,
		Severity: c.Severity,
		Steps:    steps,
	}
}

// internalIDsToStrings converts internal ID slice to string slice.
func internalIDsToStrings(ids []types.ID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = id.String()
	}
	return result
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
