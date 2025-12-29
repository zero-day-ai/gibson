//go:build integration

package attack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestIntegration_FullAttackExecution tests the complete attack flow with real components.
func TestIntegration_FullAttackExecution(t *testing.T) {
	tests := []struct {
		name           string
		targetResponse string
		expectedStatus AttackStatus
		expectFindings bool
	}{
		{
			name:           "successful attack without findings",
			targetResponse: `{"status": "ok", "message": "Normal response"}`,
			expectedStatus: AttackStatusSuccess,
			expectFindings: false,
		},
		{
			name:           "successful attack with findings",
			targetResponse: `{"status": "vulnerable", "secret": "admin_password_123"}`,
			expectedStatus: AttackStatusFindings,
			expectFindings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock HTTP server as target
			targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.targetResponse))
			}))
			defer targetServer.Close()

			// Create mock registries and stores
			agentRegistry := newMockIntegrationAgentRegistry()
			payloadRegistry := newMockIntegrationPayloadRegistry()
			missionStore := newMockIntegrationMissionStore()
			findingStore := newMockIntegrationFindingStore()
			orchestrator := newMockIntegrationOrchestrator(tt.expectFindings)

			// Create attack runner
			runner := NewAttackRunner(
				orchestrator,
				agentRegistry,
				payloadRegistry,
				missionStore,
				findingStore,
			)

			// Configure attack options
			opts := NewAttackOptions()
			opts.TargetURL = targetServer.URL
			opts.AgentName = "test-agent"
			opts.Goal = "Test vulnerability scanning"
			opts.MaxTurns = 5
			opts.Timeout = 30 * time.Second

			// Execute attack
			ctx := context.Background()
			result, err := runner.Run(ctx, opts)

			// Verify results
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectFindings, result.HasFindings())

			if tt.expectFindings {
				assert.Greater(t, result.FindingCount(), 0)
			} else {
				assert.Equal(t, 0, result.FindingCount())
			}

			// Verify execution metrics
			assert.Greater(t, result.Duration, time.Duration(0))
			assert.GreaterOrEqual(t, result.TurnsUsed, 0)
		})
	}
}

// TestIntegration_EphemeralMode tests attack execution in ephemeral mode (no persistence).
func TestIntegration_EphemeralMode(t *testing.T) {
	// Set up mock HTTP server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "vulnerable", "data": "sensitive"}`))
	}))
	defer targetServer.Close()

	// Create components
	missionStore := newMockIntegrationMissionStore()
	findingStore := newMockIntegrationFindingStore()
	orchestrator := newMockIntegrationOrchestrator(true) // Returns findings

	runner := NewAttackRunner(
		orchestrator,
		newMockIntegrationAgentRegistry(),
		newMockIntegrationPayloadRegistry(),
		missionStore,
		findingStore,
	)

	// Test with --no-persist flag
	t.Run("no_persist flag prevents persistence", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.TargetURL = targetServer.URL
		opts.AgentName = "test-agent"
		opts.NoPersist = true

		result, err := runner.Run(context.Background(), opts)
		require.NoError(t, err)
		assert.False(t, result.Persisted)
		assert.Nil(t, result.MissionID)
		assert.Equal(t, 0, missionStore.saveCount)
	})

	// Test auto-persistence with findings
	t.Run("auto persist with findings", func(t *testing.T) {
		missionStore.reset()
		findingStore.reset()

		opts := NewAttackOptions()
		opts.TargetURL = targetServer.URL
		opts.AgentName = "test-agent"
		// No persist flags - should auto-persist because findings exist

		result, err := runner.Run(context.Background(), opts)
		require.NoError(t, err)
		assert.True(t, result.Persisted)
		assert.NotNil(t, result.MissionID)
		assert.Equal(t, 1, missionStore.saveCount)
		assert.Greater(t, findingStore.storeCount, 0)
	})
}

// TestIntegration_PersistentMode tests attack execution with explicit persistence.
func TestIntegration_PersistentMode(t *testing.T) {
	// Set up mock HTTP server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer targetServer.Close()

	missionStore := newMockIntegrationMissionStore()
	findingStore := newMockIntegrationFindingStore()
	orchestrator := newMockIntegrationOrchestrator(false) // No findings

	runner := NewAttackRunner(
		orchestrator,
		newMockIntegrationAgentRegistry(),
		newMockIntegrationPayloadRegistry(),
		missionStore,
		findingStore,
	)

	// Test with --persist flag (forces persistence even without findings)
	t.Run("persist flag forces persistence", func(t *testing.T) {
		opts := NewAttackOptions()
		opts.TargetURL = targetServer.URL
		opts.AgentName = "test-agent"
		opts.Persist = true

		result, err := runner.Run(context.Background(), opts)
		require.NoError(t, err)
		assert.True(t, result.Persisted)
		assert.NotNil(t, result.MissionID)
		assert.Equal(t, 1, missionStore.saveCount)
	})

	// Test without findings and without --persist (should not persist)
	t.Run("no persist without findings", func(t *testing.T) {
		missionStore.reset()

		opts := NewAttackOptions()
		opts.TargetURL = targetServer.URL
		opts.AgentName = "test-agent"
		// No persist flags, no findings - should be ephemeral

		result, err := runner.Run(context.Background(), opts)
		require.NoError(t, err)
		assert.False(t, result.Persisted)
		assert.Nil(t, result.MissionID)
		assert.Equal(t, 0, missionStore.saveCount)
	})
}

// TestIntegration_OutputFormats tests various output format handlers.
func TestIntegration_OutputFormats(t *testing.T) {
	// Create test findings
	testFindings := []finding.EnhancedFinding{
		{
			Finding: agent.Finding{
				ID:          types.NewID(),
				Title:       "SQL Injection Vulnerability",
				Description: "Application is vulnerable to SQL injection",
				Severity:    agent.SeverityHigh,
				Category:    "Injection",
				Confidence:  0.95,
				CreatedAt:   time.Now(),
			},
			MissionID:   types.NewID(),
			AgentName:   "test-agent",
			Subcategory: "SQL",
			Status:      finding.StatusConfirmed,
			RiskScore:   8.5,
		},
		{
			Finding: agent.Finding{
				ID:          types.NewID(),
				Title:       "Information Disclosure",
				Description: "Sensitive information exposed in response",
				Severity:    agent.SeverityMedium,
				Category:    "Information Disclosure",
				Confidence:  0.85,
				CreatedAt:   time.Now(),
			},
			MissionID:   types.NewID(),
			AgentName:   "test-agent",
			Subcategory: "",
			Status:      finding.StatusConfirmed,
			RiskScore:   5.0,
		},
	}

	result := &AttackResult{
		Status:     AttackStatusFindings,
		Duration:   5 * time.Second,
		TurnsUsed:  3,
		TokensUsed: 1500,
		Findings:   testFindings,
		FindingsBySeverity: map[string]int{
			"high":   1,
			"medium": 1,
		},
		Persisted: true,
	}
	missionID := types.NewID()
	result.WithMissionID(missionID)

	tests := []struct {
		name           string
		format         string
		verbose        bool
		quiet          bool
		expectedOutput string
		validateJSON   bool
		validateSARIF  bool
	}{
		{
			name:           "text format normal",
			format:         OutputFormatText,
			verbose:        false,
			quiet:          false,
			expectedOutput: "Attack Complete",
		},
		{
			name:           "text format verbose",
			format:         OutputFormatText,
			verbose:        true,
			quiet:          false,
			expectedOutput: "Configuration:",
		},
		{
			name:           "text format quiet",
			format:         OutputFormatText,
			verbose:        false,
			quiet:          true,
			expectedOutput: "SQL Injection",
		},
		{
			name:         "json format",
			format:       OutputFormatJSON,
			validateJSON: true,
		},
		{
			name:          "sarif format",
			format:        OutputFormatSARIF,
			validateSARIF: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			handler := NewOutputHandler(tt.format, &buf, tt.verbose, tt.quiet)

			// Simulate output flow
			opts := &AttackOptions{
				TargetURL:  "https://example.com/api",
				AgentName:  "test-agent",
				Goal:       "Test security",
				MaxTurns:   5,
				Timeout:    30 * time.Second,
				Verbose:    tt.verbose,
				Quiet:      tt.quiet,
			}

			handler.OnStart(opts)
			for _, f := range testFindings {
				handler.OnFinding(f)
			}
			handler.OnComplete(result)

			output := buf.String()

			// Validate output based on format
			if tt.expectedOutput != "" {
				assert.Contains(t, output, tt.expectedOutput)
			}

			if tt.validateJSON {
				// Verify it's valid JSON stream
				lines := strings.Split(strings.TrimSpace(output), "\n")
				for _, line := range lines {
					var event map[string]interface{}
					err := json.Unmarshal([]byte(line), &event)
					require.NoError(t, err, "Invalid JSON: %s", line)
					assert.Contains(t, event, "type")
				}
			}

			if tt.validateSARIF {
				// Verify it's valid SARIF
				var sarif map[string]interface{}
				err := json.Unmarshal([]byte(output), &sarif)
				require.NoError(t, err)
				assert.Equal(t, "2.1.0", sarif["version"])
				assert.Contains(t, sarif, "runs")
			}
		})
	}
}

// TestIntegration_MockTargetServer tests with various mock HTTP server scenarios.
func TestIntegration_MockTargetServer(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		expectError    bool
		expectedStatus AttackStatus
	}{
		{
			name: "normal response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data": "normal"}`))
			},
			expectError:    false,
			expectedStatus: AttackStatusSuccess,
		},
		{
			name: "slow response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data": "slow"}`))
			},
			expectError:    false,
			expectedStatus: AttackStatusSuccess,
		},
		{
			name: "error response",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "internal error"}`))
			},
			expectError:    false,
			expectedStatus: AttackStatusSuccess,
		},
		{
			name: "multiple requests",
			serverHandler: func() http.HandlerFunc {
				count := 0
				return func(w http.ResponseWriter, r *http.Request) {
					count++
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(fmt.Sprintf(`{"request": %d}`, count)))
				}
			}(),
			expectError:    false,
			expectedStatus: AttackStatusSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			runner := NewAttackRunner(
				newMockIntegrationOrchestrator(false),
				newMockIntegrationAgentRegistry(),
				newMockIntegrationPayloadRegistry(),
				newMockIntegrationMissionStore(),
				newMockIntegrationFindingStore(),
			)

			opts := NewAttackOptions()
			opts.TargetURL = server.URL
			opts.AgentName = "test-agent"
			opts.Timeout = 5 * time.Second

			result, err := runner.Run(context.Background(), opts)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
			}
		})
	}
}

// TestIntegration_ConcurrentAttacks tests running multiple attacks concurrently.
func TestIntegration_ConcurrentAttacks(t *testing.T) {
	// Set up shared mock server
	requestCount := 0
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create shared components (must be thread-safe)
	runner := NewAttackRunner(
		newMockIntegrationOrchestrator(false),
		newMockIntegrationAgentRegistry(),
		newMockIntegrationPayloadRegistry(),
		newMockIntegrationMissionStore(),
		newMockIntegrationFindingStore(),
	)

	// Run multiple attacks concurrently
	const numAttacks = 10
	results := make([]*AttackResult, numAttacks)
	errors := make([]error, numAttacks)
	var wg sync.WaitGroup

	for i := 0; i < numAttacks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			opts := NewAttackOptions()
			opts.TargetURL = server.URL
			opts.AgentName = "test-agent"
			opts.Timeout = 10 * time.Second

			result, err := runner.Run(context.Background(), opts)
			results[idx] = result
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify all attacks completed successfully
	for i := 0; i < numAttacks; i++ {
		require.NoError(t, errors[i], "Attack %d failed", i)
		assert.NotNil(t, results[i])
		assert.Equal(t, AttackStatusSuccess, results[i].Status)
	}
}

// TestIntegration_ContextCancellation tests attack cancellation via context.
func TestIntegration_ContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewAttackRunner(
		newMockIntegrationOrchestratorWithDelay(2 * time.Second),
		newMockIntegrationAgentRegistry(),
		newMockIntegrationPayloadRegistry(),
		newMockIntegrationMissionStore(),
		newMockIntegrationFindingStore(),
	)

	opts := NewAttackOptions()
	opts.TargetURL = server.URL
	opts.AgentName = "test-agent"

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 500ms
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	result, err := runner.Run(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, AttackStatusCancelled, result.Status)
}

// TestIntegration_Timeout tests attack timeout handling.
func TestIntegration_Timeout(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewAttackRunner(
		newMockIntegrationOrchestratorWithDelay(3 * time.Second),
		newMockIntegrationAgentRegistry(),
		newMockIntegrationPayloadRegistry(),
		newMockIntegrationMissionStore(),
		newMockIntegrationFindingStore(),
	)

	opts := NewAttackOptions()
	opts.TargetURL = server.URL
	opts.AgentName = "test-agent"
	opts.Timeout = 1 * time.Second

	result, err := runner.Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, AttackStatusTimeout, result.Status)
}

// TestIntegration_DryRun tests dry-run mode.
func TestIntegration_DryRun(t *testing.T) {
	// Server should never be called in dry-run
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewAttackRunner(
		newMockIntegrationOrchestrator(false),
		newMockIntegrationAgentRegistry(),
		newMockIntegrationPayloadRegistry(),
		newMockIntegrationMissionStore(),
		newMockIntegrationFindingStore(),
	)

	opts := NewAttackOptions()
	opts.TargetURL = server.URL
	opts.AgentName = "test-agent"
	opts.DryRun = true

	result, err := runner.Run(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, AttackStatusSuccess, result.Status)
	assert.False(t, serverCalled, "Server should not be called in dry-run mode")
	assert.Equal(t, 0, result.TurnsUsed)
}
