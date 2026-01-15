package taxonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zero-day-ai/sdk/schema"
)

// SchemaBasedLoader loads taxonomy definitions from tool binaries by executing
// them with the --schema flag and extracting taxonomy mappings from the output schema.
// Tools are expected to be in the Gibson tools directory (default: ~/.gibson/tools/).
type SchemaBasedLoader struct {
	toolsDir string
	logger   *slog.Logger
}

// NewSchemaBasedLoader creates a new loader that reads taxonomy from tool binaries.
// toolsDir is the Gibson tools directory (e.g., ~/.gibson/tools/).
func NewSchemaBasedLoader(toolsDir string, logger *slog.Logger) *SchemaBasedLoader {
	if logger == nil {
		logger = slog.Default()
	}
	return &SchemaBasedLoader{
		toolsDir: toolsDir,
		logger:   logger,
	}
}

// ToolSchemaOutput represents the output of running a tool with --schema flag.
// This matches the structure that tools return when called with --schema.
type ToolSchemaOutput struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Output      schema.JSON `json:"output_schema"` // SDK uses output_schema key
}

// LoadTool executes the tool with --schema flag and extracts taxonomy mappings.
// The tool is expected to be in the Gibson tools directory.
// It returns a ToolOutputSchema that can be added to the taxonomy, or nil if no taxonomy found.
func (l *SchemaBasedLoader) LoadTool(ctx context.Context, toolName string) (*ToolOutputSchema, error) {
	// Build full path to tool in Gibson tools directory
	toolPath := filepath.Join(l.toolsDir, toolName)

	// Execute tool with --schema flag
	cmd := exec.CommandContext(ctx, toolPath, "--schema")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("tool execution failed: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute tool: %w", err)
	}

	// Parse the JSON output
	var toolSchema ToolSchemaOutput
	if err := json.Unmarshal(output, &toolSchema); err != nil {
		return nil, fmt.Errorf("failed to parse tool schema JSON: %w", err)
	}

	// Use tool name from schema if available, otherwise use the command name
	name := toolSchema.Name
	if name == "" {
		name = toolName
	}

	// Extract taxonomy mappings from the schema tree
	extracts := extractTaxonomyMappings(toolSchema.Output, "$")

	if len(extracts) == 0 {
		l.logger.Debug("no taxonomy mappings found in tool schema", "tool", name)
		return nil, nil
	}

	// Build ToolOutputSchema
	outputSchema := &ToolOutputSchema{
		Tool:         name,
		Description:  toolSchema.Description,
		OutputFormat: "json", // Tools output JSON
		Extracts:     extracts,
	}

	l.logger.Info("loaded taxonomy from tool", "tool", name, "extracts", len(extracts))

	return outputSchema, nil
}

// LoadTools loads taxonomy from a list of tools by name.
// Tools are expected to be in the Gibson tools directory.
// Returns a map of tool name to ToolOutputSchema. Tools that fail to load or have no taxonomy are skipped.
func (l *SchemaBasedLoader) LoadTools(ctx context.Context, toolNames []string) (map[string]*ToolOutputSchema, error) {
	schemas := make(map[string]*ToolOutputSchema)

	for _, toolName := range toolNames {
		toolSchema, err := l.LoadTool(ctx, toolName)
		if err != nil {
			l.logger.Warn("failed to load schema from tool",
				"tool", toolName,
				"error", err)
			continue // Continue with other tools
		}

		// Skip if no taxonomy found
		if toolSchema == nil {
			continue
		}

		schemas[toolSchema.Tool] = toolSchema
	}

	l.logger.Info("loaded taxonomy from tools",
		"requested", len(toolNames),
		"loaded", len(schemas))

	return schemas, nil
}

// LoadAllTools scans the tools directory and loads taxonomy from all tool binaries found.
// Returns a map of tool name to ToolOutputSchema. Tools that fail to load or have no taxonomy are skipped.
func (l *SchemaBasedLoader) LoadAllTools(ctx context.Context) (map[string]*ToolOutputSchema, error) {
	schemas := make(map[string]*ToolOutputSchema)

	// Check if tools directory exists
	if _, err := os.Stat(l.toolsDir); os.IsNotExist(err) {
		l.logger.Warn("tools directory does not exist", "dir", l.toolsDir)
		return schemas, nil
	}

	// Walk through tools directory
	err := filepath.Walk(l.toolsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip non-executable files
		if info.Mode()&0111 == 0 {
			return nil
		}

		// Skip files with extensions (likely not binaries)
		if filepath.Ext(path) != "" {
			return nil
		}

		// Get tool name from filename
		toolName := filepath.Base(path)

		// Try to load schema from this binary
		toolSchema, err := l.LoadTool(ctx, toolName)
		if err != nil {
			l.logger.Warn("failed to load schema from tool",
				"tool", toolName,
				"error", err)
			return nil // Continue with other tools
		}

		// Skip if no taxonomy found
		if toolSchema == nil {
			return nil
		}

		schemas[toolSchema.Tool] = toolSchema
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk tools directory: %w", err)
	}

	l.logger.Info("loaded taxonomy from tools directory",
		"tools_dir", l.toolsDir,
		"tools_found", len(schemas))

	return schemas, nil
}

// extractTaxonomyMappings recursively walks the schema tree and extracts TaxonomyMapping
// when found, converting them to ToolOutputExtraction format.
func extractTaxonomyMappings(s schema.JSON, jsonPath string) []ToolOutputExtraction {
	var extracts []ToolOutputExtraction

	// If this schema has a taxonomy mapping, extract it
	if s.Taxonomy != nil {
		extract := convertTaxonomyToExtraction(s.Taxonomy, jsonPath)
		extracts = append(extracts, extract)
	}

	// Recursively process object properties
	if s.Type == "object" && len(s.Properties) > 0 {
		for propName, propSchema := range s.Properties {
			// Build JSONPath for this property
			propPath := fmt.Sprintf("%s.%s", jsonPath, propName)
			childExtracts := extractTaxonomyMappings(propSchema, propPath)
			extracts = append(extracts, childExtracts...)
		}
	}

	// Recursively process array items
	if s.Type == "array" && s.Items != nil {
		// Array items use [*] in JSONPath to iterate
		itemPath := fmt.Sprintf("%s[*]", jsonPath)
		childExtracts := extractTaxonomyMappings(*s.Items, itemPath)
		extracts = append(extracts, childExtracts...)
	}

	return extracts
}

// convertTaxonomyToExtraction converts a schema.TaxonomyMapping to a ToolOutputExtraction.
func convertTaxonomyToExtraction(taxonomy *schema.TaxonomyMapping, jsonPath string) ToolOutputExtraction {
	extract := ToolOutputExtraction{
		NodeType:   taxonomy.NodeType,
		JSONPath:   jsonPath,
		IDTemplate: taxonomy.IDTemplate,
	}

	// Convert property mappings
	if len(taxonomy.Properties) > 0 {
		extract.Properties = make([]ToolOutputProperty, 0, len(taxonomy.Properties))
		for _, prop := range taxonomy.Properties {
			outputProp := ToolOutputProperty{
				// Build JSONPath from current path + source field
				JSONPath: fmt.Sprintf("%s.%s", jsonPath, prop.Source),
				Target:   prop.Target,
				Default:  prop.Default,
			}

			// If transform is specified, encode it as a template
			if prop.Transform != "" {
				outputProp.Template = fmt.Sprintf("{{.%s | %s}}", prop.Source, prop.Transform)
			}

			extract.Properties = append(extract.Properties, outputProp)
		}
	}

	// Convert relationship mappings
	if len(taxonomy.Relationships) > 0 {
		extract.Relationships = make([]ToolOutputRelationship, 0, len(taxonomy.Relationships))
		for _, rel := range taxonomy.Relationships {
			outputRel := ToolOutputRelationship{
				Type:         rel.Type,
				FromTemplate: rel.FromTemplate,
				ToTemplate:   rel.ToTemplate,
			}

			// Convert relationship properties
			if len(rel.Properties) > 0 {
				outputRel.Properties = make([]RelationshipProperty, 0, len(rel.Properties))
				for _, relProp := range rel.Properties {
					outputRel.Properties = append(outputRel.Properties, RelationshipProperty{
						Name:  relProp.Target,
						Value: fmt.Sprintf("{{.%s}}", relProp.Source),
					})
				}
			}

			extract.Relationships = append(extract.Relationships, outputRel)
		}
	}

	return extract
}
