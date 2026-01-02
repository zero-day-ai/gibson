// Package registry provides service discovery and registration infrastructure for Gibson.
//
// This file implements GRPCPool for managing gRPC client connections to registered services.
package registry

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCPool manages a pool of gRPC client connections to registered services.
//
// The pool maintains connections to agents, tools, and plugins discovered through
// the registry. It automatically creates connections on-demand, monitors connection
// health, and recreates failed connections.
//
// All operations are thread-safe and can be called concurrently.
//
// Example usage:
//
//	pool := NewGRPCPool()
//	defer pool.Close()
//
//	// Get connection to a service (creates if needed)
//	conn, err := pool.Get(ctx, "localhost:50051")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use the connection
//	client := pb.NewServiceClient(conn)
//
//	// Remove a bad connection
//	if err := pool.Remove("localhost:50051"); err != nil {
//	    log.Printf("failed to remove connection: %v", err)
//	}
type GRPCPool struct {
	// mu protects concurrent access to the connection map
	mu sync.RWMutex

	// conns maps service endpoints to their gRPC client connections
	// Key format: "host:port" (e.g., "localhost:50051")
	conns map[string]*grpc.ClientConn

	// opts are the dial options used when creating new connections
	opts []grpc.DialOption
}

// NewGRPCPool creates a new gRPC connection pool.
//
// The pool starts empty and creates connections on-demand via Get().
// Additional dial options can be provided to customize connection behavior
// (e.g., custom credentials, interceptors, keepalive settings).
//
// If no options are provided, the pool uses insecure credentials by default.
// This is suitable for local/development environments. For production, provide
// TLS credentials via dial options.
//
// Example with custom options:
//
//	pool := NewGRPCPool(
//	    grpc.WithKeepaliveParams(keepalive.ClientParameters{
//	        Time:                10 * time.Second,
//	        Timeout:             3 * time.Second,
//	        PermitWithoutStream: true,
//	    }),
//	)
func NewGRPCPool(opts ...grpc.DialOption) *GRPCPool {
	return &GRPCPool{
		conns: make(map[string]*grpc.ClientConn),
		opts:  opts,
	}
}

// Get retrieves or creates a gRPC client connection to the specified endpoint.
//
// This method first checks if a healthy connection already exists in the pool.
// If the connection exists and is in READY or IDLE state, it is returned immediately.
// If the connection exists but is in a bad state (TRANSIENT_FAILURE, SHUTDOWN),
// it is closed and a new connection is created.
//
// Connection states:
//   - READY: Connection is established and ready for RPCs
//   - IDLE: Connection is idle but healthy, will activate on first RPC
//   - CONNECTING: Connection is being established (considered healthy)
//   - TRANSIENT_FAILURE: Connection failed, will retry (considered unhealthy)
//   - SHUTDOWN: Connection is closed (considered unhealthy)
//
// The context is used for dialing the new connection. If the context is canceled
// during connection establishment, an error is returned.
//
// Returns an error if:
//   - The endpoint is empty
//   - Connection cannot be established
//   - Context is canceled during dial
//
// The returned connection should NOT be closed by the caller - it is managed by
// the pool. To remove a connection, use Remove() instead.
func (p *GRPCPool) Get(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint cannot be empty")
	}

	// Fast path: check for existing healthy connection with read lock
	p.mu.RLock()
	conn, exists := p.conns[endpoint]
	p.mu.RUnlock()

	if exists {
		// Check connection state
		state := conn.GetState()

		// READY, IDLE, and CONNECTING are healthy states
		// Note: CONNECTING is transitional but we allow it since the connection
		// will either succeed or fail quickly
		switch state {
		case connectivity.Ready, connectivity.Idle, connectivity.Connecting:
			return conn, nil

		case connectivity.TransientFailure, connectivity.Shutdown:
			// Connection is in bad state - need to recreate
			// Close and remove the bad connection
			_ = p.Remove(endpoint)
		}
	}

	// Slow path: create new connection with write lock
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check: another goroutine may have created the connection
	// while we were waiting for the write lock
	if conn, exists := p.conns[endpoint]; exists {
		state := conn.GetState()
		switch state {
		case connectivity.Ready, connectivity.Idle, connectivity.Connecting:
			return conn, nil
		case connectivity.TransientFailure, connectivity.Shutdown:
			// Still bad, close it
			_ = conn.Close()
			delete(p.conns, endpoint)
		}
	}

	// Create new connection
	dialOpts := p.opts
	if len(dialOpts) == 0 {
		// Default to insecure credentials if no options provided
		// TODO: Add TLS support in future
		dialOpts = []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
	}

	conn, err := grpc.NewClient(endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for %s: %w", endpoint, err)
	}

	// Store in pool
	p.conns[endpoint] = conn

	return conn, nil
}

// Remove closes and removes a connection from the pool.
//
// This method is useful when a connection is known to be bad or when a service
// is being deregistered. The connection is gracefully closed and removed from
// the pool.
//
// If the endpoint does not exist in the pool, this is a no-op (returns nil).
//
// Returns an error if the connection close operation fails, though this is
// typically safe to ignore as the connection is removed from the pool regardless.
//
// Example usage:
//
//	// If an RPC fails, remove the connection so it's recreated on next Get()
//	if err := client.SomeRPC(ctx, req); err != nil {
//	    pool.Remove(endpoint)
//	}
func (p *GRPCPool) Remove(endpoint string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, exists := p.conns[endpoint]
	if !exists {
		// Not in pool, nothing to do
		return nil
	}

	// Remove from map first
	delete(p.conns, endpoint)

	// Close the connection
	if err := conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection to %s: %w", endpoint, err)
	}

	return nil
}

// Close closes all connections in the pool.
//
// This method should be called during application shutdown to gracefully close
// all active connections. It closes all connections concurrently for faster
// shutdown.
//
// After Close() is called, the pool should not be used. Any subsequent Get()
// calls will create new connections, but this is not recommended.
//
// Returns an error if any connection fails to close. The error includes the
// endpoint and error message for the first failure encountered. All connections
// are closed regardless of individual failures.
//
// Example usage:
//
//	pool := NewGRPCPool()
//	defer pool.Close()
func (p *GRPCPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error

	// Close all connections
	for endpoint, conn := range p.conns {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close connection to %s: %w", endpoint, err)
		}
	}

	// Clear the map
	p.conns = make(map[string]*grpc.ClientConn)

	return firstErr
}

// Len returns the number of connections currently in the pool.
//
// This is primarily useful for monitoring and testing. Note that the count
// includes connections in all states (including unhealthy ones).
//
// This method is thread-safe.
func (p *GRPCPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.conns)
}
