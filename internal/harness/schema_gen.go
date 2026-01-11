package harness

import (
	"reflect"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/schema"
)

// SchemaFromType generates a JSONSchema from a Go type using reflection.
// It handles structs, pointers, slices, maps, and primitive types.
// The json struct tag is used to determine field names.
//
// Example usage:
//
//	type User struct {
//	    Name  string `json:"name"`
//	    Email string `json:"email,omitempty"`
//	    Age   int    `json:"age"`
//	}
//	schema := SchemaFromType[User]()
func SchemaFromType[T any]() schema.JSONSchema {
	var t T
	return schemaFromReflectType(reflect.TypeOf(t))
}

// schemaFromReflectType generates a JSONSchema from a reflect.Type
func schemaFromReflectType(t reflect.Type) schema.JSONSchema {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		return schemaFromReflectType(t.Elem())
	}

	switch t.Kind() {
	case reflect.Struct:
		return schemaFromStruct(t)
	case reflect.Slice, reflect.Array:
		itemSchema := schemaFieldFromReflectType(t.Elem())
		return schema.JSONSchema{
			Type:  "array",
			Items: &itemSchema,
		}
	case reflect.Map:
		// Maps are represented as objects with additionalProperties
		// Note: JSONSchema.AdditionalProperties is *bool, but we need to handle
		// the value type. For now, we'll set it to true to allow any properties.
		allowAdditional := true
		return schema.JSONSchema{
			Type:                 "object",
			AdditionalProperties: &allowAdditional,
		}
	case reflect.String:
		return schema.JSONSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schema.JSONSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return schema.JSONSchema{Type: "number"}
	case reflect.Bool:
		return schema.JSONSchema{Type: "boolean"}
	case reflect.Interface:
		// interface{} or any - allow any type (no constraints)
		return schema.JSONSchema{}
	default:
		// Fallback for unsupported types
		return schema.JSONSchema{}
	}
}

// schemaFieldFromReflectType generates a SchemaField from a reflect.Type
// This is similar to schemaFromReflectType but returns SchemaField for nested properties
func schemaFieldFromReflectType(t reflect.Type) schema.SchemaField {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		return schemaFieldFromReflectType(t.Elem())
	}

	// Special handling for time.Time
	if t == reflect.TypeOf(time.Time{}) {
		return schema.SchemaField{
			Type:   "string",
			Format: "date-time",
		}
	}

	switch t.Kind() {
	case reflect.Struct:
		return schemaFieldFromStruct(t)
	case reflect.Slice, reflect.Array:
		itemSchema := schemaFieldFromReflectType(t.Elem())
		return schema.SchemaField{
			Type:  "array",
			Items: &itemSchema,
		}
	case reflect.Map:
		// For maps in nested contexts, we create an object with additionalProperties
		// Since SchemaField doesn't have AdditionalProperties, we just return object type
		return schema.SchemaField{
			Type: "object",
		}
	case reflect.String:
		return schema.SchemaField{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schema.SchemaField{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return schema.SchemaField{Type: "number"}
	case reflect.Bool:
		return schema.SchemaField{Type: "boolean"}
	case reflect.Interface:
		// interface{} or any - allow any type (no constraints)
		return schema.SchemaField{}
	default:
		// Fallback for unsupported types
		return schema.SchemaField{}
	}
}

// schemaFromStruct generates a JSONSchema from a struct type
func schemaFromStruct(t reflect.Type) schema.JSONSchema {
	s := schema.JSONSchema{
		Type:       "object",
		Properties: make(map[string]schema.SchemaField),
		Required:   []string{},
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON field name from tag
		jsonTag := field.Tag.Get("json")
		name := field.Name

		// Parse json tag
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				if parts[0] == "-" {
					// Skip this field (json:"-")
					continue
				}
				name = parts[0]
			}
		}

		// Handle anonymous embedded fields
		if field.Anonymous {
			// For embedded structs, merge their properties into the parent
			if field.Type.Kind() == reflect.Struct || (field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct) {
				embeddedSchema := schemaFieldFromReflectType(field.Type)
				// Merge properties from embedded struct
				if embeddedSchema.Properties != nil {
					for k, v := range embeddedSchema.Properties {
						s.Properties[k] = v
					}
					// Merge required fields
					if embeddedSchema.Required != nil {
						s.Required = append(s.Required, embeddedSchema.Required...)
					}
				}
				continue
			}
		}

		// Generate schema for field
		s.Properties[name] = schemaFieldFromReflectType(field.Type)

		// Add to required if not omitempty and not a pointer
		isOmitEmpty := strings.Contains(jsonTag, "omitempty")
		isPointer := field.Type.Kind() == reflect.Ptr
		if !isOmitEmpty && !isPointer {
			s.Required = append(s.Required, name)
		}
	}

	return s
}

// schemaFieldFromStruct generates a SchemaField from a struct type
// This is used for nested structs within properties
func schemaFieldFromStruct(t reflect.Type) schema.SchemaField {
	s := schema.SchemaField{
		Type:       "object",
		Properties: make(map[string]schema.SchemaField),
		Required:   []string{},
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON field name from tag
		jsonTag := field.Tag.Get("json")
		name := field.Name

		// Parse json tag
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				if parts[0] == "-" {
					// Skip this field (json:"-")
					continue
				}
				name = parts[0]
			}
		}

		// Handle anonymous embedded fields
		if field.Anonymous {
			// For embedded structs, merge their properties into the parent
			if field.Type.Kind() == reflect.Struct || (field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct) {
				embeddedSchema := schemaFieldFromReflectType(field.Type)
				// Merge properties from embedded struct
				if embeddedSchema.Properties != nil {
					for k, v := range embeddedSchema.Properties {
						s.Properties[k] = v
					}
					// Merge required fields
					if embeddedSchema.Required != nil {
						s.Required = append(s.Required, embeddedSchema.Required...)
					}
				}
				continue
			}
		}

		// Generate schema for field
		s.Properties[name] = schemaFieldFromReflectType(field.Type)

		// Add to required if not omitempty and not a pointer
		isOmitEmpty := strings.Contains(jsonTag, "omitempty")
		isPointer := field.Type.Kind() == reflect.Ptr
		if !isOmitEmpty && !isPointer {
			s.Required = append(s.Required, name)
		}
	}

	return s
}

// TypeName returns the name of a type for tracing purposes
func TypeName[T any]() string {
	var t T
	typ := reflect.TypeOf(t)
	if typ == nil {
		return "nil"
	}
	// Handle pointer types
	if typ.Kind() == reflect.Ptr {
		return typ.Elem().Name()
	}
	return typ.Name()
}
