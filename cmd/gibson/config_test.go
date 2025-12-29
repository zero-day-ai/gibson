package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
)

// setupTestConfig creates a temporary config file for testing
func setupTestConfig(t *testing.T) (string, func()) {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err, "Failed to create temp directory")

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a sample config file
	configContent := `core:
  home_dir: /home/test/.gibson
  data_dir: /home/test/.gibson/data
  cache_dir: /home/test/.gibson/cache
  parallel_limit: 10
  timeout: 300000000000
  debug: false
database:
  path: /home/test/.gibson/gibson.db
  max_connections: 10
  timeout: 30000000000
  wal_mode: true
  auto_vacuum: true
security:
  encryption_algorithm: aes-256-gcm
  key_derivation: scrypt
  ssl_validation: true
  audit_logging: true
llm:
  default_provider: openai
logging:
  level: info
  format: json
tracing:
  enabled: false
  endpoint: ""
metrics:
  enabled: false
  port: 9090
`

	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err, "Failed to write test config file")

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return configPath, cleanup
}

func TestGetConfigValue(t *testing.T) {
	configPath, cleanup := setupTestConfig(t)
	defer cleanup()

	// Load config
	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.Load(configPath)
	require.NoError(t, err)

	tests := []struct {
		name          string
		key           string
		expectedValue string
		shouldError   bool
		errorContains string
	}{
		{
			name:          "get string value",
			key:           "llm.default_provider",
			expectedValue: "openai",
			shouldError:   false,
		},
		{
			name:          "get int value",
			key:           "core.parallel_limit",
			expectedValue: "10",
			shouldError:   false,
		},
		{
			name:          "get bool value",
			key:           "security.ssl_validation",
			expectedValue: "true",
			shouldError:   false,
		},
		{
			name:          "get nested value",
			key:           "database.max_connections",
			expectedValue: "10",
			shouldError:   false,
		},
		{
			name:          "get invalid key",
			key:           "invalid.key.path",
			shouldError:   true,
			errorContains: "invalid configuration key",
		},
		{
			name:          "get non-existent field",
			key:           "core.nonexistent",
			shouldError:   true,
			errorContains: "invalid configuration key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := getConfigValue(cfg, tt.key)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
			}
		})
	}
}

func TestSetConfigValue(t *testing.T) {
	configPath, cleanup := setupTestConfig(t)
	defer cleanup()

	// Load config
	loader := config.NewConfigLoader(config.NewValidator())

	tests := []struct {
		name          string
		key           string
		value         string
		verifyKey     string
		expectedValue string
		shouldError   bool
		errorContains string
	}{
		{
			name:          "set string value",
			key:           "llm.default_provider",
			value:         "anthropic",
			verifyKey:     "llm.default_provider",
			expectedValue: "anthropic",
			shouldError:   false,
		},
		{
			name:          "set int value",
			key:           "core.parallel_limit",
			value:         "20",
			verifyKey:     "core.parallel_limit",
			expectedValue: "20",
			shouldError:   false,
		},
		{
			name:          "set bool value true",
			key:           "core.debug",
			value:         "true",
			verifyKey:     "core.debug",
			expectedValue: "true",
			shouldError:   false,
		},
		{
			name:          "set bool value false",
			key:           "security.audit_logging",
			value:         "false",
			verifyKey:     "security.audit_logging",
			expectedValue: "false",
			shouldError:   false,
		},
		{
			name:          "set invalid key",
			key:           "invalid.key",
			value:         "value",
			shouldError:   true,
			errorContains: "invalid configuration key",
		},
		{
			name:          "set invalid int value",
			key:           "core.parallel_limit",
			value:         "invalid",
			shouldError:   true,
			errorContains: "invalid integer value",
		},
		{
			name:          "set invalid bool value",
			key:           "core.debug",
			value:         "xyz",
			shouldError:   true,
			errorContains: "invalid boolean value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reload config for each test
			cfg, err := loader.Load(configPath)
			require.NoError(t, err)

			err = setConfigValue(cfg, tt.key, tt.value)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify the value was set correctly
				value, err := getConfigValue(cfg, tt.verifyKey)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = "test-provider"
	cfg.Core.ParallelLimit = 42

	// Save it
	err = saveConfig(configPath, cfg)
	require.NoError(t, err)

	// Load it back
	loader := config.NewConfigLoader(config.NewValidator())
	loadedCfg, err := loader.Load(configPath)
	require.NoError(t, err)

	// Verify values
	assert.Equal(t, "test-provider", loadedCfg.LLM.DefaultProvider)
	assert.Equal(t, 42, loadedCfg.Core.ParallelLimit)
}

func TestSnakeToTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"home_dir", "HomeDir"},
		{"default_provider", "DefaultProvider"},
		{"max_connections", "MaxConnections"},
		{"ssl_validation", "SSLValidation"},
		{"simple", "Simple"},
		{"multiple_word_field", "MultipleWordField"},
		{"llm", "LLM"},
		{"db", "DB"},
		{"wal_mode", "WALMode"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := snakeToTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigValidation(t *testing.T) {
	configPath, cleanup := setupTestConfig(t)
	defer cleanup()

	// Load config
	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.Load(configPath)
	require.NoError(t, err)

	// Try to set an invalid value that should fail validation
	err = setConfigValue(cfg, "core.parallel_limit", "0") // 0 is below min=1
	require.NoError(t, err, "Setting value should succeed")

	// Validation should fail
	validator := config.NewValidator()
	err = validator.Validate(cfg)
	assert.Error(t, err, "Validation should fail for invalid value")
	assert.Contains(t, err.Error(), "parallel_limit", "Error should mention the invalid field")
}

func TestConfigWithMissingFile(t *testing.T) {
	// Create a temp directory but no config file
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Loading with defaults should succeed
	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.LoadWithDefaults(configPath)
	require.NoError(t, err, "LoadWithDefaults should succeed even if file doesn't exist")
	assert.NotNil(t, cfg)

	// Get value should work with defaults
	value, err := getConfigValue(cfg, "llm.default_provider")
	assert.NoError(t, err)
	// Default provider is empty string
	assert.Equal(t, "", value)
}

func TestConfigInvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644)
	require.NoError(t, err)

	// Loading should fail
	loader := config.NewConfigLoader(config.NewValidator())
	_, err = loader.Load(configPath)
	assert.Error(t, err, "Loading invalid YAML should fail")
}

func TestConfigMissingRequiredField(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	// Missing required core section
	err = os.WriteFile(configPath, []byte("llm:\n  default_provider: openai\n"), 0644)
	require.NoError(t, err)

	// Loading should fail validation
	loader := config.NewConfigLoader(config.NewValidator())
	_, err = loader.Load(configPath)
	assert.Error(t, err, "Loading config with missing required field should fail")
	assert.Contains(t, err.Error(), "validation failed", "Error should mention validation failure")
}

func TestPrintConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = "test-provider"

	// Test YAML output
	err := printConfig(cfg, "yaml")
	assert.NoError(t, err)

	// Test JSON output
	err = printConfig(cfg, "json")
	assert.NoError(t, err)

	// Test invalid format
	err = printConfig(cfg, "xml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported output format")
}

func TestFormatValue(t *testing.T) {
	configPath, cleanup := setupTestConfig(t)
	defer cleanup()

	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.Load(configPath)
	require.NoError(t, err)

	tests := []struct {
		key           string
		expectedValue string
	}{
		{"llm.default_provider", "openai"},
		{"core.parallel_limit", "10"},
		{"security.ssl_validation", "true"},
		{"database.wal_mode", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			value, err := getConfigValue(cfg, tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, value)
		})
	}
}

func TestSetFieldValueBoolVariants(t *testing.T) {
	tests := []struct {
		value       string
		expected    bool
		shouldError bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"yes", true, false},
		{"no", false, false},
		{"1", true, false},
		{"0", false, false},
		{"xyz", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			cfg := config.DefaultConfig()
			err := setConfigValue(cfg, "core.debug", tt.value)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, cfg.Core.Debug)
			}
		})
	}
}

func TestGetConfigPath(t *testing.T) {
	// Test with empty config flag (should use default)
	cmd := configShowCmd
	path, err := getConfigPath(cmd)
	assert.NoError(t, err)
	assert.Contains(t, path, ".gibson")
	assert.Contains(t, path, "config.yaml")
}

func TestConfigIntegration(t *testing.T) {
	// This test performs a full workflow: load -> get -> set -> save -> load -> verify
	tmpDir, err := os.MkdirTemp("", "gibson-config-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// 1. Create initial config
	loader := config.NewConfigLoader(config.NewValidator())
	cfg, err := loader.LoadWithDefaults(configPath)
	require.NoError(t, err)

	// 2. Get original value
	originalValue, err := getConfigValue(cfg, "llm.default_provider")
	require.NoError(t, err)
	assert.Equal(t, "", originalValue)

	// 3. Set new value
	err = setConfigValue(cfg, "llm.default_provider", "anthropic")
	require.NoError(t, err)

	// 4. Save config
	err = saveConfig(configPath, cfg)
	require.NoError(t, err)

	// 5. Load config again
	cfg2, err := loader.Load(configPath)
	require.NoError(t, err)

	// 6. Verify new value
	newValue, err := getConfigValue(cfg2, "llm.default_provider")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", newValue)
}
