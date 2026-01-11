package taxonomy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// TaxonomyRegistry provides O(1) runtime access to taxonomy definitions
// and utility functions like ID generation from templates.
type TaxonomyRegistry interface {
	// Version returns the taxonomy version (tied to Gibson version).
	Version() string

	// NodeTypes returns all node type definitions.
	NodeTypes() []NodeTypeDefinition

	// NodeType returns a specific node type definition by type name.
	NodeType(typeName string) (*NodeTypeDefinition, bool)

	// RelationshipTypes returns all relationship type definitions.
	RelationshipTypes() []RelationshipTypeDefinition

	// RelationshipType returns a specific relationship definition by type name.
	RelationshipType(typeName string) (*RelationshipTypeDefinition, bool)

	// Techniques returns techniques filtered by taxonomy source (mitre, arcanum, custom).
	// If source is empty, returns all techniques.
	Techniques(source string) []TechniqueDefinition

	// Technique returns a specific technique by ID.
	Technique(techniqueID string) (*TechniqueDefinition, bool)

	// IsCanonicalNodeType checks if a node type is in the canonical taxonomy.
	IsCanonicalNodeType(typeName string) bool

	// IsCanonicalRelationType checks if a relationship type is in the canonical taxonomy.
	IsCanonicalRelationType(typeName string) bool

	// ValidateNodeType checks if a node type string is valid.
	// Returns true if valid (always true, but logs warning if not canonical).
	ValidateNodeType(typeName string) bool

	// ValidateRelationType checks if a relationship type string is valid.
	// Returns true if valid (always true, but logs warning if not canonical).
	ValidateRelationType(typeName string) bool

	// GenerateNodeID generates a deterministic ID from a node type's ID template.
	// Properties map should contain values for all template placeholders.
	GenerateNodeID(typeName string, properties map[string]any) (string, error)
}

// taxonomyRegistry is the default implementation of TaxonomyRegistry.
type taxonomyRegistry struct {
	taxonomy *Taxonomy
	mu       sync.RWMutex // Thread-safe for concurrent access
}

// NewTaxonomyRegistry creates a new TaxonomyRegistry from a loaded taxonomy.
func NewTaxonomyRegistry(taxonomy *Taxonomy) (TaxonomyRegistry, error) {
	if taxonomy == nil {
		return nil, fmt.Errorf("taxonomy cannot be nil")
	}

	return &taxonomyRegistry{
		taxonomy: taxonomy,
	}, nil
}

// Version returns the taxonomy version.
func (r *taxonomyRegistry) Version() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	version := r.taxonomy.Version
	if r.taxonomy.IsCustomLoaded() {
		return version + "+custom"
	}
	return version
}

// NodeTypes returns all node type definitions.
func (r *taxonomyRegistry) NodeTypes() []NodeTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]NodeTypeDefinition, 0, len(r.taxonomy.NodeTypes))
	for _, nodeDef := range r.taxonomy.NodeTypes {
		types = append(types, *nodeDef)
	}
	return types
}

// NodeType returns a specific node type definition.
func (r *taxonomyRegistry) NodeType(typeName string) (*NodeTypeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.NodeTypes[typeName]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy, true
}

// RelationshipTypes returns all relationship type definitions.
func (r *taxonomyRegistry) RelationshipTypes() []RelationshipTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]RelationshipTypeDefinition, 0, len(r.taxonomy.Relationships))
	for _, relDef := range r.taxonomy.Relationships {
		types = append(types, *relDef)
	}
	return types
}

// RelationshipType returns a specific relationship definition.
func (r *taxonomyRegistry) RelationshipType(typeName string) (*RelationshipTypeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.Relationships[typeName]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy, true
}

// Techniques returns techniques filtered by taxonomy source.
func (r *taxonomyRegistry) Techniques(source string) []TechniqueDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	techniques := make([]TechniqueDefinition, 0)
	for _, techDef := range r.taxonomy.Techniques {
		if source == "" || techDef.Taxonomy == source {
			techniques = append(techniques, *techDef)
		}
	}
	return techniques
}

// Technique returns a specific technique by ID.
func (r *taxonomyRegistry) Technique(techniqueID string) (*TechniqueDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.Techniques[techniqueID]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy, true
}

// IsCanonicalNodeType checks if a node type is in the canonical taxonomy.
func (r *taxonomyRegistry) IsCanonicalNodeType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.NodeTypes[typeName]
	return ok
}

// IsCanonicalRelationType checks if a relationship type is in the canonical taxonomy.
func (r *taxonomyRegistry) IsCanonicalRelationType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.Relationships[typeName]
	return ok
}

// ValidateNodeType checks if a node type string is valid.
// Returns true if valid (always true, but logs warning if not canonical).
// This method satisfies the SDK's TaxonomyReader interface.
func (r *taxonomyRegistry) ValidateNodeType(typeName string) bool {
	if !r.IsCanonicalNodeType(typeName) {
		log.Printf("WARNING: Node type '%s' is not in the canonical taxonomy", typeName)
		// Still return true - we allow custom types but warn about them
	}
	return true
}

// ValidateRelationType checks if a relationship type string is valid.
// Returns true if valid (always true, but logs warning if not canonical).
// This method satisfies the SDK's TaxonomyReader interface.
func (r *taxonomyRegistry) ValidateRelationType(typeName string) bool {
	if !r.IsCanonicalRelationType(typeName) {
		log.Printf("WARNING: Relationship type '%s' is not in the canonical taxonomy", typeName)
		// Still return true - we allow custom types but warn about them
	}
	return true
}

// GenerateNodeID generates a deterministic ID from a node type's ID template.
func (r *taxonomyRegistry) GenerateNodeID(typeName string, properties map[string]any) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get node type definition
	nodeDef, ok := r.taxonomy.NodeTypes[typeName]
	if !ok {
		return "", fmt.Errorf("unknown node type: %s", typeName)
	}

	// Expand template with property values
	template := nodeDef.IDTemplate
	result := template

	// Find all placeholders in template
	placeholderRegex := regexp.MustCompile(`\{([^}]+)\}`)
	matches := placeholderRegex.FindAllStringSubmatch(template, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		placeholder := match[0] // Full match including braces
		key := match[1]          // Just the key name

		// Handle special functions
		if strings.HasPrefix(key, "uuid") {
			// Generate a UUID
			result = strings.Replace(result, placeholder, uuid.New().String(), 1)
			continue
		}

		if strings.HasPrefix(key, "sha256(") && strings.HasSuffix(key, ")") {
			// Extract the expression inside sha256()
			expr := strings.TrimSuffix(strings.TrimPrefix(key, "sha256("), ")")

			// Build the value to hash from the expression
			hashInput := expr
			for propKey, propVal := range properties {
				hashInput = strings.ReplaceAll(hashInput, propKey, fmt.Sprintf("%v", propVal))
			}

			// Generate SHA256 hash
			hash := sha256.Sum256([]byte(hashInput))
			hashStr := hex.EncodeToString(hash[:])

			// Check if we need to truncate (e.g., [:16])
			if strings.Contains(key, "[:") {
				// Extract truncation length
				parts := strings.Split(key, "[:")
				if len(parts) > 1 {
					lengthStr := strings.TrimSuffix(parts[1], "])")
					var length int
					fmt.Sscanf(lengthStr, "%d", &length)
					if length > 0 && length < len(hashStr) {
						hashStr = hashStr[:length]
					}
				}
			}

			result = strings.Replace(result, placeholder, hashStr, 1)
			continue
		}

		// Regular property substitution
		value, ok := properties[key]
		if !ok {
			return "", fmt.Errorf("missing required property '%s' for node type '%s'", key, typeName)
		}

		// Convert value to string
		valueStr := fmt.Sprintf("%v", value)
		result = strings.Replace(result, placeholder, valueStr, 1)
	}

	return result, nil
}

// LoadAndValidateTaxonomy is a convenience function that loads, validates, and creates a registry.
func LoadAndValidateTaxonomy() (TaxonomyRegistry, error) {
	// Load taxonomy
	taxonomy, err := LoadTaxonomy()
	if err != nil {
		return nil, fmt.Errorf("failed to load taxonomy: %w", err)
	}

	// Validate taxonomy
	if err := ValidateTaxonomy(taxonomy); err != nil {
		return nil, fmt.Errorf("taxonomy validation failed: %w", err)
	}

	// Create registry
	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		return nil, fmt.Errorf("failed to create taxonomy registry: %w", err)
	}

	return registry, nil
}

// LoadAndValidateTaxonomyWithCustom is a convenience function for loading with custom taxonomy.
func LoadAndValidateTaxonomyWithCustom(customPath string) (TaxonomyRegistry, error) {
	// Load taxonomy with custom extensions
	taxonomy, err := LoadTaxonomyWithCustom(customPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load taxonomy with custom: %w", err)
	}

	// Validate taxonomy
	if err := ValidateTaxonomy(taxonomy); err != nil {
		return nil, fmt.Errorf("taxonomy validation failed: %w", err)
	}

	// Create registry
	registry, err := NewTaxonomyRegistry(taxonomy)
	if err != nil {
		return nil, fmt.Errorf("failed to create taxonomy registry: %w", err)
	}

	return registry, nil
}
