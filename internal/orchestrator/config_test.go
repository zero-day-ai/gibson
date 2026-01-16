package orchestrator

import (
	"os"
	"testing"
)

// TestRunMode_ShouldFailFast tests the ShouldFailFast behavior for each mode
func TestRunMode_ShouldFailFast(t *testing.T) {
	tests := []struct {
		name     string
		mode     RunMode
		expected bool
	}{
		{
			name:     "production mode does not fail fast",
			mode:     RunModeProduction,
			expected: false,
		},
		{
			name:     "development mode fails fast",
			mode:     RunModeDevelopment,
			expected: true,
		},
		{
			name:     "debug mode fails fast",
			mode:     RunModeDebug,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mode.ShouldFailFast()
			if result != tt.expected {
				t.Errorf("RunMode(%s).ShouldFailFast() = %v, want %v", tt.mode, result, tt.expected)
			}
		})
	}
}

// TestRunMode_ShouldLogVerbose tests the ShouldLogVerbose behavior for each mode
func TestRunMode_ShouldLogVerbose(t *testing.T) {
	tests := []struct {
		name     string
		mode     RunMode
		expected bool
	}{
		{
			name:     "production mode does not log verbose",
			mode:     RunModeProduction,
			expected: false,
		},
		{
			name:     "development mode does not log verbose",
			mode:     RunModeDevelopment,
			expected: false,
		},
		{
			name:     "debug mode logs verbose",
			mode:     RunModeDebug,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mode.ShouldLogVerbose()
			if result != tt.expected {
				t.Errorf("RunMode(%s).ShouldLogVerbose() = %v, want %v", tt.mode, result, tt.expected)
			}
		})
	}
}

// TestRunMode_ShouldStopOnSuspect tests the ShouldStopOnSuspect behavior for each mode
func TestRunMode_ShouldStopOnSuspect(t *testing.T) {
	tests := []struct {
		name     string
		mode     RunMode
		expected bool
	}{
		{
			name:     "production mode does not stop on suspect",
			mode:     RunModeProduction,
			expected: false,
		},
		{
			name:     "development mode does not stop on suspect",
			mode:     RunModeDevelopment,
			expected: false,
		},
		{
			name:     "debug mode stops on suspect",
			mode:     RunModeDebug,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.mode.ShouldStopOnSuspect()
			if result != tt.expected {
				t.Errorf("RunMode(%s).ShouldStopOnSuspect() = %v, want %v", tt.mode, result, tt.expected)
			}
		})
	}
}

// TestGetRunModeFromEnv tests reading run mode from environment variable
func TestGetRunModeFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected RunMode
	}{
		{
			name:     "production (full)",
			envValue: "production",
			expected: RunModeProduction,
		},
		{
			name:     "production (short)",
			envValue: "prod",
			expected: RunModeProduction,
		},
		{
			name:     "production (uppercase)",
			envValue: "PRODUCTION",
			expected: RunModeProduction,
		},
		{
			name:     "production (mixed case)",
			envValue: "PrOdUcTiOn",
			expected: RunModeProduction,
		},
		{
			name:     "development (full)",
			envValue: "development",
			expected: RunModeDevelopment,
		},
		{
			name:     "development (short)",
			envValue: "dev",
			expected: RunModeDevelopment,
		},
		{
			name:     "development (uppercase)",
			envValue: "DEVELOPMENT",
			expected: RunModeDevelopment,
		},
		{
			name:     "debug",
			envValue: "debug",
			expected: RunModeDebug,
		},
		{
			name:     "debug (uppercase)",
			envValue: "DEBUG",
			expected: RunModeDebug,
		},
		{
			name:     "empty string defaults to production",
			envValue: "",
			expected: RunModeProduction,
		},
		{
			name:     "whitespace defaults to production",
			envValue: "   ",
			expected: RunModeProduction,
		},
		{
			name:     "invalid value defaults to production",
			envValue: "invalid",
			expected: RunModeProduction,
		},
		{
			name:     "whitespace with value is trimmed",
			envValue: "  debug  ",
			expected: RunModeDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment variable
			originalValue := os.Getenv("GIBSON_RUN_MODE")
			defer func() {
				if originalValue != "" {
					os.Setenv("GIBSON_RUN_MODE", originalValue)
				} else {
					os.Unsetenv("GIBSON_RUN_MODE")
				}
			}()

			// Set test value
			if tt.envValue == "" {
				os.Unsetenv("GIBSON_RUN_MODE")
			} else {
				os.Setenv("GIBSON_RUN_MODE", tt.envValue)
			}

			// Test
			result := GetRunModeFromEnv()
			if result != tt.expected {
				t.Errorf("GetRunModeFromEnv() with GIBSON_RUN_MODE=%q = %v, want %v", tt.envValue, result, tt.expected)
			}
		})
	}
}

// TestWithRunMode tests the WithRunMode option function
func TestWithRunMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     RunMode
		expected RunMode
	}{
		{
			name:     "set production mode",
			mode:     RunModeProduction,
			expected: RunModeProduction,
		},
		{
			name:     "set development mode",
			mode:     RunModeDevelopment,
			expected: RunModeDevelopment,
		},
		{
			name:     "set debug mode",
			mode:     RunModeDebug,
			expected: RunModeDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create orchestrator with run mode option
			o := &Orchestrator{}
			opt := WithRunMode(tt.mode)
			opt(o)

			if o.runMode != tt.expected {
				t.Errorf("WithRunMode(%v) set runMode to %v, want %v", tt.mode, o.runMode, tt.expected)
			}
		})
	}
}

// TestNewOrchestrator_DefaultRunMode tests that orchestrator defaults to production mode
func TestNewOrchestrator_DefaultRunMode(t *testing.T) {
	// Clear environment variable
	originalValue := os.Getenv("GIBSON_RUN_MODE")
	os.Unsetenv("GIBSON_RUN_MODE")
	defer func() {
		if originalValue != "" {
			os.Setenv("GIBSON_RUN_MODE", originalValue)
		}
	}()

	o := NewOrchestrator(nil, nil, nil)

	if o.runMode != RunModeProduction {
		t.Errorf("NewOrchestrator() with no options set runMode to %v, want %v", o.runMode, RunModeProduction)
	}
}

// TestNewOrchestrator_EnvRunMode tests that orchestrator reads from environment
func TestNewOrchestrator_EnvRunMode(t *testing.T) {
	// Save original environment
	originalValue := os.Getenv("GIBSON_RUN_MODE")
	defer func() {
		if originalValue != "" {
			os.Setenv("GIBSON_RUN_MODE", originalValue)
		} else {
			os.Unsetenv("GIBSON_RUN_MODE")
		}
	}()

	tests := []struct {
		name     string
		envValue string
		expected RunMode
	}{
		{
			name:     "environment sets debug mode",
			envValue: "debug",
			expected: RunModeDebug,
		},
		{
			name:     "environment sets development mode",
			envValue: "development",
			expected: RunModeDevelopment,
		},
		{
			name:     "environment sets production mode",
			envValue: "production",
			expected: RunModeProduction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GIBSON_RUN_MODE", tt.envValue)

			o := NewOrchestrator(nil, nil, nil)

			if o.runMode != tt.expected {
				t.Errorf("NewOrchestrator() with GIBSON_RUN_MODE=%q set runMode to %v, want %v",
					tt.envValue, o.runMode, tt.expected)
			}
		})
	}
}

// TestNewOrchestrator_OptionOverridesEnv tests that explicit option overrides environment
func TestNewOrchestrator_OptionOverridesEnv(t *testing.T) {
	// Set environment to debug
	originalValue := os.Getenv("GIBSON_RUN_MODE")
	os.Setenv("GIBSON_RUN_MODE", "debug")
	defer func() {
		if originalValue != "" {
			os.Setenv("GIBSON_RUN_MODE", originalValue)
		} else {
			os.Unsetenv("GIBSON_RUN_MODE")
		}
	}()

	// Create orchestrator with explicit production mode option
	o := NewOrchestrator(nil, nil, nil, WithRunMode(RunModeProduction))

	if o.runMode != RunModeProduction {
		t.Errorf("NewOrchestrator() with WithRunMode(Production) and GIBSON_RUN_MODE=debug set runMode to %v, want %v",
			o.runMode, RunModeProduction)
	}
}

// TestOrchestrator_RunMode tests the RunMode() getter method
func TestOrchestrator_RunMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     RunMode
		expected RunMode
	}{
		{
			name:     "get production mode",
			mode:     RunModeProduction,
			expected: RunModeProduction,
		},
		{
			name:     "get development mode",
			mode:     RunModeDevelopment,
			expected: RunModeDevelopment,
		},
		{
			name:     "get debug mode",
			mode:     RunModeDebug,
			expected: RunModeDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewOrchestrator(nil, nil, nil, WithRunMode(tt.mode))

			result := o.RunMode()
			if result != tt.expected {
				t.Errorf("Orchestrator.RunMode() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestRunModeBehaviorMatrix tests the complete behavior matrix for all modes
func TestRunModeBehaviorMatrix(t *testing.T) {
	tests := []struct {
		mode              RunMode
		shouldFailFast    bool
		shouldLogVerbose  bool
		shouldStopSuspect bool
	}{
		{
			mode:              RunModeProduction,
			shouldFailFast:    false,
			shouldLogVerbose:  false,
			shouldStopSuspect: false,
		},
		{
			mode:              RunModeDevelopment,
			shouldFailFast:    true,
			shouldLogVerbose:  false,
			shouldStopSuspect: false,
		},
		{
			mode:              RunModeDebug,
			shouldFailFast:    true,
			shouldLogVerbose:  true,
			shouldStopSuspect: true,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.ShouldFailFast(); got != tt.shouldFailFast {
				t.Errorf("%s.ShouldFailFast() = %v, want %v", tt.mode, got, tt.shouldFailFast)
			}
			if got := tt.mode.ShouldLogVerbose(); got != tt.shouldLogVerbose {
				t.Errorf("%s.ShouldLogVerbose() = %v, want %v", tt.mode, got, tt.shouldLogVerbose)
			}
			if got := tt.mode.ShouldStopOnSuspect(); got != tt.shouldStopSuspect {
				t.Errorf("%s.ShouldStopOnSuspect() = %v, want %v", tt.mode, got, tt.shouldStopSuspect)
			}
		})
	}
}
