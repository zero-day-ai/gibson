// Package engine provides the GraphRAG extraction engine components.
package engine

import (
	"fmt"
	"strings"

	"github.com/ohler55/ojg/jp"
)

// JSONPathExtractor extracts data from JSON using JSONPath expressions.
// It supports standard JSONPath syntax as well as special references:
//   - _parent: Reference to parent object in nested extractions
//   - _context: Injected context (agent_run_id, timestamp, etc.)
//   - _root: Reference to root JSON object
type JSONPathExtractor struct{}

// NewJSONPathExtractor creates a new JSONPath extractor.
func NewJSONPathExtractor() *JSONPathExtractor {
	return &JSONPathExtractor{}
}

// ExtractedItem contains extracted data with parent context.
// This is used for nested extractions where child items need to reference parent data.
type ExtractedItem struct {
	Value  any            // The extracted value
	Parent map[string]any // Parent object for _parent references
	Index  int            // Index in array (if from array)
}

// Extract extracts values matching the JSONPath from data.
// Returns a slice of extracted items (could be multiple matches).
//
// Supported JSONPath patterns:
//   - $.hosts[*]         - All items in hosts array
//   - $.hosts[*].ports   - Nested property access
//   - $[0]               - Array index access
//   - $.field            - Direct field access
//
// Returns empty slice if no matches found (not an error).
func (e *JSONPathExtractor) Extract(jsonPath string, data map[string]any) ([]any, error) {
	// Handle relative paths (converted by caller to absolute)
	if strings.HasPrefix(jsonPath, ".") {
		return nil, fmt.Errorf("relative paths not supported in Extract, use ExtractRelative")
	}

	// Parse the JSONPath expression
	expr, err := jp.ParseString(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("invalid JSONPath expression %q: %w", jsonPath, err)
	}

	// Execute the expression
	results := expr.Get(data)

	// Return empty slice if no results (not an error)
	if len(results) == 0 {
		return []any{}, nil
	}

	return results, nil
}

// ExtractSingle extracts a single value (for property mappings).
// If multiple values match, returns the first one.
// Returns nil if no matches found (not an error).
func (e *JSONPathExtractor) ExtractSingle(jsonPath string, data map[string]any) (any, error) {
	results, err := e.Extract(jsonPath, data)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ExtractRelative extracts a value using a relative path (starts with .)
// from a specific context object.
//
// Examples:
//   - .ip          - Get ip field from current object
//   - .service.name - Get nested service.name field
//   - .ports[0]    - Get first element of ports array
func (e *JSONPathExtractor) ExtractRelative(relativePath string, context map[string]any) (any, error) {
	if !strings.HasPrefix(relativePath, ".") {
		return nil, fmt.Errorf("relative path must start with '.', got %q", relativePath)
	}

	// Convert relative path to absolute path
	// .ip -> $.ip
	// .service.name -> $.service.name
	absolutePath := "$" + relativePath

	// Parse and execute
	expr, err := jp.ParseString(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("invalid relative path %q: %w", relativePath, err)
	}

	results := expr.Get(context)
	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// ExtractWithParent extracts items and maintains parent reference.
// Used for nested extractions like $.hosts[*].ports[*] where each port
// needs to reference its parent host.
//
// The extraction process:
//  1. For paths with [*], splits into parent and child paths
//  2. Extracts parent items first (e.g., $.hosts[*])
//  3. For each parent, extracts child items (e.g., .ports[*])
//  4. Returns ExtractedItem with both value and parent reference
//
// Example:
//
//	Input: $.hosts[*].ports[*]
//	Returns: Each port with its parent host available as _parent
func (e *JSONPathExtractor) ExtractWithParent(jsonPath string, data map[string]any) ([]ExtractedItem, error) {
	// Parse the path to identify nesting levels
	// $.hosts[*].ports[*] -> ["hosts", "ports"]
	// $.hosts[*].ports[*].service -> ["hosts", "ports", "service"]
	arrayPaths := e.extractArrayPaths(jsonPath)

	if len(arrayPaths) == 0 {
		// No array iteration, just extract normally
		results, err := e.Extract(jsonPath, data)
		if err != nil {
			return nil, err
		}

		items := make([]ExtractedItem, len(results))
		for i, result := range results {
			// Try to convert to map for parent reference
			var parent map[string]any
			if m, ok := result.(map[string]any); ok {
				parent = m
			}
			items[i] = ExtractedItem{
				Value:  result,
				Parent: parent,
				Index:  i,
			}
		}
		return items, nil
	}

	// Handle nested array iteration
	return e.extractNestedWithParent(data, arrayPaths, 0, nil)
}

// extractArrayPaths identifies array iteration segments in a JSONPath.
// Example: $.hosts[*].ports[*] -> ["$.hosts[*]", "ports[*]"]
func (e *JSONPathExtractor) extractArrayPaths(jsonPath string) []string {
	var paths []string

	// Split on [*] to identify array iterations
	parts := strings.Split(jsonPath, "[*]")
	if len(parts) <= 1 {
		return paths
	}

	// Build path segments
	for i := 0; i < len(parts)-1; i++ {
		if i == 0 {
			// First segment: $.hosts[*]
			paths = append(paths, parts[i]+"[*]")
		} else {
			// Subsequent segments: .ports[*], .service[*]
			// Remove leading dot if present
			segment := strings.TrimPrefix(parts[i], ".")
			if segment != "" {
				paths = append(paths, segment+"[*]")
			} else {
				// Handle case like $.hosts[*][*] (double array)
				paths = append(paths, "[*]")
			}
		}
	}

	return paths
}

// extractNestedWithParent recursively extracts nested array items with parent references.
func (e *JSONPathExtractor) extractNestedWithParent(
	data any,
	arrayPaths []string,
	level int,
	parent map[string]any,
) ([]ExtractedItem, error) {
	if level >= len(arrayPaths) {
		// Base case: extract the final value
		return []ExtractedItem{{
			Value:  data,
			Parent: parent,
			Index:  0,
		}}, nil
	}

	currentPath := arrayPaths[level]
	var results []ExtractedItem

	// Determine if this is the first level or a relative path
	var items []any
	var err error

	if level == 0 {
		// First level: absolute path like $.hosts[*]
		if m, ok := data.(map[string]any); ok {
			items, err = e.Extract(currentPath, m)
		} else {
			return nil, fmt.Errorf("root data must be a map for path %q", currentPath)
		}
	} else {
		// Nested level: relative path like .ports[*]
		// Convert to absolute for extraction from parent context
		if parent == nil {
			return nil, fmt.Errorf("parent context required for nested path %q", currentPath)
		}

		// Remove [*] and get the field
		fieldName := strings.TrimSuffix(currentPath, "[*]")
		fieldValue, exists := parent[fieldName]
		if !exists {
			// Field doesn't exist, return empty (not an error)
			return []ExtractedItem{}, nil
		}

		// Field should be an array
		if arr, ok := fieldValue.([]any); ok {
			items = arr
		} else {
			// Single item, wrap in array
			items = []any{fieldValue}
		}
	}

	if err != nil {
		return nil, err
	}

	// Process each item at this level
	for idx, item := range items {
		// Convert to map for parent reference
		var itemMap map[string]any
		if m, ok := item.(map[string]any); ok {
			itemMap = m
		}

		if level == len(arrayPaths)-1 {
			// Last level: return the items with parent reference
			results = append(results, ExtractedItem{
				Value:  item,
				Parent: parent,
				Index:  idx,
			})
		} else {
			// Continue to next level
			childResults, err := e.extractNestedWithParent(item, arrayPaths, level+1, itemMap)
			if err != nil {
				return nil, err
			}
			results = append(results, childResults...)
		}
	}

	return results, nil
}

// GetNestedValue retrieves a value from a nested map structure using dot notation.
// Supports _parent references for accessing parent object fields.
//
// Examples:
//   - "ip" from {"ip": "1.2.3.4"}
//   - "service.name" from {"service": {"name": "http"}}
//   - "_parent.ip" from parent context
func (e *JSONPathExtractor) GetNestedValue(path string, current map[string]any, parent map[string]any) (any, error) {
	// Handle _parent references
	if strings.HasPrefix(path, "_parent.") {
		if parent == nil {
			return nil, fmt.Errorf("_parent reference used but parent context is nil")
		}
		// Remove _parent. prefix and recurse
		remainingPath := strings.TrimPrefix(path, "_parent.")

		// Check for nested _parent references (e.g., _parent._parent.ip)
		if strings.HasPrefix(remainingPath, "_parent.") {
			// This would require tracking grandparent, which we don't support yet
			return nil, fmt.Errorf("nested _parent._parent references not supported: %q", path)
		}

		return e.GetNestedValue(remainingPath, parent, nil)
	}

	// Handle _parent without dot (entire parent object)
	if path == "_parent" {
		if parent == nil {
			return nil, fmt.Errorf("_parent reference used but parent context is nil")
		}
		return parent, nil
	}

	// Handle regular field access
	parts := strings.Split(path, ".")
	result := any(current)

	for _, part := range parts {
		if part == "" {
			continue
		}

		m, ok := result.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot access field %q: not a map", part)
		}

		value, exists := m[part]
		if !exists {
			// Field doesn't exist, return nil (not an error)
			return nil, nil
		}

		result = value
	}

	return result, nil
}
