package taxonomy

import (
	"strings"
	"testing"
)

func TestTaxonomyValidator_ValidateNode(t *testing.T) {
	validator := NewTaxonomyValidator()

	tests := []struct {
		name    string
		node    *NodeTypeDefinition
		wantErr bool
		errType ErrorType
	}{
		{
			name: "valid node",
			node: &NodeTypeDefinition{
				ID:          "node_test",
				Name:        "Test Node",
				Type:        "test",
				Category:    "asset",
				Description: "Test node type",
				IDTemplate:  "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true, Description: "ID"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			node: &NodeTypeDefinition{
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing name",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing type",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing category",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing id_template",
			node: &NodeTypeDefinition{
				ID:       "node_test",
				Name:     "Test Node",
				Type:     "test",
				Category: "asset",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "id_template references undefined property",
			node: &NodeTypeDefinition{
				ID:          "node_test",
				Name:        "Test Node",
				Type:        "test",
				Category:    "asset",
				IDTemplate:  "test:{undefined_prop}",
				Properties:  []PropertyDefinition{},
			},
			wantErr: true,
			errType: ErrorTypeInvalidReference,
		},
		{
			name: "id_template with sha256 function",
			node: &NodeTypeDefinition{
				ID:          "node_test",
				Name:        "Test Node",
				Type:        "test",
				Category:    "asset",
				IDTemplate:  "test:{sha256(name)}",
				Properties: []PropertyDefinition{
					{Name: "name", Type: "string", Required: true},
				},
			},
			wantErr: false,
		},
		{
			name: "id_template with uuid",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{uuid}",
				Properties: []PropertyDefinition{},
			},
			wantErr: false,
		},
		{
			name: "property missing name",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:static", // Don't reference the missing property
				Properties: []PropertyDefinition{
					{Type: "string", Required: true}, // Missing Name field
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "property missing type",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "property with invalid type",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "invalid_type", Required: true},
				},
			},
			wantErr: true,
			errType: ErrorTypeInvalidProperty,
		},
		{
			name: "property with valid array type",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
					{Name: "tags", Type: "[]string", Required: false},
				},
			},
			wantErr: false,
		},
		{
			name: "property with valid map type",
			node: &NodeTypeDefinition{
				ID:         "node_test",
				Name:       "Test Node",
				Type:       "test",
				Category:   "asset",
				IDTemplate: "test:{id}",
				Properties: []PropertyDefinition{
					{Name: "id", Type: "string", Required: true},
					{Name: "metadata", Type: "map[string]any", Required: false},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateNode(tt.node)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("ValidateNode() error type = %T, want *TaxonomyError", err)
					return
				}

				if taxErr.Type != tt.errType {
					t.Errorf("ValidateNode() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}
		})
	}
}

func TestTaxonomyValidator_ValidateRelationship(t *testing.T) {
	validator := NewTaxonomyValidator()

	// Create node types for validation
	nodeTypes := map[string]bool{
		"domain":    true,
		"subdomain": true,
		"host":      true,
		"port":      true,
	}

	tests := []struct {
		name    string
		rel     *RelationshipTypeDefinition
		wantErr bool
		errType ErrorType
	}{
		{
			name: "valid relationship",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				Description:   "Test relationship",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
				Properties:    []PropertyDefinition{},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			rel: &RelationshipTypeDefinition{
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing name",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing type",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing from_types",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing to_types",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "invalid from_type reference",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"unknown_type"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeInvalidReference,
		},
		{
			name: "invalid to_type reference",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"unknown_type"},
				Bidirectional: false,
			},
			wantErr: true,
			errType: ErrorTypeInvalidReference,
		},
		{
			name: "multiple from_types valid",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain", "subdomain"},
				ToTypes:       []string{"host"},
				Bidirectional: false,
			},
			wantErr: false,
		},
		{
			name: "multiple to_types valid",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain", "host"},
				Bidirectional: false,
			},
			wantErr: false,
		},
		{
			name: "relationship with valid property",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
				Properties: []PropertyDefinition{
					{Name: "confidence", Type: "float64", Required: false},
				},
			},
			wantErr: false,
		},
		{
			name: "relationship with invalid property",
			rel: &RelationshipTypeDefinition{
				ID:            "rel_test",
				Name:          "Test Relationship",
				Type:          "TEST_REL",
				FromTypes:     []string{"domain"},
				ToTypes:       []string{"subdomain"},
				Bidirectional: false,
				Properties: []PropertyDefinition{
					{Name: "bad_prop", Type: "invalid_type", Required: false},
				},
			},
			wantErr: true,
			errType: ErrorTypeInvalidProperty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateRelationship(tt.rel, nodeTypes)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRelationship() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("ValidateRelationship() error type = %T, want *TaxonomyError", err)
					return
				}

				if taxErr.Type != tt.errType {
					t.Errorf("ValidateRelationship() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}
		})
	}
}

func TestTaxonomyValidator_validateTechnique(t *testing.T) {
	validator := &taxonomyValidator{}

	tests := []struct {
		name    string
		tech    *TechniqueDefinition
		wantErr bool
		errType ErrorType
	}{
		{
			name: "valid MITRE technique",
			tech: &TechniqueDefinition{
				TechniqueID: "T1190",
				Name:        "Exploit Public-Facing Application",
				Taxonomy:    "mitre",
				Description: "Test technique",
				Tactic:      "initial-access",
				Platforms:   []string{"linux"},
			},
			wantErr: false,
		},
		{
			name: "valid Arcanum technique",
			tech: &TechniqueDefinition{
				TechniqueID: "ARC-T001",
				Name:        "Direct Prompt Injection",
				Taxonomy:    "arcanum",
				Description: "Test technique",
			},
			wantErr: false,
		},
		{
			name: "valid custom technique",
			tech: &TechniqueDefinition{
				TechniqueID: "CUSTOM-001",
				Name:        "Custom Technique",
				Taxonomy:    "custom",
				Description: "Test technique",
			},
			wantErr: false,
		},
		{
			name: "missing technique_id",
			tech: &TechniqueDefinition{
				Name:        "Test Technique",
				Taxonomy:    "mitre",
				Description: "Test technique",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing name",
			tech: &TechniqueDefinition{
				TechniqueID: "T1190",
				Taxonomy:    "mitre",
				Description: "Test technique",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing taxonomy",
			tech: &TechniqueDefinition{
				TechniqueID: "T1190",
				Name:        "Test Technique",
				Description: "Test technique",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "invalid taxonomy value",
			tech: &TechniqueDefinition{
				TechniqueID: "T1190",
				Name:        "Test Technique",
				Taxonomy:    "invalid_taxonomy",
				Description: "Test technique",
			},
			wantErr: true,
			errType: ErrorTypeInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateTechnique(tt.tech)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateTechnique() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("validateTechnique() error type = %T, want *TaxonomyError", err)
					return
				}

				if taxErr.Type != tt.errType {
					t.Errorf("validateTechnique() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}
		})
	}
}

func TestTaxonomyValidator_Validate(t *testing.T) {
	validator := NewTaxonomyValidator()

	tests := []struct {
		name    string
		setup   func() *Taxonomy
		wantErr bool
	}{
		{
			name: "valid complete taxonomy",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")

				// Add node types
				_ = tax.AddNodeType(&NodeTypeDefinition{
					ID:         "node_domain",
					Name:       "Domain",
					Type:       "domain",
					Category:   "asset",
					IDTemplate: "domain:{name}",
					Properties: []PropertyDefinition{
						{Name: "name", Type: "string", Required: true},
					},
				})
				_ = tax.AddNodeType(&NodeTypeDefinition{
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
				_ = tax.AddRelationship(&RelationshipTypeDefinition{
					ID:            "rel_has_subdomain",
					Name:          "Has Subdomain",
					Type:          "HAS_SUBDOMAIN",
					FromTypes:     []string{"domain"},
					ToTypes:       []string{"subdomain"},
					Bidirectional: false,
				})

				// Add technique
				_ = tax.AddTechnique(&TechniqueDefinition{
					TechniqueID: "T1190",
					Name:        "Exploit Public-Facing Application",
					Taxonomy:    "mitre",
				})

				return tax
			},
			wantErr: false,
		},
		{
			name: "invalid node type in taxonomy",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")

				// Add invalid node type (missing required field)
				_ = tax.AddNodeType(&NodeTypeDefinition{
					Name:       "Invalid Node",
					Type:       "invalid",
					Category:   "asset",
					IDTemplate: "invalid:{id}",
					// Missing ID field
				})

				return tax
			},
			wantErr: true,
		},
		{
			name: "relationship referencing non-existent node type",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")

				// Add node type
				_ = tax.AddNodeType(&NodeTypeDefinition{
					ID:         "node_domain",
					Name:       "Domain",
					Type:       "domain",
					Category:   "asset",
					IDTemplate: "domain:{name}",
					Properties: []PropertyDefinition{
						{Name: "name", Type: "string", Required: true},
					},
				})

				// Add relationship referencing non-existent type
				_ = tax.AddRelationship(&RelationshipTypeDefinition{
					ID:            "rel_invalid",
					Name:          "Invalid Rel",
					Type:          "INVALID_REL",
					FromTypes:     []string{"domain"},
					ToTypes:       []string{"nonexistent_type"},
					Bidirectional: false,
				})

				return tax
			},
			wantErr: true,
		},
		{
			name: "invalid technique",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")

				// Add invalid technique (invalid taxonomy value)
				_ = tax.AddTechnique(&TechniqueDefinition{
					TechniqueID: "T1190",
					Name:        "Test",
					Taxonomy:    "invalid_taxonomy",
				})

				return tax
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := tt.setup()
			err := validator.Validate(taxonomy)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTaxonomy(t *testing.T) {
	// Test convenience function
	taxonomy, err := LoadTaxonomy()
	if err != nil {
		t.Fatalf("LoadTaxonomy() error = %v", err)
	}

	err = ValidateTaxonomy(taxonomy)
	if err != nil {
		t.Errorf("ValidateTaxonomy() error = %v (embedded taxonomy should be valid)", err)
	}
}

func TestTaxonomyValidator_validateIDTemplate(t *testing.T) {
	validator := &taxonomyValidator{}

	tests := []struct {
		name       string
		template   string
		properties []PropertyDefinition
		wantErr    bool
	}{
		{
			name:     "valid template with property",
			template: "domain:{name}",
			properties: []PropertyDefinition{
				{Name: "name", Type: "string", Required: true},
			},
			wantErr: false,
		},
		{
			name:     "valid template with multiple properties",
			template: "host:{ip}:{port}",
			properties: []PropertyDefinition{
				{Name: "ip", Type: "string", Required: true},
				{Name: "port", Type: "int", Required: true},
			},
			wantErr: false,
		},
		{
			name:     "valid template with sha256 function",
			template: "finding:{sha256(title+description)}",
			properties: []PropertyDefinition{
				{Name: "title", Type: "string", Required: true},
				{Name: "description", Type: "string", Required: true},
			},
			wantErr: false,
		},
		{
			name:       "valid template with uuid",
			template:   "mission:{uuid}",
			properties: []PropertyDefinition{},
			wantErr:    false,
		},
		{
			name:       "valid template with timestamp",
			template:   "event:{timestamp}",
			properties: []PropertyDefinition{},
			wantErr:    false,
		},
		{
			name:     "invalid template - undefined property",
			template: "domain:{undefined_prop}",
			properties: []PropertyDefinition{
				{Name: "name", Type: "string", Required: true},
			},
			wantErr: true,
		},
		{
			name:     "invalid template - one property defined, one not",
			template: "host:{ip}:{undefined_port}",
			properties: []PropertyDefinition{
				{Name: "ip", Type: "string", Required: true},
			},
			wantErr: true,
		},
		{
			name:       "valid template with no placeholders",
			template:   "static-id",
			properties: []PropertyDefinition{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateIDTemplate(tt.template, tt.properties)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateIDTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil {
				if !strings.Contains(err.Error(), "placeholder") {
					t.Errorf("validateIDTemplate() error should mention 'placeholder', got: %v", err)
				}
			}
		})
	}
}

// TestTaxonomyValidator_validateTargetType tests validation of target types.
func TestTaxonomyValidator_validateTargetType(t *testing.T) {
	validator := &taxonomyValidator{}

	tests := []struct {
		name    string
		target  *TargetTypeDefinition
		wantErr bool
		errType ErrorType
	}{
		{
			name: "valid target type",
			target: &TargetTypeDefinition{
				ID:          "target.web.http_api",
				Type:        "http_api",
				Name:        "HTTP API",
				Category:    "web",
				Description: "HTTP-based REST API",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			target: &TargetTypeDefinition{
				Type:     "http_api",
				Name:     "HTTP API",
				Category: "web",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Type",
			target: &TargetTypeDefinition{
				ID:       "target.web.http_api",
				Name:     "HTTP API",
				Category: "web",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Name",
			target: &TargetTypeDefinition{
				ID:       "target.web.http_api",
				Type:     "http_api",
				Category: "web",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Category",
			target: &TargetTypeDefinition{
				ID:   "target.web.http_api",
				Type: "http_api",
				Name: "HTTP API",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "invalid category",
			target: &TargetTypeDefinition{
				ID:       "target.web.http_api",
				Type:     "http_api",
				Name:     "HTTP API",
				Category: "invalid_category",
			},
			wantErr: true,
			errType: ErrorTypeInvalidFormat,
		},
		{
			name: "valid web category",
			target: &TargetTypeDefinition{
				ID:       "target.web.http_api",
				Type:     "http_api",
				Name:     "HTTP API",
				Category: "web",
			},
			wantErr: false,
		},
		{
			name: "valid ai category",
			target: &TargetTypeDefinition{
				ID:       "target.ai.llm",
				Type:     "llm",
				Name:     "LLM",
				Category: "ai",
			},
			wantErr: false,
		},
		{
			name: "valid infrastructure category",
			target: &TargetTypeDefinition{
				ID:       "target.infrastructure.server",
				Type:     "server",
				Name:     "Server",
				Category: "infrastructure",
			},
			wantErr: false,
		},
		{
			name: "valid cloud category",
			target: &TargetTypeDefinition{
				ID:       "target.cloud.aws",
				Type:     "aws",
				Name:     "AWS",
				Category: "cloud",
			},
			wantErr: false,
		},
		{
			name: "valid blockchain category",
			target: &TargetTypeDefinition{
				ID:       "target.blockchain.ethereum",
				Type:     "ethereum",
				Name:     "Ethereum",
				Category: "blockchain",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateTargetType(tt.target)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateTargetType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("validateTargetType() error type = %T, want *TaxonomyError", err)
					return
				}
				if taxErr.Type != tt.errType {
					t.Errorf("validateTargetType() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}
		})
	}
}

// TestTaxonomyValidator_validateTechniqueType tests validation of technique types.
func TestTaxonomyValidator_validateTechniqueType(t *testing.T) {
	validator := &taxonomyValidator{}

	tests := []struct {
		name      string
		technique *TechniqueTypeDefinition
		wantErr   bool
		errType   ErrorType
	}{
		{
			name: "valid technique type",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				MITREIDs:        []string{"T1190"},
				DefaultSeverity: "high",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			technique: &TechniqueTypeDefinition{
				Type:     "ssrf",
				Name:     "SSRF",
				Category: "initial_access",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Type",
			technique: &TechniqueTypeDefinition{
				ID:       "technique.initial_access.ssrf",
				Name:     "SSRF",
				Category: "initial_access",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Name",
			technique: &TechniqueTypeDefinition{
				ID:       "technique.initial_access.ssrf",
				Type:     "ssrf",
				Category: "initial_access",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Category",
			technique: &TechniqueTypeDefinition{
				ID:   "technique.initial_access.ssrf",
				Type: "ssrf",
				Name: "SSRF",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "invalid category",
			technique: &TechniqueTypeDefinition{
				ID:       "technique.initial_access.ssrf",
				Type:     "ssrf",
				Name:     "SSRF",
				Category: "invalid_tactic",
			},
			wantErr: true,
			errType: ErrorTypeInvalidFormat,
		},
		{
			name: "invalid severity",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				DefaultSeverity: "super_critical",
			},
			wantErr: true,
			errType: ErrorTypeInvalidFormat,
		},
		{
			name: "valid critical severity",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				DefaultSeverity: "critical",
			},
			wantErr: false,
		},
		{
			name: "valid high severity",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				DefaultSeverity: "high",
			},
			wantErr: false,
		},
		{
			name: "valid medium severity",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				DefaultSeverity: "medium",
			},
			wantErr: false,
		},
		{
			name: "valid low severity",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				DefaultSeverity: "low",
			},
			wantErr: false,
		},
		{
			name: "valid info severity",
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "SSRF",
				Category:        "initial_access",
				DefaultSeverity: "info",
			},
			wantErr: false,
		},
		{
			name: "valid execution category",
			technique: &TechniqueTypeDefinition{
				ID:       "technique.execution.code_injection",
				Type:     "code_injection",
				Name:     "Code Injection",
				Category: "execution",
			},
			wantErr: false,
		},
		{
			name: "valid privilege_escalation category",
			technique: &TechniqueTypeDefinition{
				ID:       "technique.privilege_escalation.sudo_abuse",
				Type:     "sudo_abuse",
				Name:     "Sudo Abuse",
				Category: "privilege_escalation",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateTechniqueType(tt.technique)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateTechniqueType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("validateTechniqueType() error type = %T, want *TaxonomyError", err)
					return
				}
				if taxErr.Type != tt.errType {
					t.Errorf("validateTechniqueType() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}
		})
	}
}

// TestTaxonomyValidator_validateCapability tests validation of capabilities.
func TestTaxonomyValidator_validateCapability(t *testing.T) {
	validator := &taxonomyValidator{}

	// Create technique types for reference validation
	techniqueTypes := map[string]*TechniqueTypeDefinition{
		"ssrf": {
			ID:   "technique.initial_access.ssrf",
			Type: "ssrf",
			Name: "SSRF",
		},
		"sqli": {
			ID:   "technique.initial_access.sqli",
			Type: "sqli",
			Name: "SQL Injection",
		},
	}

	tests := []struct {
		name       string
		capability *CapabilityDefinition
		wantErr    bool
		errType    ErrorType
	}{
		{
			name: "valid capability",
			capability: &CapabilityDefinition{
				ID:             "capability.web_vulnerability_scanning",
				Name:           "Web Vulnerability Scanning",
				Description:    "Comprehensive web scanning",
				TechniqueTypes: []string{"ssrf", "sqli"},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			capability: &CapabilityDefinition{
				Name:        "Web Scanning",
				Description: "Web scanning",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "missing Name",
			capability: &CapabilityDefinition{
				ID:          "capability.web_vulnerability_scanning",
				Description: "Web scanning",
			},
			wantErr: true,
			errType: ErrorTypeMissingField,
		},
		{
			name: "valid with non-existent technique types (warns but passes)",
			capability: &CapabilityDefinition{
				ID:             "capability.web_vulnerability_scanning",
				Name:           "Web Scanning",
				Description:    "Web scanning",
				TechniqueTypes: []string{"ssrf", "nonexistent"},
			},
			wantErr: false, // Should warn but not fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateCapability(tt.capability, techniqueTypes)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateCapability() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("validateCapability() error type = %T, want *TaxonomyError", err)
					return
				}
				if taxErr.Type != tt.errType {
					t.Errorf("validateCapability() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}
		})
	}
}
