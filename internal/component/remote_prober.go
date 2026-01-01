package component

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// RemoteProber provides health checking and discovery for remote components (agents, tools, plugins)
// running on different hosts, containers, or Kubernetes pods. Remote components are configured via
// YAML and probed using gRPC or HTTP health check protocols.
//
// Remote components do not use PID files, lock files, or Unix sockets - instead, they are tracked
// purely via configuration and active health probes.
type RemoteProber interface {
	// Probe performs a health check on a single remote component at the specified address.
	// Returns the current state including health status, or an error if the probe fails.
	//
	// The probe uses the configured health check protocol (gRPC or HTTP) and respects the
	// timeout specified in the prober configuration (default 5s).
	//
	// Example:
	//   state, err := prober.Probe(ctx, "k8skiller-container:50051")
	//   if err != nil {
	//       log.Printf("probe failed: %v", err)
	//   }
	//   if state.Healthy {
	//       log.Printf("component %s is healthy", state.Name)
	//   }
	Probe(ctx context.Context, address string) (RemoteComponentState, error)

	// ProbeAll performs health checks on all configured remote components in parallel.
	// Returns a slice of component states, including both healthy and unhealthy components.
	//
	// ProbeAll does not return an error - individual probe failures are reflected in the
	// RemoteComponentState.Healthy field and Error message. This allows partial success
	// where some components are available and others are not.
	//
	// The implementation should use goroutines and context for parallel execution with a
	// reasonable timeout (e.g., 500ms total for 20 components).
	//
	// Example:
	//   states, err := prober.ProbeAll(ctx)
	//   for _, state := range states {
	//       if state.Healthy {
	//           fmt.Printf("%s: healthy\n", state.Name)
	//       } else {
	//           fmt.Printf("%s: unhealthy - %s\n", state.Name, state.Error)
	//       }
	//   }
	ProbeAll(ctx context.Context) ([]RemoteComponentState, error)

	// LoadConfig loads remote component configurations from the provided config maps.
	// This method must be called before ProbeAll to populate the prober with component
	// addresses and health check settings.
	//
	// The config maps are keyed by component name and contain address, health check protocol,
	// and optional TLS configuration.
	//
	// Example:
	//   err := prober.LoadConfig(
	//       map[string]RemoteComponentConfig{
	//           "k8skiller": {
	//               Address: "k8skiller-container:50051",
	//               HealthCheck: "grpc",
	//           },
	//       },
	//       map[string]RemoteComponentConfig{},
	//       map[string]RemoteComponentConfig{},
	//   )
	LoadConfig(agents, tools, plugins map[string]RemoteComponentConfig) error
}

// RemoteComponentState represents the runtime state of a remote component after a health probe.
// This state is ephemeral and derived from live health checks, never persisted to the database.
type RemoteComponentState struct {
	// Kind is the component type (agent, tool, or plugin)
	Kind ComponentKind `json:"kind" yaml:"kind"`

	// Name is the component name as configured in the YAML
	Name string `json:"name" yaml:"name"`

	// Address is the network address in host:port format (e.g., "k8skiller-container:50051")
	Address string `json:"address" yaml:"address"`

	// Healthy indicates whether the component passed its health check
	Healthy bool `json:"healthy" yaml:"healthy"`

	// LastCheck is the timestamp of the most recent health probe
	LastCheck time.Time `json:"last_check" yaml:"last_check"`

	// Error contains the error message if the health check failed, empty if Healthy is true
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// ResponseTime is the duration of the health check request (for monitoring)
	ResponseTime time.Duration `json:"response_time" yaml:"response_time"`
}

// RemoteComponentConfig defines the configuration for a single remote component.
// This struct maps directly to the YAML schema for remote_agents, remote_tools, and remote_plugins.
//
// Example YAML:
//
//	remote_agents:
//	  k8skiller:
//	    address: "k8skiller-container:50051"
//	    health_check: grpc
//	    tls:
//	      enabled: true
//	      cert_file: /path/to/cert.pem
//	      key_file: /path/to/key.pem
//	      ca_file: /path/to/ca.pem
type RemoteComponentConfig struct {
	// Address is the network address in host:port format
	Address string `mapstructure:"address" yaml:"address" validate:"required"`

	// HealthCheck is the protocol for health checks: "grpc" or "http"
	// Default: "grpc"
	HealthCheck string `mapstructure:"health_check" yaml:"health_check" validate:"oneof=grpc http"`

	// TLS contains optional TLS configuration for secure connections
	TLS *TLSConfig `mapstructure:"tls" yaml:"tls,omitempty"`

	// Timeout is the health check timeout duration (default: 5s)
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout,omitempty"`
}

// TLSConfig defines TLS/mTLS configuration for secure remote component connections.
// This supports both server authentication (TLS) and mutual authentication (mTLS).
type TLSConfig struct {
	// Enabled controls whether TLS is used for the connection
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// CertFile is the path to the client certificate (for mTLS)
	CertFile string `mapstructure:"cert_file" yaml:"cert_file,omitempty"`

	// KeyFile is the path to the client private key (for mTLS)
	KeyFile string `mapstructure:"key_file" yaml:"key_file,omitempty"`

	// CAFile is the path to the CA certificate for server verification
	CAFile string `mapstructure:"ca_file" yaml:"ca_file,omitempty"`

	// ServerName is the expected server name for certificate verification
	// If empty, the hostname from the address will be used
	ServerName string `mapstructure:"server_name" yaml:"server_name,omitempty"`

	// InsecureSkipVerify disables server certificate verification (not recommended for production)
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify" yaml:"insecure_skip_verify,omitempty"`
}

// DefaultRemoteProber is the default implementation of RemoteProber.
// It maintains a registry of remote components loaded from configuration and performs
// health checks using gRPC or HTTP protocols.
type DefaultRemoteProber struct {
	// components stores all remote components by kind and name
	components map[ComponentKind]map[string]RemoteComponentConfig

	// mu protects concurrent access to the components map
	mu sync.RWMutex
}

// NewDefaultRemoteProber creates a new DefaultRemoteProber instance.
func NewDefaultRemoteProber() *DefaultRemoteProber {
	return &DefaultRemoteProber{
		components: make(map[ComponentKind]map[string]RemoteComponentConfig),
	}
}

// LoadConfig loads remote component configurations from the provided config maps.
// This replaces any previously loaded configuration.
func (d *DefaultRemoteProber) LoadConfig(agents, tools, plugins map[string]RemoteComponentConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Initialize the components map
	d.components = make(map[ComponentKind]map[string]RemoteComponentConfig)
	d.components[ComponentKindAgent] = make(map[string]RemoteComponentConfig)
	d.components[ComponentKindTool] = make(map[string]RemoteComponentConfig)
	d.components[ComponentKindPlugin] = make(map[string]RemoteComponentConfig)

	// Load agents
	for name, cfg := range agents {
		if err := d.validateConfig(name, cfg); err != nil {
			return WrapComponentError(ErrCodeValidationFailed,
				fmt.Sprintf("invalid agent config for %s", name), err)
		}
		d.components[ComponentKindAgent][name] = d.normalizeConfig(cfg)
	}

	// Load tools
	for name, cfg := range tools {
		if err := d.validateConfig(name, cfg); err != nil {
			return WrapComponentError(ErrCodeValidationFailed,
				fmt.Sprintf("invalid tool config for %s", name), err)
		}
		d.components[ComponentKindTool][name] = d.normalizeConfig(cfg)
	}

	// Load plugins
	for name, cfg := range plugins {
		if err := d.validateConfig(name, cfg); err != nil {
			return WrapComponentError(ErrCodeValidationFailed,
				fmt.Sprintf("invalid plugin config for %s", name), err)
		}
		d.components[ComponentKindPlugin][name] = d.normalizeConfig(cfg)
	}

	return nil
}

// validateConfig validates a remote component configuration.
func (d *DefaultRemoteProber) validateConfig(name string, cfg RemoteComponentConfig) error {
	if cfg.Address == "" {
		return fmt.Errorf("address is required for remote component %s", name)
	}

	// Validate health check protocol
	if cfg.HealthCheck != "" && cfg.HealthCheck != "grpc" && cfg.HealthCheck != "http" {
		return fmt.Errorf("invalid health check protocol %s for %s (must be 'grpc' or 'http')", cfg.HealthCheck, name)
	}

	// Validate TLS config if present
	if cfg.TLS != nil && cfg.TLS.Enabled {
		// For mTLS, both cert and key must be present
		if cfg.TLS.CertFile != "" || cfg.TLS.KeyFile != "" {
			if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
				return fmt.Errorf("both cert_file and key_file are required for mTLS on %s", name)
			}
		}
	}

	return nil
}

// normalizeConfig normalizes a remote component configuration with default values.
func (d *DefaultRemoteProber) normalizeConfig(cfg RemoteComponentConfig) RemoteComponentConfig {
	// Set default health check protocol
	if cfg.HealthCheck == "" {
		cfg.HealthCheck = "grpc"
	}

	// Set default timeout
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultHealthCheckTimeout
	}

	return cfg
}

// Probe performs a health check on a single remote component at the specified address.
// This is a lower-level method that requires the caller to know the address.
func (d *DefaultRemoteProber) Probe(ctx context.Context, address string) (RemoteComponentState, error) {
	startTime := time.Now()

	// Default config for ad-hoc probes
	cfg := RemoteComponentConfig{
		Address:     address,
		HealthCheck: "grpc",
		Timeout:     DefaultHealthCheckTimeout,
	}

	state := RemoteComponentState{
		Name:      address, // Use address as name for ad-hoc probes
		Address:   address,
		Healthy:   false,
		LastCheck: startTime,
	}

	// Perform the health check
	err := d.probeWithConfig(ctx, cfg)
	state.ResponseTime = time.Since(startTime)

	if err != nil {
		state.Healthy = false
		state.Error = err.Error()
		return state, err
	}

	state.Healthy = true
	return state, nil
}

// ProbeAll performs health checks on all configured remote components in parallel.
// Returns a slice of component states, including both healthy and unhealthy components.
// Individual probe failures are reflected in the RemoteComponentState.Healthy field.
func (d *DefaultRemoteProber) ProbeAll(ctx context.Context) ([]RemoteComponentState, error) {
	d.mu.RLock()

	// Count total components
	totalComponents := 0
	for _, kindMap := range d.components {
		totalComponents += len(kindMap)
	}

	// If no components configured, return empty slice
	if totalComponents == 0 {
		d.mu.RUnlock()
		return []RemoteComponentState{}, nil
	}

	// Create context with timeout for the entire operation
	// Target: 500ms for 20 components = 25ms per component average
	// We'll use a more generous timeout to account for parallel execution
	totalTimeout := 30 * time.Second // Conservative timeout
	probeCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	// Use errgroup for parallel execution
	g, gCtx := errgroup.WithContext(probeCtx)

	// Channel to collect results
	resultsCh := make(chan RemoteComponentState, totalComponents)

	// Launch a goroutine for each component
	for kind, kindMap := range d.components {
		for name, cfg := range kindMap {
			// Capture loop variables
			kind := kind
			name := name
			cfg := cfg

			g.Go(func() error {
				startTime := time.Now()

				state := RemoteComponentState{
					Kind:      kind,
					Name:      name,
					Address:   cfg.Address,
					Healthy:   false,
					LastCheck: startTime,
				}

				// Create per-probe timeout context
				probeTimeout := cfg.Timeout
				if probeTimeout == 0 {
					probeTimeout = DefaultHealthCheckTimeout
				}

				probeCtx, cancel := context.WithTimeout(gCtx, probeTimeout)
				defer cancel()

				// Perform the health check
				err := d.probeWithConfig(probeCtx, cfg)
				state.ResponseTime = time.Since(startTime)

				if err != nil {
					state.Healthy = false
					state.Error = err.Error()
				} else {
					state.Healthy = true
				}

				// Send result to channel
				select {
				case resultsCh <- state:
				case <-gCtx.Done():
					// Context cancelled, don't block
				}

				// Always return nil - we want to collect all results, not fail fast
				return nil
			})
		}
	}

	d.mu.RUnlock()

	// Wait for all probes to complete
	_ = g.Wait() // Ignore error since we return nil from goroutines

	// Close channel and collect results
	close(resultsCh)

	results := make([]RemoteComponentState, 0, totalComponents)
	for state := range resultsCh {
		results = append(results, state)
	}

	return results, nil
}

// probeWithConfig performs a health check using the specified configuration.
func (d *DefaultRemoteProber) probeWithConfig(ctx context.Context, cfg RemoteComponentConfig) error {
	switch cfg.HealthCheck {
	case "grpc":
		return d.probeGRPC(ctx, cfg)
	case "http":
		return d.probeHTTP(ctx, cfg)
	default:
		return NewInvalidProtocolError(cfg.HealthCheck)
	}
}

// probeGRPC performs a gRPC health check.
func (d *DefaultRemoteProber) probeGRPC(ctx context.Context, cfg RemoteComponentConfig) error {
	// Build gRPC dial options
	opts := []grpc.DialOption{}

	// Configure TLS if enabled
	if cfg.TLS != nil && cfg.TLS.Enabled {
		creds, err := d.buildTLSCredentials(cfg.TLS)
		if err != nil {
			return WrapComponentError(ErrCodeConnectionFailed, "failed to build TLS credentials", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		// Use insecure credentials if TLS is not enabled
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Create gRPC connection
	conn, err := grpc.NewClient(cfg.Address, opts...)
	if err != nil {
		return WrapComponentError(ErrCodeConnectionFailed, "failed to create gRPC connection", err)
	}
	defer conn.Close()

	// Create health check client
	client := grpc_health_v1.NewHealthClient(conn)

	// Perform health check
	req := &grpc_health_v1.HealthCheckRequest{
		Service: "", // Empty service name checks overall server health
	}

	resp, err := client.Check(ctx, req)
	if err != nil {
		return WrapComponentError(ErrCodeHealthCheckFailed, "gRPC health check failed", err)
	}

	// Check status
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return NewHealthCheckError("", "grpc",
			fmt.Errorf("service not serving (status: %v)", resp.Status))
	}

	return nil
}

// probeHTTP performs an HTTP health check.
func (d *DefaultRemoteProber) probeHTTP(ctx context.Context, cfg RemoteComponentConfig) error {
	// Build HTTP endpoint
	// The address might be just "host:port", so we need to construct a full URL
	endpoint := cfg.Address
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		// Default to http unless TLS is configured
		if cfg.TLS != nil && cfg.TLS.Enabled {
			endpoint = "https://" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
	}

	// Add default health path if not included
	if !strings.Contains(endpoint, "/") || strings.HasSuffix(endpoint, "/") {
		endpoint += "/health"
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	// TODO: Support TLS configuration for HTTP clients in the future
	// For now, HTTP health checks use default transport

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return WrapComponentError(ErrCodeHealthCheckFailed, "failed to create HTTP request", err)
	}

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return WrapComponentError(ErrCodeHealthCheckFailed, "HTTP health check failed", err)
	}
	defer resp.Body.Close()

	// Check status code (2xx is healthy)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewHealthCheckError("", "http",
			fmt.Errorf("unhealthy status code: %d", resp.StatusCode))
	}

	return nil
}

// buildTLSCredentials builds gRPC TLS credentials from the TLS configuration.
func (d *DefaultRemoteProber) buildTLSCredentials(tlsCfg *TLSConfig) (credentials.TransportCredentials, error) {
	config := &tls.Config{
		ServerName:         tlsCfg.ServerName,
		InsecureSkipVerify: tlsCfg.InsecureSkipVerify,
	}

	// Load client certificate and key for mTLS
	if tlsCfg.CertFile != "" && tlsCfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate for server verification
	if tlsCfg.CAFile != "" {
		caData, err := os.ReadFile(tlsCfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("failed to add CA certificate to pool")
		}
		config.RootCAs = certPool
	}

	return credentials.NewTLS(config), nil
}
