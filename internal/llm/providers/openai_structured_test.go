package providers

import (
	"context"
	"testing"

	"github.com/zero-day-ai/gibson/internal/llm"
	sdktypes "github.com/zero-day-ai/gibson/sdk/types"
)

// TestOpenAIStructuredOutputInterface verifies OpenAI provider implements StructuredOutputProvider
func TestOpenAIStructuredOutputInterface(t *testing.T) {
	cfg := llm.ProviderConfig{
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	provider, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create OpenAI provider: %v", err)
	}

	// Test interface implementation
	var sop llm.StructuredOutputProvider
	var ok bool
	sop, ok = interface{}(provider).(llm.StructuredOutputProvider)
	if !ok {
		t.Fatal("OpenAI provider does not implement StructuredOutputProvider interface")
	}

	t.Log("✓ OpenAI provider implements StructuredOutputProvider interface")

	// Test format support
	tests := []struct {
		format   sdktypes.ResponseFormatType
		expected bool
		name     string
	}{
		{sdktypes.ResponseFormatJSONObject, true, "json_object"},
		{sdktypes.ResponseFormatJSONSchema, true, "json_schema"},
		{sdktypes.ResponseFormatText, false, "text"},
	}

	for _, tt := range tests {
		t.Run("Supports_"+tt.name, func(t *testing.T) {
			supported := sop.SupportsStructuredOutput(tt.format)
			if supported != tt.expected {
				t.Errorf("SupportsStructuredOutput(%s) = %v, want %v", tt.name, supported, tt.expected)
			}
		})
	}
}

// TestOpenAICompleteStructuredValidation tests validation of structured output requests
func TestOpenAICompleteStructuredValidation(t *testing.T) {
	cfg := llm.ProviderConfig{
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}

	provider, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("Failed to create OpenAI provider: %v", err)
	}

	sop := interface{}(provider).(llm.StructuredOutputProvider)

	t.Run("NilResponseFormat", func(t *testing.T) {
		req := llm.CompletionRequest{
			Model: "gpt-4",
			Messages: []llm.Message{
				llm.NewUserMessage("test"),
			},
			ResponseFormat: nil,
		}

		_, err := sop.CompleteStructured(context.Background(), req)
		if err == nil {
			t.Error("Expected error for nil ResponseFormat, got nil")
		}
	})

	t.Run("UnsupportedFormat", func(t *testing.T) {
		req := llm.CompletionRequest{
			Model: "gpt-4",
			Messages: []llm.Message{
				llm.NewUserMessage("test"),
			},
			ResponseFormat: &sdktypes.ResponseFormat{
				Type: sdktypes.ResponseFormatText,
			},
		}

		_, err := sop.CompleteStructured(context.Background(), req)
		if err == nil {
			t.Error("Expected error for unsupported format, got nil")
		}
	})

	t.Run("JSONSchemaWithoutName", func(t *testing.T) {
		schema := &sdktypes.JSONSchema{
			Type: "object",
			Properties: map[string]*sdktypes.JSONSchema{
				"name": {Type: "string"},
			},
		}

		req := llm.CompletionRequest{
			Model: "gpt-4",
			Messages: []llm.Message{
				llm.NewUserMessage("test"),
			},
			ResponseFormat: &sdktypes.ResponseFormat{
				Type:   sdktypes.ResponseFormatJSONSchema,
				Schema: schema,
				Name:   "", // Missing name
			},
		}

		_, err := sop.CompleteStructured(context.Background(), req)
		if err == nil {
			t.Error("Expected error for json_schema without name, got nil")
		}
	})
}

// TestOpenAIJSONSchemaConversion tests schema conversion helper
func TestOpenAIJSONSchemaConversion(t *testing.T) {
	t.Run("NilSchema", func(t *testing.T) {
		_, err := jsonSchemaToMap(nil)
		if err == nil {
			t.Error("Expected error for nil schema, got nil")
		}
	})

	t.Run("ValidSchema", func(t *testing.T) {
		schema := &sdktypes.JSONSchema{
			Type: "object",
			Properties: map[string]*sdktypes.JSONSchema{
				"name": {
					Type:        "string",
					Description: "The name field",
				},
				"age": {
					Type:    "integer",
					Minimum: openaiFloat64Ptr(0),
				},
			},
			Required: []string{"name"},
		}

		result, err := jsonSchemaToMap(schema)
		if err != nil {
			t.Fatalf("jsonSchemaToMap failed: %v", err)
		}

		// Verify structure
		if result["type"] != "object" {
			t.Errorf("Expected type=object, got %v", result["type"])
		}

		props, ok := result["properties"].(map[string]interface{})
		if !ok {
			t.Fatal("Properties is not a map")
		}

		if _, ok := props["name"]; !ok {
			t.Error("Missing 'name' property")
		}

		if _, ok := props["age"]; !ok {
			t.Error("Missing 'age' property")
		}

		required, ok := result["required"].([]interface{})
		if !ok {
			t.Fatal("Required is not an array")
		}

		if len(required) != 1 || required[0] != "name" {
			t.Errorf("Expected required=['name'], got %v", required)
		}
	})
}

func openaiFloat64Ptr(f float64) *float64 {
	return &f
}
