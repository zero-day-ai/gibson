package attack

import (
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// AttackOptions contains all configuration for an attack execution.
// It supports both direct URL-based attacks and saved target lookups,
// with comprehensive control over agent behavior, payload filtering,
// execution constraints, and output formatting.
type AttackOptions struct {
	// Target configuration
	TargetURL      string               // Direct URL to attack (e.g., "https://api.example.com")
	TargetName     string               // Saved target lookup name (alternative to URL)
	TargetType     types.TargetType     // Type of target (llm_chat, llm_api, rag, etc.)
	TargetProvider string               // Provider name (openai, anthropic, etc.)
	TargetHeaders  map[string]string    // Custom HTTP headers for requests
	Credential     string               // Credential ID or name for authentication

	// Agent configuration
	AgentName      string        // Required: Agent to execute (e.g., "prompt-injection")
	Goal           string        // Optional goal/objective for the agent
	MaxTurns       int           // Maximum agent turns (0 = use agent default)
	Timeout        time.Duration // Attack timeout (0 = no timeout)

	// Payload filtering
	PayloadIDs     []string // Filter to specific payload IDs
	PayloadCategory string   // Filter by payload category (e.g., "injection")
	Techniques     []string // Filter by MITRE technique IDs (e.g., ["T1059"])

	// Execution constraints
	MaxFindings       int    // Stop after N findings (0 = no limit)
	SeverityThreshold string // Minimum severity to report (low, medium, high, critical)
	RateLimit         int    // Maximum requests per second (0 = no limit)

	// Network options
	FollowRedirects bool   // Follow HTTP redirects (default: true)
	InsecureTLS     bool   // Skip TLS certificate verification
	ProxyURL        string // HTTP/HTTPS proxy URL

	// Persistence options
	Persist       bool // Always persist mission and findings
	NoPersist     bool // Never persist, even with findings (overrides auto-persist)

	// Output options
	OutputFormat  string // Output format: text, json, sarif
	Verbose       bool   // Enable verbose output
	Quiet         bool   // Suppress non-essential output
	DryRun        bool   // Validate configuration without executing
}

// AttackOption is a functional option for configuring AttackOptions.
type AttackOption func(*AttackOptions)

// NewAttackOptions creates a new AttackOptions with default values.
func NewAttackOptions() *AttackOptions {
	return &AttackOptions{
		TargetHeaders:   make(map[string]string),
		PayloadIDs:      []string{},
		Techniques:      []string{},
		FollowRedirects: true,
		OutputFormat:    "text",
		MaxTurns:        0,    // Use agent default
		Timeout:         0,    // No timeout
		MaxFindings:     0,    // No limit
		RateLimit:       0,    // No limit
	}
}

// Validate checks that options are valid and internally consistent.
// It returns an error if:
// - AgentName is not specified
// - Neither TargetURL nor TargetName is specified
// - Both Persist and NoPersist are set
// - Both Verbose and Quiet are set
// - Invalid values are provided for enums or constraints
func (o *AttackOptions) Validate() error {
	// Agent is required
	if strings.TrimSpace(o.AgentName) == "" {
		return fmt.Errorf("agent name is required (use --agent flag)")
	}

	// Target is required (either URL or name)
	if strings.TrimSpace(o.TargetURL) == "" && strings.TrimSpace(o.TargetName) == "" {
		return fmt.Errorf("target is required (provide URL or use --target-name)")
	}

	// Cannot specify both URL and name
	if strings.TrimSpace(o.TargetURL) != "" && strings.TrimSpace(o.TargetName) != "" {
		return fmt.Errorf("cannot specify both target URL and target name")
	}

	// Validate target type if specified
	if o.TargetType != "" && !o.TargetType.IsValid() {
		return fmt.Errorf("invalid target type: %s", o.TargetType)
	}

	// Validate provider if specified
	if o.TargetProvider != "" {
		provider := types.Provider(o.TargetProvider)
		if !provider.IsValid() {
			return fmt.Errorf("invalid provider: %s", o.TargetProvider)
		}
	}

	// Cannot set both persist flags
	if o.Persist && o.NoPersist {
		return fmt.Errorf("cannot specify both --persist and --no-persist")
	}

	// Cannot set both verbose and quiet
	if o.Verbose && o.Quiet {
		return fmt.Errorf("cannot specify both --verbose and --quiet")
	}

	// Validate output format (empty defaults to text)
	switch o.OutputFormat {
	case "", "text", "json", "sarif":
		// Valid formats (empty defaults to text)
		if o.OutputFormat == "" {
			o.OutputFormat = "text"
		}
	default:
		return fmt.Errorf("invalid output format: %s (must be text, json, or sarif)", o.OutputFormat)
	}

	// Validate severity threshold if specified
	if o.SeverityThreshold != "" {
		switch strings.ToLower(o.SeverityThreshold) {
		case "low", "medium", "high", "critical":
			// Valid severities
			o.SeverityThreshold = strings.ToLower(o.SeverityThreshold)
		default:
			return fmt.Errorf("invalid severity threshold: %s (must be low, medium, high, or critical)", o.SeverityThreshold)
		}
	}

	// Validate numeric constraints
	if o.MaxTurns < 0 {
		return fmt.Errorf("max-turns cannot be negative")
	}

	if o.MaxFindings < 0 {
		return fmt.Errorf("max-findings cannot be negative")
	}

	if o.RateLimit < 0 {
		return fmt.Errorf("rate-limit cannot be negative")
	}

	if o.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}

	return nil
}

// Functional options for configuration

// WithTargetURL sets the target URL.
func WithTargetURL(url string) AttackOption {
	return func(o *AttackOptions) {
		o.TargetURL = url
	}
}

// WithTargetName sets the saved target name.
func WithTargetName(name string) AttackOption {
	return func(o *AttackOptions) {
		o.TargetName = name
	}
}

// WithTargetType sets the target type.
func WithTargetType(targetType types.TargetType) AttackOption {
	return func(o *AttackOptions) {
		o.TargetType = targetType
	}
}

// WithTargetProvider sets the target provider.
func WithTargetProvider(provider string) AttackOption {
	return func(o *AttackOptions) {
		o.TargetProvider = provider
	}
}

// WithTargetHeaders sets custom HTTP headers.
func WithTargetHeaders(headers map[string]string) AttackOption {
	return func(o *AttackOptions) {
		o.TargetHeaders = headers
	}
}

// WithCredential sets the credential identifier.
func WithCredential(credential string) AttackOption {
	return func(o *AttackOptions) {
		o.Credential = credential
	}
}

// WithAgentName sets the agent name.
func WithAgentName(name string) AttackOption {
	return func(o *AttackOptions) {
		o.AgentName = name
	}
}

// WithGoal sets the agent goal/objective.
func WithGoal(goal string) AttackOption {
	return func(o *AttackOptions) {
		o.Goal = goal
	}
}

// WithMaxTurns sets the maximum number of agent turns.
func WithMaxTurns(turns int) AttackOption {
	return func(o *AttackOptions) {
		o.MaxTurns = turns
	}
}

// WithTimeout sets the attack timeout.
func WithTimeout(timeout time.Duration) AttackOption {
	return func(o *AttackOptions) {
		o.Timeout = timeout
	}
}

// WithPayloadIDs sets the payload ID filter.
func WithPayloadIDs(ids []string) AttackOption {
	return func(o *AttackOptions) {
		o.PayloadIDs = ids
	}
}

// WithPayloadCategory sets the payload category filter.
func WithPayloadCategory(category string) AttackOption {
	return func(o *AttackOptions) {
		o.PayloadCategory = category
	}
}

// WithTechniques sets the MITRE technique filter.
func WithTechniques(techniques []string) AttackOption {
	return func(o *AttackOptions) {
		o.Techniques = techniques
	}
}

// WithMaxFindings sets the maximum number of findings before stopping.
func WithMaxFindings(max int) AttackOption {
	return func(o *AttackOptions) {
		o.MaxFindings = max
	}
}

// WithSeverityThreshold sets the minimum severity to report.
func WithSeverityThreshold(severity string) AttackOption {
	return func(o *AttackOptions) {
		o.SeverityThreshold = severity
	}
}

// WithRateLimit sets the request rate limit.
func WithRateLimit(limit int) AttackOption {
	return func(o *AttackOptions) {
		o.RateLimit = limit
	}
}

// WithFollowRedirects enables or disables following HTTP redirects.
func WithFollowRedirects(follow bool) AttackOption {
	return func(o *AttackOptions) {
		o.FollowRedirects = follow
	}
}

// WithInsecureTLS enables or disables TLS certificate verification.
func WithInsecureTLS(insecure bool) AttackOption {
	return func(o *AttackOptions) {
		o.InsecureTLS = insecure
	}
}

// WithProxyURL sets the proxy URL.
func WithProxyURL(url string) AttackOption {
	return func(o *AttackOptions) {
		o.ProxyURL = url
	}
}

// WithPersist sets the persist flag.
func WithPersist(persist bool) AttackOption {
	return func(o *AttackOptions) {
		o.Persist = persist
	}
}

// WithNoPersist sets the no-persist flag.
func WithNoPersist(noPersist bool) AttackOption {
	return func(o *AttackOptions) {
		o.NoPersist = noPersist
	}
}

// WithOutputFormat sets the output format.
func WithOutputFormat(format string) AttackOption {
	return func(o *AttackOptions) {
		o.OutputFormat = format
	}
}

// WithVerbose enables verbose output.
func WithVerbose(verbose bool) AttackOption {
	return func(o *AttackOptions) {
		o.Verbose = verbose
	}
}

// WithQuiet enables quiet mode.
func WithQuiet(quiet bool) AttackOption {
	return func(o *AttackOptions) {
		o.Quiet = quiet
	}
}

// WithDryRun enables dry-run mode.
func WithDryRun(dryRun bool) AttackOption {
	return func(o *AttackOptions) {
		o.DryRun = dryRun
	}
}

// Apply applies functional options to the AttackOptions.
func (o *AttackOptions) Apply(opts ...AttackOption) {
	for _, opt := range opts {
		opt(o)
	}
}
