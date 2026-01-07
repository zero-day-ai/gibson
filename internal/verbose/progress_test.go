package verbose

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpinner_Basic(t *testing.T) {
	var buf bytes.Buffer

	spinner := NewSpinner(&buf, "Loading")
	spinner.interval = 10 * time.Millisecond // Speed up for testing

	spinner.Start()
	time.Sleep(50 * time.Millisecond) // Let it run for a bit
	spinner.Stop()

	output := buf.String()
	// Should contain the message
	assert.Contains(t, output, "Loading")

	// Should contain at least one spinner frame
	hasFrame := false
	for _, frame := range spinnerFrames {
		if strings.Contains(output, frame) {
			hasFrame = true
			break
		}
	}
	assert.True(t, hasFrame, "output should contain a spinner frame")
}

func TestSpinner_UpdateMessage(t *testing.T) {
	var buf bytes.Buffer

	spinner := NewSpinner(&buf, "Initial message")
	spinner.interval = 10 * time.Millisecond
	// Force TTY mode for reliable testing
	spinner.isTTY = true

	spinner.Start()
	time.Sleep(20 * time.Millisecond)

	spinner.UpdateMessage("Updated message")
	time.Sleep(30 * time.Millisecond)

	spinner.Stop()

	output := buf.String()
	assert.Contains(t, output, "Updated message")
}

func TestSpinner_MultipleStartStop(t *testing.T) {
	var buf bytes.Buffer

	spinner := NewSpinner(&buf, "Test")
	spinner.interval = 10 * time.Millisecond

	// Start and stop multiple times
	spinner.Start()
	time.Sleep(20 * time.Millisecond)
	spinner.Stop()

	spinner.Start()
	time.Sleep(20 * time.Millisecond)
	spinner.Stop()

	// Should not panic or hang
	assert.NotEmpty(t, buf.String())
}

func TestSpinner_StopIdempotent(t *testing.T) {
	var buf bytes.Buffer

	spinner := NewSpinner(&buf, "Test")
	spinner.interval = 10 * time.Millisecond

	spinner.Start()
	time.Sleep(20 * time.Millisecond)

	// Stop multiple times should be safe
	spinner.Stop()
	spinner.Stop()
	spinner.Stop()

	// Should not panic
	assert.NotEmpty(t, buf.String())
}

func TestProgressBar_Basic(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 100, "Processing")
	bar.Start()

	// Update progress
	bar.Update(25)
	bar.Update(50)
	bar.Update(75)
	bar.Update(100)

	bar.Stop()

	output := buf.String()
	assert.Contains(t, output, "Processing")

	// Should contain progress indicators (on non-TTY, we get percentage output)
	containsProgress := strings.Contains(output, "%") || strings.Contains(output, "=")
	assert.True(t, containsProgress, "output should contain progress indicators")
}

func TestProgressBar_Increment(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 10, "Items")
	bar.Start()

	for i := 0; i < 10; i++ {
		bar.Increment()
	}

	bar.Stop()

	output := buf.String()
	assert.Contains(t, output, "Items")
}

func TestProgressBar_ZeroTotal(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 0, "Empty")
	bar.Start()
	bar.Update(0)
	bar.Stop()

	// Should not panic with zero total
	output := buf.String()
	assert.NotNil(t, output)
}

func TestProgressBar_OverflowHandling(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 100, "Test")
	bar.Start()

	// Update with value greater than total
	bar.Update(150)

	bar.Stop()

	output := buf.String()
	// Should cap at 100%
	assert.Contains(t, output, "100%")
}

func TestProgressBar_ConcurrentUpdates(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 1000, "Concurrent")
	bar.Start()

	// Update from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				bar.Increment()
			}
			done <- true
		}(i)
	}

	// Wait for all updates
	for i := 0; i < 10; i++ {
		<-done
	}

	bar.Stop()

	// Should not panic with concurrent updates
	output := buf.String()
	assert.NotEmpty(t, output)
}

func TestProgressBar_UpdateAfterStop(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 100, "Test")
	bar.Start()
	bar.Update(50)
	bar.Stop()

	// Update after stop should be safe (no-op)
	bar.Update(75)

	// Should not panic
	assert.NotEmpty(t, buf.String())
}

func TestProgressBar_NonTTYThrottling(t *testing.T) {
	var buf bytes.Buffer

	bar := NewProgressBar(&buf, 100, "Throttled")
	bar.minUpdatePercent = 10 // Update every 10%
	bar.Start()

	// Update in small increments
	for i := 0; i <= 100; i++ {
		bar.Update(int64(i))
	}

	bar.Stop()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// With 10% throttling, we should have ~10 updates + final
	// (0%, 10%, 20%, ..., 90%, 100%)
	// Allow some tolerance for timing
	assert.Less(t, len(lines), 20, "should throttle updates on non-TTY")
}

// BenchmarkSpinner measures spinner performance.
func BenchmarkSpinner(b *testing.B) {
	var buf bytes.Buffer

	for i := 0; i < b.N; i++ {
		spinner := NewSpinner(&buf, "Benchmark")
		spinner.interval = 1 * time.Millisecond
		spinner.Start()
		time.Sleep(5 * time.Millisecond)
		spinner.Stop()
		buf.Reset()
	}
}

// BenchmarkProgressBar measures progress bar performance.
func BenchmarkProgressBar(b *testing.B) {
	var buf bytes.Buffer

	for i := 0; i < b.N; i++ {
		bar := NewProgressBar(&buf, 1000, "Benchmark")
		bar.Start()
		for j := 0; j < 1000; j++ {
			bar.Increment()
		}
		bar.Stop()
		buf.Reset()
	}
}

// TestSpinner_NonTTYFallback verifies spinner works on non-TTY output.
func TestSpinner_NonTTYFallback(t *testing.T) {
	var buf bytes.Buffer

	spinner := NewSpinner(&buf, "Non-TTY test")
	// Force non-TTY mode
	spinner.isTTY = false
	spinner.interval = 10 * time.Millisecond

	spinner.Start()
	time.Sleep(150 * time.Millisecond) // Long enough for multiple frames
	spinner.Stop()

	output := buf.String()
	// On non-TTY, spinner only writes every N frames
	// So we might not see as much output
	assert.Contains(t, output, "Non-TTY test")
}

// TestProgressBar_Messages verifies custom messages work.
func TestProgressBar_Messages(t *testing.T) {
	testCases := []struct {
		name    string
		message string
	}{
		{"Simple", "Loading"},
		{"With spaces", "Processing items"},
		{"Long message", "This is a very long progress message that should still work correctly"},
		{"Empty", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			bar := NewProgressBar(&buf, 10, tc.message)
			bar.Start()
			bar.Update(5)
			bar.Stop()

			output := buf.String()
			if tc.message != "" {
				assert.Contains(t, output, tc.message)
			}
		})
	}
}

// TestSpinner_RapidStartStop verifies spinner handles rapid start/stop cycles.
func TestSpinner_RapidStartStop(t *testing.T) {
	var buf bytes.Buffer

	spinner := NewSpinner(&buf, "Rapid")
	spinner.interval = 5 * time.Millisecond

	// Rapid cycles
	for i := 0; i < 5; i++ {
		spinner.Start()
		time.Sleep(10 * time.Millisecond)
		spinner.Stop()
	}

	// Should not hang or panic
	assert.NotEmpty(t, buf.String())
}
