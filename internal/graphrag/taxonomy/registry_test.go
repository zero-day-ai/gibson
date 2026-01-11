package taxonomy

import (
	"strings"
	"sync"
	"testing"
)

func TestNewTaxonomyRegistry(t *testing.T) {
	tests := []struct {
		name    string
		taxonomy *Taxonomy
		wantErr bool
	}{
		{
			name:     "valid taxonomy",
			taxonomy: NewTaxonomy("0.1.0"),
			wantErr:  false,
		},
		{
			name:     "nil taxonomy",
			taxonomy: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := NewTaxonomyRegistry(tt.taxonomy)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewTaxonomyRegistry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && registry == nil {
				t.Error("NewTaxonomyRegistry() returned nil registry")
			}
		})
	}
}

func TestTaxonomyRegistry_Version(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		customLoaded bool
		want         string
	}{
		{
			name:         "standard version",
			version:      "0.1.0",
			customLoaded: false,
			want:         "0.1.0",
		},
		{
			name:         "version with custom loaded",
			version:      "0.1.0",
			customLoaded: true,
			want:         "0.1.0+custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := NewTaxonomy(tt.version)
			if tt.customLoaded {
				taxonomy.MarkCustomLoaded()
			}

			registry, err := NewTaxonomyRegistry(taxonomy)
			if err != nil {
				t.Fatalf("NewTaxonomyRegistry() error = %v", err)
			}

			if got := registry.Version(); got != tt.want {
				t.Errorf("TaxonomyRegistry.Version() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaxonomyRegistry_NodeType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test node types
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
	_ = taxonomy.AddNodeType(domainNode)

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name     string
		typeName string
		wantOk   bool
	}{
		{
			name:     "existing node type",
			typeName: "domain",
			wantOk:   true,
		},
		{
			name:     "non-existent node type",
			typeName: "nonexistent",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeDef, ok := registry.NodeType(tt.typeName)

			if ok != tt.wantOk {
				t.Errorf("TaxonomyRegistry.NodeType() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk {
				if nodeDef == nil {
					t.Error("TaxonomyRegistry.NodeType() returned nil for existing type")
				} else if nodeDef.Type != tt.typeName {
					t.Errorf("TaxonomyRegistry.NodeType() type = %v, want %v", nodeDef.Type, tt.typeName)
				}
			}
		})
	}
}

func TestTaxonomyRegistry_NodeTypes(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test node types
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_host",
		Name:       "Host",
		Type:       "host",
		Category:   "asset",
		IDTemplate: "host:{ip}",
		Properties: []PropertyDefinition{
			{Name: "ip", Type: "string", Required: true},
		},
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	nodeTypes := registry.NodeTypes()

	if len(nodeTypes) != 2 {
		t.Errorf("TaxonomyRegistry.NodeTypes() returned %d types, want 2", len(nodeTypes))
	}

	// Check that returned types match what we added
	typeMap := make(map[string]bool)
	for _, nt := range nodeTypes {
		typeMap[nt.Type] = true
	}

	if !typeMap["domain"] {
		t.Error("TaxonomyRegistry.NodeTypes() missing 'domain' type")
	}
	if !typeMap["host"] {
		t.Error("TaxonomyRegistry.NodeTypes() missing 'host' type")
	}
}

func TestTaxonomyRegistry_RelationshipType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add node types first
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_subdomain",
		Name:       "Subdomain",
		Type:       "subdomain",
		Category:   "asset",
		IDTemplate: "subdomain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})

	// Add relationship
	_ = taxonomy.AddRelationship(&RelationshipTypeDefinition{
		ID:            "rel_has_subdomain",
		Name:          "Has Subdomain",
		Type:          "HAS_SUBDOMAIN",
		FromTypes:     []string{"domain"},
		ToTypes:       []string{"subdomain"},
		Bidirectional: false,
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name     string
		typeName string
		wantOk   bool
	}{
		{
			name:     "existing relationship type",
			typeName: "HAS_SUBDOMAIN",
			wantOk:   true,
		},
		{
			name:     "non-existent relationship type",
			typeName: "NONEXISTENT",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relDef, ok := registry.RelationshipType(tt.typeName)

			if ok != tt.wantOk {
				t.Errorf("TaxonomyRegistry.RelationshipType() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk {
				if relDef == nil {
					t.Error("TaxonomyRegistry.RelationshipType() returned nil for existing type")
				} else if relDef.Type != tt.typeName {
					t.Errorf("TaxonomyRegistry.RelationshipType() type = %v, want %v", relDef.Type, tt.typeName)
				}
			}
		})
	}
}

func TestTaxonomyRegistry_Technique(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test techniques
	_ = taxonomy.AddTechnique(&TechniqueDefinition{
		TechniqueID: "T1190",
		Name:        "Exploit Public-Facing Application",
		Taxonomy:    "mitre",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name        string
		techniqueID string
		wantOk      bool
	}{
		{
			name:        "existing technique",
			techniqueID: "T1190",
			wantOk:      true,
		},
		{
			name:        "non-existent technique",
			techniqueID: "T9999",
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			techDef, ok := registry.Technique(tt.techniqueID)

			if ok != tt.wantOk {
				t.Errorf("TaxonomyRegistry.Technique() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk {
				if techDef == nil {
					t.Error("TaxonomyRegistry.Technique() returned nil for existing technique")
				} else if techDef.TechniqueID != tt.techniqueID {
					t.Errorf("TaxonomyRegistry.Technique() ID = %v, want %v", techDef.TechniqueID, tt.techniqueID)
				}
			}
		})
	}
}

func TestTaxonomyRegistry_Techniques(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add techniques from different taxonomies
	_ = taxonomy.AddTechnique(&TechniqueDefinition{
		TechniqueID: "T1190",
		Name:        "Exploit Public-Facing Application",
		Taxonomy:    "mitre",
	})
	_ = taxonomy.AddTechnique(&TechniqueDefinition{
		TechniqueID: "T1566",
		Name:        "Phishing",
		Taxonomy:    "mitre",
	})
	_ = taxonomy.AddTechnique(&TechniqueDefinition{
		TechniqueID: "ARC-T001",
		Name:        "Direct Prompt Injection",
		Taxonomy:    "arcanum",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name       string
		source     string
		wantCount  int
	}{
		{
			name:       "all techniques",
			source:     "",
			wantCount:  3,
		},
		{
			name:       "MITRE techniques only",
			source:     "mitre",
			wantCount:  2,
		},
		{
			name:       "Arcanum techniques only",
			source:     "arcanum",
			wantCount:  1,
		},
		{
			name:       "non-existent source",
			source:     "nonexistent",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			techniques := registry.Techniques(tt.source)

			if len(techniques) != tt.wantCount {
				t.Errorf("TaxonomyRegistry.Techniques(%q) returned %d techniques, want %d", tt.source, len(techniques), tt.wantCount)
			}
		})
	}
}

func TestTaxonomyRegistry_IsCanonicalNodeType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name     string
		typeName string
		want     bool
	}{
		{
			name:     "canonical type",
			typeName: "domain",
			want:     true,
		},
		{
			name:     "non-canonical type",
			typeName: "custom_type",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.IsCanonicalNodeType(tt.typeName); got != tt.want {
				t.Errorf("TaxonomyRegistry.IsCanonicalNodeType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaxonomyRegistry_IsCanonicalRelationType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add node types
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_subdomain",
		Name:       "Subdomain",
		Type:       "subdomain",
		Category:   "asset",
		IDTemplate: "subdomain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})

	// Add relationship
	_ = taxonomy.AddRelationship(&RelationshipTypeDefinition{
		ID:            "rel_has_subdomain",
		Name:          "Has Subdomain",
		Type:          "HAS_SUBDOMAIN",
		FromTypes:     []string{"domain"},
		ToTypes:       []string{"subdomain"},
		Bidirectional: false,
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name     string
		typeName string
		want     bool
	}{
		{
			name:     "canonical type",
			typeName: "HAS_SUBDOMAIN",
			want:     true,
		},
		{
			name:     "non-canonical type",
			typeName: "CUSTOM_REL",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.IsCanonicalRelationType(tt.typeName); got != tt.want {
				t.Errorf("TaxonomyRegistry.IsCanonicalRelationType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaxonomyRegistry_GenerateNodeID(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add node types with different ID templates
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_host",
		Name:       "Host",
		Type:       "host",
		Category:   "asset",
		IDTemplate: "host:{ip}:{port}",
		Properties: []PropertyDefinition{
			{Name: "ip", Type: "string", Required: true},
			{Name: "port", Type: "int", Required: true},
		},
	})
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_finding",
		Name:       "Finding",
		Type:       "finding",
		Category:   "finding",
		IDTemplate: "finding:{sha256(title)}",
		Properties: []PropertyDefinition{
			{Name: "title", Type: "string", Required: true},
		},
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name        string
		typeName    string
		properties  map[string]any
		wantErr     bool
		wantContain string // Substring that should be in the ID
	}{
		{
			name:     "simple property substitution",
			typeName: "domain",
			properties: map[string]any{
				"name": "example.com",
			},
			wantErr:     false,
			wantContain: "example.com",
		},
		{
			name:     "multiple properties",
			typeName: "host",
			properties: map[string]any{
				"ip":   "192.168.1.1",
				"port": 443,
			},
			wantErr:     false,
			wantContain: "192.168.1.1",
		},
		{
			name:     "sha256 function",
			typeName: "finding",
			properties: map[string]any{
				"title": "SQL Injection",
			},
			wantErr:     false,
			wantContain: "finding:",
		},
		{
			name:       "missing required property",
			typeName:   "domain",
			properties: map[string]any{},
			wantErr:    true,
		},
		{
			name:     "unknown node type",
			typeName: "nonexistent",
			properties: map[string]any{
				"name": "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := registry.GenerateNodeID(tt.typeName, tt.properties)

			if (err != nil) != tt.wantErr {
				t.Errorf("TaxonomyRegistry.GenerateNodeID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if id == "" {
					t.Error("TaxonomyRegistry.GenerateNodeID() returned empty ID")
				}
				if tt.wantContain != "" && !strings.Contains(id, tt.wantContain) {
					t.Errorf("TaxonomyRegistry.GenerateNodeID() = %v, should contain %v", id, tt.wantContain)
				}
			}
		})
	}
}

func TestTaxonomyRegistry_ConcurrentAccess(t *testing.T) {
	// Test thread-safety of registry
	taxonomy := NewTaxonomy("0.1.0")
	_ = taxonomy.AddNodeType(&NodeTypeDefinition{
		ID:         "node_domain",
		Name:       "Domain",
		Type:       "domain",
		Category:   "asset",
		IDTemplate: "domain:{name}",
		Properties: []PropertyDefinition{
			{Name: "name", Type: "string", Required: true},
		},
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// Run concurrent reads
	var wg sync.WaitGroup
	concurrency := 100

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Test various read operations
			_ = registry.Version()
			_ = registry.NodeTypes()
			_, _ = registry.NodeType("domain")
			_ = registry.IsCanonicalNodeType("domain")
			_, _ = registry.GenerateNodeID("domain", map[string]any{"name": "test.com"})
		}()
	}

	wg.Wait()
	// If we get here without panicking, concurrency is safe
}

func TestLoadAndValidateTaxonomy(t *testing.T) {
	// Test convenience function
	registry, err := LoadAndValidateTaxonomy()
	if err != nil {
		t.Fatalf("LoadAndValidateTaxonomy() error = %v", err)
	}

	if registry == nil {
		t.Fatal("LoadAndValidateTaxonomy() returned nil registry")
	}

	// Verify registry is functional
	if registry.Version() == "" {
		t.Error("LoadAndValidateTaxonomy() returned registry with empty version")
	}

	if len(registry.NodeTypes()) == 0 {
		t.Error("LoadAndValidateTaxonomy() returned registry with no node types")
	}
}

// Benchmark tests to verify O(1) lookup performance
func BenchmarkTaxonomyRegistry_NodeType(b *testing.B) {
	taxonomy, _ := LoadTaxonomy()
	registry, _ := NewTaxonomyRegistry(taxonomy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.NodeType("domain")
	}
}

func BenchmarkTaxonomyRegistry_IsCanonicalNodeType(b *testing.B) {
	taxonomy, _ := LoadTaxonomy()
	registry, _ := NewTaxonomyRegistry(taxonomy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.IsCanonicalNodeType("domain")
	}
}

func BenchmarkTaxonomyRegistry_GenerateNodeID(b *testing.B) {
	taxonomy, _ := LoadTaxonomy()
	registry, _ := NewTaxonomyRegistry(taxonomy)

	props := map[string]any{"name": "example.com"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GenerateNodeID("domain", props)
	}
}
