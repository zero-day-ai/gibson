package taxonomy

import (
	"os"
	"testing"

	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
)

// TestIntegration_LoadValidateRegistry tests the full workflow:
// load taxonomy, validate it, create registry, and verify functionality
func TestIntegration_LoadValidateRegistry(t *testing.T) {
	// Step 1: Load taxonomy
	taxonomy, err := LoadTaxonomy()
	if err != nil {
		t.Fatalf("LoadTaxonomy() error = %v", err)
	}

	if taxonomy == nil {
		t.Fatal("LoadTaxonomy() returned nil taxonomy")
	}

	// Step 2: Validate taxonomy
	validator := NewTaxonomyValidator()
	if err := validator.Validate(taxonomy); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	// Step 3: Create registry
	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// Step 4: Verify registry functionality
	// Check node type lookups
	nodeTypes := registry.NodeTypes()
	if len(nodeTypes) == 0 {
		t.Error("Registry has no node types")
	}

	// Check specific node type
	domainNode, ok := registry.NodeType("domain")
	if !ok {
		t.Error("Registry missing 'domain' node type")
	} else if domainNode.Type != "domain" {
		t.Errorf("Node type mismatch: got %v, want domain", domainNode.Type)
	}

	// Check relationship lookups
	relTypes := registry.RelationshipTypes()
	if len(relTypes) == 0 {
		t.Error("Registry has no relationship types")
	}

	// Check specific relationship
	if !registry.IsCanonicalRelationType("HAS_SUBDOMAIN") {
		t.Error("Registry should have HAS_SUBDOMAIN relationship")
	}

	// Check technique lookups
	mitreTechniques := registry.Techniques("mitre")
	if len(mitreTechniques) == 0 {
		t.Error("Registry has no MITRE techniques")
	}

	arcanumTechniques := registry.Techniques("arcanum")
	if len(arcanumTechniques) == 0 {
		t.Error("Registry has no Arcanum techniques")
	}

	// Step 5: Test ID generation
	nodeID, err := registry.GenerateNodeID("domain", map[string]any{
		"name": "example.com",
	})
	if err != nil {
		t.Errorf("GenerateNodeID() error = %v", err)
	}
	if nodeID == "" {
		t.Error("GenerateNodeID() returned empty ID")
	}
}

// TestIntegration_SDKTaxonomy tests SDK integration with taxonomy
func TestIntegration_SDKTaxonomy(t *testing.T) {
	// Save original taxonomy
	originalTax := sdkgraphrag.Taxonomy()
	defer sdkgraphrag.SetTaxonomy(originalTax)

	// Load and validate taxonomy
	taxonomy, err := LoadTaxonomy()
	if err != nil {
		t.Fatalf("LoadTaxonomy() error = %v", err)
	}

	if err := ValidateTaxonomy(taxonomy); err != nil {
		t.Fatalf("ValidateTaxonomy() error = %v", err)
	}

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// Set taxonomy in SDK
	sdkgraphrag.SetTaxonomy(registry)

	// Verify SDK can access taxonomy
	tax := sdkgraphrag.Taxonomy()
	if tax == nil {
		t.Fatal("SDK Taxonomy() returned nil after SetTaxonomy")
	}

	// Verify version propagated
	if tax.Version() != registry.Version() {
		t.Errorf("SDK taxonomy version = %v, want %v", tax.Version(), registry.Version())
	}

	// Test SDK validation functions
	if !sdkgraphrag.ValidateAndWarnNodeType("domain") {
		t.Error("ValidateAndWarnNodeType('domain') returned false for canonical type")
	}

	if sdkgraphrag.ValidateAndWarnNodeType("nonexistent_type") {
		t.Error("ValidateAndWarnNodeType('nonexistent_type') returned true for non-canonical type")
	}

	if !sdkgraphrag.ValidateAndWarnRelationType("HAS_SUBDOMAIN") {
		t.Error("ValidateAndWarnRelationType('HAS_SUBDOMAIN') returned false for canonical type")
	}

	if sdkgraphrag.ValidateAndWarnRelationType("NONEXISTENT_REL") {
		t.Error("ValidateAndWarnRelationType('NONEXISTENT_REL') returned true for non-canonical type")
	}

	// Test node creation with validation
	node := sdkgraphrag.NewNodeWithValidation("domain")
	if node == nil {
		t.Fatal("NewNodeWithValidation('domain') returned nil")
	}
	if node.Type != "domain" {
		t.Errorf("NewNodeWithValidation() type = %v, want domain", node.Type)
	}

	// Test relationship creation with validation
	rel := sdkgraphrag.NewRelationshipWithValidation("from-id", "to-id", "HAS_SUBDOMAIN")
	if rel == nil {
		t.Fatal("NewRelationshipWithValidation() returned nil")
	}
	if rel.Type != "HAS_SUBDOMAIN" {
		t.Errorf("NewRelationshipWithValidation() type = %v, want HAS_SUBDOMAIN", rel.Type)
	}
}

// TestIntegration_ConvenienceFunctions tests the convenience functions
func TestIntegration_ConvenienceFunctions(t *testing.T) {
	// Test LoadAndValidateTaxonomy
	registry, err := LoadAndValidateTaxonomy()
	if err != nil {
		t.Fatalf("LoadAndValidateTaxonomy() error = %v", err)
	}

	if registry == nil {
		t.Fatal("LoadAndValidateTaxonomy() returned nil registry")
	}

	// Verify registry is functional
	if len(registry.NodeTypes()) == 0 {
		t.Error("LoadAndValidateTaxonomy() returned registry with no node types")
	}

	if len(registry.RelationshipTypes()) == 0 {
		t.Error("LoadAndValidateTaxonomy() returned registry with no relationships")
	}

	if len(registry.Techniques("")) == 0 {
		t.Error("LoadAndValidateTaxonomy() returned registry with no techniques")
	}
}

// TestIntegration_CustomTaxonomy tests loading with custom extensions
func TestIntegration_CustomTaxonomy(t *testing.T) {
	// Create temporary custom taxonomy
	tmpDir := t.TempDir()

	// Create custom node types
	customNodesYAML := `node_types:
  - id: node_custom_asset
    name: Custom Asset
    type: custom_asset
    category: asset
    description: A custom asset type
    id_template: "custom:{name}"
    properties:
      - name: name
        type: string
        required: true
        description: Asset name
`
	customNodesPath := tmpDir + "/custom_nodes.yaml"
	if err := writeFile(customNodesPath, customNodesYAML); err != nil {
		t.Fatalf("Failed to create custom nodes file: %v", err)
	}

	// Create custom root
	customRootYAML := `version: "0.1.0-custom"
includes:
  - custom_nodes.yaml
`
	customRootPath := tmpDir + "/taxonomy.yaml"
	if err := writeFile(customRootPath, customRootYAML); err != nil {
		t.Fatalf("Failed to create custom root file: %v", err)
	}

	// Load with custom taxonomy
	registry, err := LoadAndValidateTaxonomyWithCustom(customRootPath)
	if err != nil {
		t.Fatalf("LoadAndValidateTaxonomyWithCustom() error = %v", err)
	}

	// Verify custom node type exists
	customNode, ok := registry.NodeType("custom_asset")
	if !ok {
		t.Error("Registry missing custom 'custom_asset' node type")
	} else if customNode.Name != "Custom Asset" {
		t.Errorf("Custom node name = %v, want 'Custom Asset'", customNode.Name)
	}

	// Verify bundled types still exist
	if _, ok := registry.NodeType("domain"); !ok {
		t.Error("Registry missing bundled 'domain' type after custom merge")
	}

	// Verify version indicates custom (should be embedded version + custom suffix)
	version := registry.Version()
	if version == "" {
		t.Error("Registry version is empty")
	}
	// Version should have +custom suffix
	if len(version) < 7 || version[len(version)-7:] != "+custom" {
		t.Errorf("Registry version = %v, should end with +custom", version)
	}
}

// writeFile is a helper to write string content to a file
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
