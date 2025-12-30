package console

import (
	"testing"
)

func TestNewHistory(t *testing.T) {
	tests := []struct {
		name        string
		maxSize     int
		wantMaxSize int
	}{
		{
			name:        "valid maxSize",
			maxSize:     50,
			wantMaxSize: 50,
		},
		{
			name:        "zero maxSize uses default",
			maxSize:     0,
			wantMaxSize: 100,
		},
		{
			name:        "negative maxSize uses default",
			maxSize:     -10,
			wantMaxSize: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistory(tt.maxSize)
			if h == nil {
				t.Fatal("NewHistory returned nil")
			}
			if h.maxSize != tt.wantMaxSize {
				t.Errorf("maxSize = %d, want %d", h.maxSize, tt.wantMaxSize)
			}
			if h.index != -1 {
				t.Errorf("initial index = %d, want -1", h.index)
			}
			if h.Len() != 0 {
				t.Errorf("initial length = %d, want 0", h.Len())
			}
		})
	}
}

func TestHistoryAdd(t *testing.T) {
	tests := []struct {
		name        string
		commands    []string
		wantEntries []string
		wantLen     int
	}{
		{
			name:        "add single command",
			commands:    []string{"test command"},
			wantEntries: []string{"test command"},
			wantLen:     1,
		},
		{
			name:        "add multiple commands",
			commands:    []string{"first", "second", "third"},
			wantEntries: []string{"first", "second", "third"},
			wantLen:     3,
		},
		{
			name:        "empty command is skipped",
			commands:    []string{"first", "", "second"},
			wantEntries: []string{"first", "second"},
			wantLen:     2,
		},
		{
			name:        "consecutive duplicates are skipped",
			commands:    []string{"first", "first", "second"},
			wantEntries: []string{"first", "second"},
			wantLen:     2,
		},
		{
			name:        "non-consecutive duplicates are kept",
			commands:    []string{"first", "second", "first"},
			wantEntries: []string{"first", "second", "first"},
			wantLen:     3,
		},
		{
			name:        "whitespace-only is not treated as empty",
			commands:    []string{"first", "   ", "second"},
			wantEntries: []string{"first", "   ", "second"},
			wantLen:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistory(100)
			for _, cmd := range tt.commands {
				h.Add(cmd)
			}

			if h.Len() != tt.wantLen {
				t.Errorf("Len() = %d, want %d", h.Len(), tt.wantLen)
			}

			entries := h.Entries()
			if len(entries) != len(tt.wantEntries) {
				t.Fatalf("got %d entries, want %d", len(entries), len(tt.wantEntries))
			}

			for i, want := range tt.wantEntries {
				if entries[i] != want {
					t.Errorf("entry[%d] = %q, want %q", i, entries[i], want)
				}
			}

			// Check that index is reset after adding
			if h.index != -1 {
				t.Errorf("index after Add = %d, want -1", h.index)
			}
		})
	}
}

func TestHistoryMaxSize(t *testing.T) {
	tests := []struct {
		name        string
		maxSize     int
		commands    []string
		wantEntries []string
		wantLen     int
	}{
		{
			name:        "stays within maxSize",
			maxSize:     3,
			commands:    []string{"one", "two", "three"},
			wantEntries: []string{"one", "two", "three"},
			wantLen:     3,
		},
		{
			name:        "removes oldest when exceeding maxSize",
			maxSize:     3,
			commands:    []string{"one", "two", "three", "four"},
			wantEntries: []string{"two", "three", "four"},
			wantLen:     3,
		},
		{
			name:        "removes multiple oldest entries",
			maxSize:     2,
			commands:    []string{"one", "two", "three", "four", "five"},
			wantEntries: []string{"four", "five"},
			wantLen:     2,
		},
		{
			name:        "maxSize of 1",
			maxSize:     1,
			commands:    []string{"one", "two", "three"},
			wantEntries: []string{"three"},
			wantLen:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistory(tt.maxSize)
			for _, cmd := range tt.commands {
				h.Add(cmd)
			}

			if h.Len() != tt.wantLen {
				t.Errorf("Len() = %d, want %d", h.Len(), tt.wantLen)
			}

			entries := h.Entries()
			if len(entries) != len(tt.wantEntries) {
				t.Fatalf("got %d entries, want %d", len(entries), len(tt.wantEntries))
			}

			for i, want := range tt.wantEntries {
				if entries[i] != want {
					t.Errorf("entry[%d] = %q, want %q", i, entries[i], want)
				}
			}
		})
	}
}

func TestHistoryPrevious(t *testing.T) {
	tests := []struct {
		name           string
		commands       []string
		previousCalls  int
		wantCommands   []string
		wantOk         []bool
		wantFinalIndex int
	}{
		{
			name:           "empty history",
			commands:       []string{},
			previousCalls:  1,
			wantCommands:   []string{""},
			wantOk:         []bool{false},
			wantFinalIndex: -1,
		},
		{
			name:           "single command",
			commands:       []string{"first"},
			previousCalls:  1,
			wantCommands:   []string{"first"},
			wantOk:         []bool{true},
			wantFinalIndex: 0,
		},
		{
			name:           "navigate backward once",
			commands:       []string{"first", "second", "third"},
			previousCalls:  1,
			wantCommands:   []string{"third"},
			wantOk:         []bool{true},
			wantFinalIndex: 2,
		},
		{
			name:           "navigate backward multiple times",
			commands:       []string{"first", "second", "third"},
			previousCalls:  3,
			wantCommands:   []string{"third", "second", "first"},
			wantOk:         []bool{true, true, true},
			wantFinalIndex: 0,
		},
		{
			name:           "at beginning boundary",
			commands:       []string{"first", "second"},
			previousCalls:  4,
			wantCommands:   []string{"second", "first", "first", "first"},
			wantOk:         []bool{true, true, true, true},
			wantFinalIndex: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistory(100)
			for _, cmd := range tt.commands {
				h.Add(cmd)
			}

			for i := 0; i < tt.previousCalls; i++ {
				cmd, ok := h.Previous()
				if ok != tt.wantOk[i] {
					t.Errorf("Previous() call %d: ok = %v, want %v", i+1, ok, tt.wantOk[i])
				}
				if cmd != tt.wantCommands[i] {
					t.Errorf("Previous() call %d: cmd = %q, want %q", i+1, cmd, tt.wantCommands[i])
				}
			}

			if h.index != tt.wantFinalIndex {
				t.Errorf("final index = %d, want %d", h.index, tt.wantFinalIndex)
			}
		})
	}
}

func TestHistoryNext(t *testing.T) {
	tests := []struct {
		name           string
		commands       []string
		setup          func(*History) // Setup function to position the history
		nextCalls      int
		wantCommands   []string
		wantOk         []bool
		wantFinalIndex int
	}{
		{
			name:     "empty history",
			commands: []string{},
			setup: func(h *History) {
				// No setup needed
			},
			nextCalls:      1,
			wantCommands:   []string{""},
			wantOk:         []bool{false},
			wantFinalIndex: -1,
		},
		{
			name:     "not navigating",
			commands: []string{"first", "second"},
			setup: func(h *History) {
				// Don't call Previous, index stays at -1
			},
			nextCalls:      1,
			wantCommands:   []string{""},
			wantOk:         []bool{false},
			wantFinalIndex: -1,
		},
		{
			name:     "navigate forward once",
			commands: []string{"first", "second", "third"},
			setup: func(h *History) {
				h.Previous() // third (index=2)
				h.Previous() // second (index=1)
			},
			nextCalls:      1,
			wantCommands:   []string{"third"},
			wantOk:         []bool{true},
			wantFinalIndex: 2,
		},
		{
			name:     "navigate forward to end",
			commands: []string{"first", "second", "third"},
			setup: func(h *History) {
				h.Previous() // third (index=2)
				h.Previous() // second (index=1)
				h.Previous() // first (index=0)
			},
			nextCalls:      3,
			wantCommands:   []string{"second", "third", ""},
			wantOk:         []bool{true, true, false},
			wantFinalIndex: -1,
		},
		{
			name:     "at end boundary",
			commands: []string{"first", "second"},
			setup: func(h *History) {
				h.Previous() // second (index=1)
			},
			nextCalls:      3,
			wantCommands:   []string{"", "", ""},
			wantOk:         []bool{false, false, false},
			wantFinalIndex: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistory(100)
			for _, cmd := range tt.commands {
				h.Add(cmd)
			}

			tt.setup(h)

			for i := 0; i < tt.nextCalls; i++ {
				cmd, ok := h.Next()
				if ok != tt.wantOk[i] {
					t.Errorf("Next() call %d: ok = %v, want %v", i+1, ok, tt.wantOk[i])
				}
				if cmd != tt.wantCommands[i] {
					t.Errorf("Next() call %d: cmd = %q, want %q", i+1, cmd, tt.wantCommands[i])
				}
			}

			if h.index != tt.wantFinalIndex {
				t.Errorf("final index = %d, want %d", h.index, tt.wantFinalIndex)
			}
		})
	}
}

func TestHistoryReset(t *testing.T) {
	h := NewHistory(100)
	h.Add("first")
	h.Add("second")
	h.Add("third")

	// Navigate backward
	h.Previous()
	h.Previous()

	if h.index == -1 {
		t.Fatal("expected index to be set after Previous()")
	}

	// Reset should set index back to -1
	h.Reset()

	if h.index != -1 {
		t.Errorf("index after Reset() = %d, want -1", h.index)
	}

	// Entries should remain unchanged
	if h.Len() != 3 {
		t.Errorf("Len() after Reset() = %d, want 3", h.Len())
	}
}

func TestHistoryEntries(t *testing.T) {
	h := NewHistory(100)
	h.Add("first")
	h.Add("second")
	h.Add("third")

	entries := h.Entries()

	// Verify returned entries
	want := []string{"first", "second", "third"}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}

	for i, w := range want {
		if entries[i] != w {
			t.Errorf("entry[%d] = %q, want %q", i, entries[i], w)
		}
	}

	// Verify that modifying the returned slice doesn't affect internal state
	entries[0] = "modified"
	internalEntries := h.Entries()
	if internalEntries[0] != "first" {
		t.Error("modifying returned entries affected internal state")
	}
}

func TestHistoryLen(t *testing.T) {
	h := NewHistory(100)

	if h.Len() != 0 {
		t.Errorf("Len() on empty history = %d, want 0", h.Len())
	}

	h.Add("first")
	if h.Len() != 1 {
		t.Errorf("Len() after one add = %d, want 1", h.Len())
	}

	h.Add("second")
	h.Add("third")
	if h.Len() != 3 {
		t.Errorf("Len() after three adds = %d, want 3", h.Len())
	}

	// Add duplicate (should not increase length)
	h.Add("third")
	if h.Len() != 3 {
		t.Errorf("Len() after duplicate add = %d, want 3", h.Len())
	}

	// Add empty (should not increase length)
	h.Add("")
	if h.Len() != 3 {
		t.Errorf("Len() after empty add = %d, want 3", h.Len())
	}
}

func TestHistoryNavigationScenario(t *testing.T) {
	// Test a realistic user interaction scenario
	h := NewHistory(100)
	h.Add("/agents list")
	h.Add("/missions create")
	h.Add("/status")

	// User presses Up (should get /status)
	cmd, ok := h.Previous()
	if !ok || cmd != "/status" {
		t.Errorf("first Previous() = %q, %v; want %q, true", cmd, ok, "/status")
	}

	// User presses Up again (should get /missions create)
	cmd, ok = h.Previous()
	if !ok || cmd != "/missions create" {
		t.Errorf("second Previous() = %q, %v; want %q, true", cmd, ok, "/missions create")
	}

	// User presses Down (should get /status)
	cmd, ok = h.Next()
	if !ok || cmd != "/status" {
		t.Errorf("Next() = %q, %v; want %q, true", cmd, ok, "/status")
	}

	// User starts typing (Reset called)
	h.Reset()

	// User presses Up (should get /status again)
	cmd, ok = h.Previous()
	if !ok || cmd != "/status" {
		t.Errorf("Previous() after Reset() = %q, %v; want %q, true", cmd, ok, "/status")
	}

	// User executes a new command
	h.Add("/config show")

	// User presses Up (should get /config show)
	cmd, ok = h.Previous()
	if !ok || cmd != "/config show" {
		t.Errorf("Previous() after Add() = %q, %v; want %q, true", cmd, ok, "/config show")
	}
}
