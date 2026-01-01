package prompt

import (
	"strings"
	"testing"
	"text/template"
)

// TestTemplateIntegrationBasic tests using custom functions in a real template
func TestTemplateIntegrationBasic(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		data     map[string]any
		expected string
	}{
		{
			name:     "string functions",
			tmpl:     `{{ toUpper .name }} - {{ toLower .ROLE }}`,
			data:     map[string]any{"name": "gibson", "ROLE": "AGENT"},
			expected: "GIBSON - agent",
		},
		{
			name:     "title case",
			tmpl:     `{{ title .message }}`,
			data:     map[string]any{"message": "hello world"},
			expected: "Hello World",
		},
		{
			name:     "trim and replace",
			tmpl:     `{{ trim .text | replace "old" "new" }}`,
			data:     map[string]any{"text": "  old value  "},
			expected: "new value",
		},
		{
			name:     "default value",
			tmpl:     `{{ default "unknown" .missing }}`,
			data:     map[string]any{},
			expected: "unknown",
		},
		{
			name:     "coalesce",
			tmpl:     `{{ coalesce .a .b .c }}`,
			data:     map[string]any{"a": "", "b": "found", "c": "fallback"},
			expected: "found",
		},
		{
			name:     "ternary",
			tmpl:     `{{ ternary .isActive "enabled" "disabled" }}`,
			data:     map[string]any{"isActive": true},
			expected: "enabled",
		},
		{
			name:     "split and join",
			tmpl:     `{{ split "," .items | join " | " }}`,
			data:     map[string]any{"items": "apple,banana,orange"},
			expected: "apple | banana | orange",
		},
		{
			name:     "json serialization",
			tmpl:     `{{ toJSON .config }}`,
			data:     map[string]any{"config": map[string]any{"key": "value"}},
			expected: `{"key":"value"}`,
		},
		{
			name:     "quote string",
			tmpl:     `{{ quote .message }}`,
			data:     map[string]any{"message": "hello"},
			expected: `"hello"`,
		},
		{
			name:     "indent multiline",
			tmpl:     `{{ indent 2 .code }}`,
			data:     map[string]any{"code": "line1\nline2"},
			expected: "  line1\n  line2",
		},
	}

	funcMap := DefaultFuncMap()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("test").Funcs(funcMap).Parse(tt.tmpl)
			if err != nil {
				t.Fatalf("Failed to parse template: %v", err)
			}

			var buf strings.Builder
			if err := tmpl.Execute(&buf, tt.data); err != nil {
				t.Fatalf("Failed to execute template: %v", err)
			}

			result := buf.String()
			if result != tt.expected {
				t.Errorf("Template result = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestTemplateIntegrationAdvanced tests more complex template scenarios
func TestTemplateIntegrationAdvanced(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		data     map[string]any
		expected string
	}{
		{
			name:     "conditional with string checks",
			tmpl:     `{{ if hasPrefix .url "https://" }}Secure{{ else }}Insecure{{ end }}`,
			data:     map[string]any{"url": "https://example.com"},
			expected: "Secure",
		},
		{
			name:     "nested functions",
			tmpl:     `{{ toUpper (trim .input) }}`,
			data:     map[string]any{"input": "  hello  "},
			expected: "HELLO",
		},
		{
			name:     "combine multiple operations",
			tmpl:     `{{ $items := split "," .csv }}{{ range $items }}{{ trim . | toUpper }}{{ end }}`,
			data:     map[string]any{"csv": "a, b, c"},
			expected: "ABC",
		},
		{
			name:     "indent with nindent",
			tmpl:     `config:{{ nindent 2 .yaml }}`,
			data:     map[string]any{"yaml": "key: value\nfoo: bar"},
			expected: "config:\n  key: value\n  foo: bar",
		},
		{
			name:     "pretty JSON",
			tmpl:     `{{ toPrettyJSON .data }}`,
			data:     map[string]any{"data": map[string]any{"name": "Gibson", "version": 1}},
			expected: "{\n  \"name\": \"Gibson\",\n  \"version\": 1\n}",
		},
	}

	funcMap := DefaultFuncMap()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("test").Funcs(funcMap).Parse(tt.tmpl)
			if err != nil {
				t.Fatalf("Failed to parse template: %v", err)
			}

			var buf strings.Builder
			if err := tmpl.Execute(&buf, tt.data); err != nil {
				t.Fatalf("Failed to execute template: %v", err)
			}

			result := buf.String()
			if result != tt.expected {
				t.Errorf("Template result = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestTemplateRendererWithFuncs tests that the TemplateRenderer uses DefaultFuncMap
func TestTemplateRendererWithFuncs(t *testing.T) {
	renderer := NewTemplateRenderer()

	// Create a simple prompt with template functions
	prompt := &Prompt{
		ID:       "test-prompt",
		Position: PositionSystem,
		Content:  "Hello {{ toUpper .Variables.name }}! Your role is {{ default \"user\" .Variables.role }}.",
	}

	ctx := NewRenderContext()
	ctx.Variables["name"] = "gibson"
	// Leave role empty to test default

	result, err := renderer.Render(prompt, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Hello GIBSON! Your role is user."
	if result != expected {
		t.Errorf("Render result = %q, want %q", result, expected)
	}
}

// TestCustomFunctionRegistration tests adding custom functions to the renderer
func TestCustomFunctionRegistration(t *testing.T) {
	renderer := NewTemplateRenderer()

	// Register a custom function
	customFunc := func(s string) string {
		return "CUSTOM: " + s
	}

	err := renderer.RegisterFunc("custom", customFunc)
	if err != nil {
		t.Fatalf("Failed to register custom function: %v", err)
	}

	prompt := &Prompt{
		ID:       "test-prompt",
		Position: PositionSystem,
		Content:  "{{ custom .Variables.message }}",
	}

	ctx := NewRenderContext()
	ctx.Variables["message"] = "hello"

	result, err := renderer.Render(prompt, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "CUSTOM: hello"
	if result != expected {
		t.Errorf("Render result = %q, want %q", result, expected)
	}
}

// TestComplexPromptWithFunctions tests a realistic prompt using multiple functions
func TestComplexPromptWithFunctions(t *testing.T) {
	renderer := NewTemplateRenderer()

	prompt := &Prompt{
		ID:       "complex-prompt",
		Position: PositionSystem,
		Content: `You are {{ .Agent.name | title }}.

Mission: {{ trim .Mission.description }}
Status: {{ ternary .Mission.active "Active" "Inactive" }}

{{ if .Target.url }}Target URL: {{ .Target.url }}{{ end }}

Configuration:{{ nindent 2 (toPrettyJSON .Custom.config) }}

{{ $tags := split "," .Mission.tags }}{{ if $tags }}Tags: {{ join ", " $tags }}{{ end }}`,
	}

	ctx := NewRenderContext()
	ctx.Agent = map[string]any{
		"name": "gibson agent",
	}
	ctx.Mission = map[string]any{
		"description": "  Perform security analysis  ",
		"active":      true,
		"tags":        "security,analysis,penetration",
	}
	ctx.Target = map[string]any{
		"url": "https://example.com",
	}
	ctx.Custom = map[string]any{
		"config": map[string]any{
			"timeout": 30,
			"retries": 3,
		},
	}

	result, err := renderer.Render(prompt, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// Verify key parts of the output
	if !strings.Contains(result, "Gibson Agent") {
		t.Error("Expected title-cased agent name")
	}
	if !strings.Contains(result, "Mission: Perform security analysis") {
		t.Error("Expected trimmed mission description")
	}
	if !strings.Contains(result, "Status: Active") {
		t.Error("Expected active status")
	}
	if !strings.Contains(result, "Target URL: https://example.com") {
		t.Error("Expected target URL")
	}
	if !strings.Contains(result, "\"timeout\": 30") {
		t.Error("Expected formatted config JSON")
	}
	if !strings.Contains(result, "Tags: security, analysis, penetration") {
		t.Error("Expected formatted tags")
	}
}
