package builtins

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/schema"
)

// PayloadSearchTool provides search capabilities over the payload library.
// It implements the tool.Tool interface and is registered as a builtin tool.
type PayloadSearchTool struct {
	registry payload.PayloadRegistry
}

// NewPayloadSearchTool creates a new payload search tool.
func NewPayloadSearchTool(registry payload.PayloadRegistry) tool.Tool {
	return &PayloadSearchTool{
		registry: registry,
	}
}

// Name returns the unique identifier for this tool.
func (t *PayloadSearchTool) Name() string {
	return "payload_search"
}

// Version returns the semantic version of this tool.
func (t *PayloadSearchTool) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description of what this tool does.
func (t *PayloadSearchTool) Description() string {
	return "Search the payload library for attack payloads. Returns payload summaries (id, name, description, categories, severity) without full templates to save tokens."
}

// Tags returns a list of tags for categorization and discovery.
func (t *PayloadSearchTool) Tags() []string {
	return []string{"payload", "search", "attack"}
}

// InputSchema returns the JSON schema defining valid input parameters.
func (t *PayloadSearchTool) InputSchema() schema.JSON {
	return schema.Object(map[string]schema.JSON{
		"query": schema.JSON{
			Type:        "string",
			Description: "Search query for full-text search across payload names and descriptions",
		},
		"categories": schema.JSON{
			Type:        "array",
			Description: "Filter by payload categories (jailbreak, prompt_injection, etc.)",
			Items:       &schema.JSON{Type: "string"},
		},
		"tags": schema.JSON{
			Type:        "array",
			Description: "Filter by tags",
			Items:       &schema.JSON{Type: "string"},
		},
		"target_type": schema.JSON{
			Type:        "string",
			Description: "Filter by target type (llm_chat, llm_api, etc.)",
		},
		"severity": schema.JSON{
			Type:        "string",
			Description: "Filter by minimum severity (critical, high, medium, low, info)",
		},
		"limit": schema.JSON{
			Type:        "number",
			Description: "Maximum number of results to return (default: 10)",
			Default:     10,
		},
	})
}

// OutputSchema returns the JSON schema defining the output structure.
func (t *PayloadSearchTool) OutputSchema() schema.JSON {
	return schema.Object(map[string]schema.JSON{
		"results": schema.Array(
			schema.Object(map[string]schema.JSON{
				"id": schema.JSON{
					Type:        "string",
					Description: "Payload ID for execution",
				},
				"name": schema.JSON{
					Type:        "string",
					Description: "Payload name",
				},
				"description": schema.JSON{
					Type:        "string",
					Description: "What this payload does",
				},
				"categories": schema.JSON{
					Type:        "array",
					Description: "Payload categories",
					Items:       &schema.JSON{Type: "string"},
				},
				"severity": schema.JSON{
					Type:        "string",
					Description: "Severity level",
				},
				"reliability": schema.JSON{
					Type:        "number",
					Description: "Reliability score 0.0-1.0",
				},
				"target_types": schema.JSON{
					Type:        "array",
					Description: "Compatible target types",
					Items:       &schema.JSON{Type: "string"},
				},
			}),
		),
		"count": schema.JSON{
			Type:        "number",
			Description: "Number of results returned",
		},
	})
}

// Execute runs the tool with the given input and returns the result.
func (t *PayloadSearchTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Handle nil registry gracefully
	if t.registry == nil {
		return map[string]any{
			"results": []any{},
			"count":   0,
		}, nil
	}

	// Build filter from input
	filter := &payload.PayloadFilter{
		Enabled: boolPtr(true), // Only return enabled payloads
	}

	// Extract categories
	if categoriesVal, ok := input["categories"]; ok {
		if categoriesArr, ok := categoriesVal.([]interface{}); ok {
			categories := make([]payload.PayloadCategory, 0, len(categoriesArr))
			for _, cat := range categoriesArr {
				if catStr, ok := cat.(string); ok {
					categories = append(categories, payload.PayloadCategory(catStr))
				}
			}
			filter.Categories = categories
		}
	}

	// Extract tags
	if tagsVal, ok := input["tags"]; ok {
		if tagsArr, ok := tagsVal.([]interface{}); ok {
			tags := make([]string, 0, len(tagsArr))
			for _, tag := range tagsArr {
				if tagStr, ok := tag.(string); ok {
					tags = append(tags, tagStr)
				}
			}
			filter.Tags = tags
		}
	}

	// Extract target type
	if targetTypeVal, ok := input["target_type"]; ok {
		if targetTypeStr, ok := targetTypeVal.(string); ok {
			filter.TargetTypes = []string{targetTypeStr}
		}
	}

	// Extract severity
	if severityVal, ok := input["severity"]; ok {
		if severityStr, ok := severityVal.(string); ok {
			// Convert string to FindingSeverity and add to filter
			filter.Severities = []agent.FindingSeverity{agent.FindingSeverity(severityStr)}
		}
	}

	// Extract limit
	limit := 10
	if limitVal, ok := input["limit"]; ok {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50 // Cap at 50 to prevent token abuse
	}
	filter.Limit = limit

	// Execute search
	var payloads []*payload.Payload
	var err error

	if query, ok := input["query"].(string); ok && query != "" {
		// Full-text search
		payloads, err = t.registry.Search(ctx, query, filter)
	} else {
		// List with filters
		payloads, err = t.registry.List(ctx, filter)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search payloads: %w", err)
	}

	// Format results (summaries only, no templates)
	formattedResults := make([]map[string]any, 0, len(payloads))
	for _, p := range payloads {
		// Convert categories to strings
		categoryStrs := make([]string, len(p.Categories))
		for i, cat := range p.Categories {
			categoryStrs[i] = cat.String()
		}

		// Get target types
		targetTypes := make([]string, 0)
		if p.TargetTypes != nil {
			targetTypes = p.TargetTypes
		}

		formattedResults = append(formattedResults, map[string]any{
			"id":           p.ID.String(),
			"name":         p.Name,
			"description":  p.Description,
			"categories":   categoryStrs,
			"severity":     string(p.Severity),
			"reliability":  p.Metadata.Reliability,
			"target_types": targetTypes,
		})
	}

	return map[string]any{
		"results": formattedResults,
		"count":   len(formattedResults),
	}, nil
}

// Health returns the current health status of this tool.
func (t *PayloadSearchTool) Health(ctx context.Context) types.HealthStatus {
	if t.registry == nil {
		return types.Degraded("Payload registry not initialized")
	}
	return t.registry.Health(ctx)
}

// PayloadExecuteTool executes payloads against targets with success detection.
// It implements the tool.Tool interface and is registered as a builtin tool.
type PayloadExecuteTool struct {
	executor payload.PayloadExecutor
}

// NewPayloadExecuteTool creates a new payload execute tool.
func NewPayloadExecuteTool(executor payload.PayloadExecutor) tool.Tool {
	return &PayloadExecuteTool{
		executor: executor,
	}
}

// Name returns the unique identifier for this tool.
func (t *PayloadExecuteTool) Name() string {
	return "payload_execute"
}

// Version returns the semantic version of this tool.
func (t *PayloadExecuteTool) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description of what this tool does.
func (t *PayloadExecuteTool) Description() string {
	return "Execute an attack payload against a target. Returns success/confidence, execution details, matched indicators, and suggested findings for successful attacks."
}

// Tags returns a list of tags for categorization and discovery.
func (t *PayloadExecuteTool) Tags() []string {
	return []string{"payload", "execute", "attack"}
}

// InputSchema returns the JSON schema defining valid input parameters.
func (t *PayloadExecuteTool) InputSchema() schema.JSON {
	return schema.Object(map[string]schema.JSON{
		"payload_id": schema.JSON{
			Type:        "string",
			Description: "ID of the payload to execute (from payload_search)",
		},
		"target": schema.JSON{
			Type:        "string",
			Description: "Target ID or identifier",
		},
		"params": schema.JSON{
			Type:        "object",
			Description: "Parameter overrides for payload template",
			Properties:  map[string]schema.JSON{},
		},
		"timeout": schema.JSON{
			Type:        "number",
			Description: "Execution timeout in seconds (default: 30)",
			Default:     30,
		},
	}, "payload_id", "target") // payload_id and target are required
}

// OutputSchema returns the JSON schema defining the output structure.
func (t *PayloadExecuteTool) OutputSchema() schema.JSON {
	return schema.Object(map[string]schema.JSON{
		"success": schema.JSON{
			Type:        "boolean",
			Description: "Whether the payload execution succeeded",
		},
		"confidence": schema.JSON{
			Type:        "number",
			Description: "Confidence score 0.0-1.0 based on indicator matches",
		},
		"execution": schema.Object(map[string]schema.JSON{
			"duration_ms": schema.Number(),
			"exit_code":   schema.Number(),
			"stdout":      schema.String(),
			"stderr":      schema.String(),
		}),
		"indicators_matched": schema.Array(
			schema.Object(map[string]schema.JSON{
				"type":    schema.String(),
				"pattern": schema.String(),
				"matched": schema.Bool(),
			}),
		),
		"suggested_finding": schema.JSON{
			Type:        "object",
			Description: "Pre-filled finding details if attack succeeded",
			Properties: map[string]schema.JSON{
				"title":       schema.String(),
				"description": schema.String(),
				"severity":    schema.String(),
				"category":    schema.String(),
			},
		},
		"suggested_next": schema.JSON{
			Type:        "array",
			Description: "Related payload IDs to try next",
			Items:       &schema.JSON{Type: "string"},
		},
	})
}

// Execute runs the tool with the given input and returns the result.
func (t *PayloadExecuteTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Handle nil executor gracefully
	if t.executor == nil {
		return nil, fmt.Errorf("payload executor not initialized")
	}

	// Extract payload_id (required)
	payloadIDStr, ok := input["payload_id"].(string)
	if !ok || payloadIDStr == "" {
		return nil, fmt.Errorf("payload_id parameter is required and must be a non-empty string")
	}
	payloadID := types.ID(payloadIDStr)

	// Extract target (required)
	target, ok := input["target"].(string)
	if !ok || target == "" {
		return nil, fmt.Errorf("target parameter is required and must be a non-empty string")
	}
	targetID := types.ID(target)

	// Extract optional params
	params := make(map[string]interface{})
	if paramsVal, ok := input["params"]; ok {
		if paramsMap, ok := paramsVal.(map[string]interface{}); ok {
			params = paramsMap
		}
	}

	// Extract timeout
	timeout := 30 * time.Second
	if timeoutVal, ok := input["timeout"]; ok {
		switch v := timeoutVal.(type) {
		case float64:
			timeout = time.Duration(v) * time.Second
		case int:
			timeout = time.Duration(v) * time.Second
		}
	}

	// Build execution request
	req := &payload.ExecutionRequest{
		PayloadID:  payloadID,
		TargetID:   targetID,
		Parameters: params,
		Timeout:    timeout,
		AgentID:    types.ID("builtin-tool"), // Mark as tool execution
	}

	// Execute payload
	result, err := t.executor.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("payload execution failed: %w", err)
	}

	// Format execution details
	execution := map[string]any{
		"duration_ms": result.ResponseTime.Milliseconds(),
	}
	if result.Response != "" {
		execution["response"] = result.Response
	}

	// Format indicators matched (list of strings)
	indicatorsMatched := make([]map[string]any, len(result.IndicatorsMatched))
	for i, indicator := range result.IndicatorsMatched {
		indicatorsMatched[i] = map[string]any{
			"indicator": indicator,
			"matched":   true,
		}
	}

	// Build output
	output := map[string]any{
		"success":            result.Success,
		"confidence":         result.ConfidenceScore,
		"execution":          execution,
		"indicators_matched": indicatorsMatched,
	}

	// Add finding details if attack succeeded and created a finding
	if result.Success && result.Finding != nil {
		output["suggested_finding"] = map[string]any{
			"title":       result.Finding.Title,
			"description": result.Finding.Description,
			"severity":    string(result.Finding.Severity),
			"category":    string(result.Finding.Category),
		}
	}

	return output, nil
}

// Health returns the current health status of this tool.
func (t *PayloadExecuteTool) Health(ctx context.Context) types.HealthStatus {
	if t.executor == nil {
		return types.Degraded("Payload executor not initialized")
	}
	return types.Healthy("Payload executor ready")
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}
