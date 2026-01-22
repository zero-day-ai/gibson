package builtins

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/knowledge"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
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

// InputMessageType returns the fully-qualified proto message type name for input.
// Uses google.protobuf.Struct as temporary type until task 4.1 completes.
func (t *KnowledgeSearchTool) InputMessageType() string {
	return "google.protobuf.Struct"
}

// OutputMessageType returns the fully-qualified proto message type name for output.
// Uses google.protobuf.Struct as temporary type until task 4.1 completes.
func (t *KnowledgeSearchTool) OutputMessageType() string {
	return "google.protobuf.Struct"
}

// ExecuteProto runs the tool with proto message input and returns proto message output.
// Uses google.protobuf.Struct as temporary type until task 4.1 completes with proper proto definitions.
func (t *KnowledgeSearchTool) ExecuteProto(ctx context.Context, input proto.Message) (proto.Message, error) {
	// Type-assert input to *structpb.Struct
	inputStruct, ok := input.(*structpb.Struct)
	if !ok {
		return nil, fmt.Errorf("input must be *structpb.Struct, got %T", input)
	}

	// Convert struct to map for easier access
	inputMap := inputStruct.AsMap()

	// Handle nil store gracefully (knowledge store not initialized)
	if t.store == nil {
		emptyResult, err := structpb.NewStruct(map[string]any{
			"results": []any{},
			"count":   0,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create empty result: %w", err)
		}
		return emptyResult, nil
	}

	// Extract query parameter (required)
	query, ok := inputMap["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required and must be a non-empty string")
	}

	// Extract optional parameters with defaults
	limit := 10
	if limitVal, ok := inputMap["limit"]; ok {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	threshold := 0.7
	if thresholdVal, ok := inputMap["threshold"]; ok {
		if v, ok := thresholdVal.(float64); ok {
			threshold = v
		}
	}

	source := ""
	if sourceVal, ok := inputMap["source"]; ok {
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
		emptyResult, structErr := structpb.NewStruct(map[string]any{
			"results": []any{},
			"count":   0,
		})
		if structErr != nil {
			return nil, fmt.Errorf("failed to create empty result after search error: %w", structErr)
		}
		return emptyResult, nil
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

	// Convert results to proto Struct
	outputStruct, err := structpb.NewStruct(map[string]any{
		"results": formattedResults,
		"count":   len(formattedResults),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create output struct: %w", err)
	}

	return outputStruct, nil
}

// Health returns the current health status of this tool.
func (t *KnowledgeSearchTool) Health(ctx context.Context) types.HealthStatus {
	if t.store == nil {
		return types.Degraded("Knowledge store not initialized")
	}
	return t.store.Health(ctx)
}
