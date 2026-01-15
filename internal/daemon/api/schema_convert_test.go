package api

import (
	"testing"

	"github.com/zero-day-ai/sdk/schema"
)

func TestSchemaToProto_Simple(t *testing.T) {
	// Test simple string schema
	sdkSchema := schema.String()
	protoNode := SchemaToProto(sdkSchema)

	if protoNode == nil {
		t.Fatal("Expected non-nil proto node")
	}
	if protoNode.Type != "string" {
		t.Errorf("Expected type 'string', got %q", protoNode.Type)
	}
}

func TestSchemaToProto_WithTaxonomy(t *testing.T) {
	// Create a schema with taxonomy (like nmap's host schema)
	sdkSchema := schema.Object(map[string]schema.JSON{
		"ip":       schema.String(),
		"hostname": schema.String(),
		"ports":    schema.Array(schema.Int()),
	}, "ip").WithTaxonomy(schema.TaxonomyMapping{
		NodeType:   "host",
		IDTemplate: "host:{.ip}",
		Properties: []schema.PropertyMapping{
			{Source: "ip", Target: "ip_address"},
			{Source: "hostname", Target: "hostname"},
		},
		Relationships: []schema.RelationshipMapping{
			{
				Type:         "HAS_PORT",
				FromTemplate: "host:{.ip}",
				ToTemplate:   "port:{.ports[*]}",
			},
		},
	})

	protoNode := SchemaToProto(sdkSchema)

	if protoNode == nil {
		t.Fatal("Expected non-nil proto node")
	}
	if protoNode.Type != "object" {
		t.Errorf("Expected type 'object', got %q", protoNode.Type)
	}
	if len(protoNode.Properties) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(protoNode.Properties))
	}
	if len(protoNode.Required) != 1 || protoNode.Required[0] != "ip" {
		t.Errorf("Expected required=['ip'], got %v", protoNode.Required)
	}

	// Verify taxonomy was converted
	if protoNode.Taxonomy == nil {
		t.Fatal("Expected taxonomy to be preserved")
	}
	if protoNode.Taxonomy.NodeType != "host" {
		t.Errorf("Expected NodeType 'host', got %q", protoNode.Taxonomy.NodeType)
	}
	if protoNode.Taxonomy.IdTemplate != "host:{.ip}" {
		t.Errorf("Expected IdTemplate 'host:{.ip}', got %q", protoNode.Taxonomy.IdTemplate)
	}
	if len(protoNode.Taxonomy.Properties) != 2 {
		t.Errorf("Expected 2 property mappings, got %d", len(protoNode.Taxonomy.Properties))
	}
	if len(protoNode.Taxonomy.Relationships) != 1 {
		t.Errorf("Expected 1 relationship, got %d", len(protoNode.Taxonomy.Relationships))
	}
}

func TestProtoToSchema_RoundTrip(t *testing.T) {
	// Create a complex SDK schema with taxonomy
	original := schema.Object(map[string]schema.JSON{
		"ip":       schema.StringWithDesc("Host IP address"),
		"hostname": schema.String(),
		"os":       schema.String(),
		"ports": schema.Array(schema.Object(map[string]schema.JSON{
			"port":     schema.Int(),
			"protocol": schema.String(),
			"state":    schema.String(),
		})),
	}, "ip").WithTaxonomy(schema.TaxonomyMapping{
		NodeType:   "host",
		IDTemplate: "host:{.ip}",
		Properties: []schema.PropertyMapping{
			{Source: "ip", Target: "ip_address"},
			{Source: "hostname", Target: "hostname"},
			{Source: "os", Target: "operating_system", Default: "unknown"},
		},
		Relationships: []schema.RelationshipMapping{
			{
				Type:         "HAS_PORT",
				FromTemplate: "host:{.ip}",
				ToTemplate:   "port:{.ports[*].port}",
				Condition:    "{{.ports[*].state}} == 'open'",
			},
		},
	})

	// Convert to proto and back
	protoNode := SchemaToProto(original)
	reconstructed := ProtoToSchema(protoNode)

	// Verify basic structure
	if reconstructed.Type != original.Type {
		t.Errorf("Type mismatch: expected %q, got %q", original.Type, reconstructed.Type)
	}
	if len(reconstructed.Properties) != len(original.Properties) {
		t.Errorf("Properties count mismatch: expected %d, got %d", len(original.Properties), len(reconstructed.Properties))
	}
	if len(reconstructed.Required) != len(original.Required) {
		t.Errorf("Required count mismatch: expected %d, got %d", len(original.Required), len(reconstructed.Required))
	}

	// Verify taxonomy was preserved
	if reconstructed.Taxonomy == nil {
		t.Fatal("Taxonomy was lost during round-trip conversion")
	}
	if reconstructed.Taxonomy.NodeType != original.Taxonomy.NodeType {
		t.Errorf("NodeType mismatch: expected %q, got %q", original.Taxonomy.NodeType, reconstructed.Taxonomy.NodeType)
	}
	if reconstructed.Taxonomy.IDTemplate != original.Taxonomy.IDTemplate {
		t.Errorf("IDTemplate mismatch: expected %q, got %q", original.Taxonomy.IDTemplate, reconstructed.Taxonomy.IDTemplate)
	}
	if len(reconstructed.Taxonomy.Properties) != len(original.Taxonomy.Properties) {
		t.Errorf("Property mappings count mismatch: expected %d, got %d",
			len(original.Taxonomy.Properties), len(reconstructed.Taxonomy.Properties))
	}
	if len(reconstructed.Taxonomy.Relationships) != len(original.Taxonomy.Relationships) {
		t.Errorf("Relationships count mismatch: expected %d, got %d",
			len(original.Taxonomy.Relationships), len(reconstructed.Taxonomy.Relationships))
	}
}

func TestSchemaToProto_NoTaxonomy(t *testing.T) {
	// Test that schemas without taxonomy still work
	sdkSchema := schema.Object(map[string]schema.JSON{
		"name": schema.String(),
		"age":  schema.Int(),
	})

	protoNode := SchemaToProto(sdkSchema)

	if protoNode == nil {
		t.Fatal("Expected non-nil proto node")
	}
	if protoNode.Taxonomy != nil {
		t.Error("Expected nil taxonomy for schema without taxonomy")
	}
}

func TestSchemaToProto_EmptySchema(t *testing.T) {
	// Test empty schema
	sdkSchema := schema.JSON{}
	protoNode := SchemaToProto(sdkSchema)

	if protoNode != nil {
		t.Error("Expected nil proto node for empty schema")
	}
}

func TestTaxonomyToProto_PropertyMappings(t *testing.T) {
	taxonomy := schema.TaxonomyMapping{
		NodeType:   "test",
		IDTemplate: "test:{.id}",
		Properties: []schema.PropertyMapping{
			{Source: "src1", Target: "tgt1"},
			{Source: "src2", Target: "tgt2", Default: "default_value"},
			{Source: "src3", Target: "tgt3", Transform: "lowercase"},
		},
	}

	proto := TaxonomyToProto(taxonomy)

	if proto == nil {
		t.Fatal("Expected non-nil proto taxonomy")
	}
	if len(proto.Properties) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(proto.Properties))
	}

	// Verify first property
	if proto.Properties[0].Source != "src1" || proto.Properties[0].Target != "tgt1" {
		t.Errorf("Property 0 mismatch")
	}

	// Verify property with default
	if proto.Properties[1].DefaultValue == "" {
		t.Error("Expected default value to be encoded")
	}

	// Verify property with transform
	if proto.Properties[2].Transform != "lowercase" {
		t.Errorf("Expected transform 'lowercase', got %q", proto.Properties[2].Transform)
	}
}

func TestTaxonomyToProto_RelationshipMappings(t *testing.T) {
	taxonomy := schema.TaxonomyMapping{
		NodeType:   "test",
		IDTemplate: "test:{.id}",
		Relationships: []schema.RelationshipMapping{
			{
				Type:         "RELATES_TO",
				FromTemplate: "test:{.id}",
				ToTemplate:   "other:{.other_id}",
			},
			{
				Type:         "CONDITIONAL_REL",
				FromTemplate: "test:{.id}",
				ToTemplate:   "target:{.target_id}",
				Condition:    "{{.active}} == true",
				Properties: []schema.PropertyMapping{
					{Source: "weight", Target: "weight"},
				},
			},
		},
	}

	proto := TaxonomyToProto(taxonomy)

	if proto == nil {
		t.Fatal("Expected non-nil proto taxonomy")
	}
	if len(proto.Relationships) != 2 {
		t.Errorf("Expected 2 relationships, got %d", len(proto.Relationships))
	}

	// Verify first relationship
	rel0 := proto.Relationships[0]
	if rel0.Type != "RELATES_TO" {
		t.Errorf("Relationship 0 type mismatch")
	}

	// Verify conditional relationship
	rel1 := proto.Relationships[1]
	if rel1.Condition != "{{.active}} == true" {
		t.Errorf("Expected condition '{{.active}} == true', got %q", rel1.Condition)
	}
	if len(rel1.Properties) != 1 {
		t.Errorf("Expected 1 relationship property, got %d", len(rel1.Properties))
	}
}

func TestProtoToTaxonomy_Nil(t *testing.T) {
	result := ProtoToTaxonomy(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestSchemaToProto_NestedObjects(t *testing.T) {
	// Test nested object schema with taxonomy
	sdkSchema := schema.Object(map[string]schema.JSON{
		"metadata": schema.Object(map[string]schema.JSON{
			"id":   schema.String(),
			"name": schema.String(),
		}),
		"items": schema.Array(schema.Object(map[string]schema.JSON{
			"value": schema.Int(),
			"label": schema.String(),
		})),
	})

	protoNode := SchemaToProto(sdkSchema)

	if protoNode == nil {
		t.Fatal("Expected non-nil proto node")
	}
	if protoNode.Properties["metadata"] == nil {
		t.Fatal("Expected metadata property")
	}
	if len(protoNode.Properties["metadata"].Properties) != 2 {
		t.Errorf("Expected 2 nested properties in metadata, got %d",
			len(protoNode.Properties["metadata"].Properties))
	}
	if protoNode.Properties["items"] == nil {
		t.Fatal("Expected items property")
	}
	if protoNode.Properties["items"].Items == nil {
		t.Fatal("Expected items to have array items schema")
	}
}

func TestSchemaToProto_Constraints(t *testing.T) {
	// Test numeric and string constraints
	minVal := 0.0
	maxVal := 100.0
	minLen := 5
	maxLen := 50

	sdkSchema := schema.Object(map[string]schema.JSON{
		"score": schema.JSON{
			Type:    "number",
			Minimum: &minVal,
			Maximum: &maxVal,
		},
		"name": schema.JSON{
			Type:      "string",
			MinLength: &minLen,
			MaxLength: &maxLen,
			Pattern:   "^[a-z]+$",
			Format:    "lowercase",
		},
	})

	protoNode := SchemaToProto(sdkSchema)

	if protoNode == nil {
		t.Fatal("Expected non-nil proto node")
	}

	// Check numeric constraints
	scoreNode := protoNode.Properties["score"]
	if scoreNode.Minimum != 0.0 || scoreNode.Maximum != 100.0 {
		t.Errorf("Numeric constraints not preserved: min=%f, max=%f", scoreNode.Minimum, scoreNode.Maximum)
	}

	// Check string constraints
	nameNode := protoNode.Properties["name"]
	if nameNode.MinLength != 5 || nameNode.MaxLength != 50 {
		t.Errorf("String length constraints not preserved: minLen=%d, maxLen=%d", nameNode.MinLength, nameNode.MaxLength)
	}
	if nameNode.Pattern != "^[a-z]+$" {
		t.Errorf("Pattern not preserved: %q", nameNode.Pattern)
	}
	if nameNode.Format != "lowercase" {
		t.Errorf("Format not preserved: %q", nameNode.Format)
	}
}
