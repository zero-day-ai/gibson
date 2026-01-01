package schema_test

import (
	"encoding/json"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/schema"
)

// ExampleNewObjectSchema demonstrates creating a JSON Schema for a user object
func ExampleNewObjectSchema() {
	userSchema := schema.NewObjectSchema(map[string]schema.SchemaField{
		"username": schema.NewStringField("User's username").
			WithMinLength(3).
			WithMaxLength(20).
			WithPattern("^[a-zA-Z0-9_]+$"),
		"email": schema.NewStringField("User's email address").
			WithFormat("email"),
		"age": schema.NewIntegerField("User's age").
			WithMinMax(0, 150),
		"status": schema.NewStringField("Account status").
			WithEnum("active", "inactive", "suspended"),
	}, []string{"username", "email"})

	// Serialize to JSON
	jsonData, _ := json.MarshalIndent(userSchema, "", "  ")
	fmt.Println(string(jsonData))
}

// ExampleValidator demonstrates schema validation
func ExampleValidator() {
	// Define schema
	userSchema := schema.NewObjectSchema(map[string]schema.SchemaField{
		"username": schema.NewStringField("Username").WithMinLength(3),
		"email":    schema.NewStringField("Email").WithFormat("email"),
		"age":      schema.NewIntegerField("Age").WithMinMax(0, 150),
	}, []string{"username", "email"})

	// Create validator
	validator := schema.NewValidator()

	// Valid data
	validData := map[string]any{
		"username": "john_doe",
		"email":    "john@example.com",
		"age":      30,
	}

	errors := validator.Validate(userSchema, validData)
	fmt.Printf("Valid data errors: %d\n", len(errors))

	// Invalid data
	invalidData := map[string]any{
		"username": "ab",           // Too short
		"email":    "not-an-email", // Invalid format
		"age":      200,            // Above maximum
	}

	errors = validator.Validate(userSchema, invalidData)
	fmt.Printf("Invalid data errors: %d\n", len(errors))
	for _, err := range errors {
		fmt.Printf("  - %s: %s\n", err.Field, err.Message)
	}

	// Output:
	// Valid data errors: 0
	// Invalid data errors: 3
	//   - username: string length must be at least 3
	//   - email: invalid email format
	//   - age: value must be at most 150
}

// ExampleNewArraySchema demonstrates creating an array schema
func ExampleNewArraySchema() {
	validator := schema.NewValidator()

	// Create object schema with array field
	postSchema := schema.NewObjectSchema(map[string]schema.SchemaField{
		"title": schema.NewStringField("Post title"),
		"tags": {
			Type: "array",
			Items: &schema.SchemaField{
				Type:      "string",
				MinLength: intPtr(1),
				MaxLength: intPtr(50),
			},
		},
	}, []string{"title"})

	validData := map[string]any{
		"title": "My Post",
		"tags":  []any{"golang", "testing", "validation"},
	}

	errors := validator.Validate(postSchema, validData)
	fmt.Printf("Errors: %d\n", len(errors))

	// Output:
	// Errors: 0
}

func intPtr(v int) *int {
	return &v
}

// ExampleSchemaField_WithEnum demonstrates enum validation
func ExampleSchemaField_WithEnum() {
	prioritySchema := schema.NewObjectSchema(map[string]schema.SchemaField{
		"priority": schema.NewStringField("Task priority").
			WithEnum("low", "medium", "high", "urgent"),
	}, []string{"priority"})

	validator := schema.NewValidator()

	// Valid enum value
	validData := map[string]any{"priority": "high"}
	errors := validator.Validate(prioritySchema, validData)
	fmt.Printf("Valid enum - Errors: %d\n", len(errors))

	// Invalid enum value
	invalidData := map[string]any{"priority": "critical"}
	errors = validator.Validate(prioritySchema, invalidData)
	fmt.Printf("Invalid enum - Errors: %d\n", len(errors))

	// Output:
	// Valid enum - Errors: 0
	// Invalid enum - Errors: 1
}

// ExampleValidator_nestedObjects demonstrates nested object validation
func ExampleValidator_nestedObjects() {
	addressSchema := schema.SchemaField{
		Type: "object",
		Properties: map[string]schema.SchemaField{
			"street": schema.NewStringField("Street address"),
			"city":   schema.NewStringField("City"),
			"zip":    schema.NewStringField("ZIP code").WithPattern("^[0-9]{5}$"),
		},
		Required: []string{"city"},
	}

	userSchema := schema.NewObjectSchema(map[string]schema.SchemaField{
		"name":    schema.NewStringField("Name"),
		"address": addressSchema,
	}, []string{"name"})

	validator := schema.NewValidator()

	// Missing nested required field
	data := map[string]any{
		"name": "John Doe",
		"address": map[string]any{
			"street": "123 Main St",
			// Missing "city"
		},
	}

	errors := validator.Validate(userSchema, data)
	for _, err := range errors {
		fmt.Printf("%s: %s\n", err.Field, err.Message)
	}

	// Output:
	// address.city: required field is missing
}
