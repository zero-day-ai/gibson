// Package types provides SDK types for the Gibson framework.
//
// This package contains public SDK types that are suitable for cross-boundary
// communication and external consumption. These types are self-contained and
// do not depend on internal packages.
//
// # Structured Output Types
//
// The structured output types in this package enable LLM responses to be constrained
// to specific formats and schemas:
//
//   - ResponseFormat: Specifies the desired response format (text, json_object, json_schema)
//   - ResponseFormatType: Enum for response format types
//   - JSONSchema: Self-contained JSON Schema definition for validation
//   - StructuredOutputOptions: Configuration for structured output behavior
//
// # Usage Example
//
//	// Create a JSON schema for structured output
//	schema := &types.JSONSchema{
//		Type: "object",
//		Properties: map[string]*types.JSONSchema{
//			"name": {
//				Type:        "string",
//				Description: "User name",
//			},
//			"age": {
//				Type:        "number",
//				Description: "User age",
//			},
//		},
//		Required: []string{"name"},
//	}
//
//	// Create a response format with the schema
//	format := types.NewJSONSchemaFormat("user", schema, true)
//
//	// Configure structured output options
//	opts := types.StructuredOutputOptions{
//		Format:          format,
//		ValidateSchema:  true,
//		ReturnRawOnFail: false,
//	}
//
// # Error Types
//
// The package defines specialized error types for structured output validation:
//
//   - ValidationError: Schema validation errors
//   - UnmarshalError: JSON unmarshaling errors with raw JSON preservation
//
// These errors provide detailed context for debugging structured output issues.
package types
