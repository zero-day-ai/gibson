// Package payload provides built-in payload loading functionality.
//
// The built-in loader reads YAML payload definitions embedded in the binary
// at compile time using Go's embed directive. It supports multiple YAML formats:
//   - Single payload per file
//   - Array of payloads per file
//   - Wrapped format with "payloads:" key containing an array
//
// All loaded payloads are automatically marked as BuiltIn=true and Enabled=true.
// String IDs in YAML files are converted to deterministic UUIDs using UUID v5.
// Invalid payloads are gracefully skipped with warnings logged.
package payload

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/zero-day-ai/gibson/internal/types"
	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml builtin/**/*.yaml
var builtinFS embed.FS

// builtinNamespace is the UUID namespace used for generating deterministic UUIDs
// for built-in payloads from their string IDs. This ensures the same string ID
// always produces the same UUID across different runs and systems.
var builtinNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

// BuiltInLoader provides functionality to load built-in payloads from embedded YAML files.
// Built-in payloads are distributed with Gibson and provide a curated set of attack payloads
// covering common LLM vulnerabilities.
type BuiltInLoader interface {
	// Load parses all embedded payload files and returns them as a slice
	Load() ([]Payload, error)

	// GetCategories returns a list of all categories represented in built-in payloads
	GetCategories() []PayloadCategory

	// GetCount returns the total number of built-in payloads
	GetCount() int
}

// builtInLoader is the default implementation of BuiltInLoader
type builtInLoader struct {
	payloads     []Payload
	categories   map[PayloadCategory]bool
	loaded       bool
	loadErrors   []error // Track non-fatal errors during loading
	skippedCount int     // Count of skipped payloads due to validation errors
}

// NewBuiltInLoader creates a new built-in payload loader
func NewBuiltInLoader() BuiltInLoader {
	return &builtInLoader{
		payloads:   make([]Payload, 0),
		categories: make(map[PayloadCategory]bool),
		loaded:     false,
	}
}

// Load parses all embedded YAML files and returns the payloads
func (b *builtInLoader) Load() ([]Payload, error) {
	if b.loaded {
		return b.payloads, nil
	}

	// Walk through all embedded files
	err := fs.WalkDir(builtinFS, "builtin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking builtin directory: %w", err)
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process YAML files
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}

		// Read the file contents
		data, err := fs.ReadFile(builtinFS, path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Parse the YAML file - it might contain a single payload or multiple payloads
		// First, try to parse as a structure with a "payloads" array
		var wrapper struct {
			Payloads []Payload `yaml:"payloads"`
		}
		if err := yaml.Unmarshal(data, &wrapper); err == nil && len(wrapper.Payloads) > 0 {
			// Successfully parsed as wrapper with payloads array
			for i := range wrapper.Payloads {
				if err := b.processPayload(&wrapper.Payloads[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
					// Log the error but continue processing other payloads
					log.Printf("WARNING: Skipping invalid payload in %s[%d]: %v", path, i, err)
					b.loadErrors = append(b.loadErrors, fmt.Errorf("%s[%d]: %w", path, i, err))
					b.skippedCount++
				}
			}
			return nil
		}

		// Try parsing as a direct array of payloads
		var payloads []Payload
		if err := yaml.Unmarshal(data, &payloads); err == nil && len(payloads) > 0 {
			// Successfully parsed as array
			for i := range payloads {
				if err := b.processPayload(&payloads[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
					// Log the error but continue processing other payloads
					log.Printf("WARNING: Skipping invalid payload in %s[%d]: %v", path, i, err)
					b.loadErrors = append(b.loadErrors, fmt.Errorf("%s[%d]: %w", path, i, err))
					b.skippedCount++
				}
			}
			return nil
		}

		// Try parsing as a single payload
		var payload Payload
		if err := yaml.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("failed to parse YAML file %s (tried wrapper, array, and single formats): %w", path, err)
		}

		// Check if we actually got a valid payload (not just an empty struct)
		if payload.Name == "" && payload.Template == "" {
			return fmt.Errorf("failed to parse YAML file %s: no valid payload data found", path)
		}

		// Successfully parsed as single payload
		if err := b.processPayload(&payload, path); err != nil {
			// Log the error but don't fail the entire load
			log.Printf("WARNING: Skipping invalid payload in %s: %v", path, err)
			b.loadErrors = append(b.loadErrors, fmt.Errorf("%s: %w", path, err))
			b.skippedCount++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load built-in payloads: %w", err)
	}

	// Log summary if any payloads were skipped
	if b.skippedCount > 0 {
		log.Printf("Loaded %d built-in payloads (%d skipped due to validation errors)", len(b.payloads), b.skippedCount)
	}

	b.loaded = true
	return b.payloads, nil
}

// processPayload validates and marks a payload as built-in, then adds it to the collection
func (b *builtInLoader) processPayload(payload *Payload, source string) error {
	// If the ID is not a valid UUID (e.g., it's a simple string like "prompt-inject-001"),
	// convert it to a deterministic UUID using UUID v5
	if payload.ID != "" {
		if err := payload.ID.Validate(); err != nil {
			// ID is not a valid UUID, convert it to one
			originalID := string(payload.ID)
			deterministicUUID := uuid.NewSHA1(builtinNamespace, []byte(originalID))
			payload.ID = types.ID(deterministicUUID.String())
		}
	} else {
		// No ID provided, generate a new one
		payload.ID = types.NewID()
	}

	// Set timestamps if not already set
	now := time.Now()
	if payload.CreatedAt.IsZero() {
		payload.CreatedAt = now
	}
	if payload.UpdatedAt.IsZero() {
		payload.UpdatedAt = now
	}

	// Mark as built-in and enabled
	payload.BuiltIn = true
	payload.Enabled = true

	// Validate the payload
	if err := payload.Validate(); err != nil {
		return fmt.Errorf("validation failed for payload in %s: %w", source, err)
	}

	// Add to collection
	b.payloads = append(b.payloads, *payload)

	// Track categories
	for _, category := range payload.Categories {
		b.categories[category] = true
	}

	return nil
}

// GetCategories returns all categories represented in built-in payloads
func (b *builtInLoader) GetCategories() []PayloadCategory {
	// Ensure payloads are loaded
	if !b.loaded {
		_, _ = b.Load() // Ignore errors here, will return empty if load fails
	}

	categories := make([]PayloadCategory, 0, len(b.categories))
	for category := range b.categories {
		categories = append(categories, category)
	}

	return categories
}

// GetCount returns the total number of built-in payloads
func (b *builtInLoader) GetCount() int {
	// Ensure payloads are loaded
	if !b.loaded {
		_, _ = b.Load() // Ignore errors here, will return 0 if load fails
	}

	return len(b.payloads)
}

// LoadBuiltInPayloads is a convenience function to load all built-in payloads
// This is the primary entry point for loading built-in payloads.
func LoadBuiltInPayloads() ([]Payload, error) {
	loader := NewBuiltInLoader()
	return loader.Load()
}

// GetBuiltInPayloadCount returns the number of available built-in payloads
// without fully loading them (though it will load them on first call)
func GetBuiltInPayloadCount() int {
	loader := NewBuiltInLoader()
	return loader.GetCount()
}

// GetBuiltInCategories returns all categories available in built-in payloads
func GetBuiltInCategories() []PayloadCategory {
	loader := NewBuiltInLoader()
	return loader.GetCategories()
}
