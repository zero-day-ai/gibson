package taxonomy

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

// TaxonomyValidator provides validation functionality for taxonomy definitions.
type TaxonomyValidator interface {
	// Validate checks all taxonomy definitions for consistency and correctness.
	Validate(t *Taxonomy) error

	// ValidateNode checks a single node type definition.
	ValidateNode(n *NodeTypeDefinition) error

	// ValidateRelationship checks a single relationship definition.
	ValidateRelationship(r *RelationshipTypeDefinition, nodeTypes map[string]bool) error
}

// taxonomyValidator is the default implementation of TaxonomyValidator.
type taxonomyValidator struct{}

// NewTaxonomyValidator creates a new TaxonomyValidator.
func NewTaxonomyValidator() TaxonomyValidator {
	return &taxonomyValidator{}
}

// Validate performs comprehensive validation on the entire taxonomy.
func (v *taxonomyValidator) Validate(t *Taxonomy) error {
	// Validate all node types
	for _, nodeDef := range t.NodeTypes {
		if err := v.ValidateNode(nodeDef); err != nil {
			return fmt.Errorf("invalid node type %s: %w", nodeDef.Type, err)
		}
	}

	// Build set of valid node types for relationship validation
	nodeTypeSet := make(map[string]bool)
	for typeName := range t.NodeTypes {
		nodeTypeSet[typeName] = true
	}

	// Validate all relationships
	for _, relDef := range t.Relationships {
		if err := v.ValidateRelationship(relDef, nodeTypeSet); err != nil {
			return fmt.Errorf("invalid relationship type %s: %w", relDef.Type, err)
		}
	}

	// Validate techniques
	for _, techDef := range t.Techniques {
		if err := v.validateTechnique(techDef); err != nil {
			return fmt.Errorf("invalid technique %s: %w", techDef.TechniqueID, err)
		}
	}

	// Validate target types
	for _, targetDef := range t.TargetTypes {
		if err := v.validateTargetType(targetDef); err != nil {
			return fmt.Errorf("invalid target type %s: %w", targetDef.Type, err)
		}
	}

	// Validate technique types
	for _, techniqueDef := range t.TechniqueTypes {
		if err := v.validateTechniqueType(techniqueDef); err != nil {
			return fmt.Errorf("invalid technique type %s: %w", techniqueDef.Type, err)
		}
	}

	// Validate capabilities
	for _, capDef := range t.Capabilities {
		if err := v.validateCapability(capDef, t.TechniqueTypes); err != nil {
			return fmt.Errorf("invalid capability %s: %w", capDef.ID, err)
		}
	}

	return nil
}

// ValidateNode validates a single node type definition.
func (v *taxonomyValidator) ValidateNode(n *NodeTypeDefinition) error {
	// Check required fields
	if n.ID == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "node type ID is required",
			Field:   "id",
		}
	}

	if n.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "node type name is required",
			Field:   "name",
		}
	}

	if n.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "node type 'type' field is required",
			Field:   "type",
		}
	}

	if n.Category == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "node type category is required",
			Field:   "category",
		}
	}

	if n.IDTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "node type id_template is required",
			Field:   "id_template",
		}
	}

	// Validate ID template placeholders reference defined properties
	if err := v.validateIDTemplate(n.IDTemplate, n.Properties); err != nil {
		return err
	}

	// Validate properties
	for i, prop := range n.Properties {
		if err := v.validateProperty(&prop, fmt.Sprintf("properties[%d]", i)); err != nil {
			return err
		}
	}

	return nil
}

// ValidateRelationship validates a single relationship type definition.
func (v *taxonomyValidator) ValidateRelationship(r *RelationshipTypeDefinition, nodeTypes map[string]bool) error {
	// Check required fields
	if r.ID == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "relationship ID is required",
			Field:   "id",
		}
	}

	if r.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "relationship name is required",
			Field:   "name",
		}
	}

	if r.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "relationship 'type' field is required",
			Field:   "type",
		}
	}

	if len(r.FromTypes) == 0 {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "relationship must have at least one from_type",
			Field:   "from_types",
		}
	}

	if len(r.ToTypes) == 0 {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "relationship must have at least one to_type",
			Field:   "to_types",
		}
	}

	// Validate from_types reference valid node types
	for _, fromType := range r.FromTypes {
		if !nodeTypes[fromType] {
			return &TaxonomyError{
				Type:    ErrorTypeInvalidReference,
				Message: "from_type references unknown node type",
				Field:   "from_types",
				Value:   fromType,
			}
		}
	}

	// Validate to_types reference valid node types
	for _, toType := range r.ToTypes {
		if !nodeTypes[toType] {
			return &TaxonomyError{
				Type:    ErrorTypeInvalidReference,
				Message: "to_type references unknown node type",
				Field:   "to_types",
				Value:   toType,
			}
		}
	}

	// Validate properties
	for i, prop := range r.Properties {
		if err := v.validateProperty(&prop, fmt.Sprintf("properties[%d]", i)); err != nil {
			return err
		}
	}

	return nil
}

// validateTechnique validates a technique definition.
func (v *taxonomyValidator) validateTechnique(t *TechniqueDefinition) error {
	if t.TechniqueID == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique_id is required",
			Field:   "technique_id",
		}
	}

	if t.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique name is required",
			Field:   "name",
		}
	}

	if t.Taxonomy == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique taxonomy is required",
			Field:   "taxonomy",
		}
	}

	// Validate taxonomy is one of the known values
	validTaxonomies := map[string]bool{"mitre": true, "arcanum": true, "custom": true}
	if !validTaxonomies[t.Taxonomy] {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidFormat,
			Message: "technique taxonomy must be 'mitre', 'arcanum', or 'custom'",
			Field:   "taxonomy",
			Value:   t.Taxonomy,
		}
	}

	return nil
}

// validateProperty validates a property definition.
func (v *taxonomyValidator) validateProperty(p *PropertyDefinition, context string) error {
	if p.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: fmt.Sprintf("%s: property name is required", context),
			Field:   "name",
		}
	}

	if p.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: fmt.Sprintf("%s: property type is required", context),
			Field:   "type",
		}
	}

	// Validate property type is a recognized Go type
	validTypes := map[string]bool{
		"string":         true,
		"int":            true,
		"float64":        true,
		"bool":           true,
		"[]string":       true,
		"map[string]any": true,
	}

	if !validTypes[p.Type] {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidProperty,
			Message: fmt.Sprintf("%s: invalid property type (must be string, int, float64, bool, []string, or map[string]any)", context),
			Field:   "type",
			Value:   p.Type,
		}
	}

	return nil
}

// validateIDTemplate validates that an ID template's placeholders reference defined properties.
// validateIDTemplate validates that an ID template's placeholders reference defined properties.
func (v *taxonomyValidator) validateIDTemplate(template string, properties []PropertyDefinition) error {
	// Extract placeholders from template (format: {property_name})
	placeholderRegex := regexp.MustCompile(`\{([^}]+)\}`)
	matches := placeholderRegex.FindAllStringSubmatch(template, -1)

	// Build set of property names
	propertySet := make(map[string]bool)
	for _, prop := range properties {
		propertySet[prop.Name] = true
	}

	// Check each placeholder references a defined property
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		placeholder := match[1]

		// Handle special functions like sha256(), uuid, etc.
		if strings.Contains(placeholder, "(") || placeholder == "uuid" || placeholder == "timestamp" {
			// This is a function call or special keyword, skip validation
			continue
		}

		// Check if placeholder references a defined property
		if !propertySet[placeholder] {
			return &TaxonomyError{
				Type:    ErrorTypeInvalidReference,
				Message: "ID template placeholder references undefined property",
				Field:   "id_template",
				Value:   placeholder,
			}
		}
	}

	return nil
}

// validateTargetType validates a target type definition.
func (v *taxonomyValidator) validateTargetType(t *TargetTypeDefinition) error {
	// Check required fields
	if t.ID == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "target type ID is required",
			Field:   "id",
		}
	}

	if t.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "target type 'type' field is required",
			Field:   "type",
		}
	}

	if t.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "target type name is required",
			Field:   "name",
		}
	}

	if t.Category == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "target type category is required",
			Field:   "category",
		}
	}

	// Validate category is one of the allowed values
	validCategories := map[string]bool{
		"web":         true,
		"ai":          true,
		"infrastructure": true,
		"cloud":       true,
		"blockchain":  true,
	}
	if !validCategories[t.Category] {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidFormat,
			Message: "target type category must be one of: web, ai, infrastructure, cloud, blockchain",
			Field:   "category",
			Value:   t.Category,
		}
	}

	return nil
}

// validateTechniqueType validates a technique type definition.
func (v *taxonomyValidator) validateTechniqueType(t *TechniqueTypeDefinition) error {
	// Check required fields
	if t.ID == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique type ID is required",
			Field:   "id",
		}
	}

	if t.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique type 'type' field is required",
			Field:   "type",
		}
	}

	if t.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique type name is required",
			Field:   "name",
		}
	}

	if t.Category == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "technique type category is required",
			Field:   "category",
		}
	}

	// Validate category is a valid MITRE tactic
	validCategories := map[string]bool{
		"initial_access":     true,
		"execution":          true,
		"persistence":        true,
		"privilege_escalation": true,
		"defense_evasion":    true,
		"credential_access":  true,
		"discovery":          true,
		"lateral_movement":   true,
		"collection":         true,
		"exfiltration":       true,
		"impact":             true,
	}
	if !validCategories[t.Category] {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidFormat,
			Message: "technique type category must be a valid MITRE tactic (initial_access, execution, persistence, privilege_escalation, defense_evasion, credential_access, discovery, lateral_movement, collection, exfiltration, impact)",
			Field:   "category",
			Value:   t.Category,
		}
	}

	// Validate default severity if present
	if t.DefaultSeverity != "" {
		validSeverities := map[string]bool{
			"critical": true,
			"high":     true,
			"medium":   true,
			"low":      true,
			"info":     true,
		}
		if !validSeverities[t.DefaultSeverity] {
			return &TaxonomyError{
				Type:    ErrorTypeInvalidFormat,
				Message: "technique type default_severity must be one of: critical, high, medium, low, info",
				Field:   "default_severity",
				Value:   t.DefaultSeverity,
			}
		}
	}

	return nil
}

// validateCapability validates a capability definition.
func (v *taxonomyValidator) validateCapability(c *CapabilityDefinition, techniqueTypes map[string]*TechniqueTypeDefinition) error {
	// Check required fields
	if c.ID == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "capability ID is required",
			Field:   "id",
		}
	}

	if c.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "capability name is required",
			Field:   "name",
		}
	}

	// Validate technique_types references - warn if technique doesn't exist
	for _, techType := range c.TechniqueTypes {
		if _, exists := techniqueTypes[techType]; !exists {
			// Log warning but don't fail validation
			log.Printf("WARNING: Capability '%s' references non-existent technique type '%s'", c.ID, techType)
		}
	}

	return nil
}

// ValidateTaxonomy is a convenience function to validate a taxonomy.
func ValidateTaxonomy(t *Taxonomy) error {
	validator := NewTaxonomyValidator()
	return validator.Validate(t)
}
