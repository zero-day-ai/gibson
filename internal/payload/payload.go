package payload

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Payload represents an attack payload that can be executed against LLM targets.
// Payloads are parameterized templates that can be instantiated with specific values.
type Payload struct {
	// Identity
	ID          types.ID `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Version     string   `json:"version" yaml:"version"`
	Description string   `json:"description" yaml:"description"`

	// Categorization
	Categories []PayloadCategory `json:"categories" yaml:"categories"`
	Tags       []string          `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Content
	Template string `json:"template" yaml:"template"` // Template with {{parameter}} placeholders

	// Parameters
	Parameters []ParameterDef `json:"parameters,omitempty" yaml:"parameters,omitempty"`

	// Success Detection
	SuccessIndicators []SuccessIndicator `json:"success_indicators" yaml:"success_indicators"`

	// Targeting
	TargetTypes []string              `json:"target_types,omitempty" yaml:"target_types,omitempty"` // e.g., ["openai", "anthropic", "rag"]
	Severity    agent.FindingSeverity `json:"severity" yaml:"severity"`

	// MITRE Mappings
	MitreTechniques []string `json:"mitre_techniques,omitempty" yaml:"mitre_techniques,omitempty"` // e.g., ["AML.T0051"]

	// Metadata
	Metadata PayloadMetadata `json:"metadata" yaml:"metadata"`

	// Status
	BuiltIn   bool      `json:"built_in" yaml:"built_in"`
	Enabled   bool      `json:"enabled" yaml:"enabled"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// PayloadCategory represents the type of attack this payload performs
type PayloadCategory string

const (
	CategoryJailbreak             PayloadCategory = "jailbreak"
	CategoryPromptInjection       PayloadCategory = "prompt_injection"
	CategoryDataExtraction        PayloadCategory = "data_extraction"
	CategoryDoS                   PayloadCategory = "dos"
	CategoryModelManipulation     PayloadCategory = "model_manipulation"
	CategoryPrivilegeEscalation   PayloadCategory = "privilege_escalation"
	CategoryRAGPoisoning          PayloadCategory = "rag_poisoning"
	CategoryInformationDisclosure PayloadCategory = "information_disclosure"
	CategoryEncodingBypass        PayloadCategory = "encoding_bypass"
)

// String returns the string representation of PayloadCategory
func (pc PayloadCategory) String() string {
	return string(pc)
}

// IsValid checks if the category is a valid value
func (pc PayloadCategory) IsValid() bool {
	switch pc {
	case CategoryJailbreak, CategoryPromptInjection, CategoryDataExtraction,
		CategoryDoS, CategoryModelManipulation, CategoryPrivilegeEscalation,
		CategoryRAGPoisoning, CategoryInformationDisclosure, CategoryEncodingBypass:
		return true
	default:
		return false
	}
}

// AllCategories returns all valid payload categories
func AllCategories() []PayloadCategory {
	return []PayloadCategory{
		CategoryJailbreak,
		CategoryPromptInjection,
		CategoryDataExtraction,
		CategoryDoS,
		CategoryModelManipulation,
		CategoryPrivilegeEscalation,
		CategoryRAGPoisoning,
		CategoryInformationDisclosure,
		CategoryEncodingBypass,
	}
}

// ParameterDef defines a parameter that can be substituted in the payload template
type ParameterDef struct {
	Name        string               `json:"name" yaml:"name"`
	Type        ParameterType        `json:"type" yaml:"type"`
	Description string               `json:"description" yaml:"description"`
	Required    bool                 `json:"required" yaml:"required"`
	Default     any                  `json:"default,omitempty" yaml:"default,omitempty"`
	Generator   *ParameterGenerator  `json:"generator,omitempty" yaml:"generator,omitempty"`
	Validation  *ParameterValidation `json:"validation,omitempty" yaml:"validation,omitempty"`
}

// ParameterType represents the data type of a parameter
type ParameterType string

const (
	ParameterTypeString ParameterType = "string"
	ParameterTypeInt    ParameterType = "int"
	ParameterTypeBool   ParameterType = "bool"
	ParameterTypeFloat  ParameterType = "float"
	ParameterTypeJSON   ParameterType = "json"
	ParameterTypeList   ParameterType = "list"
)

// String returns the string representation of ParameterType
func (pt ParameterType) String() string {
	return string(pt)
}

// IsValid checks if the parameter type is valid
func (pt ParameterType) IsValid() bool {
	switch pt {
	case ParameterTypeString, ParameterTypeInt, ParameterTypeBool,
		ParameterTypeFloat, ParameterTypeJSON, ParameterTypeList:
		return true
	default:
		return false
	}
}

// ParameterGenerator defines how to automatically generate a parameter value
type ParameterGenerator struct {
	Type   GeneratorType  `json:"type" yaml:"type"`
	Config map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

// GeneratorType represents the type of generator
type GeneratorType string

const (
	GeneratorRandom     GeneratorType = "random"     // Random value
	GeneratorUUID       GeneratorType = "uuid"       // Generate UUID
	GeneratorTimestamp  GeneratorType = "timestamp"  // Current timestamp
	GeneratorSequence   GeneratorType = "sequence"   // Sequential counter
	GeneratorFromList   GeneratorType = "from_list"  // Pick from list
	GeneratorExpression GeneratorType = "expression" // Evaluate expression
)

// String returns the string representation of GeneratorType
func (gt GeneratorType) String() string {
	return string(gt)
}

// ParameterValidation defines validation rules for a parameter
type ParameterValidation struct {
	MinLength *int     `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty" yaml:"max_length,omitempty"`
	Pattern   *string  `json:"pattern,omitempty" yaml:"pattern,omitempty"` // Regex pattern
	Enum      []any    `json:"enum,omitempty" yaml:"enum,omitempty"`       // Allowed values
	Min       *float64 `json:"min,omitempty" yaml:"min,omitempty"`         // Minimum value (numeric)
	Max       *float64 `json:"max,omitempty" yaml:"max,omitempty"`         // Maximum value (numeric)
}

// SuccessIndicator defines how to detect if a payload execution was successful
type SuccessIndicator struct {
	Type        IndicatorType `json:"type" yaml:"type"`
	Value       string        `json:"value" yaml:"value"` // Pattern, substring, or value to match
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	Weight      float64       `json:"weight,omitempty" yaml:"weight,omitempty"` // 0.0-1.0, for weighted scoring
	Negate      bool          `json:"negate,omitempty" yaml:"negate,omitempty"` // True if absence indicates success
}

// IndicatorType represents the type of success indicator
type IndicatorType string

const (
	IndicatorRegex       IndicatorType = "regex"        // Regex pattern match
	IndicatorContains    IndicatorType = "contains"     // Substring match
	IndicatorNotContains IndicatorType = "not_contains" // Absence of substring
	IndicatorLength      IndicatorType = "length"       // Response length check
	IndicatorStatus      IndicatorType = "status"       // HTTP status code
	IndicatorJSON        IndicatorType = "json"         // JSON path check
	IndicatorSemantic    IndicatorType = "semantic"     // Semantic similarity check
)

// String returns the string representation of IndicatorType
func (it IndicatorType) String() string {
	return string(it)
}

// IsValid checks if the indicator type is valid
func (it IndicatorType) IsValid() bool {
	switch it {
	case IndicatorRegex, IndicatorContains, IndicatorNotContains,
		IndicatorLength, IndicatorStatus, IndicatorJSON, IndicatorSemantic:
		return true
	default:
		return false
	}
}

// PayloadMetadata contains additional metadata about the payload
type PayloadMetadata struct {
	Author      string   `json:"author,omitempty" yaml:"author,omitempty"`
	Source      string   `json:"source,omitempty" yaml:"source,omitempty"`         // URL or reference
	References  []string `json:"references,omitempty" yaml:"references,omitempty"` // Related URLs
	Notes       string   `json:"notes,omitempty" yaml:"notes,omitempty"`
	Examples    []string `json:"examples,omitempty" yaml:"examples,omitempty"`       // Example executions
	Difficulty  string   `json:"difficulty,omitempty" yaml:"difficulty,omitempty"`   // easy, medium, hard
	Reliability float64  `json:"reliability,omitempty" yaml:"reliability,omitempty"` // 0.0-1.0, expected success rate
}

// PayloadFilter defines filter criteria for querying payloads
type PayloadFilter struct {
	IDs             []types.ID              `json:"ids,omitempty"`
	Categories      []PayloadCategory       `json:"categories,omitempty"`
	Tags            []string                `json:"tags,omitempty"`
	TargetTypes     []string                `json:"target_types,omitempty"`
	Severities      []agent.FindingSeverity `json:"severities,omitempty"`
	MitreTechniques []string                `json:"mitre_techniques,omitempty"`
	BuiltIn         *bool                   `json:"built_in,omitempty"`
	Enabled         *bool                   `json:"enabled,omitempty"`
	SearchQuery     string                  `json:"search_query,omitempty"` // Full-text search
	Limit           int                     `json:"limit,omitempty"`
	Offset          int                     `json:"offset,omitempty"`
}

// NewPayload creates a new payload with the given name and template
func NewPayload(name, template string, categories ...PayloadCategory) Payload {
	now := time.Now()
	return Payload{
		ID:                types.NewID(),
		Name:              name,
		Version:           "1.0.0",
		Template:          template,
		Categories:        categories,
		Tags:              []string{},
		Parameters:        []ParameterDef{},
		SuccessIndicators: []SuccessIndicator{},
		TargetTypes:       []string{},
		MitreTechniques:   []string{},
		Metadata:          PayloadMetadata{},
		BuiltIn:           false,
		Enabled:           true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

// WithDescription sets the description for the payload
func (p Payload) WithDescription(description string) Payload {
	p.Description = description
	p.UpdatedAt = time.Now()
	return p
}

// WithVersion sets the version for the payload
func (p Payload) WithVersion(version string) Payload {
	p.Version = version
	p.UpdatedAt = time.Now()
	return p
}

// WithTags sets the tags for the payload
func (p Payload) WithTags(tags ...string) Payload {
	p.Tags = tags
	p.UpdatedAt = time.Now()
	return p
}

// WithParameters sets the parameters for the payload
func (p Payload) WithParameters(params ...ParameterDef) Payload {
	p.Parameters = params
	p.UpdatedAt = time.Now()
	return p
}

// WithSuccessIndicators sets the success indicators for the payload
func (p Payload) WithSuccessIndicators(indicators ...SuccessIndicator) Payload {
	p.SuccessIndicators = indicators
	p.UpdatedAt = time.Now()
	return p
}

// WithTargetTypes sets the target types for the payload
func (p Payload) WithTargetTypes(targetTypes ...string) Payload {
	p.TargetTypes = targetTypes
	p.UpdatedAt = time.Now()
	return p
}

// WithSeverity sets the severity for the payload
func (p Payload) WithSeverity(severity agent.FindingSeverity) Payload {
	p.Severity = severity
	p.UpdatedAt = time.Now()
	return p
}

// WithMitreTechniques sets the MITRE techniques for the payload
func (p Payload) WithMitreTechniques(techniques ...string) Payload {
	p.MitreTechniques = techniques
	p.UpdatedAt = time.Now()
	return p
}

// WithMetadata sets the metadata for the payload
func (p Payload) WithMetadata(metadata PayloadMetadata) Payload {
	p.Metadata = metadata
	p.UpdatedAt = time.Now()
	return p
}

// MarkBuiltIn marks the payload as a built-in payload
func (p Payload) MarkBuiltIn() Payload {
	p.BuiltIn = true
	p.UpdatedAt = time.Now()
	return p
}

// Enable enables the payload
func (p Payload) Enable() Payload {
	p.Enabled = true
	p.UpdatedAt = time.Now()
	return p
}

// Disable disables the payload
func (p Payload) Disable() Payload {
	p.Enabled = false
	p.UpdatedAt = time.Now()
	return p
}

// Validate checks if the payload is valid
func (p Payload) Validate() error {
	if err := p.ID.Validate(); err != nil {
		return fmt.Errorf("invalid payload ID: %w", err)
	}

	if p.Name == "" {
		return fmt.Errorf("payload name is required")
	}

	if p.Template == "" {
		return fmt.Errorf("payload template is required")
	}

	if len(p.Categories) == 0 {
		return fmt.Errorf("at least one category is required")
	}

	for i, cat := range p.Categories {
		if !cat.IsValid() {
			return fmt.Errorf("invalid category at index %d: %s", i, cat)
		}
	}

	if len(p.SuccessIndicators) == 0 {
		return fmt.Errorf("at least one success indicator is required")
	}

	for i, indicator := range p.SuccessIndicators {
		if !indicator.Type.IsValid() {
			return fmt.Errorf("invalid indicator type at index %d: %s", i, indicator.Type)
		}
		if indicator.Value == "" {
			return fmt.Errorf("indicator value is required at index %d", i)
		}
	}

	for i, param := range p.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter name is required at index %d", i)
		}
		if !param.Type.IsValid() {
			return fmt.Errorf("invalid parameter type at index %d: %s", i, param.Type)
		}
	}

	return nil
}

// HasCategory checks if the payload has the given category
func (p Payload) HasCategory(category PayloadCategory) bool {
	for _, c := range p.Categories {
		if c == category {
			return true
		}
	}
	return false
}

// HasTag checks if the payload has the given tag
func (p Payload) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// HasMitreTechnique checks if the payload has the given MITRE technique
func (p Payload) HasMitreTechnique(technique string) bool {
	for _, t := range p.MitreTechniques {
		if t == technique {
			return true
		}
	}
	return false
}

// SupportsTargetType checks if the payload supports the given target type
func (p Payload) SupportsTargetType(targetType string) bool {
	// Empty target types means supports all
	if len(p.TargetTypes) == 0 {
		return true
	}

	for _, t := range p.TargetTypes {
		if t == targetType {
			return true
		}
	}
	return false
}

// GetRequiredParameters returns all required parameters
func (p Payload) GetRequiredParameters() []ParameterDef {
	var required []ParameterDef
	for _, param := range p.Parameters {
		if param.Required {
			required = append(required, param)
		}
	}
	return required
}

// GetOptionalParameters returns all optional parameters
func (p Payload) GetOptionalParameters() []ParameterDef {
	var optional []ParameterDef
	for _, param := range p.Parameters {
		if !param.Required {
			optional = append(optional, param)
		}
	}
	return optional
}

// GetParameterByName returns a parameter by name, or nil if not found
func (p Payload) GetParameterByName(name string) *ParameterDef {
	for _, param := range p.Parameters {
		if param.Name == name {
			return &param
		}
	}
	return nil
}
