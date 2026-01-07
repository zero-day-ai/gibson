package verbose

import (
	"context"
	"sync"

	"github.com/zero-day-ai/gibson/internal/types"
)

// VerboseEventBus publishes verbose events to subscribers.
// Implementations must be thread-safe and support multiple concurrent subscribers.
type VerboseEventBus interface {
	// Emit publishes an event to all subscribers.
	// Uses non-blocking send to prevent slow subscribers from blocking producers.
	// Returns an error if the event bus is closed.
	Emit(ctx context.Context, event VerboseEvent) error

	// Subscribe creates a new subscription and returns a channel for receiving events
	// and a cleanup function to unsubscribe.
	// The cleanup function must be called to prevent resource leaks.
	Subscribe(ctx context.Context) (<-chan VerboseEvent, func())

	// Close shuts down the event bus and all subscriptions.
	Close() error
}

// DefaultVerboseEventBus implements VerboseEventBus using buffered channels.
// It supports multiple subscribers and handles slow consumers gracefully by
// dropping events if subscriber buffers are full.
type DefaultVerboseEventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan VerboseEvent
	bufferSize  int
	closed      bool
}

// VerboseEventBusOption is a functional option for configuring DefaultVerboseEventBus.
type VerboseEventBusOption func(*DefaultVerboseEventBus)

// WithBufferSize sets the buffer size for subscriber channels.
// Default is 1000. Larger buffers can handle bursty events better.
func WithBufferSize(size int) VerboseEventBusOption {
	return func(e *DefaultVerboseEventBus) {
		e.bufferSize = size
	}
}

// NewDefaultVerboseEventBus creates a new DefaultVerboseEventBus with optional configuration.
func NewDefaultVerboseEventBus(opts ...VerboseEventBusOption) *DefaultVerboseEventBus {
	bus := &DefaultVerboseEventBus{
		subscribers: make(map[string]chan VerboseEvent),
		bufferSize:  1000, // Default buffer size
		closed:      false,
	}

	for _, opt := range opts {
		opt(bus)
	}

	return bus
}

// Emit publishes an event to all subscribers.
// If a subscriber's channel is full, the event is dropped for that subscriber
// to prevent blocking other subscribers (slow consumer handling).
func (b *DefaultVerboseEventBus) Emit(ctx context.Context, event VerboseEvent) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return types.NewError(types.ErrorCode("VERBOSE_BUS_CLOSED"), "verbose event bus is closed")
	}

	// Send to all subscribers (non-blocking)
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
			// Event sent successfully
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Channel is full, drop event for this slow subscriber
			// This prevents one slow subscriber from blocking others
		}
	}

	return nil
}

// Subscribe creates a new subscription and returns a channel for receiving events.
// The returned cleanup function must be called to unsubscribe and prevent leaks.
func (b *DefaultVerboseEventBus) Subscribe(ctx context.Context) (<-chan VerboseEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Generate unique subscriber ID
	subscriberID := types.NewID().String()
	ch := make(chan VerboseEvent, b.bufferSize)
	b.subscribers[subscriberID] = ch

	// Cleanup function to unsubscribe
	cleanup := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if subCh, exists := b.subscribers[subscriberID]; exists {
			delete(b.subscribers, subscriberID)
			close(subCh)
		}
	}

	return ch, cleanup
}

// Close shuts down the event bus and closes all subscriber channels.
func (b *DefaultVerboseEventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all subscriber channels
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}

	return nil
}

// SubscriberCount returns the current number of active subscribers.
// Useful for monitoring and testing.
func (b *DefaultVerboseEventBus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// Ensure DefaultVerboseEventBus implements VerboseEventBus at compile time
var _ VerboseEventBus = (*DefaultVerboseEventBus)(nil)
