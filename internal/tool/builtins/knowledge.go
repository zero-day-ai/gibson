package builtins

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/knowledge"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/sdk/schema"
)

// KnowledgeSearchTool provides semantic search over the knowledge store.
// It implements the tool.Tool interface and is registered as a builtin tool.
type KnowledgeSearchTool struct {
	store KnowledgeStore
}

// KnowledgeStore defines the interface for knowledge storage operations.
// This interface allows tools to access the knowledge subsystem.
type KnowledgeStore interface {
	// Search performs semantic search over knowledge chunks
	Search(ctx context.Context, query string, opts knowledge.SearchOptions) ([]knowledge.KnowledgeResult, error)

	// Health returns the health status of the knowledge store
	Health(ctx context.Context) types.HealthStatus
}

// NewKnowledgeSearchTool creates a new knowledge search tool.
func NewKnowledgeSearchTool(store KnowledgeStore) tool.Tool {
	return &KnowledgeSearchTool{
		store: store,
	}
}

// Name returns the unique identifier for this tool.
func (t *KnowledgeSearchTool) Name() string {
	return "knowledge_search"
}

// Version returns the semantic version of this tool.
func (t *KnowledgeSearchTool) Version() string {
	return "1.0.0"
}

// Description returns a human-readable description of what this tool does.
func (t *KnowledgeSearchTool) Description() string {
	return "Search the local knowledge store using semantic similarity. Returns relevant chunks from ingested PDFs, URLs, and documents with similarity scores."
}

// Tags returns a list of tags for categorization and discovery.
func (t *KnowledgeSearchTool) Tags() []string {
	return []string{"knowledge", "search", "rag", "semantic"}
}

// InputSchema returns the JSON schema defining valid input parameters.
func (t *KnowledgeSearchTool) InputSchema() schema.JSON {
	return schema.Object(map[string]schema.JSON{
		"query": schema.JSON{
			Type:        "string",
			Description: "The search query to find relevant knowledge",
		},
		"limit": schema.JSON{
			Type:        "number",
			Description: "Maximum number of results to return (default: 10)",
			Default:     10,
		},
		"threshold": schema.JSON{
			Type:        "number",
			Description: "Minimum similarity threshold 0.0-1.0 (default: 0.7)",
			Default:     0.7,
		},
		"source": schema.JSON{
			Type:        "string",
			Description: "Optional: filter results by source file/URL",
		},
	}, "query") // query is required
}

// OutputSchema returns the JSON schema defining the output structure.
func (t *KnowledgeSearchTool) OutputSchema() schema.JSON {
	return schema.Object(map[string]schema.JSON{
		"results": schema.Array(
			schema.Object(map[string]schema.JSON{
				"text": schema.JSON{
					Type:        "string",
					Description: "The text content of the knowledge chunk",
				},
				"source": schema.JSON{
					Type:        "string",
					Description: "Source file path or URL",
				},
				"similarity": schema.JSON{
					Type:        "number",
					Description: "Similarity score 0.0-1.0",
				},
				"metadata": schema.Object(map[string]schema.JSON{
					"section":     schema.String(),
					"page_number": schema.Number(),
					"has_code":    schema.Bool(),
					"language":    schema.String(),
					"title":       schema.String(),
				}),
			}),
		),
		"count": schema.JSON{
			Type:        "number",
			Description: "Number of results returned",
		},
	})
}

// Execute runs the tool with the given input and returns the result.
func (t *KnowledgeSearchTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	// Handle nil store gracefully (knowledge store not initialized)
	if t.store == nil {
		return map[string]any{
			"results": []any{},
			"count":   0,
		}, nil
	}

	// Extract query parameter (required)
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required and must be a non-empty string")
	}

	// Extract optional parameters with defaults
	limit := 10
	if limitVal, ok := input["limit"]; ok {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	threshold := 0.7
	if thresholdVal, ok := input["threshold"]; ok {
		if v, ok := thresholdVal.(float64); ok {
			threshold = v
		}
	}

	source := ""
	if sourceVal, ok := input["source"]; ok {
		if v, ok := sourceVal.(string); ok {
			source = v
		}
	}

	// Validate parameters
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100 // Cap at 100 to prevent token abuse
	}
	if threshold < 0.0 {
		threshold = 0.0
	}
	if threshold > 1.0 {
		threshold = 1.0
	}

	// Build search options
	opts := knowledge.SearchOptions{
		Limit:     limit,
		Threshold: threshold,
		Source:    source,
	}

	// Execute search
	results, err := t.store.Search(ctx, query, opts)
	if err != nil {
		// Return empty results on error (knowledge store may be empty)
		return map[string]any{
			"results": []any{},
			"count":   0,
		}, nil
	}

	// Format results for output
	formattedResults := make([]map[string]any, 0, len(results))
	for _, result := range results {
		metadata := map[string]any{}
		if result.Chunk.Metadata.Section != "" {
			metadata["section"] = result.Chunk.Metadata.Section
		}
		if result.Chunk.Metadata.PageNumber > 0 {
			metadata["page_number"] = result.Chunk.Metadata.PageNumber
		}
		if result.Chunk.Metadata.HasCode {
			metadata["has_code"] = result.Chunk.Metadata.HasCode
		}
		if result.Chunk.Metadata.Language != "" {
			metadata["language"] = result.Chunk.Metadata.Language
		}
		if result.Chunk.Metadata.Title != "" {
			metadata["title"] = result.Chunk.Metadata.Title
		}

		formattedResults = append(formattedResults, map[string]any{
			"text":       result.Chunk.Text,
			"source":     result.Chunk.Source,
			"similarity": result.Score,
			"metadata":   metadata,
		})
	}

	return map[string]any{
		"results": formattedResults,
		"count":   len(formattedResults),
	}, nil
}

// Health returns the current health status of this tool.
func (t *KnowledgeSearchTool) Health(ctx context.Context) types.HealthStatus {
	if t.store == nil {
		return types.Degraded("Knowledge store not initialized")
	}
	return t.store.Health(ctx)
}
