package main

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*database.DB, string, func()) {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "gibson-test-*")
	require.NoError(t, err)

	// Set GIBSON_HOME to temp directory
	oldHome := os.Getenv("GIBSON_HOME")
	os.Setenv("GIBSON_HOME", tempDir)

	// Create database
	dbPath := filepath.Join(tempDir, "gibson.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err)

	// Initialize schema
	err = db.InitSchema()
	require.NoError(t, err)

	// Cleanup function
	cleanup := func() {
		db.Close()
		os.RemoveAll(tempDir)
		os.Setenv("GIBSON_HOME", oldHome)
	}

	return db, tempDir, cleanup
}

// TestTargetAdd tests the target add command
func TestTargetAdd(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		flags       map[string]string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *database.DB)
	}{
		{
			name: "valid URL with name",
			args: []string{"https://api.openai.com/v1/chat/completions"},
			flags: map[string]string{
				"name": "test-target",
				"type": "llm_api",
			},
			wantErr: false,
			validate: func(t *testing.T, db *database.DB) {
				dao := database.NewTargetDAO(db)
				target, err := dao.GetByName(context.Background(), "test-target")
				require.NoError(t, err)
				assert.Equal(t, "test-target", target.Name)
				assert.Equal(t, "https://api.openai.com/v1/chat/completions", target.URL)
				assert.Equal(t, types.TargetTypeLLMAPI, target.Type)
				assert.Equal(t, types.ProviderOpenAI, target.Provider)
			},
		},
		{
			name: "valid URL with provider override",
			args: []string{"https://example.com/api/v1/chat"},
			flags: map[string]string{
				"name":     "custom-target",
				"provider": "custom",
				"type":     "llm_chat",
			},
			wantErr: false,
			validate: func(t *testing.T, db *database.DB) {
				dao := database.NewTargetDAO(db)
				target, err := dao.GetByName(context.Background(), "custom-target")
				require.NoError(t, err)
				assert.Equal(t, types.ProviderCustom, target.Provider)
				assert.Equal(t, types.TargetTypeLLMChat, target.Type)
			},
		},
		{
			name: "anthropic URL auto-detection",
			args: []string{"https://api.anthropic.com/v1/messages"},
			flags: map[string]string{
				"name": "claude-target",
			},
			wantErr: false,
			validate: func(t *testing.T, db *database.DB) {
				dao := database.NewTargetDAO(db)
				target, err := dao.GetByName(context.Background(), "claude-target")
				require.NoError(t, err)
				assert.Equal(t, types.ProviderAnthropic, target.Provider)
			},
		},
		{
			name: "invalid URL - missing scheme",
			args: []string{"api.openai.com/v1/chat"},
			flags: map[string]string{
				"name": "bad-target",
			},
			wantErr:     true,
			errContains: "scheme",
		},
		{
			name: "invalid target type",
			args: []string{"https://api.openai.com/v1/chat"},
			flags: map[string]string{
				"name": "bad-type-target",
				"type": "invalid_type",
			},
			wantErr:     true,
			errContains: "invalid target type",
		},
		{
			name: "invalid provider",
			args: []string{"https://api.openai.com/v1/chat"},
			flags: map[string]string{
				"name":     "bad-provider-target",
				"provider": "invalid_provider",
			},
			wantErr:     true,
			errContains: "invalid provider",
		},
		{
			name: "duplicate target name",
			args: []string{"https://api.openai.com/v1/chat"},
			flags: map[string]string{
				"name": "duplicate-target",
			},
			wantErr: false,
			validate: func(t *testing.T, db *database.DB) {
				// Create a second command to try adding duplicate
				cmd2 := &cobra.Command{
					Use:  "add",
					Args: cobra.ExactArgs(1),
					RunE: runTargetAdd,
				}

				cmd2.Flags().StringVar(&addName, "name", "", "Human-readable name for the target (required)")
				cmd2.Flags().StringVar(&addType, "type", "llm_api", "Target type")
				cmd2.Flags().StringVar(&addProvider, "provider", "", "Provider")
				cmd2.Flags().StringVar(&addCredential, "credential", "", "Credential name")
				cmd2.Flags().StringVar(&addModel, "model", "", "Model identifier")
				cmd2.Flags().IntVar(&addTimeout, "timeout", 30, "Request timeout in seconds")

				cmd2.SetArgs([]string{"https://different-url.com"})
				cmd2.Flags().Set("name", "duplicate-target")

				buf := new(bytes.Buffer)
				cmd2.SetOut(buf)
				cmd2.SetErr(buf)

				err := cmd2.ExecuteContext(context.Background())
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "already exists")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := setupTestDB(t)
			defer cleanup()

			// Create a fresh command for each test
			cmd := &cobra.Command{
				Use:  "add",
				Args: cobra.ExactArgs(1),
				RunE: runTargetAdd,
			}

			// Re-register flags
			cmd.Flags().StringVar(&addName, "name", "", "Human-readable name for the target (required)")
			cmd.Flags().StringVar(&addType, "type", "llm_api", "Target type")
			cmd.Flags().StringVar(&addProvider, "provider", "", "Provider")
			cmd.Flags().StringVar(&addCredential, "credential", "", "Credential name")
			cmd.Flags().StringVar(&addModel, "model", "", "Model identifier")
			cmd.Flags().IntVar(&addTimeout, "timeout", 30, "Request timeout in seconds")
			cmd.MarkFlagRequired("name")

			cmd.SetArgs(tt.args)

			// Set flags
			for key, value := range tt.flags {
				cmd.Flags().Set(key, value)
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.ExecuteContext(context.Background())

			// Check error expectation
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}

			// Run validation if provided
			if tt.validate != nil && !tt.wantErr {
				tt.validate(t, db)
			}
		})
	}
}

// TestTargetList tests the target list command
func TestTargetList(t *testing.T) {
	tests := []struct {
		name         string
		setupTargets []types.Target
		statusFilter string
		provFilter   string
		wantCount    int
		wantNames    []string
	}{
		{
			name: "list all targets",
			setupTargets: []types.Target{
				*types.NewTarget("target1", "https://api.openai.com/v1", types.TargetTypeLLMAPI),
				*types.NewTarget("target2", "https://api.anthropic.com/v1", types.TargetTypeLLMAPI),
			},
			wantCount: 2,
			wantNames: []string{"target1", "target2"},
		},
		{
			name: "filter by status",
			setupTargets: []types.Target{
				func() types.Target {
					t := types.NewTarget("active-target", "https://api.openai.com/v1", types.TargetTypeLLMAPI)
					t.Status = types.TargetStatusActive
					return *t
				}(),
				func() types.Target {
					t := types.NewTarget("inactive-target", "https://api.anthropic.com/v1", types.TargetTypeLLMAPI)
					t.Status = types.TargetStatusInactive
					return *t
				}(),
			},
			statusFilter: "active",
			wantCount:    1,
			wantNames:    []string{"active-target"},
		},
		{
			name: "filter by provider",
			setupTargets: []types.Target{
				func() types.Target {
					t := types.NewTarget("openai-target", "https://api.openai.com/v1", types.TargetTypeLLMAPI)
					t.Provider = types.ProviderOpenAI
					return *t
				}(),
				func() types.Target {
					t := types.NewTarget("anthropic-target", "https://api.anthropic.com/v1", types.TargetTypeLLMAPI)
					t.Provider = types.ProviderAnthropic
					return *t
				}(),
			},
			provFilter: "openai",
			wantCount:  1,
			wantNames:  []string{"openai-target"},
		},
		{
			name:         "empty list",
			setupTargets: []types.Target{},
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := setupTestDB(t)
			defer cleanup()

			// Setup targets
			dao := database.NewTargetDAO(db)
			ctx := context.Background()
			for _, target := range tt.setupTargets {
				err := dao.Create(ctx, &target)
				require.NoError(t, err)
			}

			// Create a fresh command for each test
			cmd := &cobra.Command{
				Use:  "list",
				RunE: runTargetList,
			}

			// Re-register flags
			cmd.Flags().StringVar(&listStatusFilter, "status", "", "Filter by status")
			cmd.Flags().StringVar(&listProviderFilter, "provider", "", "Filter by provider")

			cmd.SetArgs([]string{})

			if tt.statusFilter != "" {
				cmd.Flags().Set("status", tt.statusFilter)
			}
			if tt.provFilter != "" {
				cmd.Flags().Set("provider", tt.provFilter)
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.ExecuteContext(ctx)
			require.NoError(t, err)

			// Verify output
			output := buf.String()
			if tt.wantCount == 0 {
				assert.Contains(t, output, "No targets found")
			} else {
				for _, name := range tt.wantNames {
					assert.Contains(t, output, name)
				}
			}
		})
	}
}

// TestTargetShow tests the target show command
func TestTargetShow(t *testing.T) {
	tests := []struct {
		name        string
		setupTarget *types.Target
		targetName  string
		wantErr     bool
		wantOutput  []string
	}{
		{
			name: "show existing target",
			setupTarget: func() *types.Target {
				t := types.NewTarget("show-test", "https://api.openai.com/v1", types.TargetTypeLLMAPI)
				t.Provider = types.ProviderOpenAI
				t.Model = "gpt-4"
				t.Description = "Test target"
				return t
			}(),
			targetName: "show-test",
			wantErr:    false,
			wantOutput: []string{
				"show-test",
				"https://api.openai.com/v1",
				"gpt-4",
				"openai",
				"Test target",
			},
		},
		{
			name:       "show non-existent target",
			targetName: "does-not-exist",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := setupTestDB(t)
			defer cleanup()

			// Setup target if provided
			if tt.setupTarget != nil {
				dao := database.NewTargetDAO(db)
				err := dao.Create(context.Background(), tt.setupTarget)
				require.NoError(t, err)
			}

			// Create a fresh command for each test
			cmd := &cobra.Command{
				Use:  "show",
				Args: cobra.ExactArgs(1),
				RunE: runTargetShow,
			}

			cmd.SetArgs([]string{tt.targetName})

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.ExecuteContext(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				output := buf.String()
				for _, want := range tt.wantOutput {
					assert.Contains(t, output, want)
				}
			}
		})
	}
}

// TestTargetDelete tests the target delete command
func TestTargetDelete(t *testing.T) {
	tests := []struct {
		name        string
		setupTarget *types.Target
		targetName  string
		useForce    bool
		wantErr     bool
	}{
		{
			name: "delete with force flag",
			setupTarget: func() *types.Target {
				return types.NewTarget("delete-test", "https://api.openai.com/v1", types.TargetTypeLLMAPI)
			}(),
			targetName: "delete-test",
			useForce:   true,
			wantErr:    false,
		},
		{
			name:       "delete non-existent target",
			targetName: "does-not-exist",
			useForce:   true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _, cleanup := setupTestDB(t)
			defer cleanup()

			// Setup target if provided
			var targetID types.ID
			if tt.setupTarget != nil {
				dao := database.NewTargetDAO(db)
				err := dao.Create(context.Background(), tt.setupTarget)
				require.NoError(t, err)
				targetID = tt.setupTarget.ID
			}

			// Create a fresh command for each test
			cmd := &cobra.Command{
				Use:  "delete",
				Args: cobra.ExactArgs(1),
				RunE: runTargetDelete,
			}

			// Re-register flags
			cmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")

			cmd.SetArgs([]string{tt.targetName})

			if tt.useForce {
				cmd.Flags().Set("force", "true")
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err := cmd.ExecuteContext(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify target was deleted
				dao := database.NewTargetDAO(db)
				exists, err := dao.Exists(context.Background(), targetID)
				require.NoError(t, err)
				assert.False(t, exists, "target should be deleted")
			}
		})
	}
}

// TestDetectProvider tests the provider auto-detection logic
func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "OpenAI API",
			url:      "https://api.openai.com/v1/chat/completions",
			expected: "openai",
		},
		{
			name:     "Anthropic API",
			url:      "https://api.anthropic.com/v1/messages",
			expected: "anthropic",
		},
		{
			name:     "Google API",
			url:      "https://generativelanguage.googleapis.com/v1/models",
			expected: "google",
		},
		{
			name:     "Azure OpenAI",
			url:      "https://myresource.openai.azure.com/openai/deployments",
			expected: "azure",
		},
		{
			name:     "Ollama localhost",
			url:      "http://localhost:11434/api/generate",
			expected: "ollama",
		},
		{
			name:     "Custom endpoint",
			url:      "https://my-custom-llm.example.com/api",
			expected: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			result := detectProvider(u)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectModel tests the model auto-detection logic
func TestDetectModel(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		provider string
		expected string
	}{
		{
			name:     "GPT-4 in path",
			url:      "https://api.openai.com/v1/engines/gpt-4/completions",
			provider: "openai",
			expected: "gpt-4",
		},
		{
			name:     "Claude in path",
			url:      "https://api.anthropic.com/v1/models/claude-3",
			provider: "anthropic",
			expected: "claude-3-opus",
		},
		{
			name:     "Default for OpenAI",
			url:      "https://api.openai.com/v1/chat/completions",
			provider: "openai",
			expected: "gpt-4",
		},
		{
			name:     "Default for Anthropic",
			url:      "https://api.anthropic.com/v1/messages",
			provider: "anthropic",
			expected: "claude-3-opus",
		},
		{
			name:     "Unknown custom",
			url:      "https://custom.example.com/api",
			provider: "custom",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			result := detectModel(u, tt.provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetGibsonHome tests the Gibson home directory resolution
func TestGetGibsonHome(t *testing.T) {
	tests := []struct {
		name       string
		envValue   string
		wantPrefix string
	}{
		{
			name:       "use environment variable",
			envValue:   "/custom/gibson/home",
			wantPrefix: "/custom/gibson/home",
		},
		{
			name:       "use default when env not set",
			envValue:   "",
			wantPrefix: "/.gibson",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment
			oldEnv := os.Getenv("GIBSON_HOME")
			defer os.Setenv("GIBSON_HOME", oldEnv)

			// Set test environment
			if tt.envValue != "" {
				os.Setenv("GIBSON_HOME", tt.envValue)
			} else {
				os.Unsetenv("GIBSON_HOME")
			}

			// Get home directory
			home, err := getGibsonHome()
			require.NoError(t, err)

			if tt.envValue != "" {
				assert.Equal(t, tt.envValue, home)
			} else {
				assert.Contains(t, home, tt.wantPrefix)
			}
		})
	}
}
