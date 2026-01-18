// Package component provides component management for Gibson.
// This file defines the ComponentStore interface and etcd-backed implementation
// for storing component metadata in etcd, replacing the SQLite-based ComponentDAO.

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// ComponentStore provides storage operations for installed components.
// This interface replaces database.ComponentDAO with etcd-backed storage.
//
// Component metadata is stored persistently (no TTL) at:
//
//	/{namespace}/components/{kind}/{name}
//
// Running instances are stored with leases (ephemeral) at:
//
//	/{namespace}/components/{kind}/{name}/instances/{instance-id}
type ComponentStore interface {
	// Create stores a new component's metadata in etcd (no TTL/lease).
	// Returns ErrComponentExists if (kind, name) already exists.
	Create(ctx context.Context, comp *Component) error

	// GetByName retrieves component metadata by kind and name.
	// Returns nil, nil if not found.
	GetByName(ctx context.Context, kind ComponentKind, name string) (*Component, error)

	// List returns all components of a specific kind.
	List(ctx context.Context, kind ComponentKind) ([]*Component, error)

	// ListAll returns all components across all kinds.
	ListAll(ctx context.Context) (map[ComponentKind][]*Component, error)

	// Update updates component metadata (version, paths, manifest).
	Update(ctx context.Context, comp *Component) error

	// Delete removes component metadata AND all running instances atomically.
	Delete(ctx context.Context, kind ComponentKind, name string) error

	// ListInstances returns all running instances for a component.
	ListInstances(ctx context.Context, kind ComponentKind, name string) ([]sdkregistry.ServiceInfo, error)
}

// ComponentMetadata is the JSON structure stored in etcd for installed components.
// This contains only the persistent installation data, not runtime state.
type ComponentMetadata struct {
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	RepoPath  string    `json:"repo_path"`
	BinPath   string    `json:"bin_path"`
	Source    string    `json:"source"`
	Manifest  string    `json:"manifest,omitempty"` // JSON-encoded Manifest
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// etcdComponentStore implements ComponentStore using etcd.
type etcdComponentStore struct {
	client    *clientv3.Client
	namespace string
}

// EtcdComponentStore creates an etcd-backed component store.
func EtcdComponentStore(client *clientv3.Client, namespace string) ComponentStore {
	if namespace == "" {
		namespace = "gibson"
	}
	return &etcdComponentStore{
		client:    client,
		namespace: namespace,
	}
}

// componentKey constructs the etcd key for component metadata.
// Format: /{namespace}/components/{kind}/{name}
func (s *etcdComponentStore) componentKey(kind ComponentKind, name string) string {
	return filepath.Join("/", s.namespace, "components", kind.String(), name)
}

// componentKindPrefix constructs the prefix for all components of a kind.
// Format: /{namespace}/components/{kind}/
func (s *etcdComponentStore) componentKindPrefix(kind ComponentKind) string {
	return filepath.Join("/", s.namespace, "components", kind.String()) + "/"
}

// allComponentsPrefix constructs the prefix for all components.
// Format: /{namespace}/components/
func (s *etcdComponentStore) allComponentsPrefix() string {
	return filepath.Join("/", s.namespace, "components") + "/"
}

// instancesPrefix constructs the prefix for all instances of a component.
// Format: /{namespace}/components/{kind}/{name}/instances/
func (s *etcdComponentStore) instancesPrefix(kind ComponentKind, name string) string {
	return filepath.Join("/", s.namespace, "components", kind.String(), name, "instances") + "/"
}

// Create stores a new component's metadata in etcd.
func (s *etcdComponentStore) Create(ctx context.Context, comp *Component) error {
	if s.client == nil {
		return ErrStoreUnavailable
	}

	key := s.componentKey(comp.Kind, comp.Name)

	// Serialize manifest if present
	var manifestJSON string
	if comp.Manifest != nil {
		data, err := json.Marshal(comp.Manifest)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}
		manifestJSON = string(data)
	}

	now := time.Now()
	metadata := ComponentMetadata{
		Kind:      comp.Kind.String(),
		Name:      comp.Name,
		Version:   comp.Version,
		RepoPath:  comp.RepoPath,
		BinPath:   comp.BinPath,
		Source:    comp.Source.String(),
		Manifest:  manifestJSON,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal component: %w", err)
	}

	// Use transaction to create only if key doesn't exist
	txn := s.client.Txn(ctx)
	resp, err := txn.
		If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, string(data))).
		Commit()

	if err != nil {
		return fmt.Errorf("failed to create component: %w", err)
	}

	if !resp.Succeeded {
		return ErrComponentExists
	}

	// Update component timestamps
	comp.CreatedAt = now
	comp.UpdatedAt = now

	return nil
}

// GetByName retrieves component metadata by kind and name.
func (s *etcdComponentStore) GetByName(ctx context.Context, kind ComponentKind, name string) (*Component, error) {
	if s.client == nil {
		return nil, ErrStoreUnavailable
	}

	key := s.componentKey(kind, name)

	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil
	}

	return s.unmarshalComponent(resp.Kvs[0].Value)
}

// List returns all components of a specific kind.
func (s *etcdComponentStore) List(ctx context.Context, kind ComponentKind) ([]*Component, error) {
	if s.client == nil {
		return nil, ErrStoreUnavailable
	}

	prefix := s.componentKindPrefix(kind)

	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	components := make([]*Component, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		// Skip instance keys (they contain "/instances/")
		if strings.Contains(string(kv.Key), "/instances/") {
			continue
		}

		comp, err := s.unmarshalComponent(kv.Value)
		if err != nil {
			// Log error but continue
			continue
		}
		components = append(components, comp)
	}

	return components, nil
}

// ListAll returns all components across all kinds.
func (s *etcdComponentStore) ListAll(ctx context.Context) (map[ComponentKind][]*Component, error) {
	if s.client == nil {
		return nil, ErrStoreUnavailable
	}

	prefix := s.allComponentsPrefix()

	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list all components: %w", err)
	}

	result := make(map[ComponentKind][]*Component)
	for _, kv := range resp.Kvs {
		// Skip instance keys (they contain "/instances/")
		if strings.Contains(string(kv.Key), "/instances/") {
			continue
		}

		comp, err := s.unmarshalComponent(kv.Value)
		if err != nil {
			// Log error but continue
			continue
		}
		result[comp.Kind] = append(result[comp.Kind], comp)
	}

	return result, nil
}

// Update updates component metadata.
func (s *etcdComponentStore) Update(ctx context.Context, comp *Component) error {
	if s.client == nil {
		return ErrStoreUnavailable
	}

	key := s.componentKey(comp.Kind, comp.Name)

	// Check if component exists
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check component: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return ErrComponentNotFound
	}

	// Preserve created_at from existing record
	existing, err := s.unmarshalComponent(resp.Kvs[0].Value)
	if err != nil {
		return fmt.Errorf("failed to unmarshal existing component: %w", err)
	}

	// Serialize manifest if present
	var manifestJSON string
	if comp.Manifest != nil {
		data, err := json.Marshal(comp.Manifest)
		if err != nil {
			return fmt.Errorf("failed to marshal manifest: %w", err)
		}
		manifestJSON = string(data)
	}

	now := time.Now()
	metadata := ComponentMetadata{
		Kind:      comp.Kind.String(),
		Name:      comp.Name,
		Version:   comp.Version,
		RepoPath:  comp.RepoPath,
		BinPath:   comp.BinPath,
		Source:    comp.Source.String(),
		Manifest:  manifestJSON,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal component: %w", err)
	}

	_, err = s.client.Put(ctx, key, string(data))
	if err != nil {
		return fmt.Errorf("failed to update component: %w", err)
	}

	comp.UpdatedAt = now

	return nil
}

// Delete removes component metadata AND all running instances atomically.
func (s *etcdComponentStore) Delete(ctx context.Context, kind ComponentKind, name string) error {
	if s.client == nil {
		return ErrStoreUnavailable
	}

	metadataKey := s.componentKey(kind, name)
	instancesPrefix := s.instancesPrefix(kind, name)

	// Use transaction to delete metadata and all instances atomically
	txn := s.client.Txn(ctx)
	resp, err := txn.
		If(clientv3.Compare(clientv3.Version(metadataKey), ">", 0)).
		Then(
			clientv3.OpDelete(metadataKey),
			clientv3.OpDelete(instancesPrefix, clientv3.WithPrefix()),
		).
		Commit()

	if err != nil {
		return fmt.Errorf("failed to delete component: %w", err)
	}

	if !resp.Succeeded {
		return ErrComponentNotFound
	}

	return nil
}

// ListInstances returns all running instances for a component.
func (s *etcdComponentStore) ListInstances(ctx context.Context, kind ComponentKind, name string) ([]sdkregistry.ServiceInfo, error) {
	if s.client == nil {
		return nil, ErrStoreUnavailable
	}

	prefix := s.instancesPrefix(kind, name)

	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	instances := make([]sdkregistry.ServiceInfo, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var info sdkregistry.ServiceInfo
		if err := json.Unmarshal(kv.Value, &info); err != nil {
			// Log error but continue
			continue
		}
		instances = append(instances, info)
	}

	return instances, nil
}

// unmarshalComponent unmarshals a ComponentMetadata from JSON and converts to Component.
func (s *etcdComponentStore) unmarshalComponent(data []byte) (*Component, error) {
	var metadata ComponentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal component: %w", err)
	}

	comp := &Component{
		Kind:      ComponentKind(metadata.Kind),
		Name:      metadata.Name,
		Version:   metadata.Version,
		RepoPath:  metadata.RepoPath,
		BinPath:   metadata.BinPath,
		Source:    ComponentSource(metadata.Source),
		Status:    ComponentStatusAvailable, // Default status for installed components
		CreatedAt: metadata.CreatedAt,
		UpdatedAt: metadata.UpdatedAt,
	}

	// Unmarshal manifest if present
	if metadata.Manifest != "" {
		var manifest Manifest
		if err := json.Unmarshal([]byte(metadata.Manifest), &manifest); err != nil {
			return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
		comp.Manifest = &manifest
	}

	return comp, nil
}
