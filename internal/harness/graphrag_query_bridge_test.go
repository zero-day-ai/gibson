package harness

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
	sdkgraphrag "github.com/zero-day-ai/sdk/graphrag"
)

// MockGraphRAGStore implements graphrag.GraphRAGStore for testing
type MockGraphRAGStore struct {
	// Control flags
	ShouldFailQuery               bool
	ShouldFailFindSimilarAttacks  bool
	ShouldFailFindSimilarFindings bool
	ShouldFailGetAttackChains     bool
	ShouldFailGetRelatedFindings  bool
	ShouldFailStore               bool
	ShouldFailStoreBatch          bool
	IsHealthy                     bool
	HealthMessage                 string

	// Capture method calls
	QueryCalled               bool
	FindSimilarAttacksCalled  bool
	FindSimilarFindingsCalled bool
	GetAttackChainsCalled     bool
	GetRelatedFindingsCalled  bool
	StoreCalled               bool
	StoreBatchCalled          bool
	HealthCalled              bool
	CloseCalled               bool

	// Captured arguments
	LastQuery              *graphrag.GraphRAGQuery
	LastStoreRecord        *graphrag.GraphRecord
	LastStoreBatchRecords  []graphrag.GraphRecord
	LastFindAttacksContent string
	LastFindAttacksTopK    int
	LastFindFindingsID     string
	LastFindFindingsTopK   int
	LastAttackChainsTechID string
	LastAttackChainsDepth  int
	LastRelatedFindingID   string

	// Return values
	QueryResults    []graphrag.GraphRAGResult
	AttackPatterns  []graphrag.AttackPattern
	Findings        []graphrag.FindingNode
	AttackChains    []graphrag.AttackChain
	RelatedFindings []graphrag.FindingNode
	StoredNodeID    types.ID
}

// Query executes a hybrid GraphRAG query
func (m *MockGraphRAGStore) Query(ctx context.Context, query graphrag.GraphRAGQuery) ([]graphrag.GraphRAGResult, error) {
	m.QueryCalled = true
	m.LastQuery = &query

	if m.ShouldFailQuery {
		return nil, errors.New("mock query error")
	}

	return m.QueryResults, nil
}

// FindSimilarAttacks finds attack patterns similar to the given content
func (m *MockGraphRAGStore) FindSimilarAttacks(ctx context.Context, content string, topK int) ([]graphrag.AttackPattern, error) {
	m.FindSimilarAttacksCalled = true
	m.LastFindAttacksContent = content
	m.LastFindAttacksTopK = topK

	if m.ShouldFailFindSimilarAttacks {
		return nil, errors.New("mock find similar attacks error")
	}

	return m.AttackPatterns, nil
}

// FindSimilarFindings finds findings similar to the specified finding
func (m *MockGraphRAGStore) FindSimilarFindings(ctx context.Context, findingID string, topK int) ([]graphrag.FindingNode, error) {
	m.FindSimilarFindingsCalled = true
	m.LastFindFindingsID = findingID
	m.LastFindFindingsTopK = topK

	if m.ShouldFailFindSimilarFindings {
		return nil, errors.New("mock find similar findings error")
	}

	return m.Findings, nil
}

// GetAttackChains discovers attack chains from a starting technique
func (m *MockGraphRAGStore) GetAttackChains(ctx context.Context, techniqueID string, maxDepth int) ([]graphrag.AttackChain, error) {
	m.GetAttackChainsCalled = true
	m.LastAttackChainsTechID = techniqueID
	m.LastAttackChainsDepth = maxDepth

	if m.ShouldFailGetAttackChains {
		return nil, errors.New("mock get attack chains error")
	}

	return m.AttackChains, nil
}

// GetRelatedFindings retrieves findings related to the specified finding
func (m *MockGraphRAGStore) GetRelatedFindings(ctx context.Context, findingID string) ([]graphrag.FindingNode, error) {
	m.GetRelatedFindingsCalled = true
	m.LastRelatedFindingID = findingID

	if m.ShouldFailGetRelatedFindings {
		return nil, errors.New("mock get related findings error")
	}

	return m.RelatedFindings, nil
}

// Store stores a single graph record
func (m *MockGraphRAGStore) Store(ctx context.Context, record graphrag.GraphRecord) error {
	m.StoreCalled = true
	m.LastStoreRecord = &record

	if m.ShouldFailStore {
		return errors.New("mock store error")
	}

	// Set node ID if not set
	if m.StoredNodeID == "" {
		m.StoredNodeID = record.Node.ID
	}

	return nil
}

// StoreBatch stores multiple graph records
func (m *MockGraphRAGStore) StoreBatch(ctx context.Context, records []graphrag.GraphRecord) error {
	m.StoreBatchCalled = true
	m.LastStoreBatchRecords = records

	if m.ShouldFailStoreBatch {
		return errors.New("mock store batch error")
	}

	return nil
}

// StoreAttackPattern stores a MITRE ATT&CK pattern (not used by bridge)
func (m *MockGraphRAGStore) StoreAttackPattern(ctx context.Context, pattern graphrag.AttackPattern) error {
	return nil
}

// StoreFinding stores a security finding (not used by bridge)
func (m *MockGraphRAGStore) StoreFinding(ctx context.Context, finding graphrag.FindingNode) error {
	return nil
}

// Health returns the health status
func (m *MockGraphRAGStore) Health(ctx context.Context) types.HealthStatus {
	m.HealthCalled = true

	if m.IsHealthy {
		return types.Healthy(m.HealthMessage)
	}
	return types.Unhealthy(m.HealthMessage)
}

// Close releases all resources
func (m *MockGraphRAGStore) Close() error {
	m.CloseCalled = true
	return nil
}

// Compile-time check that MockGraphRAGStore implements graphrag.GraphRAGStore
var _ graphrag.GraphRAGStore = (*MockGraphRAGStore)(nil)

// Helper function to create a valid SDK query
func createValidSDKQuery() sdkgraphrag.Query {
	return sdkgraphrag.Query{
		Text:         "test query",
		TopK:         5,
		MaxHops:      2,
		MinScore:     0.5,
		VectorWeight: 0.6,
		GraphWeight:  0.4,
		NodeTypes:    []string{"Finding"},
	}
}

// TestDefaultGraphRAGQueryBridge_Query tests the Query method
func TestDefaultGraphRAGQueryBridge_Query(t *testing.T) {
	tests := []struct {
		name        string
		query       sdkgraphrag.Query
		mockResults []graphrag.GraphRAGResult
		shouldFail  bool
		checkError  func(t *testing.T, err error)
		checkResult func(t *testing.T, results []sdkgraphrag.Result, mock *MockGraphRAGStore)
	}{
		{
			name:  "successful query with results",
			query: createValidSDKQuery(),
			mockResults: []graphrag.GraphRAGResult{
				{
					Node: graphrag.GraphNode{
						ID:         types.NewID(),
						Labels:     []graphrag.NodeType{graphrag.NodeTypeFinding},
						Properties: map[string]any{"title": "Test Finding"},
						CreatedAt:  time.Now(),
						UpdatedAt:  time.Now(),
					},
					Score:       0.95,
					VectorScore: 0.98,
					GraphScore:  0.92,
					Path:        []types.ID{types.NewID()},
					Distance:    1,
				},
			},
			shouldFail: false,
			checkResult: func(t *testing.T, results []sdkgraphrag.Result, mock *MockGraphRAGStore) {
				assert.Len(t, results, 1)
				assert.Equal(t, 0.95, results[0].Score)
				assert.Equal(t, 0.98, results[0].VectorScore)
				assert.Equal(t, 0.92, results[0].GraphScore)
				assert.Equal(t, 1, results[0].Distance)

				// Check that internal query was properly converted
				require.NotNil(t, mock.LastQuery)
				assert.Equal(t, "test query", mock.LastQuery.Text)
				assert.Equal(t, 5, mock.LastQuery.TopK)
				assert.Equal(t, 2, mock.LastQuery.MaxHops)
				assert.Equal(t, 0.5, mock.LastQuery.MinScore)
			},
		},
		{
			name:        "empty results",
			query:       createValidSDKQuery(),
			mockResults: []graphrag.GraphRAGResult{},
			shouldFail:  false,
			checkResult: func(t *testing.T, results []sdkgraphrag.Result, mock *MockGraphRAGStore) {
				assert.Len(t, results, 0)
				assert.True(t, mock.QueryCalled)
			},
		},
		{
			name:       "invalid query - missing text and embedding",
			query:      sdkgraphrag.Query{TopK: 5, MaxHops: 2},
			shouldFail: true,
			checkError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid query")
			},
		},
		{
			name: "query with mission ID filter",
			query: func() sdkgraphrag.Query {
				q := createValidSDKQuery()
				q.MissionID = types.NewID().String()
				return q
			}(),
			mockResults: []graphrag.GraphRAGResult{},
			shouldFail:  false,
			checkResult: func(t *testing.T, results []sdkgraphrag.Result, mock *MockGraphRAGStore) {
				require.NotNil(t, mock.LastQuery)
				assert.NotNil(t, mock.LastQuery.MissionID)
			},
		},
		{
			name:       "store error propagation",
			query:      createValidSDKQuery(),
			shouldFail: true,
			checkError: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "query execution failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				QueryResults:    tt.mockResults,
				ShouldFailQuery: tt.shouldFail && tt.checkError != nil,
				IsHealthy:       true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			results, err := bridge.Query(ctx, tt.query)

			if tt.checkError != nil {
				tt.checkError(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, results, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_FindSimilarAttacks tests the FindSimilarAttacks method
func TestDefaultGraphRAGQueryBridge_FindSimilarAttacks(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		topK         int
		mockPatterns []graphrag.AttackPattern
		shouldFail   bool
		checkResult  func(t *testing.T, patterns []sdkgraphrag.AttackPattern, mock *MockGraphRAGStore)
	}{
		{
			name:    "successful attack pattern search",
			content: "lateral movement using SSH",
			topK:    3,
			mockPatterns: []graphrag.AttackPattern{
				{
					ID:          types.NewID(),
					TechniqueID: "T1021.004",
					Name:        "Remote Services: SSH",
					Description: "Adversaries may use SSH to move laterally",
					Tactics:     []string{"Lateral Movement"},
					Platforms:   []string{"Linux", "macOS"},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
			shouldFail: false,
			checkResult: func(t *testing.T, patterns []sdkgraphrag.AttackPattern, mock *MockGraphRAGStore) {
				assert.Len(t, patterns, 1)
				assert.Equal(t, "T1021.004", patterns[0].TechniqueID)
				assert.Equal(t, "Remote Services: SSH", patterns[0].Name)
				assert.Contains(t, patterns[0].Tactics, "Lateral Movement")

				// Check captured arguments
				assert.Equal(t, "lateral movement using SSH", mock.LastFindAttacksContent)
				assert.Equal(t, 3, mock.LastFindAttacksTopK)
			},
		},
		{
			name:         "no patterns found",
			content:      "unknown technique",
			topK:         5,
			mockPatterns: []graphrag.AttackPattern{},
			shouldFail:   false,
			checkResult: func(t *testing.T, patterns []sdkgraphrag.AttackPattern, mock *MockGraphRAGStore) {
				assert.Len(t, patterns, 0)
				assert.True(t, mock.FindSimilarAttacksCalled)
			},
		},
		{
			name:       "store error",
			content:    "test content",
			topK:       5,
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				AttackPatterns:               tt.mockPatterns,
				ShouldFailFindSimilarAttacks: tt.shouldFail,
				IsHealthy:                    true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			patterns, err := bridge.FindSimilarAttacks(ctx, tt.content, tt.topK)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, patterns, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_FindSimilarFindings tests the FindSimilarFindings method
func TestDefaultGraphRAGQueryBridge_FindSimilarFindings(t *testing.T) {
	tests := []struct {
		name         string
		findingID    string
		topK         int
		mockFindings []graphrag.FindingNode
		shouldFail   bool
		checkResult  func(t *testing.T, findings []sdkgraphrag.FindingNode, mock *MockGraphRAGStore)
	}{
		{
			name:      "successful finding search",
			findingID: types.NewID().String(),
			topK:      5,
			mockFindings: []graphrag.FindingNode{
				{
					ID:          types.NewID(),
					Title:       "SQL Injection",
					Description: "SQL injection vulnerability detected",
					Severity:    "high",
					Category:    "injection",
					Confidence:  0.95,
					MissionID:   types.NewID(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
			shouldFail: false,
			checkResult: func(t *testing.T, findings []sdkgraphrag.FindingNode, mock *MockGraphRAGStore) {
				assert.Len(t, findings, 1)
				assert.Equal(t, "SQL Injection", findings[0].Title)
				assert.Equal(t, "high", findings[0].Severity)
				assert.Equal(t, 0.95, findings[0].Confidence)

				// Check captured arguments
				assert.Equal(t, 5, mock.LastFindFindingsTopK)
			},
		},
		{
			name:         "no similar findings",
			findingID:    types.NewID().String(),
			topK:         3,
			mockFindings: []graphrag.FindingNode{},
			shouldFail:   false,
			checkResult: func(t *testing.T, findings []sdkgraphrag.FindingNode, mock *MockGraphRAGStore) {
				assert.Len(t, findings, 0)
				assert.True(t, mock.FindSimilarFindingsCalled)
			},
		},
		{
			name:       "store error",
			findingID:  types.NewID().String(),
			topK:       5,
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				Findings:                      tt.mockFindings,
				ShouldFailFindSimilarFindings: tt.shouldFail,
				IsHealthy:                     true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			findings, err := bridge.FindSimilarFindings(ctx, tt.findingID, tt.topK)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, findings, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_GetAttackChains tests the GetAttackChains method
func TestDefaultGraphRAGQueryBridge_GetAttackChains(t *testing.T) {
	tests := []struct {
		name        string
		techniqueID string
		maxDepth    int
		mockChains  []graphrag.AttackChain
		shouldFail  bool
		checkResult func(t *testing.T, chains []sdkgraphrag.AttackChain, mock *MockGraphRAGStore)
	}{
		{
			name:        "successful attack chain discovery",
			techniqueID: "T1566",
			maxDepth:    3,
			mockChains: []graphrag.AttackChain{
				{
					ID:       types.NewID(),
					Name:     "Phishing to Data Exfiltration",
					Severity: "critical",
					Steps: []graphrag.AttackStep{
						{
							Order:       1,
							TechniqueID: "T1566",
							NodeID:      types.NewID(),
							Description: "Initial phishing email",
							Confidence:  0.9,
						},
						{
							Order:       2,
							TechniqueID: "T1059",
							NodeID:      types.NewID(),
							Description: "Command execution",
							Confidence:  0.85,
						},
					},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			},
			shouldFail: false,
			checkResult: func(t *testing.T, chains []sdkgraphrag.AttackChain, mock *MockGraphRAGStore) {
				assert.Len(t, chains, 1)
				assert.Equal(t, "Phishing to Data Exfiltration", chains[0].Name)
				assert.Equal(t, "critical", chains[0].Severity)
				assert.Len(t, chains[0].Steps, 2)
				assert.Equal(t, 1, chains[0].Steps[0].Order)
				assert.Equal(t, "T1566", chains[0].Steps[0].TechniqueID)

				// Check captured arguments
				assert.Equal(t, "T1566", mock.LastAttackChainsTechID)
				assert.Equal(t, 3, mock.LastAttackChainsDepth)
			},
		},
		{
			name:        "no chains found",
			techniqueID: "T9999",
			maxDepth:    2,
			mockChains:  []graphrag.AttackChain{},
			shouldFail:  false,
			checkResult: func(t *testing.T, chains []sdkgraphrag.AttackChain, mock *MockGraphRAGStore) {
				assert.Len(t, chains, 0)
				assert.True(t, mock.GetAttackChainsCalled)
			},
		},
		{
			name:        "store error",
			techniqueID: "T1566",
			maxDepth:    3,
			shouldFail:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				AttackChains:              tt.mockChains,
				ShouldFailGetAttackChains: tt.shouldFail,
				IsHealthy:                 true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			chains, err := bridge.GetAttackChains(ctx, tt.techniqueID, tt.maxDepth)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, chains, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_GetRelatedFindings tests the GetRelatedFindings method
func TestDefaultGraphRAGQueryBridge_GetRelatedFindings(t *testing.T) {
	tests := []struct {
		name         string
		findingID    string
		mockFindings []graphrag.FindingNode
		shouldFail   bool
		checkResult  func(t *testing.T, findings []sdkgraphrag.FindingNode, mock *MockGraphRAGStore)
	}{
		{
			name:      "successful related findings retrieval",
			findingID: types.NewID().String(),
			mockFindings: []graphrag.FindingNode{
				{
					ID:          types.NewID(),
					Title:       "Related Finding 1",
					Description: "First related finding",
					Severity:    "medium",
					Category:    "web",
					Confidence:  0.88,
					MissionID:   types.NewID(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				{
					ID:          types.NewID(),
					Title:       "Related Finding 2",
					Description: "Second related finding",
					Severity:    "low",
					Category:    "network",
					Confidence:  0.75,
					MissionID:   types.NewID(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
			shouldFail: false,
			checkResult: func(t *testing.T, findings []sdkgraphrag.FindingNode, mock *MockGraphRAGStore) {
				assert.Len(t, findings, 2)
				assert.Equal(t, "Related Finding 1", findings[0].Title)
				assert.Equal(t, "Related Finding 2", findings[1].Title)
				assert.True(t, mock.GetRelatedFindingsCalled)
			},
		},
		{
			name:         "no related findings",
			findingID:    types.NewID().String(),
			mockFindings: []graphrag.FindingNode{},
			shouldFail:   false,
			checkResult: func(t *testing.T, findings []sdkgraphrag.FindingNode, mock *MockGraphRAGStore) {
				assert.Len(t, findings, 0)
			},
		},
		{
			name:       "store error",
			findingID:  types.NewID().String(),
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				RelatedFindings:              tt.mockFindings,
				ShouldFailGetRelatedFindings: tt.shouldFail,
				IsHealthy:                    true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			findings, err := bridge.GetRelatedFindings(ctx, tt.findingID)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, findings, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_StoreNode tests the StoreNode method
func TestDefaultGraphRAGQueryBridge_StoreNode(t *testing.T) {
	tests := []struct {
		name        string
		node        sdkgraphrag.GraphNode
		missionID   string
		agentName   string
		shouldFail  bool
		checkResult func(t *testing.T, nodeID string, mock *MockGraphRAGStore)
	}{
		{
			name: "successful node storage",
			node: sdkgraphrag.GraphNode{
				Type:       "Finding",
				Content:    "Test finding content",
				Properties: map[string]any{"severity": "high"},
			},
			missionID:  types.NewID().String(),
			agentName:  "test-agent",
			shouldFail: false,
			checkResult: func(t *testing.T, nodeID string, mock *MockGraphRAGStore) {
				assert.NotEmpty(t, nodeID)
				assert.True(t, mock.StoreCalled)
				require.NotNil(t, mock.LastStoreRecord)

				// Check that mission ID and agent name were set
				node := mock.LastStoreRecord.Node
				assert.NotNil(t, node.MissionID)
				assert.Equal(t, "test-agent", node.Properties["agent_name"])
				assert.Equal(t, "high", node.Properties["severity"])
			},
		},
		{
			name: "node with existing ID",
			node: sdkgraphrag.GraphNode{
				ID:         types.NewID().String(),
				Type:       "Entity",
				Properties: map[string]any{"name": "test"},
			},
			missionID:  types.NewID().String(),
			agentName:  "agent-2",
			shouldFail: false,
			checkResult: func(t *testing.T, nodeID string, mock *MockGraphRAGStore) {
				assert.NotEmpty(t, nodeID)
				require.NotNil(t, mock.LastStoreRecord)
				assert.Equal(t, "test", mock.LastStoreRecord.Node.Properties["name"])
			},
		},
		{
			name: "invalid node - missing type",
			node: sdkgraphrag.GraphNode{
				Properties: map[string]any{"test": "value"},
			},
			missionID:  types.NewID().String(),
			agentName:  "agent",
			shouldFail: true,
		},
		{
			name: "store error",
			node: sdkgraphrag.GraphNode{
				Type: "Finding",
			},
			missionID:  types.NewID().String(),
			agentName:  "agent",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				ShouldFailStore: tt.shouldFail && tt.name == "store error",
				IsHealthy:       true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			nodeID, err := bridge.StoreNode(ctx, tt.node, tt.missionID, tt.agentName)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, nodeID, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_CreateRelationship tests the CreateRelationship method
func TestDefaultGraphRAGQueryBridge_CreateRelationship(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()

	tests := []struct {
		name        string
		rel         sdkgraphrag.Relationship
		shouldFail  bool
		checkResult func(t *testing.T, mock *MockGraphRAGStore)
	}{
		{
			name: "successful relationship creation",
			rel: sdkgraphrag.Relationship{
				FromID:     fromID.String(),
				ToID:       toID.String(),
				Type:       "SIMILAR_TO",
				Properties: map[string]any{"weight": 0.85},
			},
			shouldFail: false,
			checkResult: func(t *testing.T, mock *MockGraphRAGStore) {
				assert.True(t, mock.StoreCalled)
				require.NotNil(t, mock.LastStoreRecord)

				// Check that relationship was added
				assert.Len(t, mock.LastStoreRecord.Relationships, 1)
				rel := mock.LastStoreRecord.Relationships[0]
				assert.Equal(t, fromID, rel.FromID)
				assert.Equal(t, toID, rel.ToID)
				assert.Equal(t, graphrag.RelationType("SIMILAR_TO"), rel.Type)
			},
		},
		{
			name: "invalid relationship - missing type",
			rel: sdkgraphrag.Relationship{
				FromID: fromID.String(),
				ToID:   toID.String(),
			},
			shouldFail: true,
		},
		{
			name: "invalid from_id",
			rel: sdkgraphrag.Relationship{
				FromID: "invalid-id",
				ToID:   toID.String(),
				Type:   "RELATED_TO",
			},
			shouldFail: true,
		},
		{
			name: "store error",
			rel: sdkgraphrag.Relationship{
				FromID: fromID.String(),
				ToID:   toID.String(),
				Type:   "EXPLOITS",
			},
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				ShouldFailStore: tt.shouldFail && tt.name == "store error",
				IsHealthy:       true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			err := bridge.CreateRelationship(ctx, tt.rel)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_StoreBatch tests the StoreBatch method
func TestDefaultGraphRAGQueryBridge_StoreBatch(t *testing.T) {
	tests := []struct {
		name        string
		batch       sdkgraphrag.Batch
		missionID   string
		agentName   string
		shouldFail  bool
		checkResult func(t *testing.T, nodeIDs []string, mock *MockGraphRAGStore)
	}{
		{
			name: "successful batch storage",
			batch: sdkgraphrag.Batch{
				Nodes: []sdkgraphrag.GraphNode{
					{
						Type:       "Finding",
						Properties: map[string]any{"title": "Finding 1"},
					},
					{
						Type:       "Finding",
						Properties: map[string]any{"title": "Finding 2"},
					},
				},
				Relationships: []sdkgraphrag.Relationship{},
			},
			missionID:  types.NewID().String(),
			agentName:  "batch-agent",
			shouldFail: false,
			checkResult: func(t *testing.T, nodeIDs []string, mock *MockGraphRAGStore) {
				assert.Len(t, nodeIDs, 2)
				assert.True(t, mock.StoreBatchCalled)
				require.NotNil(t, mock.LastStoreBatchRecords)
				assert.Len(t, mock.LastStoreBatchRecords, 2)

				// Check that mission ID and agent name were set
				for _, record := range mock.LastStoreBatchRecords {
					assert.NotNil(t, record.Node.MissionID)
					assert.Equal(t, "batch-agent", record.Node.Properties["agent_name"])
				}
			},
		},
		{
			name: "batch with relationships",
			batch: func() sdkgraphrag.Batch {
				node1ID := types.NewID()
				node2ID := types.NewID()
				return sdkgraphrag.Batch{
					Nodes: []sdkgraphrag.GraphNode{
						{ID: node1ID.String(), Type: "Finding"},
						{ID: node2ID.String(), Type: "Finding"},
					},
					Relationships: []sdkgraphrag.Relationship{
						{
							FromID: node1ID.String(),
							ToID:   node2ID.String(),
							Type:   "RELATED_TO",
						},
					},
				}
			}(),
			missionID:  types.NewID().String(),
			agentName:  "agent",
			shouldFail: false,
			checkResult: func(t *testing.T, nodeIDs []string, mock *MockGraphRAGStore) {
				assert.Len(t, nodeIDs, 2)
				require.NotNil(t, mock.LastStoreBatchRecords)

				// Check that first record has the relationship
				assert.Len(t, mock.LastStoreBatchRecords[0].Relationships, 1)
			},
		},
		{
			name: "invalid node in batch",
			batch: sdkgraphrag.Batch{
				Nodes: []sdkgraphrag.GraphNode{
					{Type: "Finding"},
					{}, // Invalid - missing type
				},
			},
			missionID:  types.NewID().String(),
			agentName:  "agent",
			shouldFail: true,
		},
		{
			name: "store batch error",
			batch: sdkgraphrag.Batch{
				Nodes: []sdkgraphrag.GraphNode{
					{Type: "Finding"},
				},
			},
			missionID:  types.NewID().String(),
			agentName:  "agent",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				ShouldFailStoreBatch: tt.shouldFail && tt.name == "store batch error",
				IsHealthy:            true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			nodeIDs, err := bridge.StoreBatch(ctx, tt.batch, tt.missionID, tt.agentName)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, nodeIDs, mock)
				}
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_Traverse tests the Traverse method
func TestDefaultGraphRAGQueryBridge_Traverse(t *testing.T) {
	startNodeID := types.NewID().String()

	tests := []struct {
		name string
		opts sdkgraphrag.TraversalOptions
	}{
		{
			name: "traverse returns not implemented",
			opts: sdkgraphrag.TraversalOptions{
				MaxDepth:          3,
				Direction:         "outbound",
				RelationshipTypes: []string{"SIMILAR_TO"},
				NodeTypes:         []string{"Finding"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				IsHealthy: true,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			results, err := bridge.Traverse(ctx, startNodeID, tt.opts)

			// Traverse should return "not yet implemented" error
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "not yet implemented")
			assert.Nil(t, results)
		})
	}
}

// TestDefaultGraphRAGQueryBridge_Health tests the Health method
func TestDefaultGraphRAGQueryBridge_Health(t *testing.T) {
	tests := []struct {
		name          string
		isHealthy     bool
		healthMessage string
		checkResult   func(t *testing.T, status types.HealthStatus)
	}{
		{
			name:          "healthy store",
			isHealthy:     true,
			healthMessage: "all systems operational",
			checkResult: func(t *testing.T, status types.HealthStatus) {
				assert.True(t, status.IsHealthy())
				assert.Contains(t, status.Message, "all systems operational")
			},
		},
		{
			name:          "unhealthy store",
			isHealthy:     false,
			healthMessage: "connection error",
			checkResult: func(t *testing.T, status types.HealthStatus) {
				assert.True(t, status.IsUnhealthy())
				assert.Contains(t, status.Message, "connection error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockGraphRAGStore{
				IsHealthy:     tt.isHealthy,
				HealthMessage: tt.healthMessage,
			}

			bridge := NewGraphRAGQueryBridge(mock)
			ctx := context.Background()

			status := bridge.Health(ctx)

			assert.True(t, mock.HealthCalled)
			if tt.checkResult != nil {
				tt.checkResult(t, status)
			}
		})
	}
}

// TestDefaultGraphRAGQueryBridge_NilStore tests behavior with nil store
func TestDefaultGraphRAGQueryBridge_NilStore(t *testing.T) {
	bridge := NewGraphRAGQueryBridge(nil)
	ctx := context.Background()

	t.Run("Query with nil store", func(t *testing.T) {
		_, err := bridge.Query(ctx, createValidSDKQuery())
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("FindSimilarAttacks with nil store", func(t *testing.T) {
		_, err := bridge.FindSimilarAttacks(ctx, "test", 5)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("FindSimilarFindings with nil store", func(t *testing.T) {
		_, err := bridge.FindSimilarFindings(ctx, types.NewID().String(), 5)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("GetAttackChains with nil store", func(t *testing.T) {
		_, err := bridge.GetAttackChains(ctx, "T1566", 3)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("GetRelatedFindings with nil store", func(t *testing.T) {
		_, err := bridge.GetRelatedFindings(ctx, types.NewID().String())
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("StoreNode with nil store", func(t *testing.T) {
		node := sdkgraphrag.GraphNode{Type: "Finding"}
		_, err := bridge.StoreNode(ctx, node, types.NewID().String(), "agent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("CreateRelationship with nil store", func(t *testing.T) {
		rel := sdkgraphrag.Relationship{
			FromID: types.NewID().String(),
			ToID:   types.NewID().String(),
			Type:   "RELATED_TO",
		}
		err := bridge.CreateRelationship(ctx, rel)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("StoreBatch with nil store", func(t *testing.T) {
		batch := sdkgraphrag.Batch{
			Nodes: []sdkgraphrag.GraphNode{{Type: "Finding"}},
		}
		_, err := bridge.StoreBatch(ctx, batch, types.NewID().String(), "agent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("Traverse with nil store", func(t *testing.T) {
		opts := sdkgraphrag.TraversalOptions{MaxDepth: 2}
		_, err := bridge.Traverse(ctx, types.NewID().String(), opts)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("Health with nil store", func(t *testing.T) {
		status := bridge.Health(ctx)
		assert.True(t, status.IsUnhealthy())
		assert.Contains(t, status.Message, "nil")
	})
}

// TestNoopGraphRAGQueryBridge tests the NoopGraphRAGQueryBridge
func TestNoopGraphRAGQueryBridge(t *testing.T) {
	bridge := &NoopGraphRAGQueryBridge{}
	ctx := context.Background()

	t.Run("Query returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		_, err := bridge.Query(ctx, createValidSDKQuery())
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("FindSimilarAttacks returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		_, err := bridge.FindSimilarAttacks(ctx, "test", 5)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("FindSimilarFindings returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		_, err := bridge.FindSimilarFindings(ctx, types.NewID().String(), 5)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("GetAttackChains returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		_, err := bridge.GetAttackChains(ctx, "T1566", 3)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("GetRelatedFindings returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		_, err := bridge.GetRelatedFindings(ctx, types.NewID().String())
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("StoreNode returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		node := sdkgraphrag.GraphNode{Type: "Finding"}
		_, err := bridge.StoreNode(ctx, node, types.NewID().String(), "agent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("CreateRelationship returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		rel := sdkgraphrag.Relationship{
			FromID: types.NewID().String(),
			ToID:   types.NewID().String(),
			Type:   "RELATED_TO",
		}
		err := bridge.CreateRelationship(ctx, rel)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("StoreBatch returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		batch := sdkgraphrag.Batch{
			Nodes: []sdkgraphrag.GraphNode{{Type: "Finding"}},
		}
		_, err := bridge.StoreBatch(ctx, batch, types.NewID().String(), "agent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("Traverse returns ErrGraphRAGNotEnabled", func(t *testing.T) {
		opts := sdkgraphrag.TraversalOptions{MaxDepth: 2}
		_, err := bridge.Traverse(ctx, types.NewID().String(), opts)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sdkgraphrag.ErrGraphRAGNotEnabled)
	})

	t.Run("Health returns healthy status", func(t *testing.T) {
		status := bridge.Health(ctx)
		assert.True(t, status.IsHealthy())
		assert.Contains(t, status.Message, "disabled")
		assert.Contains(t, status.Message, "noop")
	})
}
