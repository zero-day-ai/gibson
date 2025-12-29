package payload

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestEffectivenessTracker_GetPayloadStats(t *testing.T) {
	tests := []struct {
		name           string
		setupPayload   func(*testing.T, PayloadStore) types.ID
		setupExecutions func(*testing.T, ExecutionStore, types.ID)
		wantStats      func(*testing.T, *PayloadStats)
		wantErr        bool
	}{
		{
			name: "no executions",
			setupPayload: func(t *testing.T, store PayloadStore) types.ID {
				p := createAnalyticsTestPayload("test-payload")
				ctx := context.Background()
				err := store.Save(ctx, p)
				require.NoError(t, err)
				return p.ID
			},
			setupExecutions: func(t *testing.T, store ExecutionStore, id types.ID) {
				// No executions
			},
			wantStats: func(t *testing.T, stats *PayloadStats) {
				assert.Equal(t, 0, stats.TotalExecutions)
				assert.Equal(t, 0, stats.SuccessfulAttacks)
				assert.Equal(t, 0, stats.FailedExecutions)
				assert.Equal(t, 0.0, stats.SuccessRate)
				assert.NotNil(t, stats.TargetTypeBreakdown)
			},
		},
		{
			name: "single successful execution",
			setupPayload: func(t *testing.T, store PayloadStore) types.ID {
				p := createAnalyticsTestPayload("test-payload")
				ctx := context.Background()
				err := store.Save(ctx, p)
				require.NoError(t, err)
				return p.ID
			},
			setupExecutions: func(t *testing.T, store ExecutionStore, payloadID types.ID) {
				ctx := context.Background()
				exec := createAnalyticsTestExecution(payloadID, true)
				err := store.Save(ctx, exec)
				require.NoError(t, err)
			},
			wantStats: func(t *testing.T, stats *PayloadStats) {
				assert.Equal(t, 1, stats.TotalExecutions)
				assert.Equal(t, 1, stats.SuccessfulAttacks)
				assert.Equal(t, 0, stats.FailedExecutions)
				assert.Equal(t, 1.0, stats.SuccessRate)
				assert.Greater(t, stats.ConfidenceLevel, 0.0)
				assert.InDelta(t, 0.95, stats.AverageConfidence, 0.01)
			},
		},
		{
			name: "multiple mixed executions",
			setupPayload: func(t *testing.T, store PayloadStore) types.ID {
				p := createAnalyticsTestPayload("test-payload")
				ctx := context.Background()
				err := store.Save(ctx, p)
				require.NoError(t, err)
				return p.ID
			},
			setupExecutions: func(t *testing.T, store ExecutionStore, payloadID types.ID) {
				ctx := context.Background()

				// 3 successful, 2 failed
				for i := 0; i < 3; i++ {
					exec := createAnalyticsTestExecution(payloadID, true)
					exec.FindingCreated = true
					err := store.Save(ctx, exec)
					require.NoError(t, err)
				}

				for i := 0; i < 2; i++ {
					exec := createAnalyticsTestExecution(payloadID, false)
					err := store.Save(ctx, exec)
					require.NoError(t, err)
				}
			},
			wantStats: func(t *testing.T, stats *PayloadStats) {
				assert.Equal(t, 5, stats.TotalExecutions)
				assert.Equal(t, 3, stats.SuccessfulAttacks)
				assert.Equal(t, 2, stats.FailedExecutions)
				assert.Equal(t, 0.6, stats.SuccessRate)
				assert.Equal(t, 3, stats.FindingsCreated)
				assert.Equal(t, 1.0, stats.FindingCreationRate)
			},
		},
		{
			name: "target type breakdown",
			setupPayload: func(t *testing.T, store PayloadStore) types.ID {
				p := createAnalyticsTestPayload("test-payload")
				ctx := context.Background()
				err := store.Save(ctx, p)
				require.NoError(t, err)
				return p.ID
			},
			setupExecutions: func(t *testing.T, store ExecutionStore, payloadID types.ID) {
				ctx := context.Background()

				// 2 successful LLM Chat executions
				for i := 0; i < 2; i++ {
					exec := createAnalyticsTestExecution(payloadID, true)
					exec.TargetType = types.TargetTypeLLMChat
					err := store.Save(ctx, exec)
					require.NoError(t, err)
				}

				// 1 failed RAG execution
				exec := createAnalyticsTestExecution(payloadID, false)
				exec.TargetType = types.TargetTypeRAG
				err := store.Save(ctx, exec)
				require.NoError(t, err)
			},
			wantStats: func(t *testing.T, stats *PayloadStats) {
				assert.Equal(t, 3, stats.TotalExecutions)

				// Check LLM Chat breakdown
				llmChat, exists := stats.TargetTypeBreakdown[types.TargetTypeLLMChat]
				require.True(t, exists)
				assert.Equal(t, 2, llmChat.Executions)
				assert.Equal(t, 2, llmChat.Successes)
				assert.Equal(t, 1.0, llmChat.SuccessRate)

				// Check RAG breakdown
				rag, exists := stats.TargetTypeBreakdown[types.TargetTypeRAG]
				require.True(t, exists)
				assert.Equal(t, 1, rag.Executions)
				assert.Equal(t, 0, rag.Successes)
				assert.Equal(t, 0.0, rag.SuccessRate)
			},
		},
		{
			name: "trending calculation",
			setupPayload: func(t *testing.T, store PayloadStore) types.ID {
				p := createAnalyticsTestPayload("test-payload")
				ctx := context.Background()
				err := store.Save(ctx, p)
				require.NoError(t, err)
				return p.ID
			},
			setupExecutions: func(t *testing.T, store ExecutionStore, payloadID types.ID) {
				ctx := context.Background()

				// Old executions (>30 days): 1 success, 3 failures
				oldTime := time.Now().Add(-40 * 24 * time.Hour)
				exec := createAnalyticsTestExecution(payloadID, true)
				exec.CreatedAt = oldTime
				err := store.Save(ctx, exec)
				require.NoError(t, err)

				for i := 0; i < 3; i++ {
					exec := createAnalyticsTestExecution(payloadID, false)
					exec.CreatedAt = oldTime
					err := store.Save(ctx, exec)
					require.NoError(t, err)
				}

				// Recent executions (<30 days): 3 successes, 1 failure
				recentTime := time.Now().Add(-5 * 24 * time.Hour)
				for i := 0; i < 3; i++ {
					exec := createAnalyticsTestExecution(payloadID, true)
					exec.CreatedAt = recentTime
					err := store.Save(ctx, exec)
					require.NoError(t, err)
				}

				exec = createAnalyticsTestExecution(payloadID, false)
				exec.CreatedAt = recentTime
				err = store.Save(ctx, exec)
				require.NoError(t, err)
			},
			wantStats: func(t *testing.T, stats *PayloadStats) {
				assert.Equal(t, 8, stats.TotalExecutions)
				assert.Equal(t, 4, stats.SuccessfulAttacks)
				assert.Equal(t, 0.5, stats.SuccessRate)        // Overall 4/8
				assert.Equal(t, 0.75, stats.RecentSuccessRate) // Recent 3/4
				assert.Equal(t, "up", stats.Trending)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, payloadStore, cleanup := setupTestStore(t)
			defer cleanup()

			execStore := NewExecutionStore(db)
			tracker := NewEffectivenessTracker(execStore, payloadStore)

			payloadID := tt.setupPayload(t, payloadStore)
			tt.setupExecutions(t, execStore, payloadID)

			ctx := context.Background()
			stats, err := tracker.GetPayloadStats(ctx, payloadID)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, stats)
			assert.Equal(t, payloadID, stats.PayloadID)

			if tt.wantStats != nil {
				tt.wantStats(t, stats)
			}
		})
	}
}

func TestEffectivenessTracker_GetCategoryStats(t *testing.T) {
	tests := []struct {
		name        string
		category    PayloadCategory
		setupData   func(*testing.T, PayloadStore, ExecutionStore)
		wantStats   func(*testing.T, *CategoryStats)
		wantErr     bool
	}{
		{
			name:     "empty category",
			category: CategoryJailbreak,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				// No payloads in category
			},
			wantStats: func(t *testing.T, stats *CategoryStats) {
				assert.Equal(t, CategoryJailbreak, stats.Category)
				assert.Equal(t, 0, stats.TotalPayloads)
				assert.Equal(t, 0, stats.TotalExecutions)
			},
		},
		{
			name:     "category with multiple payloads",
			category: CategoryJailbreak,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// Create 2 jailbreak payloads
				for i := 0; i < 2; i++ {
					p := createAnalyticsTestPayload("jailbreak")
					p.Categories = []PayloadCategory{CategoryJailbreak}
					err := ps.Save(ctx, p)
					require.NoError(t, err)

					// Add executions
					for j := 0; j < 3; j++ {
						exec := createAnalyticsTestExecution(p.ID, j < 2) // 2 success, 1 fail
						exec.FindingCreated = j < 2
						err := es.Save(ctx, exec)
						require.NoError(t, err)
					}
				}
			},
			wantStats: func(t *testing.T, stats *CategoryStats) {
				assert.Equal(t, CategoryJailbreak, stats.Category)
				assert.Equal(t, 2, stats.TotalPayloads)
				assert.Equal(t, 6, stats.TotalExecutions)      // 2 payloads * 3 execs
				assert.Equal(t, 4, stats.SuccessfulAttacks)    // 2 payloads * 2 success
				assert.Equal(t, 2, stats.FailedExecutions)     // 2 payloads * 1 fail
				assert.InDelta(t, 0.667, stats.SuccessRate, 0.01)
				assert.Equal(t, 4, stats.FindingsCreated)
				assert.Equal(t, 1.0, stats.FindingRate)
				assert.Len(t, stats.TopPayloads, 2)
			},
		},
		{
			name:     "payloads with no executions excluded",
			category: CategoryPromptInjection,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// Payload with executions
				p1 := createAnalyticsTestPayload("active")
				p1.Categories = []PayloadCategory{CategoryPromptInjection}
				err := ps.Save(ctx, p1)
				require.NoError(t, err)

				exec := createAnalyticsTestExecution(p1.ID, true)
				err = es.Save(ctx, exec)
				require.NoError(t, err)

				// Payload without executions
				p2 := createAnalyticsTestPayload("inactive")
				p2.Categories = []PayloadCategory{CategoryPromptInjection}
				err = ps.Save(ctx, p2)
				require.NoError(t, err)
			},
			wantStats: func(t *testing.T, stats *CategoryStats) {
				assert.Equal(t, 2, stats.TotalPayloads)
				assert.Equal(t, 1, stats.TotalExecutions)
				assert.Len(t, stats.TopPayloads, 1) // Only payload with executions
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, payloadStore, cleanup := setupTestStore(t)
			defer cleanup()

			execStore := NewExecutionStore(db)
			tracker := NewEffectivenessTracker(execStore, payloadStore)

			tt.setupData(t, payloadStore, execStore)

			ctx := context.Background()
			stats, err := tracker.GetCategoryStats(ctx, tt.category)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, stats)

			if tt.wantStats != nil {
				tt.wantStats(t, stats)
			}
		})
	}
}

func TestEffectivenessTracker_GetTargetTypeStats(t *testing.T) {
	tests := []struct {
		name       string
		targetType types.TargetType
		setupData  func(*testing.T, PayloadStore, ExecutionStore)
		wantStats  func(*testing.T, *TargetTypeStats)
		wantErr    bool
	}{
		{
			name:       "no executions for target type",
			targetType: types.TargetTypeLLMChat,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				// No payloads
			},
			wantStats: func(t *testing.T, stats *TargetTypeStats) {
				assert.Equal(t, types.TargetTypeLLMChat, stats.TargetType)
				assert.Equal(t, 0, stats.TotalExecutions)
			},
		},
		{
			name:       "multiple categories against same target type",
			targetType: types.TargetTypeLLMChat,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// Jailbreak payload
				p1 := createAnalyticsTestPayload("jailbreak")
				p1.Categories = []PayloadCategory{CategoryJailbreak}
				err := ps.Save(ctx, p1)
				require.NoError(t, err)

				exec1 := createAnalyticsTestExecution(p1.ID, true)
				exec1.TargetType = types.TargetTypeLLMChat
				err = es.Save(ctx, exec1)
				require.NoError(t, err)

				// Prompt injection payload
				p2 := createAnalyticsTestPayload("injection")
				p2.Categories = []PayloadCategory{CategoryPromptInjection}
				err = ps.Save(ctx, p2)
				require.NoError(t, err)

				exec2 := createAnalyticsTestExecution(p2.ID, true)
				exec2.TargetType = types.TargetTypeLLMChat
				err = es.Save(ctx, exec2)
				require.NoError(t, err)

				exec3 := createAnalyticsTestExecution(p2.ID, false)
				exec3.TargetType = types.TargetTypeLLMChat
				err = es.Save(ctx, exec3)
				require.NoError(t, err)
			},
			wantStats: func(t *testing.T, stats *TargetTypeStats) {
				assert.Equal(t, types.TargetTypeLLMChat, stats.TargetType)
				assert.Equal(t, 3, stats.TotalExecutions)
				assert.Equal(t, 2, stats.SuccessfulAttacks)
				assert.InDelta(t, 0.667, stats.SuccessRate, 0.01)

				// Check category breakdown
				assert.Len(t, stats.CategoryBreakdown, 2)

				jailbreak, exists := stats.CategoryBreakdown[CategoryJailbreak]
				require.True(t, exists)
				assert.Equal(t, 1, jailbreak.Executions)
				assert.Equal(t, 1, jailbreak.Successes)
				assert.Equal(t, 1.0, jailbreak.SuccessRate)

				injection, exists := stats.CategoryBreakdown[CategoryPromptInjection]
				require.True(t, exists)
				assert.Equal(t, 2, injection.Executions)
				assert.Equal(t, 1, injection.Successes)
				assert.Equal(t, 0.5, injection.SuccessRate)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, payloadStore, cleanup := setupTestStore(t)
			defer cleanup()

			execStore := NewExecutionStore(db)
			tracker := NewEffectivenessTracker(execStore, payloadStore)

			tt.setupData(t, payloadStore, execStore)

			ctx := context.Background()
			stats, err := tracker.GetTargetTypeStats(ctx, tt.targetType)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, stats)

			if tt.wantStats != nil {
				tt.wantStats(t, stats)
			}
		})
	}
}

func TestEffectivenessTracker_GetRecommendations(t *testing.T) {
	tests := []struct {
		name      string
		filter    RecommendationFilter
		setupData func(*testing.T, PayloadStore, ExecutionStore)
		wantRecs  func(*testing.T, []*PayloadRecommendation)
		wantErr   bool
	}{
		{
			name: "minimum success rate filter",
			filter: RecommendationFilter{
				MinSuccessRate: 0.8,
				MinExecutions:  2,
				Limit:          10,
			},
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// High success payload
				p1 := createAnalyticsTestPayload("high-success")
				err := ps.Save(ctx, p1)
				require.NoError(t, err)

				for i := 0; i < 5; i++ {
					exec := createAnalyticsTestExecution(p1.ID, i < 4) // 4/5 = 0.8
					err := es.Save(ctx, exec)
					require.NoError(t, err)
				}

				// Low success payload
				p2 := createAnalyticsTestPayload("low-success")
				err = ps.Save(ctx, p2)
				require.NoError(t, err)

				for i := 0; i < 5; i++ {
					exec := createAnalyticsTestExecution(p2.ID, i < 2) // 2/5 = 0.4
					err := es.Save(ctx, exec)
					require.NoError(t, err)
				}
			},
			wantRecs: func(t *testing.T, recs []*PayloadRecommendation) {
				assert.Len(t, recs, 1)
				assert.Equal(t, "high-success", recs[0].Payload.Name)
				assert.Greater(t, recs[0].Score, 0.0)
				assert.NotEmpty(t, recs[0].Reason)
			},
		},
		{
			name: "category filter",
			filter: RecommendationFilter{
				Category:       ptrTo(CategoryJailbreak),
				MinSuccessRate: 0.0,
				MinExecutions:  1,
				Limit:          10,
			},
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// Jailbreak payload
				p1 := createAnalyticsTestPayload("jailbreak")
				p1.Categories = []PayloadCategory{CategoryJailbreak}
				err := ps.Save(ctx, p1)
				require.NoError(t, err)

				exec := createAnalyticsTestExecution(p1.ID, true)
				err = es.Save(ctx, exec)
				require.NoError(t, err)

				// Prompt injection payload
				p2 := createAnalyticsTestPayload("injection")
				p2.Categories = []PayloadCategory{CategoryPromptInjection}
				err = ps.Save(ctx, p2)
				require.NoError(t, err)

				exec = createAnalyticsTestExecution(p2.ID, true)
				err = es.Save(ctx, exec)
				require.NoError(t, err)
			},
			wantRecs: func(t *testing.T, recs []*PayloadRecommendation) {
				assert.Len(t, recs, 1)
				assert.Equal(t, "jailbreak", recs[0].Payload.Name)
			},
		},
		{
			name: "sorted by score",
			filter: RecommendationFilter{
				MinSuccessRate: 0.0,
				MinExecutions:  1,
				Limit:          10,
			},
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// Medium success payload (0.5 success rate)
				p1 := createAnalyticsTestPayload("medium")
				err := ps.Save(ctx, p1)
				require.NoError(t, err)

				for i := 0; i < 2; i++ {
					exec := createAnalyticsTestExecution(p1.ID, i < 1)
					err := es.Save(ctx, exec)
					require.NoError(t, err)
				}

				// High success payload (1.0 success rate)
				p2 := createAnalyticsTestPayload("high")
				err = ps.Save(ctx, p2)
				require.NoError(t, err)

				for i := 0; i < 2; i++ {
					exec := createAnalyticsTestExecution(p2.ID, true)
					err := es.Save(ctx, exec)
					require.NoError(t, err)
				}
			},
			wantRecs: func(t *testing.T, recs []*PayloadRecommendation) {
				assert.Len(t, recs, 2)
				// Should be sorted by score, high first
				assert.Equal(t, "high", recs[0].Payload.Name)
				assert.Equal(t, "medium", recs[1].Payload.Name)
				assert.Greater(t, recs[0].Score, recs[1].Score)
			},
		},
		{
			name: "limit applied",
			filter: RecommendationFilter{
				MinSuccessRate: 0.0,
				MinExecutions:  1,
				Limit:          2,
			},
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				// Create 3 payloads
				for i := 0; i < 3; i++ {
					p := createAnalyticsTestPayload("payload")
					err := ps.Save(ctx, p)
					require.NoError(t, err)

					exec := createAnalyticsTestExecution(p.ID, true)
					err = es.Save(ctx, exec)
					require.NoError(t, err)
				}
			},
			wantRecs: func(t *testing.T, recs []*PayloadRecommendation) {
				assert.Len(t, recs, 2) // Limited to 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, payloadStore, cleanup := setupTestStore(t)
			defer cleanup()

			execStore := NewExecutionStore(db)
			tracker := NewEffectivenessTracker(execStore, payloadStore)

			tt.setupData(t, payloadStore, execStore)

			ctx := context.Background()
			recs, err := tracker.GetRecommendations(ctx, tt.filter)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, recs)

			if tt.wantRecs != nil {
				tt.wantRecs(t, recs)
			}
		})
	}
}

func TestEffectivenessTracker_ExportStats(t *testing.T) {
	tests := []struct {
		name       string
		format     ExportFormat
		setupData  func(*testing.T, PayloadStore, ExecutionStore)
		validate   func(*testing.T, []byte)
		wantErr    bool
	}{
		{
			name:   "export JSON format",
			format: ExportFormatJSON,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				p := createAnalyticsTestPayload("test")
				err := ps.Save(ctx, p)
				require.NoError(t, err)

				exec := createAnalyticsTestExecution(p.ID, true)
				err = es.Save(ctx, exec)
				require.NoError(t, err)
			},
			validate: func(t *testing.T, data []byte) {
				var stats []*PayloadStats
				err := json.Unmarshal(data, &stats)
				require.NoError(t, err)
				assert.Len(t, stats, 1)
				assert.Equal(t, "test", stats[0].PayloadName)
				assert.Equal(t, 1, stats[0].TotalExecutions)
			},
		},
		{
			name:   "export CSV format",
			format: ExportFormatCSV,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				ctx := context.Background()

				p := createAnalyticsTestPayload("test")
				err := ps.Save(ctx, p)
				require.NoError(t, err)

				exec := createAnalyticsTestExecution(p.ID, true)
				err = es.Save(ctx, exec)
				require.NoError(t, err)
			},
			validate: func(t *testing.T, data []byte) {
				reader := csv.NewReader(bytes.NewReader(data))
				records, err := reader.ReadAll()
				require.NoError(t, err)
				assert.Len(t, records, 2) // Header + 1 data row

				// Check header
				assert.Equal(t, "Payload ID", records[0][0])
				assert.Equal(t, "Payload Name", records[0][1])

				// Check data
				assert.Equal(t, "test", records[1][1])
			},
		},
		{
			name:   "empty data exports successfully",
			format: ExportFormatJSON,
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				// No data
			},
			validate: func(t *testing.T, data []byte) {
				var stats []*PayloadStats
				err := json.Unmarshal(data, &stats)
				require.NoError(t, err)
				assert.Len(t, stats, 0)
			},
		},
		{
			name:   "invalid format",
			format: ExportFormat("invalid"),
			setupData: func(t *testing.T, ps PayloadStore, es ExecutionStore) {
				// No setup needed
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, payloadStore, cleanup := setupTestStore(t)
			defer cleanup()

			execStore := NewExecutionStore(db)
			tracker := NewEffectivenessTracker(execStore, payloadStore)

			tt.setupData(t, payloadStore, execStore)

			var buf bytes.Buffer
			ctx := context.Background()
			err := tracker.ExportStats(ctx, tt.format, &buf)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, buf.Bytes())
			}
		})
	}
}

func TestCalculateConfidenceLevel(t *testing.T) {
	tests := []struct {
		name       string
		sampleSize int
		wantMin    float64
		wantMax    float64
	}{
		{"zero samples", 0, 0.0, 0.0},
		{"one sample", 1, 0.0, 0.0},
		{"four samples", 4, 0.4, 0.6},
		{"nine samples", 9, 0.6, 0.7},
		{"25 samples", 25, 0.75, 0.85},
		{"100 samples", 100, 0.85, 0.95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := calculateConfidenceLevel(tt.sampleSize)
			assert.GreaterOrEqual(t, confidence, tt.wantMin)
			assert.LessOrEqual(t, confidence, tt.wantMax)
			assert.GreaterOrEqual(t, confidence, 0.0)
			assert.LessOrEqual(t, confidence, 1.0)
		})
	}
}

func TestCalculateTrend(t *testing.T) {
	tests := []struct {
		name        string
		overallRate float64
		recentRate  float64
		want        string
	}{
		{"trending up", 0.5, 0.7, "up"},
		{"trending down", 0.7, 0.5, "down"},
		{"stable", 0.5, 0.52, "stable"},
		{"exactly same", 0.5, 0.5, "stable"},
		{"barely up", 0.5, 0.56, "up"}, // 0.56 - 0.5 = 0.06 difference is > 0.05 threshold
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trend := calculateTrend(tt.overallRate, tt.recentRate)
			assert.Equal(t, tt.want, trend)
		})
	}
}

func TestCalculateMedianDuration(t *testing.T) {
	tests := []struct {
		name      string
		durations []time.Duration
		want      time.Duration
	}{
		{
			name:      "empty list",
			durations: []time.Duration{},
			want:      0,
		},
		{
			name:      "single value",
			durations: []time.Duration{5 * time.Second},
			want:      5 * time.Second,
		},
		{
			name:      "odd number of values",
			durations: []time.Duration{1 * time.Second, 5 * time.Second, 3 * time.Second},
			want:      3 * time.Second,
		},
		{
			name:      "even number of values",
			durations: []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second},
			want:      (2*time.Second + 3*time.Second) / 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			median := calculateMedianDuration(tt.durations)
			assert.Equal(t, tt.want, median)
		})
	}
}

func TestCalculateRecommendationScore(t *testing.T) {
	tests := []struct {
		name  string
		stats *PayloadStats
		want  func(float64) bool
	}{
		{
			name: "perfect payload",
			stats: &PayloadStats{
				SuccessRate:         1.0,
				ConfidenceLevel:     1.0,
				FindingCreationRate: 1.0,
				Trending:            "up",
			},
			want: func(score float64) bool {
				return score > 0.9 && score <= 1.0
			},
		},
		{
			name: "poor payload",
			stats: &PayloadStats{
				SuccessRate:         0.1,
				ConfidenceLevel:     0.1,
				FindingCreationRate: 0.1,
				Trending:            "down",
			},
			want: func(score float64) bool {
				return score >= 0.0 && score < 0.2
			},
		},
		{
			name: "average payload",
			stats: &PayloadStats{
				SuccessRate:         0.5,
				ConfidenceLevel:     0.5,
				FindingCreationRate: 0.5,
				Trending:            "stable",
			},
			want: func(score float64) bool {
				return score > 0.4 && score < 0.6
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateRecommendationScore(tt.stats, RecommendationFilter{})
			assert.True(t, tt.want(score), "score %f does not match expectations", score)
			assert.GreaterOrEqual(t, score, 0.0)
			assert.LessOrEqual(t, score, 1.0)
		})
	}
}

func TestGenerateRecommendationReason(t *testing.T) {
	tests := []struct {
		name          string
		stats         *PayloadStats
		wantContains  []string
	}{
		{
			name: "high success rate",
			stats: &PayloadStats{
				SuccessRate:     0.9,
				ConfidenceLevel: 0.5,
				TotalExecutions: 10,
			},
			wantContains: []string{"High success rate"},
		},
		{
			name: "high confidence",
			stats: &PayloadStats{
				SuccessRate:     0.5,
				ConfidenceLevel: 0.9,
				TotalExecutions: 100,
			},
			wantContains: []string{"High confidence"},
		},
		{
			name: "reliable findings",
			stats: &PayloadStats{
				SuccessRate:         0.5,
				ConfidenceLevel:     0.5,
				FindingCreationRate: 0.9,
			},
			wantContains: []string{"Reliably creates findings"},
		},
		{
			name: "trending up",
			stats: &PayloadStats{
				SuccessRate:     0.5,
				ConfidenceLevel: 0.5,
				Trending:        "up",
			},
			wantContains: []string{"Trending up"},
		},
		{
			name: "multiple reasons",
			stats: &PayloadStats{
				SuccessRate:         0.9,
				ConfidenceLevel:     0.9,
				FindingCreationRate: 0.9,
				Trending:            "up",
				TotalExecutions:     100,
			},
			wantContains: []string{"High success rate", "High confidence", "Reliably creates findings", "Trending up"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := generateRecommendationReason(tt.stats, RecommendationFilter{})
			assert.NotEmpty(t, reason)

			for _, expected := range tt.wantContains {
				assert.Contains(t, reason, expected)
			}
		})
	}
}

// Helper functions for analytics tests

func createAnalyticsTestPayload(name string) *Payload {
	id := types.NewID()
	return &Payload{
		ID:          id,
		Name:        name,
		Version:     "1.0.0",
		Description: "Test payload",
		Categories:  []PayloadCategory{CategoryJailbreak},
		Tags:        []string{"test"},
		Severity:    agent.SeverityMedium,
		Template:    "Test template",
		Parameters:  []ParameterDef{},
		SuccessIndicators: []SuccessIndicator{
			{
				Type:   IndicatorContains,
				Value:  "success",
				Weight: 1.0,
			},
		},
		TargetTypes:     []string{"llm_chat"},
		MitreTechniques: []string{"AML.T0051"},
		Enabled:         true,
		BuiltIn:         false,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func createAnalyticsTestExecution(payloadID types.ID, success bool) *Execution {
	id := types.NewID()
	now := time.Now()
	started := now.Add(-5 * time.Second)
	completed := now

	exec := &Execution{
		ID:                id,
		PayloadID:         payloadID,
		TargetID:          types.NewID(),
		TargetType:        types.TargetTypeLLMChat,
		AgentID:           types.NewID(),
		Status:            ExecutionStatusCompleted,
		Success:           success,
		ConfidenceScore:   0.95,
		Parameters:        map[string]interface{}{},
		InstantiatedText:  "test prompt",
		Response:          "test response",
		IndicatorsMatched: []string{"success"},
		TokensUsed:        100,
		Cost:              0.001,
		FindingCreated:    false,
		CreatedAt:         now,
		StartedAt:         &started,
		CompletedAt:       &completed,
	}

	if !success {
		exec.Status = ExecutionStatusFailed
		exec.ConfidenceScore = 0.0
		exec.ErrorMessage = "Execution failed"
	}

	return exec
}

func ptrTo[T any](v T) *T {
	return &v
}
