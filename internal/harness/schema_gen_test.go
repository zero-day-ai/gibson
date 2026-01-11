package harness

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/schema"
)

// Test types
type SimpleStruct struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Email string `json:"email,omitempty"`
}

type NestedStruct struct {
	User    SimpleStruct `json:"user"`
	Active  bool         `json:"active"`
	Score   float64      `json:"score,omitempty"`
	TagsPtr *[]string    `json:"tags,omitempty"`
}

type EmbeddedBase struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type WithEmbedded struct {
	EmbeddedBase
	Title string `json:"title"`
}

type CollectionTypes struct {
	Tags     []string          `json:"tags"`
	Scores   []int             `json:"scores"`
	Metadata map[string]string `json:"metadata"`
}

type PointerFields struct {
	Name     *string  `json:"name,omitempty"`
	Age      *int     `json:"age,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
	Score    *float64 `json:"score,omitempty"`
}

type ComplexNested struct {
	Items []NestedStruct         `json:"items"`
	Meta  map[string]interface{} `json:"meta"`
}

type WithIgnoredFields struct {
	Public  string `json:"public"`
	Skipped string `json:"-"`
	private string // unexported, should be ignored
}

func TestSchemaFromType_Simple(t *testing.T) {
	schema := SchemaFromType[SimpleStruct]()

	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got '%s'", schema.Type)
	}

	if len(schema.Properties) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(schema.Properties))
	}

	// Check name field
	nameField, ok := schema.Properties["name"]
	if !ok {
		t.Error("Expected 'name' property")
	}
	if nameField.Type != "string" {
		t.Errorf("Expected name type 'string', got '%s'", nameField.Type)
	}

	// Check age field
	ageField, ok := schema.Properties["age"]
	if !ok {
		t.Error("Expected 'age' property")
	}
	if ageField.Type != "integer" {
		t.Errorf("Expected age type 'integer', got '%s'", ageField.Type)
	}

	// Check email field
	emailField, ok := schema.Properties["email"]
	if !ok {
		t.Error("Expected 'email' property")
	}
	if emailField.Type != "string" {
		t.Errorf("Expected email type 'string', got '%s'", emailField.Type)
	}

	// Check required fields - name and age should be required, email should not (omitempty)
	if len(schema.Required) != 2 {
		t.Errorf("Expected 2 required fields, got %d: %v", len(schema.Required), schema.Required)
	}
	if !contains(schema.Required, "name") {
		t.Error("Expected 'name' to be required")
	}
	if !contains(schema.Required, "age") {
		t.Error("Expected 'age' to be required")
	}
	if contains(schema.Required, "email") {
		t.Error("Expected 'email' to not be required (has omitempty)")
	}
}

func TestSchemaFromType_Nested(t *testing.T) {
	schema := SchemaFromType[NestedStruct]()

	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got '%s'", schema.Type)
	}

	// Check user field (nested struct)
	userField, ok := schema.Properties["user"]
	if !ok {
		t.Error("Expected 'user' property")
	}
	if userField.Type != "object" {
		t.Errorf("Expected user type 'object', got '%s'", userField.Type)
	}

	// Check nested properties
	if len(userField.Properties) != 3 {
		t.Errorf("Expected 3 nested properties in user, got %d", len(userField.Properties))
	}

	nameField, ok := userField.Properties["name"]
	if !ok {
		t.Error("Expected 'name' property in nested user")
	}
	if nameField.Type != "string" {
		t.Errorf("Expected name type 'string', got '%s'", nameField.Type)
	}

	// Check required fields in nested struct
	if !contains(userField.Required, "name") {
		t.Error("Expected nested 'name' to be required")
	}
}

func TestSchemaFromType_Collections(t *testing.T) {
	schema := SchemaFromType[CollectionTypes]()

	// Check tags (string array)
	tagsField, ok := schema.Properties["tags"]
	if !ok {
		t.Error("Expected 'tags' property")
	}
	if tagsField.Type != "array" {
		t.Errorf("Expected tags type 'array', got '%s'", tagsField.Type)
	}
	if tagsField.Items == nil {
		t.Error("Expected tags items to be defined")
	} else if tagsField.Items.Type != "string" {
		t.Errorf("Expected tags items type 'string', got '%s'", tagsField.Items.Type)
	}

	// Check scores (int array)
	scoresField, ok := schema.Properties["scores"]
	if !ok {
		t.Error("Expected 'scores' property")
	}
	if scoresField.Type != "array" {
		t.Errorf("Expected scores type 'array', got '%s'", scoresField.Type)
	}
	if scoresField.Items == nil {
		t.Error("Expected scores items to be defined")
	} else if scoresField.Items.Type != "integer" {
		t.Errorf("Expected scores items type 'integer', got '%s'", scoresField.Items.Type)
	}

	// Check metadata (map)
	metadataField, ok := schema.Properties["metadata"]
	if !ok {
		t.Error("Expected 'metadata' property")
	}
	if metadataField.Type != "object" {
		t.Errorf("Expected metadata type 'object', got '%s'", metadataField.Type)
	}
}

func TestSchemaFromType_Pointers(t *testing.T) {
	schema := SchemaFromType[PointerFields]()

	// All fields are pointers with omitempty, so none should be required
	if len(schema.Required) != 0 {
		t.Errorf("Expected 0 required fields for pointer struct, got %d", len(schema.Required))
	}

	// Check that pointer types are unwrapped
	nameField, ok := schema.Properties["name"]
	if !ok {
		t.Error("Expected 'name' property")
	}
	if nameField.Type != "string" {
		t.Errorf("Expected name type 'string' (unwrapped from pointer), got '%s'", nameField.Type)
	}

	ageField, ok := schema.Properties["age"]
	if !ok {
		t.Error("Expected 'age' property")
	}
	if ageField.Type != "integer" {
		t.Errorf("Expected age type 'integer' (unwrapped from pointer), got '%s'", ageField.Type)
	}
}

func TestSchemaFromType_TimeField(t *testing.T) {
	schema := SchemaFromType[WithEmbedded]()

	// Check that embedded fields are present
	createdAtField, ok := schema.Properties["created_at"]
	if !ok {
		t.Error("Expected 'created_at' property from embedded struct")
	}
	if createdAtField.Type != "string" {
		t.Errorf("Expected created_at type 'string', got '%s'", createdAtField.Type)
	}
	if createdAtField.Format != "date-time" {
		t.Errorf("Expected created_at format 'date-time', got '%s'", createdAtField.Format)
	}

	// Check that direct field is also present
	titleField, ok := schema.Properties["title"]
	if !ok {
		t.Error("Expected 'title' property")
	}
	if titleField.Type != "string" {
		t.Errorf("Expected title type 'string', got '%s'", titleField.Type)
	}
}

func TestSchemaFromType_EmbeddedFields(t *testing.T) {
	schema := SchemaFromType[WithEmbedded]()

	// Should have properties from both embedded and direct fields
	expectedFields := []string{"id", "created_at", "title"}
	if len(schema.Properties) != len(expectedFields) {
		t.Errorf("Expected %d properties, got %d", len(expectedFields), len(schema.Properties))
	}

	for _, field := range expectedFields {
		if _, ok := schema.Properties[field]; !ok {
			t.Errorf("Expected property '%s'", field)
		}
	}
}

func TestSchemaFromType_IgnoredFields(t *testing.T) {
	schema := SchemaFromType[WithIgnoredFields]()

	// Should only have 'public' field
	if len(schema.Properties) != 1 {
		t.Errorf("Expected 1 property, got %d", len(schema.Properties))
	}

	if _, ok := schema.Properties["public"]; !ok {
		t.Error("Expected 'public' property")
	}

	// Should not have skipped or private fields
	if _, ok := schema.Properties["Skipped"]; ok {
		t.Error("Did not expect 'Skipped' property (json:\"-\")")
	}
	if _, ok := schema.Properties["private"]; ok {
		t.Error("Did not expect 'private' property (unexported)")
	}
}

func TestSchemaFromType_InterfaceField(t *testing.T) {
	schema := SchemaFromType[ComplexNested]()

	// Check meta field (map[string]interface{})
	metaField, ok := schema.Properties["meta"]
	if !ok {
		t.Error("Expected 'meta' property")
	}
	if metaField.Type != "object" {
		t.Errorf("Expected meta type 'object', got '%s'", metaField.Type)
	}
}

func TestTypeName(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		expected string
	}{
		{"SimpleStruct", TypeName[SimpleStruct](), "SimpleStruct"},
		{"String", TypeName[string](), "string"},
		{"Int", TypeName[int](), "int"},
		{"Pointer", TypeName[*SimpleStruct](), "SimpleStruct"},
		{"Slice", TypeName[[]string](), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.typeName != tt.expected {
				t.Errorf("TypeName() = %v, want %v", tt.typeName, tt.expected)
			}
		})
	}
}

func TestSchemaFromType_JSONMarshaling(t *testing.T) {
	schema := SchemaFromType[SimpleStruct]()

	// Test that schema can be marshaled to JSON
	jsonData, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Failed to marshal schema to JSON: %v", err)
	}

	// Verify it's valid JSON
	var decoded map[string]interface{}
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal schema JSON: %v", err)
	}

	// Check basic structure
	if decoded["type"] != "object" {
		t.Errorf("Expected type 'object' in JSON, got '%v'", decoded["type"])
	}

	properties, ok := decoded["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected properties to be an object")
	}

	if len(properties) != 3 {
		t.Errorf("Expected 3 properties in JSON, got %d", len(properties))
	}
}

func TestSchemaFromType_PrimitiveTypes(t *testing.T) {
	tests := []struct {
		name         string
		schemaFunc   func() schema.JSONSchema
		expectedType string
	}{
		{"string", SchemaFromType[string], "string"},
		{"int", SchemaFromType[int], "integer"},
		{"int32", SchemaFromType[int32], "integer"},
		{"int64", SchemaFromType[int64], "integer"},
		{"float32", SchemaFromType[float32], "number"},
		{"float64", SchemaFromType[float64], "number"},
		{"bool", SchemaFromType[bool], "boolean"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.schemaFunc()
			if schema.Type != tt.expectedType {
				t.Errorf("Expected type '%s', got '%s'", tt.expectedType, schema.Type)
			}
		})
	}
}

func TestSchemaFromType_SliceOfStructs(t *testing.T) {
	schema := SchemaFromType[[]SimpleStruct]()

	if schema.Type != "array" {
		t.Errorf("Expected type 'array', got '%s'", schema.Type)
	}

	if schema.Items == nil {
		t.Fatal("Expected items to be defined")
	}

	if schema.Items.Type != "object" {
		t.Errorf("Expected items type 'object', got '%s'", schema.Items.Type)
	}

	if len(schema.Items.Properties) != 3 {
		t.Errorf("Expected 3 properties in array items, got %d", len(schema.Items.Properties))
	}
}

func TestSchemaFromType_PointerToStruct(t *testing.T) {
	schema := SchemaFromType[*SimpleStruct]()

	// Should unwrap pointer and return struct schema
	if schema.Type != "object" {
		t.Errorf("Expected type 'object', got '%s'", schema.Type)
	}

	if len(schema.Properties) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(schema.Properties))
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkSchemaFromType_Simple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = SchemaFromType[SimpleStruct]()
	}
}

func BenchmarkSchemaFromType_Nested(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = SchemaFromType[NestedStruct]()
	}
}

func BenchmarkSchemaFromType_Complex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = SchemaFromType[ComplexNested]()
	}
}

// Test that demonstrates the reflection approach
func TestReflectionExample(t *testing.T) {
	type Example struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	schema := SchemaFromType[Example]()

	// Verify the schema structure
	if schema.Type != "object" {
		t.Errorf("Expected object type")
	}

	if !reflect.DeepEqual(schema.Required, []string{"name", "count"}) {
		t.Errorf("Expected both fields to be required, got: %v", schema.Required)
	}
}
