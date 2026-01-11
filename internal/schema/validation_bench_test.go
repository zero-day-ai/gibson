package schema

import (
	"testing"
)

// BenchmarkJSONValidator_Validate benchmarks validation performance
func BenchmarkJSONValidator_Validate(b *testing.B) {
	schema := NewObjectSchema(map[string]SchemaField{
		"name":  NewStringField("Name").WithMinLength(1).WithMaxLength(100),
		"email": NewStringField("Email").WithFormat("email"),
		"age":   NewIntegerField("Age").WithMinMax(0, 150),
		"address": {
			Type: "object",
			Properties: map[string]SchemaField{
				"street": NewStringField("Street"),
				"city":   NewStringField("City"),
				"zip":    NewStringField("ZIP").WithPattern("^[0-9]{5}$"),
			},
			Required: []string{"city"},
		},
	}, []string{"name", "email"})

	validator := NewJSONValidator(&schema)

	validJSON := []byte(`{
		"name": "John Doe",
		"email": "john@example.com",
		"age": 30,
		"address": {
			"street": "123 Main St",
			"city": "Boston",
			"zip": "02101"
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(validJSON)
	}
}

// BenchmarkJSONValidator_ValidateAndUnmarshal benchmarks validation with unmarshaling
func BenchmarkJSONValidator_ValidateAndUnmarshal(b *testing.B) {
	type Address struct {
		Street string `json:"street"`
		City   string `json:"city"`
		ZIP    string `json:"zip"`
	}

	type User struct {
		Name    string  `json:"name"`
		Email   string  `json:"email"`
		Age     int     `json:"age"`
		Address Address `json:"address"`
	}

	schema := NewObjectSchema(map[string]SchemaField{
		"name":  NewStringField("Name").WithMinLength(1).WithMaxLength(100),
		"email": NewStringField("Email").WithFormat("email"),
		"age":   NewIntegerField("Age").WithMinMax(0, 150),
		"address": {
			Type: "object",
			Properties: map[string]SchemaField{
				"street": NewStringField("Street"),
				"city":   NewStringField("City"),
				"zip":    NewStringField("ZIP").WithPattern("^[0-9]{5}$"),
			},
			Required: []string{"city"},
		},
	}, []string{"name", "email"})

	validator := NewJSONValidator(&schema)

	validJSON := []byte(`{
		"name": "John Doe",
		"email": "john@example.com",
		"age": 30,
		"address": {
			"street": "123 Main St",
			"city": "Boston",
			"zip": "02101"
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var user User
		_ = validator.ValidateAndUnmarshal(validJSON, &user)
	}
}

// BenchmarkJSONValidator_ComplexNested benchmarks validation with complex nested structures
func BenchmarkJSONValidator_ComplexNested(b *testing.B) {
	schema := NewObjectSchema(map[string]SchemaField{
		"users": {
			Type: "array",
			Items: &SchemaField{
				Type: "object",
				Properties: map[string]SchemaField{
					"name":  NewStringField("Name"),
					"email": NewStringField("Email").WithFormat("email"),
					"tags":  {Type: "array", Items: &SchemaField{Type: "string"}},
					"profile": {
						Type: "object",
						Properties: map[string]SchemaField{
							"bio": NewStringField("Bio").WithMaxLength(500),
							"age": NewIntegerField("Age").WithMinMax(0, 150),
						},
					},
				},
				Required: []string{"name", "email"},
			},
		},
	}, []string{"users"})

	validator := NewJSONValidator(&schema)

	complexJSON := []byte(`{
		"users": [
			{
				"name": "John Doe",
				"email": "john@example.com",
				"tags": ["admin", "user"],
				"profile": {
					"bio": "Software engineer",
					"age": 30
				}
			},
			{
				"name": "Jane Smith",
				"email": "jane@example.com",
				"tags": ["user"],
				"profile": {
					"bio": "Product manager",
					"age": 28
				}
			}
		]
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(complexJSON)
	}
}
