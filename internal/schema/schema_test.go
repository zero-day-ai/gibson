package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewObjectSchema tests object schema creation
func TestNewObjectSchema(t *testing.T) {
	properties := map[string]SchemaField{
		"name": NewStringField("User name"),
		"age":  NewIntegerField("User age"),
	}
	required := []string{"name"}

	schema := NewObjectSchema(properties, required)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, properties, schema.Properties)
	assert.Equal(t, required, schema.Required)
}

// TestNewArraySchema tests array schema creation
func TestNewArraySchema(t *testing.T) {
	items := NewStringField("Array item")
	schema := NewArraySchema(items)

	assert.Equal(t, "array", schema.Type)
	require.NotNil(t, schema.Items)
	assert.Equal(t, "string", schema.Items.Type)
}

// TestFieldConstructors tests all field constructor functions
func TestFieldConstructors(t *testing.T) {
	tests := []struct {
		name         string
		field        SchemaField
		expectedType string
		description  string
	}{
		{
			name:         "string field",
			field:        NewStringField("test string"),
			expectedType: "string",
			description:  "test string",
		},
		{
			name:         "integer field",
			field:        NewIntegerField("test integer"),
			expectedType: "integer",
			description:  "test integer",
		},
		{
			name:         "number field",
			field:        NewNumberField("test number"),
			expectedType: "number",
			description:  "test number",
		},
		{
			name:         "boolean field",
			field:        NewBooleanField("test boolean"),
			expectedType: "boolean",
			description:  "test boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedType, tt.field.Type)
			assert.Equal(t, tt.description, tt.field.Description)
		})
	}
}

// TestSchemaFieldBuilders tests field builder methods
func TestSchemaFieldBuilders(t *testing.T) {
	t.Run("WithEnum", func(t *testing.T) {
		field := NewStringField("status").WithEnum("active", "inactive", "pending")
		assert.Equal(t, []string{"active", "inactive", "pending"}, field.Enum)
	})

	t.Run("WithMinMax", func(t *testing.T) {
		field := NewNumberField("age").WithMinMax(0, 120)
		require.NotNil(t, field.Minimum)
		require.NotNil(t, field.Maximum)
		assert.Equal(t, 0.0, *field.Minimum)
		assert.Equal(t, 120.0, *field.Maximum)
	})

	t.Run("WithMin", func(t *testing.T) {
		field := NewNumberField("price").WithMin(0)
		require.NotNil(t, field.Minimum)
		assert.Equal(t, 0.0, *field.Minimum)
	})

	t.Run("WithMax", func(t *testing.T) {
		field := NewNumberField("score").WithMax(100)
		require.NotNil(t, field.Maximum)
		assert.Equal(t, 100.0, *field.Maximum)
	})

	t.Run("WithPattern", func(t *testing.T) {
		field := NewStringField("email").WithPattern("^[a-z]+@[a-z]+\\.[a-z]+$")
		assert.Equal(t, "^[a-z]+@[a-z]+\\.[a-z]+$", field.Pattern)
	})

	t.Run("WithFormat", func(t *testing.T) {
		field := NewStringField("email").WithFormat("email")
		assert.Equal(t, "email", field.Format)
	})

	t.Run("WithMinLength", func(t *testing.T) {
		field := NewStringField("password").WithMinLength(8)
		require.NotNil(t, field.MinLength)
		assert.Equal(t, 8, *field.MinLength)
	})

	t.Run("WithMaxLength", func(t *testing.T) {
		field := NewStringField("username").WithMaxLength(20)
		require.NotNil(t, field.MaxLength)
		assert.Equal(t, 20, *field.MaxLength)
	})

	t.Run("WithDefault", func(t *testing.T) {
		field := NewStringField("role").WithDefault("user")
		assert.Equal(t, "user", field.Default)
	})

	t.Run("WithDescription", func(t *testing.T) {
		field := NewStringField("").WithDescription("new description")
		assert.Equal(t, "new description", field.Description)
	})

	t.Run("chaining builders", func(t *testing.T) {
		field := NewStringField("username").
			WithMinLength(3).
			WithMaxLength(20).
			WithPattern("^[a-zA-Z0-9_]+$").
			WithDescription("alphanumeric username")

		require.NotNil(t, field.MinLength)
		require.NotNil(t, field.MaxLength)
		assert.Equal(t, 3, *field.MinLength)
		assert.Equal(t, 20, *field.MaxLength)
		assert.Equal(t, "^[a-zA-Z0-9_]+$", field.Pattern)
		assert.Equal(t, "alphanumeric username", field.Description)
	})
}

// TestSchemaJSONSerialization tests JSON Schema draft-07 compatibility
func TestSchemaJSONSerialization(t *testing.T) {
	schema := NewObjectSchema(map[string]SchemaField{
		"name":   NewStringField("User name").WithMinLength(1).WithMaxLength(100),
		"age":    NewIntegerField("User age").WithMinMax(0, 150),
		"email":  NewStringField("User email").WithFormat("email"),
		"status": NewStringField("User status").WithEnum("active", "inactive"),
	}, []string{"name", "email"})

	jsonData, err := json.Marshal(schema)
	require.NoError(t, err)

	// Unmarshal to verify structure
	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "object", decoded["type"])
	assert.NotNil(t, decoded["properties"])
	assert.NotNil(t, decoded["required"])

	required := decoded["required"].([]any)
	assert.Len(t, required, 2)
	assert.Contains(t, required, "name")
	assert.Contains(t, required, "email")
}

// TestValidateRequiredFields tests required field validation
func TestValidateRequiredFields(t *testing.T) {
	validator := NewValidator()
	schema := NewObjectSchema(map[string]SchemaField{
		"name":  NewStringField("Name"),
		"email": NewStringField("Email"),
	}, []string{"name", "email"})

	tests := []struct {
		name           string
		data           map[string]any
		expectedErrors int
		missingFields  []string
	}{
		{
			name: "all required fields present",
			data: map[string]any{
				"name":  "John",
				"email": "john@example.com",
			},
			expectedErrors: 0,
		},
		{
			name: "missing one required field",
			data: map[string]any{
				"name": "John",
			},
			expectedErrors: 1,
			missingFields:  []string{"email"},
		},
		{
			name:           "missing all required fields",
			data:           map[string]any{},
			expectedErrors: 2,
			missingFields:  []string{"name", "email"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)

			for _, field := range tt.missingFields {
				found := false
				for _, err := range errors {
					if err.Field == field && err.Message == "required field is missing" {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error for missing field: %s", field)
			}
		})
	}
}

// TestValidateTypeChecking tests type validation
func TestValidateTypeChecking(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name           string
		schema         JSONSchema
		data           map[string]any
		expectedErrors int
	}{
		{
			name: "correct string type",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, nil),
			data: map[string]any{
				"name": "John",
			},
			expectedErrors: 0,
		},
		{
			name: "incorrect string type",
			schema: NewObjectSchema(map[string]SchemaField{
				"name": NewStringField("Name"),
			}, nil),
			data: map[string]any{
				"name": 123,
			},
			expectedErrors: 1,
		},
		{
			name: "correct integer type",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age"),
			}, nil),
			data: map[string]any{
				"age": 25,
			},
			expectedErrors: 0,
		},
		{
			name: "integer as float",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age"),
			}, nil),
			data: map[string]any{
				"age": 25.0, // Whole number as float should pass
			},
			expectedErrors: 0,
		},
		{
			name: "decimal for integer type",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age"),
			}, nil),
			data: map[string]any{
				"age": 25.5,
			},
			expectedErrors: 1,
		},
		{
			name: "correct boolean type",
			schema: NewObjectSchema(map[string]SchemaField{
				"active": NewBooleanField("Active"),
			}, nil),
			data: map[string]any{
				"active": true,
			},
			expectedErrors: 0,
		},
		{
			name: "incorrect boolean type",
			schema: NewObjectSchema(map[string]SchemaField{
				"active": NewBooleanField("Active"),
			}, nil),
			data: map[string]any{
				"active": "true",
			},
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(tt.schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)
		})
	}
}

// TestValidateNumericConstraints tests numeric validation
func TestValidateNumericConstraints(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name           string
		schema         JSONSchema
		data           map[string]any
		expectedErrors int
	}{
		{
			name: "number within min-max range",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age").WithMinMax(0, 150),
			}, nil),
			data: map[string]any{
				"age": 25,
			},
			expectedErrors: 0,
		},
		{
			name: "number below minimum",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age").WithMin(0),
			}, nil),
			data: map[string]any{
				"age": -5,
			},
			expectedErrors: 1,
		},
		{
			name: "number above maximum",
			schema: NewObjectSchema(map[string]SchemaField{
				"age": NewIntegerField("Age").WithMax(150),
			}, nil),
			data: map[string]any{
				"age": 200,
			},
			expectedErrors: 1,
		},
		{
			name: "number at minimum boundary",
			schema: NewObjectSchema(map[string]SchemaField{
				"score": NewNumberField("Score").WithMin(0),
			}, nil),
			data: map[string]any{
				"score": 0.0,
			},
			expectedErrors: 0,
		},
		{
			name: "number at maximum boundary",
			schema: NewObjectSchema(map[string]SchemaField{
				"score": NewNumberField("Score").WithMax(100),
			}, nil),
			data: map[string]any{
				"score": 100.0,
			},
			expectedErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(tt.schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)
		})
	}
}

// TestValidateStringConstraints tests string validation
func TestValidateStringConstraints(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name           string
		schema         JSONSchema
		data           map[string]any
		expectedErrors int
	}{
		{
			name: "string within length constraints",
			schema: NewObjectSchema(map[string]SchemaField{
				"username": NewStringField("Username").WithMinLength(3).WithMaxLength(20),
			}, nil),
			data: map[string]any{
				"username": "johndoe",
			},
			expectedErrors: 0,
		},
		{
			name: "string too short",
			schema: NewObjectSchema(map[string]SchemaField{
				"password": NewStringField("Password").WithMinLength(8),
			}, nil),
			data: map[string]any{
				"password": "short",
			},
			expectedErrors: 1,
		},
		{
			name: "string too long",
			schema: NewObjectSchema(map[string]SchemaField{
				"bio": NewStringField("Bio").WithMaxLength(100),
			}, nil),
			data: map[string]any{
				"bio": string(make([]byte, 101)),
			},
			expectedErrors: 1,
		},
		{
			name: "string matches pattern",
			schema: NewObjectSchema(map[string]SchemaField{
				"username": NewStringField("Username").WithPattern("^[a-z0-9_]+$"),
			}, nil),
			data: map[string]any{
				"username": "john_doe123",
			},
			expectedErrors: 0,
		},
		{
			name: "string does not match pattern",
			schema: NewObjectSchema(map[string]SchemaField{
				"username": NewStringField("Username").WithPattern("^[a-z0-9_]+$"),
			}, nil),
			data: map[string]any{
				"username": "john-doe",
			},
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(tt.schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)
		})
	}
}

// TestValidateEnumConstraint tests enum validation
func TestValidateEnumConstraint(t *testing.T) {
	validator := NewValidator()
	schema := NewObjectSchema(map[string]SchemaField{
		"status": NewStringField("Status").WithEnum("active", "inactive", "pending"),
	}, nil)

	tests := []struct {
		name           string
		data           map[string]any
		expectedErrors int
	}{
		{
			name: "valid enum value",
			data: map[string]any{
				"status": "active",
			},
			expectedErrors: 0,
		},
		{
			name: "invalid enum value",
			data: map[string]any{
				"status": "unknown",
			},
			expectedErrors: 1,
		},
		{
			name: "another valid enum value",
			data: map[string]any{
				"status": "pending",
			},
			expectedErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)
		})
	}
}

// TestValidateFormatConstraints tests format validation
func TestValidateFormatConstraints(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		format     string
		value      string
		shouldPass bool
	}{
		{
			name:       "valid email",
			format:     "email",
			value:      "user@example.com",
			shouldPass: true,
		},
		{
			name:       "invalid email",
			format:     "email",
			value:      "not-an-email",
			shouldPass: false,
		},
		{
			name:       "valid URI",
			format:     "uri",
			value:      "https://example.com/path",
			shouldPass: true,
		},
		{
			name:       "invalid URI",
			format:     "uri",
			value:      "not a uri",
			shouldPass: false,
		},
		{
			name:       "valid date-time",
			format:     "date-time",
			value:      "2024-01-15T10:30:00Z",
			shouldPass: true,
		},
		{
			name:       "invalid date-time",
			format:     "date-time",
			value:      "2024-01-15",
			shouldPass: false,
		},
		{
			name:       "valid UUID",
			format:     "uuid",
			value:      "550e8400-e29b-41d4-a716-446655440000",
			shouldPass: true,
		},
		{
			name:       "invalid UUID",
			format:     "uuid",
			value:      "not-a-uuid",
			shouldPass: false,
		},
		{
			name:       "valid date",
			format:     "date",
			value:      "2024-01-15",
			shouldPass: true,
		},
		{
			name:       "invalid date",
			format:     "date",
			value:      "01/15/2024",
			shouldPass: false,
		},
		{
			name:       "valid time",
			format:     "time",
			value:      "14:30:00",
			shouldPass: true,
		},
		{
			name:       "invalid time",
			format:     "time",
			value:      "2:30 PM",
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewObjectSchema(map[string]SchemaField{
				"field": NewStringField("Field").WithFormat(tt.format),
			}, nil)

			errors := validator.Validate(schema, map[string]any{
				"field": tt.value,
			})

			if tt.shouldPass {
				assert.Len(t, errors, 0, "Expected no errors for valid %s: %s", tt.format, tt.value)
			} else {
				assert.Greater(t, len(errors), 0, "Expected errors for invalid %s: %s", tt.format, tt.value)
			}
		})
	}
}

// TestValidateNestedObject tests nested object validation
func TestValidateNestedObject(t *testing.T) {
	validator := NewValidator()

	addressField := SchemaField{
		Type: "object",
		Properties: map[string]SchemaField{
			"street": NewStringField("Street address"),
			"city":   NewStringField("City"),
			"zip":    NewStringField("ZIP code").WithPattern("^[0-9]{5}$"),
		},
		Required: []string{"city"},
	}

	schema := NewObjectSchema(map[string]SchemaField{
		"name":    NewStringField("Name"),
		"address": addressField,
	}, []string{"name"})

	tests := []struct {
		name           string
		data           map[string]any
		expectedErrors int
		errorFields    []string
	}{
		{
			name: "valid nested object",
			data: map[string]any{
				"name": "John",
				"address": map[string]any{
					"street": "123 Main St",
					"city":   "Boston",
					"zip":    "02101",
				},
			},
			expectedErrors: 0,
		},
		{
			name: "missing required nested field",
			data: map[string]any{
				"name": "John",
				"address": map[string]any{
					"street": "123 Main St",
				},
			},
			expectedErrors: 1,
			errorFields:    []string{"address.city"},
		},
		{
			name: "invalid nested field pattern",
			data: map[string]any{
				"name": "John",
				"address": map[string]any{
					"city": "Boston",
					"zip":  "invalid",
				},
			},
			expectedErrors: 1,
			errorFields:    []string{"address.zip"},
		},
		{
			name: "multiple nested errors",
			data: map[string]any{
				"name": "John",
				"address": map[string]any{
					"zip": "invalid",
				},
			},
			expectedErrors: 2,
			errorFields:    []string{"address.city", "address.zip"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)

			for _, expectedField := range tt.errorFields {
				found := false
				for _, err := range errors {
					if err.Field == expectedField {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected error for field: %s", expectedField)
			}
		})
	}
}

// TestValidateArray tests array validation
func TestValidateArray(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name           string
		schema         JSONSchema
		data           map[string]any
		expectedErrors int
	}{
		{
			name: "valid array of strings",
			schema: NewObjectSchema(map[string]SchemaField{
				"tags": {
					Type:  "array",
					Items: &SchemaField{Type: "string"},
				},
			}, nil),
			data: map[string]any{
				"tags": []any{"go", "testing", "validation"},
			},
			expectedErrors: 0,
		},
		{
			name: "invalid array item type",
			schema: NewObjectSchema(map[string]SchemaField{
				"tags": {
					Type:  "array",
					Items: &SchemaField{Type: "string"},
				},
			}, nil),
			data: map[string]any{
				"tags": []any{"go", 123, "validation"},
			},
			expectedErrors: 1,
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
			data: map[string]any{
				"scores": []any{95.5, 88.0, 150.0},
			},
			expectedErrors: 1,
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
			data: map[string]any{
				"users": []any{
					map[string]any{"name": "Alice", "age": 30},
					map[string]any{"name": "Bob", "age": 25},
				},
			},
			expectedErrors: 0,
		},
		{
			name: "array of objects with error",
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
			data: map[string]any{
				"users": []any{
					map[string]any{"name": "Alice"},
					map[string]any{"age": 25}, // missing required name
				},
			},
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(tt.schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)
		})
	}
}

// TestValidateAdditionalProperties tests additional properties validation
func TestValidateAdditionalProperties(t *testing.T) {
	validator := NewValidator()

	disallowAdditional := false
	schema := JSONSchema{
		Type: "object",
		Properties: map[string]SchemaField{
			"name": NewStringField("Name"),
		},
		AdditionalProperties: &disallowAdditional,
	}

	tests := []struct {
		name           string
		data           map[string]any
		expectedErrors int
	}{
		{
			name: "no additional properties",
			data: map[string]any{
				"name": "John",
			},
			expectedErrors: 0,
		},
		{
			name: "with additional property",
			data: map[string]any{
				"name": "John",
				"age":  30,
			},
			expectedErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(schema, tt.data)
			assert.Len(t, errors, tt.expectedErrors)
		})
	}
}

// TestValidationErrorMessages tests error message formatting
func TestValidationErrorMessages(t *testing.T) {
	validator := NewValidator()

	schema := NewObjectSchema(map[string]SchemaField{
		"user": {
			Type: "object",
			Properties: map[string]SchemaField{
				"profile": {
					Type: "object",
					Properties: map[string]SchemaField{
						"email": NewStringField("Email").WithFormat("email"),
					},
					Required: []string{"email"},
				},
			},
			Required: []string{"profile"},
		},
	}, []string{"user"})

	tests := []struct {
		name          string
		data          map[string]any
		expectedField string
	}{
		{
			name:          "missing root field",
			data:          map[string]any{},
			expectedField: "user",
		},
		{
			name: "missing nested field",
			data: map[string]any{
				"user": map[string]any{
					"profile": map[string]any{},
				},
			},
			expectedField: "user.profile.email",
		},
		{
			name: "invalid nested field format",
			data: map[string]any{
				"user": map[string]any{
					"profile": map[string]any{
						"email": "invalid-email",
					},
				},
			},
			expectedField: "user.profile.email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.Validate(schema, tt.data)
			require.Greater(t, len(errors), 0, "Expected at least one error")

			found := false
			for _, err := range errors {
				if err.Field == tt.expectedField {
					found = true
					assert.NotEmpty(t, err.Message, "Error message should not be empty")
					break
				}
			}
			assert.True(t, found, "Expected error field path: %s", tt.expectedField)
		})
	}
}

// TestMultipleValidationErrors tests that all errors are returned
func TestMultipleValidationErrors(t *testing.T) {
	validator := NewValidator()

	schema := NewObjectSchema(map[string]SchemaField{
		"username": NewStringField("Username").WithMinLength(3).WithMaxLength(20).WithPattern("^[a-z]+$"),
		"age":      NewIntegerField("Age").WithMinMax(0, 150),
		"email":    NewStringField("Email").WithFormat("email"),
		"status":   NewStringField("Status").WithEnum("active", "inactive"),
	}, []string{"username", "email"})

	// Data with multiple errors
	data := map[string]any{
		"username": "AB",        // Too short, contains uppercase
		"age":      200,         // Above maximum
		"email":    "not-email", // Invalid format
		"status":   "unknown",   // Invalid enum
	}

	errors := validator.Validate(schema, data)

	// Should have multiple errors (at least 4)
	assert.GreaterOrEqual(t, len(errors), 4, "Should return all validation errors")

	// Check that different fields have errors
	errorFields := make(map[string]bool)
	for _, err := range errors {
		errorFields[err.Field] = true
	}

	assert.True(t, errorFields["username"], "Should have username error")
	assert.True(t, errorFields["age"], "Should have age error")
	assert.True(t, errorFields["email"], "Should have email error")
	assert.True(t, errorFields["status"], "Should have status error")
}

// Helper function to create float64 pointer
func float64Ptr(v float64) *float64 {
	return &v
}
