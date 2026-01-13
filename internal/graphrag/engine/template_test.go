package engine

import (
	"testing"
)

func TestTemplateEngine_Interpolate_Simple(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "simple field",
			template: "mission:{id}",
			data:     map[string]any{"id": "m123"},
			want:     "mission:m123",
			wantErr:  false,
		},
		{
			name:     "multiple placeholders",
			template: "run:{trace}:{span}",
			data:     map[string]any{"trace": "t1", "span": "s2"},
			want:     "run:t1:s2",
			wantErr:  false,
		},
		{
			name:     "no placeholders",
			template: "static:value",
			data:     map[string]any{},
			want:     "static:value",
			wantErr:  false,
		},
		{
			name:     "empty template",
			template: "",
			data:     map[string]any{"id": "123"},
			want:     "",
			wantErr:  false,
		},
		{
			name:     "missing field",
			template: "mission:{missing}",
			data:     map[string]any{"id": "123"},
			want:     "",
			wantErr:  true,
		},
		{
			name:     "integer field",
			template: "port:{port}",
			data:     map[string]any{"port": 8080},
			want:     "port:8080",
			wantErr:  false,
		},
		{
			name:     "float field (whole number)",
			template: "score:{score}",
			data:     map[string]any{"score": 100.0},
			want:     "score:100",
			wantErr:  false,
		},
		{
			name:     "float field (decimal)",
			template: "score:{score}",
			data:     map[string]any{"score": 99.5},
			want:     "score:99.5",
			wantErr:  false,
		},
		{
			name:     "boolean field",
			template: "active:{active}",
			data:     map[string]any{"active": true},
			want:     "active:true",
			wantErr:  false,
		},
		{
			name:     "underscore in field name",
			template: "agent:{agent_id}",
			data:     map[string]any{"agent_id": "a1"},
			want:     "agent:a1",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Interpolate(tt.template, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Interpolate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Interpolate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_Interpolate_Nested(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "nested field access",
			template: "port:{host.ip}:{port}",
			data: map[string]any{
				"host": map[string]any{"ip": "1.2.3.4"},
				"port": 80,
			},
			want:    "port:1.2.3.4:80",
			wantErr: false,
		},
		{
			name:     "deeply nested field",
			template: "value:{a.b.c}",
			data: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": "deep",
					},
				},
			},
			want:    "value:deep",
			wantErr: false,
		},
		{
			name:     "missing nested field",
			template: "value:{host.missing}",
			data: map[string]any{
				"host": map[string]any{"ip": "1.2.3.4"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name:     "nested field on non-object",
			template: "value:{field.nested}",
			data: map[string]any{
				"field": "not an object",
			},
			want:    "",
			wantErr: true,
		},
		{
			name:     "JSONPath-style relative path",
			template: "host:{.ip}",
			data: map[string]any{
				"ip": "1.2.3.4",
			},
			want:    "host:1.2.3.4",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Interpolate(tt.template, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Interpolate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Interpolate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_Interpolate_ArrayAccess(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "array index access",
			template: "item:{items[0]}",
			data: map[string]any{
				"items": []any{"first", "second", "third"},
			},
			want:    "item:first",
			wantErr: false,
		},
		{
			name:     "array with nested field",
			template: "name:{items[0].name}",
			data: map[string]any{
				"items": []any{
					map[string]any{"name": "Alice", "age": 30},
					map[string]any{"name": "Bob", "age": 25},
				},
			},
			want:    "name:Alice",
			wantErr: false,
		},
		{
			name:     "array out of bounds",
			template: "item:{items[10]}",
			data: map[string]any{
				"items": []any{"first", "second"},
			},
			want:    "",
			wantErr: true,
		},
		{
			name:     "array with integer values",
			template: "port:{ports[1]}",
			data: map[string]any{
				"ports": []any{80, 443, 8080},
			},
			want:    "port:443",
			wantErr: false,
		},
		{
			name:     "deeply nested array access",
			template: "value:{data[0].items[1]}",
			data: map[string]any{
				"data": []any{
					map[string]any{
						"items": []any{"a", "b", "c"},
					},
				},
			},
			want:    "value:b",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Interpolate(tt.template, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Interpolate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Interpolate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_InterpolateWithContext(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		context  map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "context variable",
			template: "discovered_by:{_context.agent_id}",
			data:     map[string]any{},
			context:  map[string]any{"agent_id": "a1"},
			want:     "discovered_by:a1",
			wantErr:  false,
		},
		{
			name:     "multiple context variables",
			template: "run:{_context.trace_id}:{_context.span_id}",
			data:     map[string]any{},
			context: map[string]any{
				"trace_id": "t1",
				"span_id":  "s2",
			},
			want:    "run:t1:s2",
			wantErr: false,
		},
		{
			name:     "mix data and context",
			template: "finding:{finding_id}:by:{_context.agent_id}",
			data:     map[string]any{"finding_id": "f123"},
			context:  map[string]any{"agent_id": "a1"},
			want:     "finding:f123:by:a1",
			wantErr:  false,
		},
		{
			name:     "parent variable",
			template: "port:{_parent.ip}:80",
			data:     map[string]any{},
			context:  map[string]any{"parent_ip": "1.2.3.4"},
			want:     "port:1.2.3.4:80",
			wantErr:  false,
		},
		{
			name:     "nil context",
			template: "value:{field}",
			data:     map[string]any{"field": "test"},
			context:  nil,
			want:     "value:test",
			wantErr:  false,
		},
		{
			name:     "nested context field",
			template: "agent:{_context.run.agent_id}",
			data:     map[string]any{},
			context: map[string]any{
				"run": map[string]any{
					"agent_id": "a1",
				},
			},
			want:    "agent:a1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.InterpolateWithContext(tt.template, tt.data, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("InterpolateWithContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("InterpolateWithContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_RealWorldExamples(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		context  map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "domain node ID",
			template: "domain:{name}",
			data:     map[string]any{"name": "example.com"},
			want:     "domain:example.com",
			wantErr:  false,
		},
		{
			name:     "subdomain node ID",
			template: "subdomain:{name}",
			data:     map[string]any{"name": "api.example.com"},
			want:     "subdomain:api.example.com",
			wantErr:  false,
		},
		{
			name:     "host node ID",
			template: "host:{ip}",
			data:     map[string]any{"ip": "192.168.1.100"},
			want:     "host:192.168.1.100",
			wantErr:  false,
		},
		{
			name:     "port node ID",
			template: "port:{host_ip}:{port}",
			data: map[string]any{
				"host_ip": "192.168.1.100",
				"port":    443,
			},
			want:    "port:192.168.1.100:443",
			wantErr: false,
		},
		{
			name:     "technique node ID",
			template: "technique:{technique_id}",
			data:     map[string]any{"technique_id": "T1190"},
			want:     "technique:T1190",
			wantErr:  false,
		},
		{
			name:     "agent run ID with context",
			template: "agent_run:{trace_id}:{span_id}",
			data: map[string]any{
				"trace_id": "550e8400-e29b-41d4-a716-446655440000",
				"span_id":  "12345",
			},
			want:    "agent_run:550e8400-e29b-41d4-a716-446655440000:12345",
			wantErr: false,
		},
		{
			name:     "mission node ID",
			template: "mission:{mission_id}",
			data:     map[string]any{"mission_id": "m_reconnaissance_20240112"},
			want:     "mission:m_reconnaissance_20240112",
			wantErr:  false,
		},
		{
			name:     "finding with multiple fields",
			template: "finding:{target_id}:{vuln_type}:{timestamp}",
			data: map[string]any{
				"target_id":  "t123",
				"vuln_type":  "sqli",
				"timestamp":  1641945600,
			},
			want:    "finding:t123:sqli:1641945600",
			wantErr: false,
		},
		{
			name:     "complex nested structure",
			template: "service:{target.host.ip}:{service.port}:{service.name}",
			data: map[string]any{
				"target": map[string]any{
					"host": map[string]any{
						"ip": "10.0.0.1",
					},
				},
				"service": map[string]any{
					"port": 443,
					"name": "https",
				},
			},
			want:    "service:10.0.0.1:443:https",
			wantErr: false,
		},
		{
			name:     "execution event with context",
			template: "execution:{_context.agent_run_id}:{event_type}",
			data:     map[string]any{"event_type": "tool_start"},
			context:  map[string]any{"agent_run_id": "run_123"},
			want:     "execution:run_123:tool_start",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.InterpolateWithContext(tt.template, tt.data, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("InterpolateWithContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("InterpolateWithContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_EdgeCases(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "escaped braces (no interpolation)",
			template: "literal:{value}",
			data:     map[string]any{"value": "test"},
			want:     "literal:test",
			wantErr:  false,
		},
		{
			name:     "consecutive placeholders",
			template: "{a}{b}{c}",
			data: map[string]any{
				"a": "1",
				"b": "2",
				"c": "3",
			},
			want:    "123",
			wantErr: false,
		},
		{
			name:     "placeholder at start",
			template: "{id}:suffix",
			data:     map[string]any{"id": "prefix"},
			want:     "prefix:suffix",
			wantErr:  false,
		},
		{
			name:     "placeholder at end",
			template: "prefix:{id}",
			data:     map[string]any{"id": "suffix"},
			want:     "prefix:suffix",
			wantErr:  false,
		},
		{
			name:     "empty string value",
			template: "value:{field}",
			data:     map[string]any{"field": ""},
			want:     "value:",
			wantErr:  false,
		},
		{
			name:     "zero numeric value",
			template: "count:{count}",
			data:     map[string]any{"count": 0},
			want:     "count:0",
			wantErr:  false,
		},
		{
			name:     "false boolean value",
			template: "enabled:{enabled}",
			data:     map[string]any{"enabled": false},
			want:     "enabled:false",
			wantErr:  false,
		},
		{
			name:     "int64 value",
			template: "timestamp:{ts}",
			data:     map[string]any{"ts": int64(1641945600)},
			want:     "timestamp:1641945600",
			wantErr:  false,
		},
		{
			name:     "special characters in values",
			template: "url:{url}",
			data:     map[string]any{"url": "https://example.com/path?query=value"},
			want:     "url:https://example.com/path?query=value",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Interpolate(tt.template, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Interpolate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Interpolate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_TypeCoercion(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name     string
		template string
		data     map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "int to string",
			template: "port:{port}",
			data:     map[string]any{"port": 8080},
			want:     "port:8080",
			wantErr:  false,
		},
		{
			name:     "float64 to string (whole)",
			template: "count:{count}",
			data:     map[string]any{"count": 42.0},
			want:     "count:42",
			wantErr:  false,
		},
		{
			name:     "float64 to string (decimal)",
			template: "score:{score}",
			data:     map[string]any{"score": 3.14159},
			want:     "score:3.14159",
			wantErr:  false,
		},
		{
			name:     "bool to string (true)",
			template: "active:{active}",
			data:     map[string]any{"active": true},
			want:     "active:true",
			wantErr:  false,
		},
		{
			name:     "bool to string (false)",
			template: "active:{active}",
			data:     map[string]any{"active": false},
			want:     "active:false",
			wantErr:  false,
		},
		{
			name:     "nil value",
			template: "value:{value}",
			data:     map[string]any{"value": nil},
			want:     "",
			wantErr:  true,
		},
		{
			name:     "unsupported type",
			template: "value:{value}",
			data:     map[string]any{"value": []string{"a", "b"}},
			want:     "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Interpolate(tt.template, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Interpolate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Interpolate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateEngine_ErrorMessages(t *testing.T) {
	engine := NewTemplateEngine()

	tests := []struct {
		name         string
		template     string
		data         map[string]any
		wantErr      bool
		errSubstring string
	}{
		{
			name:         "missing field error",
			template:     "value:{missing}",
			data:         map[string]any{},
			wantErr:      true,
			errSubstring: "not found",
		},
		{
			name:         "missing nested field error",
			template:     "value:{obj.missing}",
			data:         map[string]any{"obj": map[string]any{"field": "value"}},
			wantErr:      true,
			errSubstring: "not found",
		},
		{
			name:         "array out of bounds error",
			template:     "value:{arr[10]}",
			data:         map[string]any{"arr": []any{"a", "b"}},
			wantErr:      true,
			errSubstring: "out of bounds",
		},
		{
			name:         "nil conversion error",
			template:     "value:{nil}",
			data:         map[string]any{"nil": nil},
			wantErr:      true,
			errSubstring: "cannot convert nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.Interpolate(tt.template, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Interpolate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && !containsSubstring(err.Error(), tt.errSubstring) {
				t.Errorf("Interpolate() error = %v, want substring %v", err, tt.errSubstring)
			}
		})
	}
}

func containsSubstring(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || containsAt(str, substr))
}

func containsAt(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkTemplateEngine_SimpleInterpolation(b *testing.B) {
	engine := NewTemplateEngine()
	template := "mission:{id}"
	data := map[string]any{"id": "m123"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Interpolate(template, data)
	}
}

func BenchmarkTemplateEngine_MultipleFields(b *testing.B) {
	engine := NewTemplateEngine()
	template := "run:{trace}:{span}:{agent}"
	data := map[string]any{
		"trace": "t1",
		"span":  "s2",
		"agent": "a3",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Interpolate(template, data)
	}
}

func BenchmarkTemplateEngine_NestedAccess(b *testing.B) {
	engine := NewTemplateEngine()
	template := "service:{target.host.ip}:{service.port}"
	data := map[string]any{
		"target": map[string]any{
			"host": map[string]any{
				"ip": "10.0.0.1",
			},
		},
		"service": map[string]any{
			"port": 443,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Interpolate(template, data)
	}
}

func BenchmarkTemplateEngine_WithContext(b *testing.B) {
	engine := NewTemplateEngine()
	template := "finding:{id}:by:{_context.agent_id}"
	data := map[string]any{"id": "f123"}
	context := map[string]any{"agent_id": "a1"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.InterpolateWithContext(template, data, context)
	}
}
