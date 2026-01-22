package component

import "fmt"

// TaxonomyExtension defines custom GraphRAG taxonomy extensions for a component.
// Allows agents to declare custom node types and relationships for their domain.
type TaxonomyExtension struct {
	NodeTypes     []NodeTypeDefinition     `json:"node_types,omitempty" yaml:"node_types,omitempty"`         // Custom node type definitions
	Relationships []RelationshipDefinition `json:"relationships,omitempty" yaml:"relationships,omitempty"`   // Custom relationship definitions
}

// NodeTypeDefinition defines a custom node type for GraphRAG.
type NodeTypeDefinition struct {
	Name       string         `json:"name" yaml:"name"`                                 // Node type name (e.g., CloudFunction)
	Category   string         `json:"category" yaml:"category"`                         // Category (Asset, Finding, Technique, etc.)
	Properties []PropertyInfo `json:"properties,omitempty" yaml:"properties,omitempty"` // Node properties
}

// PropertyInfo defines a property for a custom node type.
type PropertyInfo struct {
	Name string `json:"name" yaml:"name"` // Property name
	Type string `json:"type" yaml:"type"` // Property type (string, int, bool, etc.)
}

// RelationshipDefinition defines a custom relationship type for GraphRAG.
type RelationshipDefinition struct {
	Name      string   `json:"name" yaml:"name"`             // Relationship name (e.g., INVOKES)
	FromTypes []string `json:"from_types" yaml:"from_types"` // Source node types
	ToTypes   []string `json:"to_types" yaml:"to_types"`     // Target node types
}

// Validate validates the TaxonomyExtension fields.
func (t *TaxonomyExtension) Validate() error {
	if t == nil {
		return nil
	}

	// Validate node types
	for i, nodeType := range t.NodeTypes {
		if err := nodeType.Validate(); err != nil {
			return fmt.Errorf("node type %d validation failed: %w", i, err)
		}
	}

	// Validate relationships
	for i, rel := range t.Relationships {
		if err := rel.Validate(); err != nil {
			return fmt.Errorf("relationship %d validation failed: %w", i, err)
		}
	}

	return nil
}

// Validate validates the NodeTypeDefinition fields.
func (n *NodeTypeDefinition) Validate() error {
	// Validate name
	if n.Name == "" {
		return fmt.Errorf("node type name is required")
	}

	// Validate category
	if n.Category == "" {
		return fmt.Errorf("node type category is required for %s", n.Name)
	}

	// Validate properties
	for i, prop := range n.Properties {
		if err := prop.Validate(); err != nil {
			return fmt.Errorf("property %d of node type %s validation failed: %w", i, n.Name, err)
		}
	}

	return nil
}

// Validate validates the PropertyInfo fields.
func (p *PropertyInfo) Validate() error {
	// Validate name
	if p.Name == "" {
		return fmt.Errorf("property name is required")
	}

	// Validate type
	if p.Type == "" {
		return fmt.Errorf("property type is required for %s", p.Name)
	}

	return nil
}

// Validate validates the RelationshipDefinition fields.
func (r *RelationshipDefinition) Validate() error {
	// Validate name
	if r.Name == "" {
		return fmt.Errorf("relationship name is required")
	}

	// Validate from_types
	if len(r.FromTypes) == 0 {
		return fmt.Errorf("relationship %s must have at least one from_type", r.Name)
	}

	// Validate to_types
	if len(r.ToTypes) == 0 {
		return fmt.Errorf("relationship %s must have at least one to_type", r.Name)
	}

	return nil
}
