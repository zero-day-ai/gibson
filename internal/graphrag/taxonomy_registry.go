package graphrag

import (
	"fmt"
	"sync"

	"github.com/zero-day-ai/sdk/graphrag"
)

// TaxonomyRegistry manages the core taxonomy plus agent-installed extensions.
// It provides thread-safe access to node types, relationships, and parent relationship lookups.
type TaxonomyRegistry interface {
	// Core taxonomy access (delegates to existing TaxonomyReader)
	graphrag.TaxonomyReader

	// Extension management
	RegisterExtension(agentName string, ext TaxonomyExtension) error
	UnregisterExtension(agentName string) error

	// Relationship rule lookup
	GetParentRelationship(childType, parentType string) (string, bool)
	IsAssetType(nodeType string) bool
}

// TaxonomyExtension defines custom node types and relationships for an agent.
type TaxonomyExtension struct {
	NodeTypes     []NodeTypeDefinition
	Relationships []RelationshipDefinition
}

// NodeTypeDefinition describes a custom node type.
type NodeTypeDefinition struct {
	Name       string
	Category   string // "Asset", "Finding", "Execution", etc.
	Properties []graphrag.PropertyInfo
}

// RelationshipDefinition describes a custom relationship.
type RelationshipDefinition struct {
	Name      string
	FromTypes []string
	ToTypes   []string
}

// relationshipRule maps a child-parent type pair to a relationship name.
type relationshipRule struct {
	childType  string
	parentType string
	relName    string
}

// taxonomyRegistry implements TaxonomyRegistry.
type taxonomyRegistry struct {
	mu sync.RWMutex

	// Delegate to SDK's TaxonomyReader for core taxonomy operations
	delegate graphrag.TaxonomyReader

	// Extension tracking
	extensions map[string]TaxonomyExtension // agentName -> extension

	// Relationship rule map for fast lookup: (childType, parentType) -> relationshipName
	// Built from core rules + extensions
	relationshipMap map[string]map[string]string // childType -> parentType -> relName

	// Asset types set for fast lookup
	assetTypes map[string]bool
}

// NewTaxonomyRegistry creates a new TaxonomyRegistry with core relationship rules.
func NewTaxonomyRegistry(delegate graphrag.TaxonomyReader) TaxonomyRegistry {
	r := &taxonomyRegistry{
		delegate:        delegate,
		extensions:      make(map[string]TaxonomyExtension),
		relationshipMap: make(map[string]map[string]string),
		assetTypes:      make(map[string]bool),
	}

	// Initialize core relationship rules from design document
	r.initializeCoreRules()
	r.initializeAssetTypes()

	return r
}

// initializeCoreRules populates the relationship map with core taxonomy rules.
func (r *taxonomyRegistry) initializeCoreRules() {
	coreRules := []relationshipRule{
		{childType: "port", parentType: "host", relName: graphrag.RelTypeHasPort},
		{childType: "service", parentType: "port", relName: graphrag.RelTypeRunsService},
		{childType: "endpoint", parentType: "service", relName: graphrag.RelTypeHasEndpoint},
		{childType: "subdomain", parentType: "domain", relName: graphrag.RelTypeHasSubdomain},
		{childType: "certificate", parentType: "host", relName: graphrag.RelTypeServesCertificate},
		{childType: "api_endpoint", parentType: "api", relName: graphrag.RelTypeHasEndpoint},
	}

	for _, rule := range coreRules {
		r.addRelationshipRule(rule.childType, rule.parentType, rule.relName)
	}
}

// initializeAssetTypes populates the asset types set.
func (r *taxonomyRegistry) initializeAssetTypes() {
	// Asset types from SDK canonical taxonomy
	assetTypes := []string{
		graphrag.NodeTypeDomain,
		graphrag.NodeTypeSubdomain,
		graphrag.NodeTypeHost,
		graphrag.NodeTypePort,
		graphrag.NodeTypeService,
		graphrag.NodeTypeEndpoint,
		graphrag.NodeTypeApi,
		graphrag.NodeTypeTechnology,
		graphrag.NodeTypeCloudAsset,
		graphrag.NodeTypeCertificate,
	}

	for _, nodeType := range assetTypes {
		r.assetTypes[nodeType] = true
	}
}

// addRelationshipRule adds a relationship rule to the map.
func (r *taxonomyRegistry) addRelationshipRule(childType, parentType, relName string) {
	if r.relationshipMap[childType] == nil {
		r.relationshipMap[childType] = make(map[string]string)
	}
	r.relationshipMap[childType][parentType] = relName
}

// Version delegates to the underlying TaxonomyReader.
func (r *taxonomyRegistry) Version() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.delegate == nil {
		return "unknown"
	}
	return r.delegate.Version()
}

// IsCanonicalNodeType delegates to the underlying TaxonomyReader.
func (r *taxonomyRegistry) IsCanonicalNodeType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.delegate == nil {
		return false
	}
	// Check if it's in core taxonomy
	if r.delegate.IsCanonicalNodeType(typeName) {
		return true
	}
	// Check if it's in any registered extension
	for _, ext := range r.extensions {
		for _, nt := range ext.NodeTypes {
			if nt.Name == typeName {
				return true
			}
		}
	}
	return false
}

// IsCanonicalRelationType delegates to the underlying TaxonomyReader.
func (r *taxonomyRegistry) IsCanonicalRelationType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.delegate == nil {
		return false
	}
	// Check if it's in core taxonomy
	if r.delegate.IsCanonicalRelationType(typeName) {
		return true
	}
	// Check if it's in any registered extension
	for _, ext := range r.extensions {
		for _, rel := range ext.Relationships {
			if rel.Name == typeName {
				return true
			}
		}
	}
	return false
}

// ValidateNodeType delegates to the underlying TaxonomyReader.
func (r *taxonomyRegistry) ValidateNodeType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.delegate == nil {
		return true // Allow if no delegate
	}
	return r.delegate.ValidateNodeType(typeName)
}

// ValidateRelationType delegates to the underlying TaxonomyReader.
func (r *taxonomyRegistry) ValidateRelationType(typeName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.delegate == nil {
		return true // Allow if no delegate
	}
	return r.delegate.ValidateRelationType(typeName)
}

// RegisterExtension registers a taxonomy extension for an agent.
// If the agent already has an extension, it is replaced (last-installed wins).
func (r *taxonomyRegistry) RegisterExtension(agentName string, ext TaxonomyExtension) error {
	if agentName == "" {
		return fmt.Errorf("agentName cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if agent already has an extension (conflict)
	if _, exists := r.extensions[agentName]; exists {
		// Log warning about redefinition (caller should handle logging)
		// For now, just replace
	}

	// Store extension
	r.extensions[agentName] = ext

	// Register relationship rules from extension
	for _, rel := range ext.Relationships {
		// For each relationship, register all from/to type combinations
		for _, fromType := range rel.FromTypes {
			for _, toType := range rel.ToTypes {
				r.addRelationshipRule(toType, fromType, rel.Name)
			}
		}
	}

	// Register asset types from extension (if category is "Asset")
	for _, nt := range ext.NodeTypes {
		if nt.Category == "Asset" {
			r.assetTypes[nt.Name] = true
		}
	}

	return nil
}

// UnregisterExtension removes a taxonomy extension for an agent.
func (r *taxonomyRegistry) UnregisterExtension(agentName string) error {
	if agentName == "" {
		return fmt.Errorf("agentName cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if extension exists
	ext, exists := r.extensions[agentName]
	if !exists {
		// Not an error - idempotent operation
		return nil
	}

	// Remove extension
	delete(r.extensions, agentName)

	// Rebuild relationship map and asset types (expensive but rare operation)
	r.rebuildRelationshipMap()
	r.rebuildAssetTypes()

	// Remove asset types from this extension
	for _, nt := range ext.NodeTypes {
		if nt.Category == "Asset" {
			delete(r.assetTypes, nt.Name)
		}
	}

	return nil
}

// rebuildRelationshipMap rebuilds the relationship map from core rules + remaining extensions.
func (r *taxonomyRegistry) rebuildRelationshipMap() {
	// Clear and rebuild
	r.relationshipMap = make(map[string]map[string]string)

	// Re-add core rules
	r.initializeCoreRules()

	// Re-add extension rules
	for _, ext := range r.extensions {
		for _, rel := range ext.Relationships {
			for _, fromType := range rel.FromTypes {
				for _, toType := range rel.ToTypes {
					r.addRelationshipRule(toType, fromType, rel.Name)
				}
			}
		}
	}
}

// rebuildAssetTypes rebuilds the asset types set from core types + remaining extensions.
func (r *taxonomyRegistry) rebuildAssetTypes() {
	// Clear and rebuild
	r.assetTypes = make(map[string]bool)

	// Re-add core asset types
	r.initializeAssetTypes()

	// Re-add extension asset types
	for _, ext := range r.extensions {
		for _, nt := range ext.NodeTypes {
			if nt.Category == "Asset" {
				r.assetTypes[nt.Name] = true
			}
		}
	}
}

// GetParentRelationship looks up the relationship name for a child-parent type pair.
// Returns (relationshipName, true) if found, ("", false) if not found.
func (r *taxonomyRegistry) GetParentRelationship(childType, parentType string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if parentMap, ok := r.relationshipMap[childType]; ok {
		if relName, ok := parentMap[parentType]; ok {
			return relName, true
		}
	}

	return "", false
}

// IsAssetType checks if a node type is considered an asset type.
func (r *taxonomyRegistry) IsAssetType(nodeType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.assetTypes[nodeType]
}
