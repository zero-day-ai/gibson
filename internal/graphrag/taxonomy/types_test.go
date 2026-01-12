package taxonomy

import (
	"testing"
)

// TestTaxonomy_AddTargetType tests adding target types to taxonomy.
func TestTaxonomy_AddTargetType(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *Taxonomy
		target  *TargetTypeDefinition
		wantErr bool
		errType ErrorType
	}{
		{
			name: "add valid target type",
			setup: func() *Taxonomy {
				return NewTaxonomy("0.1.0")
			},
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
			name: "add duplicate target type by ID",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")
				_ = tax.AddTargetType(&TargetTypeDefinition{
					ID:       "target.web.http_api",
					Type:     "http_api",
					Name:     "HTTP API",
					Category: "web",
				})
				return tax
			},
			target: &TargetTypeDefinition{
				ID:       "target.web.http_api",
				Type:     "http_api_v2",
				Name:     "HTTP API V2",
				Category: "web",
			},
			wantErr: true,
			errType: ErrorTypeDuplicateDefinition,
		},
		{
			name: "add duplicate target type by Type",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")
				_ = tax.AddTargetType(&TargetTypeDefinition{
					ID:       "target.web.http_api",
					Type:     "http_api",
					Name:     "HTTP API",
					Category: "web",
				})
				return tax
			},
			target: &TargetTypeDefinition{
				ID:       "target.web.http_api_v2",
				Type:     "http_api",
				Name:     "HTTP API V2",
				Category: "web",
			},
			wantErr: true,
			errType: ErrorTypeDuplicateDefinition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := tt.setup()
			err := taxonomy.AddTargetType(tt.target)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddTargetType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("AddTargetType() error type = %T, want *TaxonomyError", err)
					return
				}
				if taxErr.Type != tt.errType {
					t.Errorf("AddTargetType() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}

			if !tt.wantErr {
				// Verify the target type was added
				retrieved, ok := taxonomy.GetTargetType(tt.target.Type)
				if !ok {
					t.Error("AddTargetType() target type not found after adding")
					return
				}
				if retrieved.ID != tt.target.ID {
					t.Errorf("AddTargetType() ID = %v, want %v", retrieved.ID, tt.target.ID)
				}
			}
		})
	}
}

// TestTaxonomy_GetTargetType tests retrieving target types.
func TestTaxonomy_GetTargetType(t *testing.T) {
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
			def, ok := taxonomy.GetTargetType(tt.typeName)

			if ok != tt.wantOk {
				t.Errorf("GetTargetType() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk && def == nil {
				t.Error("GetTargetType() returned nil definition for existing type")
			}
		})
	}
}

// TestTaxonomy_AddTechniqueType tests adding technique types to taxonomy.
func TestTaxonomy_AddTechniqueType(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *Taxonomy
		technique *TechniqueTypeDefinition
		wantErr   bool
		errType   ErrorType
	}{
		{
			name: "add valid technique type",
			setup: func() *Taxonomy {
				return NewTaxonomy("0.1.0")
			},
			technique: &TechniqueTypeDefinition{
				ID:              "technique.initial_access.ssrf",
				Type:            "ssrf",
				Name:            "Server-Side Request Forgery",
				Category:        "initial_access",
				MITREIDs:        []string{"T1190"},
				Description:     "SSRF vulnerability",
				DefaultSeverity: "high",
			},
			wantErr: false,
		},
		{
			name: "add duplicate technique type by ID",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")
				_ = tax.AddTechniqueType(&TechniqueTypeDefinition{
					ID:       "technique.initial_access.ssrf",
					Type:     "ssrf",
					Name:     "SSRF",
					Category: "initial_access",
				})
				return tax
			},
			technique: &TechniqueTypeDefinition{
				ID:       "technique.initial_access.ssrf",
				Type:     "ssrf_v2",
				Name:     "SSRF V2",
				Category: "initial_access",
			},
			wantErr: true,
			errType: ErrorTypeDuplicateDefinition,
		},
		{
			name: "add duplicate technique type by Type",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")
				_ = tax.AddTechniqueType(&TechniqueTypeDefinition{
					ID:       "technique.initial_access.ssrf",
					Type:     "ssrf",
					Name:     "SSRF",
					Category: "initial_access",
				})
				return tax
			},
			technique: &TechniqueTypeDefinition{
				ID:       "technique.initial_access.ssrf_v2",
				Type:     "ssrf",
				Name:     "SSRF V2",
				Category: "initial_access",
			},
			wantErr: true,
			errType: ErrorTypeDuplicateDefinition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := tt.setup()
			err := taxonomy.AddTechniqueType(tt.technique)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddTechniqueType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("AddTechniqueType() error type = %T, want *TaxonomyError", err)
					return
				}
				if taxErr.Type != tt.errType {
					t.Errorf("AddTechniqueType() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}

			if !tt.wantErr {
				// Verify the technique type was added
				retrieved, ok := taxonomy.GetTechniqueType(tt.technique.Type)
				if !ok {
					t.Error("AddTechniqueType() technique type not found after adding")
					return
				}
				if retrieved.ID != tt.technique.ID {
					t.Errorf("AddTechniqueType() ID = %v, want %v", retrieved.ID, tt.technique.ID)
				}
			}
		})
	}
}

// TestTaxonomy_GetTechniqueType tests retrieving technique types.
func TestTaxonomy_GetTechniqueType(t *testing.T) {
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
			def, ok := taxonomy.GetTechniqueType(tt.typeName)

			if ok != tt.wantOk {
				t.Errorf("GetTechniqueType() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk && def == nil {
				t.Error("GetTechniqueType() returned nil definition for existing type")
			}
		})
	}
}

// TestTaxonomy_AddCapability tests adding capabilities to taxonomy.
func TestTaxonomy_AddCapability(t *testing.T) {
	tests := []struct {
		name       string
		setup      func() *Taxonomy
		capability *CapabilityDefinition
		wantErr    bool
		errType    ErrorType
	}{
		{
			name: "add valid capability",
			setup: func() *Taxonomy {
				return NewTaxonomy("0.1.0")
			},
			capability: &CapabilityDefinition{
				ID:             "capability.web_vulnerability_scanning",
				Name:           "Web Vulnerability Scanning",
				Description:    "Comprehensive web vulnerability scanning",
				TechniqueTypes: []string{"ssrf", "sqli", "xss"},
			},
			wantErr: false,
		},
		{
			name: "add duplicate capability by ID",
			setup: func() *Taxonomy {
				tax := NewTaxonomy("0.1.0")
				_ = tax.AddCapability(&CapabilityDefinition{
					ID:          "capability.web_vulnerability_scanning",
					Name:        "Web Scanning",
					Description: "Web scanning capability",
				})
				return tax
			},
			capability: &CapabilityDefinition{
				ID:          "capability.web_vulnerability_scanning",
				Name:        "Web Scanning V2",
				Description: "Updated web scanning",
			},
			wantErr: true,
			errType: ErrorTypeDuplicateDefinition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taxonomy := tt.setup()
			err := taxonomy.AddCapability(tt.capability)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddCapability() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				taxErr, ok := err.(*TaxonomyError)
				if !ok {
					t.Errorf("AddCapability() error type = %T, want *TaxonomyError", err)
					return
				}
				if taxErr.Type != tt.errType {
					t.Errorf("AddCapability() error type = %v, want %v", taxErr.Type, tt.errType)
				}
			}

			if !tt.wantErr {
				// Verify the capability was added
				retrieved, ok := taxonomy.GetCapability(tt.capability.ID)
				if !ok {
					t.Error("AddCapability() capability not found after adding")
					return
				}
				if retrieved.Name != tt.capability.Name {
					t.Errorf("AddCapability() Name = %v, want %v", retrieved.Name, tt.capability.Name)
				}
			}
		})
	}
}

// TestTaxonomy_GetCapability tests retrieving capabilities.
func TestTaxonomy_GetCapability(t *testing.T) {
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
			def, ok := taxonomy.GetCapability(tt.id)

			if ok != tt.wantOk {
				t.Errorf("GetCapability() ok = %v, want %v", ok, tt.wantOk)
			}

			if tt.wantOk && def == nil {
				t.Error("GetCapability() returned nil definition for existing capability")
			}
		})
	}
}
