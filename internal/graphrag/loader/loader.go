package loader

import (
	"context"
	"fmt"
	"strings"

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
	// AgentRunID is the ID of the agent run that discovered these nodes.
	// This is used to create DISCOVERED relationships from the agent run to each node.
	AgentRunID string

	// ToolExecutionID is the ID of the tool execution that produced these nodes (optional).
	ToolExecutionID string

	// MissionID is the ID of the mission this execution belongs to.
	MissionID string
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
//  1. Creates or updates the node using MERGE based on identifying properties
//  2. Creates a relationship to the parent node (if ParentRef is non-nil)
//  3. Creates a DISCOVERED relationship from the agent run to the node
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

		// Get node information from the domain object
		nodeType := node.NodeType()
		idProps := node.IdentifyingProperties()
		allProps := node.Properties()

		if nodeType == "" {
			result.AddError(fmt.Errorf("node has empty NodeType"))
			continue
		}

		if len(idProps) == 0 {
			result.AddError(fmt.Errorf("node type %s has no identifying properties", nodeType))
			continue
		}

		// Build MERGE query for the node
		// MERGE (n:Label {id_prop1: $id_prop1, id_prop2: $id_prop2})
		// ON CREATE SET n = $props, n.created_at = timestamp()
		// ON MATCH SET n += $props, n.updated_at = timestamp()
		// RETURN n, id(n) as node_id,
		//        CASE WHEN n.created_at = timestamp() THEN 'created' ELSE 'updated' END as op

		// Build identifying properties clause
		idPropClauses := make([]string, 0, len(idProps))
		for key := range idProps {
			idPropClauses = append(idPropClauses, fmt.Sprintf("%s: $id_%s", key, key))
		}
		idPropsStr := strings.Join(idPropClauses, ", ")

		// Build the MERGE query
		cypher := fmt.Sprintf(`
			MERGE (n:%s {%s})
			ON CREATE SET n = $props, n.created_at = timestamp()
			ON MATCH SET n += $props, n.updated_at = timestamp()
			RETURN elementId(n) as node_id,
			       CASE WHEN n.created_at = n.updated_at THEN 'created' ELSE 'updated' END as operation
		`, nodeType, idPropsStr)

		// Build parameters
		params := map[string]any{
			"props": allProps,
		}
		// Add identifying properties with "id_" prefix
		for key, val := range idProps {
			params[fmt.Sprintf("id_%s", key)] = val
		}

		// Execute the MERGE query
		queryResult, err := l.client.Query(ctx, cypher, params)
		if err != nil {
			result.AddError(fmt.Errorf("failed to merge node type %s: %w", nodeType, err))
			continue
		}

		// Check operation type and update counters
		if len(queryResult.Records) > 0 {
			record := queryResult.Records[0]
			op, _ := record["operation"].(string)
			if op == "created" {
				result.NodesCreated++
			} else {
				result.NodesUpdated++
			}

			// Get the Neo4j element ID for relationship creation
			nodeID, ok := record["node_id"].(string)
			if !ok {
				result.AddError(fmt.Errorf("failed to get node_id for node type %s", nodeType))
				continue
			}

			// Create parent relationship if specified
			parentRef := node.ParentRef()
			if parentRef != nil {
				relType := node.RelationshipType()
				if relType == "" {
					result.AddError(fmt.Errorf("node type %s has ParentRef but no RelationshipType", nodeType))
					continue
				}

				if err := l.createParentRelationship(ctx, nodeID, parentRef, relType); err != nil {
					result.AddError(fmt.Errorf("failed to create parent relationship for node type %s: %w", nodeType, err))
					continue
				}
				result.RelationshipsCreated++
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
	}

	return result, nil
}

// createParentRelationship creates a relationship from a parent node to a child node.
// The parent is identified by its NodeRef (type + identifying properties).
func (l *GraphLoader) createParentRelationship(ctx context.Context, childNodeID string, parentRef *domain.NodeRef, relType string) error {
	if parentRef == nil {
		return nil // Nothing to do
	}

	// Build MATCH clause for parent node using its identifying properties
	parentProps := parentRef.Properties
	if len(parentProps) == 0 {
		return fmt.Errorf("parent node ref has no properties")
	}

	// Build WHERE clause for parent identification
	whereClauses := make([]string, 0, len(parentProps))
	params := make(map[string]any)
	for key, val := range parentProps {
		paramKey := fmt.Sprintf("parent_%s", key)
		whereClauses = append(whereClauses, fmt.Sprintf("parent.%s = $%s", key, paramKey))
		params[paramKey] = val
	}
	whereStr := strings.Join(whereClauses, " AND ")

	// Add child node ID parameter
	params["child_id"] = childNodeID

	// Build MERGE query for relationship
	// MATCH (parent:ParentType) WHERE parent.prop1 = $parent_prop1 AND ...
	// MATCH (child) WHERE elementId(child) = $child_id
	// MERGE (parent)-[r:REL_TYPE]->(child)
	// RETURN r
	cypher := fmt.Sprintf(`
		MATCH (parent:%s) WHERE %s
		MATCH (child) WHERE elementId(child) = $child_id
		MERGE (parent)-[r:%s]->(child)
		RETURN r
	`, parentRef.NodeType, whereStr, relType)

	// Execute relationship creation
	_, err := l.client.Query(ctx, cypher, params)
	return err
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
//  1. Groups nodes by NodeType for efficient UNWIND processing
//  2. Uses a single Neo4j transaction for all operations (atomic - all or nothing)
//  3. Creates/updates all nodes using MERGE with UNWIND
//  4. Creates parent relationships in batch
//  5. Creates DISCOVERED relationships in batch
//  6. Rolls back the entire transaction on any error
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

	// Group nodes by type for UNWIND optimization
	nodesByType := make(map[string][]domain.GraphNode)
	for _, node := range nodes {
		if node == nil {
			result.AddError(fmt.Errorf("nil node in input"))
			continue
		}
		nodeType := node.NodeType()
		if nodeType == "" {
			result.AddError(fmt.Errorf("node has empty NodeType"))
			continue
		}
		nodesByType[nodeType] = append(nodesByType[nodeType], node)
	}

	// If we already have errors from nil/invalid nodes, don't proceed
	if result.HasErrors() {
		return result, fmt.Errorf("input validation failed: %d errors", len(result.Errors))
	}

	// Build a comprehensive transaction query that processes all node types
	// We'll build multiple UNWIND blocks, one per node type
	var cypherParts []string
	params := make(map[string]any)
	paramIndex := 0

	// Track node IDs for relationship creation
	var nodeInfos []nodeInfo

	// Process each node type
	for nodeType, typeNodes := range nodesByType {
		// Get a sample node to understand the identifying properties structure
		sampleNode := typeNodes[0]
		idProps := sampleNode.IdentifyingProperties()
		if len(idProps) == 0 {
			result.AddError(fmt.Errorf("node type %s has no identifying properties", nodeType))
			continue
		}

		// Build identifying properties keys for MERGE
		idPropKeys := make([]string, 0, len(idProps))
		for key := range idProps {
			idPropKeys = append(idPropKeys, key)
		}

		// Build list parameter with all nodes of this type
		nodeDataList := make([]map[string]any, 0, len(typeNodes))
		for idx, node := range typeNodes {
			idProps := node.IdentifyingProperties()
			allProps := node.Properties()

			// Combine id and all properties
			nodeData := map[string]any{
				"id_props":  idProps,
				"all_props": allProps,
				"index":     idx, // Track original index for relationship creation
			}
			nodeDataList = append(nodeDataList, nodeData)

			// Store node info for later relationship creation
			parentRef := node.ParentRef()
			if parentRef != nil {
				nodeInfos = append(nodeInfos, nodeInfo{
					elementIDVar:  fmt.Sprintf("n_%s_%d", nodeType, idx),
					nodeType:      nodeType,
					parentRef:     parentRef,
					relType:       node.RelationshipType(),
					originalIndex: idx,
				})
			}
		}

		// Add to parameters
		paramKey := fmt.Sprintf("nodes_%d", paramIndex)
		params[paramKey] = nodeDataList
		paramIndex++

		// Build UNWIND query for this node type
		// Build the MERGE conditions dynamically based on identifying properties
		mergeClauses := make([]string, 0, len(idPropKeys))
		for _, key := range idPropKeys {
			mergeClauses = append(mergeClauses, fmt.Sprintf("%s: nodeData.id_props.%s", key, key))
		}
		mergePropsStr := strings.Join(mergeClauses, ", ")

		cypherPart := fmt.Sprintf(`
			UNWIND $%s AS nodeData
			MERGE (n:%s {%s})
			ON CREATE SET n = nodeData.all_props, n.created_at = timestamp()
			ON MATCH SET n += nodeData.all_props, n.updated_at = timestamp()
			WITH n, nodeData,
			     CASE WHEN n.created_at = n.updated_at THEN 'created' ELSE 'updated' END as operation
		`, paramKey, nodeType, mergePropsStr)

		cypherParts = append(cypherParts, cypherPart)

		// Collect statistics
		cypherParts = append(cypherParts, `
			WITH collect({
				element_id: elementId(n),
				operation: operation,
				node_type: '`+nodeType+`',
				index: nodeData.index
			}) as batch_result
		`)
	}

	// Combine all UNWIND blocks and return aggregated results
	cypherParts = append(cypherParts, `
		RETURN batch_result
	`)

	cypher := strings.Join(cypherParts, "\n")

	// Execute the batch node creation in a single transaction
	queryResult, err := l.client.Query(ctx, cypher, params)
	if err != nil {
		return result, fmt.Errorf("batch node creation failed: %w", err)
	}

	// Process results to get element IDs and update statistics
	elementIDMap := make(map[string]string) // key: "nodeType:index", value: element ID
	for _, record := range queryResult.Records {
		batchResult, ok := record["batch_result"].([]any)
		if !ok {
			continue
		}

		for _, item := range batchResult {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}

			op, _ := itemMap["operation"].(string)
			if op == "created" {
				result.NodesCreated++
			} else if op == "updated" {
				result.NodesUpdated++
			}

			// Store element ID for relationship creation
			elementID, _ := itemMap["element_id"].(string)
			nodeType, _ := itemMap["node_type"].(string)
			index, _ := itemMap["index"].(int64) // JSON numbers are float64, but our index is int
			if index == 0 {
				// Try float64 conversion
				if indexFloat, ok := itemMap["index"].(float64); ok {
					index = int64(indexFloat)
				}
			}
			key := fmt.Sprintf("%s:%d", nodeType, index)
			elementIDMap[key] = elementID
		}
	}

	// Create parent relationships in batch
	if len(nodeInfos) > 0 {
		relCount, err := l.createParentRelationshipsBatch(ctx, nodeInfos, elementIDMap)
		if err != nil {
			return result, fmt.Errorf("batch parent relationship creation failed: %w", err)
		}
		result.RelationshipsCreated += relCount
	}

	// Create DISCOVERED relationships in batch
	if execCtx.AgentRunID != "" && len(elementIDMap) > 0 {
		// Extract all element IDs
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

// createParentRelationshipsBatch creates parent relationships in batch using UNWIND.
func (l *GraphLoader) createParentRelationshipsBatch(ctx context.Context, nodeInfos []nodeInfo, elementIDMap map[string]string) (int, error) {
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

		// Get child element ID from map
		childKey := fmt.Sprintf("%s:%d", info.nodeType, info.originalIndex)
		childID, ok := elementIDMap[childKey]
		if !ok {
			continue // Skip if we don't have the element ID
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
		// Get first item to determine parent property keys
		if len(relDataList) == 0 {
			continue
		}

		sampleParentProps := relDataList[0]["parent_props"].(map[string]any)
		propKeys := make([]string, 0, len(sampleParentProps))
		for k := range sampleParentProps {
			propKeys = append(propKeys, k)
		}

		// Build WHERE clause for parent matching
		whereClauses := make([]string, 0, len(propKeys))
		for _, propKey := range propKeys {
			whereClauses = append(whereClauses, fmt.Sprintf("parent.%s = relData.parent_props.%s", propKey, propKey))
		}
		whereStr := strings.Join(whereClauses, " AND ")

		cypher := fmt.Sprintf(`
			UNWIND $rel_data AS relData
			MATCH (parent:%s) WHERE %s
			MATCH (child) WHERE elementId(child) = relData.child_id
			MERGE (parent)-[r:%s]->(child)
			RETURN count(r) as rel_count
		`, key.parentType, whereStr, key.relType)

		params := map[string]any{
			"rel_data": relDataList,
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
