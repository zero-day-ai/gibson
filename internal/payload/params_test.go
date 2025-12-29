package payload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParameterSubstitutor_Substitute tests basic parameter substitution
func TestParameterSubstitutor_Substitute(t *testing.T) {
	sub := NewParameterSubstitutor()

	tests := []struct {
		name     string
		template string
		params   map[string]interface{}
		defs     []ParameterDef
		expected string
		wantErr  bool
	}{
		{
			name:     "simple string substitution",
			template: "Hello {{name}}!",
			params:   map[string]interface{}{"name": "World"},
			defs: []ParameterDef{
				{Name: "name", Type: ParameterTypeString, Required: true},
			},
			expected: "Hello World!",
			wantErr:  false,
		},
		{
			name:     "multiple parameters",
			template: "{{greeting}} {{name}}, you are {{age}} years old",
			params: map[string]interface{}{
				"greeting": "Hello",
				"name":     "Alice",
				"age":      30,
			},
			defs: []ParameterDef{
				{Name: "greeting", Type: ParameterTypeString, Required: true},
				{Name: "name", Type: ParameterTypeString, Required: true},
				{Name: "age", Type: ParameterTypeInt, Required: true},
			},
			expected: "Hello Alice, you are 30 years old",
			wantErr:  false,
		},
		{
			name:     "default parameter value",
			template: "Hello {{name}}!",
			params:   map[string]interface{}{},
			defs: []ParameterDef{
				{Name: "name", Type: ParameterTypeString, Required: false, Default: "Guest"},
			},
			expected: "Hello Guest!",
			wantErr:  false,
		},
		{
			name:     "missing required parameter",
			template: "Hello {{name}}!",
			params:   map[string]interface{}{},
			defs: []ParameterDef{
				{Name: "name", Type: ParameterTypeString, Required: true},
			},
			expected: "",
			wantErr:  true,
		},
		{
			name:     "boolean parameter",
			template: "Enabled: {{enabled}}",
			params:   map[string]interface{}{"enabled": true},
			defs: []ParameterDef{
				{Name: "enabled", Type: ParameterTypeBool, Required: true},
			},
			expected: "Enabled: true",
			wantErr:  false,
		},
		{
			name:     "float parameter",
			template: "Price: {{price}}",
			params:   map[string]interface{}{"price": 19.99},
			defs: []ParameterDef{
				{Name: "price", Type: ParameterTypeFloat, Required: true},
			},
			expected: "Price: 19.99",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sub.Substitute(tt.template, tt.params, tt.defs)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestParameterSubstitutor_Validate tests parameter validation
func TestParameterSubstitutor_Validate(t *testing.T) {
	sub := NewParameterSubstitutor()

	tests := []struct {
		name    string
		params  map[string]interface{}
		defs    []ParameterDef
		wantErr bool
		errMsg  string
	}{
		{
			name:   "valid required parameter",
			params: map[string]interface{}{"name": "test"},
			defs: []ParameterDef{
				{Name: "name", Type: ParameterTypeString, Required: true},
			},
			wantErr: false,
		},
		{
			name:   "missing required parameter",
			params: map[string]interface{}{},
			defs: []ParameterDef{
				{Name: "name", Type: ParameterTypeString, Required: true},
			},
			wantErr: true,
			errMsg:  "required parameter missing",
		},
		{
			name:   "wrong type for parameter",
			params: map[string]interface{}{"age": "not a number"},
			defs: []ParameterDef{
				{Name: "age", Type: ParameterTypeInt, Required: true},
			},
			wantErr: true,
			errMsg:  "expected int",
		},
		{
			name:   "string length validation - too short",
			params: map[string]interface{}{"name": "ab"},
			defs: []ParameterDef{
				{
					Name:     "name",
					Type:     ParameterTypeString,
					Required: true,
					Validation: &ParameterValidation{
						MinLength: intPtr(5),
					},
				},
			},
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name:   "string length validation - too long",
			params: map[string]interface{}{"name": "this is a very long string"},
			defs: []ParameterDef{
				{
					Name:     "name",
					Type:     ParameterTypeString,
					Required: true,
					Validation: &ParameterValidation{
						MaxLength: intPtr(10),
					},
				},
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name:   "numeric validation - below minimum",
			params: map[string]interface{}{"age": 5},
			defs: []ParameterDef{
				{
					Name:     "age",
					Type:     ParameterTypeInt,
					Required: true,
					Validation: &ParameterValidation{
						Min: float64Ptr(18),
					},
				},
			},
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name:   "numeric validation - above maximum",
			params: map[string]interface{}{"age": 150},
			defs: []ParameterDef{
				{
					Name:     "age",
					Type:     ParameterTypeInt,
					Required: true,
					Validation: &ParameterValidation{
						Max: float64Ptr(120),
					},
				},
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name:   "enum validation - valid",
			params: map[string]interface{}{"color": "red"},
			defs: []ParameterDef{
				{
					Name:     "color",
					Type:     ParameterTypeString,
					Required: true,
					Validation: &ParameterValidation{
						Enum: []any{"red", "green", "blue"},
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "enum validation - invalid",
			params: map[string]interface{}{"color": "yellow"},
			defs: []ParameterDef{
				{
					Name:     "color",
					Type:     ParameterTypeString,
					Required: true,
					Validation: &ParameterValidation{
						Enum: []any{"red", "green", "blue"},
					},
				},
			},
			wantErr: true,
			errMsg:  "not in allowed enum values",
		},
		{
			name:   "pattern validation - valid",
			params: map[string]interface{}{"email": "test@example.com"},
			defs: []ParameterDef{
				{
					Name:     "email",
					Type:     ParameterTypeString,
					Required: true,
					Validation: &ParameterValidation{
						Pattern: stringPtr(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`),
					},
				},
			},
			wantErr: false,
		},
		{
			name:   "pattern validation - invalid",
			params: map[string]interface{}{"email": "not-an-email"},
			defs: []ParameterDef{
				{
					Name:     "email",
					Type:     ParameterTypeString,
					Required: true,
					Validation: &ParameterValidation{
						Pattern: stringPtr(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`),
					},
				},
			},
			wantErr: true,
			errMsg:  "does not match pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sub.Validate(tt.params, tt.defs)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestParameterSubstitutor_GenerateValue tests value generation
func TestParameterSubstitutor_GenerateValue(t *testing.T) {
	sub := NewParameterSubstitutor()

	tests := []struct {
		name      string
		def       ParameterDef
		validator func(t *testing.T, value interface{})
		wantErr   bool
	}{
		{
			name: "uuid generator",
			def: ParameterDef{
				Name: "id",
				Type: ParameterTypeString,
				Generator: &ParameterGenerator{
					Type: GeneratorUUID,
				},
			},
			validator: func(t *testing.T, value interface{}) {
				str, ok := value.(string)
				require.True(t, ok)
				assert.Len(t, str, 36) // UUID length with dashes
			},
			wantErr: false,
		},
		{
			name: "timestamp generator",
			def: ParameterDef{
				Name: "created_at",
				Type: ParameterTypeString,
				Generator: &ParameterGenerator{
					Type: GeneratorTimestamp,
				},
			},
			validator: func(t *testing.T, value interface{}) {
				str, ok := value.(string)
				require.True(t, ok)
				assert.NotEmpty(t, str)
			},
			wantErr: false,
		},
		{
			name: "sequence generator",
			def: ParameterDef{
				Name: "seq",
				Type: ParameterTypeInt,
				Generator: &ParameterGenerator{
					Type: GeneratorSequence,
				},
			},
			validator: func(t *testing.T, value interface{}) {
				num, ok := value.(int)
				require.True(t, ok)
				assert.Greater(t, num, 0)
			},
			wantErr: false,
		},
		{
			name: "from_list generator",
			def: ParameterDef{
				Name: "option",
				Type: ParameterTypeString,
				Generator: &ParameterGenerator{
					Type: GeneratorFromList,
					Config: map[string]any{
						"values": []string{"option1", "option2", "option3"},
					},
				},
			},
			validator: func(t *testing.T, value interface{}) {
				str, ok := value.(string)
				require.True(t, ok)
				assert.Contains(t, []string{"option1", "option2", "option3"}, str)
			},
			wantErr: false,
		},
		{
			name: "random int generator",
			def: ParameterDef{
				Name: "random_num",
				Type: ParameterTypeInt,
				Generator: &ParameterGenerator{
					Type: GeneratorRandom,
					Config: map[string]any{
						"min": 1,
						"max": 100,
					},
				},
			},
			validator: func(t *testing.T, value interface{}) {
				num, ok := value.(int)
				require.True(t, ok)
				assert.GreaterOrEqual(t, num, 1)
				assert.LessOrEqual(t, num, 100)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := sub.GenerateValue(tt.def)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validator != nil {
					tt.validator(t, value)
				}
			}
		})
	}
}

// TestParameterSubstitutor_InjectTargetContext tests target context injection
func TestParameterSubstitutor_InjectTargetContext(t *testing.T) {
	sub := NewParameterSubstitutor()

	params := map[string]interface{}{
		"message": "Hello",
	}

	targetMetadata := map[string]interface{}{
		"name":     "TestTarget",
		"provider": "openai",
		"model":    "gpt-4",
	}

	result := sub.InjectTargetContext(params, targetMetadata)

	// Original parameter should be preserved
	assert.Equal(t, "Hello", result["message"])

	// Target metadata should be injected with prefix
	assert.Equal(t, "TestTarget", result["target_name"])
	assert.Equal(t, "openai", result["target_provider"])
	assert.Equal(t, "gpt-4", result["target_model"])

	// Ensure original maps are not modified
	assert.Len(t, params, 1)
	assert.Len(t, targetMetadata, 3)
}

// Helper functions are now in test_helpers.go
