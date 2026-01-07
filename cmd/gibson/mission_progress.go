package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/zero-day-ai/gibson/internal/mission"
)

// ProgressReporter subscribes to mission events and prints progress in verbose mode.
type ProgressReporter struct {
	verbose   bool
	quiet     bool
	out       io.Writer
	startTime time.Time
}

// NewProgressReporter creates a new ProgressReporter.
// If verbose is false or quiet is true, no output is produced.
func NewProgressReporter(verbose, quiet bool, out io.Writer) *ProgressReporter {
	return &ProgressReporter{
		verbose:   verbose,
		quiet:     quiet,
		out:       out,
		startTime: time.Now(),
	}
}

// Subscribe listens for events from the emitter and prints progress.
// This should be called before starting workflow execution.
// The returned cancel function should be called to stop listening.
func (p *ProgressReporter) Subscribe(ctx context.Context, emitter mission.EventEmitter) func() {
	// Don't subscribe if not in verbose mode or in quiet mode
	if !p.verbose || p.quiet {
		return func() {}
	}

	// Subscribe to events
	eventChan, unsubscribe := emitter.Subscribe(ctx)

	// Start goroutine to handle events
	go func() {
		for {
			select {
			case event, ok := <-eventChan:
				if !ok {
					return
				}
				p.handleEvent(event)
			case <-ctx.Done():
				return
			}
		}
	}()

	return unsubscribe
}

// handleEvent processes a mission event and prints appropriate output.
func (p *ProgressReporter) handleEvent(event mission.MissionEvent) {
	switch event.Type {
	case mission.EventMissionStarted:
		fmt.Fprintf(p.out, "[%s] Mission started\n", p.formatTimestamp())

	case mission.EventMissionFinding:
		// Handle finding events
		severity := p.getPayloadString(event.Payload, "severity")
		title := p.getPayloadString(event.Payload, "title")
		if severity != "" && title != "" {
			fmt.Fprintf(p.out, "[%s] Finding: [%s] %s\n", p.formatTimestamp(), severity, title)
		} else if title != "" {
			fmt.Fprintf(p.out, "[%s] Finding: %s\n", p.formatTimestamp(), title)
		} else {
			fmt.Fprintf(p.out, "[%s] Finding discovered\n", p.formatTimestamp())
		}

	case mission.EventMissionProgress:
		// Handle progress updates
		progress := p.getPayloadFloat(event.Payload, "progress")
		message := p.getPayloadString(event.Payload, "message")
		if message != "" {
			fmt.Fprintf(p.out, "[%s] %s (%.1f%%)\n", p.formatTimestamp(), message, progress*100)
		} else if progress > 0 {
			fmt.Fprintf(p.out, "[%s] Progress: %.1f%%\n", p.formatTimestamp(), progress*100)
		}

	case mission.EventMissionCompleted:
		duration := p.getPayloadString(event.Payload, "duration")
		if duration != "" {
			fmt.Fprintf(p.out, "[%s] Mission completed in %s\n", p.formatTimestamp(), duration)
		} else {
			elapsed := time.Since(p.startTime)
			fmt.Fprintf(p.out, "[%s] Mission completed in %s\n", p.formatTimestamp(), elapsed.Round(time.Millisecond))
		}

	case mission.EventMissionFailed:
		errMsg := p.getPayloadString(event.Payload, "error")
		if errMsg != "" {
			fmt.Fprintf(p.out, "[%s] Mission failed: %s\n", p.formatTimestamp(), errMsg)
		} else {
			fmt.Fprintf(p.out, "[%s] Mission failed\n", p.formatTimestamp())
		}

	case mission.EventMissionCancelled:
		fmt.Fprintf(p.out, "[%s] Mission cancelled\n", p.formatTimestamp())

	case mission.EventMissionPaused:
		reason := p.getPayloadString(event.Payload, "reason")
		if reason != "" {
			fmt.Fprintf(p.out, "[%s] Mission paused: %s\n", p.formatTimestamp(), reason)
		} else {
			fmt.Fprintf(p.out, "[%s] Mission paused\n", p.formatTimestamp())
		}

	case mission.EventMissionResumed:
		fmt.Fprintf(p.out, "[%s] Mission resumed\n", p.formatTimestamp())

	case mission.EventMissionConstraintViolation:
		constraint := p.getPayloadString(event.Payload, "constraint")
		message := p.getPayloadString(event.Payload, "message")
		if message != "" {
			fmt.Fprintf(p.out, "[%s] Constraint violated: %s - %s\n", p.formatTimestamp(), constraint, message)
		} else {
			fmt.Fprintf(p.out, "[%s] Constraint violated: %s\n", p.formatTimestamp(), constraint)
		}

	case mission.EventMissionApprovalRequired:
		fmt.Fprintf(p.out, "[%s] Approval required to continue\n", p.formatTimestamp())

	case mission.EventMissionCheckpoint:
		checkpointID := p.getPayloadString(event.Payload, "checkpoint_id")
		fmt.Fprintf(p.out, "[%s] Checkpoint created: %s\n", p.formatTimestamp(), checkpointID)

	default:
		// For other/custom events, print the event type
		if event.Payload != nil {
			fmt.Fprintf(p.out, "[%s] Event: %s\n", p.formatTimestamp(), event.Type)
		}
	}
}

// formatTimestamp returns a formatted timestamp relative to start time.
func (p *ProgressReporter) formatTimestamp() string {
	elapsed := time.Since(p.startTime)
	return fmt.Sprintf("%02d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
}

// getPayloadString extracts a string value from the event payload.
func (p *ProgressReporter) getPayloadString(payload any, key string) string {
	if payload == nil {
		return ""
	}

	if m, ok := payload.(map[string]interface{}); ok {
		if v, exists := m[key]; exists {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}

	return ""
}

// getPayloadFloat extracts a float64 value from the event payload.
func (p *ProgressReporter) getPayloadFloat(payload any, key string) float64 {
	if payload == nil {
		return 0
	}

	if m, ok := payload.(map[string]interface{}); ok {
		if v, exists := m[key]; exists {
			switch val := v.(type) {
			case float64:
				return val
			case float32:
				return float64(val)
			case int:
				return float64(val)
			case int64:
				return float64(val)
			}
		}
	}

	return 0
}
