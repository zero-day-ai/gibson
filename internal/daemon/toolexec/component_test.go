package toolexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseComponentYAML tests parsing of valid component.yaml files.
func TestParseComponentYAML(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		wantName    string
		wantVersion string
		wantKind    string
	}{
		{
			name: "complete component.yaml",
			yamlContent: `kind: tool
name: nmap
version: 1.0.0
description: Network mapper for host discovery and port scanning
author: Gibson Team
license: MIT
repository: https://github.com/zero-day-ai/tools

build:
  command: go build -o nmap .
  artifacts:
    - nmap

runtime:
  type: go
  entrypoint: ./nmap
  port: 0
  timeout:
    default: 10m
    max: 1h
    min: 30s

dependencies:
  gibson: ">=1.0.0"
  system:
    - nmap
`,
			wantName:    "nmap",
			wantVersion: "1.0.0",
			wantKind:    "tool",
		},
		{
			name: "minimal component.yaml",
			yamlContent: `kind: tool
name: ping
version: 0.1.0
description: Simple ping utility

runtime:
  type: go
  entrypoint: ./ping
  port: 0
`,
			wantName:    "ping",
			wantVersion: "0.1.0",
			wantKind:    "tool",
		},
		{
			name: "component without timeout section",
			yamlContent: `kind: tool
name: curl
version: 2.0.0
description: HTTP client tool

runtime:
  type: go
  entrypoint: ./curl
  port: 0
`,
			wantName:    "curl",
			wantVersion: "2.0.0",
			wantKind:    "tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			componentPath := filepath.Join(tmpDir, "component.yaml")
			if err := os.WriteFile(componentPath, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}

			// Parse the file
			component, err := ParseComponentYAML(componentPath)
			if err != nil {
				t.Fatalf("ParseComponentYAML() unexpected error: %v", err)
			}

			// Verify fields
			if component.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", component.Name, tt.wantName)
			}
			if component.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", component.Version, tt.wantVersion)
			}
			if component.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", component.Kind, tt.wantKind)
			}
		})
	}
}

// TestParseComponentYAML_Errors tests error handling for invalid files.
func TestParseComponentYAML_Errors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns path to test file
		wantErr string
	}{
		{
			name: "file not found",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/component.yaml"
			},
			wantErr: "read component.yaml",
		},
		{
			name: "invalid YAML syntax",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "component.yaml")
				content := `kind: tool
name: broken
version: [invalid yaml syntax
`
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				return path
			},
			wantErr: "parse component.yaml",
		},
		{
			name: "empty file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				path := filepath.Join(tmpDir, "component.yaml")
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				return path
			},
			wantErr: "", // Empty YAML is valid, just results in zero values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			_, err := ParseComponentYAML(path)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ParseComponentYAML() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ParseComponentYAML() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ParseComponentYAML() error = %v, want error containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

// TestComponentYAML_TimeoutConfig tests extraction of timeout configuration.
func TestComponentYAML_TimeoutConfig(t *testing.T) {
	tests := []struct {
		name            string
		yamlContent     string
		wantDefault     time.Duration
		wantMax         time.Duration
		wantMin         time.Duration
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "all timeout fields set",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 10m
    max: 1h
    min: 30s
`,
			wantDefault: 10 * time.Minute,
			wantMax:     1 * time.Hour,
			wantMin:     30 * time.Second,
			wantErr:     false,
		},
		{
			name: "only default set",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 5m
`,
			wantDefault: 5 * time.Minute,
			wantMax:     0,
			wantMin:     0,
			wantErr:     false,
		},
		{
			name: "only max set",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    max: 30m
`,
			wantDefault: 0,
			wantMax:     30 * time.Minute,
			wantMin:     0,
			wantErr:     false,
		},
		{
			name: "only min set",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    min: 1m
`,
			wantDefault: 0,
			wantMax:     0,
			wantMin:     1 * time.Minute,
			wantErr:     false,
		},
		{
			name: "no timeout section",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
`,
			wantDefault: 0,
			wantMax:     0,
			wantMin:     0,
			wantErr:     false,
		},
		{
			name: "empty timeout section",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
`,
			wantDefault: 0,
			wantMax:     0,
			wantMin:     0,
			wantErr:     false,
		},
		{
			name: "complex duration formats",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 1h30m
    max: 2h45m30s
    min: 500ms
`,
			wantDefault: 90 * time.Minute,
			wantMax:     2*time.Hour + 45*time.Minute + 30*time.Second,
			wantMin:     500 * time.Millisecond,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			componentPath := filepath.Join(tmpDir, "component.yaml")
			if err := os.WriteFile(componentPath, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}

			// Parse the file
			component, err := ParseComponentYAML(componentPath)
			if err != nil {
				t.Fatalf("ParseComponentYAML() unexpected error: %v", err)
			}

			// Extract timeout config
			cfg, err := component.TimeoutConfig()

			if tt.wantErr {
				if err == nil {
					t.Errorf("TimeoutConfig() expected error containing %q, got nil", tt.wantErrContains)
				} else if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("TimeoutConfig() error = %v, want error containing %q", err, tt.wantErrContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("TimeoutConfig() unexpected error: %v", err)
			}

			// Verify extracted values
			if cfg.Default != tt.wantDefault {
				t.Errorf("Default = %v, want %v", cfg.Default, tt.wantDefault)
			}
			if cfg.Max != tt.wantMax {
				t.Errorf("Max = %v, want %v", cfg.Max, tt.wantMax)
			}
			if cfg.Min != tt.wantMin {
				t.Errorf("Min = %v, want %v", cfg.Min, tt.wantMin)
			}
		})
	}
}

// TestComponentYAML_TimeoutConfig_InvalidDuration tests error handling for bad duration strings.
func TestComponentYAML_TimeoutConfig_InvalidDuration(t *testing.T) {
	tests := []struct {
		name            string
		yamlContent     string
		wantErrContains string
	}{
		{
			name: "invalid default duration",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 10 minutes
`,
			wantErrContains: "invalid default timeout",
		},
		{
			name: "invalid max duration",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    max: 1 hour
`,
			wantErrContains: "invalid max timeout",
		},
		{
			name: "invalid min duration",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    min: thirty seconds
`,
			wantErrContains: "invalid min timeout",
		},
		{
			name: "invalid duration format - no unit",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 300
`,
			wantErrContains: "invalid default timeout",
		},
		{
			name: "invalid duration format - typo",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    max: 10mins
`,
			wantErrContains: "invalid max timeout",
		},
		{
			name: "negative duration",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    min: -5m
`,
			wantErrContains: "", // Negative durations parse successfully but would fail validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			componentPath := filepath.Join(tmpDir, "component.yaml")
			if err := os.WriteFile(componentPath, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}

			// Parse the file
			component, err := ParseComponentYAML(componentPath)
			if err != nil {
				t.Fatalf("ParseComponentYAML() unexpected error: %v", err)
			}

			// Extract timeout config
			_, err = component.TimeoutConfig()

			if tt.wantErrContains == "" {
				// Test allows success (e.g., negative durations parse but may fail validation later)
				return
			}

			if err == nil {
				t.Errorf("TimeoutConfig() expected error containing %q, got nil", tt.wantErrContains)
			} else if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("TimeoutConfig() error = %v, want error containing %q", err, tt.wantErrContains)
			}
		})
	}
}

// TestComponentYAML_TimeoutConfig_ValidationErrors tests config validation errors.
func TestComponentYAML_TimeoutConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name            string
		yamlContent     string
		wantErrContains string
	}{
		{
			name: "min > max",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    min: 1h
    max: 30m
`,
			wantErrContains: "min timeout",
		},
		{
			name: "default < min",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 30s
    min: 1m
`,
			wantErrContains: "default timeout",
		},
		{
			name: "default > max",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 2h
    max: 1h
`,
			wantErrContains: "default timeout",
		},
		{
			name: "default outside both bounds",
			yamlContent: `kind: tool
name: test
version: 1.0.0
runtime:
  type: go
  entrypoint: ./test
  port: 0
  timeout:
    default: 30s
    min: 1m
    max: 10m
`,
			wantErrContains: "default timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			componentPath := filepath.Join(tmpDir, "component.yaml")
			if err := os.WriteFile(componentPath, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}

			// Parse the file
			component, err := ParseComponentYAML(componentPath)
			if err != nil {
				t.Fatalf("ParseComponentYAML() unexpected error: %v", err)
			}

			// Extract timeout config - should fail validation
			_, err = component.TimeoutConfig()

			if err == nil {
				t.Errorf("TimeoutConfig() expected validation error containing %q, got nil", tt.wantErrContains)
			} else if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("TimeoutConfig() error = %v, want error containing %q", err, tt.wantErrContains)
			}
		})
	}
}

// TestComponentYAML_RealWorldExamples tests with realistic component.yaml content.
func TestComponentYAML_RealWorldExamples(t *testing.T) {
	t.Run("nmap with aggressive timeout", func(t *testing.T) {
		yamlContent := `kind: tool
name: nmap
version: 1.0.0
description: Network mapper for host discovery and port scanning

build:
  command: go build -o nmap .
  artifacts:
    - nmap

runtime:
  type: go
  entrypoint: ./nmap
  port: 0
  timeout:
    default: 10m
    max: 1h
    min: 30s

dependencies:
  gibson: ">=1.0.0"
  system:
    - nmap
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "component.yaml")
		if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		component, err := ParseComponentYAML(path)
		if err != nil {
			t.Fatalf("ParseComponentYAML() error: %v", err)
		}

		cfg, err := component.TimeoutConfig()
		if err != nil {
			t.Fatalf("TimeoutConfig() error: %v", err)
		}

		if cfg.Default != 10*time.Minute {
			t.Errorf("Default = %v, want 10m", cfg.Default)
		}
		if cfg.Max != 1*time.Hour {
			t.Errorf("Max = %v, want 1h", cfg.Max)
		}
		if cfg.Min != 30*time.Second {
			t.Errorf("Min = %v, want 30s", cfg.Min)
		}
	})

	t.Run("quick tool with short timeout", func(t *testing.T) {
		yamlContent := `kind: tool
name: ping
version: 1.0.0
description: Simple ping utility

runtime:
  type: go
  entrypoint: ./ping
  port: 0
  timeout:
    default: 5s
    max: 30s
    min: 1s
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "component.yaml")
		if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		component, err := ParseComponentYAML(path)
		if err != nil {
			t.Fatalf("ParseComponentYAML() error: %v", err)
		}

		cfg, err := component.TimeoutConfig()
		if err != nil {
			t.Fatalf("TimeoutConfig() error: %v", err)
		}

		if cfg.Default != 5*time.Second {
			t.Errorf("Default = %v, want 5s", cfg.Default)
		}
		if cfg.Max != 30*time.Second {
			t.Errorf("Max = %v, want 30s", cfg.Max)
		}
		if cfg.Min != 1*time.Second {
			t.Errorf("Min = %v, want 1s", cfg.Min)
		}
	})
}
