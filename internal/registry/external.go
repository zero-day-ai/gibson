// Package registry provides service discovery and registration infrastructure for Gibson.
//
// This file implements ExternalRegistry which connects to an external etcd cluster
// for production deployments with high availability support.
package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/zero-day-ai/sdk/registry"
)

// ExternalRegistry implements the Registry interface using an external etcd cluster.
//
// This provides production-grade service discovery with support for high availability,
// TLS encryption, and multi-node etcd clusters. Unlike EmbeddedRegistry, this connects
// to an existing etcd cluster rather than starting an in-process server.
//
// Example:
//
//	cfg := registry.Config{
//	    Type:      "etcd",
//	    Endpoints: []string{"etcd1:2379", "etcd2:2379", "etcd3:2379"},
//	    Namespace: "gibson",
//	    TTL:       30,
//	    TLS: &registry.TLSConfig{
//	        Enabled:  true,
//	        CertFile: "/etc/gibson/certs/client.pem",
//	        KeyFile:  "/etc/gibson/certs/client-key.pem",
//	        CAFile:   "/etc/gibson/certs/ca.pem",
//	    },
//	}
//	reg, err := NewExternalRegistry(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer reg.Close()
type ExternalRegistry struct {
	cfg      registry.Config
	client   *clientv3.Client
	mu       sync.RWMutex
	leases   map[string]clientv3.LeaseID // instance-id -> lease-id
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewExternalRegistry creates a connection to an external etcd cluster.
//
// The function will:
//  1. Validate that endpoints are configured
//  2. Set up TLS configuration if enabled
//  3. Create an etcd client connection with the specified endpoints
//  4. Perform a health check to ensure connectivity
//  5. Return the initialized registry
//
// Returns an error if:
//   - No endpoints are configured
//   - TLS certificate files cannot be read
//   - Cannot connect to any etcd endpoint
//   - Health check fails (fail-fast behavior)
//
// This implementation uses fail-fast behavior: if the etcd cluster is unreachable
// during initialization, an error is returned immediately rather than retrying or
// falling back to embedded mode.
func NewExternalRegistry(cfg registry.Config) (*ExternalRegistry, error) {
	// Validate configuration
	if len(cfg.Endpoints) == 0 {
		return nil, fmt.Errorf("external registry requires at least one endpoint")
	}

	// Set defaults
	if cfg.Namespace == "" {
		cfg.Namespace = "gibson"
	}
	if cfg.TTL == 0 {
		cfg.TTL = 30
	}

	// Build etcd client config
	clientCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: 5 * time.Second,
	}

	// Configure TLS if enabled
	if cfg.TLS != nil && cfg.TLS.Enabled {
		tlsConfig, err := buildTLSConfig(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", err)
		}
		clientCfg.TLS = tlsConfig
	}

	// Create etcd client
	client, err := clientv3.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	// Perform health check (fail-fast)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to connect to etcd by doing a simple status check
	for _, endpoint := range cfg.Endpoints {
		_, err := client.Status(ctx, endpoint)
		if err == nil {
			// At least one endpoint is reachable, we're good
			goto healthCheckPassed
		}
	}

	// If we get here, no endpoints were reachable
	client.Close()
	return nil, fmt.Errorf("health check failed: cannot connect to any etcd endpoint (tried %v)", cfg.Endpoints)

healthCheckPassed:
	reg := &ExternalRegistry{
		cfg:      cfg,
		client:   client,
		leases:   make(map[string]clientv3.LeaseID),
		stopChan: make(chan struct{}),
	}

	return reg, nil
}

// buildTLSConfig creates a TLS configuration from certificate files.
//
// This enables mutual TLS (mTLS) authentication between the Gibson registry client
// and the etcd cluster, ensuring secure and authenticated communication.
func buildTLSConfig(tlsCfg *registry.TLSConfig) (*tls.Config, error) {
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(tlsCfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	// Create cert pool and add CA
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12, // Enforce TLS 1.2+ for security
	}, nil
}

// Register adds a service instance to the registry.
//
// The service is stored at the etcd key:
//
//	/{namespace}/{kind}/{name}/{instance-id}
//
// A lease is created with the configured TTL, and a background goroutine is
// started to renew the lease every TTL/3 seconds. If the lease renewal fails
// (e.g., due to network partition or component crash), the service entry will
// be automatically removed from etcd when the lease expires.
//
// This implementation is identical to EmbeddedRegistry.Register, ensuring
// consistent behavior across registry modes.
func (r *ExternalRegistry) Register(ctx context.Context, info registry.ServiceInfo) error {
	// Serialize ServiceInfo to JSON
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal service info: %w", err)
	}

	// Create lease with TTL
	ttlSeconds := int64(r.cfg.TTL)
	lease, err := r.client.Grant(ctx, ttlSeconds)
	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	// Build etcd key
	key := buildKey(r.cfg.Namespace, info.Kind, info.Name, info.InstanceID)

	// Put service info with lease
	_, err = r.client.Put(ctx, key, string(data), clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("failed to register service: %w", err)
	}

	// Track lease for this instance
	r.mu.Lock()
	r.leases[info.InstanceID] = lease.ID
	r.mu.Unlock()

	// Start keepalive goroutine
	r.wg.Add(1)
	go r.keepAlive(info.InstanceID, lease.ID)

	return nil
}

// Deregister removes a service instance from the registry.
//
// This revokes the associated lease, which causes etcd to immediately delete
// the service entry. The keepalive goroutine for this instance is also stopped.
func (r *ExternalRegistry) Deregister(ctx context.Context, info registry.ServiceInfo) error {
	// Get lease for this instance
	r.mu.Lock()
	leaseID, exists := r.leases[info.InstanceID]
	if !exists {
		r.mu.Unlock()
		// Not registered, nothing to do
		return nil
	}
	delete(r.leases, info.InstanceID)
	r.mu.Unlock()

	// Revoke lease (this deletes the key)
	_, err := r.client.Revoke(ctx, leaseID)
	if err != nil {
		return fmt.Errorf("failed to revoke lease: %w", err)
	}

	return nil
}

// Discover finds all instances of a service by kind and name.
//
// Example:
//
//	agents, err := reg.Discover(ctx, "agent", "k8skiller")
//
// Returns an empty slice if no instances are found.
func (r *ExternalRegistry) Discover(ctx context.Context, kind, name string) ([]registry.ServiceInfo, error) {
	// Build prefix for this kind/name
	prefix := buildPrefix(r.cfg.Namespace, kind, name)

	// Get all keys with this prefix
	resp, err := r.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}

	// Parse results
	var services []registry.ServiceInfo
	for _, kv := range resp.Kvs {
		var info registry.ServiceInfo
		if err := json.Unmarshal(kv.Value, &info); err != nil {
			// Skip invalid entries
			continue
		}
		services = append(services, info)
	}

	return services, nil
}

// DiscoverAll finds all instances of a given kind.
//
// Example:
//
//	allAgents, err := reg.DiscoverAll(ctx, "agent")
//
// Returns an empty slice if no instances are found.
func (r *ExternalRegistry) DiscoverAll(ctx context.Context, kind string) ([]registry.ServiceInfo, error) {
	// Build prefix for this kind
	prefix := fmt.Sprintf("/%s/%s/", r.cfg.Namespace, kind)

	// Get all keys with this prefix
	resp, err := r.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to discover all services: %w", err)
	}

	// Parse results
	var services []registry.ServiceInfo
	for _, kv := range resp.Kvs {
		var info registry.ServiceInfo
		if err := json.Unmarshal(kv.Value, &info); err != nil {
			// Skip invalid entries
			continue
		}
		services = append(services, info)
	}

	return services, nil
}

// Watch returns a channel that receives updates when services change.
//
// The channel emits the current list of instances whenever:
//   - A new instance registers
//   - An existing instance deregisters
//   - An instance's lease expires
//
// The initial state is sent immediately. The channel is closed when the
// context is canceled or Close() is called.
//
// Example:
//
//	ch, err := reg.Watch(ctx, "agent", "davinci")
//	for instances := range ch {
//	    log.Printf("Davinci agents: %d", len(instances))
//	}
func (r *ExternalRegistry) Watch(ctx context.Context, kind, name string) (<-chan []registry.ServiceInfo, error) {
	prefix := buildPrefix(r.cfg.Namespace, kind, name)
	ch := make(chan []registry.ServiceInfo, 1)

	// Send initial state
	initial, err := r.Discover(ctx, kind, name)
	if err != nil {
		close(ch)
		return ch, err
	}
	ch <- initial

	// Start watch goroutine
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer close(ch)

		watchChan := r.client.Watch(ctx, prefix, clientv3.WithPrefix())
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stopChan:
				return
			case wresp, ok := <-watchChan:
				if !ok {
					return
				}
				if wresp.Err() != nil {
					return
				}

				// Fetch current state after any change
				services, err := r.Discover(ctx, kind, name)
				if err != nil {
					return
				}

				select {
				case ch <- services:
				case <-ctx.Done():
					return
				case <-r.stopChan:
					return
				}
			}
		}
	}()

	return ch, nil
}

// Close gracefully shuts down the external registry connection and stops all background goroutines.
//
// After Close() is called, all other methods will fail. All active watches will
// be terminated and their channels closed.
func (r *ExternalRegistry) Close() error {
	// Signal all goroutines to stop
	close(r.stopChan)

	// Wait for all goroutines to finish
	r.wg.Wait()

	// Close etcd client
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			return fmt.Errorf("failed to close etcd client: %w", err)
		}
	}

	return nil
}

// keepAlive maintains the lease for a registered service instance.
//
// This goroutine runs in the background and renews the lease every TTL/3 seconds.
// If renewal fails, the lease will expire and etcd will automatically remove the
// service entry.
func (r *ExternalRegistry) keepAlive(instanceID string, leaseID clientv3.LeaseID) {
	defer r.wg.Done()

	// Create keepalive channel
	keepAliveChan, err := r.client.KeepAlive(context.Background(), leaseID)
	if err != nil {
		// Keepalive failed, lease will expire naturally
		return
	}

	// Consume keepalive responses
	for {
		select {
		case <-r.stopChan:
			return
		case ka, ok := <-keepAliveChan:
			if !ok {
				// Keepalive channel closed, lease expired
				r.mu.Lock()
				delete(r.leases, instanceID)
				r.mu.Unlock()
				return
			}
			if ka == nil {
				// Lease revoked or expired
				r.mu.Lock()
				delete(r.leases, instanceID)
				r.mu.Unlock()
				return
			}
		}
	}
}
