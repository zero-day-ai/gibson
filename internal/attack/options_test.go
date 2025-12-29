package attack

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestNewAttackOptions(t *testing.T) {
	opts := NewAttackOptions()

	assert.NotNil(t, opts)
	assert.NotNil(t, opts.TargetHeaders)
	assert.NotNil(t, opts.PayloadIDs)
	assert.NotNil(t, opts.Techniques)
	assert.True(t, opts.FollowRedirects, "FollowRedirects should be enabled by default")
	assert.Equal(t, "text", opts.OutputFormat)
	assert.Equal(t, 0, opts.MaxTurns)
	assert.Equal(t, time.Duration(0), opts.Timeout)
	assert.Equal(t, 0, opts.MaxFindings)
	assert.Equal(t, 0, opts.RateLimit)
}

func TestAttackOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*AttackOptions)
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with URL",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
			},
			wantErr: false,
		},
		{
			name: "valid with target name",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetName = "saved-target"
			},
			wantErr: false,
		},
		{
			name: "missing agent name",
			setup: func(o *AttackOptions) {
				o.TargetURL = "https://example.com"
			},
			wantErr: true,
			errMsg:  "agent name is required",
		},
		{
			name: "agent name with only whitespace",
			setup: func(o *AttackOptions) {
				o.AgentName = "   "
				o.TargetURL = "https://example.com"
			},
			wantErr: true,
			errMsg:  "agent name is required",
		},
		{
			name: "missing target",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
			},
			wantErr: true,
			errMsg:  "target is required",
		},
		{
			name: "both URL and name specified",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.TargetName = "saved-target"
			},
			wantErr: true,
			errMsg:  "cannot specify both",
		},
		{
			name: "invalid target type",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.TargetType = "invalid_type"
			},
			wantErr: true,
			errMsg:  "invalid target type",
		},
		{
			name: "valid target type",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.TargetType = types.TargetTypeLLMAPI
			},
			wantErr: false,
		},
		{
			name: "invalid provider",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.TargetProvider = "invalid_provider"
			},
			wantErr: true,
			errMsg:  "invalid provider",
		},
		{
			name: "valid provider",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.TargetProvider = "openai"
			},
			wantErr: false,
		},
		{
			name: "both persist flags set",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.Persist = true
				o.NoPersist = true
			},
			wantErr: true,
			errMsg:  "cannot specify both --persist and --no-persist",
		},
		{
			name: "both verbose and quiet set",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.Verbose = true
				o.Quiet = true
			},
			wantErr: true,
			errMsg:  "cannot specify both --verbose and --quiet",
		},
		{
			name: "invalid output format",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.OutputFormat = "xml"
			},
			wantErr: true,
			errMsg:  "invalid output format",
		},
		{
			name: "valid output formats",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.OutputFormat = "json"
			},
			wantErr: false,
		},
		{
			name: "invalid severity threshold",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.SeverityThreshold = "super-critical"
			},
			wantErr: true,
			errMsg:  "invalid severity threshold",
		},
		{
			name: "valid severity threshold - lowercase",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.SeverityThreshold = "high"
			},
			wantErr: false,
		},
		{
			name: "valid severity threshold - mixed case",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.SeverityThreshold = "Critical"
			},
			wantErr: false,
		},
		{
			name: "negative max turns",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.MaxTurns = -1
			},
			wantErr: true,
			errMsg:  "max-turns cannot be negative",
		},
		{
			name: "negative max findings",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.MaxFindings = -1
			},
			wantErr: true,
			errMsg:  "max-findings cannot be negative",
		},
		{
			name: "negative rate limit",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.RateLimit = -5
			},
			wantErr: true,
			errMsg:  "rate-limit cannot be negative",
		},
		{
			name: "negative timeout",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://example.com"
				o.Timeout = -10 * time.Second
			},
			wantErr: true,
			errMsg:  "timeout cannot be negative",
		},
		{
			name: "fully configured attack",
			setup: func(o *AttackOptions) {
				o.AgentName = "test-agent"
				o.TargetURL = "https://api.example.com/chat"
				o.TargetType = types.TargetTypeLLMChat
				o.TargetProvider = "anthropic"
				o.TargetHeaders = map[string]string{"X-Custom": "value"}
				o.Credential = "cred-123"
				o.Goal = "Test prompt injection"
				o.MaxTurns = 10
				o.Timeout = 5 * time.Minute
				o.PayloadIDs = []string{"payload-1", "payload-2"}
				o.PayloadCategory = "injection"
				o.Techniques = []string{"T1059"}
				o.MaxFindings = 5
				o.SeverityThreshold = "medium"
				o.RateLimit = 10
				o.FollowRedirects = false
				o.InsecureTLS = true
				o.ProxyURL = "http://proxy.local:8080"
				o.Persist = true
				o.OutputFormat = "json"
				o.Verbose = true
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewAttackOptions()
			tt.setup(opts)

			err := opts.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAttackOptions_FunctionalOptions(t *testing.T) {
	t.Run("single option", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.Apply(WithAgentName("test-agent"))

		assert.Equal(t, "test-agent", opts.AgentName)
	})

	t.Run("multiple options", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.Apply(
			WithAgentName("test-agent"),
			WithTargetURL("https://example.com"),
			WithGoal("Test security"),
			WithMaxTurns(20),
			WithTimeout(10*time.Minute),
			WithVerbose(true),
		)

		assert.Equal(t, "test-agent", opts.AgentName)
		assert.Equal(t, "https://example.com", opts.TargetURL)
		assert.Equal(t, "Test security", opts.Goal)
		assert.Equal(t, 20, opts.MaxTurns)
		assert.Equal(t, 10*time.Minute, opts.Timeout)
		assert.True(t, opts.Verbose)
	})

	t.Run("all target options", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer token"}
		opts := NewAttackOptions()
		opts.Apply(
			WithTargetURL("https://api.example.com"),
			WithTargetType(types.TargetTypeRAG),
			WithTargetProvider("openai"),
			WithTargetHeaders(headers),
			WithCredential("cred-456"),
		)

		assert.Equal(t, "https://api.example.com", opts.TargetURL)
		assert.Equal(t, types.TargetTypeRAG, opts.TargetType)
		assert.Equal(t, "openai", opts.TargetProvider)
		assert.Equal(t, headers, opts.TargetHeaders)
		assert.Equal(t, "cred-456", opts.Credential)
	})

	t.Run("payload filter options", func(t *testing.T) {
		ids := []string{"p1", "p2"}
		techniques := []string{"T1059", "T1071"}
		opts := NewAttackOptions()
		opts.Apply(
			WithPayloadIDs(ids),
			WithPayloadCategory("injection"),
			WithTechniques(techniques),
		)

		assert.Equal(t, ids, opts.PayloadIDs)
		assert.Equal(t, "injection", opts.PayloadCategory)
		assert.Equal(t, techniques, opts.Techniques)
	})

	t.Run("execution constraint options", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.Apply(
			WithMaxFindings(10),
			WithSeverityThreshold("high"),
			WithRateLimit(5),
		)

		assert.Equal(t, 10, opts.MaxFindings)
		assert.Equal(t, "high", opts.SeverityThreshold)
		assert.Equal(t, 5, opts.RateLimit)
	})

	t.Run("network options", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.Apply(
			WithFollowRedirects(false),
			WithInsecureTLS(true),
			WithProxyURL("http://proxy:8080"),
		)

		assert.False(t, opts.FollowRedirects)
		assert.True(t, opts.InsecureTLS)
		assert.Equal(t, "http://proxy:8080", opts.ProxyURL)
	})

	t.Run("persistence options", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.Apply(
			WithPersist(true),
		)

		assert.True(t, opts.Persist)

		opts2 := NewAttackOptions()
		opts2.Apply(
			WithNoPersist(true),
		)

		assert.True(t, opts2.NoPersist)
	})

	t.Run("output options", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.Apply(
			WithOutputFormat("sarif"),
			WithVerbose(true),
			WithDryRun(true),
		)

		assert.Equal(t, "sarif", opts.OutputFormat)
		assert.True(t, opts.Verbose)
		assert.True(t, opts.DryRun)

		opts2 := NewAttackOptions()
		opts2.Apply(
			WithQuiet(true),
		)

		assert.True(t, opts2.Quiet)
	})
}

func TestAttackOptions_SeverityNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "low", "low"},
		{"uppercase", "HIGH", "high"},
		{"mixed case", "MeDiUm", "medium"},
		{"critical", "Critical", "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewAttackOptions()
			opts.AgentName = "test-agent"
			opts.TargetURL = "https://example.com"
			opts.SeverityThreshold = tt.input

			err := opts.Validate()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, opts.SeverityThreshold)
		})
	}
}
