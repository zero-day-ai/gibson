package graphrag

import (
	"testing"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNodeType_String(t *testing.T) {
	tests := []struct {
		name     string
		nodeType NodeType
		want     string
	}{
		{
			name:     "Finding",
			nodeType: NodeTypeFinding,
			want:     "Finding",
		},
		{
			name:     "AttackPattern",
			nodeType: NodeTypeAttackPattern,
			want:     "AttackPattern",
		},
		{
			name:     "Technique",
			nodeType: NodeTypeTechnique,
			want:     "Technique",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.nodeType.String(); got != tt.want {
				t.Errorf("NodeType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		nodeType NodeType
		want     bool
	}{
		{
			name:     "valid - Finding",
			nodeType: NodeTypeFinding,
			want:     true,
		},
		{
			name:     "valid - AttackPattern",
			nodeType: NodeTypeAttackPattern,
			want:     true,
		},
		{
			name:     "invalid",
			nodeType: NodeType("Invalid"),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.nodeType.IsValid(); got != tt.want {
				t.Errorf("NodeType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRelationType_String(t *testing.T) {
	tests := []struct {
		name         string
		relationType RelationType
		want         string
	}{
		{
			name:         "EXPLOITS",
			relationType: RelationExploits,
			want:         "EXPLOITS",
		},
		{
			name:         "SIMILAR_TO",
			relationType: RelationSimilarTo,
			want:         "SIMILAR_TO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.relationType.String(); got != tt.want {
				t.Errorf("RelationType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRelationType_IsValid(t *testing.T) {
	tests := []struct {
		name         string
		relationType RelationType
		want         bool
	}{
		{
			name:         "valid - EXPLOITS",
			relationType: RelationExploits,
			want:         true,
		},
		{
			name:         "valid - SIMILAR_TO",
			relationType: RelationSimilarTo,
			want:         true,
		},
		{
			name:         "invalid",
			relationType: RelationType("INVALID"),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.relationType.IsValid(); got != tt.want {
				t.Errorf("RelationType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewGraphNode(t *testing.T) {
	id := types.NewID()
	node := NewGraphNode(id, NodeTypeFinding, NodeTypeEntity)

	if node.ID != id {
		t.Errorf("NewGraphNode() ID = %v, want %v", node.ID, id)
	}
	if len(node.Labels) != 2 {
		t.Errorf("NewGraphNode() Labels count = %v, want 2", len(node.Labels))
	}
	if node.Labels[0] != NodeTypeFinding {
		t.Errorf("NewGraphNode() Labels[0] = %v, want %v", node.Labels[0], NodeTypeFinding)
	}
	if node.Properties == nil {
		t.Error("NewGraphNode() Properties should not be nil")
	}
	if node.CreatedAt.IsZero() {
		t.Error("NewGraphNode() CreatedAt should not be zero")
	}
}

func TestGraphNode_WithProperty(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	result := node.WithProperty("key", "value")

	if result != node {
		t.Error("WithProperty() should return the same node for chaining")
	}
	if node.Properties["key"] != "value" {
		t.Errorf("WithProperty() property = %v, want 'value'", node.Properties["key"])
	}
}

func TestGraphNode_WithProperties(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	props := map[string]any{
		"key1": "value1",
		"key2": 42,
	}
	result := node.WithProperties(props)

	if result != node {
		t.Error("WithProperties() should return the same node for chaining")
	}
	if node.Properties["key1"] != "value1" {
		t.Errorf("WithProperties() key1 = %v, want 'value1'", node.Properties["key1"])
	}
	if node.Properties["key2"] != 42 {
		t.Errorf("WithProperties() key2 = %v, want 42", node.Properties["key2"])
	}
}

func TestGraphNode_WithEmbedding(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	embedding := []float64{0.1, 0.2, 0.3}
	result := node.WithEmbedding(embedding)

	if result != node {
		t.Error("WithEmbedding() should return the same node for chaining")
	}
	if len(node.Embedding) != 3 {
		t.Errorf("WithEmbedding() embedding length = %v, want 3", len(node.Embedding))
	}
}

func TestGraphNode_WithMission(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	missionID := types.NewID()
	result := node.WithMission(missionID)

	if result != node {
		t.Error("WithMission() should return the same node for chaining")
	}
	if node.MissionID == nil {
		t.Error("WithMission() MissionID should not be nil")
	}
	if *node.MissionID != missionID {
		t.Errorf("WithMission() MissionID = %v, want %v", *node.MissionID, missionID)
	}
}

func TestGraphNode_HasLabel(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding, NodeTypeEntity)

	tests := []struct {
		name  string
		label NodeType
		want  bool
	}{
		{
			name:  "has label - Finding",
			label: NodeTypeFinding,
			want:  true,
		},
		{
			name:  "has label - Entity",
			label: NodeTypeEntity,
			want:  true,
		},
		{
			name:  "does not have label - Technique",
			label: NodeTypeTechnique,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := node.HasLabel(tt.label); got != tt.want {
				t.Errorf("HasLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphNode_GetProperty(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	node.WithProperty("key", "value")

	tests := []struct {
		name string
		key  string
		want any
	}{
		{
			name: "existing property",
			key:  "key",
			want: "value",
		},
		{
			name: "non-existent property",
			key:  "nonexistent",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := node.GetProperty(tt.key)
			if got != tt.want {
				t.Errorf("GetProperty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphNode_GetStringProperty(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	node.WithProperty("string_key", "string_value")
	node.WithProperty("int_key", 42)

	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "string property",
			key:  "string_key",
			want: "string_value",
		},
		{
			name: "non-string property",
			key:  "int_key",
			want: "",
		},
		{
			name: "non-existent property",
			key:  "nonexistent",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := node.GetStringProperty(tt.key)
			if got != tt.want {
				t.Errorf("GetStringProperty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGraphNode_Validate(t *testing.T) {
	tests := []struct {
		name    string
		node    *GraphNode
		wantErr bool
	}{
		{
			name:    "valid node",
			node:    NewGraphNode(types.NewID(), NodeTypeFinding),
			wantErr: false,
		},
		{
			name: "invalid - no labels",
			node: &GraphNode{
				ID:         types.NewID(),
				Labels:     []NodeType{},
				Properties: make(map[string]any),
			},
			wantErr: true,
		},
		{
			name: "invalid - invalid label",
			node: &GraphNode{
				ID:         types.NewID(),
				Labels:     []NodeType{NodeType("Invalid")},
				Properties: make(map[string]any),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRelationship(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	rel := NewRelationship(fromID, toID, RelationExploits)

	if rel.FromID != fromID {
		t.Errorf("NewRelationship() FromID = %v, want %v", rel.FromID, fromID)
	}
	if rel.ToID != toID {
		t.Errorf("NewRelationship() ToID = %v, want %v", rel.ToID, toID)
	}
	if rel.Type != RelationExploits {
		t.Errorf("NewRelationship() Type = %v, want %v", rel.Type, RelationExploits)
	}
	if rel.Weight != 1.0 {
		t.Errorf("NewRelationship() Weight = %v, want 1.0", rel.Weight)
	}
	if rel.Properties == nil {
		t.Error("NewRelationship() Properties should not be nil")
	}
}

func TestRelationship_WithProperty(t *testing.T) {
	rel := NewRelationship(types.NewID(), types.NewID(), RelationExploits)
	result := rel.WithProperty("key", "value")

	if result != rel {
		t.Error("WithProperty() should return the same relationship for chaining")
	}
	if rel.Properties["key"] != "value" {
		t.Errorf("WithProperty() property = %v, want 'value'", rel.Properties["key"])
	}
}

func TestRelationship_WithWeight(t *testing.T) {
	rel := NewRelationship(types.NewID(), types.NewID(), RelationExploits)
	result := rel.WithWeight(0.75)

	if result != rel {
		t.Error("WithWeight() should return the same relationship for chaining")
	}
	if rel.Weight != 0.75 {
		t.Errorf("WithWeight() weight = %v, want 0.75", rel.Weight)
	}
}

func TestRelationship_Validate(t *testing.T) {
	tests := []struct {
		name    string
		rel     *Relationship
		wantErr bool
	}{
		{
			name:    "valid relationship",
			rel:     NewRelationship(types.NewID(), types.NewID(), RelationExploits),
			wantErr: false,
		},
		{
			name: "invalid - invalid type",
			rel: &Relationship{
				ID:     types.NewID(),
				FromID: types.NewID(),
				ToID:   types.NewID(),
				Type:   RelationType("INVALID"),
				Weight: 1.0,
			},
			wantErr: true,
		},
		{
			name: "invalid - weight too high",
			rel: &Relationship{
				ID:     types.NewID(),
				FromID: types.NewID(),
				ToID:   types.NewID(),
				Type:   RelationExploits,
				Weight: 1.5,
			},
			wantErr: true,
		},
		{
			name: "invalid - weight negative",
			rel: &Relationship{
				ID:     types.NewID(),
				FromID: types.NewID(),
				ToID:   types.NewID(),
				Type:   RelationExploits,
				Weight: -0.1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rel.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAttackPattern(t *testing.T) {
	ap := NewAttackPattern("T1566", "Phishing", "Phishing attack description")

	if ap.TechniqueID != "T1566" {
		t.Errorf("NewAttackPattern() TechniqueID = %v, want 'T1566'", ap.TechniqueID)
	}
	if ap.Name != "Phishing" {
		t.Errorf("NewAttackPattern() Name = %v, want 'Phishing'", ap.Name)
	}
	if ap.Description != "Phishing attack description" {
		t.Errorf("NewAttackPattern() Description = %v, want 'Phishing attack description'", ap.Description)
	}
	if ap.Tactics == nil {
		t.Error("NewAttackPattern() Tactics should not be nil")
	}
	if ap.CreatedAt.IsZero() {
		t.Error("NewAttackPattern() CreatedAt should not be zero")
	}
}

func TestAttackPattern_ToGraphNode(t *testing.T) {
	ap := NewAttackPattern("T1566", "Phishing", "Phishing attack description")
	ap.Tactics = []string{"Initial Access"}
	ap.Platforms = []string{"Windows", "Linux"}
	ap.Embedding = []float64{0.1, 0.2, 0.3}

	node := ap.ToGraphNode()

	if node.ID != ap.ID {
		t.Errorf("ToGraphNode() ID = %v, want %v", node.ID, ap.ID)
	}
	if !node.HasLabel(NodeTypeAttackPattern) {
		t.Error("ToGraphNode() should have AttackPattern label")
	}
	if node.GetStringProperty("technique_id") != "T1566" {
		t.Errorf("ToGraphNode() technique_id = %v, want 'T1566'", node.GetStringProperty("technique_id"))
	}
	if len(node.Embedding) != 3 {
		t.Errorf("ToGraphNode() embedding length = %v, want 3", len(node.Embedding))
	}
}

func TestNewFindingNode(t *testing.T) {
	id := types.NewID()
	missionID := types.NewID()
	fn := NewFindingNode(id, "Test Finding", "Description", missionID)

	if fn.ID != id {
		t.Errorf("NewFindingNode() ID = %v, want %v", fn.ID, id)
	}
	if fn.Title != "Test Finding" {
		t.Errorf("NewFindingNode() Title = %v, want 'Test Finding'", fn.Title)
	}
	if fn.MissionID != missionID {
		t.Errorf("NewFindingNode() MissionID = %v, want %v", fn.MissionID, missionID)
	}
	if fn.Confidence != 1.0 {
		t.Errorf("NewFindingNode() Confidence = %v, want 1.0", fn.Confidence)
	}
}

func TestFindingNode_ToGraphNode(t *testing.T) {
	id := types.NewID()
	missionID := types.NewID()
	targetID := types.NewID()
	fn := NewFindingNode(id, "Test Finding", "Description", missionID)
	fn.TargetID = &targetID
	fn.Severity = "high"
	fn.Category = "jailbreak"
	fn.Embedding = []float64{0.1, 0.2, 0.3}

	node := fn.ToGraphNode()

	if node.ID != fn.ID {
		t.Errorf("ToGraphNode() ID = %v, want %v", node.ID, fn.ID)
	}
	if !node.HasLabel(NodeTypeFinding) {
		t.Error("ToGraphNode() should have Finding label")
	}
	if node.GetStringProperty("title") != "Test Finding" {
		t.Errorf("ToGraphNode() title = %v, want 'Test Finding'", node.GetStringProperty("title"))
	}
	if node.MissionID == nil || *node.MissionID != missionID {
		t.Errorf("ToGraphNode() MissionID = %v, want %v", node.MissionID, missionID)
	}
	if len(node.Embedding) != 3 {
		t.Errorf("ToGraphNode() embedding length = %v, want 3", len(node.Embedding))
	}
}

func TestNewTechniqueNode(t *testing.T) {
	tn := NewTechniqueNode("T1566.001", "Spearphishing Attachment", "Description", "Initial Access")

	if tn.TechniqueID != "T1566.001" {
		t.Errorf("NewTechniqueNode() TechniqueID = %v, want 'T1566.001'", tn.TechniqueID)
	}
	if tn.Name != "Spearphishing Attachment" {
		t.Errorf("NewTechniqueNode() Name = %v, want 'Spearphishing Attachment'", tn.Name)
	}
	if tn.Tactic != "Initial Access" {
		t.Errorf("NewTechniqueNode() Tactic = %v, want 'Initial Access'", tn.Tactic)
	}
	if tn.CreatedAt.IsZero() {
		t.Error("NewTechniqueNode() CreatedAt should not be zero")
	}
}

func TestTechniqueNode_ToGraphNode(t *testing.T) {
	tn := NewTechniqueNode("T1566.001", "Spearphishing Attachment", "Description", "Initial Access")
	tn.Platform = "Windows"
	tn.Embedding = []float64{0.1, 0.2, 0.3}

	node := tn.ToGraphNode()

	if node.ID != tn.ID {
		t.Errorf("ToGraphNode() ID = %v, want %v", node.ID, tn.ID)
	}
	if !node.HasLabel(NodeTypeTechnique) {
		t.Error("ToGraphNode() should have Technique label")
	}
	if node.GetStringProperty("technique_id") != "T1566.001" {
		t.Errorf("ToGraphNode() technique_id = %v, want 'T1566.001'", node.GetStringProperty("technique_id"))
	}
	if node.GetStringProperty("platform") != "Windows" {
		t.Errorf("ToGraphNode() platform = %v, want 'Windows'", node.GetStringProperty("platform"))
	}
	if len(node.Embedding) != 3 {
		t.Errorf("ToGraphNode() embedding length = %v, want 3", len(node.Embedding))
	}
}

func TestGraphNode_UpdatedAt(t *testing.T) {
	node := NewGraphNode(types.NewID(), NodeTypeFinding)
	originalTime := node.UpdatedAt

	// Sleep briefly to ensure time difference
	time.Sleep(10 * time.Millisecond)

	node.WithProperty("key", "value")

	if !node.UpdatedAt.After(originalTime) {
		t.Error("UpdatedAt should be updated after WithProperty()")
	}
}

func TestRelationship_CreatedAt(t *testing.T) {
	before := time.Now()
	rel := NewRelationship(types.NewID(), types.NewID(), RelationExploits)
	after := time.Now()

	if rel.CreatedAt.Before(before) || rel.CreatedAt.After(after) {
		t.Errorf("CreatedAt should be between %v and %v, got %v", before, after, rel.CreatedAt)
	}
}
