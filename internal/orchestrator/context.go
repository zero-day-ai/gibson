package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/zero-day-ai/gibson/internal/payload"
)

// PayloadContextInjector provides payload context injection for agent orchestration
type PayloadContextInjector struct {
	payloadStore payload.PayloadStore
}

// NewPayloadContextInjector creates a new payload context injector
func NewPayloadContextInjector(payloadStore payload.PayloadStore) *PayloadContextInjector {
	return &PayloadContextInjector{
		payloadStore: payloadStore,
	}
}

// InjectPayloadContext adds payload information to the observation state based on target type
func (pci *PayloadContextInjector) InjectPayloadContext(ctx context.Context, state *ObservationState, targetType string) error {
	if state == nil {
		return fmt.Errorf("observation state cannot be nil")
	}

	if pci.payloadStore == nil {
		// No payload store configured, skip injection
		return nil
	}

	// Query payload store for payloads matching the target type
	summary, err := pci.payloadStore.GetSummaryForTargetType(ctx, targetType)
	if err != nil {
		return fmt.Errorf("failed to get payload summary: %w", err)
	}

	// Skip if no payloads available
	if summary.Total == 0 {
		return nil
	}

	// Build payload context string
	payloadContext := buildPayloadContextString(summary)

	// Add to observation state as additional context
	// This will be included in the prompt sent to the LLM
	if state.PayloadContext == "" {
		state.PayloadContext = payloadContext
	}

	return nil
}

// buildPayloadContextString creates a formatted string describing available payloads
func buildPayloadContextString(summary *payload.PayloadSummary) string {
	var sb strings.Builder

	sb.WriteString("## Available Payloads\n\n")
	sb.WriteString(fmt.Sprintf("You have access to %d attack payloads:\n", summary.Total))

	// List counts by category
	if len(summary.ByCategory) > 0 {
		for category, count := range summary.ByCategory {
			sb.WriteString(fmt.Sprintf("- %s: %d\n", category.String(), count))
		}
	}

	// Add severity breakdown if available
	if len(summary.BySeverity) > 0 {
		sb.WriteString("\n**Severity Breakdown:**\n")
		for severity, count := range summary.BySeverity {
			sb.WriteString(fmt.Sprintf("- %s: %d\n", severity, count))
		}
	}

	// Instructions for using payloads
	sb.WriteString("\n**Using Payloads:**\n")
	sb.WriteString("1. Use the `payload_search` tool to find relevant payloads by category, tags, or text query\n")
	sb.WriteString("2. Use the `payload_execute` tool to run a payload against the target\n")
	sb.WriteString("3. Payloads have built-in success detection and will automatically create findings\n")
	sb.WriteString("4. Consider payload severity and reliability when choosing which to execute\n")

	return sb.String()
}
