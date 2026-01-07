package workflow

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDefaultPlanningConfig(t *testing.T) {
	config := DefaultPlanningConfig()

	if config == nil {
		t.Fatal("DefaultPlanningConfig returned nil")
	}

	if config.BudgetAllocation == nil {
		t.Fatal("BudgetAllocation is nil")
	}

	// Verify default budget allocation
	if config.BudgetAllocation.Planning != DefaultStrategicPlanningPercent {
		t.Errorf("Planning = %d, want %d", config.BudgetAllocation.Planning, DefaultStrategicPlanningPercent)
	}
	if config.BudgetAllocation.Scoring != DefaultScoringPercent {
		t.Errorf("Scoring = %d, want %d", config.BudgetAllocation.Scoring, DefaultScoringPercent)
	}
	if config.BudgetAllocation.Replanning != DefaultReplanningPercent {
		t.Errorf("Replanning = %d, want %d", config.BudgetAllocation.Replanning, DefaultReplanningPercent)
	}
	if config.BudgetAllocation.Execution != DefaultExecutionPercent {
		t.Errorf("Execution = %d, want %d", config.BudgetAllocation.Execution, DefaultExecutionPercent)
	}

	// Verify other defaults
	if config.ReplanLimit != 3 {
		t.Errorf("ReplanLimit = %d, want 3", config.ReplanLimit)
	}
	if !config.MemoryQuery {
		t.Error("MemoryQuery = false, want true")
	}
	if config.PlanningTimeout != "30s" {
		t.Errorf("PlanningTimeout = %q, want \"30s\"", config.PlanningTimeout)
	}
}

func TestBudgetAllocationConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *BudgetAllocationConfig
		wantErr bool
	}{
		{
			name: "valid allocation",
			config: &BudgetAllocationConfig{
				Planning:   10,
				Scoring:    5,
				Replanning: 5,
				Execution:  80,
			},
			wantErr: false,
		},
		{
			name: "valid alternative allocation",
			config: &BudgetAllocationConfig{
				Planning:   15,
				Scoring:    10,
				Replanning: 5,
				Execution:  70,
			},
			wantErr: false,
		},
		{
			name: "sum not 100 - too high",
			config: &BudgetAllocationConfig{
				Planning:   10,
				Scoring:    10,
				Replanning: 10,
				Execution:  80,
			},
			wantErr: true,
		},
		{
			name: "sum not 100 - too low",
			config: &BudgetAllocationConfig{
				Planning:   5,
				Scoring:    5,
				Replanning: 5,
				Execution:  70,
			},
			wantErr: true,
		},
		{
			name: "negative planning",
			config: &BudgetAllocationConfig{
				Planning:   -10,
				Scoring:    5,
				Replanning: 5,
				Execution:  100,
			},
			wantErr: true,
		},
		{
			name: "negative scoring",
			config: &BudgetAllocationConfig{
				Planning:   10,
				Scoring:    -5,
				Replanning: 5,
				Execution:  90,
			},
			wantErr: true,
		},
		{
			name: "negative replanning",
			config: &BudgetAllocationConfig{
				Planning:   10,
				Scoring:    5,
				Replanning: -5,
				Execution:  90,
			},
			wantErr: true,
		},
		{
			name: "negative execution",
			config: &BudgetAllocationConfig{
				Planning:   10,
				Scoring:    5,
				Replanning: 5,
				Execution:  -20,
			},
			wantErr: true,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "zero execution - edge case",
			config: &BudgetAllocationConfig{
				Planning:   50,
				Scoring:    25,
				Replanning: 25,
				Execution:  0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPlanningConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *PlanningConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    5,
					Replanning: 5,
					Execution:  80,
				},
				ReplanLimit:     3,
				MemoryQuery:     true,
				PlanningTimeout: "30s",
			},
			wantErr: false,
		},
		{
			name: "valid with different timeout",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   15,
					Scoring:    5,
					Replanning: 5,
					Execution:  75,
				},
				ReplanLimit:     5,
				MemoryQuery:     false,
				PlanningTimeout: "1m",
			},
			wantErr: false,
		},
		{
			name: "nil budget allocation is valid",
			config: &PlanningConfig{
				BudgetAllocation: nil,
				ReplanLimit:      3,
				MemoryQuery:      true,
				PlanningTimeout:  "30s",
			},
			wantErr: false,
		},
		{
			name: "invalid budget allocation",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    10,
					Replanning: 10,
					Execution:  80,
				},
				ReplanLimit:     3,
				MemoryQuery:     true,
				PlanningTimeout: "30s",
			},
			wantErr: true,
		},
		{
			name: "negative replan limit",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    5,
					Replanning: 5,
					Execution:  80,
				},
				ReplanLimit:     -1,
				MemoryQuery:     true,
				PlanningTimeout: "30s",
			},
			wantErr: true,
		},
		{
			name: "invalid timeout format",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    5,
					Replanning: 5,
					Execution:  80,
				},
				ReplanLimit:     3,
				MemoryQuery:     true,
				PlanningTimeout: "invalid",
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    5,
					Replanning: 5,
					Execution:  80,
				},
				ReplanLimit:     3,
				MemoryQuery:     true,
				PlanningTimeout: "-30s",
			},
			wantErr: true,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "zero replan limit is valid (disables replanning)",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    5,
					Replanning: 5,
					Execution:  80,
				},
				ReplanLimit:     0,
				MemoryQuery:     true,
				PlanningTimeout: "30s",
			},
			wantErr: false,
		},
		{
			name: "empty timeout string is valid",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   10,
					Scoring:    5,
					Replanning: 5,
					Execution:  80,
				},
				ReplanLimit:     3,
				MemoryQuery:     true,
				PlanningTimeout: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPlanningConfig_ToBudgetAllocation(t *testing.T) {
	tests := []struct {
		name   string
		config *PlanningConfig
		want   *BudgetAllocationConfig
	}{
		{
			name: "non-nil allocation",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   15,
					Scoring:    10,
					Replanning: 5,
					Execution:  70,
				},
			},
			want: &BudgetAllocationConfig{
				Planning:   15,
				Scoring:    10,
				Replanning: 5,
				Execution:  70,
			},
		},
		{
			name: "nil allocation",
			config: &PlanningConfig{
				BudgetAllocation: nil,
			},
			want: nil,
		},
		{
			name:   "nil config",
			config: nil,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ToBudgetAllocation()
			if got == nil && tt.want == nil {
				return
			}
			if got == nil || tt.want == nil {
				t.Errorf("ToBudgetAllocation() = %v, want %v", got, tt.want)
				return
			}
			if got.Planning != tt.want.Planning ||
				got.Scoring != tt.want.Scoring ||
				got.Replanning != tt.want.Replanning ||
				got.Execution != tt.want.Execution {
				t.Errorf("ToBudgetAllocation() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPlanningConfig_GetPlanningTimeoutDuration(t *testing.T) {
	tests := []struct {
		name   string
		config *PlanningConfig
		want   time.Duration
	}{
		{
			name: "valid timeout",
			config: &PlanningConfig{
				PlanningTimeout: "45s",
			},
			want: 45 * time.Second,
		},
		{
			name: "minute timeout",
			config: &PlanningConfig{
				PlanningTimeout: "2m",
			},
			want: 2 * time.Minute,
		},
		{
			name: "empty timeout returns default",
			config: &PlanningConfig{
				PlanningTimeout: "",
			},
			want: 30 * time.Second,
		},
		{
			name: "invalid timeout returns default",
			config: &PlanningConfig{
				PlanningTimeout: "invalid",
			},
			want: 30 * time.Second,
		},
		{
			name:   "nil config returns default",
			config: nil,
			want:   30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetPlanningTimeoutDuration()
			if got != tt.want {
				t.Errorf("GetPlanningTimeoutDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanningConfig_ApplyDefaults(t *testing.T) {
	tests := []struct {
		name   string
		config *PlanningConfig
		check  func(*testing.T, *PlanningConfig)
	}{
		{
			name: "apply all defaults",
			config: &PlanningConfig{
				BudgetAllocation: nil,
				ReplanLimit:      0,
				PlanningTimeout:  "",
			},
			check: func(t *testing.T, cfg *PlanningConfig) {
				if cfg.BudgetAllocation == nil {
					t.Error("BudgetAllocation should not be nil after defaults")
				}
				if cfg.BudgetAllocation.Planning != DefaultStrategicPlanningPercent {
					t.Errorf("Planning = %d, want %d", cfg.BudgetAllocation.Planning, DefaultStrategicPlanningPercent)
				}
				if cfg.ReplanLimit != 3 {
					t.Errorf("ReplanLimit = %d, want 3", cfg.ReplanLimit)
				}
				if cfg.PlanningTimeout != "30s" {
					t.Errorf("PlanningTimeout = %q, want \"30s\"", cfg.PlanningTimeout)
				}
			},
		},
		{
			name: "preserve existing values",
			config: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   15,
					Scoring:    10,
					Replanning: 5,
					Execution:  70,
				},
				ReplanLimit:     5,
				PlanningTimeout: "1m",
			},
			check: func(t *testing.T, cfg *PlanningConfig) {
				if cfg.BudgetAllocation.Planning != 15 {
					t.Errorf("Planning = %d, want 15", cfg.BudgetAllocation.Planning)
				}
				if cfg.ReplanLimit != 5 {
					t.Errorf("ReplanLimit = %d, want 5", cfg.ReplanLimit)
				}
				if cfg.PlanningTimeout != "1m" {
					t.Errorf("PlanningTimeout = %q, want \"1m\"", cfg.PlanningTimeout)
				}
			},
		},
		{
			name:   "nil config is safe",
			config: nil,
			check:  func(t *testing.T, cfg *PlanningConfig) {}, // Should not crash
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.ApplyDefaults()
			tt.check(t, tt.config)
		})
	}
}

func TestPlanningConfig_YAMLSerialization(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want *PlanningConfig
	}{
		{
			name: "full config",
			yaml: `
replan_limit: 5
memory_query: true
planning_timeout: "45s"
budget_allocation:
  planning: 15
  scoring: 5
  replanning: 5
  execution: 75
`,
			want: &PlanningConfig{
				BudgetAllocation: &BudgetAllocationConfig{
					Planning:   15,
					Scoring:    5,
					Replanning: 5,
					Execution:  75,
				},
				ReplanLimit:     5,
				MemoryQuery:     true,
				PlanningTimeout: "45s",
			},
		},
		{
			name: "minimal config",
			yaml: `
replan_limit: 2
`,
			want: &PlanningConfig{
				ReplanLimit: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PlanningConfig
			if err := yaml.Unmarshal([]byte(tt.yaml), &got); err != nil {
				t.Fatalf("yaml.Unmarshal() error = %v", err)
			}

			// Compare fields
			if got.ReplanLimit != tt.want.ReplanLimit {
				t.Errorf("ReplanLimit = %d, want %d", got.ReplanLimit, tt.want.ReplanLimit)
			}
			if got.MemoryQuery != tt.want.MemoryQuery {
				t.Errorf("MemoryQuery = %v, want %v", got.MemoryQuery, tt.want.MemoryQuery)
			}
			if got.PlanningTimeout != tt.want.PlanningTimeout {
				t.Errorf("PlanningTimeout = %q, want %q", got.PlanningTimeout, tt.want.PlanningTimeout)
			}

			if tt.want.BudgetAllocation == nil {
				if got.BudgetAllocation != nil {
					t.Error("BudgetAllocation should be nil")
				}
			} else {
				if got.BudgetAllocation == nil {
					t.Fatal("BudgetAllocation is nil")
				}
				if got.BudgetAllocation.Planning != tt.want.BudgetAllocation.Planning {
					t.Errorf("Planning = %d, want %d", got.BudgetAllocation.Planning, tt.want.BudgetAllocation.Planning)
				}
				if got.BudgetAllocation.Scoring != tt.want.BudgetAllocation.Scoring {
					t.Errorf("Scoring = %d, want %d", got.BudgetAllocation.Scoring, tt.want.BudgetAllocation.Scoring)
				}
			}
		})
	}
}

func TestPlanningConfig_YAMLRoundtrip(t *testing.T) {
	original := &PlanningConfig{
		BudgetAllocation: &BudgetAllocationConfig{
			Planning:   20,
			Scoring:    10,
			Replanning: 10,
			Execution:  60,
		},
		ReplanLimit:     7,
		MemoryQuery:     false,
		PlanningTimeout: "2m",
	}

	// Marshal to YAML
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	// Unmarshal back
	var roundtrip PlanningConfig
	if err := yaml.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	// Compare
	if roundtrip.ReplanLimit != original.ReplanLimit {
		t.Errorf("ReplanLimit = %d, want %d", roundtrip.ReplanLimit, original.ReplanLimit)
	}
	if roundtrip.MemoryQuery != original.MemoryQuery {
		t.Errorf("MemoryQuery = %v, want %v", roundtrip.MemoryQuery, original.MemoryQuery)
	}
	if roundtrip.PlanningTimeout != original.PlanningTimeout {
		t.Errorf("PlanningTimeout = %q, want %q", roundtrip.PlanningTimeout, original.PlanningTimeout)
	}
	if roundtrip.BudgetAllocation.Planning != original.BudgetAllocation.Planning {
		t.Errorf("Planning = %d, want %d", roundtrip.BudgetAllocation.Planning, original.BudgetAllocation.Planning)
	}
}
