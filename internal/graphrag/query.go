package graphrag

import (
	"fmt"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// GraphRAGQuery represents a hybrid graph + vector search query.
// Combines semantic search (embeddings) with graph traversal for contextual retrieval.
type GraphRAGQuery struct {
	// Query inputs
	Text      string    `json:"text,omitempty"`      // Text to search for (will be embedded)
	Embedding []float64 `json:"embedding,omitempty"` // Pre-computed embedding vector

	// Search parameters
	TopK     int     `json:"top_k"`               // Number of results to return
	MaxHops  int     `json:"max_hops"`            // Maximum graph traversal depth
	MinScore float64 `json:"min_score,omitempty"` // Minimum similarity threshold (0-1)

	// Filters
	Filters   TraversalFilters `json:"filters,omitempty"`
	NodeTypes []NodeType       `json:"node_types,omitempty"` // Filter by node types
	MissionID *types.ID        `json:"mission_id,omitempty"` // Filter by mission

	// Scoring weights
	VectorWeight float64 `json:"vector_weight,omitempty"` // Weight for vector similarity (0-1)
	GraphWeight  float64 `json:"graph_weight,omitempty"`  // Weight for graph proximity (0-1)
}

// NewGraphRAGQuery creates a new query from text.
func NewGraphRAGQuery(text string) *GraphRAGQuery {
	return &GraphRAGQuery{
		Text:         text,
		TopK:         10,
		MaxHops:      3,
		MinScore:     0.7,
		VectorWeight: 0.6,
		GraphWeight:  0.4,
		Filters:      TraversalFilters{},
	}
}

// NewGraphRAGQueryFromEmbedding creates a new query from a pre-computed embedding.
func NewGraphRAGQueryFromEmbedding(embedding []float64) *GraphRAGQuery {
	return &GraphRAGQuery{
		Embedding:    embedding,
		TopK:         10,
		MaxHops:      3,
		MinScore:     0.7,
		VectorWeight: 0.6,
		GraphWeight:  0.4,
		Filters:      TraversalFilters{},
	}
}

// WithTopK sets the number of results to return.
func (q *GraphRAGQuery) WithTopK(topK int) *GraphRAGQuery {
	q.TopK = topK
	return q
}

// WithMaxHops sets the maximum graph traversal depth.
func (q *GraphRAGQuery) WithMaxHops(maxHops int) *GraphRAGQuery {
	q.MaxHops = maxHops
	return q
}

// WithMinScore sets the minimum similarity threshold.
func (q *GraphRAGQuery) WithMinScore(minScore float64) *GraphRAGQuery {
	q.MinScore = minScore
	return q
}

// WithNodeTypes filters results to specific node types.
func (q *GraphRAGQuery) WithNodeTypes(types ...NodeType) *GraphRAGQuery {
	q.NodeTypes = types
	return q
}

// WithMission filters results to a specific mission.
func (q *GraphRAGQuery) WithMission(missionID types.ID) *GraphRAGQuery {
	q.MissionID = &missionID
	return q
}

// WithFilters sets the traversal filters.
func (q *GraphRAGQuery) WithFilters(filters TraversalFilters) *GraphRAGQuery {
	q.Filters = filters
	return q
}

// WithWeights sets the vector and graph scoring weights.
func (q *GraphRAGQuery) WithWeights(vectorWeight, graphWeight float64) *GraphRAGQuery {
	q.VectorWeight = vectorWeight
	q.GraphWeight = graphWeight
	return q
}

// Validate validates the GraphRAGQuery fields.
func (q *GraphRAGQuery) Validate() error {
	// Must have either Text or Embedding
	if q.Text == "" && len(q.Embedding) == 0 {
		return NewInvalidQueryError("query must have either text or embedding")
	}
	if q.Text != "" && len(q.Embedding) > 0 {
		return NewInvalidQueryError("query cannot have both text and embedding")
	}

	// Validate parameters
	if q.TopK <= 0 {
		return NewInvalidQueryError(fmt.Sprintf("top_k must be greater than 0, got %d", q.TopK))
	}
	if q.MaxHops < 0 {
		return NewInvalidQueryError(fmt.Sprintf("max_hops must be >= 0, got %d", q.MaxHops))
	}
	if q.MinScore < 0.0 || q.MinScore > 1.0 {
		return NewInvalidQueryError(fmt.Sprintf("min_score must be between 0.0 and 1.0, got %f", q.MinScore))
	}
	if q.VectorWeight < 0.0 || q.VectorWeight > 1.0 {
		return NewInvalidQueryError(fmt.Sprintf("vector_weight must be between 0.0 and 1.0, got %f", q.VectorWeight))
	}
	if q.GraphWeight < 0.0 || q.GraphWeight > 1.0 {
		return NewInvalidQueryError(fmt.Sprintf("graph_weight must be between 0.0 and 1.0, got %f", q.GraphWeight))
	}

	// Validate node types
	for _, nt := range q.NodeTypes {
		if !nt.IsValid() {
			return NewInvalidQueryError(fmt.Sprintf("invalid node type: %s", nt))
		}
	}

	return nil
}

// GraphRAGResult represents a single result from a GraphRAG query.
// Includes the node, scoring information, and traversal path.
type GraphRAGResult struct {
	Node        GraphNode  `json:"node"`
	Score       float64    `json:"score"`        // Combined hybrid score
	VectorScore float64    `json:"vector_score"` // Cosine similarity (0-1)
	GraphScore  float64    `json:"graph_score"`  // Graph proximity score (0-1)
	Path        []types.ID `json:"path"`         // Path from query node to result node
	Distance    int        `json:"distance"`     // Graph distance (hops)
	Timestamp   time.Time  `json:"timestamp"`
}

// NewGraphRAGResult creates a new result with the given node and scores.
func NewGraphRAGResult(node GraphNode, vectorScore, graphScore float64) *GraphRAGResult {
	return &GraphRAGResult{
		Node:        node,
		VectorScore: vectorScore,
		GraphScore:  graphScore,
		Path:        []types.ID{},
		Distance:    0,
		Timestamp:   time.Now(),
	}
}

// WithPath sets the traversal path for the result.
func (r *GraphRAGResult) WithPath(path []types.ID) *GraphRAGResult {
	r.Path = path
	r.Distance = len(path) - 1 // Distance is number of edges
	return r
}

// ComputeScore computes the combined hybrid score using the given weights.
func (r *GraphRAGResult) ComputeScore(vectorWeight, graphWeight float64) {
	r.Score = (r.VectorScore * vectorWeight) + (r.GraphScore * graphWeight)
}

// Validate validates the GraphRAGResult fields.
func (r *GraphRAGResult) Validate() error {
	if err := r.Node.Validate(); err != nil {
		return fmt.Errorf("invalid node in result: %w", err)
	}
	if r.Score < 0.0 || r.Score > 1.0 {
		return fmt.Errorf("score must be between 0.0 and 1.0, got %f", r.Score)
	}
	if r.VectorScore < 0.0 || r.VectorScore > 1.0 {
		return fmt.Errorf("vector_score must be between 0.0 and 1.0, got %f", r.VectorScore)
	}
	if r.GraphScore < 0.0 || r.GraphScore > 1.0 {
		return fmt.Errorf("graph_score must be between 0.0 and 1.0, got %f", r.GraphScore)
	}
	if r.Distance < 0 {
		return fmt.Errorf("distance must be >= 0, got %d", r.Distance)
	}
	return nil
}

// NodeQuery represents a query for specific nodes by properties.
// Used for exact lookups rather than similarity search.
type NodeQuery struct {
	NodeTypes  []NodeType     `json:"node_types,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	MissionID  *types.ID      `json:"mission_id,omitempty"`
	Limit      int            `json:"limit,omitempty"`
}

// NewNodeQuery creates a new NodeQuery.
func NewNodeQuery() *NodeQuery {
	return &NodeQuery{
		Properties: make(map[string]any),
		Limit:      100,
	}
}

// WithNodeTypes filters by node types.
func (nq *NodeQuery) WithNodeTypes(types ...NodeType) *NodeQuery {
	nq.NodeTypes = types
	return nq
}

// WithProperty adds a property filter.
func (nq *NodeQuery) WithProperty(key string, value any) *NodeQuery {
	nq.Properties[key] = value
	return nq
}

// WithMission filters by mission ID.
func (nq *NodeQuery) WithMission(missionID types.ID) *NodeQuery {
	nq.MissionID = &missionID
	return nq
}

// WithLimit sets the maximum number of results.
func (nq *NodeQuery) WithLimit(limit int) *NodeQuery {
	nq.Limit = limit
	return nq
}

// TraversalFilters contains filters for graph traversal.
// Controls which relationships and nodes to include during traversal.
type TraversalFilters struct {
	AllowedRelations []RelationType `json:"allowed_relations,omitempty"`  // Only traverse these relations
	BlockedRelations []RelationType `json:"blocked_relations,omitempty"`  // Never traverse these relations
	AllowedNodeTypes []NodeType     `json:"allowed_node_types,omitempty"` // Only include these node types
	BlockedNodeTypes []NodeType     `json:"blocked_node_types,omitempty"` // Never include these node types
	MinWeight        float64        `json:"min_weight,omitempty"`         // Minimum relationship weight
	MaxDepth         int            `json:"max_depth,omitempty"`          // Override query max_hops
}

// AttackChain represents a sequence of attack techniques forming a chain.
// Used for attack path analysis and correlation.
type AttackChain struct {
	ID         types.ID     `json:"id"`
	Name       string       `json:"name"`
	Steps      []AttackStep `json:"steps"`
	MissionID  types.ID     `json:"mission_id"`
	Confidence float64      `json:"confidence"` // Overall chain confidence
	Severity   string       `json:"severity"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// AttackStep represents a single step in an attack chain.
type AttackStep struct {
	Order       int        `json:"order"`
	TechniqueID string     `json:"technique_id"`
	NodeID      types.ID   `json:"node_id"`
	Description string     `json:"description"`
	Evidence    []types.ID `json:"evidence"` // Finding IDs as evidence
	Confidence  float64    `json:"confidence"`
}

// NewAttackChain creates a new AttackChain.
func NewAttackChain(name string, missionID types.ID) *AttackChain {
	now := time.Now()
	return &AttackChain{
		ID:         types.NewID(),
		Name:       name,
		Steps:      []AttackStep{},
		MissionID:  missionID,
		Confidence: 1.0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// AddStep adds a step to the attack chain.
func (ac *AttackChain) AddStep(step AttackStep) {
	step.Order = len(ac.Steps) + 1
	ac.Steps = append(ac.Steps, step)
	ac.UpdatedAt = time.Now()
}

// VectorResult represents a pure vector search result (no graph traversal).
// Used for initial similarity search before graph expansion.
type VectorResult struct {
	NodeID     types.ID       `json:"node_id"`
	Similarity float64        `json:"similarity"` // Cosine similarity (0-1)
	Embedding  []float64      `json:"embedding,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// NewVectorResult creates a new VectorResult.
func NewVectorResult(nodeID types.ID, similarity float64) *VectorResult {
	return &VectorResult{
		NodeID:     nodeID,
		Similarity: similarity,
		Metadata:   make(map[string]any),
	}
}

// WithEmbedding sets the embedding vector.
func (vr *VectorResult) WithEmbedding(embedding []float64) *VectorResult {
	vr.Embedding = embedding
	return vr
}

// WithMetadata sets metadata for the result.
func (vr *VectorResult) WithMetadata(metadata map[string]any) *VectorResult {
	vr.Metadata = metadata
	return vr
}
