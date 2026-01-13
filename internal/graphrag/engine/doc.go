// Package engine provides the core template interpolation engine for the TaxonomyGraphEngine.
//
// # Overview
//
// The engine package implements a flexible template interpolation system for generating
// node IDs and property values in the Gibson GraphRAG taxonomy system. It supports
// various placeholder formats including simple fields, nested field access, array indexing,
// and special context variables.
//
// # Template Syntax
//
// Templates use curly braces {} to denote placeholders that will be replaced with
// values from the data map:
//
//   - Simple field: {field_name}
//   - Nested field: {parent.child.value}
//   - Array indexing: {items[0]}, {items[1].name}
//   - JSONPath-style: {.field} (relative path)
//   - Context variables: {_context.agent_id}, {_context.trace_id}
//   - Parent variables: {_parent.ip} (for hierarchical relationships)
//
// # Basic Usage
//
// Simple field interpolation:
//
//	engine := NewTemplateEngine()
//	result, err := engine.Interpolate("domain:{name}", map[string]any{
//	    "name": "example.com",
//	})
//	// result: "domain:example.com"
//
// Multiple fields:
//
//	result, err := engine.Interpolate("port:{ip}:{port}", map[string]any{
//	    "ip":   "192.168.1.1",
//	    "port": 443,
//	})
//	// result: "port:192.168.1.1:443"
//
// # Nested Field Access
//
// Access nested fields using dot notation:
//
//	data := map[string]any{
//	    "target": map[string]any{
//	        "host": map[string]any{
//	            "ip": "10.0.0.1",
//	        },
//	    },
//	    "service": map[string]any{
//	        "port": 443,
//	        "name": "https",
//	    },
//	}
//
//	result, err := engine.Interpolate(
//	    "service:{target.host.ip}:{service.port}:{service.name}",
//	    data,
//	)
//	// result: "service:10.0.0.1:443:https"
//
// # Array Access
//
// Access array elements using bracket notation:
//
//	data := map[string]any{
//	    "hosts": []any{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
//	}
//
//	result, err := engine.Interpolate("host:{hosts[0]}", data)
//	// result: "host:192.168.1.1"
//
// Array with nested fields:
//
//	data := map[string]any{
//	    "services": []any{
//	        map[string]any{"name": "http", "port": 80},
//	        map[string]any{"name": "https", "port": 443},
//	    },
//	}
//
//	result, err := engine.Interpolate("service:{services[1].name}", data)
//	// result: "service:https"
//
// # Context Variables
//
// Context variables provide additional metadata not present in the primary data:
//
//	data := map[string]any{
//	    "finding_id": "f123",
//	}
//
//	context := map[string]any{
//	    "agent_id":  "whistler",
//	    "trace_id":  "550e8400-e29b-41d4-a716-446655440000",
//	    "span_id":   "12345",
//	}
//
//	result, err := engine.InterpolateWithContext(
//	    "finding:{finding_id}:discovered_by:{_context.agent_id}",
//	    data,
//	    context,
//	)
//	// result: "finding:f123:discovered_by:whistler"
//
// # Real-World Examples
//
// Domain node ID:
//
//	template: "domain:{name}"
//	data: {"name": "example.com"}
//	result: "domain:example.com"
//
// Subdomain node ID:
//
//	template: "subdomain:{name}"
//	data: {"name": "api.example.com"}
//	result: "subdomain:api.example.com"
//
// Host node ID:
//
//	template: "host:{ip}"
//	data: {"ip": "192.168.1.100"}
//	result: "host:192.168.1.100"
//
// Port node ID:
//
//	template: "port:{host_ip}:{port}"
//	data: {"host_ip": "192.168.1.100", "port": 443}
//	result: "port:192.168.1.100:443"
//
// Technique node ID:
//
//	template: "technique:{technique_id}"
//	data: {"technique_id": "T1190"}
//	result: "technique:T1190"
//
// Agent run with tracing:
//
//	template: "agent_run:{trace_id}:{span_id}"
//	data: {"trace_id": "t1", "span_id": "s2"}
//	result: "agent_run:t1:s2"
//
// Execution event with context:
//
//	template: "execution:{_context.agent_run_id}:{event_type}"
//	data: {"event_type": "tool_start"}
//	context: {"agent_run_id": "run_123"}
//	result: "execution:run_123:tool_start"
//
// # Type Coercion
//
// The engine automatically converts values to strings:
//
//   - Integers: 8080 → "8080"
//   - Floats: 3.14 → "3.14", 100.0 → "100"
//   - Booleans: true → "true", false → "false"
//   - Strings: passed through unchanged
//
// # Error Handling
//
// The engine returns errors for:
//
//   - Missing fields: "field not found in data"
//   - Array out of bounds: "array index N out of bounds"
//   - Type mismatches: "cannot access field on non-object type"
//   - Nil values: "cannot convert nil to string"
//   - Unsupported types: "cannot convert type T to string"
//
// Example error handling:
//
//	result, err := engine.Interpolate("missing:{field}", map[string]any{})
//	if err != nil {
//	    // Handle error: "failed to resolve placeholder {field}: field not found"
//	}
//
// # Performance
//
// Benchmark results on Intel i7-4770K @ 3.50GHz:
//
//   - Simple interpolation: ~738 ns/op, 395 B/op, 6 allocs/op
//   - Multiple fields: ~1,688 ns/op, 395 B/op, 6 allocs/op
//   - Nested access: ~12,015 ns/op, 8,111 B/op, 83 allocs/op
//   - With context: ~6,694 ns/op, 4,549 B/op, 46 allocs/op
//
// The engine is optimized for common use cases while maintaining flexibility.
//
// # Thread Safety
//
// TemplateEngine is safe for concurrent use. Multiple goroutines can call
// Interpolate and InterpolateWithContext simultaneously.
package engine
