package component

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// HealthChecker performs health checks against a component.
// This interface abstracts the protocol used for health checking,
// allowing both HTTP and gRPC health checks to be used interchangeably.
type HealthChecker interface {
	// Check performs a health check and returns nil if healthy.
	// Returns an error if the component is unhealthy or unreachable.
	Check(ctx context.Context) error

	// Protocol returns the protocol used by this checker.
	Protocol() HealthCheckProtocol

	// Close releases any resources held by the checker.
	Close() error
}

// HTTPHealthChecker performs HTTP-based health checks.
// It makes GET requests to a health endpoint and checks for 2xx status codes.
type HTTPHealthChecker struct {
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

// HTTPHealthCheckerOption configures an HTTPHealthChecker.
type HTTPHealthCheckerOption func(*HTTPHealthChecker)

// WithHTTPClient sets a custom HTTP client for the health checker.
func WithHTTPClient(client *http.Client) HTTPHealthCheckerOption {
	return func(h *HTTPHealthChecker) {
		h.client = client
	}
}

// WithHTTPTimeout sets the timeout for HTTP health checks.
func WithHTTPTimeout(timeout time.Duration) HTTPHealthCheckerOption {
	return func(h *HTTPHealthChecker) {
		h.timeout = timeout
		if h.client != nil {
			h.client.Timeout = timeout
		}
	}
}

// NewHTTPHealthChecker creates a new HTTP health checker.
// The endpoint should be a full URL (e.g., "http://localhost:8080/health").
func NewHTTPHealthChecker(endpoint string, opts ...HTTPHealthCheckerOption) *HTTPHealthChecker {
	h := &HTTPHealthChecker{
		endpoint: endpoint,
		timeout:  DefaultHealthCheckTimeout,
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	// Create default client if not provided
	if h.client == nil {
		h.client = &http.Client{
			Timeout: h.timeout,
		}
	}

	return h
}

// Check performs an HTTP health check.
// Returns nil if the endpoint returns a 2xx status code.
func (h *HTTPHealthChecker) Check(ctx context.Context) error {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint, nil)
	if err != nil {
		return NewHealthCheckError("", "http", fmt.Errorf("failed to create request: %w", err))
	}

	// Perform request
	resp, err := h.client.Do(req)
	if err != nil {
		return NewHealthCheckError("", "http", fmt.Errorf("request failed: %w", err))
	}
	defer resp.Body.Close()

	// Check status code (2xx is healthy)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return NewHealthCheckError("", "http", fmt.Errorf("unhealthy status code: %d", resp.StatusCode))
	}

	return nil
}

// Protocol returns the health check protocol (HTTP).
func (h *HTTPHealthChecker) Protocol() HealthCheckProtocol {
	return HealthCheckProtocolHTTP
}

// Close releases resources. For HTTP checker, this is a no-op.
func (h *HTTPHealthChecker) Close() error {
	// HTTP clients don't need explicit cleanup
	return nil
}

// Endpoint returns the health check endpoint URL.
func (h *HTTPHealthChecker) Endpoint() string {
	return h.endpoint
}

// GRPCHealthChecker performs gRPC-based health checks using the standard
// grpc_health_v1 protocol defined in https://github.com/grpc/grpc/blob/master/doc/health-checking.md
type GRPCHealthChecker struct {
	address     string
	conn        *grpc.ClientConn
	client      grpc_health_v1.HealthClient
	timeout     time.Duration
	serviceName string // Empty string checks overall server health
}

// GRPCHealthCheckerOption configures a GRPCHealthChecker.
type GRPCHealthCheckerOption func(*GRPCHealthChecker)

// WithGRPCTimeout sets the timeout for gRPC health checks.
func WithGRPCTimeout(timeout time.Duration) GRPCHealthCheckerOption {
	return func(g *GRPCHealthChecker) {
		g.timeout = timeout
	}
}

// WithServiceName sets the gRPC service name to check.
// Empty string (default) checks overall server health.
func WithServiceName(name string) GRPCHealthCheckerOption {
	return func(g *GRPCHealthChecker) {
		g.serviceName = name
	}
}

// NewGRPCHealthChecker creates a new gRPC health checker.
// The address should be in the format "host:port" (e.g., "localhost:50051").
func NewGRPCHealthChecker(address string, opts ...GRPCHealthCheckerOption) (*GRPCHealthChecker, error) {
	g := &GRPCHealthChecker{
		address:     address,
		timeout:     DefaultHealthCheckTimeout,
		serviceName: "", // Empty means check overall server health
	}

	// Apply options
	for _, opt := range opts {
		opt(g)
	}

	// Establish gRPC connection
	// Use insecure credentials by default (TLS can be added via options later)
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, NewHealthCheckError("", "grpc", fmt.Errorf("failed to create gRPC connection: %w", err))
	}

	g.conn = conn
	g.client = grpc_health_v1.NewHealthClient(conn)

	return g, nil
}

// Check performs a gRPC health check using the grpc_health_v1 protocol.
// Returns nil if the service status is SERVING.
func (g *GRPCHealthChecker) Check(ctx context.Context) error {
	// Create timeout context if not already set
	checkCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// Make the health check request
	req := &grpc_health_v1.HealthCheckRequest{
		Service: g.serviceName,
	}

	resp, err := g.client.Check(checkCtx, req)
	if err != nil {
		return NewHealthCheckError("", "grpc", fmt.Errorf("health check request failed: %w", err))
	}

	// Check the status
	switch resp.Status {
	case grpc_health_v1.HealthCheckResponse_SERVING:
		return nil
	case grpc_health_v1.HealthCheckResponse_NOT_SERVING:
		return NewHealthCheckError("", "grpc", fmt.Errorf("service not serving"))
	case grpc_health_v1.HealthCheckResponse_UNKNOWN:
		return NewHealthCheckError("", "grpc", fmt.Errorf("service health unknown"))
	case grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN:
		return NewHealthCheckError("", "grpc", fmt.Errorf("service '%s' not found", g.serviceName))
	default:
		return NewHealthCheckError("", "grpc", fmt.Errorf("unexpected health status: %v", resp.Status))
	}
}

// Protocol returns the health check protocol (gRPC).
func (g *GRPCHealthChecker) Protocol() HealthCheckProtocol {
	return HealthCheckProtocolGRPC
}

// Close releases the gRPC connection.
func (g *GRPCHealthChecker) Close() error {
	if g.conn != nil {
		return g.conn.Close()
	}
	return nil
}

// Address returns the gRPC server address.
func (g *GRPCHealthChecker) Address() string {
	return g.address
}

// ServiceName returns the gRPC service name being checked.
func (g *GRPCHealthChecker) ServiceName() string {
	return g.serviceName
}

// ProtocolAwareHealthChecker wraps health checkers with automatic protocol detection.
// When configured with "auto" protocol, it tries gRPC first and falls back to HTTP
// on connection errors (not timeouts or service unavailable).
type ProtocolAwareHealthChecker struct {
	address           string
	port              int
	preferredProtocol HealthCheckProtocol
	httpEndpoint      string
	grpcServiceName   string
	timeout           time.Duration

	mu               sync.RWMutex
	detectedProtocol HealthCheckProtocol
	activeChecker    HealthChecker
	detected         bool
}

// NewProtocolAwareHealthChecker creates a new protocol-aware health checker.
// The address should be "host:port" format for gRPC or will be used to construct HTTP URLs.
func NewProtocolAwareHealthChecker(address string, port int, config *HealthCheckConfig) *ProtocolAwareHealthChecker {
	if config == nil {
		config = &HealthCheckConfig{}
	}

	return &ProtocolAwareHealthChecker{
		address:           address,
		port:              port,
		preferredProtocol: config.GetProtocol(),
		httpEndpoint:      config.GetEndpoint(),
		grpcServiceName:   config.GetServiceName(),
		timeout:           config.GetTimeout(),
		detectedProtocol:  HealthCheckProtocolAuto,
		detected:          false,
	}
}

// Check performs a health check, auto-detecting the protocol if needed.
func (p *ProtocolAwareHealthChecker) Check(ctx context.Context) error {
	p.mu.RLock()
	checker := p.activeChecker
	detected := p.detected
	p.mu.RUnlock()

	// If we already have a working checker, use it
	if detected && checker != nil {
		return checker.Check(ctx)
	}

	// Need to detect or create the checker
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if p.detected && p.activeChecker != nil {
		return p.activeChecker.Check(ctx)
	}

	// Determine which protocol(s) to try
	switch p.preferredProtocol {
	case HealthCheckProtocolGRPC:
		return p.tryGRPC(ctx)
	case HealthCheckProtocolHTTP:
		return p.tryHTTP(ctx)
	case HealthCheckProtocolAuto, "":
		return p.tryAutoDetect(ctx)
	default:
		return NewInvalidProtocolError(string(p.preferredProtocol))
	}
}

// tryGRPC attempts to check health using gRPC.
func (p *ProtocolAwareHealthChecker) tryGRPC(ctx context.Context) error {
	// Create gRPC checker if not exists
	if p.activeChecker == nil || p.activeChecker.Protocol() != HealthCheckProtocolGRPC {
		grpcAddr := fmt.Sprintf("%s:%d", p.address, p.port)
		checker, err := NewGRPCHealthChecker(
			grpcAddr,
			WithGRPCTimeout(p.timeout),
			WithServiceName(p.grpcServiceName),
		)
		if err != nil {
			return err
		}
		// Close old checker if exists
		if p.activeChecker != nil {
			p.activeChecker.Close()
		}
		p.activeChecker = checker
	}

	err := p.activeChecker.Check(ctx)
	if err == nil {
		p.detectedProtocol = HealthCheckProtocolGRPC
		p.detected = true
	}
	return err
}

// tryHTTP attempts to check health using HTTP.
func (p *ProtocolAwareHealthChecker) tryHTTP(ctx context.Context) error {
	// Create HTTP checker if not exists
	if p.activeChecker == nil || p.activeChecker.Protocol() != HealthCheckProtocolHTTP {
		endpoint := fmt.Sprintf("http://%s:%d%s", p.address, p.port, p.httpEndpoint)
		checker := NewHTTPHealthChecker(endpoint, WithHTTPTimeout(p.timeout))
		// Close old checker if exists
		if p.activeChecker != nil {
			p.activeChecker.Close()
		}
		p.activeChecker = checker
	}

	err := p.activeChecker.Check(ctx)
	if err == nil {
		p.detectedProtocol = HealthCheckProtocolHTTP
		p.detected = true
	}
	return err
}

// tryAutoDetect tries gRPC first, then falls back to HTTP on connection errors.
func (p *ProtocolAwareHealthChecker) tryAutoDetect(ctx context.Context) error {
	// Try gRPC first
	grpcErr := p.tryGRPC(ctx)
	if grpcErr == nil {
		return nil
	}

	// Check if it's a connection error (worth trying HTTP) vs service error (don't retry)
	if !isConnectionError(grpcErr) {
		// Not a connection error - the service responded but is unhealthy
		// Don't fall back to HTTP
		return grpcErr
	}

	// Try HTTP as fallback
	httpErr := p.tryHTTP(ctx)
	if httpErr == nil {
		return nil
	}

	// Both failed - return a combined error
	return NewProtocolDetectError("", fmt.Errorf("gRPC: %v; HTTP: %v", grpcErr, httpErr))
}

// isConnectionError checks if the error indicates a connection failure
// (as opposed to the service being unhealthy or unavailable).
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Connection refused, timeout during dial, DNS errors, etc.
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "failed to create gRPC connection")
}

// Protocol returns the detected protocol, or "auto" if not yet detected.
func (p *ProtocolAwareHealthChecker) Protocol() HealthCheckProtocol {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.detected {
		return p.detectedProtocol
	}
	return HealthCheckProtocolAuto
}

// DetectedProtocol returns the protocol that was detected during health checks.
// Returns empty string if detection hasn't occurred yet.
func (p *ProtocolAwareHealthChecker) DetectedProtocol() HealthCheckProtocol {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.detected {
		return p.detectedProtocol
	}
	return ""
}

// Close releases resources held by the active checker.
func (p *ProtocolAwareHealthChecker) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.activeChecker != nil {
		err := p.activeChecker.Close()
		p.activeChecker = nil
		return err
	}
	return nil
}

// Address returns the target address.
func (p *ProtocolAwareHealthChecker) Address() string {
	return p.address
}

// Port returns the target port.
func (p *ProtocolAwareHealthChecker) Port() int {
	return p.port
}
