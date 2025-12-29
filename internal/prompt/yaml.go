package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PromptFile represents a YAML file containing prompts.
// It supports two formats:
//   1. Multiple prompts: prompts: [...]
//   2. Single prompt: direct YAML mapping
type PromptFile struct {
	Prompts []Prompt `yaml:"prompts"`
}

// LoadPromptsFromFile loads prompts from a YAML file.
// The file can contain either:
//   - An array of prompts under the "prompts" key
//   - A single prompt as a direct YAML mapping
//
// The function will try parsing as PromptFile first, and if the prompts array
// is empty, it will attempt to parse as a single Prompt.
//
// All loaded prompts are validated before being returned.
//
// Returns:
//   - []Prompt: The loaded and validated prompts
//   - error: ErrCodeYAMLParse if YAML syntax is invalid
//   - error: ErrCodeYAMLValidation if prompt validation fails
func LoadPromptsFromFile(path string) ([]Prompt, error) {
	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, NewYAMLParseError(path, fmt.Errorf("failed to read file: %w", err))
	}

	// Try parsing as PromptFile (array format) first
	var promptFile PromptFile
	if err := yaml.Unmarshal(data, &promptFile); err != nil {
		return nil, NewYAMLParseError(path, enrichYAMLError(err))
	}

	var prompts []Prompt

	// If prompts array is populated, use it
	if len(promptFile.Prompts) > 0 {
		prompts = promptFile.Prompts
	} else {
		// Try parsing as single Prompt
		var singlePrompt Prompt
		if err := yaml.Unmarshal(data, &singlePrompt); err != nil {
			return nil, NewYAMLParseError(path, enrichYAMLError(err))
		}
		prompts = []Prompt{singlePrompt}
	}

	// Validate each prompt
	for i, p := range prompts {
		if err := p.Validate(); err != nil {
			return nil, NewYAMLValidationError(
				path,
				fmt.Sprintf("prompt at index %d failed validation: %v", i, err),
			)
		}
	}

	return prompts, nil
}

// LoadPromptsFromDirectory loads all .yaml and .yml files from a directory.
// It processes each file with LoadPromptsFromFile and aggregates all prompts.
//
// The function continues loading files even if individual files fail, but returns
// the first error encountered along with any successfully loaded prompts.
//
// Returns:
//   - []Prompt: All successfully loaded and validated prompts
//   - error: First error encountered (if any)
func LoadPromptsFromDirectory(dir string) ([]Prompt, error) {
	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, NewYAMLParseError(dir, fmt.Errorf("failed to read directory: %w", err))
	}

	var allPrompts []Prompt
	var firstError error

	// Process each YAML file
	for _, entry := range entries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		// Only process .yaml and .yml files
		filename := entry.Name()
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		// Load prompts from file
		filePath := filepath.Join(dir, filename)
		prompts, err := LoadPromptsFromFile(filePath)
		if err != nil {
			// Capture first error but continue processing
			if firstError == nil {
				firstError = err
			}
			continue
		}

		// Aggregate prompts
		allPrompts = append(allPrompts, prompts...)
	}

	return allPrompts, firstError
}

// enrichYAMLError attempts to extract line and column information from YAML errors.
// yaml.v3 provides detailed error messages with line/column info that we can preserve.
func enrichYAMLError(err error) error {
	if err == nil {
		return nil
	}

	// yaml.v3 errors already include line:column information in their messages
	// We just wrap them with additional context
	return fmt.Errorf("YAML syntax error: %w", err)
}
