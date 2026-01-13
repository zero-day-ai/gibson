package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractHosts(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{"ip": "1.2.3.4", "hostname": "host1"},
			map[string]any{"ip": "5.6.7.8", "hostname": "host2"},
		},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.Extract("$.hosts[*]", data)

	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify first host
	host1, ok := results[0].(map[string]any)
	require.True(t, ok, "First result should be a map")
	assert.Equal(t, "1.2.3.4", host1["ip"])
	assert.Equal(t, "host1", host1["hostname"])

	// Verify second host
	host2, ok := results[1].(map[string]any)
	require.True(t, ok, "Second result should be a map")
	assert.Equal(t, "5.6.7.8", host2["ip"])
	assert.Equal(t, "host2", host2["hostname"])
}

func TestExtractNestedPorts(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip": "1.2.3.4",
				"ports": []any{
					map[string]any{"port": 22, "service": "ssh"},
					map[string]any{"port": 80, "service": "http"},
				},
			},
		},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.ExtractWithParent("$.hosts[*].ports[*]", data)

	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Each result should have parent reference with ip
	assert.NotNil(t, results[0].Parent)
	assert.Equal(t, "1.2.3.4", results[0].Parent["ip"])

	// Verify first port
	port1, ok := results[0].Value.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 22, port1["port"])
	assert.Equal(t, "ssh", port1["service"])

	// Verify second port
	port2, ok := results[1].Value.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 80, port2["port"])
	assert.Equal(t, "http", port2["service"])
}

func TestExtractMultipleHostsWithPorts(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip": "1.2.3.4",
				"ports": []any{
					map[string]any{"port": 22, "service": "ssh"},
					map[string]any{"port": 80, "service": "http"},
				},
			},
			map[string]any{
				"ip": "5.6.7.8",
				"ports": []any{
					map[string]any{"port": 443, "service": "https"},
				},
			},
		},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.ExtractWithParent("$.hosts[*].ports[*]", data)

	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify parent IPs are correct
	assert.Equal(t, "1.2.3.4", results[0].Parent["ip"])
	assert.Equal(t, "1.2.3.4", results[1].Parent["ip"])
	assert.Equal(t, "5.6.7.8", results[2].Parent["ip"])

	// Verify port values
	port1 := results[0].Value.(map[string]any)
	assert.Equal(t, 22, port1["port"])

	port2 := results[1].Value.(map[string]any)
	assert.Equal(t, 80, port2["port"])

	port3 := results[2].Value.(map[string]any)
	assert.Equal(t, 443, port3["port"])
}

func TestExtractSingle(t *testing.T) {
	data := map[string]any{
		"host": map[string]any{
			"ip":       "1.2.3.4",
			"hostname": "example.com",
		},
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.ExtractSingle("$.host.ip", data)

	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", result)
}

func TestExtractSingleMultipleMatches(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{"ip": "1.2.3.4"},
			map[string]any{"ip": "5.6.7.8"},
		},
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.ExtractSingle("$.hosts[*].ip", data)

	require.NoError(t, err)
	// Should return first match
	assert.Equal(t, "1.2.3.4", result)
}

func TestExtractRelative(t *testing.T) {
	context := map[string]any{
		"ip":       "1.2.3.4",
		"hostname": "example.com",
		"service": map[string]any{
			"name":    "http",
			"version": "2.0",
		},
	}

	extractor := NewJSONPathExtractor()

	// Simple field access
	result, err := extractor.ExtractRelative(".ip", context)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", result)

	// Nested field access
	result, err = extractor.ExtractRelative(".service.name", context)
	require.NoError(t, err)
	assert.Equal(t, "http", result)
}

func TestExtractRelativeArrayAccess(t *testing.T) {
	context := map[string]any{
		"ports": []any{
			map[string]any{"number": 22},
			map[string]any{"number": 80},
		},
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.ExtractRelative(".ports[0]", context)

	require.NoError(t, err)
	port, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 22, port["number"])
}

func TestGetNestedValue(t *testing.T) {
	current := map[string]any{
		"ip":   "1.2.3.4",
		"port": 80,
		"service": map[string]any{
			"name":    "http",
			"version": "2.0",
		},
	}

	extractor := NewJSONPathExtractor()

	// Simple field
	result, err := extractor.GetNestedValue("ip", current, nil)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", result)

	// Nested field
	result, err = extractor.GetNestedValue("service.name", current, nil)
	require.NoError(t, err)
	assert.Equal(t, "http", result)
}

func TestGetNestedValueWithParent(t *testing.T) {
	parent := map[string]any{
		"ip":       "1.2.3.4",
		"hostname": "host1",
	}

	current := map[string]any{
		"port": 80,
		"service": map[string]any{
			"name": "http",
		},
	}

	extractor := NewJSONPathExtractor()

	// Access parent field
	result, err := extractor.GetNestedValue("_parent.ip", current, parent)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", result)

	// Access parent nested field
	result, err = extractor.GetNestedValue("_parent.hostname", current, parent)
	require.NoError(t, err)
	assert.Equal(t, "host1", result)

	// Access current field
	result, err = extractor.GetNestedValue("port", current, parent)
	require.NoError(t, err)
	assert.Equal(t, 80, result)
}

func TestGetNestedValueEntireParent(t *testing.T) {
	parent := map[string]any{
		"ip":       "1.2.3.4",
		"hostname": "host1",
	}

	current := map[string]any{
		"port": 80,
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.GetNestedValue("_parent", current, parent)

	require.NoError(t, err)
	parentMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1.2.3.4", parentMap["ip"])
	assert.Equal(t, "host1", parentMap["hostname"])
}

func TestExtractEmptyArray(t *testing.T) {
	data := map[string]any{
		"hosts": []any{},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.Extract("$.hosts[*]", data)

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestExtractMissingPath(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{"ip": "1.2.3.4"},
		},
	}

	extractor := NewJSONPathExtractor()

	// Missing field should return empty, not error
	results, err := extractor.Extract("$.hosts[*].nonexistent", data)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestExtractInvalidJSONPath(t *testing.T) {
	data := map[string]any{
		"hosts": []any{},
	}

	extractor := NewJSONPathExtractor()
	_, err := extractor.Extract("$[[[invalid", data)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSONPath")
}

func TestExtractRelativeInvalidPath(t *testing.T) {
	context := map[string]any{
		"ip": "1.2.3.4",
	}

	extractor := NewJSONPathExtractor()
	_, err := extractor.ExtractRelative("ip", context) // Missing leading dot

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must start with '.'")
}

func TestExtractWithParentMissingField(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip": "1.2.3.4",
				// No ports field
			},
		},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.ExtractWithParent("$.hosts[*].ports[*]", data)

	require.NoError(t, err)
	assert.Empty(t, results) // Should return empty, not error
}

func TestExtractDeeplyNestedStructure(t *testing.T) {
	data := map[string]any{
		"scan": map[string]any{
			"results": map[string]any{
				"hosts": []any{
					map[string]any{
						"ip": "1.2.3.4",
						"services": []any{
							map[string]any{
								"name": "http",
								"vulnerabilities": []any{
									map[string]any{"id": "CVE-2023-1234"},
									map[string]any{"id": "CVE-2023-5678"},
								},
							},
						},
					},
				},
			},
		},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.Extract("$.scan.results.hosts[*].services[*].vulnerabilities[*].id", data)

	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "CVE-2023-1234", results[0])
	assert.Equal(t, "CVE-2023-5678", results[1])
}

func TestExtractArrayIndex(t *testing.T) {
	data := map[string]any{
		"hosts": []any{
			map[string]any{"ip": "1.2.3.4"},
			map[string]any{"ip": "5.6.7.8"},
			map[string]any{"ip": "9.10.11.12"},
		},
	}

	extractor := NewJSONPathExtractor()

	// First element
	result, err := extractor.ExtractSingle("$.hosts[0].ip", data)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", result)

	// Last element
	result, err = extractor.ExtractSingle("$.hosts[2].ip", data)
	require.NoError(t, err)
	assert.Equal(t, "9.10.11.12", result)
}

func TestExtractMixedTypes(t *testing.T) {
	data := map[string]any{
		"config": map[string]any{
			"enabled": true,
			"timeout": 30,
			"weight":  3.14,
			"tags":    []any{"web", "api"},
		},
	}

	extractor := NewJSONPathExtractor()

	// Boolean
	result, err := extractor.ExtractSingle("$.config.enabled", data)
	require.NoError(t, err)
	assert.Equal(t, true, result)

	// Integer
	result, err = extractor.ExtractSingle("$.config.timeout", data)
	require.NoError(t, err)
	assert.Equal(t, 30, result)

	// Float
	result, err = extractor.ExtractSingle("$.config.weight", data)
	require.NoError(t, err)
	assert.Equal(t, 3.14, result)

	// Array
	results, err := extractor.Extract("$.config.tags[*]", data)
	require.NoError(t, err)
	assert.Equal(t, []any{"web", "api"}, results)
}

func TestGetNestedValueMissingField(t *testing.T) {
	current := map[string]any{
		"ip": "1.2.3.4",
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.GetNestedValue("nonexistent", current, nil)

	// Should return nil, not error
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetNestedValueNonMapAccess(t *testing.T) {
	current := map[string]any{
		"ip": "1.2.3.4",
	}

	extractor := NewJSONPathExtractor()
	_, err := extractor.GetNestedValue("ip.invalid", current, nil)

	// Trying to access field on string should error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a map")
}

func TestGetNestedValueParentWithoutContext(t *testing.T) {
	current := map[string]any{
		"port": 80,
	}

	extractor := NewJSONPathExtractor()
	_, err := extractor.GetNestedValue("_parent.ip", current, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent context is nil")
}

func TestExtractWithParentThreeLevels(t *testing.T) {
	// This tests deeply nested extraction: hosts -> ports -> services
	data := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip": "1.2.3.4",
				"ports": []any{
					map[string]any{
						"number": 80,
						"services": []any{
							map[string]any{"name": "http", "version": "2.0"},
							map[string]any{"name": "nginx", "version": "1.21"},
						},
					},
				},
			},
		},
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.ExtractWithParent("$.hosts[*].ports[*].services[*]", data)

	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Each service should have parent port
	assert.NotNil(t, results[0].Parent)
	assert.Equal(t, 80, results[0].Parent["number"])

	// Verify service values
	service1 := results[0].Value.(map[string]any)
	assert.Equal(t, "http", service1["name"])

	service2 := results[1].Value.(map[string]any)
	assert.Equal(t, "nginx", service2["name"])
}

func TestExtractSingleNilData(t *testing.T) {
	data := map[string]any{
		"config": map[string]any{
			"value": nil,
		},
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.ExtractSingle("$.config.value", data)

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExtractRootPath(t *testing.T) {
	data := map[string]any{
		"ip":       "1.2.3.4",
		"hostname": "example.com",
	}

	extractor := NewJSONPathExtractor()
	results, err := extractor.Extract("$", data)

	require.NoError(t, err)
	assert.Len(t, results, 1)

	result, ok := results[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1.2.3.4", result["ip"])
	assert.Equal(t, "example.com", result["hostname"])
}

func TestExtractArrayPaths(t *testing.T) {
	extractor := NewJSONPathExtractor()

	tests := []struct {
		name     string
		jsonPath string
		expected []string
	}{
		{
			name:     "single array",
			jsonPath: "$.hosts[*]",
			expected: []string{"$.hosts[*]"},
		},
		{
			name:     "nested arrays",
			jsonPath: "$.hosts[*].ports[*]",
			expected: []string{"$.hosts[*]", "ports[*]"},
		},
		{
			name:     "three levels",
			jsonPath: "$.hosts[*].ports[*].services[*]",
			expected: []string{"$.hosts[*]", "ports[*]", "services[*]"},
		},
		{
			name:     "no arrays",
			jsonPath: "$.host.ip",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.extractArrayPaths(tt.jsonPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractEmptyString(t *testing.T) {
	data := map[string]any{
		"host": map[string]any{
			"name": "",
		},
	}

	extractor := NewJSONPathExtractor()
	result, err := extractor.ExtractSingle("$.host.name", data)

	require.NoError(t, err)
	assert.Equal(t, "", result)
}
