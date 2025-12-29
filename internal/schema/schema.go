package schema

import "encoding/json"

// JSONSchema represents a JSON Schema for validation compatible with draft-07
type JSONSchema struct {
	Type                 string                 `json:"type"`
	Description          string                 `json:"description,omitempty"`
	Properties           map[string]SchemaField `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Items                *SchemaField           `json:"items,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
}

// SchemaField represents a field within a schema
type SchemaField struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Enum        []string               `json:"enum,omitempty"`
	Default     any                    `json:"default,omitempty"`
	Minimum     *float64               `json:"minimum,omitempty"`
	Maximum     *float64               `json:"maximum,omitempty"`
	MinLength   *int                   `json:"minLength,omitempty"`
	MaxLength   *int                   `json:"maxLength,omitempty"`
	Pattern     string                 `json:"pattern,omitempty"`
	Format      string                 `json:"format,omitempty"`
	Items       *SchemaField           `json:"items,omitempty"`
	Properties  map[string]SchemaField `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
}

// NewObjectSchema creates a new object schema with the given properties and required fields
func NewObjectSchema(properties map[string]SchemaField, required []string) JSONSchema {
	return JSONSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

// NewArraySchema creates a new array schema with the given item schema
func NewArraySchema(items SchemaField) JSONSchema {
	return JSONSchema{
		Type:  "array",
		Items: &items,
	}
}

// NewStringField creates a new string field with the given description
func NewStringField(description string) SchemaField {
	return SchemaField{
		Type:        "string",
		Description: description,
	}
}

// NewIntegerField creates a new integer field with the given description
func NewIntegerField(description string) SchemaField {
	return SchemaField{
		Type:        "integer",
		Description: description,
	}
}

// NewNumberField creates a new number field with the given description
func NewNumberField(description string) SchemaField {
	return SchemaField{
		Type:        "number",
		Description: description,
	}
}

// NewBooleanField creates a new boolean field with the given description
func NewBooleanField(description string) SchemaField {
	return SchemaField{
		Type:        "boolean",
		Description: description,
	}
}

// WithEnum adds enum constraint to the field
func (f SchemaField) WithEnum(values ...string) SchemaField {
	f.Enum = values
	return f
}

// WithMinMax adds minimum and maximum constraints to numeric fields
func (f SchemaField) WithMinMax(min, max float64) SchemaField {
	f.Minimum = &min
	f.Maximum = &max
	return f
}

// WithMin adds minimum constraint to numeric fields
func (f SchemaField) WithMin(min float64) SchemaField {
	f.Minimum = &min
	return f
}

// WithMax adds maximum constraint to numeric fields
func (f SchemaField) WithMax(max float64) SchemaField {
	f.Maximum = &max
	return f
}

// WithPattern adds regex pattern constraint to string fields
func (f SchemaField) WithPattern(pattern string) SchemaField {
	f.Pattern = pattern
	return f
}

// WithFormat adds format constraint to string fields (e.g., uri, email, date-time, uuid)
func (f SchemaField) WithFormat(format string) SchemaField {
	f.Format = format
	return f
}

// WithMinLength adds minimum length constraint to string fields
func (f SchemaField) WithMinLength(length int) SchemaField {
	f.MinLength = &length
	return f
}

// WithMaxLength adds maximum length constraint to string fields
func (f SchemaField) WithMaxLength(length int) SchemaField {
	f.MaxLength = &length
	return f
}

// WithDefault sets the default value for the field
func (f SchemaField) WithDefault(value any) SchemaField {
	f.Default = value
	return f
}

// WithDescription sets the description for the field
func (f SchemaField) WithDescription(description string) SchemaField {
	f.Description = description
	return f
}

// MarshalJSON ensures proper JSON serialization
func (s JSONSchema) MarshalJSON() ([]byte, error) {
	type Alias JSONSchema
	return json.Marshal((Alias)(s))
}

// MarshalJSON ensures proper JSON serialization for SchemaField
func (f SchemaField) MarshalJSON() ([]byte, error) {
	type Alias SchemaField
	return json.Marshal((Alias)(f))
}
