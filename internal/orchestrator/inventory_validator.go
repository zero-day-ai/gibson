package orchestrator

import (
	"fmt"
	"strings"
)

// InventoryValidator validates orchestrator decisions against the component inventory.
// It ensures that decisions reference only valid, registered components (agents, tools, plugins).
type InventoryValidator struct {
	inventory *ComponentInventory
}

// NewInventoryValidator creates a new InventoryValidator with the given component inventory.
// The inventory can be nil, in which case validation is skipped gracefully.
func NewInventoryValidator(inv *ComponentInventory) *InventoryValidator {
	return &InventoryValidator{
		inventory: inv,
	}
}

// ValidateSpawnAgent validates a spawn_agent decision against the component inventory.
// It checks if the requested agent name exists in the inventory.
//
// Returns nil if:
//   - decision is nil
//   - decision.Action is not ActionSpawnAgent
//   - the agent name is valid and exists in inventory
//
// Returns ComponentValidationError if:
//   - spawn_config is nil
//   - agent name doesn't exist in inventory (includes suggestions)
func (v *InventoryValidator) ValidateSpawnAgent(decision *Decision) error {
	// Not applicable for non-spawn-agent decisions
	if decision == nil || decision.Action != ActionSpawnAgent {
		return nil
	}

	// Spawn config is required for spawn_agent action
	if decision.SpawnConfig == nil {
		return fmt.Errorf("spawn_config is required for spawn_agent action")
	}

	// If no inventory available, skip validation (log warning externally)
	if v.inventory == nil {
		return nil
	}

	// Check if agent exists in inventory
	agentName := decision.SpawnConfig.AgentName
	if v.inventory.HasAgent(agentName) {
		return nil
	}

	// Agent not found - return validation error with suggestions
	available := v.inventory.AgentNames()
	suggestions := v.SuggestAlternatives(agentName, "agent")

	return &ComponentValidationError{
		ComponentType: "agent",
		RequestedName: agentName,
		Available:     available,
		Suggestions:   suggestions,
		Message: fmt.Sprintf(
			"agent %q not found in registry. Available agents: %s",
			agentName,
			strings.Join(available, ", "),
		),
	}
}

// ValidateDecision validates any decision that references components.
// It routes to appropriate validators based on the decision action type.
//
// Currently validates:
//   - ActionSpawnAgent: validates agent name exists in inventory
//
// Returns nil for:
//   - nil decision
//   - nil inventory (validation skipped)
//   - actions that don't reference components
//
// Returns error if validation fails for a specific action.
func (v *InventoryValidator) ValidateDecision(decision *Decision) error {
	// Handle nil decision
	if decision == nil {
		return fmt.Errorf("decision is nil")
	}

	// If no inventory available, skip validation gracefully
	if v.inventory == nil {
		// Log warning externally: "No component inventory available, skipping validation"
		return nil
	}

	// Route to appropriate validator based on action
	switch decision.Action {
	case ActionSpawnAgent:
		return v.ValidateSpawnAgent(decision)

	case ActionExecuteAgent, ActionSkipAgent, ActionModifyParams, ActionRetry, ActionComplete:
		// These actions don't reference components (yet)
		// Future: validate tool references in modify_params
		return nil

	default:
		// Unknown action - pass through
		return nil
	}
}

// SuggestAlternatives returns component names similar to the given name using fuzzy matching.
// It uses simple string matching techniques:
//  1. Substring matches (case-insensitive)
//  2. Prefix matches
//  3. Levenshtein distance for typo detection
//
// Returns up to 3 most similar component names.
// Returns empty slice if no good matches found (distance > 3).
func (v *InventoryValidator) SuggestAlternatives(name string, componentType string) []string {
	if v.inventory == nil {
		return []string{}
	}

	var candidates []string
	switch componentType {
	case "agent":
		candidates = v.inventory.AgentNames()
	case "tool":
		candidates = v.inventory.ToolNames()
	case "plugin":
		candidates = v.inventory.PluginNames()
	default:
		return []string{}
	}

	if len(candidates) == 0 {
		return []string{}
	}

	// Score each candidate
	type scored struct {
		name  string
		score int
	}

	nameLower := strings.ToLower(name)
	scores := make([]scored, 0, len(candidates))

	for _, candidate := range candidates {
		candidateLower := strings.ToLower(candidate)

		// Calculate score (lower is better)
		var score int

		// Exact match (shouldn't happen, but just in case)
		if candidateLower == nameLower {
			score = 0
		} else if strings.Contains(candidateLower, nameLower) || strings.Contains(nameLower, candidateLower) {
			// Substring match - very good
			score = 1
		} else if strings.HasPrefix(candidateLower, nameLower) || strings.HasPrefix(nameLower, candidateLower) {
			// Prefix match - good
			score = 2
		} else {
			// Use Levenshtein distance for typo detection
			distance := levenshteinDistance(nameLower, candidateLower)
			if distance > 3 {
				// Too different - skip
				continue
			}
			score = 10 + distance // Lower priority than substring/prefix matches
		}

		scores = append(scores, scored{name: candidate, score: score})
	}

	// Sort by score (lower is better)
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score < scores[i].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// Return top 3
	result := make([]string, 0, 3)
	for i := 0; i < len(scores) && i < 3; i++ {
		result = append(result, scores[i].name)
	}

	return result
}

// levenshteinDistance computes the Levenshtein distance between two strings.
// This is the minimum number of single-character edits (insertions, deletions, substitutions)
// required to change one string into the other.
//
// This implementation uses dynamic programming with O(m*n) time and O(min(m,n)) space complexity.
func levenshteinDistance(a, b string) int {
	// Convert to rune slices for proper Unicode handling
	aRunes := []rune(a)
	bRunes := []rune(b)

	aLen := len(aRunes)
	bLen := len(bRunes)

	// Handle edge cases
	if aLen == 0 {
		return bLen
	}
	if bLen == 0 {
		return aLen
	}

	// Use space-optimized DP (only need previous row)
	// Make sure to iterate over the shorter string as columns
	if aLen > bLen {
		aRunes, bRunes = bRunes, aRunes
		aLen, bLen = bLen, aLen
	}

	// Create a single row for DP
	prevRow := make([]int, aLen+1)
	currRow := make([]int, aLen+1)

	// Initialize first row
	for i := 0; i <= aLen; i++ {
		prevRow[i] = i
	}

	// Calculate distances
	for i := 1; i <= bLen; i++ {
		currRow[0] = i

		for j := 1; j <= aLen; j++ {
			cost := 1
			if bRunes[i-1] == aRunes[j-1] {
				cost = 0
			}

			currRow[j] = min(
				currRow[j-1]+1,    // insertion
				prevRow[j]+1,      // deletion
				prevRow[j-1]+cost, // substitution
			)
		}

		// Swap rows
		prevRow, currRow = currRow, prevRow
	}

	return prevRow[aLen]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
