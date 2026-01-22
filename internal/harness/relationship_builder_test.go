package harness

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
)

// MockTaxonomyRegistry is a mock implementation of TaxonomyRegistry for testing.
type MockTaxonomyRegistry struct {
	mock.Mock
}

func (m *MockTaxonomyRegistry) GetParentRelationship(childType, parentType string) (string, bool) {
	args := m.Called(childType, parentType)
	return args.String(0), args.Bool(1)
}

func (m *MockTaxonomyRegistry) IsAssetType(nodeType string) bool {
	args := m.Called(nodeType)
	return args.Bool(0)
}

// MockNodeStore is a mock implementation of NodeStore for testing.
type MockNodeStore struct {
	mock.Mock
}

func (m *MockNodeStore) GetNode(ctx context.Context, nodeID types.ID) (*graphrag.GraphNode, error) {
	args := m.Called(ctx, nodeID)
	if node := args.Get(0); node != nil {
		return node.(*graphrag.GraphNode), args.Error(1)
	}
	return nil, args.Error(1)
}

// TestNewRelationshipBuilder verifies the constructor.
func TestNewRelationshipBuilder(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	assert.NotNil(t, builder)
}

// TestNewRelationshipBuilder_NilLogger verifies nil logger defaults to slog.Default().
func TestNewRelationshipBuilder_NilLogger(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)

	builder := NewRelationshipBuilder(registry, store, nil)

	assert.NotNil(t, builder)
}

// TestBuildRelationships_AssetType_CreatesDiscoveredRelationship tests that asset types
// create a DISCOVERED relationship from AgentRun to the node.
func TestBuildRelationships_AssetType_CreatesDiscoveredRelationship(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a host node (asset type)
	hostID := types.NewID()
	hostNode := graphrag.NewGraphNode(hostID, graphrag.NodeType(graphrag.NodeTypeHost))
	hostNode.WithProperty("ip", "192.168.1.100")

	// Setup mock expectations
	registry.On("IsAssetType", graphrag.NodeTypeHost).Return(true)

	// Create context with agent_run_id (must be valid UUID)
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, hostNode)

	// Verify
	require.NoError(t, err)
	require.Len(t, relationships, 1)

	rel := relationships[0]
	assert.Equal(t, graphrag.RelationType(graphrag.RelTypeDiscovered), rel.Type)
	assert.Equal(t, agentRunID, rel.FromID)
	assert.Equal(t, hostID, rel.ToID)

	registry.AssertExpectations(t)
}

// TestBuildRelationships_ParentRelationship_CreatesParentRelationship tests that nodes
// with parent_id create the appropriate parent relationship from taxonomy.
func TestBuildRelationships_ParentRelationship_CreatesParentRelationship(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a port node with parent_id pointing to host
	hostID := types.NewID()
	portID := types.NewID()
	portNode := graphrag.NewGraphNode(portID, graphrag.NodeType(graphrag.NodeTypePort))
	portNode.WithProperty("number", 443)
	portNode.WithProperty("parent_id", hostID.String())

	// Create parent host node
	hostNode := graphrag.NewGraphNode(hostID, graphrag.NodeType(graphrag.NodeTypeHost))
	hostNode.WithProperty("ip", "192.168.1.100")

	// Setup mock expectations
	registry.On("IsAssetType", graphrag.NodeTypePort).Return(true)
	store.On("GetNode", mock.Anything, hostID).Return(hostNode, nil)
	registry.On("GetParentRelationship", graphrag.NodeTypePort, graphrag.NodeTypeHost).
		Return(graphrag.RelTypeHasPort, true)

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, portNode)

	// Verify
	require.NoError(t, err)
	require.Len(t, relationships, 2) // DISCOVERED + HAS_PORT

	// Find the parent relationship (HAS_PORT)
	var parentRel *graphrag.Relationship
	for _, rel := range relationships {
		if rel.Type == graphrag.RelationType(graphrag.RelTypeHasPort) {
			parentRel = rel
			break
		}
	}

	require.NotNil(t, parentRel, "Expected HAS_PORT relationship")
	assert.Equal(t, hostID, parentRel.FromID)
	assert.Equal(t, portID, parentRel.ToID)

	registry.AssertExpectations(t)
	store.AssertExpectations(t)
}

// TestBuildRelationships_ParentNotFound_LogsWarning tests that when a parent node
// is not found, a warning is logged and the relationship is skipped.
func TestBuildRelationships_ParentNotFound_LogsWarning(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a port node with parent_id pointing to non-existent host
	hostID := types.NewID()
	portID := types.NewID()
	portNode := graphrag.NewGraphNode(portID, graphrag.NodeType(graphrag.NodeTypePort))
	portNode.WithProperty("number", 443)
	portNode.WithProperty("parent_id", hostID.String())

	// Setup mock expectations - parent not found
	registry.On("IsAssetType", graphrag.NodeTypePort).Return(true)
	store.On("GetNode", mock.Anything, hostID).Return(nil, errors.New("node not found"))

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, portNode)

	// Verify - should only have DISCOVERED relationship, no parent relationship
	require.NoError(t, err)
	require.Len(t, relationships, 1)

	// Verify only DISCOVERED relationship exists
	assert.Equal(t, graphrag.RelationType(graphrag.RelTypeDiscovered), relationships[0].Type)

	registry.AssertExpectations(t)
	store.AssertExpectations(t)
}

// TestBuildRelationships_CustomTaxonomyExtension tests that custom taxonomy
// relationships work correctly.
func TestBuildRelationships_CustomTaxonomyExtension(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a custom node type (e.g., "api_key" as child of "service")
	serviceID := types.NewID()
	apiKeyID := types.NewID()
	apiKeyNode := graphrag.NewGraphNode(apiKeyID, graphrag.NodeType("api_key"))
	apiKeyNode.WithProperty("key", "sk-123")
	apiKeyNode.WithProperty("parent_id", serviceID.String())

	// Create parent service node
	serviceNode := graphrag.NewGraphNode(serviceID, graphrag.NodeType(graphrag.NodeTypeService))
	serviceNode.WithProperty("name", "https")

	// Setup mock expectations
	registry.On("IsAssetType", "api_key").Return(true)
	store.On("GetNode", mock.Anything, serviceID).Return(serviceNode, nil)
	registry.On("GetParentRelationship", "api_key", graphrag.NodeTypeService).
		Return("EXPOSES_API_KEY", true) // Custom relationship type

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, apiKeyNode)

	// Verify
	require.NoError(t, err)
	require.Len(t, relationships, 2) // DISCOVERED + EXPOSES_API_KEY

	// Find the custom relationship
	var customRel *graphrag.Relationship
	for _, rel := range relationships {
		if rel.Type == graphrag.RelationType("EXPOSES_API_KEY") {
			customRel = rel
			break
		}
	}

	require.NotNil(t, customRel, "Expected EXPOSES_API_KEY relationship")
	assert.Equal(t, serviceID, customRel.FromID)
	assert.Equal(t, apiKeyID, customRel.ToID)

	registry.AssertExpectations(t)
	store.AssertExpectations(t)
}

// TestBuildRelationships_NonAssetType_NoDiscoveredRelationship tests that non-asset
// types do NOT create DISCOVERED relationships.
func TestBuildRelationships_NonAssetType_NoDiscoveredRelationship(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a finding node (non-asset type)
	findingID := types.NewID()
	findingNode := graphrag.NewGraphNode(findingID, graphrag.NodeType(graphrag.NodeTypeFinding))
	findingNode.WithProperty("title", "SQL Injection")

	// Setup mock expectations - finding is NOT an asset type
	registry.On("IsAssetType", graphrag.NodeTypeFinding).Return(false)

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, findingNode)

	// Verify - no DISCOVERED relationship created
	require.NoError(t, err)
	assert.Empty(t, relationships)

	registry.AssertExpectations(t)
}

// TestBuildRelationships_NoParentId_OnlyDiscoveredRelationship tests that nodes
// without parent_id create only the DISCOVERED relationship.
func TestBuildRelationships_NoParentId_OnlyDiscoveredRelationship(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a host node without parent_id (root asset)
	hostID := types.NewID()
	hostNode := graphrag.NewGraphNode(hostID, graphrag.NodeType(graphrag.NodeTypeHost))
	hostNode.WithProperty("ip", "192.168.1.100")
	// No parent_id property

	// Setup mock expectations
	registry.On("IsAssetType", graphrag.NodeTypeHost).Return(true)

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, hostNode)

	// Verify - only DISCOVERED relationship
	require.NoError(t, err)
	require.Len(t, relationships, 1)

	rel := relationships[0]
	assert.Equal(t, graphrag.RelationType(graphrag.RelTypeDiscovered), rel.Type)
	assert.Equal(t, agentRunID, rel.FromID)
	assert.Equal(t, hostID, rel.ToID)

	registry.AssertExpectations(t)
}

// TestBuildRelationships_NoLabels_ReturnsError tests that nodes without labels
// return an error.
func TestBuildRelationships_NoLabels_ReturnsError(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a node with no labels
	nodeID := types.NewID()
	node := &graphrag.GraphNode{
		ID:         nodeID,
		Labels:     []graphrag.NodeType{}, // Empty labels
		Properties: make(map[string]any),
	}

	ctx := context.Background()

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, node)

	// Verify error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no labels")
	assert.Empty(t, relationships)
}

// TestBuildRelationships_MissingAgentRunID_WarnsButContinues tests that missing
// agent_run_id logs a warning but doesn't fail.
func TestBuildRelationships_MissingAgentRunID_WarnsButContinues(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a host node (asset type)
	hostID := types.NewID()
	hostNode := graphrag.NewGraphNode(hostID, graphrag.NodeType(graphrag.NodeTypeHost))
	hostNode.WithProperty("ip", "192.168.1.100")

	// Setup mock expectations
	registry.On("IsAssetType", graphrag.NodeTypeHost).Return(true)

	// Create context WITHOUT agent_run_id
	ctx := context.Background()

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, hostNode)

	// Verify - no error, but no DISCOVERED relationship created
	require.NoError(t, err)
	assert.Empty(t, relationships)

	registry.AssertExpectations(t)
}

// TestBuildRelationships_MultiplePropertyNamingConventions tests that the builder
// supports multiple parent ID property naming conventions.
func TestBuildRelationships_MultiplePropertyNamingConventions(t *testing.T) {
	testCases := []struct {
		name         string
		nodeType     string
		parentProp   string
		expectedRel  string
		expectParent bool
	}{
		{
			name:         "standard parent_id",
			nodeType:     graphrag.NodeTypePort,
			parentProp:   "parent_id",
			expectedRel:  graphrag.RelTypeHasPort,
			expectParent: true,
		},
		{
			name:         "legacy parent_ref",
			nodeType:     graphrag.NodeTypePort,
			parentProp:   "parent_ref",
			expectedRel:  graphrag.RelTypeHasPort,
			expectParent: true,
		},
		{
			name:         "type-specific host_id for port",
			nodeType:     graphrag.NodeTypePort,
			parentProp:   "host_id",
			expectedRel:  graphrag.RelTypeHasPort,
			expectParent: true,
		},
		{
			name:         "type-specific port_id for service",
			nodeType:     graphrag.NodeTypeService,
			parentProp:   "port_id",
			expectedRel:  graphrag.RelTypeRunsService,
			expectParent: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry := new(MockTaxonomyRegistry)
			store := new(MockNodeStore)
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			builder := NewRelationshipBuilder(registry, store, logger)

			// Create parent node with UUID
			parentID := types.NewID()
			var parentType string
			switch tc.nodeType {
			case graphrag.NodeTypePort:
				parentType = graphrag.NodeTypeHost
			case graphrag.NodeTypeService:
				parentType = graphrag.NodeTypePort
			default:
				parentType = graphrag.NodeTypeHost
			}
			parentNode := graphrag.NewGraphNode(parentID, graphrag.NodeType(parentType))

			// Create child node
			childID := types.NewID()
			childNode := graphrag.NewGraphNode(childID, graphrag.NodeType(tc.nodeType))
			childNode.WithProperty(tc.parentProp, parentID.String())

			// Setup mock expectations
			registry.On("IsAssetType", tc.nodeType).Return(true)
			store.On("GetNode", mock.Anything, parentID).Return(parentNode, nil)
			registry.On("GetParentRelationship", tc.nodeType, parentType).
				Return(tc.expectedRel, tc.expectParent)

			// Create context with agent_run_id
			agentRunID := types.NewID()
			ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

			// Build relationships
			relationships, err := builder.BuildRelationships(ctx, childNode)

			// Verify
			require.NoError(t, err)
			if tc.expectParent {
				require.Len(t, relationships, 2) // DISCOVERED + parent relationship

				// Find the parent relationship
				var parentRel *graphrag.Relationship
				for _, rel := range relationships {
					if rel.Type == graphrag.RelationType(tc.expectedRel) {
						parentRel = rel
						break
					}
				}

				require.NotNil(t, parentRel, "Expected parent relationship: %s", tc.expectedRel)
				assert.Equal(t, parentID, parentRel.FromID)
				assert.Equal(t, childID, parentRel.ToID)
			}

			registry.AssertExpectations(t)
			store.AssertExpectations(t)
		})
	}
}

// TestBuildRelationships_TaxonomyRelationshipNotDefined_LogsWarning tests that when
// taxonomy doesn't define a parent relationship, a warning is logged.
func TestBuildRelationships_TaxonomyRelationshipNotDefined_LogsWarning(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create a port node with parent_id
	hostID := types.NewID()
	portID := types.NewID()
	portNode := graphrag.NewGraphNode(portID, graphrag.NodeType(graphrag.NodeTypePort))
	portNode.WithProperty("parent_id", hostID.String())

	// Create parent host node
	hostNode := graphrag.NewGraphNode(hostID, graphrag.NodeType(graphrag.NodeTypeHost))

	// Setup mock expectations - relationship NOT defined in taxonomy
	registry.On("IsAssetType", graphrag.NodeTypePort).Return(true)
	store.On("GetNode", mock.Anything, hostID).Return(hostNode, nil)
	registry.On("GetParentRelationship", graphrag.NodeTypePort, graphrag.NodeTypeHost).
		Return("", false) // Relationship not defined

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, portNode)

	// Verify - only DISCOVERED relationship, no parent relationship
	require.NoError(t, err)
	require.Len(t, relationships, 1)
	assert.Equal(t, graphrag.RelationType(graphrag.RelTypeDiscovered), relationships[0].Type)

	registry.AssertExpectations(t)
	store.AssertExpectations(t)
}

// TestBuildRelationships_ServiceWithPortParent tests the full chain:
// host -> port -> service with proper HAS_PORT and RUNS_SERVICE relationships.
func TestBuildRelationships_ServiceWithPortParent(t *testing.T) {
	registry := new(MockTaxonomyRegistry)
	store := new(MockNodeStore)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	builder := NewRelationshipBuilder(registry, store, logger)

	// Create service node with port parent
	portID := types.NewID()
	serviceID := types.NewID()
	serviceNode := graphrag.NewGraphNode(serviceID, graphrag.NodeType(graphrag.NodeTypeService))
	serviceNode.WithProperty("name", "https")
	serviceNode.WithProperty("parent_id", portID.String())

	// Create parent port node
	portNode := graphrag.NewGraphNode(portID, graphrag.NodeType(graphrag.NodeTypePort))
	portNode.WithProperty("number", 443)

	// Setup mock expectations
	registry.On("IsAssetType", graphrag.NodeTypeService).Return(true)
	store.On("GetNode", mock.Anything, portID).Return(portNode, nil)
	registry.On("GetParentRelationship", graphrag.NodeTypeService, graphrag.NodeTypePort).
		Return(graphrag.RelTypeRunsService, true)

	// Create context with agent_run_id
	agentRunID := types.NewID()
	ctx := ContextWithAgentRunID(context.Background(), agentRunID.String())

	// Build relationships
	relationships, err := builder.BuildRelationships(ctx, serviceNode)

	// Verify
	require.NoError(t, err)
	require.Len(t, relationships, 2) // DISCOVERED + RUNS_SERVICE

	// Find the RUNS_SERVICE relationship
	var runsServiceRel *graphrag.Relationship
	for _, rel := range relationships {
		if rel.Type == graphrag.RelationType(graphrag.RelTypeRunsService) {
			runsServiceRel = rel
			break
		}
	}

	require.NotNil(t, runsServiceRel, "Expected RUNS_SERVICE relationship")
	assert.Equal(t, portID, runsServiceRel.FromID)
	assert.Equal(t, serviceID, runsServiceRel.ToID)

	registry.AssertExpectations(t)
	store.AssertExpectations(t)
}
