package schema

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Validator validates data against a schema
type Validator interface {
	Validate(schema JSONSchema, data map[string]any) []ValidationError
}

// ValidationError represents a schema validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   any    `json:"value,omitempty"`
}

// Error implements the error interface
func (e ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("%s: %s (value: %v)", e.Field, e.Message, e.Value)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// DefaultValidator implements Validator
type DefaultValidator struct{}

// NewValidator creates a new DefaultValidator
func NewValidator() *DefaultValidator {
	return &DefaultValidator{}
}

// Validate validates data against the provided schema
func (v *DefaultValidator) Validate(schema JSONSchema, data map[string]any) []ValidationError {
	var errors []ValidationError

	// Validate type
	if schema.Type != "object" {
		errors = append(errors, ValidationError{
			Field:   "",
			Message: fmt.Sprintf("root type must be object, got %s", schema.Type),
		})
		return errors
	}

	// Validate required fields
	for _, field := range schema.Required {
		if _, exists := data[field]; !exists {
			errors = append(errors, ValidationError{
				Field:   field,
				Message: "required field is missing",
			})
		}
	}

	// Validate each property
	for fieldName, value := range data {
		fieldSchema, hasSchema := schema.Properties[fieldName]
		if !hasSchema {
			// Field not in schema - check additionalProperties
			if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
				errors = append(errors, ValidationError{
					Field:   fieldName,
					Message: "additional property not allowed",
					Value:   value,
				})
			}
			continue
		}

		// Validate field value against its schema
		fieldErrors := v.validateField(fieldName, fieldSchema, value)
		errors = append(errors, fieldErrors...)
	}

	return errors
}

// validateField validates a single field against its schema
func (v *DefaultValidator) validateField(fieldPath string, schema SchemaField, value any) []ValidationError {
	var errors []ValidationError

	// Type validation
	actualType := getJSONType(value)
	if !v.isTypeCompatible(schema.Type, actualType, value) {
		errors = append(errors, ValidationError{
			Field:   fieldPath,
			Message: fmt.Sprintf("expected type %s, got %s", schema.Type, actualType),
			Value:   value,
		})
		return errors // Return early if type is wrong
	}

	// Type-specific validation
	switch schema.Type {
	case "string":
		errors = append(errors, v.validateString(fieldPath, schema, value)...)
	case "number", "integer":
		errors = append(errors, v.validateNumber(fieldPath, schema, value)...)
	case "boolean":
		// Boolean type already validated above
	case "array":
		errors = append(errors, v.validateArray(fieldPath, schema, value)...)
	case "object":
		errors = append(errors, v.validateObject(fieldPath, schema, value)...)
	}

	// Enum validation
	if len(schema.Enum) > 0 {
		errors = append(errors, v.validateEnum(fieldPath, schema, value)...)
	}

	return errors
}

// validateString validates string-specific constraints
func (v *DefaultValidator) validateString(fieldPath string, schema SchemaField, value any) []ValidationError {
	var errors []ValidationError
	str, ok := value.(string)
	if !ok {
		return errors
	}

	// MinLength
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		errors = append(errors, ValidationError{
			Field:   fieldPath,
			Message: fmt.Sprintf("string length must be at least %d", *schema.MinLength),
			Value:   value,
		})
	}

	// MaxLength
	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		errors = append(errors, ValidationError{
			Field:   fieldPath,
			Message: fmt.Sprintf("string length must be at most %d", *schema.MaxLength),
			Value:   value,
		})
	}

	// Pattern
	if schema.Pattern != "" {
		matched, err := regexp.MatchString(schema.Pattern, str)
		if err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: fmt.Sprintf("invalid pattern: %v", err),
			})
		} else if !matched {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: fmt.Sprintf("string does not match pattern %s", schema.Pattern),
				Value:   value,
			})
		}
	}

	// Format validation
	if schema.Format != "" {
		errors = append(errors, v.validateFormat(fieldPath, schema.Format, str)...)
	}

	return errors
}

// validateNumber validates numeric constraints
func (v *DefaultValidator) validateNumber(fieldPath string, schema SchemaField, value any) []ValidationError {
	var errors []ValidationError
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

	// Integer validation - check if number is whole
	if schema.Type == "integer" {
		if num != float64(int64(num)) {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "expected integer, got decimal number",
				Value:   value,
			})
		}
	}

	// Minimum
	if schema.Minimum != nil && num < *schema.Minimum {
		errors = append(errors, ValidationError{
			Field:   fieldPath,
			Message: fmt.Sprintf("value must be at least %v", *schema.Minimum),
			Value:   value,
		})
	}

	// Maximum
	if schema.Maximum != nil && num > *schema.Maximum {
		errors = append(errors, ValidationError{
			Field:   fieldPath,
			Message: fmt.Sprintf("value must be at most %v", *schema.Maximum),
			Value:   value,
		})
	}

	return errors
}

// validateArray validates array items
func (v *DefaultValidator) validateArray(fieldPath string, schema SchemaField, value any) []ValidationError {
	var errors []ValidationError
	arr, ok := value.([]any)
	if !ok {
		return errors
	}

	// Validate each item against the items schema
	if schema.Items != nil {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", fieldPath, i)
			itemErrors := v.validateField(itemPath, *schema.Items, item)
			errors = append(errors, itemErrors...)
		}
	}

	return errors
}

// validateObject validates nested objects
func (v *DefaultValidator) validateObject(fieldPath string, schema SchemaField, value any) []ValidationError {
	var errors []ValidationError
	obj, ok := value.(map[string]any)
	if !ok {
		return errors
	}

	// Check required fields
	for _, required := range schema.Required {
		if _, exists := obj[required]; !exists {
			nestedPath := fmt.Sprintf("%s.%s", fieldPath, required)
			errors = append(errors, ValidationError{
				Field:   nestedPath,
				Message: "required field is missing",
			})
		}
	}

	// Validate each property
	for propName, propValue := range obj {
		propSchema, hasSchema := schema.Properties[propName]
		if !hasSchema {
			continue
		}

		nestedPath := fmt.Sprintf("%s.%s", fieldPath, propName)
		propErrors := v.validateField(nestedPath, propSchema, propValue)
		errors = append(errors, propErrors...)
	}

	return errors
}

// validateEnum validates enum constraints
func (v *DefaultValidator) validateEnum(fieldPath string, schema SchemaField, value any) []ValidationError {
	var errors []ValidationError
	strValue := fmt.Sprintf("%v", value)

	found := false
	for _, enumValue := range schema.Enum {
		if strValue == enumValue {
			found = true
			break
		}
	}

	if !found {
		errors = append(errors, ValidationError{
			Field:   fieldPath,
			Message: fmt.Sprintf("value must be one of: %s", strings.Join(schema.Enum, ", ")),
			Value:   value,
		})
	}

	return errors
}

// validateFormat validates string format constraints
func (v *DefaultValidator) validateFormat(fieldPath, format, value string) []ValidationError {
	var errors []ValidationError

	switch format {
	case "uri", "url":
		if _, err := url.ParseRequestURI(value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "invalid URI format",
				Value:   value,
			})
		}
	case "email":
		if _, err := mail.ParseAddress(value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "invalid email format",
				Value:   value,
			})
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "invalid date-time format (expected RFC3339)",
				Value:   value,
			})
		}
	case "uuid":
		if _, err := uuid.Parse(value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "invalid UUID format",
				Value:   value,
			})
		}
	case "date":
		if _, err := time.Parse("2006-01-02", value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "invalid date format (expected YYYY-MM-DD)",
				Value:   value,
			})
		}
	case "time":
		if _, err := time.Parse("15:04:05", value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "invalid time format (expected HH:MM:SS)",
				Value:   value,
			})
		}
	}

	return errors
}

// isTypeCompatible checks if the actual type is compatible with the expected type
func (v *DefaultValidator) isTypeCompatible(expectedType, actualType string, value any) bool {
	if expectedType == actualType {
		return true
	}

	// Integer is compatible with number in JSON
	if expectedType == "integer" && actualType == "number" {
		// Check if it's actually a whole number
		switch v := value.(type) {
		case float64:
			return v == float64(int64(v))
		case int, int64, int32:
			return true
		}
	}

	return false
}

// getJSONType returns the JSON type of a value
func getJSONType(value any) string {
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
