package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPromptConfig_Validate tests validation of PromptConfig
func TestPromptConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  PromptConfig
		wantErr bool
	}{
		{
			name: "valid config with all fields",
			config: PromptConfig{
				PromptsDir:     "/home/user/.gibson/prompts",
				LoadBuiltins:   true,
				DefaultPersona: "assistant",
			},
			wantErr: false,
		},
		{
			name: "valid config with empty prompts dir",
			config: PromptConfig{
				PromptsDir:     "",
				LoadBuiltins:   true,
				DefaultPersona: "",
			},
			wantErr: false,
		},
		{
			name: "valid config with relative path",
			config: PromptConfig{
				PromptsDir:     "./prompts",
				LoadBuiltins:   false,
				DefaultPersona: "coder",
			},
			wantErr: false,
		},
		{
			name: "valid config with absolute path",
			config: PromptConfig{
				PromptsDir:     "/etc/gibson/prompts",
				LoadBuiltins:   true,
				DefaultPersona: "",
			},
			wantErr: false,
		},
		{
			name: "invalid config with dot path",
			config: PromptConfig{
				PromptsDir:   ".",
				LoadBuiltins: true,
			},
			wantErr: true,
		},
		{
			name: "invalid config with root path",
			config: PromptConfig{
				PromptsDir:   "/",
				LoadBuiltins: true,
			},
			wantErr: true,
		},
		{
			name: "valid config with tilde path",
			config: PromptConfig{
				PromptsDir:     "~/prompts",
				LoadBuiltins:   true,
				DefaultPersona: "helper",
			},
			wantErr: false,
		},
		{
			name: "valid config with LoadBuiltins false",
			config: PromptConfig{
				PromptsDir:     "/custom/prompts",
				LoadBuiltins:   false,
				DefaultPersona: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestPromptConfig_ApplyDefaults tests default application
func TestPromptConfig_ApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		initial  PromptConfig
		expected PromptConfig
	}{
		{
			name:    "all defaults",
			initial: PromptConfig{},
			expected: PromptConfig{
				PromptsDir:     "",
				LoadBuiltins:   false, // Zero value for bool
				DefaultPersona: "",
			},
		},
		{
			name: "partial defaults - prompts dir set",
			initial: PromptConfig{
				PromptsDir: "/custom/prompts",
			},
			expected: PromptConfig{
				PromptsDir:     "/custom/prompts",
				LoadBuiltins:   false,
				DefaultPersona: "",
			},
		},
		{
			name: "partial defaults - load builtins set",
			initial: PromptConfig{
				LoadBuiltins: true,
			},
			expected: PromptConfig{
				PromptsDir:     "",
				LoadBuiltins:   true,
				DefaultPersona: "",
			},
		},
		{
			name: "partial defaults - default persona set",
			initial: PromptConfig{
				DefaultPersona: "assistant",
			},
			expected: PromptConfig{
				PromptsDir:     "",
				LoadBuiltins:   false,
				DefaultPersona: "assistant",
			},
		},
		{
			name: "no defaults needed",
			initial: PromptConfig{
				PromptsDir:     "/etc/prompts",
				LoadBuiltins:   true,
				DefaultPersona: "coder",
			},
			expected: PromptConfig{
				PromptsDir:     "/etc/prompts",
				LoadBuiltins:   true,
				DefaultPersona: "coder",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			config.ApplyDefaults()
			assert.Equal(t, tt.expected, config)
		})
	}
}

// TestNewDefaultPromptConfig tests the default config constructor
func TestNewDefaultPromptConfig(t *testing.T) {
	config := NewDefaultPromptConfig()

	require.NotNil(t, config)

	// Verify defaults are applied
	assert.Equal(t, "", config.PromptsDir)
	assert.Equal(t, false, config.LoadBuiltins) // Zero value
	assert.Equal(t, "", config.DefaultPersona)

	// Verify config is valid
	err := config.Validate()
	require.NoError(t, err)
}

// TestPromptConfig_EdgeCases tests edge cases
func TestPromptConfig_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		config  PromptConfig
		wantErr bool
	}{
		{
			name: "very long path",
			config: PromptConfig{
				PromptsDir: "/very/long/path/to/prompts/directory/that/might/exist/somewhere",
			},
			wantErr: false,
		},
		{
			name: "path with spaces",
			config: PromptConfig{
				PromptsDir: "/path/with spaces/to/prompts",
			},
			wantErr: false,
		},
		{
			name: "path with special characters",
			config: PromptConfig{
				PromptsDir: "/path/with-dashes_and_underscores/prompts",
			},
			wantErr: false,
		},
		{
			name: "windows-style path",
			config: PromptConfig{
				PromptsDir: "C:\\Users\\Gibson\\prompts",
			},
			wantErr: false,
		},
		{
			name: "persona with spaces",
			config: PromptConfig{
				DefaultPersona: "helpful assistant",
			},
			wantErr: false,
		},
		{
			name: "persona with special characters",
			config: PromptConfig{
				DefaultPersona: "code-reviewer-v2",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
