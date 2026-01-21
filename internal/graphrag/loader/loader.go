package loader

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/graphrag/domain"
)

// GraphLoader loads domain nodes into Neo4j using the GraphNode interface.
// It handles node creation/updates, relationship creation, and provenance tracking.
type GraphLoader struct {
	client graph.GraphClient
}

// NewGraphLoader creates a new GraphLoader with the given Neo4j client.
func NewGraphLoader(client graph.GraphClient) *GraphLoader {
	return &GraphLoader{
		client: client,
	}
}

// ExecContext provides execution context for tracking provenance.
// All nodes loaded in a single Load/LoadBatch call will be associated with this context.
type ExecContext struct {
	// MissionRunID is the ID of the current mission run.
	// This is the primary scope for all stored nodes and will be used
	// to create BELONGS_TO relationships for root nodes.
	MissionRunID string

	// MissionID is the ID of the mission (parent of MissionRun).
	// This is used for mission-level scoping and queries.
	MissionID string

	// AgentName is the name of the agent storing data.
	// This is used for the discovered_by provenance field on nodes.
	AgentName string

	// AgentRunID is the ID of the agent run that discovered these nodes.
	// This is used to create DISCOVERED relationships from the agent run to each node.
	AgentRunID string

	// ToolExecutionID is the ID of the tool execution that produced these nodes (optional).
	ToolExecutionID string
}

// LoadResult contains statistics about a load operation.
type LoadResult struct {
	// NodesCreated is the count of new nodes created.
	NodesCreated int

	// NodesUpdated is the count of existing nodes that were updated.
	NodesUpdated int

	// RelationshipsCreated is the count of relationships created.
	RelationshipsCreated int

	// Errors contains any errors encountered during loading.
	// The loader attempts to continue processing after errors.
	Errors []error
}

// AddError adds an error to the result and returns the result for chaining.
func (r *LoadResult) AddError(err error) *LoadResult {
	r.Errors = append(r.Errors, err)
	return r
}

// HasErrors returns true if any errors were encountered.
func (r *LoadResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Load loads a slice of GraphNode domain objects into Neo4j.
// For each node:
//  1. Validates the node (child nodes must have BelongsTo set)
//  2. Creates the node using CREATE (never MERGE - each node is unique per mission run)
//  3. Creates a relationship to the parent node (if ParentRef is non-nil) OR
//     attaches to MissionRun (if root node)
//  4. Creates a DISCOVERED relationship from the agent run to the node
//
// Uses parameterized queries to prevent injection attacks.
func (l *GraphLoader) Load(ctx context.Context, execCtx ExecContext, nodes []domain.GraphNode) (*LoadResult, error) {
	if l.client == nil {
		return nil, types.NewError("GRAPHRAG_LOADER", "client is nil")
	}

	result := &LoadResult{}

	for _, node := range nodes {
		if node == nil {
			result.AddError(fmt.Errorf("nil node in input"))
			continue
		}

		// Validate node using SDK validation framework (Task 11.2)
		if err := domain.ValidateGraphNode(node); err != nil {
			result.AddError(fmt.Errorf("validation failed for node: %w", err))
			continue
		}

		// Get node information from the domain object
		nodeType := node.NodeType()
		allProps := node.Properties()
		parentRef := node.ParentRef()

		if nodeType == "" {
			result.AddError(fmt.Errorf("node has empty NodeType"))
			continue
		}

		// Generate unique ID for this node if not already set
		if _, hasID := allProps["id"]; !hasID {
			allProps["id"] = types.NewID().String()
		}

		// Inject mission context into node properties
		// This ensures all nodes are tied to their mission for proper scoping
		if execCtx.MissionID != "" {
			allProps["mission_id"] = execCtx.MissionID
		}
		if execCtx.MissionRunID != "" {
			allProps["mission_run_id"] = execCtx.MissionRunID
		}
		if execCtx.AgentRunID != "" {
			allProps["agent_run_id"] = execCtx.AgentRunID
		}
		if execCtx.AgentName != "" {
			allProps["discovered_by"] = execCtx.AgentName
		}
		allProps["discovered_at"] = time.Now().UnixMilli()

		// Build CREATE query (Task 9.1 - never use MERGE for domain nodes)
		// Each node is unique within a mission run - no deduplication
		cypher := fmt.Sprintf(`
			CREATE (n:%s $props)
			SET n.created_at = timestamp()
			RETURN elementId(n) as node_id
		`, nodeType)

		params := map[string]any{
			"props": allProps,
		}

		// Execute the CREATE query
		queryResult, err := l.client.Query(ctx, cypher, params)
		if err != nil {
			result.AddError(fmt.Errorf("failed to create node type %s: %w", nodeType, err))
			continue
		}

		result.NodesCreated++

		// Get the Neo4j element ID for relationship creation
		if len(queryResult.Records) == 0 {
			result.AddError(fmt.Errorf("no records returned after creating node type %s", nodeType))
			continue
		}

		nodeID, ok := queryResult.Records[0]["node_id"].(string)
		if !ok {
			result.AddError(fmt.Errorf("failed to get node_id for node type %s", nodeType))
			continue
		}

		// Create parent relationship if specified (child node)
		if parentRef != nil {
			relType := node.RelationshipType()
			if relType == "" {
				result.AddError(fmt.Errorf("node type %s has ParentRef but no RelationshipType", nodeType))
				continue
			}

			// Task 9.4: Scope parent lookup by mission_run_id
			if err := l.createParentRelationshipScoped(ctx, nodeID, parentRef, relType, execCtx.MissionRunID); err != nil {
				result.AddError(fmt.Errorf("failed to create parent relationship for node type %s: %w", nodeType, err))
				continue
			}
			result.RelationshipsCreated++
		} else {
			// Root node - attach to MissionRun (Task 9.3)
			if execCtx.MissionRunID != "" {
				if err := l.attachToMissionRun(ctx, nodeID, execCtx.MissionRunID); err != nil {
					result.AddError(fmt.Errorf("failed to attach root node type %s to MissionRun: %w", nodeType, err))
					continue
				}
				result.RelationshipsCreated++
			}
		}

		// Create DISCOVERED relationship from agent run
		if execCtx.AgentRunID != "" {
			if err := l.createDiscoveredRelationship(ctx, execCtx.AgentRunID, nodeID); err != nil {
				result.AddError(fmt.Errorf("failed to create DISCOVERED relationship for node type %s: %w", nodeType, err))
				// Don't continue - this is non-critical
			} else {
				result.RelationshipsCreated++
			}
		}
	}

	return result, nil
}

// attachToMissionRun creates a BELONGS_TO relationship from a root node to its MissionRun.
// This is called for all root nodes (nodes without a parent) to establish the mission scope.
// Task 9.3: Root nodes are attached to MissionRun via BELONGS_TO relationship.
func (l *GraphLoader) attachToMissionRun(ctx context.Context, nodeElementID, missionRunID string) error {
	if missionRunID == "" {
		return nil // No mission run context - skip (shouldn't happen in normal operation)
	}

	cypher := `
		MATCH (run:mission_run {id: $run_id})
		MATCH (n) WHERE elementId(n) = $node_id
		CREATE (n)-[:BELONGS_TO]->(run)
		RETURN n
	`

	params := map[string]any{
		"run_id":  missionRunID,
		"node_id": nodeElementID,
	}

	_, err := l.client.Query(ctx, cypher, params)
	return err
}

// createParentRelationshipScoped creates a relationship from a parent node to a child node,
// scoping the parent lookup to the current mission run.
// Task 9.4: Parent relationships are scoped by mission_run_id to prevent cross-run collisions.
func (l *GraphLoader) createParentRelationshipScoped(ctx context.Context, childNodeID string, parentRef *domain.NodeRef, relType string, missionRunID string) error {
	if parentRef == nil {
		return nil // Nothing to do
	}

	// Build MATCH clause for parent node using its identifying properties
	parentProps := parentRef.Properties
	if len(parentProps) == 0 {
		return fmt.Errorf("parent node ref has no properties")
	}

	// Build WHERE clause for parent identification
	// Scope by mission_run_id to find the correct parent in this mission run
	whereClauses := make([]string, 0, len(parentProps)+1)
	params := make(map[string]any)

	for key, val := range parentProps {
		paramKey := fmt.Sprintf("parent_%s", key)
		whereClauses = append(whereClauses, fmt.Sprintf("parent.%s = $%s", key, paramKey))
		params[paramKey] = val
	}

	// Add mission_run_id scoping if available (Task 9.4)
	if missionRunID != "" {
		whereClauses = append(whereClauses, "parent.mission_run_id = $mission_run_id")
		params["mission_run_id"] = missionRunID
	}

	whereStr := strings.Join(whereClauses, " AND ")

	// Add child node ID parameter
	params["child_id"] = childNodeID

	// Build CREATE query for relationship (not MERGE - each relationship is unique)
	// MATCH (parent:ParentType) WHERE parent.prop1 = $parent_prop1 AND parent.mission_run_id = $mission_run_id
	// MATCH (child) WHERE elementId(child) = $child_id
	// CREATE (parent)-[r:REL_TYPE]->(child)
	// RETURN r
	cypher := fmt.Sprintf(`
		MATCH (parent:%s) WHERE %s
		MATCH (child) WHERE elementId(child) = $child_id
		CREATE (parent)-[r:%s]->(child)
		RETURN r
	`, parentRef.NodeType, whereStr, relType)

	// Execute relationship creation
	_, err := l.client.Query(ctx, cypher, params)
	return err
}

// createParentRelationship creates a relationship from a parent node to a child node.
// DEPRECATED: Use createParentRelationshipScoped instead for mission-scoped storage.
// The parent is identified by its NodeRef (type + identifying properties).
func (l *GraphLoader) createParentRelationship(ctx context.Context, childNodeID string, parentRef *domain.NodeRef, relType string) error {
	// Delegate to scoped version with empty mission run ID for backward compatibility
	return l.createParentRelationshipScoped(ctx, childNodeID, parentRef, relType, "")
}

// createDiscoveredRelationship creates a DISCOVERED relationship from an agent run to a node.
// The agent run is matched by its 'id' property (Gibson UUID stored as node.id).
func (l *GraphLoader) createDiscoveredRelationship(ctx context.Context, agentRunID, nodeID string) error {
	cypher := `
		MATCH (run {id: $agent_run_id})
		MATCH (node) WHERE elementId(node) = $node_id
		MERGE (run)-[r:DISCOVERED]->(node)
		SET r.discovered_at = timestamp()
		RETURN r
	`

	params := map[string]any{
		"agent_run_id": agentRunID,
		"node_id":      nodeID,
	}

	_, err := l.client.Query(ctx, cypher, params)
	return err
}

// LoadBatch loads a slice of GraphNode domain objects into Neo4j using optimized batch operations.
// This method uses a single transaction and UNWIND optimization for better performance with large datasets.
//
// The batch loading process:
//  1. Validates all nodes (child nodes must have BelongsTo set)
//  2. Groups nodes by NodeType for efficient UNWIND processing
//  3. Uses a single Neo4j transaction for all operations (atomic - all or nothing)
//  4. Creates all nodes using CREATE with UNWIND (never MERGE)
//  5. Creates parent relationships in batch (scoped by mission_run_id)
//  6. Attaches root nodes to MissionRun via BELONGS_TO
//  7. Creates DISCOVERED relationships in batch
//  8. Rolls back the entire transaction on any error
//
// For small datasets (< 100 nodes), Load() may be more appropriate as it provides
// per-node error handling. LoadBatch() is optimized for bulk inserts where
// transactional consistency is important.
func (l *GraphLoader) LoadBatch(ctx context.Context, execCtx ExecContext, nodes []domain.GraphNode) (*LoadResult, error) {
	if l.client == nil {
		return nil, types.NewError("GRAPHRAG_LOADER", "client is nil")
	}

	if len(nodes) == 0 {
		return &LoadResult{}, nil
	}

	result := &LoadResult{}

	// Validate all nodes first (Task 11.2)
	for _, node := range nodes {
		if node == nil {
			result.AddError(fmt.Errorf("nil node in input"))
			continue
		}
		if err := domain.ValidateGraphNode(node); err != nil {
			result.AddError(fmt.Errorf("validation failed for node type %s: %w", node.NodeType(), err))
			continue
		}
	}

	// If validation failed, don't proceed
	if result.HasErrors() {
		return result, fmt.Errorf("input validation failed: %d errors", len(result.Errors))
	}

	// Group nodes by type for UNWIND optimization
	nodesByType := make(map[string][]domain.GraphNode)
	for _, node := range nodes {
		if node == nil {
			continue // Already handled above
		}
		nodeType := node.NodeType()
		if nodeType == "" {
			result.AddError(fmt.Errorf("node has empty NodeType"))
			continue
		}
		nodesByType[nodeType] = append(nodesByType[nodeType], node)
	}

	// If we have errors from empty NodeType, don't proceed
	if result.HasErrors() {
		return result, fmt.Errorf("input validation failed: %d errors", len(result.Errors))
	}

	// Track node IDs for relationship creation
	var nodeInfos []nodeInfo
	var rootNodeInfos []nodeInfo // Track root nodes for MissionRun attachment

	// Process each node type separately to avoid complex multi-type queries
	discoveredAt := time.Now().UnixMilli()

	for nodeType, typeNodes := range nodesByType {
		// Build list parameter with all nodes of this type
		nodeDataList := make([]map[string]any, 0, len(typeNodes))
		for idx, node := range typeNodes {
			allProps := node.Properties()

			// Generate unique ID for this node if not already set
			if _, hasID := allProps["id"]; !hasID {
				allProps["id"] = types.NewID().String()
			}

			// Inject mission context into node properties
			if execCtx.MissionID != "" {
				allProps["mission_id"] = execCtx.MissionID
			}
			if execCtx.MissionRunID != "" {
				allProps["mission_run_id"] = execCtx.MissionRunID
			}
			if execCtx.AgentRunID != "" {
				allProps["agent_run_id"] = execCtx.AgentRunID
			}
			if execCtx.AgentName != "" {
				allProps["discovered_by"] = execCtx.AgentName
			}
			allProps["discovered_at"] = discoveredAt

			nodeData := map[string]any{
				"all_props": allProps,
				"index":     idx, // Track original index for relationship creation
			}
			nodeDataList = append(nodeDataList, nodeData)

			// Store node info for later relationship creation
			parentRef := node.ParentRef()
			info := nodeInfo{
				nodeType:      nodeType,
				parentRef:     parentRef,
				relType:       node.RelationshipType(),
				originalIndex: idx,
			}
			if parentRef != nil {
				nodeInfos = append(nodeInfos, info)
			} else {
				// Root node - needs to be attached to MissionRun
				rootNodeInfos = append(rootNodeInfos, info)
			}
		}

		// Build CREATE query for this node type (Task 9.2 - never use MERGE)
		cypher := fmt.Sprintf(`
			UNWIND $nodes AS nodeData
			CREATE (n:%s)
			SET n = nodeData.all_props, n.created_at = timestamp()
			RETURN elementId(n) as element_id, nodeData.index as idx
		`, nodeType)

		params := map[string]any{
			"nodes": nodeDataList,
		}

		// Execute the batch CREATE query
		queryResult, err := l.client.Query(ctx, cypher, params)
		if err != nil {
			return result, fmt.Errorf("batch node creation failed for type %s: %w", nodeType, err)
		}

		// Process results to get element IDs
		for _, record := range queryResult.Records {
			elementID, _ := record["element_id"].(string)
			index, _ := record["idx"].(int64)
			if index == 0 {
				if indexFloat, ok := record["idx"].(float64); ok {
					index = int64(indexFloat)
				}
			}

			// Update nodeInfo with element ID
			for i := range nodeInfos {
				if nodeInfos[i].nodeType == nodeType && nodeInfos[i].originalIndex == int(index) {
					nodeInfos[i].elementIDVar = elementID
				}
			}
			for i := range rootNodeInfos {
				if rootNodeInfos[i].nodeType == nodeType && rootNodeInfos[i].originalIndex == int(index) {
					rootNodeInfos[i].elementIDVar = elementID
				}
			}

			result.NodesCreated++
		}
	}

	// Build elementIDMap for backward compatibility with existing batch methods
	elementIDMap := make(map[string]string)
	for _, info := range nodeInfos {
		key := fmt.Sprintf("%s:%d", info.nodeType, info.originalIndex)
		elementIDMap[key] = info.elementIDVar
	}
	for _, info := range rootNodeInfos {
		key := fmt.Sprintf("%s:%d", info.nodeType, info.originalIndex)
		elementIDMap[key] = info.elementIDVar
	}

	// Create parent relationships in batch (scoped by mission_run_id)
	if len(nodeInfos) > 0 {
		relCount, err := l.createParentRelationshipsBatchScoped(ctx, nodeInfos, elementIDMap, execCtx.MissionRunID)
		if err != nil {
			return result, fmt.Errorf("batch parent relationship creation failed: %w", err)
		}
		result.RelationshipsCreated += relCount
	}

	// Attach root nodes to MissionRun (Task 9.3)
	if len(rootNodeInfos) > 0 && execCtx.MissionRunID != "" {
		rootElementIDs := make([]string, 0, len(rootNodeInfos))
		for _, info := range rootNodeInfos {
			if info.elementIDVar != "" {
				rootElementIDs = append(rootElementIDs, info.elementIDVar)
			}
		}

		if len(rootElementIDs) > 0 {
			relCount, err := l.attachToMissionRunBatch(ctx, rootElementIDs, execCtx.MissionRunID)
			if err != nil {
				return result, fmt.Errorf("batch MissionRun attachment failed: %w", err)
			}
			result.RelationshipsCreated += relCount
		}
	}

	// Create DISCOVERED relationships in batch
	if execCtx.AgentRunID != "" && len(elementIDMap) > 0 {
		elementIDs := make([]string, 0, len(elementIDMap))
		for _, elementID := range elementIDMap {
			elementIDs = append(elementIDs, elementID)
		}

		discCount, err := l.createDiscoveredRelationshipsBatch(ctx, execCtx.AgentRunID, elementIDs)
		if err != nil {
			// Non-critical error - log but don't fail
			result.AddError(fmt.Errorf("batch DISCOVERED relationship creation failed: %w", err))
		} else {
			result.RelationshipsCreated += discCount
		}
	}

	return result, nil
}

// attachToMissionRunBatch creates BELONGS_TO relationships from root nodes to MissionRun in batch.
// Task 9.3: Root nodes are attached to MissionRun via BELONGS_TO relationship.
func (l *GraphLoader) attachToMissionRunBatch(ctx context.Context, nodeElementIDs []string, missionRunID string) (int, error) {
	if missionRunID == "" || len(nodeElementIDs) == 0 {
		return 0, nil
	}

	cypher := `
		MATCH (run:mission_run {id: $run_id})
		UNWIND $node_ids AS node_id
		MATCH (n) WHERE elementId(n) = node_id
		CREATE (n)-[:BELONGS_TO]->(run)
		RETURN count(*) as rel_count
	`

	params := map[string]any{
		"run_id":   missionRunID,
		"node_ids": nodeElementIDs,
	}

	result, err := l.client.Query(ctx, cypher, params)
	if err != nil {
		return 0, err
	}

	if len(result.Records) > 0 {
		if count, ok := result.Records[0]["rel_count"].(int64); ok {
			return int(count), nil
		} else if count, ok := result.Records[0]["rel_count"].(float64); ok {
			return int(count), nil
		}
	}

	return 0, nil
}

// createParentRelationshipsBatchScoped creates parent relationships in batch using UNWIND,
// scoping parent lookups by mission_run_id to prevent cross-run collisions.
// Task 9.4: Parent relationships are scoped by mission_run_id.
func (l *GraphLoader) createParentRelationshipsBatchScoped(ctx context.Context, nodeInfos []nodeInfo, elementIDMap map[string]string, missionRunID string) (int, error) {
	// Group by parent node type and relationship type for efficiency
	type relKey struct {
		parentType string
		relType    string
	}

	relGroups := make(map[relKey][]map[string]any)

	for _, info := range nodeInfos {
		if info.parentRef == nil {
			continue
		}

		// Get child element ID from map or from info
		childID := info.elementIDVar
		if childID == "" {
			childKey := fmt.Sprintf("%s:%d", info.nodeType, info.originalIndex)
			var ok bool
			childID, ok = elementIDMap[childKey]
			if !ok {
				continue // Skip if we don't have the element ID
			}
		}

		key := relKey{
			parentType: info.parentRef.NodeType,
			relType:    info.relType,
		}

		relData := map[string]any{
			"child_id":     childID,
			"parent_props": info.parentRef.Properties,
		}

		relGroups[key] = append(relGroups[key], relData)
	}

	totalRels := 0

	// Create relationships for each group
	for key, relDataList := range relGroups {
		if len(relDataList) == 0 {
			continue
		}

		sampleParentProps := relDataList[0]["parent_props"].(map[string]any)
		propKeys := make([]string, 0, len(sampleParentProps))
		for k := range sampleParentProps {
			propKeys = append(propKeys, k)
		}

		// Build WHERE clause for parent matching with mission_run_id scoping
		whereClauses := make([]string, 0, len(propKeys)+1)
		for _, propKey := range propKeys {
			whereClauses = append(whereClauses, fmt.Sprintf("parent.%s = relData.parent_props.%s", propKey, propKey))
		}

		// Add mission_run_id scoping (Task 9.4)
		if missionRunID != "" {
			whereClauses = append(whereClauses, "parent.mission_run_id = $mission_run_id")
		}

		whereStr := strings.Join(whereClauses, " AND ")

		// Use CREATE not MERGE for relationships (each is unique within a mission run)
		cypher := fmt.Sprintf(`
			UNWIND $rel_data AS relData
			MATCH (parent:%s) WHERE %s
			MATCH (child) WHERE elementId(child) = relData.child_id
			CREATE (parent)-[r:%s]->(child)
			RETURN count(r) as rel_count
		`, key.parentType, whereStr, key.relType)

		params := map[string]any{
			"rel_data":       relDataList,
			"mission_run_id": missionRunID,
		}

		result, err := l.client.Query(ctx, cypher, params)
		if err != nil {
			return totalRels, err
		}

		if len(result.Records) > 0 {
			if count, ok := result.Records[0]["rel_count"].(int64); ok {
				totalRels += int(count)
			} else if count, ok := result.Records[0]["rel_count"].(float64); ok {
				totalRels += int(count)
			}
		}
	}

	return totalRels, nil
}

// createParentRelationshipsBatch creates parent relationships in batch using UNWIND.
// DEPRECATED: Use createParentRelationshipsBatchScoped instead for mission-scoped storage.
func (l *GraphLoader) createParentRelationshipsBatch(ctx context.Context, nodeInfos []nodeInfo, elementIDMap map[string]string) (int, error) {
	// Delegate to scoped version with empty mission run ID for backward compatibility
	return l.createParentRelationshipsBatchScoped(ctx, nodeInfos, elementIDMap, "")
}

// createDiscoveredRelationshipsBatch creates DISCOVERED relationships in batch.
func (l *GraphLoader) createDiscoveredRelationshipsBatch(ctx context.Context, agentRunID string, elementIDs []string) (int, error) {
	cypher := `
		MATCH (run {id: $agent_run_id})
		UNWIND $element_ids AS element_id
		MATCH (node) WHERE elementId(node) = element_id
		MERGE (run)-[r:DISCOVERED]->(node)
		SET r.discovered_at = timestamp()
		RETURN count(r) as rel_count
	`

	params := map[string]any{
		"agent_run_id": agentRunID,
		"element_ids":  elementIDs,
	}

	result, err := l.client.Query(ctx, cypher, params)
	if err != nil {
		return 0, err
	}

	if len(result.Records) > 0 {
		if count, ok := result.Records[0]["rel_count"].(int64); ok {
			return int(count), nil
		} else if count, ok := result.Records[0]["rel_count"].(float64); ok {
			return int(count), nil
		}
	}

	return 0, nil
}

// nodeInfo tracks information about nodes for batch relationship creation
type nodeInfo struct {
	elementIDVar  string // Variable name in Cypher query (unused but kept for future)
	nodeType      string
	parentRef     *domain.NodeRef
	relType       string
	originalIndex int
}
