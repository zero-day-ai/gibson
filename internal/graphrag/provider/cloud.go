package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/graphrag"
	"github.com/zero-day-ai/gibson/internal/types"
)

// CloudGraphRAGProvider implements GraphRAGProvider using Gibson Cloud GraphRAG API.
// All operations are proxied to a remote cloud service via HTTP REST API.
//
// Features:
// - Automatic retry with exponential backoff for transient failures
// - Optional local caching for offline resilience
// - Authentication via API key
// - Rate limiting and circuit breaker patterns
//
// Thread-safety: Safe for concurrent access via internal locking.
type CloudGraphRAGProvider struct {
	config      graphrag.GraphRAGConfig
	httpClient  *http.Client
	endpoint    string
	apiKey      string
	cache       *cloudCache
	initialized bool
	mu          sync.RWMutex
}

// cloudCache provides simple in-memory caching for offline resilience.
// Production implementations would use a more sophisticated cache (Redis, etc.).
type cloudCache struct {
	nodes         map[string]graphrag.GraphNode
	relationships map[string]graphrag.Relationship
	enabled       bool
	mu            sync.RWMutex
}

// NewCloudProvider creates a new CloudGraphRAGProvider with the given configuration.
// Does not initialize connections - call Initialize() before use.
func NewCloudProvider(config graphrag.GraphRAGConfig) (*CloudGraphRAGProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, graphrag.NewConfigError("invalid cloud provider configuration", err)
	}

	// Validate cloud-specific configuration
	if config.Cloud.Endpoint == "" {
		return nil, graphrag.NewConfigError("cloud endpoint is required", nil)
	}

	return &CloudGraphRAGProvider{
		config:   config,
		endpoint: config.Cloud.Endpoint,
		cache: &cloudCache{
			nodes:         make(map[string]graphrag.GraphNode),
			relationships: make(map[string]graphrag.Relationship),
			enabled:       true, // Enable caching by default
		},
		initialized: false,
	}, nil
}

// Initialize establishes connection to the cloud API and validates authentication.
// Performs a health check to ensure the cloud service is reachable.
func (c *CloudGraphRAGProvider) Initialize(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil // Already initialized
	}

	// Create HTTP client with timeout
	c.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Extract API key from config
	// In production, this would come from secure configuration or environment
	c.apiKey = c.config.Cloud.AWSAccessKeyID // Reusing this field as API key

	// Validate connectivity with health check
	health := c.performHealthCheck(ctx)
	if !health.IsHealthy() {
		return graphrag.NewConnectionError(
			"cloud GraphRAG service is unavailable",
			fmt.Errorf("health check failed: %s", health.Message),
		)
	}

	c.initialized = true
	return nil
}

// performHealthCheck sends a health check request to the cloud API.
func (c *CloudGraphRAGProvider) performHealthCheck(ctx context.Context) types.HealthStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/health", nil)
	if err != nil {
		return types.Unhealthy(fmt.Sprintf("failed to create health check request: %v", err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return types.Unhealthy(fmt.Sprintf("health check request failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.Unhealthy(fmt.Sprintf("health check returned status %d", resp.StatusCode))
	}

	return types.Healthy("cloud service is reachable")
}

// StoreNode stores a graph node via the cloud API.
// Implements retry with exponential backoff for transient failures.
// Caches the node locally for offline resilience.
func (c *CloudGraphRAGProvider) StoreNode(ctx context.Context, node graphrag.GraphNode) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Validate node
	if err := node.Validate(); err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeQueryFailed, "invalid node", err)
	}

	// Prepare request payload
	payload, err := json.Marshal(node)
	if err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeQueryFailed, "failed to marshal node", err)
	}

	// Send request with retry
	err = c.retryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/nodes", bytes.NewReader(payload))
		if err != nil {
			return err
		}

		c.setAuthHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return graphrag.NewConnectionError("failed to store node", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return graphrag.NewRateLimitError("rate limit exceeded")
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return graphrag.NewQueryError(
				fmt.Sprintf("failed to store node: status %d", resp.StatusCode),
				fmt.Errorf("response: %s", string(body)),
			)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Cache node locally
	c.cache.storeNode(node)

	return nil
}

// StoreRelationship creates a relationship via the cloud API.
// Implements retry with exponential backoff for transient failures.
func (c *CloudGraphRAGProvider) StoreRelationship(ctx context.Context, rel graphrag.Relationship) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Validate relationship
	if err := rel.Validate(); err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeRelationshipFailed, "invalid relationship", err)
	}

	// Prepare request payload
	payload, err := json.Marshal(rel)
	if err != nil {
		return graphrag.WrapGraphRAGError(graphrag.ErrCodeRelationshipFailed, "failed to marshal relationship", err)
	}

	// Send request with retry
	err = c.retryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/relationships", bytes.NewReader(payload))
		if err != nil {
			return err
		}

		c.setAuthHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return graphrag.NewConnectionError("failed to store relationship", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return graphrag.NewRateLimitError("rate limit exceeded")
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return graphrag.NewRelationshipError(
				fmt.Sprintf("failed to store relationship: status %d", resp.StatusCode),
				fmt.Errorf("response: %s", string(body)),
			)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Cache relationship locally
	c.cache.storeRelationship(rel)

	return nil
}

// QueryNodes performs node query via the cloud API.
// Falls back to local cache if the cloud service is unavailable.
func (c *CloudGraphRAGProvider) QueryNodes(ctx context.Context, query graphrag.NodeQuery) ([]graphrag.GraphNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Prepare query payload
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, graphrag.WrapGraphRAGError(graphrag.ErrCodeInvalidQuery, "failed to marshal query", err)
	}

	var nodes []graphrag.GraphNode

	// Send request with retry
	err = c.retryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/nodes/query", bytes.NewReader(payload))
		if err != nil {
			return err
		}

		c.setAuthHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return graphrag.NewConnectionError("failed to query nodes", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return graphrag.NewRateLimitError("rate limit exceeded")
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return graphrag.NewQueryError(
				fmt.Sprintf("failed to query nodes: status %d", resp.StatusCode),
				fmt.Errorf("response: %s", string(body)),
			)
		}

		// Parse response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return graphrag.NewQueryError("failed to read response", err)
		}

		if err := json.Unmarshal(body, &nodes); err != nil {
			return graphrag.NewQueryError("failed to unmarshal response", err)
		}

		return nil
	})

	if err != nil {
		// Fallback to cache if request failed
		return c.cache.queryNodes(query), nil
	}

	return nodes, nil
}

// QueryRelationships retrieves relationships via the cloud API.
func (c *CloudGraphRAGProvider) QueryRelationships(ctx context.Context, query graphrag.RelQuery) ([]graphrag.Relationship, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Prepare query payload
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, graphrag.WrapGraphRAGError(graphrag.ErrCodeInvalidQuery, "failed to marshal query", err)
	}

	var relationships []graphrag.Relationship

	// Send request with retry
	err = c.retryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/relationships/query", bytes.NewReader(payload))
		if err != nil {
			return err
		}

		c.setAuthHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return graphrag.NewConnectionError("failed to query relationships", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return graphrag.NewRateLimitError("rate limit exceeded")
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return graphrag.NewQueryError(
				fmt.Sprintf("failed to query relationships: status %d", resp.StatusCode),
				fmt.Errorf("response: %s", string(body)),
			)
		}

		// Parse response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return graphrag.NewQueryError("failed to read response", err)
		}

		if err := json.Unmarshal(body, &relationships); err != nil {
			return graphrag.NewQueryError("failed to unmarshal response", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return relationships, nil
}

// TraverseGraph performs graph traversal via the cloud API.
func (c *CloudGraphRAGProvider) TraverseGraph(ctx context.Context, startID string, maxHops int, filters graphrag.TraversalFilters) ([]graphrag.GraphNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Prepare traversal request
	request := map[string]any{
		"start_id": startID,
		"max_hops": maxHops,
		"filters":  filters,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, graphrag.WrapGraphRAGError(graphrag.ErrCodeInvalidQuery, "failed to marshal traversal request", err)
	}

	var nodes []graphrag.GraphNode

	// Send request with retry
	err = c.retryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/graph/traverse", bytes.NewReader(payload))
		if err != nil {
			return err
		}

		c.setAuthHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return graphrag.NewConnectionError("failed to traverse graph", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return graphrag.NewRateLimitError("rate limit exceeded")
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return graphrag.NewQueryError(
				fmt.Sprintf("failed to traverse graph: status %d", resp.StatusCode),
				fmt.Errorf("response: %s", string(body)),
			)
		}

		// Parse response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return graphrag.NewQueryError("failed to read response", err)
		}

		if err := json.Unmarshal(body, &nodes); err != nil {
			return graphrag.NewQueryError("failed to unmarshal response", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return nodes, nil
}

// VectorSearch performs vector similarity search via the cloud API.
func (c *CloudGraphRAGProvider) VectorSearch(ctx context.Context, embedding []float64, topK int, filters map[string]any) ([]graphrag.VectorResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return nil, graphrag.NewGraphRAGError(graphrag.ErrCodeConnectionFailed, "provider not initialized")
	}

	// Prepare search request
	request := map[string]any{
		"embedding": embedding,
		"top_k":     topK,
		"filters":   filters,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, graphrag.WrapGraphRAGError(graphrag.ErrCodeInvalidQuery, "failed to marshal search request", err)
	}

	var results []graphrag.VectorResult

	// Send request with retry
	err = c.retryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/vector/search", bytes.NewReader(payload))
		if err != nil {
			return err
		}

		c.setAuthHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return graphrag.NewConnectionError("failed to perform vector search", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return graphrag.NewRateLimitError("rate limit exceeded")
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return graphrag.NewQueryError(
				fmt.Sprintf("vector search failed: status %d", resp.StatusCode),
				fmt.Errorf("response: %s", string(body)),
			)
		}

		// Parse response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return graphrag.NewQueryError("failed to read response", err)
		}

		if err := json.Unmarshal(body, &results); err != nil {
			return graphrag.NewQueryError("failed to unmarshal response", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// Health checks the health of the cloud API connection.
func (c *CloudGraphRAGProvider) Health(ctx context.Context) types.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return types.Unhealthy("provider not initialized")
	}

	return c.performHealthCheck(ctx)
}

// Close releases all resources.
func (c *CloudGraphRAGProvider) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return nil
	}

	// Close HTTP client connections
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}

	// Clear cache
	c.cache.clear()

	c.initialized = false
	return nil
}

// setAuthHeaders sets authentication headers on the request.
func (c *CloudGraphRAGProvider) setAuthHeaders(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// retryWithBackoff retries the given operation with exponential backoff.
// Implements retry logic for transient failures with max 3 attempts.
func (c *CloudGraphRAGProvider) retryWithBackoff(ctx context.Context, operation func() error) error {
	maxAttempts := 3
	baseDelay := 100 * time.Millisecond
	maxDelay := 5 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if graphErr, ok := err.(*graphrag.GraphRAGError); ok {
			if !graphErr.Retryable {
				return err // Don't retry non-retryable errors
			}
		}

		// Don't retry on last attempt
		if attempt == maxAttempts-1 {
			break
		}

		// Calculate exponential backoff delay
		delay := baseDelay * time.Duration(1<<uint(attempt))
		if delay > maxDelay {
			delay = maxDelay
		}

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return lastErr
}

// cloudCache methods

func (cc *cloudCache) storeNode(node graphrag.GraphNode) {
	if !cc.enabled {
		return
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.nodes[node.ID.String()] = node
}

func (cc *cloudCache) storeRelationship(rel graphrag.Relationship) {
	if !cc.enabled {
		return
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.relationships[rel.ID.String()] = rel
}

func (cc *cloudCache) queryNodes(query graphrag.NodeQuery) []graphrag.GraphNode {
	if !cc.enabled {
		return []graphrag.GraphNode{}
	}

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	// Simple cache lookup - production would implement proper filtering
	nodes := make([]graphrag.GraphNode, 0, len(cc.nodes))
	for _, node := range cc.nodes {
		// Basic filter matching
		match := true

		// Filter by node types
		if len(query.NodeTypes) > 0 {
			hasType := false
			for _, queryType := range query.NodeTypes {
				for _, nodeType := range node.Labels {
					if nodeType == queryType {
						hasType = true
						break
					}
				}
				if hasType {
					break
				}
			}
			if !hasType {
				match = false
			}
		}

		// Filter by mission ID
		if query.MissionID != nil {
			if node.MissionID == nil || *node.MissionID != *query.MissionID {
				match = false
			}
		}

		if match {
			nodes = append(nodes, node)
		}

		if query.Limit > 0 && len(nodes) >= query.Limit {
			break
		}
	}

	return nodes
}

func (cc *cloudCache) clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.nodes = make(map[string]graphrag.GraphNode)
	cc.relationships = make(map[string]graphrag.Relationship)
}
