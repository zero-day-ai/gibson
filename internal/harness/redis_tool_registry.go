package harness

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/tool"
	"github.com/zero-day-ai/sdk/queue"
)

// RedisToolRegistry discovers and manages tools from Redis.
// It provides a registry interface for tools that are executed via Redis queues.
type RedisToolRegistry struct {
	client  *queue.RedisClient
	proxies map[string]*RedisToolProxy
	logger  *slog.Logger
	mu      sync.RWMutex
}

// NewRedisToolRegistry creates a new Redis-based tool registry.
func NewRedisToolRegistry(client *queue.RedisClient, logger *slog.Logger) *RedisToolRegistry {
	return &RedisToolRegistry{
		client:  client,
		proxies: make(map[string]*RedisToolProxy),
		logger:  logger.With("component", "redis_tool_registry"),
	}
}

// Refresh discovers tools from Redis tools:available set and updates the registry.
// It fetches metadata for each tool and creates RedisToolProxy instances.
func (r *RedisToolRegistry) Refresh(ctx context.Context) error {
	r.logger.DebugContext(ctx, "refreshing tool registry from Redis")

	// List all tools from Redis
	tools, err := r.client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	r.logger.InfoContext(ctx, "discovered tools from Redis", "count", len(tools))

	// Lock for writing
	r.mu.Lock()
	defer r.mu.Unlock()

	// Track tools found in this refresh
	found := make(map[string]bool)

	// Create or update proxies for each tool
	for _, meta := range tools {
		// Validate tool metadata
		if err := meta.IsValid(); err != nil {
			r.logger.WarnContext(ctx, "skipping invalid tool metadata",
				"tool", meta.Name,
				"error", err,
			)
			continue
		}

		found[meta.Name] = true

		// Create new proxy if it doesn't exist
		if _, exists := r.proxies[meta.Name]; !exists {
			proxy := NewRedisToolProxy(r.client, meta, r.logger)
			r.proxies[meta.Name] = proxy

			r.logger.InfoContext(ctx, "registered tool from Redis",
				"tool", meta.Name,
				"version", meta.Version,
				"input_type", meta.InputMessageType,
				"output_type", meta.OutputMessageType,
			)
		} else {
			// Update existing proxy metadata
			r.proxies[meta.Name].meta = meta

			r.logger.DebugContext(ctx, "updated tool metadata",
				"tool", meta.Name,
				"version", meta.Version,
			)
		}
	}

	// Remove proxies for tools that no longer exist in Redis
	for name := range r.proxies {
		if !found[name] {
			delete(r.proxies, name)
			r.logger.InfoContext(ctx, "removed tool from registry",
				"tool", name,
				"reason", "no longer in Redis",
			)
		}
	}

	return nil
}

// Get returns a tool proxy by name.
// Returns nil if the tool is not registered.
func (r *RedisToolRegistry) Get(name string) (tool.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	proxy, exists := r.proxies[name]
	if !exists {
		return nil, false
	}

	return proxy, true
}

// List returns all available tool names.
func (r *RedisToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.proxies))
	for name := range r.proxies {
		names = append(names, name)
	}

	return names
}

// IsHealthy checks if a tool has healthy workers.
// It verifies that:
// 1. The tool exists in the registry
// 2. The tool has at least one worker (tool:<name>:workers > 0)
// 3. The tool's health key is fresh (tool:<name>:health TTL not expired)
func (r *RedisToolRegistry) IsHealthy(ctx context.Context, name string) bool {
	r.mu.RLock()
	proxy, exists := r.proxies[name]
	r.mu.RUnlock()

	if !exists {
		return false
	}

	// Use the proxy's Health method to check worker status
	healthStatus := proxy.Health(ctx)
	return healthStatus.State == "healthy"
}

// GetHealthStatus returns detailed health information for a tool.
// This is useful for monitoring and debugging.
func (r *RedisToolRegistry) GetHealthStatus(ctx context.Context, name string) (string, error) {
	r.mu.RLock()
	proxy, exists := r.proxies[name]
	r.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("tool %s not found in registry", name)
	}

	healthStatus := proxy.Health(ctx)
	return fmt.Sprintf("state=%s message=%s checked_at=%s",
		healthStatus.State,
		healthStatus.Message,
		healthStatus.CheckedAt.Format(time.RFC3339),
	), nil
}

// WaitForTools blocks until at least one tool is available or context is cancelled.
// This is useful during startup to ensure tools are ready before processing missions.
func (r *RedisToolRegistry) WaitForTools(ctx context.Context, checkInterval time.Duration) error {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		// Refresh tool registry
		if err := r.Refresh(ctx); err != nil {
			r.logger.WarnContext(ctx, "failed to refresh tools while waiting",
				"error", err,
			)
		}

		// Check if we have any tools
		if len(r.List()) > 0 {
			r.logger.InfoContext(ctx, "tools available",
				"count", len(r.List()),
			)
			return nil
		}

		r.logger.DebugContext(ctx, "waiting for tools to register...")

		select {
		case <-ticker.C:
			// Continue waiting
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for tools: %w", ctx.Err())
		}
	}
}

// Count returns the number of registered tools.
func (r *RedisToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.proxies)
}

// GetMetadata returns the metadata for a tool.
// This is useful for inspection and debugging.
func (r *RedisToolRegistry) GetMetadata(name string) (queue.ToolMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	proxy, exists := r.proxies[name]
	if !exists {
		return queue.ToolMeta{}, false
	}

	return proxy.meta, true
}

// GetAllMetadata returns metadata for all registered tools.
func (r *RedisToolRegistry) GetAllMetadata() []queue.ToolMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata := make([]queue.ToolMeta, 0, len(r.proxies))
	for _, proxy := range r.proxies {
		metadata = append(metadata, proxy.meta)
	}

	return metadata
}

// Close closes the registry and cleans up resources.
// Currently a no-op, but included for future use.
func (r *RedisToolRegistry) Close() error {
	r.logger.Info("closing Redis tool registry")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear proxies
	r.proxies = make(map[string]*RedisToolProxy)

	return nil
}
