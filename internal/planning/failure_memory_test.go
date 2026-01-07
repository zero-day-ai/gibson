package planning

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFailureMemory(t *testing.T) {
	fm := NewFailureMemory()

	require.NotNil(t, fm)
	assert.NotNil(t, fm.Attempts)
	assert.NotNil(t, fm.ReplanHistory)
	assert.Equal(t, 0, len(fm.Attempts))
	assert.Equal(t, 0, len(fm.ReplanHistory))
}

func TestRecordAttempt(t *testing.T) {
	tests := []struct {
		name     string
		nodeID   string
		attempt  AttemptRecord
		validate func(t *testing.T, fm *FailureMemory)
	}{
		{
			name:   "record single attempt",
			nodeID: "node1",
			attempt: AttemptRecord{
				Approach: "approach1",
				Result:   "success",
				Success:  true,
			},
			validate: func(t *testing.T, fm *FailureMemory) {
				attempts := fm.GetAttempts("node1")
				require.Len(t, attempts, 1)
				assert.Equal(t, "approach1", attempts[0].Approach)
				assert.Equal(t, "success", attempts[0].Result)
				assert.True(t, attempts[0].Success)
				assert.False(t, attempts[0].Timestamp.IsZero())
			},
		},
		{
			name:   "record attempt with timestamp",
			nodeID: "node2",
			attempt: AttemptRecord{
				Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				Approach:  "approach2",
				Result:    "failed",
				Success:   false,
			},
			validate: func(t *testing.T, fm *FailureMemory) {
				attempts := fm.GetAttempts("node2")
				require.Len(t, attempts, 1)
				assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), attempts[0].Timestamp)
			},
		},
		{
			name:   "record multiple attempts for same node",
			nodeID: "node3",
			attempt: AttemptRecord{
				Approach: "approach1",
				Result:   "failed",
				Success:  false,
			},
			validate: func(t *testing.T, fm *FailureMemory) {
				// Record second attempt
				fm.RecordAttempt("node3", AttemptRecord{
					Approach: "approach2",
					Result:   "success",
					Success:  true,
				})

				attempts := fm.GetAttempts("node3")
				require.Len(t, attempts, 2)
				assert.Equal(t, "approach1", attempts[0].Approach)
				assert.Equal(t, "approach2", attempts[1].Approach)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewFailureMemory()
			fm.RecordAttempt(tt.nodeID, tt.attempt)
			tt.validate(t, fm)
		})
	}
}

func TestRecordReplan(t *testing.T) {
	tests := []struct {
		name     string
		record   ReplanRecord
		validate func(t *testing.T, fm *FailureMemory)
	}{
		{
			name: "record single replan",
			record: ReplanRecord{
				TriggerNode: "node1",
				OldPlan:     []string{"a", "b", "c"},
				NewPlan:     []string{"a", "c", "b"},
				Rationale:   "reordering for efficiency",
			},
			validate: func(t *testing.T, fm *FailureMemory) {
				assert.Equal(t, 1, fm.ReplanCount())
				history := fm.GetReplanHistory()
				require.Len(t, history, 1)
				assert.Equal(t, "node1", history[0].TriggerNode)
				assert.Equal(t, []string{"a", "b", "c"}, history[0].OldPlan)
				assert.Equal(t, []string{"a", "c", "b"}, history[0].NewPlan)
				assert.Equal(t, "reordering for efficiency", history[0].Rationale)
				assert.False(t, history[0].Timestamp.IsZero())
			},
		},
		{
			name: "record replan with timestamp",
			record: ReplanRecord{
				Timestamp:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				TriggerNode: "node2",
				OldPlan:     []string{"x"},
				NewPlan:     []string{"y"},
				Rationale:   "pivot strategy",
			},
			validate: func(t *testing.T, fm *FailureMemory) {
				history := fm.GetReplanHistory()
				require.Len(t, history, 1)
				assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), history[0].Timestamp)
			},
		},
		{
			name: "record multiple replans",
			record: ReplanRecord{
				TriggerNode: "node1",
				OldPlan:     []string{"a"},
				NewPlan:     []string{"b"},
				Rationale:   "first replan",
			},
			validate: func(t *testing.T, fm *FailureMemory) {
				// Record second replan
				fm.RecordReplan(ReplanRecord{
					TriggerNode: "node2",
					OldPlan:     []string{"b"},
					NewPlan:     []string{"c"},
					Rationale:   "second replan",
				})

				assert.Equal(t, 2, fm.ReplanCount())
				history := fm.GetReplanHistory()
				require.Len(t, history, 2)
				assert.Equal(t, "first replan", history[0].Rationale)
				assert.Equal(t, "second replan", history[1].Rationale)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewFailureMemory()
			fm.RecordReplan(tt.record)
			tt.validate(t, fm)
		})
	}
}

func TestHasTried(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(fm *FailureMemory)
		nodeID    string
		approach  string
		wantTried bool
	}{
		{
			name:      "no attempts recorded",
			setup:     func(fm *FailureMemory) {},
			nodeID:    "node1",
			approach:  "any approach",
			wantTried: false,
		},
		{
			name: "exact match",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "use nmap scan",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "use nmap scan",
			wantTried: true,
		},
		{
			name: "case insensitive match",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "Use NMAP Scan",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "use nmap scan",
			wantTried: true,
		},
		{
			name: "whitespace normalization",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "  use   nmap   scan  ",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "use nmap scan",
			wantTried: true,
		},
		{
			name: "substring match - longer approach contains query",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "use nmap scan with aggressive timing",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "use nmap scan",
			wantTried: true,
		},
		{
			name: "substring match - query contains recorded approach",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "nmap scan",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "use nmap scan with options",
			wantTried: true,
		},
		{
			name: "high word overlap (Jaccard > 0.7)",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "run aggressive nmap port scan",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "run nmap port scan aggressive",
			wantTried: true,
		},
		{
			name: "low word overlap (Jaccard < 0.7)",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "use nmap for scanning",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "try sqlmap injection attack",
			wantTried: false,
		},
		{
			name: "different node not found",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "some approach",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node2",
			approach:  "some approach",
			wantTried: false,
		},
		{
			name: "multiple attempts - one matches",
			setup: func(fm *FailureMemory) {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "approach 1",
					Result:   "failed",
					Success:  false,
				})
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "approach 2",
					Result:   "failed",
					Success:  false,
				})
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: "use sqlmap",
					Result:   "failed",
					Success:  false,
				})
			},
			nodeID:    "node1",
			approach:  "use sqlmap attack",
			wantTried: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewFailureMemory()
			tt.setup(fm)
			got := fm.HasTried(tt.nodeID, tt.approach)
			assert.Equal(t, tt.wantTried, got)
		})
	}
}

func TestGetAttempts(t *testing.T) {
	fm := NewFailureMemory()

	// Empty case
	attempts := fm.GetAttempts("nonexistent")
	assert.NotNil(t, attempts)
	assert.Len(t, attempts, 0)

	// Add attempts
	fm.RecordAttempt("node1", AttemptRecord{
		Approach: "approach1",
		Result:   "result1",
		Success:  true,
	})
	fm.RecordAttempt("node1", AttemptRecord{
		Approach: "approach2",
		Result:   "result2",
		Success:  false,
	})

	attempts = fm.GetAttempts("node1")
	require.Len(t, attempts, 2)
	assert.Equal(t, "approach1", attempts[0].Approach)
	assert.Equal(t, "approach2", attempts[1].Approach)

	// Verify returned slice is a copy (modification doesn't affect original)
	attempts[0].Approach = "modified"
	originalAttempts := fm.GetAttempts("node1")
	assert.Equal(t, "approach1", originalAttempts[0].Approach, "returned slice should be a copy")
}

func TestReplanCount(t *testing.T) {
	fm := NewFailureMemory()

	// Initial count
	assert.Equal(t, 0, fm.ReplanCount())

	// After adding replans
	fm.RecordReplan(ReplanRecord{
		TriggerNode: "node1",
		OldPlan:     []string{"a"},
		NewPlan:     []string{"b"},
		Rationale:   "reason1",
	})
	assert.Equal(t, 1, fm.ReplanCount())

	fm.RecordReplan(ReplanRecord{
		TriggerNode: "node2",
		OldPlan:     []string{"b"},
		NewPlan:     []string{"c"},
		Rationale:   "reason2",
	})
	assert.Equal(t, 2, fm.ReplanCount())
}

func TestGetReplanHistory(t *testing.T) {
	fm := NewFailureMemory()

	// Empty case
	history := fm.GetReplanHistory()
	assert.NotNil(t, history)
	assert.Len(t, history, 0)

	// Add replans
	fm.RecordReplan(ReplanRecord{
		TriggerNode: "node1",
		OldPlan:     []string{"a"},
		NewPlan:     []string{"b"},
		Rationale:   "reason1",
	})
	fm.RecordReplan(ReplanRecord{
		TriggerNode: "node2",
		OldPlan:     []string{"b"},
		NewPlan:     []string{"c"},
		Rationale:   "reason2",
	})

	history = fm.GetReplanHistory()
	require.Len(t, history, 2)
	assert.Equal(t, "node1", history[0].TriggerNode)
	assert.Equal(t, "node2", history[1].TriggerNode)

	// Verify returned slice is a copy
	history[0].TriggerNode = "modified"
	originalHistory := fm.GetReplanHistory()
	assert.Equal(t, "node1", originalHistory[0].TriggerNode, "returned slice should be a copy")
}

func TestClear(t *testing.T) {
	fm := NewFailureMemory()

	// Add data
	fm.RecordAttempt("node1", AttemptRecord{
		Approach: "approach1",
		Result:   "result1",
		Success:  true,
	})
	fm.RecordReplan(ReplanRecord{
		TriggerNode: "node1",
		OldPlan:     []string{"a"},
		NewPlan:     []string{"b"},
		Rationale:   "reason1",
	})

	// Verify data exists
	assert.Len(t, fm.GetAttempts("node1"), 1)
	assert.Equal(t, 1, fm.ReplanCount())

	// Clear
	fm.Clear()

	// Verify cleared
	assert.Len(t, fm.GetAttempts("node1"), 0)
	assert.Equal(t, 0, fm.ReplanCount())
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("concurrent RecordAttempt", func(t *testing.T) {
		fm := NewFailureMemory()
		var wg sync.WaitGroup
		numGoroutines := 100
		attemptsPerGoroutine := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				nodeID := fmt.Sprintf("node%d", id%10)
				for j := 0; j < attemptsPerGoroutine; j++ {
					fm.RecordAttempt(nodeID, AttemptRecord{
						Approach: fmt.Sprintf("approach_%d_%d", id, j),
						Result:   "result",
						Success:  j%2 == 0,
					})
				}
			}(i)
		}

		wg.Wait()

		// Verify all attempts were recorded
		totalAttempts := 0
		for i := 0; i < 10; i++ {
			nodeID := fmt.Sprintf("node%d", i)
			totalAttempts += len(fm.GetAttempts(nodeID))
		}
		assert.Equal(t, numGoroutines*attemptsPerGoroutine, totalAttempts)
	})

	t.Run("concurrent RecordReplan", func(t *testing.T) {
		fm := NewFailureMemory()
		var wg sync.WaitGroup
		numReplans := 100

		for i := 0; i < numReplans; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				fm.RecordReplan(ReplanRecord{
					TriggerNode: fmt.Sprintf("node%d", id),
					OldPlan:     []string{"old"},
					NewPlan:     []string{"new"},
					Rationale:   fmt.Sprintf("reason%d", id),
				})
			}(i)
		}

		wg.Wait()

		assert.Equal(t, numReplans, fm.ReplanCount())
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		fm := NewFailureMemory()
		var wg sync.WaitGroup
		numOperations := 100

		// Writers
		for i := 0; i < numOperations/2; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				fm.RecordAttempt("shared_node", AttemptRecord{
					Approach: fmt.Sprintf("approach_%d", id),
					Result:   "result",
					Success:  true,
				})
			}(i)
		}

		// Readers
		for i := 0; i < numOperations/2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = fm.GetAttempts("shared_node")
				_ = fm.HasTried("shared_node", "some approach")
				_ = fm.ReplanCount()
			}()
		}

		wg.Wait()

		// Should complete without race conditions
		attempts := fm.GetAttempts("shared_node")
		assert.GreaterOrEqual(t, len(attempts), 0)
	})

	t.Run("concurrent HasTried with RecordAttempt", func(t *testing.T) {
		fm := NewFailureMemory()
		var wg sync.WaitGroup

		// Record some initial attempts
		fm.RecordAttempt("node1", AttemptRecord{
			Approach: "test approach",
			Result:   "result",
			Success:  true,
		})

		numReaders := 50
		numWriters := 50

		// Readers checking HasTried
		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				_ = fm.HasTried("node1", "test approach")
				_ = fm.HasTried("node1", fmt.Sprintf("approach_%d", id))
			}(i)
		}

		// Writers recording attempts
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: fmt.Sprintf("approach_%d", id),
					Result:   "result",
					Success:  true,
				})
			}(i)
		}

		wg.Wait()

		// Verify no race conditions occurred
		assert.True(t, fm.HasTried("node1", "test approach"))
	})
}

func TestNormalizeApproach(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Simple", "simple"},
		{"  Whitespace  ", "whitespace"},
		{"Multiple   Spaces", "multiple spaces"},
		{"UPPERCASE", "uppercase"},
		{"MixedCase", "mixedcase"},
		{"  Mixed   Case   With   Spaces  ", "mixed case with spaces"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeApproach(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsSimilarApproach(t *testing.T) {
	tests := []struct {
		name      string
		approach1 string
		approach2 string
		want      bool
	}{
		{
			name:      "exact match",
			approach1: "test approach",
			approach2: "test approach",
			want:      true,
		},
		{
			name:      "substring - first contains second",
			approach1: "test approach with options",
			approach2: "test approach",
			want:      true,
		},
		{
			name:      "substring - second contains first",
			approach1: "test",
			approach2: "test approach",
			want:      true,
		},
		{
			name:      "high jaccard similarity (>0.7)",
			approach1: "use nmap scan aggressive timing",
			approach2: "nmap aggressive scan timing use",
			want:      true,
		},
		{
			name:      "low jaccard similarity (<0.7)",
			approach1: "use nmap for scanning",
			approach2: "try sqlmap injection",
			want:      false,
		},
		{
			name:      "completely different",
			approach1: "abc def",
			approach2: "xyz uvw",
			want:      false,
		},
		{
			name:      "empty strings",
			approach1: "",
			approach2: "",
			want:      true,
		},
		{
			name:      "one empty",
			approach1: "test",
			approach2: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSimilarApproach(tt.approach1, tt.approach2)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name   string
		words1 []string
		words2 []string
		want   float64
	}{
		{
			name:   "identical sets",
			words1: []string{"a", "b", "c"},
			words2: []string{"a", "b", "c"},
			want:   1.0,
		},
		{
			name:   "no overlap",
			words1: []string{"a", "b"},
			words2: []string{"c", "d"},
			want:   0.0,
		},
		{
			name:   "partial overlap",
			words1: []string{"a", "b", "c"},
			words2: []string{"b", "c", "d"},
			want:   0.5, // intersection: 2, union: 4
		},
		{
			name:   "subset",
			words1: []string{"a", "b"},
			words2: []string{"a", "b", "c"},
			want:   2.0 / 3.0, // intersection: 2, union: 3
		},
		{
			name:   "empty sets",
			words1: []string{},
			words2: []string{},
			want:   0.0,
		},
		{
			name:   "one empty",
			words1: []string{"a"},
			words2: []string{},
			want:   0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jaccardSimilarity(tt.words1, tt.words2)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

// Benchmark tests
func BenchmarkRecordAttempt(b *testing.B) {
	fm := NewFailureMemory()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm.RecordAttempt(fmt.Sprintf("node%d", i%100), AttemptRecord{
			Approach: fmt.Sprintf("approach%d", i),
			Result:   "result",
			Success:  true,
		})
	}
}

func BenchmarkHasTried(b *testing.B) {
	fm := NewFailureMemory()
	// Pre-populate with some data
	for i := 0; i < 100; i++ {
		fm.RecordAttempt("node1", AttemptRecord{
			Approach: fmt.Sprintf("approach%d", i),
			Result:   "result",
			Success:  true,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fm.HasTried("node1", fmt.Sprintf("approach%d", i%100))
	}
}

func BenchmarkConcurrentAccess(b *testing.B) {
	fm := NewFailureMemory()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				fm.RecordAttempt("node1", AttemptRecord{
					Approach: fmt.Sprintf("approach%d", i),
					Result:   "result",
					Success:  true,
				})
			} else {
				fm.HasTried("node1", fmt.Sprintf("approach%d", i))
			}
			i++
		}
	})
}
