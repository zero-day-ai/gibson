package planning

import (
	"strings"
	"sync"
	"time"
)

// AttemptRecord tracks a single attempt at executing a node with a specific approach.
type AttemptRecord struct {
	Timestamp     time.Time // When the attempt was made
	Approach      string    // Description of the approach (strategy, parameters, etc.)
	Result        string    // What happened (error message, result summary, etc.)
	Success       bool      // Whether the attempt succeeded
	FindingsCount int       // Number of findings discovered during this attempt
}

// ReplanRecord tracks a single replanning event.
type ReplanRecord struct {
	Timestamp   time.Time // When the replan occurred
	TriggerNode string    // Node that triggered the replan
	OldPlan     []string  // Node order before replanning
	NewPlan     []string  // Node order after replanning
	Rationale   string    // Why the replan was necessary
}

// FailureMemory tracks all attempted approaches and replanning history
// to prevent infinite loops and repeated failures.
type FailureMemory struct {
	// Attempts tracks all attempts per node ID
	Attempts map[string][]AttemptRecord

	// ReplanHistory tracks all replanning events
	ReplanHistory []ReplanRecord

	// mu protects concurrent access to all fields
	mu sync.RWMutex
}

// NewFailureMemory creates a new FailureMemory instance.
func NewFailureMemory() *FailureMemory {
	return &FailureMemory{
		Attempts:      make(map[string][]AttemptRecord),
		ReplanHistory: make([]ReplanRecord, 0),
	}
}

// RecordAttempt records a new attempt for a given node.
// Thread-safe for concurrent writes.
func (fm *FailureMemory) RecordAttempt(nodeID string, attempt AttemptRecord) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Ensure timestamp is set
	if attempt.Timestamp.IsZero() {
		attempt.Timestamp = time.Now()
	}

	fm.Attempts[nodeID] = append(fm.Attempts[nodeID], attempt)
}

// RecordReplan records a new replanning event.
// Thread-safe for concurrent writes.
func (fm *FailureMemory) RecordReplan(record ReplanRecord) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Ensure timestamp is set
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	fm.ReplanHistory = append(fm.ReplanHistory, record)
}

// HasTried checks if a specific approach has already been tried for a node.
// Uses fuzzy matching to detect similar approaches even if not exactly identical.
// Thread-safe for concurrent reads.
func (fm *FailureMemory) HasTried(nodeID string, approach string) bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	attempts, exists := fm.Attempts[nodeID]
	if !exists || len(attempts) == 0 {
		return false
	}

	// Normalize the approach for comparison
	normalizedApproach := normalizeApproach(approach)

	// Check each previous attempt using fuzzy matching
	for _, attempt := range attempts {
		normalizedAttempt := normalizeApproach(attempt.Approach)

		// Fuzzy matching: check if approaches are similar
		if isSimilarApproach(normalizedApproach, normalizedAttempt) {
			return true
		}
	}

	return false
}

// GetAttempts returns all recorded attempts for a given node.
// Returns a copy to prevent external modification.
// Thread-safe for concurrent reads.
func (fm *FailureMemory) GetAttempts(nodeID string) []AttemptRecord {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	attempts, exists := fm.Attempts[nodeID]
	if !exists {
		return []AttemptRecord{}
	}

	// Return a copy to prevent external modification
	result := make([]AttemptRecord, len(attempts))
	copy(result, attempts)
	return result
}

// ReplanCount returns the total number of replanning events.
// Thread-safe for concurrent reads.
func (fm *FailureMemory) ReplanCount() int {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	return len(fm.ReplanHistory)
}

// GetReplanHistory returns a copy of all replanning records.
// Thread-safe for concurrent reads.
func (fm *FailureMemory) GetReplanHistory() []ReplanRecord {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make([]ReplanRecord, len(fm.ReplanHistory))
	copy(result, fm.ReplanHistory)
	return result
}

// Clear resets all failure memory.
// Useful for testing or resetting state.
// Thread-safe for concurrent writes.
func (fm *FailureMemory) Clear() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.Attempts = make(map[string][]AttemptRecord)
	fm.ReplanHistory = make([]ReplanRecord, 0)
}

// normalizeApproach normalizes an approach string for comparison.
// Converts to lowercase, trims whitespace, and removes extra spaces.
func normalizeApproach(approach string) string {
	// Convert to lowercase
	normalized := strings.ToLower(approach)

	// Trim leading/trailing whitespace
	normalized = strings.TrimSpace(normalized)

	// Replace multiple spaces with single space
	normalized = strings.Join(strings.Fields(normalized), " ")

	return normalized
}

// isSimilarApproach checks if two normalized approaches are similar.
// Uses multiple heuristics to detect similarity:
// 1. Exact match
// 2. One contains the other (substring match)
// 3. High word overlap (Jaccard similarity)
func isSimilarApproach(approach1, approach2 string) bool {
	// Handle empty strings - only similar if both are empty
	if approach1 == "" && approach2 == "" {
		return true
	}
	if approach1 == "" || approach2 == "" {
		return false
	}

	// Exact match
	if approach1 == approach2 {
		return true
	}

	// Substring match (one contains the other)
	// Note: We already checked for empty strings above
	if strings.Contains(approach1, approach2) || strings.Contains(approach2, approach1) {
		return true
	}

	// Word-based similarity (Jaccard similarity)
	words1 := strings.Fields(approach1)
	words2 := strings.Fields(approach2)

	// Handle empty cases
	if len(words1) == 0 || len(words2) == 0 {
		return false
	}

	// Calculate Jaccard similarity
	similarity := jaccardSimilarity(words1, words2)

	// Consider similar if > 70% overlap
	return similarity > 0.7
}

// jaccardSimilarity calculates the Jaccard similarity between two word sets.
// Returns a value between 0 and 1, where 1 means identical sets.
func jaccardSimilarity(words1, words2 []string) float64 {
	// Build sets from word slices
	set1 := make(map[string]bool)
	for _, word := range words1 {
		set1[word] = true
	}

	set2 := make(map[string]bool)
	for _, word := range words2 {
		set2[word] = true
	}

	// Count intersection
	intersection := 0
	for word := range set1 {
		if set2[word] {
			intersection++
		}
	}

	// Count union
	union := len(set1) + len(set2) - intersection

	// Avoid division by zero
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}
