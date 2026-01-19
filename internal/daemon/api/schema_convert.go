package api

import (
	"encoding/json"

	"github.com/zero-day-ai/sdk/schema"
)

// SchemaToProto converts SDK schema.JSON to proto JSONSchemaNode.
// This preserves taxonomy mappings and all schema properties during gRPC transport.
func SchemaToProto(s schema.JSON) *JSONSchemaNode {
	// Return nil for empty schemas
	if s.Type == "" && len(s.Properties) == 0 && s.Items == nil {
		return nil
	}

	node := &JSONSchemaNode{
		Type:        s.Type,
		Description: s.Description,
		Required:    s.Required,
		Pattern:     s.Pattern,
		Format:      s.Format,
		Ref:         s.Ref,
	}

	// Convert properties recursively
	if len(s.Properties) > 0 {
		node.Properties = make(map[string]*JSONSchemaNode)
		for name, prop := range s.Properties {
			node.Properties[name] = SchemaToProto(prop)
		}
	}

	// Convert items recursively
	if s.Items != nil {
		node.Items = SchemaToProto(*s.Items)
	}

	// Convert enum values
	if len(s.Enum) > 0 {
		for _, v := range s.Enum {
			if str, ok := v.(string); ok {
				node.EnumValues = append(node.EnumValues, str)
			}
		}
	}

	// Convert numeric constraints
	if s.Minimum != nil {
		node.Minimum = *s.Minimum
	}
	if s.Maximum != nil {
		node.Maximum = *s.Maximum
	}
	if s.MinLength != nil {
		node.MinLength = int32(*s.MinLength)
	}
	if s.MaxLength != nil {
		node.MaxLength = int32(*s.MaxLength)
	}

	// Convert default value (JSON encode)
	if s.Default != nil {
		if data, err := json.Marshal(s.Default); err == nil {
			node.DefaultValue = string(data)
		}
	}

	// Convert taxonomy (THE KEY FEATURE)
	if s.Taxonomy != nil {
		node.Taxonomy = TaxonomyToProto(*s.Taxonomy)
	}

	return node
}

// TaxonomyToProto converts SDK TaxonomyMapping to proto TaxonomyMapping.
func TaxonomyToProto(t schema.TaxonomyMapping) *TaxonomyMapping {
	proto := &TaxonomyMapping{
		NodeType:              t.NodeType,
		IdentifyingProperties: t.IdentifyingProperties,
	}

	// Convert property mappings
	for _, p := range t.Properties {
		propProto := &PropertyMapping{
			Source:    p.Source,
			Target:    p.Target,
			Transform: p.Transform,
		}
		// Convert default value (JSON encode)
		if p.Default != nil {
			if data, err := json.Marshal(p.Default); err == nil {
				propProto.DefaultValue = string(data)
			}
		}
		proto.Properties = append(proto.Properties, propProto)
	}

	// Convert relationship mappings
	for _, r := range t.Relationships {
		relProto := &RelationshipMapping{
			Type:      r.Type,
			Condition: r.Condition,
			From: &NodeReference{
				Type:       r.From.Type,
				Properties: r.From.Properties,
			},
			To: &NodeReference{
				Type:       r.To.Type,
				Properties: r.To.Properties,
			},
		}
		// Convert relationship properties
		for _, p := range r.Properties {
			relProto.RelProperties = append(relProto.RelProperties, &PropertyMapping{
				Source:    p.Source,
				Target:    p.Target,
				Transform: p.Transform,
			})
		}
		proto.Relationships = append(proto.Relationships, relProto)
	}

	return proto
}

// ProtoToSchema converts proto JSONSchemaNode to SDK schema.JSON.
// This reconstructs the full SDK schema with taxonomy from the proto representation.
func ProtoToSchema(node *JSONSchemaNode) schema.JSON {
	if node == nil {
		return schema.JSON{}
	}

	s := schema.JSON{
		Type:        node.Type,
		Description: node.Description,
		Required:    node.Required,
		Pattern:     node.Pattern,
		Format:      node.Format,
		Ref:         node.Ref,
	}

	// Convert properties recursively
	if len(node.Properties) > 0 {
		s.Properties = make(map[string]schema.JSON)
		for name, prop := range node.Properties {
			s.Properties[name] = ProtoToSchema(prop)
		}
	}

	// Convert items recursively
	if node.Items != nil {
		items := ProtoToSchema(node.Items)
		s.Items = &items
	}

	// Convert enum values
	if len(node.EnumValues) > 0 {
		for _, v := range node.EnumValues {
			s.Enum = append(s.Enum, v)
		}
	}

	// Convert numeric constraints
	if node.Minimum != 0 {
		min := node.Minimum
		s.Minimum = &min
	}
	if node.Maximum != 0 {
		max := node.Maximum
		s.Maximum = &max
	}
	if node.MinLength != 0 {
		minLen := int(node.MinLength)
		s.MinLength = &minLen
	}
	if node.MaxLength != 0 {
		maxLen := int(node.MaxLength)
		s.MaxLength = &maxLen
	}

	// Convert default value (JSON decode)
	if node.DefaultValue != "" {
		var def any
		if err := json.Unmarshal([]byte(node.DefaultValue), &def); err == nil {
			s.Default = def
		}
	}

	// Convert taxonomy (THE KEY FEATURE)
	if node.Taxonomy != nil {
		s.Taxonomy = ProtoToTaxonomy(node.Taxonomy)
	}

	return s
}

// ProtoToTaxonomy converts proto TaxonomyMapping to SDK TaxonomyMapping.
func ProtoToTaxonomy(proto *TaxonomyMapping) *schema.TaxonomyMapping {
	if proto == nil {
		return nil
	}

	t := &schema.TaxonomyMapping{
		NodeType:              proto.NodeType,
		IdentifyingProperties: proto.IdentifyingProperties,
	}

	// Convert property mappings
	for _, p := range proto.Properties {
		prop := schema.PropertyMapping{
			Source:    p.Source,
			Target:    p.Target,
			Transform: p.Transform,
		}
		// Convert default value (JSON decode)
		if p.DefaultValue != "" {
			var def any
			if err := json.Unmarshal([]byte(p.DefaultValue), &def); err == nil {
				prop.Default = def
			}
		}
		t.Properties = append(t.Properties, prop)
	}

	// Convert relationship mappings
	for _, r := range proto.Relationships {
		rel := schema.RelationshipMapping{
			Type:      r.Type,
			Condition: r.Condition,
		}

		// Convert From NodeReference
		if r.From != nil {
			rel.From = schema.NodeReference{
				Type:       r.From.Type,
				Properties: r.From.Properties,
			}
		}

		// Convert To NodeReference
		if r.To != nil {
			rel.To = schema.NodeReference{
				Type:       r.To.Type,
				Properties: r.To.Properties,
			}
		}

		// Convert relationship properties
		for _, p := range r.RelProperties {
			rel.Properties = append(rel.Properties, schema.PropertyMapping{
				Source:    p.Source,
				Target:    p.Target,
				Transform: p.Transform,
			})
		}
		t.Relationships = append(t.Relationships, rel)
	}

	return t
}
