package verbose

import (
	"context"
	"errors"
	"io"
	"sync"
	"syscall"
)

// VerboseWriter subscribes to a verbose event bus, formats events,
// and writes them to an output stream. It handles filtering by level
// and manages graceful shutdown.
type VerboseWriter struct {
	bus       *DefaultVerboseEventBus
	formatter VerboseFormatter
	writer    io.Writer
	level     VerboseLevel
	done      chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	started   bool
}

// NewVerboseWriter creates a new VerboseWriter.
//
// Parameters:
//   - w: Output writer (typically os.Stdout or os.Stderr)
//   - level: Minimum verbosity level to output (events with level > this are filtered)
//   - jsonOutput: If true, use JSON formatter; otherwise use text formatter
func NewVerboseWriter(w io.Writer, level VerboseLevel, jsonOutput bool) *VerboseWriter {
	var formatter VerboseFormatter
	if jsonOutput {
		formatter = NewJSONVerboseFormatter()
	} else {
		formatter = NewTextVerboseFormatter()
	}

	return &VerboseWriter{
		bus:       NewDefaultVerboseEventBus(),
		formatter: formatter,
		writer:    w,
		level:     level,
		done:      make(chan struct{}),
		started:   false,
	}
}

// Start begins processing events from the bus.
// This should be called in a separate goroutine or before emitting events.
// Returns immediately after starting the background goroutine.
func (vw *VerboseWriter) Start(ctx context.Context) {
	vw.mu.Lock()
	if vw.started {
		vw.mu.Unlock()
		return
	}
	vw.started = true
	vw.mu.Unlock()

	// Subscribe to the bus
	eventCh, cleanup := vw.bus.Subscribe(ctx)

	vw.wg.Add(1)
	go func() {
		defer vw.wg.Done()
		defer cleanup()

		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					// Channel closed, exit
					return
				}

				// Filter by level - only show events at or below configured level
				if event.Level > vw.level {
					continue
				}

				// Format the event
				formatted := vw.formatter.Format(event)

				// Write to output (handle EPIPE gracefully)
				if err := vw.safeWrite(formatted); err != nil {
					if isEPIPE(err) {
						// Broken pipe - reader has closed (e.g., `gibson attack | head`)
						// Gracefully exit instead of panicking
						return
					}
					// Other write errors are ignored to prevent blocking
					// In production, you might want to log these to a separate error channel
				}

			case <-vw.done:
				// Shutdown signal received
				return

			case <-ctx.Done():
				// Context cancelled
				return
			}
		}
	}()
}

// Stop signals the writer to stop processing events and waits for completion.
// This closes the event bus and blocks until the background goroutine exits.
func (vw *VerboseWriter) Stop() {
	vw.mu.Lock()
	if !vw.started {
		vw.mu.Unlock()
		return
	}
	vw.mu.Unlock()

	// Signal shutdown
	close(vw.done)

	// Close the bus (this will close all subscriber channels)
	vw.bus.Close()

	// Wait for the goroutine to finish
	vw.wg.Wait()
}

// Bus returns the underlying event bus for emitting events.
func (vw *VerboseWriter) Bus() VerboseEventBus {
	return vw.bus
}

// safeWrite writes data to the output writer, handling errors gracefully.
func (vw *VerboseWriter) safeWrite(data string) error {
	_, err := vw.writer.Write([]byte(data))
	return err
}

// isEPIPE checks if an error is a broken pipe (EPIPE) error.
// This occurs when the reader has closed the pipe (e.g., `command | head -n 10`).
func isEPIPE(err error) bool {
	if err == nil {
		return false
	}

	// Check for EPIPE syscall error
	var pathErr *syscall.Errno
	if errors.As(err, &pathErr) {
		return *pathErr == syscall.EPIPE
	}

	// Check for EPIPE in error chain
	return errors.Is(err, syscall.EPIPE)
}
