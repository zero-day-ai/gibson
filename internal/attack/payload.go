package attack

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/payload"
	"github.com/zero-day-ai/gibson/internal/types"
)

// PayloadFilter filters payloads for an attack based on various criteria.
// It integrates with the PayloadRegistry to retrieve and filter payloads
// by ID, category, and MITRE technique. When no filters are specified,
// it returns all available payloads from the registry.
type PayloadFilter interface {
	// Filter applies the attack options to filter payloads
	Filter(ctx context.Context, opts *AttackOptions) ([]payload.Payload, error)
}

// DefaultPayloadFilter implements PayloadFilter using the PayloadRegistry.
// It supports filtering by:
// - Payload IDs: Specific payloads to include
// - Category: Filter by payload category (e.g., "jailbreak", "prompt_injection")
// - MITRE Techniques: Filter by MITRE ATT&CK technique IDs
// When no filters are specified, all payloads are returned.
type DefaultPayloadFilter struct {
	registry payload.PayloadRegistry
}

// NewPayloadFilter creates a new DefaultPayloadFilter with the given registry.
func NewPayloadFilter(registry payload.PayloadRegistry) *DefaultPayloadFilter {
	return &DefaultPayloadFilter{
		registry: registry,
	}
}

// Filter applies the attack options to filter payloads from the registry.
// It applies filters in the following order:
// 1. If PayloadIDs are specified, only those payloads are returned
// 2. If PayloadCategory is specified, filter by category
// 3. If Techniques are specified, filter by MITRE techniques
// 4. If no filters are specified, return all enabled payloads
//
// Returns an error if:
// - The registry cannot be queried
// - Specified payload IDs are not found
// - Invalid category is specified
func (f *DefaultPayloadFilter) Filter(ctx context.Context, opts *AttackOptions) ([]payload.Payload, error) {
	if opts == nil {
		return nil, fmt.Errorf("attack options cannot be nil")
	}

	// Case 1: Filter by specific payload IDs
	if len(opts.PayloadIDs) > 0 {
		return f.filterByIDs(ctx, opts.PayloadIDs)
	}

	// Case 2: Build filter based on category and techniques
	filter := &payload.PayloadFilter{
		Enabled: boolPtr(true), // Only return enabled payloads
	}

	// Apply category filter if specified
	if opts.PayloadCategory != "" {
		category := payload.PayloadCategory(opts.PayloadCategory)
		if !category.IsValid() {
			return nil, fmt.Errorf("invalid payload category: %s", opts.PayloadCategory)
		}
		filter.Categories = []payload.PayloadCategory{category}
	}

	// Apply MITRE technique filter if specified
	if len(opts.Techniques) > 0 {
		filter.MitreTechniques = opts.Techniques
	}

	// Query the registry with the filter
	payloadPtrs, err := f.registry.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list payloads: %w", err)
	}

	// Convert []*Payload to []Payload
	payloads := make([]payload.Payload, len(payloadPtrs))
	for i, p := range payloadPtrs {
		if p == nil {
			continue
		}
		payloads[i] = *p
	}

	return payloads, nil
}

// filterByIDs retrieves specific payloads by their IDs.
// Returns an error if any payload ID is not found.
func (f *DefaultPayloadFilter) filterByIDs(ctx context.Context, payloadIDs []string) ([]payload.Payload, error) {
	payloads := make([]payload.Payload, 0, len(payloadIDs))

	for _, idStr := range payloadIDs {
		// Parse the ID string
		id := types.ID(idStr)
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("invalid payload ID %s: %w", idStr, err)
		}

		// Retrieve the payload
		p, err := f.registry.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get payload %s: %w", idStr, err)
		}

		if p == nil {
			return nil, fmt.Errorf("payload %s not found", idStr)
		}

		payloads = append(payloads, *p)
	}

	return payloads, nil
}

// boolPtr returns a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}
