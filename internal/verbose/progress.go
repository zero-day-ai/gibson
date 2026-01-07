package verbose

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Spinner provides a simple indeterminate progress indicator.
// It cycles through |/-\ characters to show activity during long operations.
// On TTY, updates happen in-place using carriage return. On non-TTY, it falls
// back to periodic line output.
//
// Example usage:
//
//	spinner := NewSpinner(os.Stderr, "Loading components")
//	spinner.Start()
//	defer spinner.Stop()
//	// ... long operation ...
type Spinner struct {
	writer  io.Writer
	message string
	frames  []string
	isTTY   bool

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}

	interval time.Duration
}

// spinnerFrames are the animation frames for the spinner.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// NewSpinner creates a new spinner with the given message.
// The spinner automatically detects if the writer is a TTY for optimal display.
func NewSpinner(w io.Writer, message string) *Spinner {
	return &Spinner{
		writer:   w,
		message:  message,
		frames:   spinnerFrames,
		isTTY:    isWriterTTY(w),
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
		interval: 100 * time.Millisecond, // Default 100ms update interval
	}
}

// Start begins the spinner animation in a background goroutine.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	// Recreate channels for multiple start/stop cycles
	s.stopChan = make(chan struct{})
	s.doneChan = make(chan struct{})
	s.mu.Unlock()

	go s.run()
}

// Stop halts the spinner and clears the line (on TTY).
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopChan)
	<-s.doneChan
}

// UpdateMessage changes the spinner message while it's running.
// This is thread-safe and takes effect on the next frame.
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// run is the main spinner loop.
func (s *Spinner) run() {
	defer close(s.doneChan)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frameIndex := 0

	for {
		select {
		case <-s.stopChan:
			// Clear the spinner line on TTY
			if s.isTTY {
				s.clearLine()
			}
			return

		case <-ticker.C:
			s.mu.Lock()
			frame := s.frames[frameIndex%len(s.frames)]
			message := s.message
			s.mu.Unlock()

			s.render(frame, message)
			frameIndex++
		}
	}
}

// render writes the current frame to the output.
func (s *Spinner) render(frame, message string) {
	if s.isTTY {
		// TTY: use carriage return for in-place update
		fmt.Fprintf(s.writer, "\r%s %s", frame, message)
	} else {
		// Non-TTY: write line periodically (less frequently to avoid spam)
		// Only write every 10th frame to reduce output
		// This is a simple heuristic - adjust as needed
		if strings.HasPrefix(frame, "|") {
			fmt.Fprintf(s.writer, "%s %s\n", frame, message)
		}
	}
}

// clearLine clears the current line on TTY.
func (s *Spinner) clearLine() {
	if s.isTTY {
		// Clear line: move to column 0, write spaces, move to column 0 again
		s.mu.Lock()
		messageLen := len(s.message)
		s.mu.Unlock()

		// Overwrite with spaces
		fmt.Fprintf(s.writer, "\r%s\r", strings.Repeat(" ", messageLen+2))
	}
}

// ProgressBar provides a determinate progress indicator.
// It displays a progress bar like: [=====>    ] 50% Complete
//
// On TTY, updates happen in-place. On non-TTY, it outputs progress at intervals
// to avoid excessive line output.
//
// Example usage:
//
//	bar := NewProgressBar(os.Stderr, 100, "Processing items")
//	bar.Start()
//	defer bar.Stop()
//	for i := 0; i < 100; i++ {
//	    // ... process item ...
//	    bar.Update(i + 1)
//	}
type ProgressBar struct {
	writer  io.Writer
	message string
	total   int64
	isTTY   bool

	mu       sync.Mutex
	current  int64
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}

	width            int // Width of progress bar in characters
	lastRenderedPct  int // Last rendered percentage (for non-TTY throttling)
	minUpdatePercent int // Minimum % change before non-TTY update
}

// NewProgressBar creates a new progress bar with the given total and message.
func NewProgressBar(w io.Writer, total int64, message string) *ProgressBar {
	return &ProgressBar{
		writer:           w,
		message:          message,
		total:            total,
		isTTY:            isWriterTTY(w),
		stopChan:         make(chan struct{}),
		doneChan:         make(chan struct{}),
		width:            40, // Default 40 character bar width
		minUpdatePercent: 5,  // Update non-TTY output every 5% change
	}
}

// Start initializes the progress bar.
func (p *ProgressBar) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	// Render initial state
	p.render()
}

// Update sets the current progress value and re-renders the bar.
// The value should be between 0 and total.
func (p *ProgressBar) Update(current int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	p.current = current
	p.render()
}

// Increment increases the current progress by 1.
func (p *ProgressBar) Increment() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}

	p.current++
	p.render()
}

// Stop finalizes the progress bar and clears the line (on TTY).
func (p *ProgressBar) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	// Clear the line on TTY
	if p.isTTY {
		p.clearLine()
	} else {
		// On non-TTY, write final completion line
		fmt.Fprintf(p.writer, "Progress: 100%% - %s\n", p.message)
	}

	close(p.doneChan)
}

// render draws the progress bar.
// Must be called with p.mu held.
func (p *ProgressBar) render() {
	if p.total == 0 {
		return
	}

	// Calculate percentage
	percent := int(float64(p.current) / float64(p.total) * 100)
	if percent > 100 {
		percent = 100
	}

	// For non-TTY, only update when percentage changes significantly
	if !p.isTTY && percent-p.lastRenderedPct < p.minUpdatePercent && percent != 100 {
		return
	}
	p.lastRenderedPct = percent

	if p.isTTY {
		// TTY: render full progress bar with animation
		filled := int(float64(p.width) * float64(p.current) / float64(p.total))
		if filled > p.width {
			filled = p.width
		}

		var bar strings.Builder
		bar.WriteString("[")

		// Draw filled portion
		if filled > 0 {
			bar.WriteString(strings.Repeat("=", filled-1))
			if filled < p.width {
				bar.WriteString(">")
			} else {
				bar.WriteString("=")
			}
		}

		// Draw empty portion
		empty := p.width - filled
		if empty > 0 {
			bar.WriteString(strings.Repeat(" ", empty))
		}

		bar.WriteString("]")

		// Write: [=====>    ] 50% Message
		fmt.Fprintf(p.writer, "\r%s %3d%% %s", bar.String(), percent, p.message)
	} else {
		// Non-TTY: simple text output at intervals
		fmt.Fprintf(p.writer, "Progress: %d%% - %s\n", percent, p.message)
	}
}

// clearLine clears the current line on TTY.
func (p *ProgressBar) clearLine() {
	if p.isTTY {
		// Calculate total width: bar + percentage + message + spaces
		totalWidth := p.width + 3 + 1 + len(p.message) + 4
		fmt.Fprintf(p.writer, "\r%s\r", strings.Repeat(" ", totalWidth))
	}
}

// isWriterTTY checks if a writer is connected to a terminal.
func isWriterTTY(w io.Writer) bool {
	if w == os.Stdout || w == os.Stderr {
		fileInfo, err := os.Stdout.Stat()
		if err != nil {
			return false
		}
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}
