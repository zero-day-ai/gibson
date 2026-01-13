// Package engine provides the core template interpolation engine for the TaxonomyGraphEngine.
// It handles template variable substitution for node IDs and properties, supporting nested
// field access, array indexing, and special context variables.
package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TemplateEngine handles template interpolation for node IDs and properties.
// It supports various placeholder formats including simple fields, nested access,
// array indexing, and special context variables.
type TemplateEngine struct {
	// placeholderRegex matches template placeholders like {field}, {parent.child}, {items[0]}
	placeholderRegex *regexp.Regexp
}

// NewTemplateEngine creates a new template engine with compiled regex patterns.
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{
		// Match {placeholder} patterns, supporting dots, brackets, and underscores
		placeholderRegex: regexp.MustCompile(`\{([a-zA-Z0-9_.\[\]]+)\}`),
	}
}

// Interpolate replaces placeholders in template with values from data.
//
// Template format examples:
//   - "mission:{mission_id}" + {"mission_id": "abc"} → "mission:abc"
//   - "agent_run:{trace_id}:{span_id}" + {"trace_id": "t1", "span_id": "s1"} → "agent_run:t1:s1"
//   - "host:{ip}" + {"ip": "1.2.3.4"} → "host:1.2.3.4"
//
// Returns an error if any required placeholder cannot be resolved.
func (t *TemplateEngine) Interpolate(template string, data map[string]any) (string, error) {
	return t.InterpolateWithContext(template, data, nil)
}

// InterpolateWithContext interpolates with additional context variables.
// Context variables are accessible via the _context prefix (e.g., {_context.agent_id}).
// Parent variables are accessible via the _parent prefix (e.g., {_parent.ip}).
//
// The function performs multiple passes to resolve all placeholders, supporting
// nested field access and type coercion.
func (t *TemplateEngine) InterpolateWithContext(template string, data map[string]any, context map[string]any) (string, error) {
	if template == "" {
		return "", nil
	}

	// Merge context into data with special prefixes
	mergedData := make(map[string]any)
	for k, v := range data {
		mergedData[k] = v
	}

	if context != nil {
		// Create _context and _parent as nested maps
		contextMap := make(map[string]any)
		parentMap := make(map[string]any)

		for k, v := range context {
			contextMap[k] = v
			// Also support _parent prefix for backwards compatibility
			if strings.HasPrefix(k, "parent_") {
				parentMap[strings.TrimPrefix(k, "parent_")] = v
			}
		}

		mergedData["_context"] = contextMap
		if len(parentMap) > 0 {
			mergedData["_parent"] = parentMap
		}
	}

	// Replace all placeholders
	result := template
	var lastError error

	result = t.placeholderRegex.ReplaceAllStringFunc(result, func(match string) string {
		// Extract the field path from {field.path}
		fieldPath := strings.Trim(match, "{}")

		// Resolve the value
		value, err := t.resolveValue(fieldPath, mergedData)
		if err != nil {
			lastError = fmt.Errorf("failed to resolve placeholder {%s}: %w", fieldPath, err)
			return match // Keep the original placeholder on error
		}

		// Convert value to string
		strValue, err := t.valueToString(value)
		if err != nil {
			lastError = fmt.Errorf("failed to convert placeholder {%s} to string: %w", fieldPath, err)
			return match
		}

		return strValue
	})

	if lastError != nil {
		return "", lastError
	}

	return result, nil
}

// resolveValue resolves a field path to a value from the data map.
// Supports:
//   - Simple fields: "field_name"
//   - Nested fields: "parent.child.value"
//   - Array indexing: "items[0]", "items[0].name"
//   - JSONPath-style: ".field" (relative path, same as "field")
func (t *TemplateEngine) resolveValue(fieldPath string, data map[string]any) (any, error) {
	// Handle JSONPath-style relative paths
	if strings.HasPrefix(fieldPath, ".") {
		fieldPath = strings.TrimPrefix(fieldPath, ".")
	}

	// Check if it's a simple field (no dots or brackets)
	if !strings.Contains(fieldPath, ".") && !strings.Contains(fieldPath, "[") {
		value, exists := data[fieldPath]
		if !exists {
			return nil, fmt.Errorf("field %q not found in data", fieldPath)
		}
		return value, nil
	}

	// Parse complex path (nested fields and array indexing)
	return t.resolveComplexPath(fieldPath, data)
}

// resolveComplexPath resolves nested field paths and array indexing.
// Examples:
//   - "host.ip" → data["host"].(map)["ip"]
//   - "items[0]" → data["items"].([]any)[0]
//   - "items[0].name" → data["items"].([]any)[0].(map)["name"]
func (t *TemplateEngine) resolveComplexPath(fieldPath string, data map[string]any) (any, error) {
	// Split path into segments, handling both dots and brackets
	segments := t.parseFieldPath(fieldPath)
	if len(segments) == 0 {
		return nil, fmt.Errorf("invalid field path: %q", fieldPath)
	}

	// Start with the root data
	var current any = data

	// Traverse each segment
	for i, segment := range segments {
		// Handle array indexing
		if segment.isArray {
			current = t.resolveArrayIndex(current, segment.index)
			if current == nil {
				return nil, fmt.Errorf("array index %d out of bounds in path %q at segment %d",
					segment.index, fieldPath, i)
			}
			continue
		}

		// Handle map/object field access
		currentMap, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot access field %q on non-object type %T in path %q at segment %d",
				segment.field, current, fieldPath, i)
		}

		value, exists := currentMap[segment.field]
		if !exists {
			return nil, fmt.Errorf("field %q not found in path %q at segment %d",
				segment.field, fieldPath, i)
		}

		current = value
	}

	return current, nil
}

// pathSegment represents a segment in a field path.
type pathSegment struct {
	field   string // Field name (for map access)
	isArray bool   // Whether this is an array index
	index   int    // Array index (if isArray is true)
}

// parseFieldPath parses a field path into segments.
// Examples:
//   - "host.ip" → [{"host", false, 0}, {"ip", false, 0}]
//   - "items[0]" → [{"items", false, 0}, {"", true, 0}]
//   - "items[0].name" → [{"items", false, 0}, {"", true, 0}, {"name", false, 0}]
func (t *TemplateEngine) parseFieldPath(fieldPath string) []pathSegment {
	var segments []pathSegment

	// Regex to match field names and array indices
	// Matches: "field", "[0]", ".field", etc.
	segmentRegex := regexp.MustCompile(`([a-zA-Z0-9_]+)|\[(\d+)\]`)
	matches := segmentRegex.FindAllStringSubmatch(fieldPath, -1)

	for _, match := range matches {
		if match[1] != "" {
			// Field name
			segments = append(segments, pathSegment{
				field:   match[1],
				isArray: false,
			})
		} else if match[2] != "" {
			// Array index
			index, _ := strconv.Atoi(match[2])
			segments = append(segments, pathSegment{
				isArray: true,
				index:   index,
			})
		}
	}

	return segments
}

// resolveArrayIndex resolves an array index operation on a value.
// Supports both []any and []interface{} types.
func (t *TemplateEngine) resolveArrayIndex(value any, index int) any {
	if value == nil {
		return nil
	}

	// Handle []any
	if arr, ok := value.([]any); ok {
		if index < 0 || index >= len(arr) {
			return nil
		}
		return arr[index]
	}

	// Handle []interface{}
	if arr, ok := value.([]interface{}); ok {
		if index < 0 || index >= len(arr) {
			return nil
		}
		return arr[index]
	}

	return nil
}

// valueToString converts a value to its string representation.
// Handles various types including primitives, arrays, and maps.
func (t *TemplateEngine) valueToString(value any) (string, error) {
	if value == nil {
		return "", fmt.Errorf("cannot convert nil to string")
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		// Check if it's a whole number
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case fmt.Stringer:
		return v.String(), nil
	default:
		return "", fmt.Errorf("cannot convert type %T to string", value)
	}
}
