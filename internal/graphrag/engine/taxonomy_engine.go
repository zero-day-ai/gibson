package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TaxonomyGraphEngine processes events and creates graph nodes/relationships
// based on taxonomy definitions. It provides a declarative, configuration-driven
// approach to graph construction.
type TaxonomyGraphEngine interface {
	// HandleEvent processes an execution event (mission.start, agent.start, llm.call, etc.)
	// and creates graph nodes/relationships based on taxonomy definitions.
	HandleEvent(ctx context.Context, eventType string, data map[string]any) error

	// HandleToolOutput processes tool output and extracts assets based on tool output schemas.
	// The agentRunID is used to link discovered assets to the agent execution.
	HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error

	// HandleFinding processes a finding submission and creates Finding node with relationships.
	HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error

	// Health returns engine health status including Neo4j connectivity.
	Health(ctx context.Context) HealthStatus
}

// HealthStatus represents engine health state.
type HealthStatus struct {
	Healthy     bool   // Overall health status
	Neo4jStatus string // Neo4j connection state
	Message     string // Human-readable status message
}

// taxonomyGraphEngine is the default implementation of TaxonomyGraphEngine.
type taxonomyGraphEngine struct {
	registry    taxonomy.TaxonomyRegistry
	graphClient graph.GraphClient
	logger      *slog.Logger
}

// NewTaxonomyGraphEngine creates a new taxonomy-driven graph engine.
// The registry provides taxonomy definitions (node types, relationships, event schemas, tool schemas).
// The graphClient handles Neo4j interactions.
func NewTaxonomyGraphEngine(
	registry taxonomy.TaxonomyRegistry,
	graphClient graph.GraphClient,
	logger *slog.Logger,
) TaxonomyGraphEngine {
	if logger == nil {
		logger = slog.Default()
	}

	return &taxonomyGraphEngine{
		registry:    registry,
		graphClient: graphClient,
		logger:      logger.With("component", "taxonomy_graph_engine"),
	}
}

// HandleEvent processes an execution event and creates graph nodes/relationships.
func (e *taxonomyGraphEngine) HandleEvent(ctx context.Context, eventType string, data map[string]any) error {
	logger := e.logger.With("event_type", eventType)

	// Look up event definition from registry
	eventDef := e.registry.GetExecutionEvent(eventType)
	if eventDef == nil {
		// Graceful degradation - log warning but don't error
		logger.Warn("execution event not found in taxonomy, skipping graph operations")
		return nil
	}

	logger.Debug("processing execution event", "event_type", eventDef.EventType)

	// Create node if specified
	if eventDef.CreatesNode != nil {
		if err := e.createNodeFromEvent(ctx, eventDef.CreatesNode, data); err != nil {
			return fmt.Errorf("failed to create node from event: %w", err)
		}
	}

	// Create relationships if specified
	for _, relSpec := range eventDef.CreatesRelationships {
		if err := e.createRelationshipFromEvent(ctx, &relSpec, data); err != nil {
			return fmt.Errorf("failed to create relationship from event: %w", err)
		}
	}

	logger.Debug("successfully processed execution event")
	return nil
}

// HandleToolOutput processes tool output and extracts assets.
func (e *taxonomyGraphEngine) HandleToolOutput(ctx context.Context, toolName string, output map[string]any, agentRunID string) error {
	logger := e.logger.With("tool_name", toolName, "agent_run_id", agentRunID)

	// Look up tool output schema from registry
	schema := e.registry.GetToolOutputSchema(toolName)
	if schema == nil {
		// Graceful degradation - tool just doesn't have a schema defined
		logger.Debug("tool output schema not found, skipping extraction")
		return nil
	}

	logger.Debug("processing tool output", "tool", schema.Tool, "extractions", len(schema.Extracts))

	// Process each extraction rule
	for _, extractSpec := range schema.Extracts {
		if err := e.processExtraction(ctx, &extractSpec, output, agentRunID); err != nil {
			// Log error but continue processing other extractions
			logger.Error("failed to process extraction", "node_type", extractSpec.NodeType, "error", err)
		}
	}

	logger.Debug("successfully processed tool output")
	return nil
}

// HandleFinding processes a finding and creates graph nodes/relationships.
func (e *taxonomyGraphEngine) HandleFinding(ctx context.Context, finding agent.Finding, missionID string) error {
	logger := e.logger.With("finding_id", finding.ID, "mission_id", missionID)

	// Create Finding node
	findingProps := map[string]any{
		"id":          finding.ID,
		"title":       finding.Title,
		"description": finding.Description,
		"severity":    string(finding.Severity),
		"confidence":  finding.Confidence,
		"category":    finding.Category,
		"created_at":  finding.CreatedAt.Unix(),
	}

	// Add optional fields
	if finding.TargetID != nil {
		findingProps["target_id"] = *finding.TargetID
	}
	if finding.CVSS != nil {
		findingProps["cvss_score"] = finding.CVSS.Score
		findingProps["cvss_vector"] = finding.CVSS.Vector
	}
	if len(finding.CWE) > 0 {
		findingProps["cwes"] = finding.CWE
	}

	// Create Finding node using MERGE for idempotency
	cypher := `
		MERGE (f:Finding {id: $id})
		SET f += $props
		RETURN f
	`
	params := map[string]any{
		"id":    string(finding.ID),
		"props": findingProps,
	}

	if _, err := e.graphClient.Query(ctx, cypher, params); err != nil {
		return fmt.Errorf("failed to create Finding node: %w", err)
	}

	logger.Debug("created Finding node")

	// Create AFFECTS relationship to target if target exists
	if finding.TargetID != nil {
		affectsRel := `
			MATCH (f:Finding {id: $finding_id})
			MATCH (t {id: $target_id})
			MERGE (f)-[r:AFFECTS]->(t)
			SET r.discovered_at = timestamp()
		`
		affectsParams := map[string]any{
			"finding_id": string(finding.ID),
			"target_id":  string(*finding.TargetID),
		}

		if _, err := e.graphClient.Query(ctx, affectsRel, affectsParams); err != nil {
			// Log but don't fail if target doesn't exist yet
			logger.Warn("failed to create AFFECTS relationship", "error", err)
		} else {
			logger.Debug("created AFFECTS relationship")
		}
	}

	// Create USES_TECHNIQUE relationships for CWEs
	for _, cwe := range finding.CWE {
		techniqueRel := `
			MATCH (f:Finding {id: $finding_id})
			MERGE (t:Technique {id: $cwe})
			MERGE (f)-[r:USES_TECHNIQUE]->(t)
		`
		techniqueParams := map[string]any{
			"finding_id": string(finding.ID),
			"cwe":        cwe,
		}

		if _, err := e.graphClient.Query(ctx, techniqueRel, techniqueParams); err != nil {
			logger.Warn("failed to create USES_TECHNIQUE relationship", "cwe", cwe, "error", err)
		}
	}

	// Create PART_OF relationship to mission
	if missionID != "" {
		missionRel := `
			MATCH (f:Finding {id: $finding_id})
			MATCH (m:Mission {id: $mission_id})
			MERGE (f)-[r:PART_OF]->(m)
		`
		missionParams := map[string]any{
			"finding_id": string(finding.ID),
			"mission_id": missionID,
		}

		if _, err := e.graphClient.Query(ctx, missionRel, missionParams); err != nil {
			logger.Warn("failed to create PART_OF relationship to mission", "error", err)
		} else {
			logger.Debug("created PART_OF relationship to mission")
		}
	}

	logger.Debug("successfully processed finding")
	return nil
}

// Health returns the engine health status.
func (e *taxonomyGraphEngine) Health(ctx context.Context) HealthStatus {
	// Check Neo4j connectivity
	neo4jHealth := e.graphClient.Health(ctx)

	healthy := neo4jHealth.State == types.HealthStateHealthy
	message := "engine operational"
	if !healthy {
		message = fmt.Sprintf("neo4j unhealthy: %s", neo4jHealth.Message)
	}

	return HealthStatus{
		Healthy:     healthy,
		Neo4jStatus: string(neo4jHealth.State),
		Message:     message,
	}
}

// createNodeFromEvent creates a node based on event data.
func (e *taxonomyGraphEngine) createNodeFromEvent(ctx context.Context, nodeSpec *taxonomy.EventNodeCreation, data map[string]any) error {
	// Interpolate ID template
	nodeID, err := e.interpolateTemplate(nodeSpec.IDTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to interpolate node ID: %w", err)
	}

	// Build properties from mappings
	props := make(map[string]any)
	props["id"] = nodeID

	for _, propMapping := range nodeSpec.Properties {
		var value any
		var ok bool

		if propMapping.Source != "" {
			// Source-based mapping (from event data)
			value, ok = data[propMapping.Source]
		} else if propMapping.Value != nil {
			// Static value mapping
			value = propMapping.Value
			ok = true
		}

		if ok || !propMapping.Optional {
			if ok {
				props[propMapping.Target] = value
			} else if !propMapping.Optional {
				return fmt.Errorf("required property %s not found in data", propMapping.Target)
			}
		}
	}

	// Generate MERGE Cypher query for idempotency
	cypher := fmt.Sprintf(`
		MERGE (n:%s {id: $id})
		SET n += $props
		RETURN n
	`, nodeSpec.Type)

	params := map[string]any{
		"id":    nodeID,
		"props": props,
	}

	if _, err := e.graphClient.Query(ctx, cypher, params); err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	e.logger.Debug("created node from event",
		"node_type", nodeSpec.Type,
		"node_id", nodeID,
	)

	return nil
}

// createRelationshipFromEvent creates a relationship based on event data.
func (e *taxonomyGraphEngine) createRelationshipFromEvent(ctx context.Context, relSpec *taxonomy.EventRelationshipCreation, data map[string]any) error {
	// Interpolate from and to node references
	fromNodeID, err := e.interpolateTemplate(relSpec.FromTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to interpolate from node: %w", err)
	}

	toNodeID, err := e.interpolateTemplate(relSpec.ToTemplate, data)
	if err != nil {
		return fmt.Errorf("failed to interpolate to node: %w", err)
	}

	// Build relationship properties
	relProps := make(map[string]any)
	for _, propMapping := range relSpec.Properties {
		var value any
		var ok bool

		if propMapping.Source != "" {
			value, ok = data[propMapping.Source]
		} else if propMapping.Value != nil {
			value = propMapping.Value
			ok = true
		}

		if ok {
			relProps[propMapping.Target] = value
		} else if !propMapping.Optional {
			return fmt.Errorf("required relationship property %s not found in data", propMapping.Target)
		}
	}

	// Generate MERGE Cypher for relationship
	var cypher string
	if len(relProps) > 0 {
		cypher = fmt.Sprintf(`
			MATCH (from {id: $from_id})
			MATCH (to {id: $to_id})
			MERGE (from)-[r:%s]->(to)
			SET r += $props
			RETURN r
		`, relSpec.Type)
	} else {
		cypher = fmt.Sprintf(`
			MATCH (from {id: $from_id})
			MATCH (to {id: $to_id})
			MERGE (from)-[r:%s]->(to)
			RETURN r
		`, relSpec.Type)
	}

	params := map[string]any{
		"from_id": fromNodeID,
		"to_id":   toNodeID,
		"props":   relProps,
	}

	if _, err := e.graphClient.Query(ctx, cypher, params); err != nil {
		return fmt.Errorf("failed to create relationship: %w", err)
	}

	e.logger.Debug("created relationship from event",
		"relationship_type", relSpec.Type,
		"from", fromNodeID,
		"to", toNodeID,
	)

	return nil
}

// processExtraction processes a single extraction rule from tool output.
func (e *taxonomyGraphEngine) processExtraction(ctx context.Context, extractSpec *taxonomy.ToolOutputExtraction, output map[string]any, agentRunID string) error {
	logger := e.logger.With("node_type", extractSpec.NodeType)

	// Extract items using JSONPath
	items, err := e.extractItems(output, extractSpec.JSONPath)
	if err != nil {
		return fmt.Errorf("failed to extract items: %w", err)
	}

	logger.Debug("extracted items from tool output", "count", len(items))

	// Process each extracted item
	for i, item := range items {
		if err := e.createNodeFromExtraction(ctx, extractSpec, item, agentRunID); err != nil {
			logger.Error("failed to create node from extraction",
				"item_index", i,
				"error", err,
			)
			// Continue processing other items
		}
	}

	return nil
}

// extractItems extracts items from output using a selector.
// Currently supports simple JSONPath-like dot notation (e.g., "hosts.host").
func (e *taxonomyGraphEngine) extractItems(output map[string]any, selector string) ([]map[string]any, error) {
	// Simple implementation: split on dots and traverse
	parts := strings.Split(selector, ".")
	current := output

	for i, part := range parts {
		value, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("selector path not found: %s (at %s)", selector, part)
		}

		// If this is the last part, return the items
		if i == len(parts)-1 {
			// Handle both array and single item cases
			switch v := value.(type) {
			case []map[string]any:
				return v, nil
			case []any:
				// Convert []any to []map[string]any
				result := make([]map[string]any, 0, len(v))
				for _, item := range v {
					if m, ok := item.(map[string]any); ok {
						result = append(result, m)
					}
				}
				return result, nil
			case map[string]any:
				// Single item
				return []map[string]any{v}, nil
			default:
				return nil, fmt.Errorf("selector result is not a map or array: %T", value)
			}
		}

		// Continue traversing
		if nextMap, ok := value.(map[string]any); ok {
			current = nextMap
		} else {
			return nil, fmt.Errorf("selector path not traversable at: %s", part)
		}
	}

	return nil, fmt.Errorf("selector extraction failed")
}

// createNodeFromExtraction creates a node from extracted tool output data.
func (e *taxonomyGraphEngine) createNodeFromExtraction(ctx context.Context, extractSpec *taxonomy.ToolOutputExtraction, item map[string]any, agentRunID string) error {
	// Build properties from mappings
	props := make(map[string]any)

	for _, prop := range extractSpec.Properties {
		var value any
		var ok bool

		if prop.JSONPath != "" {
			// Extract from item using JSONPath (simplified - just field access)
			value, ok = item[prop.JSONPath]
		} else if prop.Template != "" {
			// Apply template
			templated, err := e.interpolateTemplate(prop.Template, item)
			if err == nil {
				value = templated
				ok = true
			}
		} else if prop.Default != nil {
			value = prop.Default
			ok = true
		}

		if ok {
			props[prop.Target] = value
		}
	}

	// Generate node ID from node type definition
	nodeDef, ok := e.registry.NodeType(extractSpec.NodeType)
	if !ok {
		return fmt.Errorf("node type not found in taxonomy: %s", extractSpec.NodeType)
	}

	nodeID, err := e.interpolateTemplate(nodeDef.IDTemplate, props)
	if err != nil {
		return fmt.Errorf("failed to generate node ID: %w", err)
	}

	props["id"] = nodeID

	// Create node using MERGE for idempotency
	cypher := fmt.Sprintf(`
		MERGE (n:%s {id: $id})
		SET n += $props
		RETURN n
	`, extractSpec.NodeType)

	params := map[string]any{
		"id":    nodeID,
		"props": props,
	}

	if _, err := e.graphClient.Query(ctx, cypher, params); err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	e.logger.Debug("created node from extraction",
		"node_type", extractSpec.NodeType,
		"node_id", nodeID,
	)

	// Create relationships
	for _, relSpec := range extractSpec.Relationships {
		if err := e.createRelationshipFromExtraction(ctx, &relSpec, nodeID, agentRunID); err != nil {
			e.logger.Warn("failed to create relationship from extraction",
				"relationship_type", relSpec.Type,
				"error", err,
			)
		}
	}

	return nil
}

// createRelationshipFromExtraction creates a relationship from extracted data.
func (e *taxonomyGraphEngine) createRelationshipFromExtraction(ctx context.Context, relSpec *taxonomy.ToolOutputRelationship, fromNodeID string, agentRunID string) error {
	// Interpolate to node template
	toNodeID, err := e.interpolateTemplate(relSpec.ToTemplate, map[string]any{
		"agent_run_id": agentRunID,
		"from_node_id": fromNodeID,
	})
	if err != nil {
		return fmt.Errorf("failed to interpolate to node: %w", err)
	}

	// Handle special node references
	if toNodeID == "tool_execution" || toNodeID == "agent_run" {
		toNodeID = agentRunID
	}

	// Build relationship properties
	relProps := make(map[string]any)
	relProps["created_at"] = "timestamp()" // Let Cypher handle timestamp

	for _, prop := range relSpec.Properties {
		if prop.Value != "" {
			relProps[prop.Name] = prop.Value
		}
	}

	// Generate MERGE Cypher for relationship
	cypher := fmt.Sprintf(`
		MATCH (from {id: $from_id})
		MATCH (to {id: $to_id})
		MERGE (from)-[r:%s]->(to)
		SET r.created_at = timestamp()
		RETURN r
	`, relSpec.Type)

	params := map[string]any{
		"from_id": fromNodeID,
		"to_id":   toNodeID,
	}

	if _, err := e.graphClient.Query(ctx, cypher, params); err != nil {
		return fmt.Errorf("failed to create relationship: %w", err)
	}

	e.logger.Debug("created relationship from extraction",
		"relationship_type", relSpec.Type,
		"from", fromNodeID,
		"to", toNodeID,
	)

	return nil
}

// interpolateTemplate replaces placeholders in a template with data values.
// Supports {field} syntax for simple replacements and {field.nested} for nested access.
func (e *taxonomyGraphEngine) interpolateTemplate(template string, data map[string]any) (string, error) {
	// Find all {placeholders}
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	result := template
	for _, match := range matches {
		placeholder := match[0] // Full match including braces
		fieldPath := match[1]   // Just the field path

		// Handle nested field access with dot notation
		value, err := e.getNestedField(data, fieldPath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve placeholder %s: %w", placeholder, err)
		}

		// Convert value to string
		valueStr := fmt.Sprint(value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}

	return result, nil
}

// getNestedField retrieves a nested field value from a map using dot notation.
func (e *taxonomyGraphEngine) getNestedField(data map[string]any, path string) (any, error) {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		value, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("field not found: %s", part)
		}

		// If this is the last part, return the value
		if i == len(parts)-1 {
			return value, nil
		}

		// Continue traversing if it's a map
		if nextMap, ok := value.(map[string]any); ok {
			current = nextMap
		} else {
			return nil, fmt.Errorf("cannot traverse non-map field: %s", part)
		}
	}

	return nil, fmt.Errorf("field path resolution failed: %s", path)
}

// marshalJSON is a helper to convert any value to JSON string for logging.
func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
