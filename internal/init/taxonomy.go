package init

import (
	"fmt"
	"log"

	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
)

var (
	// globalTaxonomyRegistry holds the loaded taxonomy for Gibson components
	globalTaxonomyRegistry taxonomy.TaxonomyRegistry
)

// InitTaxonomy loads and validates the taxonomy during Gibson initialization.
// This must be called before any agents or harness components start.
// Returns an error if the taxonomy fails to load or validate.
func InitTaxonomy(customTaxonomyPath string) error {
	var err error

	// Load taxonomy (with custom extensions if provided)
	if customTaxonomyPath != "" {
		log.Printf("Loading taxonomy with custom extensions from: %s", customTaxonomyPath)
		globalTaxonomyRegistry, err = taxonomy.LoadAndValidateTaxonomyWithCustom(customTaxonomyPath)
	} else {
		log.Printf("Loading embedded taxonomy")
		globalTaxonomyRegistry, err = taxonomy.LoadAndValidateTaxonomy()
	}

	if err != nil {
		return fmt.Errorf("failed to initialize taxonomy: %w", err)
	}

	log.Printf("Taxonomy loaded successfully: version %s", globalTaxonomyRegistry.Version())
	log.Printf("  Node types: %d", len(globalTaxonomyRegistry.NodeTypes()))
	log.Printf("  Relationships: %d", len(globalTaxonomyRegistry.RelationshipTypes()))
	log.Printf("  MITRE techniques: %d", len(globalTaxonomyRegistry.Techniques("mitre")))
	log.Printf("  Arcanum techniques: %d", len(globalTaxonomyRegistry.Techniques("arcanum")))

	return nil
}

// ValidateTaxonomyOnly loads and validates the taxonomy, then exits.
// This is used for CI validation mode (--validate-taxonomy flag).
// Returns an error if validation fails.
func ValidateTaxonomyOnly(customTaxonomyPath string) error {
	if err := InitTaxonomy(customTaxonomyPath); err != nil {
		return fmt.Errorf("taxonomy validation failed: %w", err)
	}

	log.Printf("✓ Taxonomy validation successful")
	return nil
}

// GetTaxonomyRegistry returns the global taxonomy registry.
// This should only be called after InitTaxonomy has been called.
// Returns nil if taxonomy has not been initialized.
func GetTaxonomyRegistry() taxonomy.TaxonomyRegistry {
	return globalTaxonomyRegistry
}
