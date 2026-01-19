package engine_test

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/graphrag/engine"
)

// ExampleTemplateEngine_simple demonstrates basic template interpolation
func ExampleTemplateEngine_simple() {
	e := engine.NewTemplateEngine()

	result, _ := e.Interpolate("domain:{name}", map[string]any{
		"name": "example.com",
	})

	fmt.Println(result)
	// Output: domain:example.com
}

// ExampleTemplateEngine_multiple demonstrates multiple field interpolation
func ExampleTemplateEngine_multiple() {
	e := engine.NewTemplateEngine()

	result, _ := e.Interpolate("port:{ip}:{port}", map[string]any{
		"ip":   "192.168.1.1",
		"port": 443,
	})

	fmt.Println(result)
	// Output: port:192.168.1.1:443
}

// ExampleTemplateEngine_nested demonstrates nested field access
func ExampleTemplateEngine_nested() {
	e := engine.NewTemplateEngine()

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

	result, _ := e.Interpolate("service:{target.host.ip}:{service.port}", data)

	fmt.Println(result)
	// Output: service:10.0.0.1:443
}

// ExampleTemplateEngine_array demonstrates array indexing
func ExampleTemplateEngine_array() {
	e := engine.NewTemplateEngine()

	data := map[string]any{
		"hosts": []any{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
	}

	result, _ := e.Interpolate("host:{hosts[0]}", data)

	fmt.Println(result)
	// Output: host:192.168.1.1
}

// ExampleTemplateEngine_context demonstrates context variables
func ExampleTemplateEngine_context() {
	e := engine.NewTemplateEngine()

	data := map[string]any{
		"finding_id": "f123",
	}

	context := map[string]any{
		"agent_id": "whistler",
	}

	result, _ := e.InterpolateWithContext(
		"finding:{finding_id}:by:{_context.agent_id}",
		data,
		context,
	)

	fmt.Println(result)
	// Output: finding:f123:by:whistler
}

// ExampleTemplateEngine_typeCoercion demonstrates automatic type conversion
func ExampleTemplateEngine_typeCoercion() {
	e := engine.NewTemplateEngine()

	data := map[string]any{
		"port":   8080,
		"active": true,
		"score":  99.5,
	}

	result1, _ := e.Interpolate("port:{port}", data)
	result2, _ := e.Interpolate("active:{active}", data)
	result3, _ := e.Interpolate("score:{score}", data)

	fmt.Println(result1)
	fmt.Println(result2)
	fmt.Println(result3)
	// Output:
	// port:8080
	// active:true
	// score:99.5
}

// ExampleTemplateEngine_realWorld demonstrates a complete real-world scenario
func ExampleTemplateEngine_realWorld() {
	e := engine.NewTemplateEngine()

	// Simulate discovering a subdomain during a recon mission
	data := map[string]any{
		"name":       "api.example.com",
		"ip":         "192.168.1.100",
		"status":     "active",
		"discovered": "2024-01-12T10:30:00Z",
	}

	context := map[string]any{
		"agent_id":   "bishop",
		"mission_id": "recon_001",
		"trace_id":   "550e8400-e29b-41d4-a716-446655440000",
	}

	// Generate node ID for the subdomain
	nodeID, _ := e.Interpolate("subdomain:{name}", data)
	fmt.Printf("Node ID: %s\n", nodeID)

	// Generate discovery event ID with context
	eventID, _ := e.InterpolateWithContext(
		"discovery:{_context.mission_id}:{name}",
		data,
		context,
	)
	fmt.Printf("Event ID: %s\n", eventID)

	// Generate relationship to the discovering agent
	relID, _ := e.InterpolateWithContext(
		"discovered_by:{name}:{_context.agent_id}",
		data,
		context,
	)
	fmt.Printf("Relationship: %s\n", relID)

	// Output:
	// Node ID: subdomain:api.example.com
	// Event ID: discovery:recon_001:api.example.com
	// Relationship: discovered_by:api.example.com:bishop
}
