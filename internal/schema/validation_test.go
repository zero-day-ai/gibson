package schema

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewValidator tests validator creation
func TestNewValidator(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"name": NewStringField("Name"),
	}, []string{"name"})

	validator := NewJSONValidator(&schema)
	require.NotNil(t, validator)
	assert.Equal(t, &schema, validator.schema)
}

// TestValidate_ValidJSON tests successful validation
func TestValidate_ValidJSON(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"name":  NewStringField("Name"),
		"age":   NewIntegerField("Age"),
		"email": NewStringField("Email").WithFormat("email"),
	}, []string{"name"})

	validator := NewJSONValidator(&schema)

	validJSON := []byte(`{
		"name": "John Doe",
		"age": 30,
		"email": "john@example.com"
	}`)

	err := validator.Validate(validJSON)
	assert.NoError(t, err)
}

// TestValidate_InvalidJSON tests invalid JSON parsing
func TestValidate_InvalidJSON(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"name": NewStringField("Name"),
	}, []string{"name"})

	validator := NewJSONValidator(&schema)

	invalidJSON := []byte(`{invalid json}`)

	err := validator.Validate(invalidJSON)
	require.Error(t, err)

	validationErr, ok := err.(*SchemaValidationError)
	require.True(t, ok, "Expected ValidationError")
	assert.Equal(t, "$", validationErr.Path)
	assert.Contains(t, validationErr.Message, "invalid JSON")
}

// TestValidate_MissingRequiredFields tests required field validation
func TestValidate_MissingRequiredFields(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"name":  NewStringField("Name"),
		"email": NewStringField("Email"),
	}, []string{"name", "email"})

	validator := NewJSONValidator(&schema)

	tests := []struct {
		name         string
		json         string
		expectedPath string
	}{
		{
			name:         "missing name",
			json:         `{"email": "test@example.com"}`,
			expectedPath: "$.name",
		},
		{
			name:         "missing email",
			json:         `{"name": "John"}`,
			expectedPath: "$.email",
		},
		{
			name:         "missing both",
			json:         `{}`,
			expectedPath: "$.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate([]byte(tt.json))
			require.Error(t, err)

			validationErr, ok := err.(*SchemaValidationError)
			require.True(t, ok)
			assert.Equal(t, tt.expectedPath, validationErr.Path)
			assert.Contains(t, validationErr.Message, "required field")
		})
	}
}

// TestValidate_TypeMismatch tests type validation
func TestValidate_TypeMismatch(t *testing.T) {
	tests := []struct {
		name         string
		schema       JSONSchema
		json         string
		expectedPath string
		expectedType string
		actualType   string
	}{
		{
			name: "string instead of integer",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age"),
			}, nil),
			json:         `{"age": "thirty"}`,
			expectedPath: "$.age",
			expectedType: "integer",
			actualType:   "string",
		},
		{
			name: "integer instead of string",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, nil),
			json:         `{"name": 123}`,
			expectedPath: "$.name",
			expectedType: "string",
			actualType:   "number",
		},
		{
			name: "string instead of boolean",
			schema: NewObjectSchema(map[string]SchemaField{
				"active": NewBooleanField("Active"),
			}, nil),
			json:         `{"active": "true"}`,
			expectedPath: "$.active",
			expectedType: "boolean",
			actualType:   "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))
			require.Error(t, err)

			validationErr, ok := err.(*SchemaValidationError)
			require.True(t, ok)
			assert.Equal(t, tt.expectedPath, validationErr.Path)
			assert.Equal(t, tt.expectedType, validationErr.Expected)
			assert.Equal(t, tt.actualType, validationErr.Actual)
		})
	}
}

// TestValidate_StringConstraints tests string validation
func TestValidate_StringConstraints(t *testing.T) {
	tests := []struct {
		name         string
		schema       JSONSchema
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name: "valid string length",
			schema: NewObjectSchema(map[string]SchemaField{
				"username": NewStringField("Username").WithMinLength(3).WithMaxLength(20),
			}, nil),
			json:       `{"username": "john_doe"}`,
			shouldPass: true,
		},
		{
			name: "string too short",
			schema: NewObjectSchema(map[string]SchemaField{
				"password": NewStringField("Password").WithMinLength(8),
			}, nil),
			json:         `{"password": "short"}`,
			shouldPass:   false,
			expectedPath: "$.password",
		},
		{
			name: "string too long",
			schema: NewObjectSchema(map[string]SchemaField{
				"bio": NewStringField("Bio").WithMaxLength(10),
			}, nil),
			json:         `{"bio": "this is a very long biography"}`,
			shouldPass:   false,
			expectedPath: "$.bio",
		},
		{
			name: "valid pattern",
			schema: NewObjectSchema(map[string]SchemaField{
				"username": NewStringField("Username").WithPattern("^[a-z0-9_]+$"),
			}, nil),
			json:       `{"username": "john_doe123"}`,
			shouldPass: true,
		},
		{
			name: "invalid pattern",
			schema: NewObjectSchema(map[string]SchemaField{
				"username": NewStringField("Username").WithPattern("^[a-z0-9_]+$"),
			}, nil),
			json:         `{"username": "john-doe"}`,
			shouldPass:   false,
			expectedPath: "$.username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedPath, validationErr.Path)
			}
		})
	}
}

// TestValidate_NumericConstraints tests numeric validation
func TestValidate_NumericConstraints(t *testing.T) {
	tests := []struct {
		name         string
		schema       JSONSchema
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name: "valid integer in range",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age").WithMinMax(0, 150),
			}, nil),
			json:       `{"age": 30}`,
			shouldPass: true,
		},
		{
			name: "integer below minimum",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age").WithMin(0),
			}, nil),
			json:         `{"age": -5}`,
			shouldPass:   false,
			expectedPath: "$.age",
		},
		{
			name: "integer above maximum",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age").WithMax(150),
			}, nil),
			json:         `{"age": 200}`,
			shouldPass:   false,
			expectedPath: "$.age",
		},
		{
			name: "decimal for integer type",
			schema: NewObjectSchema(map[string]SchemaField{
				"count": NewIntegerField("Count"),
			}, nil),
			json:         `{"count": 25.5}`,
			shouldPass:   false,
			expectedPath: "$.count",
		},
		{
			name: "whole number as float for integer type",
			schema: NewObjectSchema(map[string]SchemaField{
				"count": NewIntegerField("Count"),
			}, nil),
			json:       `{"count": 25.0}`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedPath, validationErr.Path)
			}
		})
	}
}

// TestValidate_EnumConstraints tests enum validation
func TestValidate_EnumConstraints(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"status": NewStringField("Status").WithEnum("active", "inactive", "pending"),
	}, nil)

	validator := NewJSONValidator(&schema)

	tests := []struct {
		name       string
		json       string
		shouldPass bool
	}{
		{
			name:       "valid enum value - active",
			json:       `{"status": "active"}`,
			shouldPass: true,
		},
		{
			name:       "valid enum value - pending",
			json:       `{"status": "pending"}`,
			shouldPass: true,
		},
		{
			name:       "invalid enum value",
			json:       `{"status": "unknown"}`,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, "$.status", validationErr.Path)
				assert.Contains(t, validationErr.Message, "invalid enum")
			}
		})
	}
}

// TestValidate_FormatConstraints tests format validation
func TestValidate_FormatConstraints(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		value      string
		shouldPass bool
	}{
		{"valid email", "email", "user@example.com", true},
		{"invalid email", "email", "not-an-email", false},
		{"valid URI", "uri", "https://example.com/path", true},
		{"invalid URI", "uri", "not a uri", false},
		{"valid date-time", "date-time", "2024-01-15T10:30:00Z", true},
		{"invalid date-time", "date-time", "2024-01-15", false},
		{"valid UUID", "uuid", "550e8400-e29b-41d4-a716-446655440000", true},
		{"invalid UUID", "uuid", "not-a-uuid", false},
		{"valid date", "date", "2024-01-15", true},
		{"invalid date", "date", "01/15/2024", false},
		{"valid time", "time", "14:30:00", true},
		{"invalid time", "time", "2:30 PM", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewObjectSchema(map[string]SchemaField{
				"field": NewStringField("Field").WithFormat(tt.format),
			}, nil)

			validator := NewJSONValidator(&schema)
			json := fmt.Sprintf(`{"field": "%s"}`, tt.value)
			err := validator.Validate([]byte(json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, "$.field", validationErr.Path)
			}
		})
	}
}

// TestValidate_NestedObjects tests nested object validation
func TestValidate_NestedObjects(t *testing.T) {
	addressSchema := SchemaField{
		Type: "object",
		Properties: map[string]SchemaField{
			"street": NewStringField("Street"),
			"city":   NewStringField("City"),
			"zip":    NewStringField("ZIP").WithPattern("^[0-9]{5}$"),
		},
		Required: []string{"city"},
	}

	schema := NewObjectSchema(map[string]SchemaField{
		"name":    NewStringField("Name"),
		"address": addressSchema,
	}, []string{"name"})

	validator := NewJSONValidator(&schema)

	tests := []struct {
		name         string
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name: "valid nested object",
			json: `{
				"name": "John",
				"address": {
					"street": "123 Main St",
					"city": "Boston",
					"zip": "02101"
				}
			}`,
			shouldPass: true,
		},
		{
			name: "missing required nested field",
			json: `{
				"name": "John",
				"address": {
					"street": "123 Main St"
				}
			}`,
			shouldPass:   false,
			expectedPath: "$.address.city",
		},
		{
			name: "invalid nested field pattern",
			json: `{
				"name": "John",
				"address": {
					"city": "Boston",
					"zip": "invalid"
				}
			}`,
			shouldPass:   false,
			expectedPath: "$.address.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedPath, validationErr.Path)
			}
		})
	}
}

// TestValidate_Arrays tests array validation
func TestValidate_Arrays(t *testing.T) {
	tests := []struct {
		name         string
		schema       JSONSchema
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name: "valid array of strings",
			schema: NewObjectSchema(map[string]SchemaField{
				"tags": {
					Type:  "array",
					Items: &SchemaField{Type: "string"},
				},
			}, nil),
			json:       `{"tags": ["go", "testing", "validation"]}`,
			shouldPass: true,
		},
		{
			name: "invalid array item type",
			schema: NewObjectSchema(map[string]SchemaField{
				"tags": {
					Type:  "array",
					Items: &SchemaField{Type: "string"},
				},
			}, nil),
			json:         `{"tags": ["go", 123, "validation"]}`,
			shouldPass:   false,
			expectedPath: "$.tags[1]",
		},
		{
			name: "array with constraint validation",
			schema: NewObjectSchema(map[string]SchemaField{
				"scores": {
					Type: "array",
					Items: &SchemaField{
						Type:    "number",
						Minimum: float64Ptr(0),
						Maximum: float64Ptr(100),
					},
				},
			}, nil),
			json:         `{"scores": [95.5, 88.0, 150.0]}`,
			shouldPass:   false,
			expectedPath: "$.scores[2]",
		},
		{
			name: "array of objects",
			schema: NewObjectSchema(map[string]SchemaField{
				"users": {
					Type: "array",
					Items: &SchemaField{
						Type: "object",
						Properties: map[string]SchemaField{
							"name": NewStringField("Name"),
							"age":  NewIntegerField("Age").WithMin(0),
						},
						Required: []string{"name"},
					},
				},
			}, nil),
			json: `{
				"users": [
					{"name": "Alice", "age": 30},
					{"name": "Bob", "age": 25}
				]
			}`,
			shouldPass: true,
		},
		{
			name: "array of objects with missing required field",
			schema: NewObjectSchema(map[string]SchemaField{
				"users": {
					Type: "array",
					Items: &SchemaField{
						Type: "object",
						Properties: map[string]SchemaField{
							"name": NewStringField("Name"),
						},
						Required: []string{"name"},
					},
				},
			}, nil),
			json: `{
				"users": [
					{"name": "Alice"},
					{"age": 25}
				]
			}`,
			shouldPass:   false,
			expectedPath: "$.users[1].name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedPath, validationErr.Path)
			}
		})
	}
}

// TestValidate_AdditionalProperties tests additional properties validation
func TestValidate_AdditionalProperties(t *testing.T) {
	disallowAdditional := false
	schema := JSONSchema{
		Type: "object",
		Properties: map[string]SchemaField{
			"name": NewStringField("Name"),
		},
		AdditionalProperties: &disallowAdditional,
	}

	validator := NewJSONValidator(&schema)

	tests := []struct {
		name         string
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name:       "no additional properties",
			json:       `{"name": "John"}`,
			shouldPass: true,
		},
		{
			name:         "with additional property",
			json:         `{"name": "John", "age": 30}`,
			shouldPass:   false,
			expectedPath: "$.age",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, tt.expectedPath, validationErr.Path)
			}
		})
	}
}

// TestValidateAndUnmarshal tests validation with unmarshaling
func TestValidateAndUnmarshal(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}

	schema := NewObjectSchema(map[string]SchemaField{
		"name":  NewStringField("Name"),
		"email": NewStringField("Email").WithFormat("email"),
		"age":   NewIntegerField("Age").WithMinMax(0, 150),
	}, []string{"name", "email"})

	validator := NewJSONValidator(&schema)

	t.Run("valid data", func(t *testing.T) {
		validJSON := []byte(`{
			"name": "John Doe",
			"email": "john@example.com",
			"age": 30
		}`)

		var user User
		err := validator.ValidateAndUnmarshal(validJSON, &user)
		require.NoError(t, err)

		assert.Equal(t, "John Doe", user.Name)
		assert.Equal(t, "john@example.com", user.Email)
		assert.Equal(t, 30, user.Age)
	})

	t.Run("invalid data - validation fails before unmarshal", func(t *testing.T) {
		invalidJSON := []byte(`{
			"name": "John Doe",
			"email": "invalid-email",
			"age": 30
		}`)

		var user User
		err := validator.ValidateAndUnmarshal(invalidJSON, &user)
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.email", validationErr.Path)
	})

	t.Run("missing required field", func(t *testing.T) {
		invalidJSON := []byte(`{
			"name": "John Doe",
			"age": 30
		}`)

		var user User
		err := validator.ValidateAndUnmarshal(invalidJSON, &user)
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.email", validationErr.Path)
	})
}

// TestValidationError_Error tests error message formatting
func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      SchemaValidationError
		expected string
	}{
		{
			name: "with expected and actual",
			err: SchemaValidationError{
				Message:  "type mismatch",
				Path:     "$.age",
				Expected: "integer",
				Actual:   "string",
			},
			expected: "validation failed at $.age: type mismatch (expected integer, got string)",
		},
		{
			name: "without expected and actual",
			err: SchemaValidationError{
				Message: "required field is missing",
				Path:    "$.email",
			},
			expected: "validation failed at $.email: required field is missing",
		},
		{
			name: "nested path",
			err: SchemaValidationError{
				Message:  "invalid format",
				Path:     "$.user.address.zip",
				Expected: "5 digits",
				Actual:   "invalid",
			},
			expected: "validation failed at $.user.address.zip: invalid format (expected 5 digits, got invalid)",
		},
		{
			name: "array path",
			err: SchemaValidationError{
				Message:  "type mismatch",
				Path:     "$.users[2].age",
				Expected: "integer",
				Actual:   "string",
			},
			expected: "validation failed at $.users[2].age: type mismatch (expected integer, got string)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

// TestValidate_ComplexNestedStructure tests deeply nested validation
func TestValidate_ComplexNestedStructure(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"user": {
			Type: "object",
			Properties: map[string]SchemaField{
				"profile": {
					Type: "object",
					Properties: map[string]SchemaField{
						"contacts": {
							Type: "array",
							Items: &SchemaField{
								Type: "object",
								Properties: map[string]SchemaField{
									"type":  NewStringField("Type").WithEnum("email", "phone"),
									"value": NewStringField("Value"),
								},
								Required: []string{"type", "value"},
							},
						},
					},
				},
			},
			Required: []string{"profile"},
		},
	}, []string{"user"})

	validator := NewJSONValidator(&schema)

	t.Run("valid complex structure", func(t *testing.T) {
		json := `{
			"user": {
				"profile": {
					"contacts": [
						{"type": "email", "value": "john@example.com"},
						{"type": "phone", "value": "555-1234"}
					]
				}
			}
		}`

		err := validator.Validate([]byte(json))
		assert.NoError(t, err)
	})

	t.Run("invalid enum in nested array", func(t *testing.T) {
		json := `{
			"user": {
				"profile": {
					"contacts": [
						{"type": "email", "value": "john@example.com"},
						{"type": "fax", "value": "555-1234"}
					]
				}
			}
		}`

		err := validator.Validate([]byte(json))
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.user.profile.contacts[1].type", validationErr.Path)
	})

	t.Run("missing required field in nested array object", func(t *testing.T) {
		json := `{
			"user": {
				"profile": {
					"contacts": [
						{"type": "email", "value": "john@example.com"},
						{"type": "phone"}
					]
				}
			}
		}`

		err := validator.Validate([]byte(json))
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.user.profile.contacts[1].value", validationErr.Path)
	})
}

// TestValidate_EdgeCases tests edge cases like empty objects, empty arrays, null values
func TestValidate_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		schema       JSONSchema
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name: "empty object when properties are optional",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, nil),
			json:       `{}`,
			shouldPass: true,
		},
		{
			name: "empty object when properties are required",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, []string{"name"}),
			json:         `{}`,
			shouldPass:   false,
			expectedPath: "$.name",
		},
		{
			name: "empty array",
			schema: NewObjectSchema(map[string]SchemaField{
				"tags": {
					Type:  "array",
					Items: &SchemaField{Type: "string"},
				},
			}, nil),
			json:       `{"tags": []}`,
			shouldPass: true,
		},
		{
			name: "null value for optional field",
			schema: NewObjectSchema(map[string]SchemaField{
				"name":        NewStringField("Name"),
				"description": NewStringField("Description"),
			}, []string{"name"}),
			json:       `{"name": "test", "description": null}`,
			shouldPass: true,
		},
		{
			name: "empty string passes when no minLength",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, []string{"name"}),
			json:       `{"name": ""}`,
			shouldPass: true,
		},
		{
			name: "empty string fails when minLength set",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name").WithMinLength(1),
			}, []string{"name"}),
			json:         `{"name": ""}`,
			shouldPass:   false,
			expectedPath: "$.name",
		},
		{
			name: "zero value for number",
			schema: NewObjectSchema(map[string]SchemaField{
				"count": NewIntegerField("Count").WithMin(0),
			}, nil),
			json:       `{"count": 0}`,
			shouldPass: true,
		},
		{
			name: "false value for boolean",
			schema: NewObjectSchema(map[string]SchemaField{
				"enabled": NewBooleanField("Enabled"),
			}, nil),
			json:       `{"enabled": false}`,
			shouldPass: true,
		},
		{
			name: "deeply nested empty objects",
			schema: NewObjectSchema(map[string]SchemaField{
				"level1": {
					Type: "object",
					Properties: map[string]SchemaField{
						"level2": {
							Type: "object",
							Properties: map[string]SchemaField{
								"level3": {
									Type:       "object",
									Properties: map[string]SchemaField{},
								},
							},
						},
					},
				},
			}, nil),
			json:       `{"level1": {"level2": {"level3": {}}}}`,
			shouldPass: true,
		},
		{
			name: "array of empty objects",
			schema: NewObjectSchema(map[string]SchemaField{
				"items": {
					Type: "array",
					Items: &SchemaField{
						Type:       "object",
						Properties: map[string]SchemaField{},
					},
				},
			}, nil),
			json:       `{"items": [{}, {}, {}]}`,
			shouldPass: true,
		},
		{
			name: "nested array with empty inner arrays",
			schema: NewObjectSchema(map[string]SchemaField{
				"matrix": {
					Type: "array",
					Items: &SchemaField{
						Type: "array",
						Items: &SchemaField{
							Type: "number",
						},
					},
				},
			}, nil),
			json:       `{"matrix": [[], [1, 2], []]}`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				if tt.expectedPath != "" {
					assert.Equal(t, tt.expectedPath, validationErr.Path)
				}
			}
		})
	}
}

// TestValidate_RootLevelTypes tests validation of non-object root types
func TestValidate_RootLevelTypes(t *testing.T) {
	tests := []struct {
		name       string
		schema     JSONSchema
		json       string
		shouldPass bool
	}{
		{
			name:       "root level string",
			schema:     JSONSchema{Type: "string"},
			json:       `"hello world"`,
			shouldPass: true,
		},
		{
			name:       "root level number",
			schema:     JSONSchema{Type: "number"},
			json:       `42.5`,
			shouldPass: true,
		},
		{
			name:       "root level integer",
			schema:     JSONSchema{Type: "integer"},
			json:       `42`,
			shouldPass: true,
		},
		{
			name:       "root level boolean",
			schema:     JSONSchema{Type: "boolean"},
			json:       `true`,
			shouldPass: true,
		},
		{
			name: "root level array",
			schema: JSONSchema{
				Type:  "array",
				Items: &SchemaField{Type: "string"},
			},
			json:       `["a", "b", "c"]`,
			shouldPass: true,
		},
		{
			name:       "invalid root level type",
			schema:     JSONSchema{Type: "string"},
			json:       `123`,
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				assert.Equal(t, "$", validationErr.Path)
			}
		})
	}
}

// TestValidate_NullValues tests handling of null values in various contexts
func TestValidate_NullValues(t *testing.T) {
	tests := []struct {
		name         string
		schema       JSONSchema
		json         string
		shouldPass   bool
		expectedPath string
	}{
		{
			name: "null value for optional string field",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, nil),
			json:       `{"name": null}`,
			shouldPass: true,
		},
		{
			name: "null value in array",
			schema: NewObjectSchema(map[string]SchemaField{
				"tags": {
					Type:  "array",
					Items: &SchemaField{Type: "string"},
				},
			}, nil),
			json:         `{"tags": ["a", null, "c"]}`,
			shouldPass:   false,
			expectedPath: "$.tags[1]",
		},
		{
			name: "null value for nested object",
			schema: NewObjectSchema(map[string]SchemaField{
				"address": {
					Type: "object",
					Properties: map[string]SchemaField{
						"city": NewStringField("City"),
					},
				},
			}, nil),
			json:       `{"address": null}`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewJSONValidator(&tt.schema)
			err := validator.Validate([]byte(tt.json))

			if tt.shouldPass {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				validationErr, ok := err.(*SchemaValidationError)
				require.True(t, ok)
				if tt.expectedPath != "" {
					assert.Equal(t, tt.expectedPath, validationErr.Path)
				}
			}
		})
	}
}

// TestValidate_DeeplyNestedArrays tests validation of deeply nested array structures
func TestValidate_DeeplyNestedArrays(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"organizations": {
			Type: "array",
			Items: &SchemaField{
				Type: "object",
				Properties: map[string]SchemaField{
					"name": NewStringField("Name"),
					"departments": {
						Type: "array",
						Items: &SchemaField{
							Type: "object",
							Properties: map[string]SchemaField{
								"name": NewStringField("Department Name"),
								"teams": {
									Type: "array",
									Items: &SchemaField{
										Type: "object",
										Properties: map[string]SchemaField{
											"name": NewStringField("Team Name"),
											"members": {
												Type: "array",
												Items: &SchemaField{
													Type: "object",
													Properties: map[string]SchemaField{
														"name":  NewStringField("Member Name"),
														"email": NewStringField("Email").WithFormat("email"),
													},
													Required: []string{"name", "email"},
												},
											},
										},
										Required: []string{"name"},
									},
								},
							},
							Required: []string{"name"},
						},
					},
				},
				Required: []string{"name"},
			},
		},
	}, nil)

	validator := NewJSONValidator(&schema)

	t.Run("valid deeply nested structure", func(t *testing.T) {
		json := `{
			"organizations": [
				{
					"name": "TechCorp",
					"departments": [
						{
							"name": "Engineering",
							"teams": [
								{
									"name": "Backend",
									"members": [
										{"name": "Alice", "email": "alice@tech.com"},
										{"name": "Bob", "email": "bob@tech.com"}
									]
								}
							]
						}
					]
				}
			]
		}`

		err := validator.Validate([]byte(json))
		assert.NoError(t, err)
	})

	t.Run("error at deep nesting level", func(t *testing.T) {
		json := `{
			"organizations": [
				{
					"name": "TechCorp",
					"departments": [
						{
							"name": "Engineering",
							"teams": [
								{
									"name": "Backend",
									"members": [
										{"name": "Alice", "email": "alice@tech.com"},
										{"name": "Bob", "email": "invalid-email"}
									]
								}
							]
						}
					]
				}
			]
		}`

		err := validator.Validate([]byte(json))
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.organizations[0].departments[0].teams[0].members[1].email", validationErr.Path)
	})

	t.Run("missing required field in deep structure", func(t *testing.T) {
		json := `{
			"organizations": [
				{
					"name": "TechCorp",
					"departments": [
						{
							"name": "Engineering",
							"teams": [
								{
									"members": []
								}
							]
						}
					]
				}
			]
		}`

		err := validator.Validate([]byte(json))
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.organizations[0].departments[0].teams[0].name", validationErr.Path)
	})
}

// TestValidate_MultipleTypes tests scenarios with mixed types
func TestValidate_MultipleTypes(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"data": {
			Type: "array",
			Items: &SchemaField{
				Type: "object",
				Properties: map[string]SchemaField{
					"id":     NewIntegerField("ID"),
					"name":   NewStringField("Name"),
					"active": NewBooleanField("Active"),
					"score":  NewNumberField("Score"),
				},
				Required: []string{"id", "name"},
			},
		},
	}, []string{"data"})

	validator := NewJSONValidator(&schema)

	t.Run("valid mixed types", func(t *testing.T) {
		json := `{
			"data": [
				{"id": 1, "name": "Item 1", "active": true, "score": 95.5},
				{"id": 2, "name": "Item 2", "active": false, "score": 87.0},
				{"id": 3, "name": "Item 3"}
			]
		}`

		err := validator.Validate([]byte(json))
		assert.NoError(t, err)
	})

	t.Run("type mismatch in array element", func(t *testing.T) {
		json := `{
			"data": [
				{"id": 1, "name": "Item 1"},
				{"id": "two", "name": "Item 2"}
			]
		}`

		err := validator.Validate([]byte(json))
		require.Error(t, err)

		validationErr, ok := err.(*SchemaValidationError)
		require.True(t, ok)
		assert.Equal(t, "$.data[1].id", validationErr.Path)
		assert.Equal(t, "integer", validationErr.Expected)
		assert.Equal(t, "string", validationErr.Actual)
	})
}
