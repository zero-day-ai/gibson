package payload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestPayloadCategory_String tests PayloadCategory.String()
func TestPayloadCategory_String(t *testing.T) {
	tests := []struct {
		name     string
		category PayloadCategory
		expected string
	}{
		{"jailbreak", CategoryJailbreak, "jailbreak"},
		{"prompt_injection", CategoryPromptInjection, "prompt_injection"},
		{"data_extraction", CategoryDataExtraction, "data_extraction"},
		{"dos", CategoryDoS, "dos"},
		{"model_manipulation", CategoryModelManipulation, "model_manipulation"},
		{"privilege_escalation", CategoryPrivilegeEscalation, "privilege_escalation"},
		{"rag_poisoning", CategoryRAGPoisoning, "rag_poisoning"},
		{"information_disclosure", CategoryInformationDisclosure, "information_disclosure"},
		{"encoding_bypass", CategoryEncodingBypass, "encoding_bypass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.category.String())
		})
	}
}

// TestPayloadCategory_IsValid tests PayloadCategory.IsValid()
func TestPayloadCategory_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		category PayloadCategory
		expected bool
	}{
		{"valid jailbreak", CategoryJailbreak, true},
		{"valid prompt_injection", CategoryPromptInjection, true},
		{"valid data_extraction", CategoryDataExtraction, true},
		{"valid dos", CategoryDoS, true},
		{"valid model_manipulation", CategoryModelManipulation, true},
		{"valid privilege_escalation", CategoryPrivilegeEscalation, true},
		{"valid rag_poisoning", CategoryRAGPoisoning, true},
		{"valid information_disclosure", CategoryInformationDisclosure, true},
		{"valid encoding_bypass", CategoryEncodingBypass, true},
		{"invalid empty", PayloadCategory(""), false},
		{"invalid unknown", PayloadCategory("unknown"), false},
		{"invalid random", PayloadCategory("foobar"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.category.IsValid())
		})
	}
}

// TestAllCategories tests AllCategories()
func TestAllCategories(t *testing.T) {
	categories := AllCategories()

	assert.Len(t, categories, 9, "should have 9 categories")

	// Verify all are valid
	for _, cat := range categories {
		assert.True(t, cat.IsValid(), "category %s should be valid", cat)
	}

	// Verify expected categories are present
	expectedCategories := []PayloadCategory{
		CategoryJailbreak,
		CategoryPromptInjection,
		CategoryDataExtraction,
		CategoryDoS,
		CategoryModelManipulation,
		CategoryPrivilegeEscalation,
		CategoryRAGPoisoning,
		CategoryInformationDisclosure,
		CategoryEncodingBypass,
	}

	assert.ElementsMatch(t, expectedCategories, categories)
}

// TestParameterType_String tests ParameterType.String()
func TestParameterType_String(t *testing.T) {
	tests := []struct {
		name     string
		paramType ParameterType
		expected string
	}{
		{"string", ParameterTypeString, "string"},
		{"int", ParameterTypeInt, "int"},
		{"bool", ParameterTypeBool, "bool"},
		{"float", ParameterTypeFloat, "float"},
		{"json", ParameterTypeJSON, "json"},
		{"list", ParameterTypeList, "list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.paramType.String())
		})
	}
}

// TestParameterType_IsValid tests ParameterType.IsValid()
func TestParameterType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		paramType ParameterType
		expected  bool
	}{
		{"valid string", ParameterTypeString, true},
		{"valid int", ParameterTypeInt, true},
		{"valid bool", ParameterTypeBool, true},
		{"valid float", ParameterTypeFloat, true},
		{"valid json", ParameterTypeJSON, true},
		{"valid list", ParameterTypeList, true},
		{"invalid empty", ParameterType(""), false},
		{"invalid unknown", ParameterType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.paramType.IsValid())
		})
	}
}

// TestGeneratorType_String tests GeneratorType.String()
func TestGeneratorType_String(t *testing.T) {
	tests := []struct {
		name     string
		genType  GeneratorType
		expected string
	}{
		{"random", GeneratorRandom, "random"},
		{"uuid", GeneratorUUID, "uuid"},
		{"timestamp", GeneratorTimestamp, "timestamp"},
		{"sequence", GeneratorSequence, "sequence"},
		{"from_list", GeneratorFromList, "from_list"},
		{"expression", GeneratorExpression, "expression"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.genType.String())
		})
	}
}

// TestIndicatorType_String tests IndicatorType.String()
func TestIndicatorType_String(t *testing.T) {
	tests := []struct {
		name     string
		indType  IndicatorType
		expected string
	}{
		{"regex", IndicatorRegex, "regex"},
		{"contains", IndicatorContains, "contains"},
		{"not_contains", IndicatorNotContains, "not_contains"},
		{"length", IndicatorLength, "length"},
		{"status", IndicatorStatus, "status"},
		{"json", IndicatorJSON, "json"},
		{"semantic", IndicatorSemantic, "semantic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.indType.String())
		})
	}
}

// TestIndicatorType_IsValid tests IndicatorType.IsValid()
func TestIndicatorType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		indType  IndicatorType
		expected bool
	}{
		{"valid regex", IndicatorRegex, true},
		{"valid contains", IndicatorContains, true},
		{"valid not_contains", IndicatorNotContains, true},
		{"valid length", IndicatorLength, true},
		{"valid status", IndicatorStatus, true},
		{"valid json", IndicatorJSON, true},
		{"valid semantic", IndicatorSemantic, true},
		{"invalid empty", IndicatorType(""), false},
		{"invalid unknown", IndicatorType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.indType.IsValid())
		})
	}
}

// TestNewPayload tests NewPayload constructor
func TestNewPayload(t *testing.T) {
	before := time.Now()
	payload := NewPayload("test-payload", "Hello {{target}}", CategoryJailbreak, CategoryPromptInjection)
	after := time.Now()

	assert.NotEmpty(t, payload.ID)
	assert.Equal(t, "test-payload", payload.Name)
	assert.Equal(t, "1.0.0", payload.Version)
	assert.Equal(t, "Hello {{target}}", payload.Template)
	assert.Len(t, payload.Categories, 2)
	assert.Contains(t, payload.Categories, CategoryJailbreak)
	assert.Contains(t, payload.Categories, CategoryPromptInjection)
	assert.Empty(t, payload.Tags)
	assert.Empty(t, payload.Parameters)
	assert.Empty(t, payload.SuccessIndicators)
	assert.Empty(t, payload.TargetTypes)
	assert.Empty(t, payload.MitreTechniques)
	assert.False(t, payload.BuiltIn)
	assert.True(t, payload.Enabled)
	assert.True(t, payload.CreatedAt.After(before) || payload.CreatedAt.Equal(before))
	assert.True(t, payload.CreatedAt.Before(after) || payload.CreatedAt.Equal(after))
	assert.Equal(t, payload.CreatedAt, payload.UpdatedAt)
}

// TestPayload_BuilderMethods tests fluent builder methods
func TestPayload_BuilderMethods(t *testing.T) {
	payload := NewPayload("test", "template", CategoryJailbreak)

	// Test WithDescription
	payload = payload.WithDescription("test description")
	assert.Equal(t, "test description", payload.Description)

	// Test WithVersion
	payload = payload.WithVersion("2.0.0")
	assert.Equal(t, "2.0.0", payload.Version)

	// Test WithTags
	payload = payload.WithTags("tag1", "tag2")
	assert.Equal(t, []string{"tag1", "tag2"}, payload.Tags)

	// Test WithSeverity
	payload = payload.WithSeverity(agent.SeverityCritical)
	assert.Equal(t, agent.SeverityCritical, payload.Severity)

	// Test WithTargetTypes
	payload = payload.WithTargetTypes("openai", "anthropic")
	assert.Equal(t, []string{"openai", "anthropic"}, payload.TargetTypes)

	// Test WithMitreTechniques
	payload = payload.WithMitreTechniques("AML.T0051")
	assert.Equal(t, []string{"AML.T0051"}, payload.MitreTechniques)

	// Test WithMetadata
	metadata := PayloadMetadata{
		Author: "test author",
		Source: "test source",
	}
	payload = payload.WithMetadata(metadata)
	assert.Equal(t, "test author", payload.Metadata.Author)
	assert.Equal(t, "test source", payload.Metadata.Source)

	// Test MarkBuiltIn
	payload = payload.MarkBuiltIn()
	assert.True(t, payload.BuiltIn)

	// Test Enable/Disable
	payload = payload.Disable()
	assert.False(t, payload.Enabled)
	payload = payload.Enable()
	assert.True(t, payload.Enabled)
}

// TestPayload_WithParameters tests WithParameters builder method
func TestPayload_WithParameters(t *testing.T) {
	param1 := ParameterDef{
		Name:     "target",
		Type:     ParameterTypeString,
		Required: true,
	}
	param2 := ParameterDef{
		Name:     "count",
		Type:     ParameterTypeInt,
		Required: false,
		Default:  10,
	}

	payload := NewPayload("test", "template", CategoryJailbreak)
	payload = payload.WithParameters(param1, param2)

	assert.Len(t, payload.Parameters, 2)
	assert.Equal(t, "target", payload.Parameters[0].Name)
	assert.Equal(t, "count", payload.Parameters[1].Name)
}

// TestPayload_WithSuccessIndicators tests WithSuccessIndicators builder method
func TestPayload_WithSuccessIndicators(t *testing.T) {
	indicator1 := SuccessIndicator{
		Type:   IndicatorRegex,
		Value:  "(?i)jailbreak",
		Weight: 1.0,
	}
	indicator2 := SuccessIndicator{
		Type:   IndicatorContains,
		Value:  "success",
		Weight: 0.8,
	}

	payload := NewPayload("test", "template", CategoryJailbreak)
	payload = payload.WithSuccessIndicators(indicator1, indicator2)

	assert.Len(t, payload.SuccessIndicators, 2)
	assert.Equal(t, IndicatorRegex, payload.SuccessIndicators[0].Type)
	assert.Equal(t, IndicatorContains, payload.SuccessIndicators[1].Type)
}

// TestPayload_Validate tests Payload.Validate()
func TestPayload_Validate(t *testing.T) {
	tests := []struct {
		name      string
		payload   func() Payload
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid payload",
			payload: func() Payload {
				return NewPayload("test", "template", CategoryJailbreak).
					WithSuccessIndicators(SuccessIndicator{
						Type:  IndicatorContains,
						Value: "success",
					})
			},
			expectErr: false,
		},
		{
			name: "empty name",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.Name = ""
				return p
			},
			expectErr: true,
			errMsg:    "name is required",
		},
		{
			name: "empty template",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.Template = ""
				return p
			},
			expectErr: true,
			errMsg:    "template is required",
		},
		{
			name: "no categories",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.Categories = []PayloadCategory{}
				return p
			},
			expectErr: true,
			errMsg:    "at least one category is required",
		},
		{
			name: "invalid category",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.Categories = []PayloadCategory{"invalid"}
				return p
			},
			expectErr: true,
			errMsg:    "invalid category",
		},
		{
			name: "no success indicators",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.SuccessIndicators = []SuccessIndicator{}
				return p
			},
			expectErr: true,
			errMsg:    "at least one success indicator is required",
		},
		{
			name: "invalid indicator type",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.SuccessIndicators = []SuccessIndicator{{
					Type:  IndicatorType("invalid"),
					Value: "test",
				}}
				return p
			},
			expectErr: true,
			errMsg:    "invalid indicator type",
		},
		{
			name: "empty indicator value",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak)
				p.SuccessIndicators = []SuccessIndicator{{
					Type:  IndicatorContains,
					Value: "",
				}}
				return p
			},
			expectErr: true,
			errMsg:    "indicator value is required",
		},
		{
			name: "parameter missing name",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak).
					WithSuccessIndicators(SuccessIndicator{
						Type:  IndicatorContains,
						Value: "test",
					}).
					WithParameters(ParameterDef{
						Name: "",
						Type: ParameterTypeString,
					})
				return p
			},
			expectErr: true,
			errMsg:    "parameter name is required",
		},
		{
			name: "parameter invalid type",
			payload: func() Payload {
				p := NewPayload("test", "template", CategoryJailbreak).
					WithSuccessIndicators(SuccessIndicator{
						Type:  IndicatorContains,
						Value: "test",
					}).
					WithParameters(ParameterDef{
						Name: "param1",
						Type: ParameterType("invalid"),
					})
				return p
			},
			expectErr: true,
			errMsg:    "invalid parameter type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.payload()
			err := payload.Validate()

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPayload_HasCategory tests HasCategory method
func TestPayload_HasCategory(t *testing.T) {
	payload := NewPayload("test", "template", CategoryJailbreak, CategoryPromptInjection)

	assert.True(t, payload.HasCategory(CategoryJailbreak))
	assert.True(t, payload.HasCategory(CategoryPromptInjection))
	assert.False(t, payload.HasCategory(CategoryDataExtraction))
	assert.False(t, payload.HasCategory(CategoryDoS))
}

// TestPayload_HasTag tests HasTag method
func TestPayload_HasTag(t *testing.T) {
	payload := NewPayload("test", "template", CategoryJailbreak).
		WithTags("tag1", "tag2", "tag3")

	assert.True(t, payload.HasTag("tag1"))
	assert.True(t, payload.HasTag("tag2"))
	assert.True(t, payload.HasTag("tag3"))
	assert.False(t, payload.HasTag("tag4"))
	assert.False(t, payload.HasTag("nonexistent"))
}

// TestPayload_HasMitreTechnique tests HasMitreTechnique method
func TestPayload_HasMitreTechnique(t *testing.T) {
	payload := NewPayload("test", "template", CategoryJailbreak).
		WithMitreTechniques("AML.T0051", "AML.T0024")

	assert.True(t, payload.HasMitreTechnique("AML.T0051"))
	assert.True(t, payload.HasMitreTechnique("AML.T0024"))
	assert.False(t, payload.HasMitreTechnique("AML.T0000"))
	assert.False(t, payload.HasMitreTechnique("invalid"))
}

// TestPayload_SupportsTargetType tests SupportsTargetType method
func TestPayload_SupportsTargetType(t *testing.T) {
	// Payload with no target types (supports all)
	payload1 := NewPayload("test", "template", CategoryJailbreak)
	assert.True(t, payload1.SupportsTargetType("openai"))
	assert.True(t, payload1.SupportsTargetType("anthropic"))
	assert.True(t, payload1.SupportsTargetType("anything"))

	// Payload with specific target types
	payload2 := NewPayload("test", "template", CategoryJailbreak).
		WithTargetTypes("openai", "anthropic")
	assert.True(t, payload2.SupportsTargetType("openai"))
	assert.True(t, payload2.SupportsTargetType("anthropic"))
	assert.False(t, payload2.SupportsTargetType("cohere"))
	assert.False(t, payload2.SupportsTargetType("anything"))
}

// TestPayload_GetRequiredParameters tests GetRequiredParameters method
func TestPayload_GetRequiredParameters(t *testing.T) {
	param1 := ParameterDef{Name: "required1", Type: ParameterTypeString, Required: true}
	param2 := ParameterDef{Name: "optional1", Type: ParameterTypeString, Required: false}
	param3 := ParameterDef{Name: "required2", Type: ParameterTypeInt, Required: true}
	param4 := ParameterDef{Name: "optional2", Type: ParameterTypeBool, Required: false}

	payload := NewPayload("test", "template", CategoryJailbreak).
		WithParameters(param1, param2, param3, param4)

	required := payload.GetRequiredParameters()
	assert.Len(t, required, 2)
	assert.Equal(t, "required1", required[0].Name)
	assert.Equal(t, "required2", required[1].Name)
}

// TestPayload_GetOptionalParameters tests GetOptionalParameters method
func TestPayload_GetOptionalParameters(t *testing.T) {
	param1 := ParameterDef{Name: "required1", Type: ParameterTypeString, Required: true}
	param2 := ParameterDef{Name: "optional1", Type: ParameterTypeString, Required: false}
	param3 := ParameterDef{Name: "required2", Type: ParameterTypeInt, Required: true}
	param4 := ParameterDef{Name: "optional2", Type: ParameterTypeBool, Required: false}

	payload := NewPayload("test", "template", CategoryJailbreak).
		WithParameters(param1, param2, param3, param4)

	optional := payload.GetOptionalParameters()
	assert.Len(t, optional, 2)
	assert.Equal(t, "optional1", optional[0].Name)
	assert.Equal(t, "optional2", optional[1].Name)
}

// TestPayload_GetParameterByName tests GetParameterByName method
func TestPayload_GetParameterByName(t *testing.T) {
	param1 := ParameterDef{Name: "param1", Type: ParameterTypeString, Required: true}
	param2 := ParameterDef{Name: "param2", Type: ParameterTypeInt, Required: false}

	payload := NewPayload("test", "template", CategoryJailbreak).
		WithParameters(param1, param2)

	// Existing parameters
	found1 := payload.GetParameterByName("param1")
	require.NotNil(t, found1)
	assert.Equal(t, "param1", found1.Name)
	assert.Equal(t, ParameterTypeString, found1.Type)
	assert.True(t, found1.Required)

	found2 := payload.GetParameterByName("param2")
	require.NotNil(t, found2)
	assert.Equal(t, "param2", found2.Name)
	assert.Equal(t, ParameterTypeInt, found2.Type)
	assert.False(t, found2.Required)

	// Non-existent parameter
	notFound := payload.GetParameterByName("nonexistent")
	assert.Nil(t, notFound)
}

// TestPayloadFilter tests PayloadFilter struct
func TestPayloadFilter(t *testing.T) {
	// Create a filter with various criteria
	id1 := types.NewID()
	id2 := types.NewID()
	enabled := true
	builtIn := false

	filter := &PayloadFilter{
		IDs:             []types.ID{id1, id2},
		Categories:      []PayloadCategory{CategoryJailbreak, CategoryPromptInjection},
		Tags:            []string{"tag1", "tag2"},
		TargetTypes:     []string{"openai", "anthropic"},
		Severities:      []agent.FindingSeverity{agent.SeverityCritical, agent.SeverityHigh},
		MitreTechniques: []string{"AML.T0051"},
		BuiltIn:         &builtIn,
		Enabled:         &enabled,
		SearchQuery:     "jailbreak",
		Limit:           50,
		Offset:          10,
	}

	// Verify filter fields
	assert.Len(t, filter.IDs, 2)
	assert.Len(t, filter.Categories, 2)
	assert.Len(t, filter.Tags, 2)
	assert.Len(t, filter.TargetTypes, 2)
	assert.Len(t, filter.Severities, 2)
	assert.Len(t, filter.MitreTechniques, 1)
	assert.NotNil(t, filter.BuiltIn)
	assert.False(t, *filter.BuiltIn)
	assert.NotNil(t, filter.Enabled)
	assert.True(t, *filter.Enabled)
	assert.Equal(t, "jailbreak", filter.SearchQuery)
	assert.Equal(t, 50, filter.Limit)
	assert.Equal(t, 10, filter.Offset)
}

// TestParameterValidation tests ParameterValidation struct
func TestParameterValidation(t *testing.T) {
	minLen := 5
	maxLen := 100
	pattern := "^[a-z]+$"
	min := 10.0
	max := 100.0

	validation := &ParameterValidation{
		MinLength: &minLen,
		MaxLength: &maxLen,
		Pattern:   &pattern,
		Enum:      []any{"option1", "option2", "option3"},
		Min:       &min,
		Max:       &max,
	}

	assert.Equal(t, 5, *validation.MinLength)
	assert.Equal(t, 100, *validation.MaxLength)
	assert.Equal(t, "^[a-z]+$", *validation.Pattern)
	assert.Len(t, validation.Enum, 3)
	assert.Equal(t, 10.0, *validation.Min)
	assert.Equal(t, 100.0, *validation.Max)
}

// TestParameterGenerator tests ParameterGenerator struct
func TestParameterGenerator(t *testing.T) {
	generator := &ParameterGenerator{
		Type: GeneratorRandom,
		Config: map[string]any{
			"min": 1,
			"max": 100,
		},
	}

	assert.Equal(t, GeneratorRandom, generator.Type)
	assert.Len(t, generator.Config, 2)
	assert.Equal(t, 1, generator.Config["min"])
	assert.Equal(t, 100, generator.Config["max"])
}

// TestSuccessIndicator tests SuccessIndicator struct
func TestSuccessIndicator(t *testing.T) {
	indicator := SuccessIndicator{
		Type:        IndicatorRegex,
		Value:       "(?i)success",
		Description: "Check for success message",
		Weight:      0.9,
		Negate:      false,
	}

	assert.Equal(t, IndicatorRegex, indicator.Type)
	assert.Equal(t, "(?i)success", indicator.Value)
	assert.Equal(t, "Check for success message", indicator.Description)
	assert.Equal(t, 0.9, indicator.Weight)
	assert.False(t, indicator.Negate)
}

// TestPayloadMetadata tests PayloadMetadata struct
func TestPayloadMetadata(t *testing.T) {
	metadata := PayloadMetadata{
		Author:      "John Doe",
		Source:      "https://example.com/payload",
		References:  []string{"https://ref1.com", "https://ref2.com"},
		Notes:       "This is a test payload",
		Examples:    []string{"example1", "example2"},
		Difficulty:  "medium",
		Reliability: 0.85,
	}

	assert.Equal(t, "John Doe", metadata.Author)
	assert.Equal(t, "https://example.com/payload", metadata.Source)
	assert.Len(t, metadata.References, 2)
	assert.Equal(t, "This is a test payload", metadata.Notes)
	assert.Len(t, metadata.Examples, 2)
	assert.Equal(t, "medium", metadata.Difficulty)
	assert.Equal(t, 0.85, metadata.Reliability)
}
