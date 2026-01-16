package orchestrator

import (
	"os"
	"strings"
)

// RunMode controls error handling behavior in the orchestrator.
// Different modes enable different levels of fault tolerance and debugging.
type RunMode string

const (
	// RunModeProduction is the default mode - adaptive error handling,
	// continues on recoverable errors, uses recovery hints from registry.
	// This mode prioritizes resilience and completion over strict correctness.
	RunModeProduction RunMode = "production"

	// RunModeDevelopment enables fail-fast behavior for development.
	// Stops on first error to facilitate debugging and rapid iteration.
	RunModeDevelopment RunMode = "development"

	// RunModeDebug enables extremely verbose logging and stops on any anomaly.
	// Even suspect results (low confidence but not errors) will halt execution.
	// This mode is for deep debugging and troubleshooting.
	RunModeDebug RunMode = "debug"
)

// ShouldFailFast returns true if errors should immediately halt execution.
// In development and debug modes, we fail fast to surface issues quickly.
// In production mode, we attempt recovery and continuation.
func (m RunMode) ShouldFailFast() bool {
	return m == RunModeDevelopment || m == RunModeDebug
}

// ShouldLogVerbose returns true if verbose error logging is enabled.
// Only debug mode enables verbose logging to avoid log spam in production.
func (m RunMode) ShouldLogVerbose() bool {
	return m == RunModeDebug
}

// ShouldStopOnSuspect returns true if suspect results should halt execution.
// Suspect results are those with low confidence but no explicit error.
// Only debug mode halts on suspect results for investigation.
func (m RunMode) ShouldStopOnSuspect() bool {
	return m == RunModeDebug
}

// GetRunModeFromEnv reads the GIBSON_RUN_MODE environment variable
// and returns the corresponding RunMode. If the environment variable
// is not set or contains an invalid value, returns RunModeProduction.
//
// Valid values (case-insensitive):
//   - "production", "prod" -> RunModeProduction
//   - "development", "dev" -> RunModeDevelopment
//   - "debug" -> RunModeDebug
func GetRunModeFromEnv() RunMode {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("GIBSON_RUN_MODE")))

	switch value {
	case "production", "prod":
		return RunModeProduction
	case "development", "dev":
		return RunModeDevelopment
	case "debug":
		return RunModeDebug
	default:
		// Default to production mode (most lenient)
		return RunModeProduction
	}
}

// WithRunMode sets the run mode for the orchestrator.
// This controls error handling behavior:
//   - Production: Adaptive, continues on recoverable errors (default)
//   - Development: Fail fast on first error
//   - Debug: Extremely verbose, stops on any anomaly
//
// Example:
//
//	o := NewOrchestrator(observer, thinker, actor,
//	    WithRunMode(RunModeDevelopment),
//	)
func WithRunMode(mode RunMode) OrchestratorOption {
	return func(o *Orchestrator) {
		o.runMode = mode
	}
}
