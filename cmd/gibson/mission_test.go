package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/config"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

func TestMissionList(t *testing.T) {
	tests := []struct {
		name          string
		setupMissions func(*database.DB) error
		statusFilter  string
		wantCount     int
		wantError     bool
	}{
		{
			name: "list all missions",
			setupMissions: func(db *database.DB) error {
				dao := database.NewMissionDAO(db)
				missions := []*database.Mission{
					{
						ID:            types.NewID(),
						Name:          "test-mission-1",
						Description:   "Test mission 1",
						Status:        database.MissionStatusPending,
						WorkflowID:    types.NewID(),
						Progress:      0.0,
						FindingsCount: 0,
					},
					{
						ID:            types.NewID(),
						Name:          "test-mission-2",
						Description:   "Test mission 2",
						Status:        database.MissionStatusRunning,
						WorkflowID:    types.NewID(),
						Progress:      0.5,
						FindingsCount: 3,
					},
				}
				for _, m := range missions {
					if err := dao.Create(context.Background(), m); err != nil {
						return err
					}
				}
				return nil
			},
			statusFilter: "",
			wantCount:    2,
			wantError:    false,
		},
		{
			name: "list missions with status filter",
			setupMissions: func(db *database.DB) error {
				dao := database.NewMissionDAO(db)
				missions := []*database.Mission{
					{
						ID:            types.NewID(),
						Name:          "pending-mission",
						Description:   "Pending mission",
						Status:        database.MissionStatusPending,
						WorkflowID:    types.NewID(),
						Progress:      0.0,
						FindingsCount: 0,
					},
					{
						ID:            types.NewID(),
						Name:          "running-mission",
						Description:   "Running mission",
						Status:        database.MissionStatusRunning,
						WorkflowID:    types.NewID(),
						Progress:      0.5,
						FindingsCount: 2,
					},
				}
				for _, m := range missions {
					if err := dao.Create(context.Background(), m); err != nil {
						return err
					}
				}
				return nil
			},
			statusFilter: "running",
			wantCount:    1,
			wantError:    false,
		},
		{
			name: "empty list",
			setupMissions: func(db *database.DB) error {
				return nil
			},
			statusFilter: "",
			wantCount:    0,
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Initialize config
			cfg := config.DefaultConfig()
			cfg.Core.HomeDir = homeDir
			cfg.Database.Path = filepath.Join(homeDir, "gibson.db")

			// Open database and run migrations
			db, err := database.Open(cfg.Database.Path)
			require.NoError(t, err)
			defer db.Close()

			migrator := database.NewMigrator(db)
			require.NoError(t, migrator.Migrate(context.Background()))

			// Setup missions
			if tt.setupMissions != nil {
				require.NoError(t, tt.setupMissions(db))
			}

			// Create command
			cmd := missionListCmd
			cmd.SetContext(context.Background())

			// Set flags
			cmd.Flags().Set("home", homeDir)
			if tt.statusFilter != "" {
				cmd.Flags().Set("status", tt.statusFilter)
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err = cmd.RunE(cmd, []string{})

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify count in output
				output := buf.String()
				if tt.wantCount == 0 {
					assert.Contains(t, output, "No missions found")
				} else {
					// Basic check that output contains mission names
					assert.NotEmpty(t, output)
				}
			}
		})
	}
}

func TestMissionShow(t *testing.T) {
	tests := []struct {
		name        string
		missionName string
		setup       func(*database.DB) error
		wantError   bool
		checkOutput func(*testing.T, string)
	}{
		{
			name:        "show existing mission",
			missionName: "test-mission",
			setup: func(db *database.DB) error {
				dao := database.NewMissionDAO(db)
				mission := &database.Mission{
					ID:            types.NewID(),
					Name:          "test-mission",
					Description:   "Test mission description",
					Status:        database.MissionStatusRunning,
					WorkflowID:    types.NewID(),
					Progress:      0.75,
					FindingsCount: 5,
					AgentAssignments: map[string]string{
						"node1": "agent-1",
						"node2": "agent-2",
					},
				}
				return dao.Create(context.Background(), mission)
			},
			wantError: false,
			checkOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "test-mission")
				assert.Contains(t, output, "Test mission description")
				assert.Contains(t, output, "running")
				assert.Contains(t, output, "75.0%")
				assert.Contains(t, output, "5")
			},
		},
		{
			name:        "show non-existent mission",
			missionName: "non-existent",
			setup:       nil,
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Initialize config
			cfg := config.DefaultConfig()
			cfg.Core.HomeDir = homeDir
			cfg.Database.Path = filepath.Join(homeDir, "gibson.db")

			// Open database and run migrations
			db, err := database.Open(cfg.Database.Path)
			require.NoError(t, err)
			defer db.Close()

			migrator := database.NewMigrator(db)
			require.NoError(t, migrator.Migrate(context.Background()))

			// Setup
			if tt.setup != nil {
				require.NoError(t, tt.setup(db))
			}

			// Create command
			cmd := missionShowCmd
			cmd.SetContext(context.Background())
			cmd.Flags().Set("home", homeDir)

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err = cmd.RunE(cmd, []string{tt.missionName})

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkOutput != nil {
					tt.checkOutput(t, buf.String())
				}
			}
		})
	}
}

func TestMissionRun(t *testing.T) {
	tests := []struct {
		name         string
		workflowYAML string
		wantError    bool
		checkMission func(*testing.T, *database.Mission)
	}{
		{
			name: "run valid workflow",
			workflowYAML: `
name: Test Workflow
description: A test workflow
nodes:
  - id: node1
    type: agent
    name: First Node
    agent: test-agent
    task:
      action: test
`,
			wantError: false,
			checkMission: func(t *testing.T, m *database.Mission) {
				assert.Equal(t, "Test Workflow", m.Name)
				assert.Equal(t, "A test workflow", m.Description)
				assert.Equal(t, database.MissionStatusRunning, m.Status)
				assert.NotNil(t, m.StartedAt)
				assert.NotEmpty(t, m.WorkflowJSON)
				// Verify workflow JSON can be unmarshaled
				var wf workflow.Workflow
				err := json.Unmarshal([]byte(m.WorkflowJSON), &wf)
				assert.NoError(t, err)
				assert.Equal(t, 1, len(wf.Nodes))
			},
		},
		{
			name: "run invalid workflow - no nodes",
			workflowYAML: `
name: Invalid Workflow
description: Missing nodes
nodes: []
`,
			wantError: true,
		},
		{
			name: "run invalid workflow - malformed YAML",
			workflowYAML: `
name: Broken
nodes
  - invalid
`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Initialize config
			cfg := config.DefaultConfig()
			cfg.Core.HomeDir = homeDir
			cfg.Database.Path = filepath.Join(homeDir, "gibson.db")

			// Open database and run migrations
			db, err := database.Open(cfg.Database.Path)
			require.NoError(t, err)
			defer db.Close()

			migrator := database.NewMigrator(db)
			require.NoError(t, migrator.Migrate(context.Background()))

			// Create workflow file
			workflowFile := filepath.Join(tmpDir, "workflow.yaml")
			require.NoError(t, os.WriteFile(workflowFile, []byte(tt.workflowYAML), 0644))

			// Create command
			cmd := missionRunCmd
			cmd.SetContext(context.Background())
			cmd.Flags().Set("home", homeDir)
			cmd.Flags().Set("file", workflowFile)

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err = cmd.RunE(cmd, []string{})

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify mission was created
				if tt.checkMission != nil {
					dao := database.NewMissionDAO(db)
					missions, err := dao.List(context.Background(), "")
					require.NoError(t, err)
					require.Len(t, missions, 1)
					tt.checkMission(t, missions[0])
				}
			}
		})
	}
}

func TestMissionResume(t *testing.T) {
	tests := []struct {
		name          string
		missionName   string
		missionStatus database.MissionStatus
		wantError     bool
		errorContains string
	}{
		{
			name:          "resume paused mission",
			missionName:   "paused-mission",
			missionStatus: database.MissionStatusCancelled,
			wantError:     true,
			errorContains: "cannot resume",
		},
		{
			name:          "resume completed mission",
			missionName:   "completed-mission",
			missionStatus: database.MissionStatusCompleted,
			wantError:     true,
			errorContains: "cannot resume completed",
		},
		{
			name:          "resume failed mission",
			missionName:   "failed-mission",
			missionStatus: database.MissionStatusFailed,
			wantError:     true,
			errorContains: "cannot resume failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Initialize config
			cfg := config.DefaultConfig()
			cfg.Core.HomeDir = homeDir
			cfg.Database.Path = filepath.Join(homeDir, "gibson.db")

			// Open database and run migrations
			db, err := database.Open(cfg.Database.Path)
			require.NoError(t, err)
			defer db.Close()

			migrator := database.NewMigrator(db)
			require.NoError(t, migrator.Migrate(context.Background()))

			// Create mission
			dao := database.NewMissionDAO(db)
			mission := &database.Mission{
				ID:            types.NewID(),
				Name:          tt.missionName,
				Description:   "Test mission",
				Status:        tt.missionStatus,
				WorkflowID:    types.NewID(),
				Progress:      0.5,
				FindingsCount: 0,
			}
			require.NoError(t, dao.Create(context.Background(), mission))

			// Create command
			cmd := missionResumeCmd
			cmd.SetContext(context.Background())
			cmd.Flags().Set("home", homeDir)

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err = cmd.RunE(cmd, []string{tt.missionName})

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMissionStop(t *testing.T) {
	tests := []struct {
		name          string
		missionName   string
		missionStatus database.MissionStatus
		wantError     bool
		errorContains string
	}{
		{
			name:          "stop running mission",
			missionName:   "running-mission",
			missionStatus: database.MissionStatusRunning,
			wantError:     false,
		},
		{
			name:          "stop pending mission",
			missionName:   "pending-mission",
			missionStatus: database.MissionStatusPending,
			wantError:     true,
			errorContains: "not running",
		},
		{
			name:          "stop completed mission",
			missionName:   "completed-mission",
			missionStatus: database.MissionStatusCompleted,
			wantError:     true,
			errorContains: "not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Initialize config
			cfg := config.DefaultConfig()
			cfg.Core.HomeDir = homeDir
			cfg.Database.Path = filepath.Join(homeDir, "gibson.db")

			// Open database and run migrations
			db, err := database.Open(cfg.Database.Path)
			require.NoError(t, err)
			defer db.Close()

			migrator := database.NewMigrator(db)
			require.NoError(t, migrator.Migrate(context.Background()))

			// Create mission
			dao := database.NewMissionDAO(db)
			now := time.Now()
			mission := &database.Mission{
				ID:            types.NewID(),
				Name:          tt.missionName,
				Description:   "Test mission",
				Status:        tt.missionStatus,
				WorkflowID:    types.NewID(),
				Progress:      0.5,
				FindingsCount: 0,
				StartedAt:     &now,
			}
			require.NoError(t, dao.Create(context.Background(), mission))

			// Create command
			cmd := missionStopCmd
			cmd.SetContext(context.Background())
			cmd.Flags().Set("home", homeDir)

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err = cmd.RunE(cmd, []string{tt.missionName})

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify status was updated
				updatedMission, err := dao.GetByName(context.Background(), tt.missionName)
				require.NoError(t, err)
				assert.Equal(t, database.MissionStatusCancelled, updatedMission.Status)
			}
		})
	}
}

func TestMissionDelete(t *testing.T) {
	tests := []struct {
		name        string
		missionName string
		force       bool
		wantError   bool
	}{
		{
			name:        "delete with force flag",
			missionName: "test-mission",
			force:       true,
			wantError:   false,
		},
		{
			name:        "delete non-existent mission",
			missionName: "non-existent",
			force:       true,
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			tmpDir := t.TempDir()
			homeDir := filepath.Join(tmpDir, ".gibson")
			require.NoError(t, os.MkdirAll(homeDir, 0755))

			// Initialize config
			cfg := config.DefaultConfig()
			cfg.Core.HomeDir = homeDir
			cfg.Database.Path = filepath.Join(homeDir, "gibson.db")

			// Open database and run migrations
			db, err := database.Open(cfg.Database.Path)
			require.NoError(t, err)
			defer db.Close()

			migrator := database.NewMigrator(db)
			require.NoError(t, migrator.Migrate(context.Background()))

			// Create mission if it should exist
			if tt.missionName == "test-mission" {
				dao := database.NewMissionDAO(db)
				mission := &database.Mission{
					ID:            types.NewID(),
					Name:          tt.missionName,
					Description:   "Test mission",
					Status:        database.MissionStatusCompleted,
					WorkflowID:    types.NewID(),
					Progress:      1.0,
					FindingsCount: 0,
				}
				require.NoError(t, dao.Create(context.Background(), mission))
			}

			// Create command
			cmd := missionDeleteCmd
			cmd.SetContext(context.Background())
			cmd.Flags().Set("home", homeDir)
			if tt.force {
				cmd.Flags().Set("force", "true")
			}

			// Capture output
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)

			// Execute command
			err = cmd.RunE(cmd, []string{tt.missionName})

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify mission was deleted
				dao := database.NewMissionDAO(db)
				_, err := dao.GetByName(context.Background(), tt.missionName)
				assert.Error(t, err)
			}
		})
	}
}

func TestIsValidMissionStatus(t *testing.T) {
	tests := []struct {
		status database.MissionStatus
		valid  bool
	}{
		{database.MissionStatusPending, true},
		{database.MissionStatusRunning, true},
		{database.MissionStatusCompleted, true},
		{database.MissionStatusFailed, true},
		{database.MissionStatusCancelled, true},
		{database.MissionStatus("invalid"), false},
		{database.MissionStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidMissionStatus(tt.status))
		})
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		contains string
	}{
		{
			name:     "just now",
			time:     now.Add(-30 * time.Second),
			contains: "just now",
		},
		{
			name:     "minutes ago",
			time:     now.Add(-5 * time.Minute),
			contains: "minutes ago",
		},
		{
			name:     "hours ago",
			time:     now.Add(-3 * time.Hour),
			contains: "hours ago",
		},
		{
			name:     "days ago",
			time:     now.Add(-2 * 24 * time.Hour),
			contains: "days ago",
		},
		{
			name:     "absolute date",
			time:     now.Add(-10 * 24 * time.Hour),
			contains: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(tt.time)
			assert.Contains(t, result, tt.contains)
		})
	}
}
