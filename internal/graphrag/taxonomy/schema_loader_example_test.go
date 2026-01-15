package taxonomy_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
)

// ExampleSchemaBasedLoader demonstrates loading taxonomy from tool binaries.
func ExampleSchemaBasedLoader() {
	// Create a schema-based loader pointing to the tools directory
	toolsDir := "/opt/gibson/tools"
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loader := taxonomy.NewSchemaBasedLoader(toolsDir, logger)

	// Load taxonomy from all tools in the directory
	ctx := context.Background()
	schemas, err := loader.LoadAllTools(ctx)
	if err != nil {
		fmt.Printf("Error loading tools: %v\n", err)
		return
	}

	// Print summary of loaded schemas
	fmt.Printf("Loaded taxonomy from %d tools:\n", len(schemas))
	for toolName, schema := range schemas {
		fmt.Printf("- %s: %d extraction rules\n", toolName, len(schema.Extracts))
	}
}

// ExampleSchemaBasedLoader_LoadTool demonstrates loading taxonomy from a single tool binary.
func ExampleSchemaBasedLoader_LoadTool() {
	// Create a schema-based loader pointing to the Gibson tools directory
	toolsDir := "/opt/gibson/tools"
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loader := taxonomy.NewSchemaBasedLoader(toolsDir, logger)

	// Load from a specific tool by name (tool is in toolsDir)
	ctx := context.Background()
	schema, err := loader.LoadTool(ctx, "nmap")
	if err != nil {
		fmt.Printf("Error loading tool: %v\n", err)
		return
	}

	if schema == nil {
		fmt.Println("Tool has no taxonomy mappings")
		return
	}

	// Display the loaded schema
	fmt.Printf("Tool: %s\n", schema.Tool)
	fmt.Printf("Description: %s\n", schema.Description)
	fmt.Printf("Extraction rules:\n")
	for _, extract := range schema.Extracts {
		fmt.Printf("  - Node type: %s (JSONPath: %s)\n", extract.NodeType, extract.JSONPath)
		fmt.Printf("    ID template: %s\n", extract.IDTemplate)
		fmt.Printf("    Properties: %d\n", len(extract.Properties))
		fmt.Printf("    Relationships: %d\n", len(extract.Relationships))
	}
}

// ExampleSchemaBasedLoader_integration demonstrates integrating schema-based loading
// with the existing taxonomy system.
func ExampleSchemaBasedLoader_integration() {
	// Load the base taxonomy
	baseTaxonomy, err := taxonomy.LoadTaxonomy()
	if err != nil {
		fmt.Printf("Error loading base taxonomy: %v\n", err)
		return
	}

	// Create a schema-based loader to load tool-specific schemas
	toolsDir := "/opt/gibson/tools"
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loader := taxonomy.NewSchemaBasedLoader(toolsDir, logger)

	// Load all tool schemas
	ctx := context.Background()
	toolSchemas, err := loader.LoadAllTools(ctx)
	if err != nil {
		fmt.Printf("Error loading tool schemas: %v\n", err)
		return
	}

	// Add each tool schema to the taxonomy
	for _, schema := range toolSchemas {
		if err := baseTaxonomy.AddToolOutputSchema(schema); err != nil {
			fmt.Printf("Error adding tool schema %s: %v\n", schema.Tool, err)
			continue
		}
	}

	// Create a registry for efficient querying
	registry, err := taxonomy.NewTaxonomyRegistry(baseTaxonomy)
	if err != nil {
		fmt.Printf("Error creating registry: %v\n", err)
		return
	}

	// Now you can query the registry
	fmt.Printf("Total tools with taxonomy: %d\n", len(registry.ListToolOutputSchemas()))
}
