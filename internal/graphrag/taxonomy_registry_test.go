package graphrag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/sdk/graphrag"
)

// mockTaxonomyReader is a simple mock implementation of TaxonomyReader for testing
type mockTaxonomyReader struct {
	version               string
	canonicalNodeTypes    map[string]bool
	canonicalRelTypes     map[string]bool
	validateNodeResult    bool
	validateRelResult     bool
}

func newMockTaxonomyReader() *mockTaxonomyReader {
	return &mockTaxonomyReader{
		version:            "test-1.0.0",
		canonicalNodeTypes: make(map[string]bool),
		canonicalRelTypes:  make(map[string]bool),
		validateNodeResult: true,
		validateRelResult:  true,
	}
}

func (m *mockTaxonomyReader) Version() string {
	return m.version
}

func (m *mockTaxonomyReader) IsCanonicalNodeType(typeName string) bool {
	return m.canonicalNodeTypes[typeName]
}

func (m *mockTaxonomyReader) IsCanonicalRelationType(typeName string) bool {
	return m.canonicalRelTypes[typeName]
}

func (m *mockTaxonomyReader) ValidateNodeType(typeName string) bool {
	return m.validateNodeResult
}

func (m *mockTaxonomyReader) ValidateRelationType(typeName string) bool {
	return m.validateRelResult
}

func TestNewTaxonomyRegistry(t *testing.T) {
	delegate := newMockTaxonomyReader()
	registry := NewTaxonomyRegistry(delegate)

	require.NotNil(t, registry)
	assert.Equal(t, "test-1.0.0", registry.Version())
}

func TestTaxonomyRegistry_CoreRelationshipLookups(t *testing.T) {
	tests := []struct {
		name         string
		childType    string
		parentType   string
		wantRelName  string
		wantFound    bool
	}{
		{
			name:        "port to host has HAS_PORT",
			childType:   "port",
			parentType:  "host",
			wantRelName: graphrag.RelTypeHasPort,
			wantFound:   true,
		},
		{
			name:        "service to port has RUNS_SERVICE",
			childType:   "service",
			parentType:  "port",
			wantRelName: graphrag.RelTypeRunsService,
			wantFound:   true,
		},
		{
			name:        "endpoint to service has HAS_ENDPOINT",
			childType:   "endpoint",
			parentType:  "service",
			wantRelName: graphrag.RelTypeHasEndpoint,
			wantFound:   true,
		},
		{
			name:        "subdomain to domain has HAS_SUBDOMAIN",
			childType:   "subdomain",
			parentType:  "domain",
			wantRelName: graphrag.RelTypeHasSubdomain,
			wantFound:   true,
		},
		{
			name:        "certificate to host has SERVES_CERTIFICATE",
			childType:   "certificate",
			parentType:  "host",
			wantRelName: graphrag.RelTypeServesCertificate,
			wantFound:   true,
		},
		{
			name:        "api_endpoint to api has HAS_ENDPOINT",
			childType:   "api_endpoint",
			parentType:  "api",
			wantRelName: graphrag.RelTypeHasEndpoint,
			wantFound:   true,
		},
		{
			name:       "unknown child-parent combination returns not found",
			childType:  "unknown_child",
			parentType: "unknown_parent",
			wantFound:  false,
		},
		{
			name:       "reversed relationship not found",
			childType:  "host",
			parentType: "port",
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delegate := newMockTaxonomyReader()
			registry := NewTaxonomyRegistry(delegate)

			relName, found := registry.GetParentRelationship(tt.childType, tt.parentType)

			assert.Equal(t, tt.wantFound, found, "found status should match")
			if tt.wantFound {
				assert.Equal(t, tt.wantRelName, relName, "relationship name should match")
			} else {
				assert.Empty(t, relName, "relationship name should be empty when not found")
			}
		})
	}
}

func TestTaxonomyRegistry_IsAssetType(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		want     bool
	}{
		// Core asset types
		{
			name:     "domain is asset type",
			nodeType: graphrag.NodeTypeDomain,
			want:     true,
		},
		{
			name:     "subdomain is asset type",
			nodeType: graphrag.NodeTypeSubdomain,
			want:     true,
		},
		{
			name:     "host is asset type",
			nodeType: graphrag.NodeTypeHost,
			want:     true,
		},
		{
			name:     "port is asset type",
			nodeType: graphrag.NodeTypePort,
			want:     true,
		},
		{
			name:     "service is asset type",
			nodeType: graphrag.NodeTypeService,
			want:     true,
		},
		{
			name:     "endpoint is asset type",
			nodeType: graphrag.NodeTypeEndpoint,
			want:     true,
		},
		{
			name:     "api is asset type",
			nodeType: graphrag.NodeTypeApi,
			want:     true,
		},
		{
			name:     "technology is asset type",
			nodeType: graphrag.NodeTypeTechnology,
			want:     true,
		},
		{
			name:     "cloud_asset is asset type",
			nodeType: graphrag.NodeTypeCloudAsset,
			want:     true,
		},
		{
			name:     "certificate is asset type",
			nodeType: graphrag.NodeTypeCertificate,
			want:     true,
		},
		// Non-asset types
		{
			name:     "finding is not asset type",
			nodeType: "finding",
			want:     false,
		},
		{
			name:     "technique is not asset type",
			nodeType: "technique",
			want:     false,
		},
		{
			name:     "mission is not asset type",
			nodeType: "mission",
			want:     false,
		},
		{
			name:     "unknown type is not asset type",
			nodeType: "unknown_type",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delegate := newMockTaxonomyReader()
			registry := NewTaxonomyRegistry(delegate)

			result := registry.IsAssetType(tt.nodeType)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestTaxonomyRegistry_RegisterExtension(t *testing.T) {
	t.Run("register new extension successfully", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		extension := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{
					Name:     "custom_asset",
					Category: "Asset",
					Properties: []graphrag.PropertyInfo{
						{Name: "custom_field", Type: "string", Required: true},
					},
				},
			},
			Relationships: []RelationshipDefinition{
				{
					Name:      "CUSTOM_REL",
					FromTypes: []string{"custom_asset"},
					ToTypes:   []string{"host"},
				},
			},
		}

		err := registry.RegisterExtension("test-agent", extension)
		require.NoError(t, err)

		// Verify custom asset type is registered as asset
		assert.True(t, registry.IsAssetType("custom_asset"))

		// Verify relationship rule is registered
		relName, found := registry.GetParentRelationship("host", "custom_asset")
		assert.True(t, found)
		assert.Equal(t, "CUSTOM_REL", relName)
	})

	t.Run("register extension with multiple from/to types", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		extension := TaxonomyExtension{
			Relationships: []RelationshipDefinition{
				{
					Name:      "CONNECTS_TO",
					FromTypes: []string{"service_a", "service_b"},
					ToTypes:   []string{"backend_a", "backend_b"},
				},
			},
		}

		err := registry.RegisterExtension("test-agent", extension)
		require.NoError(t, err)

		// Verify all combinations are registered
		relName, found := registry.GetParentRelationship("backend_a", "service_a")
		assert.True(t, found)
		assert.Equal(t, "CONNECTS_TO", relName)

		relName, found = registry.GetParentRelationship("backend_b", "service_a")
		assert.True(t, found)
		assert.Equal(t, "CONNECTS_TO", relName)

		relName, found = registry.GetParentRelationship("backend_a", "service_b")
		assert.True(t, found)
		assert.Equal(t, "CONNECTS_TO", relName)

		relName, found = registry.GetParentRelationship("backend_b", "service_b")
		assert.True(t, found)
		assert.Equal(t, "CONNECTS_TO", relName)
	})

	t.Run("reject empty agent name", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		extension := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{Name: "custom_type", Category: "Asset"},
			},
		}

		err := registry.RegisterExtension("", extension)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agentName cannot be empty")
	})
}

func TestTaxonomyRegistry_ConflictHandling_LastInstalledWins(t *testing.T) {
	delegate := newMockTaxonomyReader()
	registry := NewTaxonomyRegistry(delegate)

	// First agent registers a relationship
	ext1 := TaxonomyExtension{
		Relationships: []RelationshipDefinition{
			{
				Name:      "FIRST_REL",
				FromTypes: []string{"parent_type"},
				ToTypes:   []string{"child_type"},
			},
		},
	}

	err := registry.RegisterExtension("agent-1", ext1)
	require.NoError(t, err)

	// Verify first relationship is registered
	relName, found := registry.GetParentRelationship("child_type", "parent_type")
	assert.True(t, found)
	assert.Equal(t, "FIRST_REL", relName)

	// Second agent registers overlapping relationship (conflict)
	ext2 := TaxonomyExtension{
		Relationships: []RelationshipDefinition{
			{
				Name:      "SECOND_REL",
				FromTypes: []string{"parent_type"},
				ToTypes:   []string{"child_type"},
			},
		},
	}

	err = registry.RegisterExtension("agent-2", ext2)
	require.NoError(t, err)

	// Verify last-installed wins (second relationship overwrites first)
	relName, found = registry.GetParentRelationship("child_type", "parent_type")
	assert.True(t, found)
	assert.Equal(t, "SECOND_REL", relName, "last-installed extension should win")
}

func TestTaxonomyRegistry_ConflictHandling_SameAgentReplacement(t *testing.T) {
	delegate := newMockTaxonomyReader()
	registry := NewTaxonomyRegistry(delegate)

	// First registration
	ext1 := TaxonomyExtension{
		NodeTypes: []NodeTypeDefinition{
			{Name: "type_v1", Category: "Asset"},
		},
		Relationships: []RelationshipDefinition{
			{
				Name:      "REL_V1",
				FromTypes: []string{"parent"},
				ToTypes:   []string{"child"},
			},
		},
	}

	err := registry.RegisterExtension("test-agent", ext1)
	require.NoError(t, err)

	assert.True(t, registry.IsAssetType("type_v1"))
	relName, _ := registry.GetParentRelationship("child", "parent")
	assert.Equal(t, "REL_V1", relName)

	// Re-register same agent with updated extension (replacement)
	ext2 := TaxonomyExtension{
		NodeTypes: []NodeTypeDefinition{
			{Name: "type_v2", Category: "Asset"},
		},
		Relationships: []RelationshipDefinition{
			{
				Name:      "REL_V2",
				FromTypes: []string{"parent"},
				ToTypes:   []string{"child"},
			},
		},
	}

	err = registry.RegisterExtension("test-agent", ext2)
	require.NoError(t, err)

	// Old types/relationships are replaced
	assert.True(t, registry.IsAssetType("type_v2"))
	relName, found := registry.GetParentRelationship("child", "parent")
	assert.True(t, found)
	assert.Equal(t, "REL_V2", relName, "replacement extension should overwrite")
}

func TestTaxonomyRegistry_UnregisterExtension(t *testing.T) {
	t.Run("unregister existing extension removes relationships and types", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		extension := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{Name: "custom_asset", Category: "Asset"},
			},
			Relationships: []RelationshipDefinition{
				{
					Name:      "CUSTOM_REL",
					FromTypes: []string{"custom_asset"},
					ToTypes:   []string{"host"},
				},
			},
		}

		err := registry.RegisterExtension("test-agent", extension)
		require.NoError(t, err)

		// Verify extension is active
		assert.True(t, registry.IsAssetType("custom_asset"))
		_, found := registry.GetParentRelationship("host", "custom_asset")
		assert.True(t, found)

		// Unregister extension
		err = registry.UnregisterExtension("test-agent")
		require.NoError(t, err)

		// Verify extension is removed
		assert.False(t, registry.IsAssetType("custom_asset"))
		_, found = registry.GetParentRelationship("host", "custom_asset")
		assert.False(t, found)
	})

	t.Run("unregister preserves core relationships", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		extension := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{Name: "custom_type", Category: "Asset"},
			},
		}

		err := registry.RegisterExtension("test-agent", extension)
		require.NoError(t, err)

		// Unregister extension
		err = registry.UnregisterExtension("test-agent")
		require.NoError(t, err)

		// Core relationships should still exist
		relName, found := registry.GetParentRelationship("port", "host")
		assert.True(t, found, "core relationships should remain after unregister")
		assert.Equal(t, graphrag.RelTypeHasPort, relName)

		// Core asset types should still exist
		assert.True(t, registry.IsAssetType(graphrag.NodeTypeHost))
	})

	t.Run("unregister non-existent extension is idempotent", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		// Unregister non-existent agent (should not error)
		err := registry.UnregisterExtension("non-existent-agent")
		assert.NoError(t, err, "unregister should be idempotent")
	})

	t.Run("unregister empty agent name returns error", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		err := registry.UnregisterExtension("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agentName cannot be empty")
	})

	t.Run("unregister preserves other agents extensions", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		registry := NewTaxonomyRegistry(delegate)

		// Register two agents with different extensions
		ext1 := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{Name: "agent1_type", Category: "Asset"},
			},
			Relationships: []RelationshipDefinition{
				{
					Name:      "AGENT1_REL",
					FromTypes: []string{"agent1_parent"},
					ToTypes:   []string{"agent1_child"},
				},
			},
		}

		ext2 := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{Name: "agent2_type", Category: "Asset"},
			},
			Relationships: []RelationshipDefinition{
				{
					Name:      "AGENT2_REL",
					FromTypes: []string{"agent2_parent"},
					ToTypes:   []string{"agent2_child"},
				},
			},
		}

		err := registry.RegisterExtension("agent-1", ext1)
		require.NoError(t, err)

		err = registry.RegisterExtension("agent-2", ext2)
		require.NoError(t, err)

		// Unregister agent-1
		err = registry.UnregisterExtension("agent-1")
		require.NoError(t, err)

		// Agent-1's extensions should be removed
		assert.False(t, registry.IsAssetType("agent1_type"))
		_, found := registry.GetParentRelationship("agent1_child", "agent1_parent")
		assert.False(t, found)

		// Agent-2's extensions should remain
		assert.True(t, registry.IsAssetType("agent2_type"))
		relName, found := registry.GetParentRelationship("agent2_child", "agent2_parent")
		assert.True(t, found)
		assert.Equal(t, "AGENT2_REL", relName)
	})
}

func TestTaxonomyRegistry_DelegatedMethods(t *testing.T) {
	t.Run("Version delegates to underlying reader", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		delegate.version = "custom-version-2.0.0"
		registry := NewTaxonomyRegistry(delegate)

		assert.Equal(t, "custom-version-2.0.0", registry.Version())
	})

	t.Run("IsCanonicalNodeType delegates to reader and checks extensions", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		delegate.canonicalNodeTypes["core_type"] = true
		registry := NewTaxonomyRegistry(delegate)

		// Core type should be canonical
		assert.True(t, registry.IsCanonicalNodeType("core_type"))

		// Unknown type should not be canonical
		assert.False(t, registry.IsCanonicalNodeType("unknown_type"))

		// Register extension with custom type
		extension := TaxonomyExtension{
			NodeTypes: []NodeTypeDefinition{
				{Name: "custom_type", Category: "Asset"},
			},
		}
		err := registry.RegisterExtension("test-agent", extension)
		require.NoError(t, err)

		// Custom type should now be canonical
		assert.True(t, registry.IsCanonicalNodeType("custom_type"))
	})

	t.Run("IsCanonicalRelationType delegates to reader and checks extensions", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		delegate.canonicalRelTypes["CORE_REL"] = true
		registry := NewTaxonomyRegistry(delegate)

		// Core relationship should be canonical
		assert.True(t, registry.IsCanonicalRelationType("CORE_REL"))

		// Unknown relationship should not be canonical
		assert.False(t, registry.IsCanonicalRelationType("UNKNOWN_REL"))

		// Register extension with custom relationship
		extension := TaxonomyExtension{
			Relationships: []RelationshipDefinition{
				{
					Name:      "CUSTOM_REL",
					FromTypes: []string{"parent"},
					ToTypes:   []string{"child"},
				},
			},
		}
		err := registry.RegisterExtension("test-agent", extension)
		require.NoError(t, err)

		// Custom relationship should now be canonical
		assert.True(t, registry.IsCanonicalRelationType("CUSTOM_REL"))
	})

	t.Run("ValidateNodeType delegates to reader", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		delegate.validateNodeResult = true
		registry := NewTaxonomyRegistry(delegate)

		assert.True(t, registry.ValidateNodeType("any_type"))

		delegate.validateNodeResult = false
		assert.False(t, registry.ValidateNodeType("any_type"))
	})

	t.Run("ValidateRelationType delegates to reader", func(t *testing.T) {
		delegate := newMockTaxonomyReader()
		delegate.validateRelResult = true
		registry := NewTaxonomyRegistry(delegate)

		assert.True(t, registry.ValidateRelationType("ANY_REL"))

		delegate.validateRelResult = false
		assert.False(t, registry.ValidateRelationType("ANY_REL"))
	})

	t.Run("nil delegate returns safe defaults", func(t *testing.T) {
		registry := NewTaxonomyRegistry(nil)

		assert.Equal(t, "unknown", registry.Version())
		assert.False(t, registry.IsCanonicalNodeType("any_type"))
		assert.False(t, registry.IsCanonicalRelationType("ANY_REL"))
		assert.True(t, registry.ValidateNodeType("any_type"))
		assert.True(t, registry.ValidateRelationType("ANY_REL"))
	})
}

func TestTaxonomyRegistry_GetParentRelationship_UnknownCombinations(t *testing.T) {
	delegate := newMockTaxonomyReader()
	registry := NewTaxonomyRegistry(delegate)

	tests := []struct {
		name       string
		childType  string
		parentType string
	}{
		{
			name:       "completely unknown types",
			childType:  "unknown_child",
			parentType: "unknown_parent",
		},
		{
			name:       "known child, unknown parent",
			childType:  "port",
			parentType: "unknown_parent",
		},
		{
			name:       "unknown child, known parent",
			childType:  "unknown_child",
			parentType: "host",
		},
		{
			name:       "reversed core relationship",
			childType:  "host",
			parentType: "port",
		},
		{
			name:       "unrelated known types",
			childType:  "service",
			parentType: "domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relName, found := registry.GetParentRelationship(tt.childType, tt.parentType)

			assert.False(t, found, "unknown combinations should return not found")
			assert.Empty(t, relName, "relationship name should be empty string")
		})
	}
}

func TestTaxonomyRegistry_ThreadSafety(t *testing.T) {
	// This test verifies that concurrent operations don't cause data races
	// Run with: go test -race ./internal/graphrag/...
	delegate := newMockTaxonomyReader()
	registry := NewTaxonomyRegistry(delegate)

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			registry.GetParentRelationship("port", "host")
			registry.IsAssetType("host")
			registry.IsCanonicalNodeType("host")
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		agentName := "agent-" + string(rune('A'+i))
		go func(name string) {
			ext := TaxonomyExtension{
				NodeTypes: []NodeTypeDefinition{
					{Name: name + "_type", Category: "Asset"},
				},
			}
			registry.RegisterExtension(name, ext)
			done <- true
		}(agentName)
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
}
