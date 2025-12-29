package finding

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// FindingStatus represents the lifecycle status of a finding
type FindingStatus string

const (
	StatusOpen          FindingStatus = "open"
	StatusConfirmed     FindingStatus = "confirmed"
	StatusResolved      FindingStatus = "resolved"
	StatusFalsePositive FindingStatus = "false_positive"
)

// FindingCategory represents the type of security finding
type FindingCategory string

const (
	CategoryJailbreak             FindingCategory = "jailbreak"
	CategoryPromptInjection       FindingCategory = "prompt_injection"
	CategoryDataExtraction        FindingCategory = "data_extraction"
	CategoryPrivilegeEscalation   FindingCategory = "privilege_escalation"
	CategoryDoS                   FindingCategory = "dos"
	CategoryModelManipulation     FindingCategory = "model_manipulation"
	CategoryInformationDisclosure FindingCategory = "information_disclosure"
	CategoryUncategorized         FindingCategory = "uncategorized"
)

// String returns the string representation of FindingCategory
func (fc FindingCategory) String() string {
	return string(fc)
}

// IsValid checks if the category is a valid value
func (fc FindingCategory) IsValid() bool {
	switch fc {
	case CategoryJailbreak, CategoryPromptInjection, CategoryDataExtraction,
		CategoryPrivilegeEscalation, CategoryDoS, CategoryModelManipulation,
		CategoryInformationDisclosure, CategoryUncategorized:
		return true
	default:
		return false
	}
}

// EnhancedFinding extends agent.Finding with additional classification and tracking metadata
type EnhancedFinding struct {
	agent.Finding // Embedded base finding type

	// Context
	MissionID     types.ID `json:"mission_id"`
	AgentName     string   `json:"agent_name"`
	DelegatedFrom *string  `json:"delegated_from,omitempty"`

	// Classification
	Subcategory string        `json:"subcategory"`
	Status      FindingStatus `json:"status"`
	RiskScore   float64       `json:"risk_score"` // 0.0 - 10.0 (CVSS-like)

	// Remediation
	Remediation string   `json:"remediation"`
	References  []string `json:"references,omitempty"`

	// Reproduction
	ReproSteps []ReproStep `json:"repro_steps,omitempty"`

	// MITRE Mappings
	MitreAttack []SimpleMitreMapping `json:"mitre_attack,omitempty"`
	MitreAtlas  []SimpleMitreMapping `json:"mitre_atlas,omitempty"`

	// Relationships
	RelatedIDs      []types.ID `json:"related_ids,omitempty"`
	OccurrenceCount int        `json:"occurrence_count"`

	// Timestamps
	UpdatedAt time.Time `json:"updated_at"`
}

// ReproStep represents a single step in a reproduction sequence
type ReproStep struct {
	StepNumber     int    `json:"step_number"`
	Description    string `json:"description"`
	ExpectedResult string `json:"expected_result"`
	EvidenceRef    string `json:"evidence_ref,omitempty"` // Reference to evidence by title
}

// SimpleMitreMapping is a simplified MITRE mapping for JSON storage
type SimpleMitreMapping struct {
	TechniqueID   string `json:"technique_id"`   // e.g., "T1566", "AML.T0043"
	TechniqueName string `json:"technique_name"` // Human-readable name
	Tactic        string `json:"tactic,omitempty"` // e.g., "Initial Access"
}

// Classification represents the output of the finding classifier
type Classification struct {
	Category    FindingCategory       `json:"category"`
	Subcategory string                `json:"subcategory"`
	Severity    agent.FindingSeverity `json:"severity"`
	Confidence  float64               `json:"confidence"` // 0.0 - 1.0
	MitreAttack []SimpleMitreMapping  `json:"mitre_attack,omitempty"`
	MitreAtlas  []SimpleMitreMapping  `json:"mitre_atlas,omitempty"`
	RiskScore   float64               `json:"risk_score"` // 0.0 - 10.0
	Remediation string                `json:"remediation"`
	NeedsReview bool                  `json:"needs_review"` // True if classifier is uncertain

	// Metadata about the classification
	Method    ClassificationMethod `json:"method"`    // Which classifier produced this result
	Rationale string               `json:"rationale"` // Explanation for the classification
}

// ClassificationMethod indicates which classifier produced the classification
type ClassificationMethod string

const (
	MethodHeuristic ClassificationMethod = "heuristic"
	MethodLLM       ClassificationMethod = "llm"
	MethodComposite ClassificationMethod = "composite"
	MethodManual    ClassificationMethod = "manual"
)

// String returns the string representation of ClassificationMethod
func (cm ClassificationMethod) String() string {
	return string(cm)
}

// NewEnhancedFinding creates a new enhanced finding from a base finding
func NewEnhancedFinding(baseFinding agent.Finding, missionID types.ID, agentName string) EnhancedFinding {
	now := time.Now()
	return EnhancedFinding{
		Finding:         baseFinding,
		MissionID:       missionID,
		AgentName:       agentName,
		Status:          StatusOpen,
		RiskScore:       0.0,
		References:      []string{},
		ReproSteps:      []ReproStep{},
		MitreAttack:     []SimpleMitreMapping{},
		MitreAtlas:      []SimpleMitreMapping{},
		RelatedIDs:      []types.ID{},
		OccurrenceCount: 1,
		UpdatedAt:       now,
	}
}

// WithClassification applies classification results to the finding
func (f EnhancedFinding) WithClassification(c Classification) EnhancedFinding {
	f.Category = string(c.Category)
	f.Subcategory = c.Subcategory
	f.Severity = c.Severity
	f.Confidence = c.Confidence
	f.RiskScore = c.RiskScore
	f.Remediation = c.Remediation
	f.MitreAttack = c.MitreAttack
	f.MitreAtlas = c.MitreAtlas
	f.UpdatedAt = time.Now()
	return f
}

// WithStatus sets the finding status
func (f EnhancedFinding) WithStatus(status FindingStatus) EnhancedFinding {
	f.Status = status
	f.UpdatedAt = time.Now()
	return f
}

// WithReproSteps sets the reproduction steps
func (f EnhancedFinding) WithReproSteps(steps []ReproStep) EnhancedFinding {
	f.ReproSteps = steps
	f.UpdatedAt = time.Now()
	return f
}

// WithReferences sets the reference URLs
func (f EnhancedFinding) WithReferences(refs ...string) EnhancedFinding {
	f.References = refs
	f.UpdatedAt = time.Now()
	return f
}

// WithRelatedFindings sets related finding IDs
func (f EnhancedFinding) WithRelatedFindings(ids ...types.ID) EnhancedFinding {
	f.RelatedIDs = ids
	f.UpdatedAt = time.Now()
	return f
}

// WithDelegation marks the finding as delegated from another agent
func (f EnhancedFinding) WithDelegation(fromAgent string) EnhancedFinding {
	f.DelegatedFrom = &fromAgent
	f.UpdatedAt = time.Now()
	return f
}

// IncrementOccurrence increments the occurrence count
func (f *EnhancedFinding) IncrementOccurrence() {
	f.OccurrenceCount++
	f.UpdatedAt = time.Now()
}

// IsConfirmed returns true if the finding has been confirmed
func (f EnhancedFinding) IsConfirmed() bool {
	return f.Status == StatusConfirmed
}

// IsResolved returns true if the finding has been resolved
func (f EnhancedFinding) IsResolved() bool {
	return f.Status == StatusResolved
}

// IsFalsePositive returns true if the finding is marked as a false positive
func (f EnhancedFinding) IsFalsePositive() bool {
	return f.Status == StatusFalsePositive
}

// IsCritical returns true if the finding is critical severity
func (f EnhancedFinding) IsCritical() bool {
	return f.Severity == agent.SeverityCritical
}

// NeedsAttention returns true if the finding needs immediate attention
func (f EnhancedFinding) NeedsAttention() bool {
	return (f.Severity == agent.SeverityCritical || f.Severity == agent.SeverityHigh) &&
		(f.Status == StatusOpen || f.Status == StatusConfirmed)
}
