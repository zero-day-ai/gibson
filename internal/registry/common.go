// Package registry provides service discovery and registration infrastructure for Gibson.
//
// This file contains shared helper functions used by both EmbeddedRegistry and ExternalRegistry.
package registry

import (
	"path/filepath"
)

// buildKey constructs the etcd key for a service instance.
//
// Format: /{namespace}/{kind}/{name}/{instance-id}
// Example: /gibson/agent/k8skiller/550e8400-e29b-41d4-a716-446655440000
//
// This key structure enables efficient prefix-based queries:
//   - All services: /{namespace}/
//   - All agents: /{namespace}/agent/
//   - All k8skiller agents: /{namespace}/agent/k8skiller/
//   - Specific instance: /{namespace}/agent/k8skiller/{instance-id}
func buildKey(namespace, kind, name, instanceID string) string {
	return filepath.Join("/", namespace, kind, name, instanceID)
}

// buildPrefix constructs the etcd key prefix for discovering services.
//
// Format: /{namespace}/{kind}/{name}/
// Example: /gibson/agent/k8skiller/
//
// The trailing slash is important for prefix queries - it ensures we only
// match instances of this specific service, not other services whose names
// happen to start with the same string.
func buildPrefix(namespace, kind, name string) string {
	return filepath.Join("/", namespace, kind, name) + "/"
}
