package payload

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ParameterSubstitutor handles parameter substitution in payload templates
type ParameterSubstitutor interface {
	// Substitute replaces {{parameter}} placeholders in template with actual values
	Substitute(template string, params map[string]interface{}, defs []ParameterDef) (string, error)

	// Validate checks if provided parameters match parameter definitions
	Validate(params map[string]interface{}, defs []ParameterDef) error

	// GenerateValue generates a value for a parameter using its generator
	GenerateValue(def ParameterDef) (interface{}, error)

	// InjectTargetContext automatically injects target metadata into parameters
	InjectTargetContext(params map[string]interface{}, targetMetadata map[string]interface{}) map[string]interface{}
}

// parameterSubstitutor implements ParameterSubstitutor
type parameterSubstitutor struct {
	paramRegex *regexp.Regexp
	sequences  map[string]int // Track sequence counters per parameter
}

// NewParameterSubstitutor creates a new parameter substitutor
func NewParameterSubstitutor() ParameterSubstitutor {
	return &parameterSubstitutor{
		paramRegex: regexp.MustCompile(`\{\{([a-zA-Z0-9_]+)\}\}`),
		sequences:  make(map[string]int),
	}
}

// Substitute replaces {{parameter}} placeholders with actual values
func (s *parameterSubstitutor) Substitute(template string, params map[string]interface{}, defs []ParameterDef) (string, error) {
	// First validate parameters
	if err := s.Validate(params, defs); err != nil {
		return "", fmt.Errorf("parameter validation failed: %w", err)
	}

	// Create a map of parameter definitions for quick lookup
	defMap := make(map[string]ParameterDef)
	for _, def := range defs {
		defMap[def.Name] = def
	}

	// Build final parameter map with defaults and generated values
	finalParams := make(map[string]interface{})
	for name, value := range params {
		finalParams[name] = value
	}

	// Fill in missing optional parameters with defaults or generated values
	for _, def := range defs {
		if _, exists := finalParams[def.Name]; !exists {
			if !def.Required {
				// Use default if available
				if def.Default != nil {
					finalParams[def.Name] = def.Default
				} else if def.Generator != nil {
					// Generate value
					generated, err := s.GenerateValue(def)
					if err != nil {
						return "", fmt.Errorf("failed to generate value for parameter %s: %w", def.Name, err)
					}
					finalParams[def.Name] = generated
				}
			}
		}
	}

	// Replace placeholders
	result := s.paramRegex.ReplaceAllStringFunc(template, func(match string) string {
		// Extract parameter name from {{name}}
		paramName := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")

		// Get value from parameters
		value, exists := finalParams[paramName]
		if !exists {
			// Parameter not found - this shouldn't happen after validation,
			// but we'll keep the placeholder as-is
			return match
		}

		// Convert value to string based on type
		def, hasDef := defMap[paramName]
		if hasDef {
			return s.valueToString(value, def.Type)
		}

		// No definition - convert as generic
		return fmt.Sprintf("%v", value)
	})

	return result, nil
}

// Validate checks if provided parameters match parameter definitions
func (s *parameterSubstitutor) Validate(params map[string]interface{}, defs []ParameterDef) error {
	// Create a map of parameter definitions for quick lookup
	defMap := make(map[string]ParameterDef)
	for _, def := range defs {
		defMap[def.Name] = def
	}

	// Check that all required parameters are provided
	for _, def := range defs {
		if def.Required {
			if _, exists := params[def.Name]; !exists {
				// Check if it has a default or generator
				if def.Default == nil && def.Generator == nil {
					return fmt.Errorf("required parameter missing: %s", def.Name)
				}
			}
		}
	}

	// Validate each provided parameter
	for name, value := range params {
		def, exists := defMap[name]
		if !exists {
			// Unknown parameter - we'll allow it for flexibility
			continue
		}

		// Validate type
		if err := s.validateType(value, def.Type); err != nil {
			return fmt.Errorf("parameter %s: %w", name, err)
		}

		// Apply validation rules if present
		if def.Validation != nil {
			if err := s.validateRules(value, def.Validation, def.Type); err != nil {
				return fmt.Errorf("parameter %s: %w", name, err)
			}
		}
	}

	return nil
}

// validateType checks if a value matches the expected type
func (s *parameterSubstitutor) validateType(value interface{}, paramType ParameterType) error {
	switch paramType {
	case ParameterTypeString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case ParameterTypeInt:
		switch value.(type) {
		case int, int32, int64, float64:
			// Accept numeric types that can be converted to int
		default:
			return fmt.Errorf("expected int, got %T", value)
		}
	case ParameterTypeBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
	case ParameterTypeFloat:
		switch value.(type) {
		case float64, float32, int, int32, int64:
			// Accept numeric types
		default:
			return fmt.Errorf("expected float, got %T", value)
		}
	case ParameterTypeJSON:
		// JSON can be any type, but we'll try to marshal it to verify
		if _, err := json.Marshal(value); err != nil {
			return fmt.Errorf("invalid JSON value: %w", err)
		}
	case ParameterTypeList:
		// Check if it's a slice or array
		switch value.(type) {
		case []interface{}, []string, []int, []float64:
			// Valid list types
		default:
			return fmt.Errorf("expected list, got %T", value)
		}
	default:
		return fmt.Errorf("unknown parameter type: %s", paramType)
	}

	return nil
}

// validateRules applies validation rules to a value
func (s *parameterSubstitutor) validateRules(value interface{}, validation *ParameterValidation, paramType ParameterType) error {
	// String-specific validations
	if paramType == ParameterTypeString {
		strValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string value")
		}

		// MinLength
		if validation.MinLength != nil && len(strValue) < *validation.MinLength {
			return fmt.Errorf("string length %d is less than minimum %d", len(strValue), *validation.MinLength)
		}

		// MaxLength
		if validation.MaxLength != nil && len(strValue) > *validation.MaxLength {
			return fmt.Errorf("string length %d exceeds maximum %d", len(strValue), *validation.MaxLength)
		}

		// Pattern (regex)
		if validation.Pattern != nil {
			matched, err := regexp.MatchString(*validation.Pattern, strValue)
			if err != nil {
				return fmt.Errorf("invalid regex pattern: %w", err)
			}
			if !matched {
				return fmt.Errorf("string does not match pattern: %s", *validation.Pattern)
			}
		}
	}

	// Numeric validations
	if paramType == ParameterTypeInt || paramType == ParameterTypeFloat {
		var numValue float64
		switch v := value.(type) {
		case int:
			numValue = float64(v)
		case int32:
			numValue = float64(v)
		case int64:
			numValue = float64(v)
		case float32:
			numValue = float64(v)
		case float64:
			numValue = v
		default:
			return fmt.Errorf("expected numeric value")
		}

		// Min
		if validation.Min != nil && numValue < *validation.Min {
			return fmt.Errorf("value %v is less than minimum %v", numValue, *validation.Min)
		}

		// Max
		if validation.Max != nil && numValue > *validation.Max {
			return fmt.Errorf("value %v exceeds maximum %v", numValue, *validation.Max)
		}
	}

	// Enum validation (applies to all types)
	if validation.Enum != nil && len(validation.Enum) > 0 {
		found := false
		for _, enumValue := range validation.Enum {
			if value == enumValue {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value %v is not in allowed enum values", value)
		}
	}

	return nil
}

// GenerateValue generates a value for a parameter using its generator
func (s *parameterSubstitutor) GenerateValue(def ParameterDef) (interface{}, error) {
	if def.Generator == nil {
		return nil, fmt.Errorf("no generator defined for parameter: %s", def.Name)
	}

	switch def.Generator.Type {
	case GeneratorUUID:
		return uuid.New().String(), nil

	case GeneratorTimestamp:
		// Support format in config
		format := "2006-01-02T15:04:05Z07:00" // RFC3339 default
		if f, ok := def.Generator.Config["format"].(string); ok {
			format = f
		}
		return time.Now().Format(format), nil

	case GeneratorSequence:
		// Increment and return sequence number
		s.sequences[def.Name]++
		return s.sequences[def.Name], nil

	case GeneratorFromList:
		// Pick a value from a list
		list, ok := def.Generator.Config["values"]
		if !ok {
			return nil, fmt.Errorf("from_list generator requires 'values' config")
		}

		var values []interface{}
		switch v := list.(type) {
		case []interface{}:
			values = v
		case []string:
			for _, s := range v {
				values = append(values, s)
			}
		default:
			return nil, fmt.Errorf("from_list values must be a list")
		}

		if len(values) == 0 {
			return nil, fmt.Errorf("from_list values is empty")
		}

		// For simplicity, return the first value
		// In a real implementation, this could be random or sequential
		return values[0], nil

	case GeneratorRandom:
		// Generate random value based on parameter type
		switch def.Type {
		case ParameterTypeInt:
			min := 0
			max := 100
			if m, ok := def.Generator.Config["min"].(int); ok {
				min = m
			}
			if m, ok := def.Generator.Config["max"].(int); ok {
				max = m
			}
			// Simple pseudo-random (use time-based for demo)
			return min + (int(time.Now().UnixNano()) % (max - min + 1)), nil

		case ParameterTypeString:
			length := 10
			if l, ok := def.Generator.Config["length"].(int); ok {
				length = l
			}
			// Generate random string (simplified - using uuid prefix)
			return uuid.New().String()[:length], nil

		default:
			return nil, fmt.Errorf("random generator not supported for type: %s", def.Type)
		}

	case GeneratorExpression:
		// Expression evaluation would require an expression engine
		// For now, return a simple error
		return nil, fmt.Errorf("expression generator not yet implemented")

	default:
		return nil, fmt.Errorf("unknown generator type: %s", def.Generator.Type)
	}
}

// InjectTargetContext automatically injects target metadata into parameters
func (s *parameterSubstitutor) InjectTargetContext(params map[string]interface{}, targetMetadata map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy existing parameters
	for k, v := range params {
		result[k] = v
	}

	// Inject target metadata with "target_" prefix
	for k, v := range targetMetadata {
		paramKey := "target_" + k
		// Don't override if already set
		if _, exists := result[paramKey]; !exists {
			result[paramKey] = v
		}
	}

	return result
}

// valueToString converts a value to string based on its type
func (s *parameterSubstitutor) valueToString(value interface{}, paramType ParameterType) string {
	switch paramType {
	case ParameterTypeString:
		if str, ok := value.(string); ok {
			return str
		}

	case ParameterTypeInt:
		switch v := value.(type) {
		case int:
			return strconv.Itoa(v)
		case int32:
			return strconv.FormatInt(int64(v), 10)
		case int64:
			return strconv.FormatInt(v, 10)
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}

	case ParameterTypeBool:
		if b, ok := value.(bool); ok {
			return strconv.FormatBool(b)
		}

	case ParameterTypeFloat:
		switch v := value.(type) {
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case float32:
			return strconv.FormatFloat(float64(v), 'f', -1, 32)
		case int:
			return strconv.FormatFloat(float64(v), 'f', -1, 64)
		}

	case ParameterTypeJSON:
		// Marshal to JSON string
		if jsonBytes, err := json.Marshal(value); err == nil {
			return string(jsonBytes)
		}

	case ParameterTypeList:
		// Marshal to JSON array string
		if jsonBytes, err := json.Marshal(value); err == nil {
			return string(jsonBytes)
		}
	}

	// Fallback to fmt.Sprintf
	return fmt.Sprintf("%v", value)
}
