// Package parser provides YAML parsing functionality for taxonomy files.
package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseYAMLBytes_ValidTaxonomy(t *testing.T) {
	yaml := `
version: "1.0.0"
kind: core
node_types:
  - name: host
    category: asset
    description: A network host
    properties:
      - name: ip
        type: string
        required: true
        description: IP address
      - name: hostname
        type: string
        description: Hostname

relationship_types:
  - name: HAS_PORT
    description: Host has a port
    from_types: [host]
    to_types: [port]
`

	taxonomy, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.NoError(t, err)
	require.NotNil(t, taxonomy)

	assert.Equal(t, "1.0.0", taxonomy.Version)
	assert.Equal(t, "core", taxonomy.Kind)
	assert.Len(t, taxonomy.NodeTypes, 1)
	assert.Equal(t, "host", taxonomy.NodeTypes[0].Name)
	assert.Len(t, taxonomy.NodeTypes[0].Properties, 2)
	assert.Len(t, taxonomy.RelationshipTypes, 1)
	assert.Equal(t, "HAS_PORT", taxonomy.RelationshipTypes[0].Name)
}

func TestParseYAMLBytes_MissingVersion(t *testing.T) {
	yaml := `
node_types:
  - name: host
    category: asset
    description: A network host
`

	_, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestParseYAMLBytes_MissingKind(t *testing.T) {
	yaml := `
version: "1.0.0"
node_types:
  - name: host
    category: asset
`

	_, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind is required")
}

func TestParseYAMLBytes_MissingNodeTypes(t *testing.T) {
	yaml := `
version: "1.0.0"
kind: core
`

	_, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one node type is required")
}

func TestParseYAMLBytes_InvalidYAML(t *testing.T) {
	yaml := `
version: "1.0.0
node_types: [
`

	_, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestParseYAMLBytes_NodeTypeMissingName(t *testing.T) {
	yaml := `
version: "1.0.0"
kind: core
node_types:
  - category: asset
    description: A network host
`

	_, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestParseYAMLBytes_PropertyMissingName(t *testing.T) {
	yaml := `
version: "1.0.0"
kind: core
node_types:
  - name: host
    category: asset
    description: A network host
    properties:
      - type: string
        required: true
`

	_, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "property name is required")
}

func TestParseYAMLBytes_WithValidations(t *testing.T) {
	yaml := `
version: "1.0.0"
kind: core
node_types:
  - name: port
    category: asset
    description: A network port
    properties:
      - name: number
        type: int32
        required: true
    validations:
      - rule: "port.number >= 1 && port.number <= 65535"
        message: "Port number must be between 1 and 65535"
`

	taxonomy, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.NoError(t, err)
	require.NotNil(t, taxonomy)

	assert.Len(t, taxonomy.NodeTypes[0].Validations, 1)
	assert.Contains(t, taxonomy.NodeTypes[0].Validations[0].Rule, "port.number")
}

func TestParseYAMLBytes_WithParentConfig(t *testing.T) {
	yaml := `
version: "1.0.0"
kind: core
node_types:
  - name: port
    category: asset
    description: A network port
    properties:
      - name: number
        type: int32
        required: true
    parent:
      type: host
      relationship: HAS_PORT
      required: true
`

	taxonomy, err := ParseYAMLBytes([]byte(yaml), "test.yaml")
	require.NoError(t, err)
	require.NotNil(t, taxonomy)

	require.NotNil(t, taxonomy.NodeTypes[0].Parent)
	assert.Equal(t, "host", taxonomy.NodeTypes[0].Parent.Type)
	assert.Equal(t, "HAS_PORT", taxonomy.NodeTypes[0].Parent.Relationship)
	assert.True(t, taxonomy.NodeTypes[0].Parent.Required)
}

func TestValidatePropertyType(t *testing.T) {
	validTypes := []string{"string", "int32", "int64", "float64", "bool", "timestamp", "bytes"}
	for _, pt := range validTypes {
		assert.True(t, ValidatePropertyType(pt), "type %s should be valid", pt)
	}

	invalidTypes := []string{"invalid", "Integer", "String", "", "float", "[]string"}
	for _, pt := range invalidTypes {
		assert.False(t, ValidatePropertyType(pt), "type %s should be invalid", pt)
	}
}

func TestParseError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ParseError
		expected string
	}{
		{
			name:     "with line number",
			err:      ParseError{Path: "test.yaml", Line: 10, Message: "syntax error"},
			expected: "test.yaml:10: syntax error",
		},
		{
			name:     "without line number",
			err:      ParseError{Path: "test.yaml", Line: 0, Message: "file not found"},
			expected: "test.yaml: file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}
