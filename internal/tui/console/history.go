package console

// History manages command history for the console with navigation support.
// It maintains a list of previously entered commands and provides
// Previous/Next navigation similar to shell history.
type History struct {
	entries []string // Stores command strings in chronological order
	index   int      // Current navigation position (-1 means not navigating)
	maxSize int      // Maximum number of entries to retain
}

// NewHistory creates a new History instance with the specified maximum size.
// The maxSize parameter determines how many commands to retain in history.
// Older commands are discarded when the limit is exceeded.
func NewHistory(maxSize int) *History {
	if maxSize <= 0 {
		maxSize = 100 // Default maximum size
	}
	return &History{
		entries: make([]string, 0, maxSize),
		index:   -1,
		maxSize: maxSize,
	}
}

// Add appends a command to the history.
// It skips empty commands and duplicates of the most recent entry.
// When maxSize is exceeded, the oldest entries are removed.
func (h *History) Add(cmd string) {
	// Skip empty commands
	if cmd == "" {
		return
	}

	// Skip if this is a duplicate of the last command
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == cmd {
		return
	}

	// Add the command
	h.entries = append(h.entries, cmd)

	// Trim to maxSize if exceeded
	if len(h.entries) > h.maxSize {
		// Remove oldest entries to stay within maxSize
		excess := len(h.entries) - h.maxSize
		h.entries = h.entries[excess:]
	}

	// Reset navigation index
	h.index = -1
}

// Previous returns the previous command in history (moving backward).
// Returns the command string and true if successful.
// Returns empty string and false if at the beginning of history.
// Used when the user presses the Up arrow key.
func (h *History) Previous() (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}

	// If not currently navigating, start from the end
	if h.index == -1 {
		h.index = len(h.entries) - 1
		return h.entries[h.index], true
	}

	// If already at the beginning, can't go further back
	if h.index == 0 {
		return h.entries[h.index], true
	}

	// Move backward in history
	h.index--
	return h.entries[h.index], true
}

// Next returns the next command in history (moving forward).
// Returns the command string and true if there is a next command.
// Returns empty string and false when reaching the end of history.
// Used when the user presses the Down arrow key.
func (h *History) Next() (string, bool) {
	if len(h.entries) == 0 || h.index == -1 {
		return "", false
	}

	// Move forward in history
	h.index++

	// If we've reached the end, reset and return empty
	if h.index >= len(h.entries) {
		h.index = -1
		return "", false
	}

	return h.entries[h.index], true
}

// Reset resets the navigation index to the end of history.
// This should be called when the user starts typing a new command
// to exit history navigation mode.
func (h *History) Reset() {
	h.index = -1
}

// Entries returns a copy of all history entries.
// The returned slice is a copy to prevent external modification.
func (h *History) Entries() []string {
	if len(h.entries) == 0 {
		return []string{}
	}

	// Return a copy to prevent external modification
	entries := make([]string, len(h.entries))
	copy(entries, h.entries)
	return entries
}

// Len returns the number of entries currently in history.
func (h *History) Len() int {
	return len(h.entries)
}
