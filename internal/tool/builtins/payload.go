package builtins

import (
	"context"
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/gibson/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
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

// InputMessageType returns the protobuf message type for input.
// Using google.protobuf.Struct as temporary type until task 4.1 creates PayloadSearchRequest.
func (t *PayloadSearchTool) InputMessageType() string {
	return "google.protobuf.Struct"
}

// OutputMessageType returns the protobuf message type for output.
// Using google.protobuf.Struct as temporary type until task 4.1 creates PayloadSearchResponse.
func (t *PayloadSearchTool) OutputMessageType() string {
	return "google.protobuf.Struct"
}

// ExecuteProto runs the tool with protobuf input and returns protobuf output.
func (t *PayloadSearchTool) ExecuteProto(ctx context.Context, input proto.Message) (proto.Message, error) {
	// Type-assert to structpb.Struct (temporary until task 4.1 creates PayloadSearchRequest)
	req, ok := input.(*structpb.Struct)
	if !ok {
		return nil, fmt.Errorf("expected *structpb.Struct, got %T", input)
	}

	// Handle nil registry gracefully
	if t.registry == nil {
		emptyResults, _ := structpb.NewStruct(map[string]interface{}{
			"results": []interface{}{},
			"count":   0,
		})
		return emptyResults, nil
	}

	// Build filter from proto struct fields
	filter := &payload.PayloadFilter{
		Enabled: boolPtr(true), // Only return enabled payloads
	}

	fields := req.GetFields()

	// Extract categories
	if categoriesVal, ok := fields["categories"]; ok {
		if categoriesList := categoriesVal.GetListValue(); categoriesList != nil {
			categories := make([]payload.PayloadCategory, 0)
			for _, cat := range categoriesList.GetValues() {
				if catStr := cat.GetStringValue(); catStr != "" {
					categories = append(categories, payload.PayloadCategory(catStr))
				}
			}
			filter.Categories = categories
		}
	}

	// Extract tags
	if tagsVal, ok := fields["tags"]; ok {
		if tagsList := tagsVal.GetListValue(); tagsList != nil {
			tags := make([]string, 0)
			for _, tag := range tagsList.GetValues() {
				if tagStr := tag.GetStringValue(); tagStr != "" {
					tags = append(tags, tagStr)
				}
			}
			filter.Tags = tags
		}
	}

	// Extract target type
	if targetTypeVal, ok := fields["target_type"]; ok {
		if targetTypeStr := targetTypeVal.GetStringValue(); targetTypeStr != "" {
			filter.TargetTypes = []string{targetTypeStr}
		}
	}

	// Extract severity
	if severityVal, ok := fields["severity"]; ok {
		if severityStr := severityVal.GetStringValue(); severityStr != "" {
			filter.Severities = []agent.FindingSeverity{agent.FindingSeverity(severityStr)}
		}
	}

	// Extract limit
	limit := 10
	if limitVal, ok := fields["limit"]; ok {
		if limitNum := limitVal.GetNumberValue(); limitNum > 0 {
			limit = int(limitNum)
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

	query := ""
	if queryVal, ok := fields["query"]; ok {
		query = queryVal.GetStringValue()
	}

	if query != "" {
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
	formattedResults := make([]interface{}, 0, len(payloads))
	for _, p := range payloads {
		// Convert categories to strings
		categoryStrs := make([]interface{}, len(p.Categories))
		for i, cat := range p.Categories {
			categoryStrs[i] = cat.String()
		}

		// Get target types
		targetTypes := make([]interface{}, 0)
		if p.TargetTypes != nil {
			for _, tt := range p.TargetTypes {
				targetTypes = append(targetTypes, tt)
			}
		}

		formattedResults = append(formattedResults, map[string]interface{}{
			"id":           p.ID.String(),
			"name":         p.Name,
			"description":  p.Description,
			"categories":   categoryStrs,
			"severity":     string(p.Severity),
			"reliability":  p.Metadata.Reliability,
			"target_types": targetTypes,
		})
	}

	// Create response as structpb.Struct
	response, err := structpb.NewStruct(map[string]interface{}{
		"results": formattedResults,
		"count":   len(formattedResults),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create response struct: %w", err)
	}

	return response, nil
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

// InputMessageType returns the protobuf message type for input.
// Using google.protobuf.Struct as temporary type until task 4.1 creates PayloadExecuteRequest.
func (t *PayloadExecuteTool) InputMessageType() string {
	return "google.protobuf.Struct"
}

// OutputMessageType returns the protobuf message type for output.
// Using google.protobuf.Struct as temporary type until task 4.1 creates PayloadExecuteResponse.
func (t *PayloadExecuteTool) OutputMessageType() string {
	return "google.protobuf.Struct"
}

// ExecuteProto runs the tool with protobuf input and returns protobuf output.
func (t *PayloadExecuteTool) ExecuteProto(ctx context.Context, input proto.Message) (proto.Message, error) {
	// Type-assert to structpb.Struct (temporary until task 4.1 creates PayloadExecuteRequest)
	req, ok := input.(*structpb.Struct)
	if !ok {
		return nil, fmt.Errorf("expected *structpb.Struct, got %T", input)
	}
	// Handle nil executor gracefully
	if t.executor == nil {
		return nil, fmt.Errorf("payload executor not initialized")
	}

	fields := req.GetFields()

	// Extract payload_id (required)
	payloadIDVal, ok := fields["payload_id"]
	if !ok {
		return nil, fmt.Errorf("payload_id parameter is required")
	}
	payloadIDStr := payloadIDVal.GetStringValue()
	if payloadIDStr == "" {
		return nil, fmt.Errorf("payload_id parameter must be a non-empty string")
	}
	payloadID := types.ID(payloadIDStr)

	// Extract target (required)
	targetVal, ok := fields["target"]
	if !ok {
		return nil, fmt.Errorf("target parameter is required")
	}
	target := targetVal.GetStringValue()
	if target == "" {
		return nil, fmt.Errorf("target parameter must be a non-empty string")
	}
	targetID := types.ID(target)

	// Extract optional params
	params := make(map[string]interface{})
	if paramsVal, ok := fields["params"]; ok {
		if paramsStruct := paramsVal.GetStructValue(); paramsStruct != nil {
			// Convert structpb.Struct to map[string]interface{}
			for k, v := range paramsStruct.GetFields() {
				params[k] = v.AsInterface()
			}
		}
	}

	// Extract timeout
	timeout := 30 * time.Second
	if timeoutVal, ok := fields["timeout"]; ok {
		if timeoutNum := timeoutVal.GetNumberValue(); timeoutNum > 0 {
			timeout = time.Duration(timeoutNum) * time.Second
		}
	}

	// Build execution request
	execReq := &payload.ExecutionRequest{
		PayloadID:  payloadID,
		TargetID:   targetID,
		Parameters: params,
		Timeout:    timeout,
		AgentID:    types.ID("builtin-tool"), // Mark as tool execution
	}

	// Execute payload
	result, err := t.executor.Execute(ctx, execReq)
	if err != nil {
		return nil, fmt.Errorf("payload execution failed: %w", err)
	}

	// Format execution details
	execution := map[string]interface{}{
		"duration_ms": float64(result.ResponseTime.Milliseconds()),
	}
	if result.Response != "" {
		execution["response"] = result.Response
	}

	// Format indicators matched (list of objects)
	indicatorsMatched := make([]interface{}, len(result.IndicatorsMatched))
	for i, indicator := range result.IndicatorsMatched {
		indicatorsMatched[i] = map[string]interface{}{
			"indicator": indicator,
			"matched":   true,
		}
	}

	// Build output
	output := map[string]interface{}{
		"success":            result.Success,
		"confidence":         result.ConfidenceScore,
		"execution":          execution,
		"indicators_matched": indicatorsMatched,
	}

	// Add finding details if attack succeeded and created a finding
	if result.Success && result.Finding != nil {
		output["suggested_finding"] = map[string]interface{}{
			"title":       result.Finding.Title,
			"description": result.Finding.Description,
			"severity":    string(result.Finding.Severity),
			"category":    string(result.Finding.Category),
		}
	}

	// Create response as structpb.Struct
	response, err := structpb.NewStruct(output)
	if err != nil {
		return nil, fmt.Errorf("failed to create response struct: %w", err)
	}

	return response, nil
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
