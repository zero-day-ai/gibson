package memory

import (
	"testing"
)

func BenchmarkWorkingMemory_Set(b *testing.B) {
	wm := NewWorkingMemory(100000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wm.Set("test-key", "test-value")
	}
}

func BenchmarkWorkingMemory_Get(b *testing.B) {
	wm := NewWorkingMemory(100000)
	_ = wm.Set("test-key", "test-value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wm.Get("test-key")
	}
}

func BenchmarkWorkingMemory_Delete(b *testing.B) {
	wm := NewWorkingMemory(100000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		_ = wm.Set("test-key", "test-value")
		b.StartTimer()
		_ = wm.Delete("test-key")
	}
}

func BenchmarkWorkingMemory_SetWithEviction(b *testing.B) {
	// Small limit to trigger eviction
	wm := NewWorkingMemory(100)
	values := make([]string, 20)
	for i := range values {
		values[i] = string(make([]byte, 50))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wm.Set(string(rune(i%20)), values[i%20])
	}
}
