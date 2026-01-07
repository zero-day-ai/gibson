package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvalOptions(t *testing.T) {
	opts := NewEvalOptions()

	assert.False(t, opts.Enabled, "Evaluation should be disabled by default")
	assert.False(t, opts.FeedbackEnabled, "Feedback should be disabled by default")
	assert.Empty(t, opts.OutputPath, "OutputPath should be empty by default")
	assert.Empty(t, opts.ConfigPath, "ConfigPath should be empty by default")
	assert.Empty(t, opts.Scorers, "Scorers should be empty by default")
	assert.Empty(t, opts.GroundTruthPath, "GroundTruthPath should be empty by default")
	assert.Empty(t, opts.ExpectedToolsPath, "ExpectedToolsPath should be empty by default")
	assert.Equal(t, 0.5, opts.WarningThreshold, "WarningThreshold should default to 0.5")
	assert.Equal(t, 0.2, opts.CriticalThreshold, "CriticalThreshold should default to 0.2")
	assert.False(t, opts.ExportLangfuse, "ExportLangfuse should be disabled by default")
	assert.False(t, opts.ExportOTel, "ExportOTel should be disabled by default")
}

func TestEvalOptions_ApplyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		opts     *EvalOptions
		expected *EvalOptions
	}{
		{
			name: "apply defaults to zero values",
			opts: &EvalOptions{
				WarningThreshold:  0.0,
				CriticalThreshold: 0.0,
			},
			expected: &EvalOptions{
				WarningThreshold:  0.5,
				CriticalThreshold: 0.2,
			},
		},
		{
			name: "preserve non-zero values",
			opts: &EvalOptions{
				WarningThreshold:  0.7,
				CriticalThreshold: 0.3,
			},
			expected: &EvalOptions{
				WarningThreshold:  0.7,
				CriticalThreshold: 0.3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ApplyDefaults()
			assert.Equal(t, tt.expected.WarningThreshold, tt.opts.WarningThreshold)
			assert.Equal(t, tt.expected.CriticalThreshold, tt.opts.CriticalThreshold)
		})
	}
}

func TestEvalOptions_Validate_Disabled(t *testing.T) {
	// When evaluation is disabled, validation should always pass
	opts := &EvalOptions{
		Enabled: false,
		// Invalid values that would normally fail
		WarningThreshold:  2.0,
		CriticalThreshold: -1.0,
		GroundTruthPath:   "",
	}

	err := opts.Validate()
	assert.NoError(t, err, "Validation should pass when evaluation is disabled")
}

func TestEvalOptions_Validate_Thresholds(t *testing.T) {
	tmpDir := t.TempDir()
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	require.NoError(t, os.WriteFile(groundTruthPath, []byte("{}"), 0644))

	tests := []struct {
		name              string
		warningThreshold  float64
		criticalThreshold float64
		expectError       bool
		errorContains     string
	}{
		{
			name:              "valid thresholds",
			warningThreshold:  0.5,
			criticalThreshold: 0.2,
			expectError:       false,
		},
		{
			name:              "equal thresholds are valid",
			warningThreshold:  0.5,
			criticalThreshold: 0.5,
			expectError:       false,
		},
		{
			name:              "warning threshold too high",
			warningThreshold:  1.5,
			criticalThreshold: 0.2,
			expectError:       true,
			errorContains:     "warning_threshold must be between 0.0 and 1.0",
		},
		{
			name:              "warning threshold too low",
			warningThreshold:  -0.1,
			criticalThreshold: 0.2,
			expectError:       true,
			errorContains:     "warning_threshold must be between 0.0 and 1.0",
		},
		{
			name:              "critical threshold too high",
			warningThreshold:  0.5,
			criticalThreshold: 1.5,
			expectError:       true,
			errorContains:     "critical_threshold must be between 0.0 and 1.0",
		},
		{
			name:              "critical threshold too low",
			warningThreshold:  0.5,
			criticalThreshold: -0.1,
			expectError:       true,
			errorContains:     "critical_threshold must be between 0.0 and 1.0",
		},
		{
			name:              "critical threshold greater than warning",
			warningThreshold:  0.3,
			criticalThreshold: 0.5,
			expectError:       true,
			errorContains:     "critical_threshold",
		},
		{
			name:              "boundary values - both 0.0",
			warningThreshold:  0.0,
			criticalThreshold: 0.0,
			expectError:       false,
		},
		{
			name:              "boundary values - both 1.0",
			warningThreshold:  1.0,
			criticalThreshold: 1.0,
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &EvalOptions{
				Enabled:           true,
				WarningThreshold:  tt.warningThreshold,
				CriticalThreshold: tt.criticalThreshold,
				GroundTruthPath:   groundTruthPath,
			}

			err := opts.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvalOptions_Validate_GroundTruthPath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		setupFunc     func() string
		expectError   bool
		errorContains string
	}{
		{
			name: "missing ground truth path",
			setupFunc: func() string {
				return ""
			},
			expectError:   true,
			errorContains: "ground_truth_path is required",
		},
		{
			name: "valid ground truth file",
			setupFunc: func() string {
				path := filepath.Join(tmpDir, "valid.json")
				require.NoError(t, os.WriteFile(path, []byte("{}"), 0644))
				return path
			},
			expectError: false,
		},
		{
			name: "ground truth file does not exist",
			setupFunc: func() string {
				return filepath.Join(tmpDir, "nonexistent.json")
			},
			expectError:   true,
			errorContains: "does not exist",
		},
		{
			name: "ground truth path is a directory",
			setupFunc: func() string {
				dirPath := filepath.Join(tmpDir, "gt_dir")
				require.NoError(t, os.Mkdir(dirPath, 0755))
				return dirPath
			},
			expectError:   true,
			errorContains: "must be a file, not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &EvalOptions{
				Enabled:           true,
				WarningThreshold:  0.5,
				CriticalThreshold: 0.2,
				GroundTruthPath:   tt.setupFunc(),
			}

			err := opts.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvalOptions_Validate_ExpectedToolsPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid ground truth file
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	require.NoError(t, os.WriteFile(groundTruthPath, []byte("{}"), 0644))

	tests := []struct {
		name          string
		setupFunc     func() string
		expectError   bool
		errorContains string
	}{
		{
			name: "empty expected tools path is valid",
			setupFunc: func() string {
				return ""
			},
			expectError: false,
		},
		{
			name: "valid expected tools file",
			setupFunc: func() string {
				path := filepath.Join(tmpDir, "tools.json")
				require.NoError(t, os.WriteFile(path, []byte("{}"), 0644))
				return path
			},
			expectError: false,
		},
		{
			name: "expected tools file does not exist",
			setupFunc: func() string {
				return filepath.Join(tmpDir, "missing_tools.json")
			},
			expectError:   true,
			errorContains: "does not exist",
		},
		{
			name: "expected tools path is a directory",
			setupFunc: func() string {
				dirPath := filepath.Join(tmpDir, "tools_dir")
				require.NoError(t, os.Mkdir(dirPath, 0755))
				return dirPath
			},
			expectError:   true,
			errorContains: "must be a file, not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &EvalOptions{
				Enabled:           true,
				WarningThreshold:  0.5,
				CriticalThreshold: 0.2,
				GroundTruthPath:   groundTruthPath,
				ExpectedToolsPath: tt.setupFunc(),
			}

			err := opts.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvalOptions_Validate_ConfigPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid ground truth file
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	require.NoError(t, os.WriteFile(groundTruthPath, []byte("{}"), 0644))

	tests := []struct {
		name          string
		setupFunc     func() string
		expectError   bool
		errorContains string
	}{
		{
			name: "empty config path is valid",
			setupFunc: func() string {
				return ""
			},
			expectError: false,
		},
		{
			name: "valid config file",
			setupFunc: func() string {
				path := filepath.Join(tmpDir, "config.yaml")
				require.NoError(t, os.WriteFile(path, []byte("{}"), 0644))
				return path
			},
			expectError: false,
		},
		{
			name: "config file does not exist",
			setupFunc: func() string {
				return filepath.Join(tmpDir, "missing_config.yaml")
			},
			expectError:   true,
			errorContains: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &EvalOptions{
				Enabled:           true,
				WarningThreshold:  0.5,
				CriticalThreshold: 0.2,
				GroundTruthPath:   groundTruthPath,
				ConfigPath:        tt.setupFunc(),
			}

			err := opts.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvalOptions_Validate_OutputPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid ground truth file
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	require.NoError(t, os.WriteFile(groundTruthPath, []byte("{}"), 0644))

	tests := []struct {
		name          string
		setupFunc     func() string
		expectError   bool
		errorContains string
	}{
		{
			name: "empty output path is valid",
			setupFunc: func() string {
				return ""
			},
			expectError: false,
		},
		{
			name: "valid output path in existing directory",
			setupFunc: func() string {
				return filepath.Join(tmpDir, "results.json")
			},
			expectError: false,
		},
		{
			name: "output path in nonexistent directory",
			setupFunc: func() string {
				return filepath.Join(tmpDir, "nonexistent", "results.json")
			},
			expectError:   true,
			errorContains: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &EvalOptions{
				Enabled:           true,
				WarningThreshold:  0.5,
				CriticalThreshold: 0.2,
				GroundTruthPath:   groundTruthPath,
				OutputPath:        tt.setupFunc(),
			}

			err := opts.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvalOptions_Validate_Scorers(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid ground truth file
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	require.NoError(t, os.WriteFile(groundTruthPath, []byte("{}"), 0644))

	tests := []struct {
		name          string
		scorers       []string
		expectError   bool
		errorContains string
	}{
		{
			name:        "empty scorers is valid",
			scorers:     []string{},
			expectError: false,
		},
		{
			name:        "single valid scorer",
			scorers:     []string{"exact_match"},
			expectError: false,
		},
		{
			name:        "multiple valid scorers",
			scorers:     []string{"exact_match", "semantic_similarity", "tool_correctness"},
			expectError: false,
		},
		{
			name:        "custom scorer",
			scorers:     []string{"custom"},
			expectError: false,
		},
		{
			name:          "invalid scorer",
			scorers:       []string{"invalid_scorer"},
			expectError:   true,
			errorContains: "invalid scorer 'invalid_scorer'",
		},
		{
			name:          "mix of valid and invalid scorers",
			scorers:       []string{"exact_match", "invalid_scorer"},
			expectError:   true,
			errorContains: "invalid scorer 'invalid_scorer'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &EvalOptions{
				Enabled:           true,
				WarningThreshold:  0.5,
				CriticalThreshold: 0.2,
				GroundTruthPath:   groundTruthPath,
				Scorers:           tt.scorers,
			}

			err := opts.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvalOptions_Validate_CompleteConfiguration(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup all required files
	groundTruthPath := filepath.Join(tmpDir, "ground_truth.json")
	require.NoError(t, os.WriteFile(groundTruthPath, []byte(`{"task1": "expected output"}`), 0644))

	expectedToolsPath := filepath.Join(tmpDir, "tools.json")
	require.NoError(t, os.WriteFile(expectedToolsPath, []byte(`{"task1": []}`), 0644))

	configPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("scorers: []"), 0644))

	outputPath := filepath.Join(tmpDir, "results.json")

	// Test complete valid configuration
	opts := &EvalOptions{
		Enabled:           true,
		FeedbackEnabled:   true,
		OutputPath:        outputPath,
		ConfigPath:        configPath,
		Scorers:           []string{"exact_match", "semantic_similarity"},
		GroundTruthPath:   groundTruthPath,
		ExpectedToolsPath: expectedToolsPath,
		WarningThreshold:  0.6,
		CriticalThreshold: 0.3,
		ExportLangfuse:    true,
		ExportOTel:        true,
	}

	err := opts.Validate()
	assert.NoError(t, err, "Complete valid configuration should pass validation")
}

func TestEvalOptions_Validate_MultipleErrors(t *testing.T) {
	// Test that validation returns the first encountered error
	opts := &EvalOptions{
		Enabled:           true,
		WarningThreshold:  2.0,  // Invalid
		CriticalThreshold: -1.0, // Invalid
		GroundTruthPath:   "",   // Missing
	}

	err := opts.Validate()
	require.Error(t, err)
	// Should fail on first check (warning threshold)
	assert.Contains(t, err.Error(), "warning_threshold")
}
