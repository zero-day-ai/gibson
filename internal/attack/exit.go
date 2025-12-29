package attack

import "github.com/zero-day-ai/gibson/internal/agent"

// Exit codes for the attack command.
// These codes enable scripting and CI/CD integration by providing
// programmatic detection of attack outcomes.
//
// Exit Code Reference:
//
//	0  - Success (no findings discovered)
//	1  - Findings (low or medium severity findings discovered)
//	2  - Critical Findings (high or critical severity findings discovered)
//	3  - Error (execution error occurred)
//	4  - Timeout (attack exceeded timeout limit)
//	5  - Cancelled (attack was cancelled by user)
//	10 - Config Error (invalid configuration provided)
//
// Example usage in shell scripts:
//
//	gibson attack https://example.com --agent web-scanner
//	EXIT_CODE=$?
//	if [ $EXIT_CODE -eq 2 ]; then
//	    echo "Critical findings discovered!"
//	    exit 1
//	fi
const (
	// ExitSuccess indicates the attack completed successfully with no findings.
	// Use case: Target passed all security checks.
	ExitSuccess = 0

	// ExitWithFindings indicates low or medium severity findings were discovered.
	// Use case: Non-critical vulnerabilities found that should be reviewed.
	ExitWithFindings = 1

	// ExitCriticalFindings indicates high or critical severity findings were discovered.
	// Use case: Serious vulnerabilities found requiring immediate attention.
	// CI/CD pipelines should typically fail on this exit code.
	ExitCriticalFindings = 2

	// ExitError indicates an execution error occurred during the attack.
	// Use case: Network failures, agent crashes, or other runtime errors.
	// The attack may not have completed successfully.
	ExitError = 3

	// ExitTimeout indicates the attack exceeded the configured timeout.
	// Use case: Attack took too long and was terminated.
	// Partial results may be available.
	ExitTimeout = 4

	// ExitCancelled indicates the attack was cancelled by the user (SIGINT/Ctrl-C).
	// Use case: User interrupted the attack.
	// Partial results may be available.
	ExitCancelled = 5

	// ExitConfigError indicates invalid configuration was provided.
	// Use case: Missing required flags, invalid URLs, unknown agents, etc.
	// The attack was not executed due to configuration problems.
	ExitConfigError = 10
)

// ExitCodeFromResult determines the appropriate exit code based on the attack result.
// This function examines the attack status, findings, and errors to select the most
// appropriate exit code for scripting and automation purposes.
//
// Exit code priority (highest to lowest):
//  1. Configuration errors (ExitConfigError)
//  2. Cancellation (ExitCancelled)
//  3. Timeout (ExitTimeout)
//  4. Execution errors (ExitError)
//  5. Critical/High severity findings (ExitCriticalFindings)
//  6. Low/Medium severity findings (ExitWithFindings)
//  7. Success with no findings (ExitSuccess)
//
// Example:
//
//	result, err := runner.Run(ctx, opts)
//	if err != nil {
//	    // Handle error
//	    os.Exit(ExitError)
//	}
//	os.Exit(ExitCodeFromResult(result))
func ExitCodeFromResult(result *AttackResult) int {
	if result == nil {
		return ExitError
	}

	// Check status first - these override finding-based codes
	switch result.Status {
	case AttackStatusCancelled:
		return ExitCancelled
	case AttackStatusTimeout:
		return ExitTimeout
	case AttackStatusFailed:
		return ExitError
	}

	// If there was an error, return error code
	if result.Error != nil {
		return ExitError
	}

	// Check for critical or high severity findings
	if hasCriticalFindings(result) {
		return ExitCriticalFindings
	}

	// Check for any findings (low/medium/info)
	if hasAnyFindings(result) {
		return ExitWithFindings
	}

	// No findings, successful execution
	return ExitSuccess
}

// hasCriticalFindings checks if the result contains any high or critical severity findings.
func hasCriticalFindings(result *AttackResult) bool {
	if result == nil {
		return false
	}

	// Check severity summary map first (more efficient)
	if result.FindingsBySeverity != nil {
		critical := result.FindingsBySeverity[string(agent.SeverityCritical)]
		high := result.FindingsBySeverity[string(agent.SeverityHigh)]
		if critical > 0 || high > 0 {
			return true
		}
	}

	// Fallback: iterate through findings
	for _, finding := range result.Findings {
		if finding.Severity == agent.SeverityCritical || finding.Severity == agent.SeverityHigh {
			return true
		}
	}

	return false
}

// hasAnyFindings checks if the result contains any findings of any severity.
func hasAnyFindings(result *AttackResult) bool {
	if result == nil {
		return false
	}

	// Check findings slice first
	if len(result.Findings) > 0 {
		return true
	}

	// Check severity summary map for any non-zero counts
	if result.FindingsBySeverity != nil {
		for _, count := range result.FindingsBySeverity {
			if count > 0 {
				return true
			}
		}
	}

	return false
}
