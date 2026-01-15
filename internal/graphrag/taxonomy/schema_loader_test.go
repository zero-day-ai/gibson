package taxonomy

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zero-day-ai/sdk/schema"
)

func TestSchemaBasedLoader_LoadTool(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool binary
	mockToolPath := filepath.Join(tmpDir, "mock-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
{
  "name": "mock-tool",
  "description": "A mock tool for testing",
  "output_schema": {
    "type": "object",
    "properties": {
      "hosts": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "hostname": {
              "type": "string"
            },
            "ip": {
              "type": "string"
            },
            "ports": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "port": {
                    "type": "integer"
                  },
                  "service": {
                    "type": "string"
                  }
                },
                "taxonomy": {
                  "node_type": "port",
                  "id_template": "port:{{.ip}}:{{.port}}",
                  "properties": [
                    {
                      "source": "port",
                      "target": "port_number"
                    },
                    {
                      "source": "service",
                      "target": "service_name"
                    }
                  ],
                  "relationships": [
                    {
                      "type": "RUNS_ON",
                      "from_template": "port:{{.ip}}:{{.port}}",
                      "to_template": "host:{{.ip}}"
                    }
                  ]
                }
              }
            }
          },
          "taxonomy": {
            "node_type": "host",
            "id_template": "host:{{.ip}}",
            "properties": [
              {
                "source": "hostname",
                "target": "hostname"
              },
              {
                "source": "ip",
                "target": "ip_address"
              }
            ]
          }
        }
      }
    }
  }
}
EOF
fi
`

	// Write the script
	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Load from binary
	ctx := context.Background()
	toolSchema, err := loader.LoadTool(ctx, "mock-tool")
	if err != nil {
		t.Fatalf("LoadTool failed: %v", err)
	}

	// Verify tool schema
	if toolSchema == nil {
		t.Fatal("expected non-nil tool schema")
	}

	if toolSchema.Tool != "mock-tool" {
		t.Errorf("expected tool name 'mock-tool', got '%s'", toolSchema.Tool)
	}

	if toolSchema.OutputFormat != "json" {
		t.Errorf("expected output format 'json', got '%s'", toolSchema.OutputFormat)
	}

	// Verify extracts
	if len(toolSchema.Extracts) != 2 {
		t.Fatalf("expected 2 extracts, got %d", len(toolSchema.Extracts))
	}

	// Check first extract (host)
	hostExtract := toolSchema.Extracts[0]
	if hostExtract.NodeType != "host" {
		t.Errorf("expected node type 'host', got '%s'", hostExtract.NodeType)
	}

	if hostExtract.JSONPath != "$.hosts[*]" {
		t.Errorf("expected JSONPath '$.hosts[*]', got '%s'", hostExtract.JSONPath)
	}

	if hostExtract.IDTemplate != "host:{{.ip}}" {
		t.Errorf("expected ID template 'host:{{.ip}}', got '%s'", hostExtract.IDTemplate)
	}

	if len(hostExtract.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(hostExtract.Properties))
	}

	// Check second extract (port)
	portExtract := toolSchema.Extracts[1]
	if portExtract.NodeType != "port" {
		t.Errorf("expected node type 'port', got '%s'", portExtract.NodeType)
	}

	if portExtract.JSONPath != "$.hosts[*].ports[*]" {
		t.Errorf("expected JSONPath '$.hosts[*].ports[*]', got '%s'", portExtract.JSONPath)
	}

	if len(portExtract.Relationships) != 1 {
		t.Errorf("expected 1 relationship, got %d", len(portExtract.Relationships))
	}
}

func TestSchemaBasedLoader_LoadAllTools(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create multiple mock tool binaries
	tools := []struct {
		name   string
		schema string
	}{
		{
			name: "tool1",
			schema: `{
				"name": "tool1",
				"description": "Tool 1",
				"output_schema": {
					"type": "object",
					"properties": {
						"results": {
							"type": "array",
							"items": {
								"type": "object",
								"taxonomy": {
									"node_type": "asset",
									"id_template": "asset:{{.id}}"
								}
							}
						}
					}
				}
			}`,
		},
		{
			name: "tool2",
			schema: `{
				"name": "tool2",
				"description": "Tool 2",
				"output_schema": {
					"type": "object",
					"properties": {
						"findings": {
							"type": "array",
							"items": {
								"type": "object",
								"taxonomy": {
									"node_type": "vulnerability",
									"id_template": "vuln:{{.id}}"
								}
							}
						}
					}
				}
			}`,
		},
	}

	for _, tool := range tools {
		toolPath := filepath.Join(tmpDir, tool.name)
		script := "#!/bin/bash\nif [ \"$1\" = \"--schema\" ]; then\ncat <<'EOF'\n" + tool.schema + "\nEOF\nfi\n"
		if err := os.WriteFile(toolPath, []byte(script), 0755); err != nil {
			t.Fatalf("failed to create %s: %v", tool.name, err)
		}
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Load all tools
	ctx := context.Background()
	schemas, err := loader.LoadAllTools(ctx)
	if err != nil {
		t.Fatalf("LoadAllTools failed: %v", err)
	}

	// Verify we got both tools
	if len(schemas) != 2 {
		t.Errorf("expected 2 tool schemas, got %d", len(schemas))
	}

	// Verify tool1
	if schema1, ok := schemas["tool1"]; !ok {
		t.Error("expected tool1 in schemas")
	} else if schema1.Tool != "tool1" {
		t.Errorf("expected tool1.Tool to be 'tool1', got '%s'", schema1.Tool)
	}

	// Verify tool2
	if schema2, ok := schemas["tool2"]; !ok {
		t.Error("expected tool2 in schemas")
	} else if schema2.Tool != "tool2" {
		t.Errorf("expected tool2.Tool to be 'tool2', got '%s'", schema2.Tool)
	}
}

func TestExtractTaxonomyMappings(t *testing.T) {
	tests := []struct {
		name          string
		schema        schema.JSON
		jsonPath      string
		expectedCount int
		expectedTypes []string
		expectedPaths []string
	}{
		{
			name: "simple object with taxonomy",
			schema: schema.JSON{
				Type: "object",
				Taxonomy: &schema.TaxonomyMapping{
					NodeType:   "test_node",
					IDTemplate: "test:{{.id}}",
				},
			},
			jsonPath:      "$",
			expectedCount: 1,
			expectedTypes: []string{"test_node"},
			expectedPaths: []string{"$"},
		},
		{
			name: "nested object properties",
			schema: schema.JSON{
				Type: "object",
				Properties: map[string]schema.JSON{
					"data": {
						Type: "object",
						Taxonomy: &schema.TaxonomyMapping{
							NodeType:   "data_node",
							IDTemplate: "data:{{.id}}",
						},
					},
					"metadata": {
						Type: "object",
						Taxonomy: &schema.TaxonomyMapping{
							NodeType:   "meta_node",
							IDTemplate: "meta:{{.id}}",
						},
					},
				},
			},
			jsonPath:      "$",
			expectedCount: 2,
			expectedTypes: []string{"data_node", "meta_node"},
			expectedPaths: []string{"$.data", "$.metadata"},
		},
		{
			name: "array with taxonomy on items",
			schema: schema.JSON{
				Type: "object",
				Properties: map[string]schema.JSON{
					"items": {
						Type: "array",
						Items: &schema.JSON{
							Type: "object",
							Taxonomy: &schema.TaxonomyMapping{
								NodeType:   "item_node",
								IDTemplate: "item:{{.id}}",
							},
						},
					},
				},
			},
			jsonPath:      "$",
			expectedCount: 1,
			expectedTypes: []string{"item_node"},
			expectedPaths: []string{"$.items[*]"},
		},
		{
			name: "deeply nested with multiple taxonomies",
			schema: schema.JSON{
				Type: "object",
				Properties: map[string]schema.JSON{
					"hosts": {
						Type: "array",
						Items: &schema.JSON{
							Type: "object",
							Taxonomy: &schema.TaxonomyMapping{
								NodeType:   "host",
								IDTemplate: "host:{{.ip}}",
							},
							Properties: map[string]schema.JSON{
								"ports": {
									Type: "array",
									Items: &schema.JSON{
										Type: "object",
										Taxonomy: &schema.TaxonomyMapping{
											NodeType:   "port",
											IDTemplate: "port:{{.port}}",
										},
									},
								},
							},
						},
					},
				},
			},
			jsonPath:      "$",
			expectedCount: 2,
			expectedTypes: []string{"host", "port"},
			expectedPaths: []string{"$.hosts[*]", "$.hosts[*].ports[*]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extracts := extractTaxonomyMappings(tt.schema, tt.jsonPath)

			if len(extracts) != tt.expectedCount {
				t.Errorf("expected %d extracts, got %d", tt.expectedCount, len(extracts))
			}

			// Build maps for checking presence (order is non-deterministic for map iteration)
			foundTypes := make(map[string]bool)
			foundPaths := make(map[string]bool)
			for _, extract := range extracts {
				foundTypes[extract.NodeType] = true
				foundPaths[extract.JSONPath] = true
			}

			// Verify all expected node types are present
			for _, expectedType := range tt.expectedTypes {
				if !foundTypes[expectedType] {
					t.Errorf("expected node type '%s' not found in extracts", expectedType)
				}
			}

			// Verify all expected JSON paths are present
			for _, expectedPath := range tt.expectedPaths {
				if !foundPaths[expectedPath] {
					t.Errorf("expected JSONPath '%s' not found in extracts", expectedPath)
				}
			}
		})
	}
}

func TestConvertTaxonomyToExtraction(t *testing.T) {
	taxonomy := &schema.TaxonomyMapping{
		NodeType:   "test_node",
		IDTemplate: "test:{{.id}}",
		Properties: []schema.PropertyMapping{
			{
				Source: "name",
				Target: "display_name",
			},
			{
				Source:  "value",
				Target:  "count",
				Default: 0,
			},
		},
		Relationships: []schema.RelationshipMapping{
			{
				Type:         "RELATED_TO",
				FromTemplate: "test:{{.id}}",
				ToTemplate:   "other:{{.other_id}}",
				Properties: []schema.PropertyMapping{
					{
						Source: "weight",
						Target: "strength",
					},
				},
			},
		},
	}

	jsonPath := "$.data.items[*]"
	extract := convertTaxonomyToExtraction(taxonomy, jsonPath)

	// Verify basic fields
	if extract.NodeType != "test_node" {
		t.Errorf("expected node type 'test_node', got '%s'", extract.NodeType)
	}

	if extract.IDTemplate != "test:{{.id}}" {
		t.Errorf("expected ID template 'test:{{.id}}', got '%s'", extract.IDTemplate)
	}

	if extract.JSONPath != jsonPath {
		t.Errorf("expected JSONPath '%s', got '%s'", jsonPath, extract.JSONPath)
	}

	// Verify properties
	if len(extract.Properties) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(extract.Properties))
	}

	prop1 := extract.Properties[0]
	if prop1.Target != "display_name" {
		t.Errorf("expected target 'display_name', got '%s'", prop1.Target)
	}
	if prop1.JSONPath != "$.data.items[*].name" {
		t.Errorf("expected JSONPath '$.data.items[*].name', got '%s'", prop1.JSONPath)
	}

	prop2 := extract.Properties[1]
	if prop2.Default != 0 {
		t.Errorf("expected default 0, got %v", prop2.Default)
	}

	// Verify relationships
	if len(extract.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(extract.Relationships))
	}

	rel := extract.Relationships[0]
	if rel.Type != "RELATED_TO" {
		t.Errorf("expected relationship type 'RELATED_TO', got '%s'", rel.Type)
	}

	if rel.FromTemplate != "test:{{.id}}" {
		t.Errorf("expected from template 'test:{{.id}}', got '%s'", rel.FromTemplate)
	}

	if len(rel.Properties) != 1 {
		t.Fatalf("expected 1 relationship property, got %d", len(rel.Properties))
	}
}

func TestSchemaBasedLoader_NoTaxonomy(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool with no taxonomy
	mockToolPath := filepath.Join(tmpDir, "no-taxonomy-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
{
  "name": "no-taxonomy-tool",
  "description": "A tool with no taxonomy",
  "output_schema": {
    "type": "object",
    "properties": {
      "result": {
        "type": "string"
      }
    }
  }
}
EOF
fi
`

	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Load from binary
	ctx := context.Background()
	toolSchema, err := loader.LoadTool(ctx, "no-taxonomy-tool")
	if err != nil {
		t.Fatalf("LoadTool failed: %v", err)
	}

	// Should return nil when no taxonomy found
	if toolSchema != nil {
		t.Error("expected nil tool schema when no taxonomy mappings found")
	}
}

func TestSchemaBasedLoader_InvalidBinary(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create loader pointing to the temp directory
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Try to load a tool that doesn't exist in the tools directory
	ctx := context.Background()
	_, err := loader.LoadTool(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for invalid binary path")
	}
}

func TestToolSchemaOutput_Unmarshal(t *testing.T) {
	jsonData := `{
		"name": "test-tool",
		"description": "Test tool description",
		"output_schema": {
			"type": "object",
			"properties": {
				"data": {
					"type": "string"
				}
			}
		}
	}`

	var output ToolSchemaOutput
	err := json.Unmarshal([]byte(jsonData), &output)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if output.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got '%s'", output.Name)
	}

	if output.Description != "Test tool description" {
		t.Errorf("expected description 'Test tool description', got '%s'", output.Description)
	}

	if output.Output.Type != "object" {
		t.Errorf("expected output type 'object', got '%s'", output.Output.Type)
	}
}

func TestSchemaBasedLoader_InvalidJSON(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool that outputs invalid JSON
	mockToolPath := filepath.Join(tmpDir, "invalid-json-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  echo "not valid json {"
fi
`

	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Try to load from binary with invalid JSON
	ctx := context.Background()
	_, err := loader.LoadTool(ctx, "mock-tool")
	if err == nil {
		t.Error("expected error for invalid JSON output")
	}
}

func TestSchemaBasedLoader_ToolExitError(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool that exits with error
	mockToolPath := filepath.Join(tmpDir, "error-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  echo "Error: schema not supported" >&2
  exit 1
fi
`

	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Try to load from binary that exits with error
	ctx := context.Background()
	_, err := loader.LoadTool(ctx, "error-tool")
	if err == nil {
		t.Error("expected error when tool exits with error")
	}
}

func TestSchemaBasedLoader_EmptyToolName(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool that outputs schema without name (should use filename)
	mockToolPath := filepath.Join(tmpDir, "nameless-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
{
  "description": "A tool without a name field",
  "output_schema": {
    "type": "object",
    "properties": {
      "data": {
        "type": "string",
        "taxonomy": {
          "node_type": "data_node",
          "id_template": "data:{{.id}}"
        }
      }
    }
  }
}
EOF
fi
`

	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Load from binary
	ctx := context.Background()
	toolSchema, err := loader.LoadTool(ctx, "nameless-tool")
	if err != nil {
		t.Fatalf("LoadTool failed: %v", err)
	}

	// Tool name should be inferred from filename
	if toolSchema.Tool != "nameless-tool" {
		t.Errorf("expected tool name 'nameless-tool', got '%s'", toolSchema.Tool)
	}
}

func TestSchemaBasedLoader_PropertyTransform(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool with transform property
	mockToolPath := filepath.Join(tmpDir, "transform-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
{
  "name": "transform-tool",
  "description": "A tool with property transforms",
  "output_schema": {
    "type": "object",
    "properties": {
      "items": {
        "type": "array",
        "items": {
          "type": "object",
          "taxonomy": {
            "node_type": "item",
            "id_template": "item:{{.id}}",
            "properties": [
              {
                "source": "name",
                "target": "normalized_name",
                "transform": "lowercase"
              }
            ]
          }
        }
      }
    }
  }
}
EOF
fi
`

	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Load from binary
	ctx := context.Background()
	toolSchema, err := loader.LoadTool(ctx, "transform-tool")
	if err != nil {
		t.Fatalf("LoadTool failed: %v", err)
	}

	if toolSchema == nil {
		t.Fatal("expected non-nil tool schema")
	}

	if len(toolSchema.Extracts) != 1 {
		t.Fatalf("expected 1 extract, got %d", len(toolSchema.Extracts))
	}

	extract := toolSchema.Extracts[0]
	if len(extract.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(extract.Properties))
	}

	prop := extract.Properties[0]
	// Transform should be encoded in Template field
	expectedTemplate := "{{.name | lowercase}}"
	if prop.Template != expectedTemplate {
		t.Errorf("expected template '%s', got '%s'", expectedTemplate, prop.Template)
	}
}

func TestSchemaBasedLoader_SkipsNonExecutables(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a non-executable file
	nonExecPath := filepath.Join(tmpDir, "not-executable")
	if err := os.WriteFile(nonExecPath, []byte("not a script"), 0644); err != nil {
		t.Fatalf("failed to create non-executable file: %v", err)
	}

	// Create an executable tool
	execToolPath := filepath.Join(tmpDir, "executable-tool")
	execToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
{
  "name": "executable-tool",
  "description": "An executable tool",
  "output_schema": {
    "type": "object",
    "properties": {
      "data": {
        "type": "string",
        "taxonomy": {
          "node_type": "data",
          "id_template": "data:{{.id}}"
        }
      }
    }
  }
}
EOF
fi
`
	if err := os.WriteFile(execToolPath, []byte(execToolScript), 0755); err != nil {
		t.Fatalf("failed to create executable tool: %v", err)
	}

	// Create loader
	loader := NewSchemaBasedLoader(tmpDir, slog.Default())

	// Load all tools
	ctx := context.Background()
	schemas, err := loader.LoadAllTools(ctx)
	if err != nil {
		t.Fatalf("LoadAllTools failed: %v", err)
	}

	// Should only have the executable tool
	if len(schemas) != 1 {
		t.Errorf("expected 1 tool schema (only executable), got %d", len(schemas))
	}

	if _, ok := schemas["executable-tool"]; !ok {
		t.Error("expected 'executable-tool' in schemas")
	}
}

func TestSchemaBasedLoader_Integration_WithRegistry(t *testing.T) {
	// Create a temporary directory for test binaries
	tmpDir := t.TempDir()

	// Create a mock tool with taxonomy
	mockToolPath := filepath.Join(tmpDir, "schema-tool")
	mockToolScript := `#!/bin/bash
if [ "$1" = "--schema" ]; then
  cat <<'EOF'
{
  "name": "schema-tool",
  "description": "A tool with embedded taxonomy",
  "output_schema": {
    "type": "object",
    "properties": {
      "hosts": {
        "type": "array",
        "items": {
          "type": "object",
          "taxonomy": {
            "node_type": "host",
            "id_template": "host:{{.ip}}",
            "properties": [
              {
                "source": "ip",
                "target": "ip_address"
              }
            ]
          }
        }
      }
    }
  }
}
EOF
fi
`
	if err := os.WriteFile(mockToolPath, []byte(mockToolScript), 0755); err != nil {
		t.Fatalf("failed to create mock tool: %v", err)
	}

	// Create taxonomy with a YAML-based tool schema
	taxonomy := NewTaxonomy("0.1.0")
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "yaml-tool",
		Description: "Tool loaded from YAML",
	})
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "schema-tool", // Same name as the binary - will be replaced
		Description: "YAML version that should be replaced",
	})

	// Create registry
	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry failed: %v", err)
	}

	// Load tool schemas from binaries
	ctx := context.Background()
	err = registry.LoadToolSchemasFromBinaries(ctx, tmpDir, slog.Default())
	if err != nil {
		t.Fatalf("LoadToolSchemasFromBinaries failed: %v", err)
	}

	// Verify yaml-tool is still there (no binary for it)
	yamlSchema := registry.GetToolOutputSchema("yaml-tool")
	if yamlSchema == nil {
		t.Error("expected yaml-tool schema to still exist")
	}
	if registry.IsSchemaBasedToolSchema("yaml-tool") {
		t.Error("yaml-tool should NOT be schema-based")
	}

	// Verify schema-tool was replaced with binary version
	schemaToolSchema := registry.GetToolOutputSchema("schema-tool")
	if schemaToolSchema == nil {
		t.Error("expected schema-tool schema to exist")
	}
	if !registry.IsSchemaBasedToolSchema("schema-tool") {
		t.Error("schema-tool SHOULD be schema-based")
	}
	// Should have the embedded taxonomy extracts, not the empty YAML version
	if len(schemaToolSchema.Extracts) != 1 {
		t.Errorf("expected 1 extract from schema-based version, got %d", len(schemaToolSchema.Extracts))
	}
}
