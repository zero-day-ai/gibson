package harness

import (
	"context"
	"sync"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

// FindingStore provides persistent storage for security findings discovered during agent execution.
// Findings are organized by mission ID to enable mission-scoped queries and reporting.
//
// Implementations must be safe for concurrent use from multiple goroutines, as agents
// may submit findings in parallel during mission execution.
type FindingStore interface {
	// Store persists a finding for a specific mission.
	// The finding is indexed by mission ID to enable efficient mission-scoped queries.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - missionID: The ID of the mission that produced this finding
	//   - finding: The security finding to store (must have a valid ID)
	//
	// Returns:
	//   - error: Non-nil if storage fails (e.g., database error, invalid finding)
	//
	// Example:
	//   finding := agent.NewFinding("SQL Injection", "Vulnerable endpoint found", agent.SeverityHigh)
	//   err := store.Store(ctx, missionID, finding)
	Store(ctx context.Context, missionID types.ID, finding agent.Finding) error

	// Get retrieves findings for a specific mission, optionally filtered by criteria.
	// An empty filter returns all findings for the mission.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - missionID: The ID of the mission whose findings to retrieve
	//   - filter: Optional filter to narrow results (see FindingFilter)
	//
	// Returns:
	//   - []agent.Finding: Slice of findings matching the filter (empty if none match)
	//   - error: Non-nil if retrieval fails (e.g., database error)
	//
	// Example:
	//   // Get all critical findings for a mission
	//   filter := NewFindingFilter().WithSeverity(agent.SeverityCritical)
	//   findings, err := store.Get(ctx, missionID, *filter)
	Get(ctx context.Context, missionID types.ID, filter FindingFilter) ([]agent.Finding, error)
}

// InMemoryFindingStore is a thread-safe, in-memory implementation of FindingStore.
// It stores findings in memory organized by mission ID using nested maps.
//
// This implementation is suitable for:
//   - Testing and development
//   - Single-instance deployments where persistence is not required
//   - Short-lived missions where findings don't need to survive restarts
//
// For production deployments with multiple instances or persistence requirements,
// consider implementing FindingStore backed by a database (PostgreSQL, MongoDB, etc.).
//
// Thread-safety: All methods use read-write locks to ensure safe concurrent access.
type InMemoryFindingStore struct {
	mu       sync.RWMutex
	findings map[types.ID][]agent.Finding // missionID -> findings
}

// NewInMemoryFindingStore creates a new in-memory finding store.
// The store is ready to use immediately with no additional configuration.
func NewInMemoryFindingStore() *InMemoryFindingStore {
	return &InMemoryFindingStore{
		findings: make(map[types.ID][]agent.Finding),
	}
}

// Store persists a finding in memory for the specified mission.
// The finding is appended to the mission's finding list.
//
// This implementation:
//   - Does not validate the finding (caller should ensure validity)
//   - Does not check for duplicate findings
//   - Ignores the context (no I/O operations to cancel)
func (s *InMemoryFindingStore) Store(ctx context.Context, missionID types.ID, finding agent.Finding) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize slice for this mission if it doesn't exist
	if s.findings[missionID] == nil {
		s.findings[missionID] = make([]agent.Finding, 0)
	}

	// Append the finding to the mission's list
	s.findings[missionID] = append(s.findings[missionID], finding)

	return nil
}

// Get retrieves findings for a mission, applying optional filters.
// If no findings exist for the mission, returns an empty slice (not an error).
//
// This implementation:
//   - Applies filters using FindingFilter.Matches()
//   - Returns a copy of matching findings (modifications won't affect stored data)
//   - Ignores the context (no I/O operations to cancel)
func (s *InMemoryFindingStore) Get(ctx context.Context, missionID types.ID, filter FindingFilter) ([]agent.Finding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all findings for this mission
	missionFindings, exists := s.findings[missionID]
	if !exists {
		// No findings for this mission - return empty slice
		return []agent.Finding{}, nil
	}

	// If no filter criteria, return all findings (copy to prevent external modification)
	result := make([]agent.Finding, 0, len(missionFindings))

	// Apply filter to each finding
	for _, finding := range missionFindings {
		if filter.Matches(finding) {
			result = append(result, finding)
		}
	}

	return result, nil
}

// Count returns the total number of findings stored for a mission.
// This is a convenience method for tracking mission progress.
func (s *InMemoryFindingStore) Count(missionID types.ID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.findings[missionID])
}

// Clear removes all findings for a specific mission.
// This is useful for cleaning up after mission completion or cancellation.
func (s *InMemoryFindingStore) Clear(missionID types.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.findings, missionID)
}

// ClearAll removes all findings for all missions.
// This is primarily useful for testing scenarios.
func (s *InMemoryFindingStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.findings = make(map[types.ID][]agent.Finding)
}

// Ensure InMemoryFindingStore implements FindingStore at compile time
var _ FindingStore = (*InMemoryFindingStore)(nil)
