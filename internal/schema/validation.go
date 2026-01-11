package schema

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// JSONValidator validates JSON data against a JSONSchema
// This validator works with raw JSON bytes and provides detailed error paths.
type JSONValidator struct {
	schema *JSONSchema
}

// NewJSONValidator creates a new schema validator
func NewJSONValidator(schema *JSONSchema) *JSONValidator {
	return &JSONValidator{schema: schema}
}

// SchemaValidationError contains details about schema validation failure
type SchemaValidationError struct {
	Message  string `json:"message"`
	Path     string `json:"path"`     // JSON path to invalid field (e.g., "$.hosts[0].ip")
	Expected string `json:"expected"` // What was expected
	Actual   string `json:"actual"`   // What was received
	Raw      string `json:"-"`        // Raw response for debugging
}

func (e *SchemaValidationError) Error() string {
	if e.Expected != "" && e.Actual != "" {
		return fmt.Sprintf("validation failed at %s: %s (expected %s, got %s)", e.Path, e.Message, e.Expected, e.Actual)
	}
	return fmt.Sprintf("validation failed at %s: %s", e.Path, e.Message)
}

// Validate validates JSON data against the schema
func (v *JSONValidator) Validate(data []byte) error {
	// Parse JSON
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return &SchemaValidationError{
			Message: "invalid JSON",
			Path:    "$",
			Raw:     string(data),
		}
	}

	// Validate against schema
	if errs := v.validateValue(parsed, v.schema, "$"); len(errs) > 0 {
		// Return the first error for simplicity
		// In production, you might want to return all errors
		return &errs[0]
	}

	return nil
}

// ValidateAndUnmarshal validates and unmarshals JSON into target
func (v *JSONValidator) ValidateAndUnmarshal(data []byte, target any) error {
	if err := v.Validate(data); err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// validateValue recursively validates a value against a schema
func (v *JSONValidator) validateValue(value any, schema *JSONSchema, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	// Validate type
	if schema.Type != "" {
		actualType := getValueType(value)
		if !isTypeCompatible(schema.Type, actualType, value) {
			errors = append(errors, SchemaValidationError{
				Message:  "type mismatch",
				Path:     path,
				Expected: schema.Type,
				Actual:   actualType,
			})
			return errors // Return early on type mismatch
		}
	}

	// Type-specific validation
	switch schema.Type {
	case "object":
		errors = append(errors, v.validateObject(value, schema, path)...)
	case "array":
		errors = append(errors, v.validateArray(value, schema, path)...)
	}

	return errors
}

// validateObject validates object properties and required fields
func (v *JSONValidator) validateObject(value any, schema *JSONSchema, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	obj, ok := value.(map[string]any)
	if !ok {
		errors = append(errors, SchemaValidationError{
			Message:  "expected object",
			Path:     path,
			Expected: "object",
			Actual:   getValueType(value),
		})
		return errors
	}

	// Check required fields
	for _, required := range schema.Required {
		if _, exists := obj[required]; !exists {
			fieldPath := fmt.Sprintf("%s.%s", path, required)
			errors = append(errors, SchemaValidationError{
				Message:  "required field is missing",
				Path:     fieldPath,
				Expected: "field to be present",
				Actual:   "field is missing",
			})
		}
	}

	// Validate additional properties
	if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
		for fieldName := range obj {
			if _, hasSchema := schema.Properties[fieldName]; !hasSchema {
				fieldPath := fmt.Sprintf("%s.%s", path, fieldName)
				errors = append(errors, SchemaValidationError{
					Message:  "additional property not allowed",
					Path:     fieldPath,
					Expected: "no additional properties",
					Actual:   fmt.Sprintf("found property '%s'", fieldName),
				})
			}
		}
	}

	// Validate each property
	for fieldName, fieldValue := range obj {
		fieldSchema, hasSchema := schema.Properties[fieldName]
		if !hasSchema {
			continue
		}

		fieldPath := fmt.Sprintf("%s.%s", path, fieldName)
		fieldErrors := v.validateField(fieldValue, &fieldSchema, fieldPath)
		errors = append(errors, fieldErrors...)
	}

	return errors
}

// validateArray validates array items
func (v *JSONValidator) validateArray(value any, schema *JSONSchema, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	arr, ok := value.([]any)
	if !ok {
		errors = append(errors, SchemaValidationError{
			Message:  "expected array",
			Path:     path,
			Expected: "array",
			Actual:   getValueType(value),
		})
		return errors
	}

	// Validate each item
	if schema.Items != nil {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			itemErrors := v.validateField(item, schema.Items, itemPath)
			errors = append(errors, itemErrors...)
		}
	}

	return errors
}

// validateField validates a field against its schema
func (v *JSONValidator) validateField(value any, schema *SchemaField, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	// Type validation
	if schema.Type != "" {
		actualType := getValueType(value)
		if !isTypeCompatible(schema.Type, actualType, value) {
			errors = append(errors, SchemaValidationError{
				Message:  "type mismatch",
				Path:     path,
				Expected: schema.Type,
				Actual:   actualType,
			})
			return errors // Return early on type mismatch
		}
	}

	// Type-specific validation
	switch schema.Type {
	case "string":
		errors = append(errors, v.validateStringField(value, schema, path)...)
	case "number", "integer":
		errors = append(errors, v.validateNumberField(value, schema, path)...)
	case "array":
		errors = append(errors, v.validateArrayField(value, schema, path)...)
	case "object":
		errors = append(errors, v.validateObjectField(value, schema, path)...)
	}

	// Enum validation
	if len(schema.Enum) > 0 {
		errors = append(errors, v.validateEnum(value, schema, path)...)
	}

	return errors
}

// validateStringField validates string-specific constraints
func (v *JSONValidator) validateStringField(value any, schema *SchemaField, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	str, ok := value.(string)
	if !ok {
		return errors
	}

	// MinLength
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		errors = append(errors, SchemaValidationError{
			Message:  "string too short",
			Path:     path,
			Expected: fmt.Sprintf("length >= %d", *schema.MinLength),
			Actual:   fmt.Sprintf("length = %d", len(str)),
		})
	}

	// MaxLength
	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		errors = append(errors, SchemaValidationError{
			Message:  "string too long",
			Path:     path,
			Expected: fmt.Sprintf("length <= %d", *schema.MaxLength),
			Actual:   fmt.Sprintf("length = %d", len(str)),
		})
	}

	// Pattern
	if schema.Pattern != "" {
		matched, err := regexp.MatchString(schema.Pattern, str)
		if err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  fmt.Sprintf("invalid regex pattern: %v", err),
				Path:     path,
				Expected: fmt.Sprintf("pattern: %s", schema.Pattern),
				Actual:   "pattern validation failed",
			})
		} else if !matched {
			errors = append(errors, SchemaValidationError{
				Message:  "string does not match pattern",
				Path:     path,
				Expected: fmt.Sprintf("pattern: %s", schema.Pattern),
				Actual:   fmt.Sprintf("value: %s", str),
			})
		}
	}

	// Format validation
	if schema.Format != "" {
		errors = append(errors, v.validateFormat(str, schema.Format, path)...)
	}

	return errors
}

// validateNumberField validates numeric constraints
func (v *JSONValidator) validateNumberField(value any, schema *SchemaField, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	var num float64
	switch v := value.(type) {
	case float64:
		num = v
	case float32:
		num = float64(v)
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case int32:
		num = float64(v)
	default:
		return errors
	}

	// Integer validation
	if schema.Type == "integer" {
		if num != float64(int64(num)) {
			errors = append(errors, SchemaValidationError{
				Message:  "not a valid integer",
				Path:     path,
				Expected: "integer value",
				Actual:   fmt.Sprintf("decimal: %v", num),
			})
		}
	}

	// Minimum
	if schema.Minimum != nil && num < *schema.Minimum {
		errors = append(errors, SchemaValidationError{
			Message:  "value below minimum",
			Path:     path,
			Expected: fmt.Sprintf(">= %v", *schema.Minimum),
			Actual:   fmt.Sprintf("%v", num),
		})
	}

	// Maximum
	if schema.Maximum != nil && num > *schema.Maximum {
		errors = append(errors, SchemaValidationError{
			Message:  "value above maximum",
			Path:     path,
			Expected: fmt.Sprintf("<= %v", *schema.Maximum),
			Actual:   fmt.Sprintf("%v", num),
		})
	}

	return errors
}

// validateArrayField validates array items
func (v *JSONValidator) validateArrayField(value any, schema *SchemaField, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	arr, ok := value.([]any)
	if !ok {
		return errors
	}

	// Validate each item
	if schema.Items != nil {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			itemErrors := v.validateField(item, schema.Items, itemPath)
			errors = append(errors, itemErrors...)
		}
	}

	return errors
}

// validateObjectField validates nested objects
func (v *JSONValidator) validateObjectField(value any, schema *SchemaField, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	obj, ok := value.(map[string]any)
	if !ok {
		return errors
	}

	// Check required fields
	for _, required := range schema.Required {
		if _, exists := obj[required]; !exists {
			fieldPath := fmt.Sprintf("%s.%s", path, required)
			errors = append(errors, SchemaValidationError{
				Message:  "required field is missing",
				Path:     fieldPath,
				Expected: "field to be present",
				Actual:   "field is missing",
			})
		}
	}

	// Validate each property
	for propName, propValue := range obj {
		propSchema, hasSchema := schema.Properties[propName]
		if !hasSchema {
			continue
		}

		propPath := fmt.Sprintf("%s.%s", path, propName)
		propErrors := v.validateField(propValue, &propSchema, propPath)
		errors = append(errors, propErrors...)
	}

	return errors
}

// validateEnum validates enum constraints
func (v *JSONValidator) validateEnum(value any, schema *SchemaField, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	strValue := fmt.Sprintf("%v", value)

	found := false
	for _, enumValue := range schema.Enum {
		if strValue == enumValue {
			found = true
			break
		}
	}

	if !found {
		errors = append(errors, SchemaValidationError{
			Message:  "invalid enum value",
			Path:     path,
			Expected: fmt.Sprintf("one of: %s", strings.Join(schema.Enum, ", ")),
			Actual:   strValue,
		})
	}

	return errors
}

// validateFormat validates string format constraints
func (v *JSONValidator) validateFormat(value, format, path string) []SchemaValidationError {
	var errors []SchemaValidationError

	switch format {
	case "uri", "url":
		if _, err := url.ParseRequestURI(value); err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  "invalid URI format",
				Path:     path,
				Expected: "valid URI",
				Actual:   value,
			})
		}
	case "email":
		if _, err := mail.ParseAddress(value); err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  "invalid email format",
				Path:     path,
				Expected: "valid email address",
				Actual:   value,
			})
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  "invalid date-time format",
				Path:     path,
				Expected: "RFC3339 format (e.g., 2006-01-02T15:04:05Z07:00)",
				Actual:   value,
			})
		}
	case "uuid":
		if _, err := uuid.Parse(value); err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  "invalid UUID format",
				Path:     path,
				Expected: "valid UUID",
				Actual:   value,
			})
		}
	case "date":
		if _, err := time.Parse("2006-01-02", value); err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  "invalid date format",
				Path:     path,
				Expected: "YYYY-MM-DD format",
				Actual:   value,
			})
		}
	case "time":
		if _, err := time.Parse("15:04:05", value); err != nil {
			errors = append(errors, SchemaValidationError{
				Message:  "invalid time format",
				Path:     path,
				Expected: "HH:MM:SS format",
				Actual:   value,
			})
		}
	}

	return errors
}

// isTypeCompatible checks if actual type is compatible with expected type
func isTypeCompatible(expected, actual string, value any) bool {
	if expected == actual {
		return true
	}

	// Integer is compatible with number in JSON
	if expected == "integer" && actual == "number" {
		switch v := value.(type) {
		case float64:
			return v == float64(int64(v))
		case int, int64, int32:
			return true
		}
	}

	return false
}

// getValueType returns the JSON type of a value
func getValueType(value any) string {
	if value == nil {
		return "null"
	}

	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}
