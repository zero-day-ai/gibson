package main

import (
	"testing"

	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
)

func TestGlobalFlags_VerbosityLevel(t *testing.T) {
	tests := []struct {
		name          string
		verbose       bool
		veryVerbose   bool
		debugVerbose  bool
		quiet         bool
		expectedLevel internal.VerboseLevel
	}{
		{
			name:          "All flags false returns LevelNone",
			verbose:       false,
			veryVerbose:   false,
			debugVerbose:  false,
			quiet:         false,
			expectedLevel: internal.LevelNone,
		},
		{
			name:          "Quiet mode returns LevelNone",
			verbose:       true,
			veryVerbose:   true,
			debugVerbose:  true,
			quiet:         true,
			expectedLevel: internal.LevelNone,
		},
		{
			name:          "Verbose returns LevelBasic",
			verbose:       true,
			veryVerbose:   false,
			debugVerbose:  false,
			quiet:         false,
			expectedLevel: internal.LevelBasic,
		},
		{
			name:          "VeryVerbose returns LevelVerbose",
			verbose:       false,
			veryVerbose:   true,
			debugVerbose:  false,
			quiet:         false,
			expectedLevel: internal.LevelVerbose,
		},
		{
			name:          "DebugVerbose returns LevelDebug",
			verbose:       false,
			veryVerbose:   false,
			debugVerbose:  true,
			quiet:         false,
			expectedLevel: internal.LevelDebug,
		},
		{
			name:          "DebugVerbose takes precedence over VeryVerbose",
			verbose:       true,
			veryVerbose:   true,
			debugVerbose:  true,
			quiet:         false,
			expectedLevel: internal.LevelDebug,
		},
		{
			name:          "VeryVerbose takes precedence over Verbose",
			verbose:       true,
			veryVerbose:   true,
			debugVerbose:  false,
			quiet:         false,
			expectedLevel: internal.LevelVerbose,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := &GlobalFlags{
				Verbose:      tt.verbose,
				VeryVerbose:  tt.veryVerbose,
				DebugVerbose: tt.debugVerbose,
				Quiet:        tt.quiet,
			}

			level := flags.VerbosityLevel()
			if level != tt.expectedLevel {
				t.Errorf("Expected level %v, got %v", tt.expectedLevel, level)
			}
		})
	}
}

func TestGlobalFlags_IsVerbose(t *testing.T) {
	tests := []struct {
		name     string
		verbose  bool
		quiet    bool
		expected bool
	}{
		{
			name:     "Verbose without quiet returns true",
			verbose:  true,
			quiet:    false,
			expected: true,
		},
		{
			name:     "Verbose with quiet returns false",
			verbose:  true,
			quiet:    true,
			expected: false,
		},
		{
			name:     "Not verbose returns false",
			verbose:  false,
			quiet:    false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := &GlobalFlags{
				Verbose: tt.verbose,
				Quiet:   tt.quiet,
			}

			result := flags.IsVerbose()
			if result != tt.expected {
				t.Errorf("Expected IsVerbose()=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGlobalFlags_IsQuiet(t *testing.T) {
	flags := &GlobalFlags{Quiet: true}
	if !flags.IsQuiet() {
		t.Error("Expected IsQuiet() to return true")
	}

	flags = &GlobalFlags{Quiet: false}
	if flags.IsQuiet() {
		t.Error("Expected IsQuiet() to return false")
	}
}
