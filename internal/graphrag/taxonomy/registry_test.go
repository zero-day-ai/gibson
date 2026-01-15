package taxonomy

import (
	"context"
	"os"
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

// TestTaxonomyRegistry_GetTargetType tests retrieving target types from registry.
func TestTaxonomyRegistry_GetTargetType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test target types
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.web.http_api",
		Type:     "http_api",
		Name:     "HTTP API",
		Category: "web",
	})
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.ai.llm",
		Type:     "llm",
		Name:     "Large Language Model",
		Category: "ai",
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
			name:     "existing target type",
			typeName: "http_api",
			wantOk:   true,
		},
		{
			name:     "non-existent target type",
			typeName: "nonexistent",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetDef, ok := registry.GetTargetType(tt.typeName)

			if ok != tt.wantOk {
				t.Errorf("GetTargetType() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk {
				if targetDef == nil {
					t.Error("GetTargetType() returned nil for existing type")
				} else if targetDef.Type != tt.typeName {
					t.Errorf("GetTargetType() type = %v, want %v", targetDef.Type, tt.typeName)
				}
			}
		})
	}
}

// TestTaxonomyRegistry_ListTargetTypes tests listing all target types.
func TestTaxonomyRegistry_ListTargetTypes(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test target types
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.web.http_api",
		Type:     "http_api",
		Name:     "HTTP API",
		Category: "web",
	})
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.ai.llm",
		Type:     "llm",
		Name:     "LLM",
		Category: "ai",
	})
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.web.graphql",
		Type:     "graphql",
		Name:     "GraphQL",
		Category: "web",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	targetTypes := registry.ListTargetTypes()

	if len(targetTypes) != 3 {
		t.Errorf("ListTargetTypes() returned %d types, want 3", len(targetTypes))
	}

	// Check that returned types match what we added
	typeMap := make(map[string]bool)
	for _, tt := range targetTypes {
		typeMap[tt.Type] = true
	}

	if !typeMap["http_api"] {
		t.Error("ListTargetTypes() missing 'http_api' type")
	}
	if !typeMap["llm"] {
		t.Error("ListTargetTypes() missing 'llm' type")
	}
	if !typeMap["graphql"] {
		t.Error("ListTargetTypes() missing 'graphql' type")
	}
}

// TestTaxonomyRegistry_ListTargetTypesByCategory tests filtering target types by category.
func TestTaxonomyRegistry_ListTargetTypesByCategory(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test target types
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.web.http_api",
		Type:     "http_api",
		Name:     "HTTP API",
		Category: "web",
	})
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.ai.llm",
		Type:     "llm",
		Name:     "LLM",
		Category: "ai",
	})
	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.web.graphql",
		Type:     "graphql",
		Name:     "GraphQL",
		Category: "web",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name      string
		category  string
		wantCount int
	}{
		{
			name:      "web category",
			category:  "web",
			wantCount: 2,
		},
		{
			name:      "ai category",
			category:  "ai",
			wantCount: 1,
		},
		{
			name:      "non-existent category",
			category:  "nonexistent",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetTypes := registry.ListTargetTypesByCategory(tt.category)

			if len(targetTypes) != tt.wantCount {
				t.Errorf("ListTargetTypesByCategory(%q) returned %d types, want %d", tt.category, len(targetTypes), tt.wantCount)
			}
		})
	}
}

// TestTaxonomyRegistry_ValidateTargetType tests target type validation.
func TestTaxonomyRegistry_ValidateTargetType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	_ = taxonomy.AddTargetType(&TargetTypeDefinition{
		ID:       "target.web.http_api",
		Type:     "http_api",
		Name:     "HTTP API",
		Category: "web",
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
			name:     "existing type",
			typeName: "http_api",
			want:     true,
		},
		{
			name:     "non-existent type",
			typeName: "nonexistent",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.ValidateTargetType(tt.typeName); got != tt.want {
				t.Errorf("ValidateTargetType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTaxonomyRegistry_GetTechniqueType tests retrieving technique types from registry.
func TestTaxonomyRegistry_GetTechniqueType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test technique types
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.ssrf",
		Type:     "ssrf",
		Name:     "SSRF",
		Category: "initial_access",
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.sqli",
		Type:     "sqli",
		Name:     "SQL Injection",
		Category: "initial_access",
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
			name:     "existing technique type",
			typeName: "ssrf",
			wantOk:   true,
		},
		{
			name:     "non-existent technique type",
			typeName: "nonexistent",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			techniqueDef, ok := registry.GetTechniqueType(tt.typeName)

			if ok != tt.wantOk {
				t.Errorf("GetTechniqueType() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk {
				if techniqueDef == nil {
					t.Error("GetTechniqueType() returned nil for existing type")
				} else if techniqueDef.Type != tt.typeName {
					t.Errorf("GetTechniqueType() type = %v, want %v", techniqueDef.Type, tt.typeName)
				}
			}
		})
	}
}

// TestTaxonomyRegistry_ListTechniqueTypes tests listing all technique types.
func TestTaxonomyRegistry_ListTechniqueTypes(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test technique types
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.ssrf",
		Type:     "ssrf",
		Name:     "SSRF",
		Category: "initial_access",
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.sqli",
		Type:     "sqli",
		Name:     "SQL Injection",
		Category: "initial_access",
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.execution.code_injection",
		Type:     "code_injection",
		Name:     "Code Injection",
		Category: "execution",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	techniqueTypes := registry.ListTechniqueTypes()

	if len(techniqueTypes) != 3 {
		t.Errorf("ListTechniqueTypes() returned %d types, want 3", len(techniqueTypes))
	}

	// Check that returned types match what we added
	typeMap := make(map[string]bool)
	for _, tt := range techniqueTypes {
		typeMap[tt.Type] = true
	}

	if !typeMap["ssrf"] {
		t.Error("ListTechniqueTypes() missing 'ssrf' type")
	}
	if !typeMap["sqli"] {
		t.Error("ListTechniqueTypes() missing 'sqli' type")
	}
	if !typeMap["code_injection"] {
		t.Error("ListTechniqueTypes() missing 'code_injection' type")
	}
}

// TestTaxonomyRegistry_ListTechniqueTypesByCategory tests filtering technique types by category.
func TestTaxonomyRegistry_ListTechniqueTypesByCategory(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test technique types
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.ssrf",
		Type:     "ssrf",
		Name:     "SSRF",
		Category: "initial_access",
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.sqli",
		Type:     "sqli",
		Name:     "SQL Injection",
		Category: "initial_access",
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.execution.code_injection",
		Type:     "code_injection",
		Name:     "Code Injection",
		Category: "execution",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name      string
		category  string
		wantCount int
	}{
		{
			name:      "initial_access category",
			category:  "initial_access",
			wantCount: 2,
		},
		{
			name:      "execution category",
			category:  "execution",
			wantCount: 1,
		},
		{
			name:      "non-existent category",
			category:  "nonexistent",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			techniqueTypes := registry.ListTechniqueTypesByCategory(tt.category)

			if len(techniqueTypes) != tt.wantCount {
				t.Errorf("ListTechniqueTypesByCategory(%q) returned %d types, want %d", tt.category, len(techniqueTypes), tt.wantCount)
			}
		})
	}
}

// TestTaxonomyRegistry_ValidateTechniqueType tests technique type validation.
func TestTaxonomyRegistry_ValidateTechniqueType(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.ssrf",
		Type:     "ssrf",
		Name:     "SSRF",
		Category: "initial_access",
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
			name:     "existing type",
			typeName: "ssrf",
			want:     true,
		},
		{
			name:     "non-existent type",
			typeName: "nonexistent",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.ValidateTechniqueType(tt.typeName); got != tt.want {
				t.Errorf("ValidateTechniqueType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTaxonomyRegistry_GetTechniqueTypesByMITRE tests retrieving technique types by MITRE ID.
func TestTaxonomyRegistry_GetTechniqueTypesByMITRE(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test technique types with MITRE IDs
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.ssrf",
		Type:     "ssrf",
		Name:     "SSRF",
		Category: "initial_access",
		MITREIDs: []string{"T1190"},
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.initial_access.sqli",
		Type:     "sqli",
		Name:     "SQL Injection",
		Category: "initial_access",
		MITREIDs: []string{"T1190", "T1059"},
	})
	_ = taxonomy.AddTechniqueType(&TechniqueTypeDefinition{
		ID:       "technique.execution.code_injection",
		Type:     "code_injection",
		Name:     "Code Injection",
		Category: "execution",
		MITREIDs: []string{"T1059"},
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name      string
		mitreID   string
		wantCount int
	}{
		{
			name:      "T1190 - found in 2 technique types",
			mitreID:   "T1190",
			wantCount: 2,
		},
		{
			name:      "T1059 - found in 2 technique types",
			mitreID:   "T1059",
			wantCount: 2,
		},
		{
			name:      "non-existent MITRE ID",
			mitreID:   "T9999",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			techniqueTypes := registry.GetTechniqueTypesByMITRE(tt.mitreID)

			if len(techniqueTypes) != tt.wantCount {
				t.Errorf("GetTechniqueTypesByMITRE(%q) returned %d types, want %d", tt.mitreID, len(techniqueTypes), tt.wantCount)
			}
		})
	}
}

// TestTaxonomyRegistry_GetCapability tests retrieving capabilities from registry.
func TestTaxonomyRegistry_GetCapability(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test capabilities
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:             "capability.web_vulnerability_scanning",
		Name:           "Web Vulnerability Scanning",
		Description:    "Comprehensive web scanning",
		TechniqueTypes: []string{"ssrf", "sqli"},
	})
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:          "capability.api_security_testing",
		Name:        "API Security Testing",
		Description: "API testing",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name   string
		id     string
		wantOk bool
	}{
		{
			name:   "existing capability",
			id:     "capability.web_vulnerability_scanning",
			wantOk: true,
		},
		{
			name:   "non-existent capability",
			id:     "capability.nonexistent",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capDef, ok := registry.GetCapability(tt.id)

			if ok != tt.wantOk {
				t.Errorf("GetCapability() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk {
				if capDef == nil {
					t.Error("GetCapability() returned nil for existing capability")
				} else if capDef.ID != tt.id {
					t.Errorf("GetCapability() ID = %v, want %v", capDef.ID, tt.id)
				}
			}
		})
	}
}

// TestTaxonomyRegistry_ListCapabilities tests listing all capabilities.
func TestTaxonomyRegistry_ListCapabilities(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test capabilities
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:          "capability.web_vulnerability_scanning",
		Name:        "Web Vulnerability Scanning",
		Description: "Web scanning",
	})
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:          "capability.api_security_testing",
		Name:        "API Security Testing",
		Description: "API testing",
	})
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:          "capability.cloud_security_testing",
		Name:        "Cloud Security Testing",
		Description: "Cloud testing",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	capabilities := registry.ListCapabilities()

	if len(capabilities) != 3 {
		t.Errorf("ListCapabilities() returned %d capabilities, want 3", len(capabilities))
	}

	// Check that returned capabilities match what we added
	capMap := make(map[string]bool)
	for _, cap := range capabilities {
		capMap[cap.ID] = true
	}

	if !capMap["capability.web_vulnerability_scanning"] {
		t.Error("ListCapabilities() missing 'web_vulnerability_scanning' capability")
	}
	if !capMap["capability.api_security_testing"] {
		t.Error("ListCapabilities() missing 'api_security_testing' capability")
	}
	if !capMap["capability.cloud_security_testing"] {
		t.Error("ListCapabilities() missing 'cloud_security_testing' capability")
	}
}

// TestTaxonomyRegistry_GetTechniqueTypesForCapability tests retrieving technique types for a capability.
func TestTaxonomyRegistry_GetTechniqueTypesForCapability(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test capabilities
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:             "capability.web_vulnerability_scanning",
		Name:           "Web Vulnerability Scanning",
		Description:    "Web scanning",
		TechniqueTypes: []string{"ssrf", "sqli", "xss"},
	})
	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:             "capability.api_security_testing",
		Name:           "API Security Testing",
		Description:    "API testing",
		TechniqueTypes: []string{"api_abuse"},
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name       string
		capability string
		wantCount  int
	}{
		{
			name:       "web scanning capability",
			capability: "capability.web_vulnerability_scanning",
			wantCount:  3,
		},
		{
			name:       "api testing capability",
			capability: "capability.api_security_testing",
			wantCount:  1,
		},
		{
			name:       "non-existent capability",
			capability: "capability.nonexistent",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			techniqueTypes := registry.GetTechniqueTypesForCapability(tt.capability)

			if len(techniqueTypes) != tt.wantCount {
				t.Errorf("GetTechniqueTypesForCapability(%q) returned %d types, want %d", tt.capability, len(techniqueTypes), tt.wantCount)
			}
		})
	}
}

// TestTaxonomyRegistry_ValidateCapability tests capability validation.
func TestTaxonomyRegistry_ValidateCapability(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	_ = taxonomy.AddCapability(&CapabilityDefinition{
		ID:          "capability.web_vulnerability_scanning",
		Name:        "Web Vulnerability Scanning",
		Description: "Web scanning",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{
			name: "existing capability",
			id:   "capability.web_vulnerability_scanning",
			want: true,
		},
		{
			name: "non-existent capability",
			id:   "capability.nonexistent",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.ValidateCapability(tt.id); got != tt.want {
				t.Errorf("ValidateCapability() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTaxonomyRegistry_GetExecutionEvent tests retrieving execution events from registry.
func TestTaxonomyRegistry_GetExecutionEvent(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test execution events
	_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
		EventType:   "tool.call.started",
		Description: "Tool execution started",
	})
	_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
		EventType:   "tool.call.completed",
		Description: "Tool execution completed",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name      string
		eventType string
		wantNil   bool
	}{
		{
			name:      "existing event",
			eventType: "tool.call.started",
			wantNil:   false,
		},
		{
			name:      "non-existent event",
			eventType: "nonexistent",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventDef := registry.GetExecutionEvent(tt.eventType)

			if (eventDef == nil) != tt.wantNil {
				t.Errorf("GetExecutionEvent() nil = %v, want %v", eventDef == nil, tt.wantNil)
			}

			if !tt.wantNil {
				if eventDef.EventType != tt.eventType {
					t.Errorf("GetExecutionEvent() event_type = %v, want %v", eventDef.EventType, tt.eventType)
				}
			}
		})
	}
}

// TestTaxonomyRegistry_GetToolOutputSchema tests retrieving tool output schemas from registry.
func TestTaxonomyRegistry_GetToolOutputSchema(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test tool output schemas
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:         "nmap",
		Description:  "Nmap scan output schema",
		OutputFormat: "json",
	})
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:         "sqlmap",
		Description:  "SQLMap output schema",
		OutputFormat: "json",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name     string
		toolName string
		wantNil  bool
	}{
		{
			name:     "existing schema",
			toolName: "nmap",
			wantNil:  false,
		},
		{
			name:     "non-existent schema",
			toolName: "nonexistent",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaDef := registry.GetToolOutputSchema(tt.toolName)

			if (schemaDef == nil) != tt.wantNil {
				t.Errorf("GetToolOutputSchema() nil = %v, want %v", schemaDef == nil, tt.wantNil)
			}

			if !tt.wantNil {
				if schemaDef.Tool != tt.toolName {
					t.Errorf("GetToolOutputSchema() tool = %v, want %v", schemaDef.Tool, tt.toolName)
				}
			}
		})
	}
}

// TestTaxonomyRegistry_ListExecutionEvents tests listing all execution events.
func TestTaxonomyRegistry_ListExecutionEvents(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test execution events
	_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
		EventType:   "tool.call.started",
		Description: "Tool Started",
	})
	_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
		EventType:   "tool.call.completed",
		Description: "Tool Completed",
	})
	_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
		EventType:   "agent.iteration.started",
		Description: "Agent Started",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	eventTypes := registry.ListExecutionEvents()

	if len(eventTypes) != 3 {
		t.Errorf("ListExecutionEvents() returned %d event types, want 3", len(eventTypes))
	}

	// Check that returned event types match what we added
	eventMap := make(map[string]bool)
	for _, et := range eventTypes {
		eventMap[et] = true
	}

	if !eventMap["tool.call.started"] {
		t.Error("ListExecutionEvents() missing 'tool.call.started' event type")
	}
	if !eventMap["tool.call.completed"] {
		t.Error("ListExecutionEvents() missing 'tool.call.completed' event type")
	}
	if !eventMap["agent.iteration.started"] {
		t.Error("ListExecutionEvents() missing 'agent.iteration.started' event type")
	}
}

// TestTaxonomyRegistry_ListToolOutputSchemas tests listing all tool output schemas.
func TestTaxonomyRegistry_ListToolOutputSchemas(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test tool output schemas
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "nmap",
		Description: "Nmap Scan Results",
	})
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "sqlmap",
		Description: "SQLMap Results",
	})
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "nuclei",
		Description: "Nuclei Results",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	toolNames := registry.ListToolOutputSchemas()

	if len(toolNames) != 3 {
		t.Errorf("ListToolOutputSchemas() returned %d tool names, want 3", len(toolNames))
	}

	// Check that returned tool names match what we added
	toolMap := make(map[string]bool)
	for _, tn := range toolNames {
		toolMap[tn] = true
	}

	if !toolMap["nmap"] {
		t.Error("ListToolOutputSchemas() missing 'nmap' tool name")
	}
	if !toolMap["sqlmap"] {
		t.Error("ListToolOutputSchemas() missing 'sqlmap' tool name")
	}
	if !toolMap["nuclei"] {
		t.Error("ListToolOutputSchemas() missing 'nuclei' tool name")
	}
}

// TestTaxonomyRegistry_HasExecutionEvent tests checking if execution events exist.
func TestTaxonomyRegistry_HasExecutionEvent(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
		EventType:   "tool_started",
		Description: "Tool Started",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name      string
		eventType string
		want      bool
	}{
		{
			name:      "existing event",
			eventType: "tool_started",
			want:      true,
		},
		{
			name:      "non-existent event",
			eventType: "nonexistent",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.HasExecutionEvent(tt.eventType); got != tt.want {
				t.Errorf("HasExecutionEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTaxonomyRegistry_HasToolOutputSchema tests checking if tool output schemas exist.
func TestTaxonomyRegistry_HasToolOutputSchema(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "nmap",
		Description: "Nmap Scan Results",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{
			name:     "existing schema",
			toolName: "nmap",
			want:     true,
		},
		{
			name:     "non-existent schema",
			toolName: "nonexistent",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registry.HasToolOutputSchema(tt.toolName); got != tt.want {
				t.Errorf("HasToolOutputSchema() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTaxonomyRegistry_ExecutionEventConcurrency tests thread-safe access to execution events.
func TestTaxonomyRegistry_ExecutionEventConcurrency(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test execution events
	for i := 0; i < 10; i++ {
		_ = taxonomy.AddExecutionEvent(&ExecutionEventDefinition{
			EventType:   "test_event_" + string(rune('0'+i)),
			Description: "Test Event " + string(rune('0'+i)),
		})
	}

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			eventType := "test_event_" + string(rune('0'+(idx%10)))
			_ = registry.GetExecutionEvent(eventType)
			_ = registry.HasExecutionEvent(eventType)
			_ = registry.ListExecutionEvents()
		}(i)
	}

	wg.Wait()
}

// TestTaxonomyRegistry_ToolOutputSchemaConcurrency tests thread-safe access to tool output schemas.
func TestTaxonomyRegistry_ToolOutputSchemaConcurrency(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add test tool output schemas
	for i := 0; i < 10; i++ {
		_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
			Tool:        "test_tool_" + string(rune('0'+i)),
			Description: "Test Tool " + string(rune('0'+i)),
		})
	}

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			toolName := "test_tool_" + string(rune('0'+(idx%10)))
			_ = registry.GetToolOutputSchema(toolName)
			_ = registry.HasToolOutputSchema(toolName)
			_ = registry.ListToolOutputSchemas()
		}(i)
	}

	wg.Wait()
}

// TestTaxonomyRegistry_IsSchemaBasedToolSchema tests the IsSchemaBasedToolSchema method.
func TestTaxonomyRegistry_IsSchemaBasedToolSchema(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	// Add a YAML-based tool schema
	_ = taxonomy.AddToolOutputSchema(&ToolOutputSchema{
		Tool:        "yaml_tool",
		Description: "Tool loaded from YAML",
	})

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// All tools should NOT be schema-based initially
	if registry.IsSchemaBasedToolSchema("yaml_tool") {
		t.Error("expected yaml_tool to NOT be schema-based initially")
	}

	// Non-existent tools should also return false
	if registry.IsSchemaBasedToolSchema("nonexistent") {
		t.Error("expected nonexistent tool to return false")
	}
}

// TestTaxonomyRegistry_LoadToolSchemasFromBinaries_EmptyDir tests loading from non-existent directory.
func TestTaxonomyRegistry_LoadToolSchemasFromBinaries_EmptyDir(t *testing.T) {
	taxonomy := NewTaxonomy("0.1.0")

	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		t.Fatalf("NewTaxonomyRegistry() error = %v", err)
	}

	// Loading from non-existent directory should succeed with no tools loaded
	err = registry.LoadToolSchemasFromBinaries(context.Background(), "/nonexistent/dir", nil)
	if err != nil {
		t.Errorf("LoadToolSchemasFromBinaries() error = %v, want nil", err)
	}

	// No schema-based tools should be loaded
	if len(registry.ListToolOutputSchemas()) != 0 {
		t.Error("expected 0 tool schemas after loading from nonexistent dir")
	}
}

// TestLoadAndValidateTaxonomyWithTools tests the convenience function for loading with tools.
func TestLoadAndValidateTaxonomyWithTools(t *testing.T) {
	// Test with empty tools dir
	registry, err := LoadAndValidateTaxonomyWithTools(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("LoadAndValidateTaxonomyWithTools() error = %v", err)
	}

	if registry == nil {
		t.Fatal("LoadAndValidateTaxonomyWithTools() returned nil registry")
	}

	// Version should be valid
	if registry.Version() == "" {
		t.Error("expected non-empty version")
	}
}

// TestLoadAndValidateTaxonomyWithToolsAndCustom tests loading with custom taxonomy and tools.
func TestLoadAndValidateTaxonomyWithToolsAndCustom(t *testing.T) {
	// Create temp dir for custom taxonomy with proper structure
	tempDir := t.TempDir()

	// Create nodes directory (path must contain "nodes/" per loader.go)
	nodesDir := tempDir + "/nodes"
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		t.Fatalf("failed to create nodes dir: %v", err)
	}

	// Create node types file in the nodes/ directory
	nodeTypesContent := `
node_types:
  - type: CustomTestNode
    id: custom_test_node
    name: Custom Test Node
    category: custom
    description: A custom test node
    id_template: "custom:{value}"
    properties:
      - name: value
        type: string
        required: true
        description: Test value property
`
	nodeTypesFile := nodesDir + "/custom.yaml"
	if err := os.WriteFile(nodeTypesFile, []byte(nodeTypesContent), 0644); err != nil {
		t.Fatalf("failed to write nodes/custom.yaml: %v", err)
	}

	// Create root custom taxonomy file that includes the node_types file
	rootContent := `
version: "0.1.0"
includes:
  - nodes/custom.yaml
`
	customFile := tempDir + "/custom_taxonomy.yaml"
	if err := os.WriteFile(customFile, []byte(rootContent), 0644); err != nil {
		t.Fatalf("failed to write custom taxonomy root: %v", err)
	}

	// Test loading with custom taxonomy and empty tools dir
	registry, err := LoadAndValidateTaxonomyWithToolsAndCustom(context.Background(), "", customFile, nil)
	if err != nil {
		t.Fatalf("LoadAndValidateTaxonomyWithToolsAndCustom() error = %v", err)
	}

	if registry == nil {
		t.Fatal("LoadAndValidateTaxonomyWithToolsAndCustom() returned nil registry")
	}

	// Custom node type should exist
	if _, ok := registry.NodeType("CustomTestNode"); !ok {
		t.Error("expected CustomTestNode to exist in registry")
	}

	// Version should include +custom suffix
	if !strings.HasSuffix(registry.Version(), "+custom") {
		t.Errorf("expected version to end with +custom, got %s", registry.Version())
	}
}
