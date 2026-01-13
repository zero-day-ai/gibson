package taxonomy

// ExecutionEventDefinition defines how an execution event (mission start, tool execution, etc.)
// should be represented in the graph. Events define what nodes and relationships to create
// when certain execution events occur.
type ExecutionEventDefinition struct {
	EventType   string `yaml:"event_type"`            // Type of event (e.g., "mission.started", "tool.call.started")
	Description string `yaml:"description,omitempty"` // Description of when this event occurs

	// CreatesNode defines the node type to create when this event occurs
	CreatesNode *EventNodeCreation `yaml:"creates_node,omitempty"`

	// UpdatesNode defines the node to update when this event occurs
	UpdatesNode *EventNodeUpdate `yaml:"updates_node,omitempty"`

	// CreatesRelationships defines relationships to create when this event occurs
	CreatesRelationships []EventRelationshipCreation `yaml:"creates_relationships,omitempty"`
}

// EventNodeCreation defines how to create a node from an event.
type EventNodeCreation struct {
	Type       string            `yaml:"type"`        // Type of node to create (must match taxonomy)
	IDTemplate string            `yaml:"id_template"` // Template for generating node ID
	Properties []PropertyMapping `yaml:"properties"`  // Property mappings from event to node
}

// EventNodeUpdate defines how to update an existing node from an event.
type EventNodeUpdate struct {
	Type       string            `yaml:"type"`        // Type of node to update (must match taxonomy)
	IDTemplate string            `yaml:"id_template"` // Template for identifying the node to update
	Properties []PropertyMapping `yaml:"properties"`  // Property mappings from event to node
}

// PropertyMapping defines how to map an event field to a node/relationship property.
type PropertyMapping struct {
	Source   string `yaml:"source,omitempty"`   // Source field from event payload
	Value    any    `yaml:"value,omitempty"`    // Static value to use instead of source field
	Target   string `yaml:"target"`             // Target property name on node/relationship
	Optional bool   `yaml:"optional,omitempty"` // If true, don't fail if source field is missing
}

// EventRelationshipCreation defines how to create a relationship from an event.
type EventRelationshipCreation struct {
	Type         string            `yaml:"type"`          // Type of relationship (must match taxonomy)
	FromTemplate string            `yaml:"from_template"` // Template for source node ID
	ToTemplate   string            `yaml:"to_template"`   // Template for target node ID
	Properties   []PropertyMapping `yaml:"properties,omitempty"` // Property mappings for the relationship
}

// ToolOutputSchema defines how to parse tool output and create graph nodes/relationships.
// This enables automatic asset discovery and graphing from tool outputs.
type ToolOutputSchema struct {
	Tool         string                  `yaml:"tool"`          // Name of the tool (e.g., "nmap", "subfinder")
	Description  string                  `yaml:"description"`   // Description of what this schema extracts
	OutputFormat string                  `yaml:"output_format"` // Output format (json, xml, text, line-delimited, etc.)
	Extracts     []ToolOutputExtraction  `yaml:"extracts"`      // What data to extract and how to create nodes
}

// ToolOutputExtraction defines a single extraction rule for tool output.
type ToolOutputExtraction struct {
	NodeType      string                      `yaml:"node_type"`       // Type of node to create (must match taxonomy)
	JSONPath      string                      `yaml:"json_path"`       // JSONPath expression to locate data
	IDTemplate    string                      `yaml:"id_template"`     // Template for generating node ID
	Condition     string                      `yaml:"condition,omitempty"` // Optional condition to filter extractions
	Properties    []ToolOutputProperty        `yaml:"properties"`      // Property mappings from JSON to node
	Relationships []ToolOutputRelationship    `yaml:"relationships,omitempty"` // Relationships to create
}

// ToolOutputProperty defines a property mapping from tool output to node property.
type ToolOutputProperty struct {
	JSONPath string `yaml:"json_path"`        // JSONPath to extract value from
	Target   string `yaml:"target"`           // Target property name on node
	Template string `yaml:"template,omitempty"` // Optional template for formatting value
	Default  any    `yaml:"default,omitempty"`  // Default value if JSONPath yields nothing
}

// ToolOutputRelationship defines a relationship to create from extracted tool output.
type ToolOutputRelationship struct {
	Type         string                `yaml:"type"`          // Type of relationship (must match taxonomy)
	FromTemplate string                `yaml:"from_template"` // Template for source node ID
	ToTemplate   string                `yaml:"to_template"`   // Template for target node ID
	Properties   []RelationshipProperty `yaml:"properties,omitempty"` // Property mappings for the relationship
}

// RelationshipProperty defines a property on a relationship.
type RelationshipProperty struct {
	Name  string `yaml:"name"`  // Property name
	Value string `yaml:"value"` // Property value (can be a template)
}

// ExecutionEventFile represents the structure of an execution events YAML file.
type ExecutionEventFile struct {
	ExecutionEvents []ExecutionEventDefinition `yaml:"execution_events"`
}

// ToolOutputFile represents the structure of a tool outputs YAML file.
type ToolOutputFile struct {
	ToolOutputs []ToolOutputSchema `yaml:"tool_outputs"`
}

// Validate checks that the ExecutionEventDefinition is valid.
// Returns an error if required fields are missing or invalid.
func (e *ExecutionEventDefinition) Validate() error {
	if e.EventType == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "event_type is required",
			Field:   "event_type",
		}
	}

	// At least one of CreatesNode, UpdatesNode, or CreatesRelationships must be specified
	if e.CreatesNode == nil && e.UpdatesNode == nil && len(e.CreatesRelationships) == 0 {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidFormat,
			Message: "execution event must create/update a node or create relationships",
			Field:   "event_type",
			Value:   e.EventType,
		}
	}

	// Validate node creation spec if present
	if e.CreatesNode != nil {
		if err := e.CreatesNode.Validate(); err != nil {
			return err
		}
	}

	// Validate node update spec if present
	if e.UpdatesNode != nil {
		if err := e.UpdatesNode.Validate(); err != nil {
			return err
		}
	}

	// Validate relationship specs
	for _, rel := range e.CreatesRelationships {
		if err := rel.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the EventNodeCreation is valid.
func (n *EventNodeCreation) Validate() error {
	if n.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "type is required",
			Field:   "type",
		}
	}

	if n.IDTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "id_template is required",
			Field:   "id_template",
		}
	}

	// Validate property mappings
	for _, prop := range n.Properties {
		if err := prop.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the EventNodeUpdate is valid.
func (n *EventNodeUpdate) Validate() error {
	if n.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "type is required",
			Field:   "type",
		}
	}

	if n.IDTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "id_template is required",
			Field:   "id_template",
		}
	}

	// Validate property mappings
	for _, prop := range n.Properties {
		if err := prop.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the PropertyMapping is valid.
func (p *PropertyMapping) Validate() error {
	if p.Target == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "target is required",
			Field:   "target",
		}
	}

	// Either source or value must be specified
	if p.Source == "" && p.Value == nil {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidFormat,
			Message: "either source or value must be specified",
			Field:   "target",
			Value:   p.Target,
		}
	}

	return nil
}

// Validate checks that the EventRelationshipCreation is valid.
func (r *EventRelationshipCreation) Validate() error {
	if r.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "type is required",
			Field:   "type",
		}
	}

	if r.FromTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "from_template is required",
			Field:   "from_template",
		}
	}

	if r.ToTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "to_template is required",
			Field:   "to_template",
		}
	}

	// Validate property mappings
	for _, prop := range r.Properties {
		if err := prop.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the ToolOutputSchema is valid.
func (t *ToolOutputSchema) Validate() error {
	if t.Tool == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "tool is required",
			Field:   "tool",
		}
	}

	if len(t.Extracts) == 0 {
		return &TaxonomyError{
			Type:    ErrorTypeInvalidFormat,
			Message: "tool output schema must have at least one extract spec",
			Field:   "tool",
			Value:   t.Tool,
		}
	}

	// Validate each extract spec
	for _, extract := range t.Extracts {
		if err := extract.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the ToolOutputExtraction is valid.
func (e *ToolOutputExtraction) Validate() error {
	if e.NodeType == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "node_type is required",
			Field:   "node_type",
		}
	}

	if e.JSONPath == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "json_path is required",
			Field:   "json_path",
		}
	}

	if e.IDTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "id_template is required",
			Field:   "id_template",
		}
	}

	// Validate property mappings
	for _, prop := range e.Properties {
		if err := prop.Validate(); err != nil {
			return err
		}
	}

	// Validate relationships
	for _, rel := range e.Relationships {
		if err := rel.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the ToolOutputProperty is valid.
func (p *ToolOutputProperty) Validate() error {
	if p.JSONPath == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "json_path is required",
			Field:   "json_path",
		}
	}

	if p.Target == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "target is required",
			Field:   "target",
		}
	}

	return nil
}

// Validate checks that the ToolOutputRelationship is valid.
func (r *ToolOutputRelationship) Validate() error {
	if r.Type == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "type is required",
			Field:   "type",
		}
	}

	if r.FromTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "from_template is required",
			Field:   "from_template",
		}
	}

	if r.ToTemplate == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "to_template is required",
			Field:   "to_template",
		}
	}

	// Validate properties
	for _, prop := range r.Properties {
		if err := prop.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that the RelationshipProperty is valid.
func (p *RelationshipProperty) Validate() error {
	if p.Name == "" {
		return &TaxonomyError{
			Type:    ErrorTypeMissingField,
			Message: "name is required",
			Field:   "name",
		}
	}

	return nil
}

// HasNodeCreation checks if the execution event definition creates a node.
func (e *ExecutionEventDefinition) HasNodeCreation() bool {
	return e.CreatesNode != nil
}

// HasNodeUpdate checks if the execution event definition updates a node.
func (e *ExecutionEventDefinition) HasNodeUpdate() bool {
	return e.UpdatesNode != nil
}

// HasRelationships checks if the execution event definition creates any relationships.
func (e *ExecutionEventDefinition) HasRelationships() bool {
	return len(e.CreatesRelationships) > 0
}

// GetExtractByNodeType finds an extract spec for a specific node type.
// Returns nil if no extract spec is found for the given node type.
func (t *ToolOutputSchema) GetExtractByNodeType(nodeType string) *ToolOutputExtraction {
	for i := range t.Extracts {
		if t.Extracts[i].NodeType == nodeType {
			return &t.Extracts[i]
		}
	}
	return nil
}

// HasExtractForNodeType checks if the tool output schema has an extract spec for the given node type.
func (t *ToolOutputSchema) HasExtractForNodeType(nodeType string) bool {
	return t.GetExtractByNodeType(nodeType) != nil
}

// HasRelationships checks if the extraction creates any relationships.
func (e *ToolOutputExtraction) HasRelationships() bool {
	return len(e.Relationships) > 0
}
