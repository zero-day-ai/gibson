package taxonomy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TaxonomyLoader provides functionality to load taxonomy definitions from embedded
// YAML files and optionally merge with custom taxonomy files.
type TaxonomyLoader interface {
	// Load parses all embedded taxonomy YAML files and returns a complete Taxonomy.
	Load() (*Taxonomy, error)

	// LoadWithCustom loads embedded taxonomy and merges custom definitions from the specified path.
	// Custom definitions are additive only - they cannot override or remove bundled types.
	LoadWithCustom(customPath string) (*Taxonomy, error)
}

// taxonomyLoader is the default implementation of TaxonomyLoader.
type taxonomyLoader struct {
	embeddedFS fs.FS
}

// NewTaxonomyLoader creates a new TaxonomyLoader using the embedded filesystem.
func NewTaxonomyLoader() TaxonomyLoader {
	return &taxonomyLoader{
		embeddedFS: GetEmbeddedFS(),
	}
}

// Load parses all embedded YAML files and constructs the complete taxonomy.
func (l *taxonomyLoader) Load() (*Taxonomy, error) {
	// First, load the root taxonomy.yaml to get version and metadata
	rootData, err := fs.ReadFile(l.embeddedFS, "taxonomy.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read root taxonomy.yaml: %w", err)
	}

	var root struct {
		Version  string           `yaml:"version"`
		Metadata TaxonomyMetadata `yaml:"metadata"`
		Includes []string         `yaml:"includes"`
	}

	if err := yaml.Unmarshal(rootData, &root); err != nil {
		return nil, fmt.Errorf("failed to parse root taxonomy.yaml: %w", err)
	}

	// Create taxonomy with version from root file
	taxonomy := NewTaxonomy(root.Version)
	taxonomy.Metadata = root.Metadata
	taxonomy.Includes = root.Includes

	// Load each included file
	for _, includePath := range root.Includes {
		if err := l.loadFile(taxonomy, includePath, "embedded"); err != nil {
			return nil, fmt.Errorf("failed to load included file %s: %w", includePath, err)
		}
	}

	return taxonomy, nil
}

// LoadWithCustom loads embedded taxonomy and merges custom definitions.
func (l *taxonomyLoader) LoadWithCustom(customPath string) (*Taxonomy, error) {
	// First load the embedded taxonomy
	taxonomy, err := l.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded taxonomy: %w", err)
	}

	// Load custom taxonomy from filesystem
	customData, err := os.ReadFile(customPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read custom taxonomy file %s: %w", customPath, err)
	}

	// Parse custom taxonomy root
	var customRoot struct {
		Version  string   `yaml:"version"`
		Includes []string `yaml:"includes"`
	}

	if err := yaml.Unmarshal(customData, &customRoot); err != nil {
		return nil, fmt.Errorf("failed to parse custom taxonomy file: %w", err)
	}

	// Get the directory containing the custom taxonomy file
	customDir := filepath.Dir(customPath)

	// Load each custom included file
	for _, includePath := range customRoot.Includes {
		fullPath := filepath.Join(customDir, includePath)
		if err := l.loadFileFromDisk(taxonomy, fullPath, "custom"); err != nil {
			return nil, fmt.Errorf("failed to load custom file %s: %w", fullPath, err)
		}
	}

	// Mark taxonomy as having custom definitions
	taxonomy.MarkCustomLoaded()

	return taxonomy, nil
}

// loadFile loads and parses a single taxonomy file from the embedded filesystem.
func (l *taxonomyLoader) loadFile(taxonomy *Taxonomy, path string, source string) error {
	data, err := fs.ReadFile(l.embeddedFS, path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return l.parseAndMerge(taxonomy, data, path, source)
}

// loadFileFromDisk loads and parses a single taxonomy file from disk.
func (l *taxonomyLoader) loadFileFromDisk(taxonomy *Taxonomy, path string, source string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return l.parseAndMerge(taxonomy, data, path, source)
}

// parseAndMerge parses YAML data and merges it into the taxonomy.
func (l *taxonomyLoader) parseAndMerge(taxonomy *Taxonomy, data []byte, path string, source string) error {
	// Determine file type from path
	if strings.Contains(path, "nodes/") || strings.HasSuffix(path, "nodes.yaml") {
		return l.parseNodeTypes(taxonomy, data, path, source)
	} else if strings.Contains(path, "relationships/") || strings.HasSuffix(path, "relationships.yaml") {
		return l.parseRelationshipTypes(taxonomy, data, path, source)
	} else if strings.Contains(path, "techniques/") || strings.HasSuffix(path, "techniques.yaml") {
		return l.parseTechniques(taxonomy, data, path, source)
	} else if strings.Contains(path, "targets/") || strings.HasSuffix(path, "targets.yaml") {
		return l.parseTargetTypes(taxonomy, data, path, source)
	} else if strings.Contains(path, "technique-types/") || strings.HasSuffix(path, "technique-types.yaml") {
		return l.parseTechniqueTypes(taxonomy, data, path, source)
	} else if strings.Contains(path, "capabilities/") || strings.HasSuffix(path, "capabilities.yaml") {
		return l.parseCapabilities(taxonomy, data, path, source)
	}

	return fmt.Errorf("unknown file type for path: %s", path)
}

// parseNodeTypes parses node type definitions from YAML.
func (l *taxonomyLoader) parseNodeTypes(taxonomy *Taxonomy, data []byte, path string, source string) error {
	var nodeFile NodeTypeFile
	if err := yaml.Unmarshal(data, &nodeFile); err != nil {
		return fmt.Errorf("failed to parse node types YAML: %w", err)
	}

	for i := range nodeFile.NodeTypes {
		nodeDef := &nodeFile.NodeTypes[i]
		if err := taxonomy.AddNodeType(nodeDef); err != nil {
			// If this is a custom taxonomy and the error is a duplicate, provide clear message
			if source == "custom" {
				if taxErr, ok := err.(*TaxonomyError); ok && taxErr.Type == ErrorTypeDuplicateDefinition {
					return fmt.Errorf("custom node type %s (ID: %s) conflicts with bundled taxonomy - custom types cannot override bundled types", nodeDef.Type, nodeDef.ID)
				}
			}
			return fmt.Errorf("failed to add node type %s from %s: %w", nodeDef.Type, path, err)
		}
	}

	return nil
}

// parseRelationshipTypes parses relationship type definitions from YAML.
func (l *taxonomyLoader) parseRelationshipTypes(taxonomy *Taxonomy, data []byte, path string, source string) error {
	var relFile RelationshipTypeFile
	if err := yaml.Unmarshal(data, &relFile); err != nil {
		return fmt.Errorf("failed to parse relationship types YAML: %w", err)
	}

	for i := range relFile.RelationshipTypes {
		relDef := &relFile.RelationshipTypes[i]
		if err := taxonomy.AddRelationship(relDef); err != nil {
			// If this is a custom taxonomy and the error is a duplicate, provide clear message
			if source == "custom" {
				if taxErr, ok := err.(*TaxonomyError); ok && taxErr.Type == ErrorTypeDuplicateDefinition {
					return fmt.Errorf("custom relationship %s (ID: %s) conflicts with bundled taxonomy - custom types cannot override bundled types", relDef.Type, relDef.ID)
				}
			}
			return fmt.Errorf("failed to add relationship type %s from %s: %w", relDef.Type, path, err)
		}
	}

	return nil
}

// parseTechniques parses technique definitions from YAML.
func (l *taxonomyLoader) parseTechniques(taxonomy *Taxonomy, data []byte, path string, source string) error {
	var techFile TechniqueFile
	if err := yaml.Unmarshal(data, &techFile); err != nil {
		return fmt.Errorf("failed to parse techniques YAML: %w", err)
	}

	for i := range techFile.Techniques {
		techDef := &techFile.Techniques[i]
		if err := taxonomy.AddTechnique(techDef); err != nil {
			// If this is a custom taxonomy and the error is a duplicate, provide clear message
			if source == "custom" {
				if taxErr, ok := err.(*TaxonomyError); ok && taxErr.Type == ErrorTypeDuplicateDefinition {
					return fmt.Errorf("custom technique %s conflicts with bundled taxonomy - custom techniques cannot override bundled techniques", techDef.TechniqueID)
				}
			}
			return fmt.Errorf("failed to add technique %s from %s: %w", techDef.TechniqueID, path, err)
		}
	}

	return nil
}

// parseTargetTypes parses target type definitions from YAML.
func (l *taxonomyLoader) parseTargetTypes(taxonomy *Taxonomy, data []byte, path string, source string) error {
	var targetFile TargetTypeFile
	if err := yaml.Unmarshal(data, &targetFile); err != nil {
		return fmt.Errorf("failed to parse target types YAML: %w", err)
	}

	for i := range targetFile.TargetTypes {
		targetDef := &targetFile.TargetTypes[i]
		if err := taxonomy.AddTargetType(targetDef); err != nil {
			// If this is a custom taxonomy and the error is a duplicate, provide clear message
			if source == "custom" {
				if taxErr, ok := err.(*TaxonomyError); ok && taxErr.Type == ErrorTypeDuplicateDefinition {
					return fmt.Errorf("custom target type %s (ID: %s) conflicts with bundled taxonomy - custom types cannot override bundled types", targetDef.Type, targetDef.ID)
				}
			}
			return fmt.Errorf("failed to add target type %s from %s: %w", targetDef.Type, path, err)
		}
	}

	return nil
}

// parseTechniqueTypes parses technique type definitions from YAML.
func (l *taxonomyLoader) parseTechniqueTypes(taxonomy *Taxonomy, data []byte, path string, source string) error {
	var techTypeFile TechniqueTypeFile
	if err := yaml.Unmarshal(data, &techTypeFile); err != nil {
		return fmt.Errorf("failed to parse technique types YAML: %w", err)
	}

	for i := range techTypeFile.TechniqueTypes {
		techTypeDef := &techTypeFile.TechniqueTypes[i]
		if err := taxonomy.AddTechniqueType(techTypeDef); err != nil {
			// If this is a custom taxonomy and the error is a duplicate, provide clear message
			if source == "custom" {
				if taxErr, ok := err.(*TaxonomyError); ok && taxErr.Type == ErrorTypeDuplicateDefinition {
					return fmt.Errorf("custom technique type %s (ID: %s) conflicts with bundled taxonomy - custom types cannot override bundled types", techTypeDef.Type, techTypeDef.ID)
				}
			}
			return fmt.Errorf("failed to add technique type %s from %s: %w", techTypeDef.Type, path, err)
		}
	}

	return nil
}

// parseCapabilities parses capability definitions from YAML.
func (l *taxonomyLoader) parseCapabilities(taxonomy *Taxonomy, data []byte, path string, source string) error {
	var capFile CapabilityFile
	if err := yaml.Unmarshal(data, &capFile); err != nil {
		return fmt.Errorf("failed to parse capabilities YAML: %w", err)
	}

	for i := range capFile.Capabilities {
		capDef := &capFile.Capabilities[i]
		if err := taxonomy.AddCapability(capDef); err != nil {
			// If this is a custom taxonomy and the error is a duplicate, provide clear message
			if source == "custom" {
				if taxErr, ok := err.(*TaxonomyError); ok && taxErr.Type == ErrorTypeDuplicateDefinition {
					return fmt.Errorf("custom capability %s conflicts with bundled taxonomy - custom capabilities cannot override bundled capabilities", capDef.ID)
				}
			}
			return fmt.Errorf("failed to add capability %s from %s: %w", capDef.ID, path, err)
		}
	}

	return nil
}

// LoadTaxonomy is a convenience function to load the embedded taxonomy.
// This is the primary entry point for loading the taxonomy in Gibson.
func LoadTaxonomy() (*Taxonomy, error) {
	loader := NewTaxonomyLoader()
	return loader.Load()
}

// LoadTaxonomyWithCustom is a convenience function to load taxonomy with custom extensions.
func LoadTaxonomyWithCustom(customPath string) (*Taxonomy, error) {
	loader := NewTaxonomyLoader()
	return loader.LoadWithCustom(customPath)
}
