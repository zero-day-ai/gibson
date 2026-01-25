package parser

import (
	"fmt"

	"github.com/zero-day-ai/gibson/cmd/taxonomy-gen/schema"
)

// Merge combines a base taxonomy with an extension.
// Extensions can only add new types, not override existing ones.
func Merge(base, extension *schema.Taxonomy) (*schema.Taxonomy, error) {
	// Validate extension references correct base version
	if extension.Kind != "extension" {
		return nil, fmt.Errorf("cannot merge: second taxonomy is not an extension (kind=%s)", extension.Kind)
	}
	if base.Kind != "core" {
		return nil, fmt.Errorf("cannot merge: base taxonomy is not core (kind=%s)", base.Kind)
	}
	if extension.Extends != base.Version {
		return nil, fmt.Errorf("extension version mismatch: extension extends %s but base is %s", extension.Extends, base.Version)
	}

	// Create merged taxonomy
	merged := &schema.Taxonomy{
		Version:           base.Version,
		Kind:              "core", // Merged result is treated as core
		NodeTypes:         make([]schema.NodeType, 0, len(base.NodeTypes)+len(extension.NodeTypes)),
		RelationshipTypes: make([]schema.RelationshipType, 0, len(base.RelationshipTypes)+len(extension.RelationshipTypes)),
		Techniques:        make([]schema.Technique, 0, len(base.Techniques)+len(extension.Techniques)),
	}

	// Build index of base types for conflict detection
	baseNodeTypes := make(map[string]bool)
	for _, nt := range base.NodeTypes {
		baseNodeTypes[nt.Name] = true
	}

	baseRelTypes := make(map[string]bool)
	for _, rt := range base.RelationshipTypes {
		baseRelTypes[rt.Name] = true
	}

	baseTechniques := make(map[string]bool)
	for _, t := range base.Techniques {
		baseTechniques[t.ID] = true
	}

	// Copy base types
	merged.NodeTypes = append(merged.NodeTypes, base.NodeTypes...)
	merged.RelationshipTypes = append(merged.RelationshipTypes, base.RelationshipTypes...)
	merged.Techniques = append(merged.Techniques, base.Techniques...)

	// Add extension node types (check for conflicts)
	for _, nt := range extension.NodeTypes {
		if baseNodeTypes[nt.Name] {
			return nil, &MergeError{
				Field:   "node_types",
				Name:    nt.Name,
				Message: "extension cannot override core node type",
			}
		}
		merged.NodeTypes = append(merged.NodeTypes, nt)
	}

	// Add extension relationship types (check for conflicts)
	for _, rt := range extension.RelationshipTypes {
		if baseRelTypes[rt.Name] {
			return nil, &MergeError{
				Field:   "relationship_types",
				Name:    rt.Name,
				Message: "extension cannot override core relationship type",
			}
		}
		merged.RelationshipTypes = append(merged.RelationshipTypes, rt)
	}

	// Add extension techniques (check for conflicts)
	for _, t := range extension.Techniques {
		if baseTechniques[t.ID] {
			return nil, &MergeError{
				Field:   "techniques",
				Name:    t.ID,
				Message: "extension cannot override core technique",
			}
		}
		merged.Techniques = append(merged.Techniques, t)
	}

	// Validate parent references in extension types
	for _, nt := range extension.NodeTypes {
		if nt.Parent != nil {
			// Check parent type exists (in base or extension)
			parentExists := baseNodeTypes[nt.Parent.Type]
			if !parentExists {
				for _, extNt := range extension.NodeTypes {
					if extNt.Name == nt.Parent.Type {
						parentExists = true
						break
					}
				}
			}
			if !parentExists {
				return nil, &MergeError{
					Field:   "node_types",
					Name:    nt.Name,
					Message: fmt.Sprintf("parent type '%s' does not exist", nt.Parent.Type),
				}
			}
		}
	}

	return merged, nil
}

// MergeError represents an error during taxonomy merge.
type MergeError struct {
	Field   string // node_types, relationship_types, techniques
	Name    string // Name of conflicting element
	Message string
}

func (e *MergeError) Error() string {
	return fmt.Sprintf("merge error in %s[%s]: %s", e.Field, e.Name, e.Message)
}
