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

	return node
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

	return s
}
