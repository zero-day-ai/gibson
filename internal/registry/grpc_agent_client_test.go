package registry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/sdk/registry"
)

func TestGRPCAgentClient_BasicMetadata(t *testing.T) {
	// Create a mock ServiceInfo with metadata
	info := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "test-agent",
		Version:    "1.0.0",
		InstanceID: "test-instance-123",
		Endpoint:   "localhost:50051",
		Metadata: map[string]string{
			"description":     "Test agent for security testing",
			"capabilities":    "prompt_injection,jailbreak,data_extraction",
			"target_types":    "llm_chat,llm_api,rag_system",
			"technique_types": "prompt_injection,model_extraction,jailbreak",
		},
		StartedAt: time.Now(),
	}

	// Create client with nil connection (we're only testing metadata parsing)
	var conn *grpc.ClientConn = nil
	client := NewGRPCAgentClient(conn, info)

	// Test basic metadata
	assert.Equal(t, "test-agent", client.Name())
	assert.Equal(t, "1.0.0", client.Version())
	assert.Equal(t, "Test agent for security testing", client.Description())

	// Test capabilities parsing
	caps := client.Capabilities()
	require.Len(t, caps, 3)
	assert.Contains(t, caps, "prompt_injection")
	assert.Contains(t, caps, "jailbreak")
	assert.Contains(t, caps, "data_extraction")

	// Test target types parsing
	targets := client.TargetTypes()
	require.Len(t, targets, 3)
	assert.Contains(t, targets, component.TargetType("llm_chat"))
	assert.Contains(t, targets, component.TargetType("llm_api"))
	assert.Contains(t, targets, component.TargetType("rag_system"))

	// Test technique types parsing
	techniques := client.TechniqueTypes()
	require.Len(t, techniques, 3)
	assert.Contains(t, techniques, component.TechniqueType("prompt_injection"))
	assert.Contains(t, techniques, component.TechniqueType("model_extraction"))
	assert.Contains(t, techniques, component.TechniqueType("jailbreak"))
}

func TestGRPCAgentClient_EmptyMetadata(t *testing.T) {
	// Create ServiceInfo with empty metadata
	info := registry.ServiceInfo{
		Kind:       "agent",
		Name:       "minimal-agent",
		Version:    "0.1.0",
		InstanceID: "minimal-instance",
		Endpoint:   "localhost:50052",
		Metadata:   map[string]string{},
		StartedAt:  time.Now(),
	}

	var conn *grpc.ClientConn = nil
	client := NewGRPCAgentClient(conn, info)

	// Should handle empty metadata gracefully
	assert.Equal(t, "minimal-agent", client.Name())
	assert.Equal(t, "0.1.0", client.Version())
	assert.Equal(t, "", client.Description())
	assert.Empty(t, client.Capabilities())
	assert.Empty(t, client.TargetTypes())
	assert.Empty(t, client.TechniqueTypes())
}

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "normal input",
			input:    "a,b,c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with spaces",
			input:    "a, b, c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with extra spaces",
			input:    "  a  ,  b  ,  c  ",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single value",
			input:    "single",
			expected: []string{"single"},
		},
		{
			name:     "empty parts",
			input:    "a,,b,,,c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "only commas",
			input:    ",,,",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommaSeparated(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertTargetTypes(t *testing.T) {
	input := []string{"llm_chat", "llm_api", "rag_system"}
	result := convertTargetTypes(input)

	require.Len(t, result, 3)
	assert.Equal(t, component.TargetType("llm_chat"), result[0])
	assert.Equal(t, component.TargetType("llm_api"), result[1])
	assert.Equal(t, component.TargetType("rag_system"), result[2])
}

func TestConvertTechniqueTypes(t *testing.T) {
	input := []string{"prompt_injection", "jailbreak", "model_extraction"}
	result := convertTechniqueTypes(input)

	require.Len(t, result, 3)
	assert.Equal(t, component.TechniqueType("prompt_injection"), result[0])
	assert.Equal(t, component.TechniqueType("jailbreak"), result[1])
	assert.Equal(t, component.TechniqueType("model_extraction"), result[2])
}
