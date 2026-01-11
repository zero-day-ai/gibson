package toolexec

import (
	"fmt"
	"os"
	"time"

	sdktypes "github.com/zero-day-ai/sdk/types"
	"gopkg.in/yaml.v3"
)

// ComponentYAML represents the structure of a component.yaml file.
// It defines the complete schema for tool manifests, including metadata,
// build configuration, runtime settings, and dependencies.
type ComponentYAML struct {
	// Kind specifies the component type (e.g., "tool", "agent").
	Kind string `yaml:"kind"`

	// Name is the unique identifier for the component.
	Name string `yaml:"name"`

	// Version is the semantic version string (e.g., "1.0.0").
	Version string `yaml:"version"`

	// Description provides a human-readable explanation of the component's purpose.
	Description string `yaml:"description"`

	// Author identifies the component creator or maintainer.
	Author string `yaml:"author,omitempty"`

	// License specifies the software license (e.g., "MIT", "Apache-2.0").
	License string `yaml:"license,omitempty"`

	// Repository is the source code URL (e.g., GitHub repository).
	Repository string `yaml:"repository,omitempty"`

	// Build contains build-time configuration and artifact information.
	Build struct {
		// Command is the build command to execute (e.g., "go build -o tool .").
		Command string `yaml:"command"`

		// Artifacts lists the output files produced by the build.
		Artifacts []string `yaml:"artifacts"`

		// Workdir specifies the working directory for the build command.
		Workdir string `yaml:"workdir,omitempty"`
	} `yaml:"build,omitempty"`

	// Runtime contains execution-time configuration.
	Runtime struct {
		// Type specifies the runtime environment (e.g., "go", "python").
		Type string `yaml:"type"`

		// Entrypoint is the command to execute the tool (e.g., "./tool").
		Entrypoint string `yaml:"entrypoint"`

		// Port specifies the gRPC port for server-based tools (0 for subprocess mode).
		Port int `yaml:"port"`

		// Timeout configures execution timeout bounds.
		Timeout struct {
			// Default is the timeout to use if the caller doesn't specify one.
			// Format: Go duration string (e.g., "5m", "30s", "1h30m").
			Default string `yaml:"default,omitempty"`

			// Max is the maximum allowed timeout for this tool.
			// Format: Go duration string (e.g., "1h", "30m").
			Max string `yaml:"max,omitempty"`

			// Min is the minimum allowed timeout for this tool.
			// Format: Go duration string (e.g., "30s", "1m").
			Min string `yaml:"min,omitempty"`
		} `yaml:"timeout,omitempty"`
	} `yaml:"runtime"`

	// Dependencies specifies required Gibson versions, system packages, and environment.
	Dependencies struct {
		// Gibson specifies the minimum Gibson version (e.g., ">=1.0.0").
		Gibson string `yaml:"gibson,omitempty"`

		// System lists required system packages (e.g., ["nmap", "curl"]).
		System []string `yaml:"system,omitempty"`

		// Env contains required environment variables.
		Env map[string]string `yaml:"env,omitempty"`
	} `yaml:"dependencies,omitempty"`
}

// ParseComponentYAML reads and parses a component.yaml file from the specified path.
// It performs basic YAML unmarshaling but does not validate the contents.
// Use the TimeoutConfig() method to extract and validate timeout settings.
//
// Returns an error if the file cannot be read or parsed as valid YAML.
func ParseComponentYAML(path string) (*ComponentYAML, error) {
	// Read the file contents
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read component.yaml: %w", err)
	}

	// Parse YAML into struct
	var component ComponentYAML
	if err := yaml.Unmarshal(data, &component); err != nil {
		return nil, fmt.Errorf("parse component.yaml: %w", err)
	}

	return &component, nil
}

// TimeoutConfig extracts and validates the timeout configuration from the component.
// It parses duration strings from the YAML and returns a validated TimeoutConfig.
//
// The method:
// - Parses "default", "max", and "min" duration strings (e.g., "5m", "1h30m")
// - Handles missing/optional fields gracefully (zero values = not set)
// - Validates the configuration for internal consistency
//
// Returns an error if:
// - Any duration string is malformed (not a valid Go duration)
// - The configuration is invalid (e.g., min > max, default outside bounds)
func (c *ComponentYAML) TimeoutConfig() (sdktypes.TimeoutConfig, error) {
	var cfg sdktypes.TimeoutConfig

	// Parse default timeout
	if c.Runtime.Timeout.Default != "" {
		d, err := time.ParseDuration(c.Runtime.Timeout.Default)
		if err != nil {
			return cfg, fmt.Errorf("invalid default timeout %q: %w",
				c.Runtime.Timeout.Default, err)
		}
		cfg.Default = d
	}

	// Parse max timeout
	if c.Runtime.Timeout.Max != "" {
		d, err := time.ParseDuration(c.Runtime.Timeout.Max)
		if err != nil {
			return cfg, fmt.Errorf("invalid max timeout %q: %w",
				c.Runtime.Timeout.Max, err)
		}
		cfg.Max = d
	}

	// Parse min timeout
	if c.Runtime.Timeout.Min != "" {
		d, err := time.ParseDuration(c.Runtime.Timeout.Min)
		if err != nil {
			return cfg, fmt.Errorf("invalid min timeout %q: %w",
				c.Runtime.Timeout.Min, err)
		}
		cfg.Min = d
	}

	// Validate the configuration for internal consistency
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("timeout config: %w", err)
	}

	return cfg, nil
}
