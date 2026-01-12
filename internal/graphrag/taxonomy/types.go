// Package taxonomy provides a YAML-driven taxonomy system for GraphRAG node types,
// relationship types, and property schemas across the Zero-Day.AI platform.
//
// The taxonomy is embedded in the Gibson binary at compile time, with the version
// tied to the Gibson release. This ensures consistent behavior across deployments
// while allowing the YAML source files to be human-editable between releases.
package taxonomy

// TaxonomyMetadata contains version and descriptive information about the taxonomy.
type TaxonomyMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	UpdatedAt   string `yaml:"updated_at"`
}

// PropertyDefinition defines a property that can be attached to a node or relationship.
// Properties are strongly typed and can be marked as required or optional.
type PropertyDefinition struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // string, int, float64, bool, []string, map[string]any
	Required    bool   `yaml:"required"`
	Description string `yaml:"description"`
	Enum        []any  `yaml:"enum,omitempty"`    // For constrained values (e.g., severity levels)
	Default     any    `yaml:"default,omitempty"` // Default value if not provided
}

// NodeTypeDefinition defines a node type in the taxonomy.
// Each node type has a unique identifier, type string, and property schema.
type NodeTypeDefinition struct {
	ID          string               `yaml:"id"`           // Unique identifier (e.g., "node.asset.domain")
	Name        string               `yaml:"name"`         // Human-readable name (e.g., "Domain")
	Type        string               `yaml:"type"`         // GraphRAG node type string (e.g., "domain")
	Category    string               `yaml:"category"`     // Grouping category (e.g., "asset", "finding")
	Description string               `yaml:"description"`  // Purpose and usage description
	IDTemplate  string               `yaml:"id_template"`  // Format string for generating node IDs (e.g., "domain:{name}")
	Properties  []PropertyDefinition `yaml:"properties"`   // Property definitions
	Examples    []map[string]any     `yaml:"examples,omitempty"` // Example node definitions
}

// RelationshipTypeDefinition defines a relationship type in the taxonomy.
// Relationships have from/to type constraints to ensure graph consistency.
type RelationshipTypeDefinition struct {
	ID            string               `yaml:"id"`          // Unique identifier (e.g., "rel.asset.has_subdomain")
	Name          string               `yaml:"name"`        // Human-readable name (e.g., "HAS_SUBDOMAIN")
	Type          string               `yaml:"type"`        // GraphRAG relationship type string (e.g., "HAS_SUBDOMAIN")
	Category      string               `yaml:"category"`    // Grouping category (e.g., "asset_hierarchy", "finding")
	Description   string               `yaml:"description"` // Purpose and usage description
	FromTypes     []string             `yaml:"from_types"`  // Valid source node types
	ToTypes       []string             `yaml:"to_types"`    // Valid target node types
	Properties    []PropertyDefinition `yaml:"properties,omitempty"` // Optional properties on the relationship
	Bidirectional bool                 `yaml:"bidirectional"` // Whether relationship should be created in both directions
}

// TechniqueDefinition defines an attack technique (MITRE ATT&CK or Arcanum PI).
// Techniques can be mapped to findings to track attack patterns.
type TechniqueDefinition struct {
	TechniqueID         string   `yaml:"technique_id"`         // e.g., "T1190" or "ARC-T001"
	Name                string   `yaml:"name"`                 // Technique name
	Taxonomy            string   `yaml:"taxonomy"`             // "mitre" or "arcanum"
	Category            string   `yaml:"category"`             // e.g., "initial_access" or "attack_technique"
	Description         string   `yaml:"description"`          // Detailed description
	Tactic              string   `yaml:"tactic,omitempty"`     // For MITRE (e.g., "Initial Access")
	Platforms           []string `yaml:"platforms,omitempty"`  // Applicable platforms (e.g., ["Windows", "Linux"])
	Examples            []string `yaml:"examples,omitempty"`   // Example applications
	MITREMapping        []string `yaml:"mitre_mapping,omitempty"` // For Arcanum cross-reference to MITRE
	DetectionStrategies []string `yaml:"detection_strategies,omitempty"` // How to detect this technique
}

// ConnectionSchema defines the schema for target connection configurations.
// It specifies which configuration fields are required vs optional for connecting to a target.
type ConnectionSchema struct {
	Required []string `yaml:"required"` // Required connection fields (e.g., ["url", "api_key"])
	Optional []string `yaml:"optional"` // Optional connection fields (e.g., ["timeout", "headers"])
}

// TargetTypeDefinition defines a target type in the taxonomy.
// Target types represent systems, services, or applications that can be tested.
type TargetTypeDefinition struct {
	ID               string            `yaml:"id"`          // Unique identifier (e.g., "target.web.http_api")
	Type             string            `yaml:"type"`        // Target type string (e.g., "http_api")
	Name             string            `yaml:"name"`        // Human-readable name (e.g., "HTTP API")
	Category         string            `yaml:"category"`    // Grouping category (e.g., "web", "ai", "cloud")
	Description      string            `yaml:"description"` // Purpose and usage description
	ConnectionSchema *ConnectionSchema `yaml:"connection_schema,omitempty"` // Connection configuration schema
}

// TechniqueTypeDefinition defines a technique type in the taxonomy.
// Technique types represent categories of security testing techniques (e.g., SSRF, SQLi).
type TechniqueTypeDefinition struct {
	ID              string   `yaml:"id"`               // Unique identifier (e.g., "technique.initial_access.ssrf")
	Type            string   `yaml:"type"`             // Technique type string (e.g., "ssrf")
	Name            string   `yaml:"name"`             // Human-readable name (e.g., "Server-Side Request Forgery")
	Category        string   `yaml:"category"`         // MITRE ATT&CK tactic (e.g., "initial_access")
	MITREIDs        []string `yaml:"mitre_ids"`        // Related MITRE ATT&CK IDs (e.g., ["T1190"])
	Description     string   `yaml:"description"`      // Purpose and usage description
	DefaultSeverity string   `yaml:"default_severity"` // Default severity level (e.g., "high")
}

// CapabilityDefinition defines a capability in the taxonomy.
// Capabilities represent high-level testing capabilities that bundle technique types.
type CapabilityDefinition struct {
	ID             string   `yaml:"id"`              // Unique identifier (e.g., "capability.web_vulnerability_scanning")
	Name           string   `yaml:"name"`            // Human-readable name (e.g., "Web Vulnerability Scanning")
	Description    string   `yaml:"description"`     // Purpose and usage description
	TechniqueTypes []string `yaml:"technique_types"` // Related technique type IDs
}

// Taxonomy represents the complete loaded taxonomy with all node types,
// relationship types, and techniques. This is the in-memory representation
// that gets built from the embedded YAML files.
type Taxonomy struct {
	Version  string           `yaml:"version"`  // Taxonomy version (matches Gibson binary version)
	Metadata TaxonomyMetadata `yaml:"metadata"` // Descriptive metadata
	Includes []string         `yaml:"includes,omitempty"` // List of included YAML files (from root)

	// Primary collections - keyed by Type field for fast lookup
	NodeTypes       map[string]*NodeTypeDefinition         `yaml:"-"` // Keyed by Type field
	Relationships   map[string]*RelationshipTypeDefinition `yaml:"-"` // Keyed by Type field
	Techniques      map[string]*TechniqueDefinition        `yaml:"-"` // Keyed by TechniqueID
	TargetTypes     map[string]*TargetTypeDefinition       `yaml:"-"` // Keyed by Type field
	TechniqueTypes  map[string]*TechniqueTypeDefinition    `yaml:"-"` // Keyed by Type field
	Capabilities    map[string]*CapabilityDefinition       `yaml:"-"` // Keyed by ID field

	// Secondary indices for alternative lookups - built at load time
	nodeTypesByID       map[string]*NodeTypeDefinition         `yaml:"-"` // Keyed by ID field
	relationshipsByID   map[string]*RelationshipTypeDefinition `yaml:"-"` // Keyed by ID field
	targetTypesByID     map[string]*TargetTypeDefinition       `yaml:"-"` // Keyed by ID field
	techniqueTypesByID  map[string]*TechniqueTypeDefinition    `yaml:"-"` // Keyed by ID field
	capabilitiesByID    map[string]*CapabilityDefinition       `yaml:"-"` // Keyed by ID field

	// Extension tracking
	isCustomLoaded bool `yaml:"-"` // Whether custom taxonomy has been merged
}

// NodeTypeFile represents the structure of a node types YAML file.
// Multiple node type files can be loaded and merged into the taxonomy.
type NodeTypeFile struct {
	NodeTypes []NodeTypeDefinition `yaml:"node_types"`
}

// RelationshipTypeFile represents the structure of a relationship types YAML file.
// Multiple relationship type files can be loaded and merged into the taxonomy.
type RelationshipTypeFile struct {
	RelationshipTypes []RelationshipTypeDefinition `yaml:"relationship_types"`
}

// TechniqueFile represents the structure of a techniques YAML file.
// Multiple technique files can be loaded and merged into the taxonomy.
type TechniqueFile struct {
	Techniques []TechniqueDefinition `yaml:"techniques"`
}

// TargetTypeFile represents the structure of a target types YAML file.
// Multiple target type files can be loaded and merged into the taxonomy.
type TargetTypeFile struct {
	TargetTypes []TargetTypeDefinition `yaml:"target_types"`
}

// TechniqueTypeFile represents the structure of a technique types YAML file.
// Multiple technique type files can be loaded and merged into the taxonomy.
type TechniqueTypeFile struct {
	TechniqueTypes []TechniqueTypeDefinition `yaml:"technique_types"`
}

// CapabilityFile represents the structure of a capabilities YAML file.
// Multiple capability files can be loaded and merged into the taxonomy.
type CapabilityFile struct {
	Capabilities []CapabilityDefinition `yaml:"capabilities"`
}

// NewTaxonomy creates a new Taxonomy with initialized maps.
func NewTaxonomy(version string) *Taxonomy {
	return &Taxonomy{
		Version:            version,
		NodeTypes:          make(map[string]*NodeTypeDefinition),
		Relationships:      make(map[string]*RelationshipTypeDefinition),
		Techniques:         make(map[string]*TechniqueDefinition),
		TargetTypes:        make(map[string]*TargetTypeDefinition),
		TechniqueTypes:     make(map[string]*TechniqueTypeDefinition),
		Capabilities:       make(map[string]*CapabilityDefinition),
		nodeTypesByID:      make(map[string]*NodeTypeDefinition),
		relationshipsByID:  make(map[string]*RelationshipTypeDefinition),
		targetTypesByID:    make(map[string]*TargetTypeDefinition),
		techniqueTypesByID: make(map[string]*TechniqueTypeDefinition),
		capabilitiesByID:   make(map[string]*CapabilityDefinition),
		isCustomLoaded:     false,
	}
}

// AddNodeType adds a node type definition to the taxonomy.
// Returns an error if a node type with the same ID or Type already exists.
func (t *Taxonomy) AddNodeType(def *NodeTypeDefinition) error {
	// Check for ID collision
	if _, exists := t.nodeTypesByID[def.ID]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "node type with ID already exists",
			Field:   "id",
			Value:   def.ID,
		}
	}

	// Check for Type collision
	if _, exists := t.NodeTypes[def.Type]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "node type with Type already exists",
			Field:   "type",
			Value:   def.Type,
		}
	}

	t.NodeTypes[def.Type] = def
	t.nodeTypesByID[def.ID] = def
	return nil
}

// AddRelationship adds a relationship type definition to the taxonomy.
// Returns an error if a relationship with the same ID or Type already exists.
func (t *Taxonomy) AddRelationship(def *RelationshipTypeDefinition) error {
	// Check for ID collision
	if _, exists := t.relationshipsByID[def.ID]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "relationship type with ID already exists",
			Field:   "id",
			Value:   def.ID,
		}
	}

	// Check for Type collision
	if _, exists := t.Relationships[def.Type]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "relationship type with Type already exists",
			Field:   "type",
			Value:   def.Type,
		}
	}

	t.Relationships[def.Type] = def
	t.relationshipsByID[def.ID] = def
	return nil
}

// AddTechnique adds a technique definition to the taxonomy.
// Returns an error if a technique with the same ID already exists.
func (t *Taxonomy) AddTechnique(def *TechniqueDefinition) error {
	// Check for ID collision
	if _, exists := t.Techniques[def.TechniqueID]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "technique with ID already exists",
			Field:   "technique_id",
			Value:   def.TechniqueID,
		}
	}

	t.Techniques[def.TechniqueID] = def
	return nil
}

// GetNodeType retrieves a node type by its Type field.
func (t *Taxonomy) GetNodeType(typeName string) (*NodeTypeDefinition, bool) {
	def, ok := t.NodeTypes[typeName]
	return def, ok
}

// GetNodeTypeByID retrieves a node type by its ID field.
func (t *Taxonomy) GetNodeTypeByID(id string) (*NodeTypeDefinition, bool) {
	def, ok := t.nodeTypesByID[id]
	return def, ok
}

// GetRelationship retrieves a relationship type by its Type field.
func (t *Taxonomy) GetRelationship(typeName string) (*RelationshipTypeDefinition, bool) {
	def, ok := t.Relationships[typeName]
	return def, ok
}

// GetRelationshipByID retrieves a relationship type by its ID field.
func (t *Taxonomy) GetRelationshipByID(id string) (*RelationshipTypeDefinition, bool) {
	def, ok := t.relationshipsByID[id]
	return def, ok
}

// GetTechnique retrieves a technique by its TechniqueID.
func (t *Taxonomy) GetTechnique(techniqueID string) (*TechniqueDefinition, bool) {
	def, ok := t.Techniques[techniqueID]
	return def, ok
}

// AddTargetType adds a target type definition to the taxonomy.
// Returns an error if a target type with the same ID or Type already exists.
func (t *Taxonomy) AddTargetType(def *TargetTypeDefinition) error {
	// Check for ID collision
	if _, exists := t.targetTypesByID[def.ID]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "target type with ID already exists",
			Field:   "id",
			Value:   def.ID,
		}
	}

	// Check for Type collision
	if _, exists := t.TargetTypes[def.Type]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "target type with Type already exists",
			Field:   "type",
			Value:   def.Type,
		}
	}

	t.TargetTypes[def.Type] = def
	t.targetTypesByID[def.ID] = def
	return nil
}

// GetTargetType retrieves a target type by its Type field.
func (t *Taxonomy) GetTargetType(typeName string) (*TargetTypeDefinition, bool) {
	def, ok := t.TargetTypes[typeName]
	return def, ok
}

// GetTargetTypeByID retrieves a target type by its ID field.
func (t *Taxonomy) GetTargetTypeByID(id string) (*TargetTypeDefinition, bool) {
	def, ok := t.targetTypesByID[id]
	return def, ok
}

// AddTechniqueType adds a technique type definition to the taxonomy.
// Returns an error if a technique type with the same ID or Type already exists.
func (t *Taxonomy) AddTechniqueType(def *TechniqueTypeDefinition) error {
	// Check for ID collision
	if _, exists := t.techniqueTypesByID[def.ID]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "technique type with ID already exists",
			Field:   "id",
			Value:   def.ID,
		}
	}

	// Check for Type collision
	if _, exists := t.TechniqueTypes[def.Type]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "technique type with Type already exists",
			Field:   "type",
			Value:   def.Type,
		}
	}

	t.TechniqueTypes[def.Type] = def
	t.techniqueTypesByID[def.ID] = def
	return nil
}

// GetTechniqueType retrieves a technique type by its Type field.
func (t *Taxonomy) GetTechniqueType(typeName string) (*TechniqueTypeDefinition, bool) {
	def, ok := t.TechniqueTypes[typeName]
	return def, ok
}

// GetTechniqueTypeByID retrieves a technique type by its ID field.
func (t *Taxonomy) GetTechniqueTypeByID(id string) (*TechniqueTypeDefinition, bool) {
	def, ok := t.techniqueTypesByID[id]
	return def, ok
}

// AddCapability adds a capability definition to the taxonomy.
// Returns an error if a capability with the same ID already exists.
func (t *Taxonomy) AddCapability(def *CapabilityDefinition) error {
	// Check for ID collision
	if _, exists := t.capabilitiesByID[def.ID]; exists {
		return &TaxonomyError{
			Type:    ErrorTypeDuplicateDefinition,
			Message: "capability with ID already exists",
			Field:   "id",
			Value:   def.ID,
		}
	}

	// Also store in both maps (ID is used for both primary and secondary)
	t.Capabilities[def.ID] = def
	t.capabilitiesByID[def.ID] = def
	return nil
}

// GetCapability retrieves a capability by its ID.
func (t *Taxonomy) GetCapability(id string) (*CapabilityDefinition, bool) {
	def, ok := t.Capabilities[id]
	return def, ok
}

// GetCapabilityByID is an alias for GetCapability for consistency.
func (t *Taxonomy) GetCapabilityByID(id string) (*CapabilityDefinition, bool) {
	def, ok := t.capabilitiesByID[id]
	return def, ok
}

// IsCustomLoaded returns whether custom taxonomy has been merged.
func (t *Taxonomy) IsCustomLoaded() bool {
	return t.isCustomLoaded
}

// MarkCustomLoaded marks the taxonomy as having custom definitions merged.
func (t *Taxonomy) MarkCustomLoaded() {
	t.isCustomLoaded = true
}

// ErrorType represents the type of taxonomy error.
type ErrorType string

const (
	ErrorTypeDuplicateDefinition ErrorType = "duplicate_definition"
	ErrorTypeInvalidReference    ErrorType = "invalid_reference"
	ErrorTypeInvalidProperty     ErrorType = "invalid_property"
	ErrorTypeMissingField        ErrorType = "missing_field"
	ErrorTypeInvalidFormat       ErrorType = "invalid_format"
)

// TaxonomyError represents an error in taxonomy configuration.
type TaxonomyError struct {
	Type    ErrorType
	Message string
	Field   string // Which field caused the error
	Value   string // The problematic value
}

// Error implements the error interface.
func (e *TaxonomyError) Error() string {
	if e.Field != "" && e.Value != "" {
		return e.Message + " (field: " + e.Field + ", value: " + e.Value + ")"
	} else if e.Field != "" {
		return e.Message + " (field: " + e.Field + ")"
	}
	return e.Message
}

// Is implements error comparison for errors.Is.
func (e *TaxonomyError) Is(target error) bool {
	t, ok := target.(*TaxonomyError)
	if !ok {
		return false
	}
	return e.Type == t.Type
}
