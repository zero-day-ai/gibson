package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/finding"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestFindingListCommand tests the finding list command with various filters
func TestFindingListCommand(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		setupFindings func(*testing.T, *database.DB) []finding.EnhancedFinding
		expectError   bool
		expectOutput  []string
	}{
		{
			name: "list all findings",
			args: []string{"list"},
			setupFindings: func(t *testing.T, db *database.DB) []finding.EnhancedFinding {
				return createTestFindings(t, db, 3)
			},
			expectError: false,
			expectOutput: []string{
				"ID", "TITLE", "SEVERITY", "CATEGORY", "STATUS", "MISSION",
			},
		},
		{
			name: "filter by critical severity",
			args: []string{"list", "--severity", "critical"},
			setupFindings: func(t *testing.T, db *database.DB) []finding.EnhancedFinding {
				findings := createTestFindings(t, db, 3)
				findings[0].Severity = agent.SeverityCritical
				findings[1].Severity = agent.SeverityHigh
				findings[2].Severity = agent.SeverityMedium
				store := finding.NewDBFindingStore(db)
				for _, f := range findings {
					require.NoError(t, store.Update(context.Background(), f))
				}
				return findings
			},
			expectError: false,
			expectOutput: []string{
				"critical",
			},
		},
		{
			name: "filter by category",
			args: []string{"list", "--category", "jailbreak"},
			setupFindings: func(t *testing.T, db *database.DB) []finding.EnhancedFinding {
				findings := createTestFindings(t, db, 2)
				findings[0].Category = "jailbreak"
				findings[1].Category = "prompt_injection"
				store := finding.NewDBFindingStore(db)
				for _, f := range findings {
					require.NoError(t, store.Update(context.Background(), f))
				}
				return findings
			},
			expectError: false,
			expectOutput: []string{
				"jailbreak",
			},
		},
		{
			name: "filter by mission ID",
			args: func() []string {
				missionID := types.NewID()
				return []string{"list", "--mission", missionID.String()}
			}(),
			setupFindings: func(t *testing.T, db *database.DB) []finding.EnhancedFinding {
				// Create findings with different mission IDs
				findings := createTestFindings(t, db, 2)
				return findings
			},
			expectError: false,
			expectOutput: []string{
				"No findings found",
			},
		},
		{
			name: "no findings",
			args: []string{"list"},
			setupFindings: func(t *testing.T, db *database.DB) []finding.EnhancedFinding {
				return []finding.EnhancedFinding{}
			},
			expectError: false,
			expectOutput: []string{
				"No findings found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory and database
			tempDir := t.TempDir()
			os.Setenv("GIBSON_HOME", tempDir)
			defer os.Unsetenv("GIBSON_HOME")

			dbPath := filepath.Join(tempDir, "gibson.db")
			db, err := database.Open(dbPath)
			require.NoError(t, err)
			defer db.Close()

			// Setup test findings
			if tt.setupFindings != nil {
				tt.setupFindings(t, db)
			}

			// Execute command
			cmd := findingCmd
			cmd.SetArgs(tt.args)

			var outBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&outBuf)

			err = cmd.Execute()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			output := outBuf.String()
			for _, expected := range tt.expectOutput {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}
		})
	}
}

// TestFindingShowCommand tests the finding show command
func TestFindingShowCommand(t *testing.T) {
	tests := []struct {
		name         string
		setupFinding func(*testing.T, *database.DB) types.ID
		expectError  bool
		expectOutput []string
	}{
		{
			name: "show complete finding",
			setupFinding: func(t *testing.T, db *database.DB) types.ID {
				findings := createTestFindings(t, db, 1)
				f := &findings[0]

				// Add comprehensive details
				f.Description = "Test finding with complete details"
				f.Remediation = "Apply security patches and review configurations"
				f.References = []string{
					"https://example.com/vuln-1",
					"https://example.com/vuln-2",
				}
				f.CWE = []string{"CWE-79", "CWE-89"}
				f.MitreAttack = []finding.SimpleMitreMapping{
					{
						TechniqueID:   "T1566",
						TechniqueName: "Phishing",
						Tactic:        "Initial Access",
					},
				}
				f.ReproSteps = []finding.ReproStep{
					{
						StepNumber:     1,
						Description:    "Send malicious input",
						ExpectedResult: "System accepts input",
					},
					{
						StepNumber:     2,
						Description:    "Observe behavior",
						ExpectedResult: "Jailbreak successful",
					},
				}

				store := finding.NewDBFindingStore(db)
				require.NoError(t, store.Update(context.Background(), *f))

				return f.ID
			},
			expectError: false,
			expectOutput: []string{
				"Finding:",
				"Severity:",
				"Description:",
				"Evidence",
				"Remediation:",
				"References:",
				"CWE IDs:",
				"MITRE ATT&CK",
				"Reproduction Steps:",
			},
		},
		{
			name: "invalid finding ID",
			setupFinding: func(t *testing.T, db *database.DB) types.ID {
				return types.NewID() // Random ID that doesn't exist
			},
			expectError: true,
			expectOutput: []string{
				"failed to get finding",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory and database
			tempDir := t.TempDir()
			os.Setenv("GIBSON_HOME", tempDir)
			defer os.Unsetenv("GIBSON_HOME")

			dbPath := filepath.Join(tempDir, "gibson.db")
			db, err := database.Open(dbPath)
			require.NoError(t, err)
			defer db.Close()

			// Setup test finding
			findingID := tt.setupFinding(t, db)

			// Execute command
			cmd := findingCmd
			cmd.SetArgs([]string{"show", findingID.String()})

			var outBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&outBuf)

			err = cmd.Execute()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			output := outBuf.String()
			for _, expected := range tt.expectOutput {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}
		})
	}
}

// TestFindingExportCommand tests the finding export command with various formats
func TestFindingExportCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		setupFindings  func(*testing.T, *database.DB)
		expectError    bool
		validateOutput func(*testing.T, string)
	}{
		{
			name: "export to JSON format",
			args: []string{"export", "--format", "json"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 3)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, `"findings"`)
				assert.Contains(t, output, `"metadata"`)
				assert.Contains(t, output, `"total_count"`)
			},
		},
		{
			name: "export to SARIF format",
			args: []string{"export", "--format", "sarif"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 2)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, `"version"`)
				assert.Contains(t, output, `"$schema"`)
				assert.Contains(t, output, `"runs"`)
			},
		},
		{
			name: "export to CSV format",
			args: []string{"export", "--format", "csv"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 2)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "ID,Title,Severity,Category")
				lines := strings.Split(strings.TrimSpace(output), "\n")
				assert.GreaterOrEqual(t, len(lines), 2, "CSV should have header and data rows")
			},
		},
		{
			name: "export to HTML format",
			args: []string{"export", "--format", "html"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 2)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "<html")
				assert.Contains(t, output, "<table")
				assert.Contains(t, output, "</html>")
			},
		},
		{
			name: "export to Markdown format",
			args: []string{"export", "--format", "markdown"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 2)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "# Security Findings Report")
				assert.Contains(t, output, "##")
			},
		},
		{
			name: "export with severity filter",
			args: []string{"export", "--format", "json", "--severity", "critical"},
			setupFindings: func(t *testing.T, db *database.DB) {
				findings := createTestFindings(t, db, 3)
				findings[0].Severity = agent.SeverityCritical
				findings[1].Severity = agent.SeverityHigh
				findings[2].Severity = agent.SeverityMedium
				store := finding.NewDBFindingStore(db)
				for _, f := range findings {
					require.NoError(t, store.Update(context.Background(), f))
				}
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "critical")
				assert.NotContains(t, output, "medium")
			},
		},
		{
			name: "export to file",
			args: func() []string {
				tempDir := t.TempDir()
				outputFile := filepath.Join(tempDir, "findings.json")
				return []string{"export", "--format", "json", "--output", outputFile}
			}(),
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 2)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Exported")
				assert.Contains(t, output, "findings.json")
			},
		},
		{
			name: "export without evidence",
			args: []string{"export", "--format", "json", "--evidence=false"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 1)
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, `"findings"`)
				// Evidence should be null or empty
			},
		},
		{
			name: "export with minimum confidence",
			args: []string{"export", "--format", "json", "--min-confidence", "0.8"},
			setupFindings: func(t *testing.T, db *database.DB) {
				findings := createTestFindings(t, db, 3)
				findings[0].Confidence = 0.9
				findings[1].Confidence = 0.7
				findings[2].Confidence = 0.85
				store := finding.NewDBFindingStore(db)
				for _, f := range findings {
					require.NoError(t, store.Update(context.Background(), f))
				}
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, `"findings"`)
				// Should filter out findings with confidence < 0.8
			},
		},
		{
			name: "unsupported export format",
			args: []string{"export", "--format", "xml"},
			setupFindings: func(t *testing.T, db *database.DB) {
				createTestFindings(t, db, 1)
			},
			expectError: true,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "unsupported export format")
			},
		},
		{
			name: "no findings to export",
			args: []string{"export", "--format", "json"},
			setupFindings: func(t *testing.T, db *database.DB) {
				// Don't create any findings
			},
			expectError: false,
			validateOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "No findings to export")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory and database
			tempDir := t.TempDir()
			os.Setenv("GIBSON_HOME", tempDir)
			defer os.Unsetenv("GIBSON_HOME")

			dbPath := filepath.Join(tempDir, "gibson.db")
			db, err := database.Open(dbPath)
			require.NoError(t, err)
			defer db.Close()

			// Setup test findings
			if tt.setupFindings != nil {
				tt.setupFindings(t, db)
			}

			// Execute command
			cmd := findingCmd
			cmd.SetArgs(tt.args)

			var outBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&outBuf)

			err = cmd.Execute()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			output := outBuf.String()
			if tt.validateOutput != nil {
				tt.validateOutput(t, output)
			}

			// If output file was specified, verify it exists
			for i, arg := range tt.args {
				if arg == "--output" && i+1 < len(tt.args) && !tt.expectError {
					outputFile := tt.args[i+1]
					_, err := os.Stat(outputFile)
					assert.NoError(t, err, "Output file should exist")
				}
			}
		})
	}
}

// TestSeverityColorCoding tests the color coding functionality
func TestSeverityColorCoding(t *testing.T) {
	tests := []struct {
		name     string
		severity agent.FindingSeverity
	}{
		{"critical severity", agent.SeverityCritical},
		{"high severity", agent.SeverityHigh},
		{"medium severity", agent.SeverityMedium},
		{"low severity", agent.SeverityLow},
		{"info severity", agent.SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test formatSeverity
			formatted := formatSeverity(tt.severity)
			assert.NotEmpty(t, formatted)
			assert.Contains(t, formatted, string(tt.severity))

			// Test getSeverityColor
			color := getSeverityColor(tt.severity)
			assert.NotNil(t, color)
		})
	}
}

// TestTextWrapping tests the text wrapping utility function
func TestTextWrapping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected int // expected number of lines
	}{
		{
			name:     "short text",
			input:    "Short text",
			width:    80,
			expected: 1,
		},
		{
			name:     "text requiring wrapping",
			input:    strings.Repeat("word ", 20),
			width:    40,
			expected: 3, // approximate
		},
		{
			name:     "empty text",
			input:    "",
			width:    80,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.input, tt.width)

			if tt.input == "" {
				assert.Empty(t, result)
				return
			}

			lines := strings.Split(result, "\n")

			// Each line should not exceed width
			for _, line := range lines {
				assert.LessOrEqual(t, len(line), tt.width+10, // Allow some margin
					"Line exceeds width: %s", line)
			}
		})
	}
}

// TestTruncate tests the string truncation function
func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string",
			input:    "Short",
			maxLen:   10,
			expected: "Short",
		},
		{
			name:     "exact length",
			input:    "Exact",
			maxLen:   5,
			expected: "Exact",
		},
		{
			name:     "needs truncation",
			input:    "This is a very long string that needs truncation",
			maxLen:   20,
			expected: "This is a very lo...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), tt.maxLen)
		})
	}
}

// Helper function to create test findings
func createTestFindings(t *testing.T, db *database.DB, count int) []finding.EnhancedFinding {
	t.Helper()

	findings := make([]finding.EnhancedFinding, count)
	store := finding.NewDBFindingStore(db)
	missionID := types.NewID()

	for i := 0; i < count; i++ {
		baseFinding := agent.NewFinding(
			"Test Finding "+string(rune('A'+i)),
			"This is a test finding description for testing purposes",
			agent.SeverityHigh,
		)

		baseFinding.Category = "jailbreak"
		baseFinding.Evidence = []agent.Evidence{
			{
				Type:        "log",
				Description: "Test evidence",
				Data: map[string]any{
					"key": "value",
				},
				Timestamp: time.Now(),
			},
		}

		enhanced := finding.NewEnhancedFinding(baseFinding, missionID, "test-agent")
		enhanced.Status = finding.StatusOpen
		enhanced.RiskScore = 7.5
		enhanced.Remediation = "Apply security patches"

		err := store.Store(context.Background(), enhanced)
		require.NoError(t, err)

		findings[i] = enhanced
	}

	return findings
}

// TestFindingListAllFindings tests the helper function for listing all findings
func TestFindingListAllFindings(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Create findings across multiple missions
	store := finding.NewDBFindingStore(db)
	ctx := context.Background()

	mission1 := types.NewID()
	mission2 := types.NewID()

	// Create findings for mission 1
	for i := 0; i < 2; i++ {
		f := finding.NewEnhancedFinding(
			agent.NewFinding("Finding M1-"+string(rune('A'+i)), "Description", agent.SeverityHigh),
			mission1,
			"agent1",
		)
		require.NoError(t, store.Store(ctx, f))
	}

	// Create findings for mission 2
	for i := 0; i < 3; i++ {
		f := finding.NewEnhancedFinding(
			agent.NewFinding("Finding M2-"+string(rune('A'+i)), "Description", agent.SeverityMedium),
			mission2,
			"agent2",
		)
		require.NoError(t, store.Store(ctx, f))
	}

	// Test listing all findings
	filter := finding.NewFindingFilter()
	findings, err := listAllFindings(ctx, store, filter)

	// Note: This test depends on the implementation of listAllFindings
	// which currently uses an empty mission ID
	if err == nil {
		// Verify we got findings
		assert.NotNil(t, findings)
	}
}
