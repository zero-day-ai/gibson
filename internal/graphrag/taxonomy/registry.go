package taxonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
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

	// GetTargetType returns a target type definition by its Type field.
	GetTargetType(typeName string) (*TargetTypeDefinition, bool)

	// ListTargetTypes returns all target type definitions.
	ListTargetTypes() []*TargetTypeDefinition

	// ListTargetTypesByCategory returns target types filtered by category.
	ListTargetTypesByCategory(category string) []*TargetTypeDefinition

	// ValidateTargetType checks if a target type string exists.
	ValidateTargetType(typeName string) bool

	// GetTechniqueType returns a technique type definition by its Type field.
	GetTechniqueType(typeName string) (*TechniqueTypeDefinition, bool)

	// ListTechniqueTypes returns all technique type definitions.
	ListTechniqueTypes() []*TechniqueTypeDefinition

	// ListTechniqueTypesByCategory returns technique types filtered by category (MITRE tactic).
	ListTechniqueTypesByCategory(category string) []*TechniqueTypeDefinition

	// ValidateTechniqueType checks if a technique type string exists.
	ValidateTechniqueType(typeName string) bool

	// GetTechniqueTypesByMITRE returns technique types that reference a specific MITRE ATT&CK ID.
	GetTechniqueTypesByMITRE(mitreID string) []*TechniqueTypeDefinition

	// GetCapability returns a capability definition by its ID.
	GetCapability(id string) (*CapabilityDefinition, bool)

	// ListCapabilities returns all capability definitions.
	ListCapabilities() []*CapabilityDefinition

	// GetTechniqueTypesForCapability returns the technique types associated with a capability.
	GetTechniqueTypesForCapability(capability string) []string

	// ValidateCapability checks if a capability ID exists.
	ValidateCapability(id string) bool

	// GetExecutionEvent returns the event definition for the given event type.
	GetExecutionEvent(eventType string) *ExecutionEventDefinition

	// GetToolOutputSchema returns the output schema for the given tool.
	GetToolOutputSchema(toolName string) *ToolOutputSchema

	// ListExecutionEvents returns all registered event types.
	ListExecutionEvents() []string

	// ListToolOutputSchemas returns all registered tool names with schemas.
	ListToolOutputSchemas() []string

	// HasExecutionEvent checks if an event type is defined.
	HasExecutionEvent(eventType string) bool

	// HasToolOutputSchema checks if a tool has an output schema.
	HasToolOutputSchema(toolName string) bool

	// LoadToolSchemasFromBinaries loads taxonomy from tool binaries in the given directory.
	// Tools must embed their taxonomy using schema.TaxonomyMapping in their Go schema files.
	LoadToolSchemasFromBinaries(ctx context.Context, toolsDir string, logger *slog.Logger) error

	// IsSchemaBasedToolSchema returns true if the tool schema was loaded from a tool binary.
	IsSchemaBasedToolSchema(toolName string) bool
}

// taxonomyRegistry is the default implementation of TaxonomyRegistry.
type taxonomyRegistry struct {
	taxonomy *Taxonomy
	mu       sync.RWMutex // Thread-safe for concurrent access

	// schemaBasedTools tracks which tools had their schema loaded from binaries.
	schemaBasedTools map[string]bool
}

// NewTaxonomyRegistry creates a new TaxonomyRegistry from a loaded taxonomy.
func NewTaxonomyRegistry(taxonomy *Taxonomy) (TaxonomyRegistry, error) {
	if taxonomy == nil {
		return nil, fmt.Errorf("taxonomy cannot be nil")
	}

	return &taxonomyRegistry{
		taxonomy:         taxonomy,
		schemaBasedTools: make(map[string]bool),
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

// LoadAndValidateTaxonomyWithTools is a convenience function that loads taxonomy
// and augments it with schema-based tool output schemas from tool binaries.
// Schema-based schemas take precedence over YAML-based schemas.
func LoadAndValidateTaxonomyWithTools(ctx context.Context, toolsDir string, logger *slog.Logger) (TaxonomyRegistry, error) {
	// Load base taxonomy
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

	// Load schema-based tool schemas from tool binaries
	if toolsDir != "" {
		if err := registry.LoadToolSchemasFromBinaries(ctx, toolsDir, logger); err != nil {
			if logger != nil {
				logger.Warn("failed to load schema-based tool schemas",
					"tools_dir", toolsDir,
					"error", err)
			}
		}
	}

	return registry, nil
}

// LoadAndValidateTaxonomyWithToolsAndCustom is a convenience function that loads taxonomy
// with custom extensions and schema-based tool output schemas.
func LoadAndValidateTaxonomyWithToolsAndCustom(ctx context.Context, toolsDir, customPath string, logger *slog.Logger) (TaxonomyRegistry, error) {
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

	// Load schema-based tool schemas from tool binaries
	if toolsDir != "" {
		if err := registry.LoadToolSchemasFromBinaries(ctx, toolsDir, logger); err != nil {
			if logger != nil {
				logger.Warn("failed to load schema-based tool schemas",
					"tools_dir", toolsDir,
					"error", err)
			}
		}
	}

	return registry, nil
}

// GetTargetType returns a target type definition by its Type field.
func (r *taxonomyRegistry) GetTargetType(typeName string) (*TargetTypeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.TargetTypes[typeName]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy, true
}

// ListTargetTypes returns all target type definitions.
func (r *taxonomyRegistry) ListTargetTypes() []*TargetTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]*TargetTypeDefinition, 0, len(r.taxonomy.TargetTypes))
	for _, targetDef := range r.taxonomy.TargetTypes {
		defCopy := *targetDef
		types = append(types, &defCopy)
	}
	return types
}

// ListTargetTypesByCategory returns target types filtered by category.
func (r *taxonomyRegistry) ListTargetTypesByCategory(category string) []*TargetTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]*TargetTypeDefinition, 0)
	for _, targetDef := range r.taxonomy.TargetTypes {
		if targetDef.Category == category {
			defCopy := *targetDef
			types = append(types, &defCopy)
		}
	}
	return types
}

// ValidateTargetType checks if a target type string exists.
func (r *taxonomyRegistry) ValidateTargetType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.TargetTypes[typeName]
	return ok
}

// GetTechniqueType returns a technique type definition by its Type field.
func (r *taxonomyRegistry) GetTechniqueType(typeName string) (*TechniqueTypeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.TechniqueTypes[typeName]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy, true
}

// ListTechniqueTypes returns all technique type definitions.
func (r *taxonomyRegistry) ListTechniqueTypes() []*TechniqueTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]*TechniqueTypeDefinition, 0, len(r.taxonomy.TechniqueTypes))
	for _, techniqueDef := range r.taxonomy.TechniqueTypes {
		defCopy := *techniqueDef
		types = append(types, &defCopy)
	}
	return types
}

// ListTechniqueTypesByCategory returns technique types filtered by category (MITRE tactic).
func (r *taxonomyRegistry) ListTechniqueTypesByCategory(category string) []*TechniqueTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]*TechniqueTypeDefinition, 0)
	for _, techniqueDef := range r.taxonomy.TechniqueTypes {
		if techniqueDef.Category == category {
			defCopy := *techniqueDef
			types = append(types, &defCopy)
		}
	}
	return types
}

// ValidateTechniqueType checks if a technique type string exists.
func (r *taxonomyRegistry) ValidateTechniqueType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.TechniqueTypes[typeName]
	return ok
}

// GetTechniqueTypesByMITRE returns technique types that reference a specific MITRE ATT&CK ID.
func (r *taxonomyRegistry) GetTechniqueTypesByMITRE(mitreID string) []*TechniqueTypeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]*TechniqueTypeDefinition, 0)
	for _, techniqueDef := range r.taxonomy.TechniqueTypes {
		// Check if mitreID is in the mitre_ids slice
		for _, id := range techniqueDef.MITREIDs {
			if id == mitreID {
				defCopy := *techniqueDef
				types = append(types, &defCopy)
				break
			}
		}
	}
	return types
}

// GetCapability returns a capability definition by its ID.
func (r *taxonomyRegistry) GetCapability(id string) (*CapabilityDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.Capabilities[id]
	if !ok {
		return nil, false
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy, true
}

// ListCapabilities returns all capability definitions.
func (r *taxonomyRegistry) ListCapabilities() []*CapabilityDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	capabilities := make([]*CapabilityDefinition, 0, len(r.taxonomy.Capabilities))
	for _, capDef := range r.taxonomy.Capabilities {
		defCopy := *capDef
		capabilities = append(capabilities, &defCopy)
	}
	return capabilities
}

// GetTechniqueTypesForCapability returns the technique types associated with a capability.
func (r *taxonomyRegistry) GetTechniqueTypesForCapability(capability string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cap, ok := r.taxonomy.Capabilities[capability]
	if !ok {
		return []string{}
	}

	// Return a copy of the slice to prevent external modification
	techniqueTypes := make([]string, len(cap.TechniqueTypes))
	copy(techniqueTypes, cap.TechniqueTypes)
	return techniqueTypes
}

// ValidateCapability checks if a capability ID exists.
func (r *taxonomyRegistry) ValidateCapability(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.Capabilities[id]
	return ok
}

// GetExecutionEvent returns the event definition for the given event type.
func (r *taxonomyRegistry) GetExecutionEvent(eventType string) *ExecutionEventDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.ExecutionEvents[eventType]
	if !ok {
		return nil
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy
}

// GetToolOutputSchema returns the output schema for the given tool.
func (r *taxonomyRegistry) GetToolOutputSchema(toolName string) *ToolOutputSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.taxonomy.ToolOutputSchemas[toolName]
	if !ok {
		return nil
	}
	// Return a copy to prevent external modification
	defCopy := *def
	return &defCopy
}

// ListExecutionEvents returns all registered event types.
func (r *taxonomyRegistry) ListExecutionEvents() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	eventTypes := make([]string, 0, len(r.taxonomy.ExecutionEvents))
	for eventType := range r.taxonomy.ExecutionEvents {
		eventTypes = append(eventTypes, eventType)
	}
	return eventTypes
}

// ListToolOutputSchemas returns all registered tool names with schemas.
func (r *taxonomyRegistry) ListToolOutputSchemas() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	toolNames := make([]string, 0, len(r.taxonomy.ToolOutputSchemas))
	for toolName := range r.taxonomy.ToolOutputSchemas {
		toolNames = append(toolNames, toolName)
	}
	return toolNames
}

// HasExecutionEvent checks if an event type is defined.
func (r *taxonomyRegistry) HasExecutionEvent(eventType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.ExecutionEvents[eventType]
	return ok
}

// HasToolOutputSchema checks if a tool has an output schema.
func (r *taxonomyRegistry) HasToolOutputSchema(toolName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.taxonomy.ToolOutputSchemas[toolName]
	return ok
}

// LoadToolSchemasFromBinaries loads taxonomy from tool binaries in the given directory.
// Tools must embed their taxonomy using schema.TaxonomyMapping in their Go schema files.
func (r *taxonomyRegistry) LoadToolSchemasFromBinaries(ctx context.Context, toolsDir string, logger *slog.Logger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if logger == nil {
		logger = slog.Default()
	}

	// Create schema-based loader
	loader := NewSchemaBasedLoader(toolsDir, logger)

	// Load all tool schemas from binaries
	schemas, err := loader.LoadAllTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to load schemas from binaries: %w", err)
	}

	// Add schema-based schemas to taxonomy
	for toolName, schema := range schemas {
		r.taxonomy.ToolOutputSchemas[toolName] = schema
		r.schemaBasedTools[toolName] = true
		logger.Debug("loaded schema-based tool",
			"tool", toolName,
			"extracts", len(schema.Extracts))
	}

	logger.Info("loaded tool schemas from binaries",
		"tools_dir", toolsDir,
		"schema_based_tools", len(schemas))

	return nil
}

// IsSchemaBasedToolSchema returns true if the tool schema was loaded from a tool binary.
func (r *taxonomyRegistry) IsSchemaBasedToolSchema(toolName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.schemaBasedTools[toolName]
}
