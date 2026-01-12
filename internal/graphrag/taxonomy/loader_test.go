package taxonomy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaxonomyLoader_Load(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "successful load from embedded",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewTaxonomyLoader()
			taxonomy, err := loader.Load()

			if (err != nil) != tt.wantErr {
				t.Errorf("TaxonomyLoader.Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Validate loaded taxonomy
				if taxonomy == nil {
					t.Error("TaxonomyLoader.Load() returned nil taxonomy")
					return
				}

				if taxonomy.Version == "" {
					t.Error("TaxonomyLoader.Load() returned taxonomy with empty version")
				}

				if len(taxonomy.NodeTypes) == 0 {
					t.Error("TaxonomyLoader.Load() returned taxonomy with no node types")
				}

				if len(taxonomy.Relationships) == 0 {
					t.Error("TaxonomyLoader.Load() returned taxonomy with no relationships")
				}

				// Check for specific expected node types from assets.yaml
				expectedNodeTypes := []string{"domain", "subdomain", "host", "port", "service"}
				for _, nodeType := range expectedNodeTypes {
					if _, exists := taxonomy.NodeTypes[nodeType]; !exists {
						t.Errorf("TaxonomyLoader.Load() missing expected node type: %s", nodeType)
					}
				}

				// Check for specific expected relationships
				expectedRelTypes := []string{"HAS_SUBDOMAIN", "RESOLVES_TO", "HAS_PORT", "RUNS_SERVICE"}
				for _, relType := range expectedRelTypes {
					if _, exists := taxonomy.Relationships[relType]; !exists {
						t.Errorf("TaxonomyLoader.Load() missing expected relationship type: %s", relType)
					}
				}

				// Check techniques were loaded
				if len(taxonomy.Techniques) == 0 {
					t.Error("TaxonomyLoader.Load() returned taxonomy with no techniques")
				}

				// Check target types were loaded
				if len(taxonomy.TargetTypes) == 0 {
					t.Error("TaxonomyLoader.Load() returned taxonomy with no target types")
				}

				// Check technique types were loaded
				if len(taxonomy.TechniqueTypes) == 0 {
					t.Error("TaxonomyLoader.Load() returned taxonomy with no technique types")
				}

				// Check capabilities were loaded
				if len(taxonomy.Capabilities) == 0 {
					t.Error("TaxonomyLoader.Load() returned taxonomy with no capabilities")
				}
			}
		})
	}
}

func TestTaxonomyLoader_LoadWithCustom(t *testing.T) {
	tests := []struct {
		name          string
		setupCustom   func(t *testing.T) string
		wantErr       bool
		validateExtra func(t *testing.T, taxonomy *Taxonomy)
	}{
		{
			name: "load with valid custom taxonomy",
			setupCustom: func(t *testing.T) string {
				// Create temporary directory for custom taxonomy
				tmpDir := t.TempDir()

				// Create custom node types file
				customNodesYAML := `node_types:
  - id: custom_node_1
    name: Custom Node Type
    type: custom_asset
    category: asset
    description: A custom node type for testing
    id_template: "custom:{name}"
    properties:
      - name: name
        type: string
        required: true
        description: Name of custom asset
`
				nodesPath := filepath.Join(tmpDir, "custom_nodes.yaml")
				if err := os.WriteFile(nodesPath, []byte(customNodesYAML), 0644); err != nil {
					t.Fatalf("Failed to create custom nodes file: %v", err)
				}

				// Create custom root taxonomy.yaml
				customRootYAML := `version: "0.1.0-custom"
includes:
  - custom_nodes.yaml
`
				rootPath := filepath.Join(tmpDir, "taxonomy.yaml")
				if err := os.WriteFile(rootPath, []byte(customRootYAML), 0644); err != nil {
					t.Fatalf("Failed to create custom root file: %v", err)
				}

				return rootPath
			},
			wantErr: false,
			validateExtra: func(t *testing.T, taxonomy *Taxonomy) {
				// Verify custom node type was added
				customNode, exists := taxonomy.NodeTypes["custom_asset"]
				if !exists {
					t.Error("Custom node type 'custom_asset' not found in merged taxonomy")
					return
				}

				if customNode.Name != "Custom Node Type" {
					t.Errorf("Custom node type name = %v, want 'Custom Node Type'", customNode.Name)
				}

				// Verify bundled node types still exist
				if _, exists := taxonomy.NodeTypes["domain"]; !exists {
					t.Error("Bundled node type 'domain' missing after custom merge")
				}

				// Verify taxonomy is marked as custom loaded
				if !taxonomy.IsCustomLoaded() {
					t.Error("Taxonomy should be marked as custom loaded")
				}
			},
		},
		{
			name: "load with duplicate node type should fail",
			setupCustom: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Try to override bundled 'domain' type
				customNodesYAML := `node_types:
  - id: node_domain
    name: Overridden Domain
    type: domain
    category: asset
    description: Attempting to override bundled type
    id_template: "domain:{name}"
    properties:
      - name: name
        type: string
        required: true
        description: Domain name
`
				nodesPath := filepath.Join(tmpDir, "custom_nodes.yaml")
				if err := os.WriteFile(nodesPath, []byte(customNodesYAML), 0644); err != nil {
					t.Fatalf("Failed to create custom nodes file: %v", err)
				}

				customRootYAML := `version: "0.1.0-custom"
includes:
  - custom_nodes.yaml
`
				rootPath := filepath.Join(tmpDir, "taxonomy.yaml")
				if err := os.WriteFile(rootPath, []byte(customRootYAML), 0644); err != nil {
					t.Fatalf("Failed to create custom root file: %v", err)
				}

				return rootPath
			},
			wantErr: true,
		},
		{
			name: "load with invalid YAML should fail",
			setupCustom: func(t *testing.T) string {
				tmpDir := t.TempDir()

				// Create invalid YAML
				invalidYAML := `node_types:
  - id: bad_node
    name: [this is not valid
    type: "broken
`
				nodesPath := filepath.Join(tmpDir, "bad_nodes.yaml")
				if err := os.WriteFile(nodesPath, []byte(invalidYAML), 0644); err != nil {
					t.Fatalf("Failed to create invalid nodes file: %v", err)
				}

				customRootYAML := `version: "0.1.0-custom"
includes:
  - bad_nodes.yaml
`
				rootPath := filepath.Join(tmpDir, "taxonomy.yaml")
				if err := os.WriteFile(rootPath, []byte(customRootYAML), 0644); err != nil {
					t.Fatalf("Failed to create custom root file: %v", err)
				}

				return rootPath
			},
			wantErr: true,
		},
		{
			name: "load with missing file should fail",
			setupCustom: func(t *testing.T) string {
				return "/nonexistent/path/taxonomy.yaml"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customPath := tt.setupCustom(t)

			loader := NewTaxonomyLoader()
			taxonomy, err := loader.LoadWithCustom(customPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("TaxonomyLoader.LoadWithCustom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if taxonomy == nil {
					t.Error("TaxonomyLoader.LoadWithCustom() returned nil taxonomy")
					return
				}

				// Run extra validation if provided
				if tt.validateExtra != nil {
					tt.validateExtra(t, taxonomy)
				}
			}
		})
	}
}

func TestLoadTaxonomy(t *testing.T) {
	// Test convenience function
	taxonomy, err := LoadTaxonomy()
	if err != nil {
		t.Fatalf("LoadTaxonomy() error = %v", err)
	}

	if taxonomy == nil {
		t.Fatal("LoadTaxonomy() returned nil taxonomy")
	}

	if taxonomy.Version == "" {
		t.Error("LoadTaxonomy() returned taxonomy with empty version")
	}

	if len(taxonomy.NodeTypes) == 0 {
		t.Error("LoadTaxonomy() returned taxonomy with no node types")
	}
}

func TestTaxonomyLoader_parseNodeTypes(t *testing.T) {
	loader := &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid node types",
			yaml: `node_types:
  - id: test_node
    name: Test Node
    type: test
    category: test
    description: Test node type
    id_template: "test:{id}"
    properties:
      - name: id
        type: string
        required: true
        description: Test ID
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `node_types:
  - id: [broken
    name: "invalid
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := NewTaxonomy("test")
			err := loader.parseNodeTypes(taxonomy, []byte(tt.yaml), "test.yaml", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("parseNodeTypes() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(taxonomy.NodeTypes) == 0 {
					t.Error("parseNodeTypes() did not add node types to taxonomy")
				}
			}
		})
	}
}

func TestTaxonomyLoader_parseRelationshipTypes(t *testing.T) {
	loader := &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}

	// First add node types that relationships can reference
	taxonomy := NewTaxonomy("test")
	domainNode := &NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}
	subdomainNode := &NodeTypeDefinition{
		ID:         "node_subdomain",
		Name:       "Subdomain",
		Type:       "subdomain",
		Category:   "asset",
		IDTemplate: "subdomain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	}
	_ = taxonomy.AddNodeType(domainNode)
	_ = taxonomy.AddNodeType(subdomainNode)

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid relationship types",
			yaml: `relationship_types:
  - id: rel_test
    name: Test Relationship
    type: TEST_REL
    description: Test relationship
    from_types: [domain]
    to_types: [subdomain]
    bidirectional: false
    properties: []
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `relationship_types:
  - id: [broken
    type: "invalid
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.parseRelationshipTypes(taxonomy, []byte(tt.yaml), "test.yaml", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("parseRelationshipTypes() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(taxonomy.Relationships) == 0 {
					t.Error("parseRelationshipTypes() did not add relationships to taxonomy")
				}
			}
		})
	}
}

func TestTaxonomyLoader_parseTechniques(t *testing.T) {
	loader := &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid techniques",
			yaml: `techniques:
  - technique_id: T1234
    name: Test Technique
    taxonomy: mitre
    description: Test technique
    tactics: [initial-access]
    platforms: [linux]
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `techniques:
  - technique_id: [broken
    name: "invalid
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := NewTaxonomy("test")
			err := loader.parseTechniques(taxonomy, []byte(tt.yaml), "test.yaml", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("parseTechniques() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(taxonomy.Techniques) == 0 {
					t.Error("parseTechniques() did not add techniques to taxonomy")
				}
			}
		})
	}
}

// TestTaxonomyLoader_parseTargetTypes tests parsing target types from YAML.
func TestTaxonomyLoader_parseTargetTypes(t *testing.T) {
	loader := &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid target types",
			yaml: `target_types:
  - id: target.web.http_api
    type: http_api
    name: HTTP API
    category: web
    description: HTTP-based REST API
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `target_types:
  - id: [broken
    type: "invalid
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := NewTaxonomy("test")
			err := loader.parseTargetTypes(taxonomy, []byte(tt.yaml), "test.yaml", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("parseTargetTypes() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(taxonomy.TargetTypes) == 0 {
					t.Error("parseTargetTypes() did not add target types to taxonomy")
				}
			}
		})
	}
}

// TestTaxonomyLoader_parseTechniqueTypes tests parsing technique types from YAML.
func TestTaxonomyLoader_parseTechniqueTypes(t *testing.T) {
	loader := &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid technique types",
			yaml: `technique_types:
  - id: technique.initial_access.ssrf
    type: ssrf
    name: Server-Side Request Forgery
    category: initial_access
    mitre_ids: [T1190]
    description: SSRF vulnerability
    default_severity: high
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `technique_types:
  - id: [broken
    type: "invalid
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := NewTaxonomy("test")
			err := loader.parseTechniqueTypes(taxonomy, []byte(tt.yaml), "test.yaml", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("parseTechniqueTypes() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(taxonomy.TechniqueTypes) == 0 {
					t.Error("parseTechniqueTypes() did not add technique types to taxonomy")
				}
			}
		})
	}
}

// TestTaxonomyLoader_parseCapabilities tests parsing capabilities from YAML.
func TestTaxonomyLoader_parseCapabilities(t *testing.T) {
	loader := &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid capabilities",
			yaml: `capabilities:
  - id: capability.web_vulnerability_scanning
    name: Web Vulnerability Scanning
    description: Comprehensive web vulnerability scanning
    technique_types: [ssrf, sqli, xss]
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `capabilities:
  - id: [broken
    name: "invalid
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := NewTaxonomy("test")
			err := loader.parseCapabilities(taxonomy, []byte(tt.yaml), "test.yaml", "test")

			if (err != nil) != tt.wantErr {
				t.Errorf("parseCapabilities() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(taxonomy.Capabilities) == 0 {
					t.Error("parseCapabilities() did not add capabilities to taxonomy")
				}
			}
		})
	}
}
