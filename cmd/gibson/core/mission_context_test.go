package core

import (
	"testing"
)

func TestExtractMissionName(t *testing.T) {
	tests := []struct {
		name         string
		workflowPath string
		missionID    string
		expected     string
	}{
		{
			name:         "workflow path with yaml extension",
			workflowPath: "/path/to/recon-webapp.yaml",
			missionID:    "recon-webapp-20260107-123456",
			expected:     "recon-webapp",
		},
		{
			name:         "workflow path with yml extension",
			workflowPath: "/workflows/api-scan.yml",
			missionID:    "api-scan-20260107-123456",
			expected:     "api-scan",
		},
		{
			name:         "no workflow path, extract from ID",
			workflowPath: "",
			missionID:    "security-audit-20260107-123456",
			expected:     "security-audit",
		},
		{
			name:         "mission ID without timestamp",
			workflowPath: "",
			missionID:    "simple-mission",
			expected:     "simple-mission",
		},
		{
			name:         "mission ID with multiple hyphens",
			workflowPath: "",
			missionID:    "web-app-security-scan-20260107-123456",
			expected:     "web-app-security-scan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMissionName(tt.workflowPath, tt.missionID)
			if result != tt.expected {
				t.Errorf("extractMissionName(%q, %q) = %q; want %q",
					tt.workflowPath, tt.missionID, result, tt.expected)
			}
		})
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "numeric string",
			input:    "123456",
			expected: true,
		},
		{
			name:     "alphanumeric string",
			input:    "abc123",
			expected: false,
		},
		{
			name:     "non-numeric string",
			input:    "abc",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "date format",
			input:    "20260107",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("isNumeric(%q) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMissionContextResult(t *testing.T) {
	// Test that the result structure can be created and has expected fields
	result := &MissionContextResult{
		MissionName:      "test-mission",
		MissionID:        "test-mission-20260107-123456",
		RunNumber:        3,
		TotalRuns:        5,
		Status:           "running",
		Resumed:          true,
		ResumedFromNode:  "checkpoint-5-nodes",
		PreviousRunID:    "test-mission-20260107-113456",
		PreviousStatus:   "completed",
		TotalFindings:    47,
		MemoryContinuity: "inherit",
		WorkflowName:     "/workflows/test-mission.yaml",
	}

	if result.MissionName != "test-mission" {
		t.Errorf("MissionName = %q; want %q", result.MissionName, "test-mission")
	}
	if result.RunNumber != 3 {
		t.Errorf("RunNumber = %d; want %d", result.RunNumber, 3)
	}
	if result.TotalRuns != 5 {
		t.Errorf("TotalRuns = %d; want %d", result.TotalRuns, 5)
	}
	if !result.Resumed {
		t.Error("Resumed should be true")
	}
	if result.TotalFindings != 47 {
		t.Errorf("TotalFindings = %d; want %d", result.TotalFindings, 47)
	}
}
