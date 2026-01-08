package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	defaultMaxRetries    = 3
	defaultRetryDelay    = 1 * time.Second
	langfuseAPIPath      = "/api/public/ingestion"
)

// LangfuseOption is a functional option for configuring the Langfuse exporter.
type LangfuseOption func(*langfuseOptions)

// langfuseOptions holds configuration options for the Langfuse exporter.
type langfuseOptions struct {
	batchSize     int
	flushInterval time.Duration
	maxRetries    int
	retryDelay    time.Duration
}

// WithBatchSize sets the maximum number of spans to buffer before flushing.
// Larger batch sizes reduce network overhead but increase memory usage.
func WithBatchSize(size int) LangfuseOption {
	return func(o *langfuseOptions) {
		if size > 0 {
			o.batchSize = size
		}
	}
}

// WithFlushInterval sets the maximum time between automatic flushes.
// Spans are automatically flushed when this interval expires.
func WithFlushInterval(interval time.Duration) LangfuseOption {
	return func(o *langfuseOptions) {
		if interval > 0 {
			o.flushInterval = interval
		}
	}
}

// WithRetryPolicy sets the retry behavior for failed HTTP requests.
// The exporter will retry up to maxRetries times with exponential backoff.
func WithRetryPolicy(maxRetries int, retryDelay time.Duration) LangfuseOption {
	return func(o *langfuseOptions) {
		if maxRetries >= 0 {
			o.maxRetries = maxRetries
		}
		if retryDelay > 0 {
			o.retryDelay = retryDelay
		}
	}
}

// LangfuseExporter exports OpenTelemetry spans to Langfuse for LLM observability.
// It implements the sdktrace.SpanExporter interface and provides batching,
// buffering, and retry capabilities for reliable span export.
type LangfuseExporter struct {
	client        *http.Client
	host          string
	publicKey     string
	secretKey     string
	buffer        []sdktrace.ReadOnlySpan
	bufferMu      sync.Mutex
	batchSize     int
	flushInterval time.Duration
	maxRetries    int
	retryDelay    time.Duration
	sessionID     string
	userID        string
	sessionMu     sync.RWMutex
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewLangfuseExporter creates a new Langfuse exporter with the specified configuration.
// It starts a background goroutine for periodic flushing of buffered spans.
//
// Parameters:
//   - cfg: Langfuse configuration with host, public key, and secret key
//   - opts: Optional functional options for customizing behavior
//
// Returns:
//   - *LangfuseExporter: The initialized exporter
//   - error: Any error encountered during initialization
func NewLangfuseExporter(cfg LangfuseConfig, opts ...LangfuseOption) (*LangfuseExporter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, WrapObservabilityError(ErrAuthenticationFailed, "invalid langfuse configuration", err)
	}

	// Set default options
	options := &langfuseOptions{
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		maxRetries:    defaultMaxRetries,
		retryDelay:    defaultRetryDelay,
	}

	// Apply functional options
	for _, opt := range opts {
		opt(options)
	}

	exporter := &LangfuseExporter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		host:          cfg.Host,
		publicKey:     cfg.PublicKey,
		secretKey:     cfg.SecretKey,
		buffer:        make([]sdktrace.ReadOnlySpan, 0, options.batchSize),
		batchSize:     options.batchSize,
		flushInterval: options.flushInterval,
		maxRetries:    options.maxRetries,
		retryDelay:    options.retryDelay,
		stopCh:        make(chan struct{}),
	}

	// Start background flusher
	exporter.wg.Add(1)
	go exporter.backgroundFlusher()

	return exporter, nil
}

// ExportSpans exports a batch of spans to Langfuse.
// This method implements the sdktrace.SpanExporter interface.
//
// Spans are buffered and exported in batches to reduce network overhead.
// If the buffer reaches the configured batch size, spans are immediately flushed.
//
// Parameters:
//   - ctx: Context for the export operation
//   - spans: Slice of read-only spans to export
//
// Returns:
//   - error: Any error encountered during export
func (e *LangfuseExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}

	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()

	// Add spans to buffer
	e.buffer = append(e.buffer, spans...)

	// Flush if buffer is full
	if len(e.buffer) >= e.batchSize {
		return e.flush(ctx)
	}

	return nil
}

// Shutdown gracefully shuts down the exporter, flushing any pending spans.
// This method implements the sdktrace.SpanExporter interface.
//
// Parameters:
//   - ctx: Context with optional timeout for shutdown operation
//
// Returns:
//   - error: Any error encountered during shutdown
func (e *LangfuseExporter) Shutdown(ctx context.Context) error {
	// Signal background flusher to stop
	close(e.stopCh)

	// Wait for background flusher to finish
	doneCh := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// Background flusher stopped
	case <-ctx.Done():
		return WrapObservabilityError(ErrShutdownTimeout, "timeout waiting for background flusher", ctx.Err())
	}

	// Flush remaining spans
	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()

	if len(e.buffer) > 0 {
		if err := e.flush(ctx); err != nil {
			return WrapObservabilityError(ErrShutdownTimeout, "failed to flush remaining spans", err)
		}
	}

	return nil
}

// SetSession sets the session ID to be associated with all exported spans.
// This allows grouping spans by session in the Langfuse UI.
func (e *LangfuseExporter) SetSession(sessionID string) {
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	e.sessionID = sessionID
}

// SetUser sets the user ID to be associated with all exported spans.
// This allows filtering and grouping spans by user in the Langfuse UI.
func (e *LangfuseExporter) SetUser(userID string) {
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	e.userID = userID
}

// backgroundFlusher periodically flushes buffered spans.
func (e *LangfuseExporter) backgroundFlusher() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.bufferMu.Lock()
			if len(e.buffer) > 0 {
				// Use background context for periodic flushes
				_ = e.flush(context.Background())
			}
			e.bufferMu.Unlock()

		case <-e.stopCh:
			return
		}
	}
}

// flush sends buffered spans to Langfuse with retry logic.
// Must be called with bufferMu locked.
func (e *LangfuseExporter) flush(ctx context.Context) error {
	if len(e.buffer) == 0 {
		return nil
	}

	// Convert spans to Langfuse format
	traces := e.convertSpans(e.buffer)

	// Marshal to JSON
	payload, err := json.Marshal(map[string]interface{}{
		"batch": traces,
	})
	if err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to marshal spans", err)
	}

	// Send with retry logic
	if err := e.sendWithRetry(ctx, payload); err != nil {
		return err
	}

	// Clear buffer after successful send
	e.buffer = e.buffer[:0]

	return nil
}

// sendWithRetry sends a payload to Langfuse with exponential backoff retry.
func (e *LangfuseExporter) sendWithRetry(ctx context.Context, payload []byte) error {
	url := e.host + langfuseAPIPath

	var lastErr error
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * e.retryDelay
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return WrapObservabilityError(ErrExporterConnection, "context cancelled during retry", ctx.Err())
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(e.publicKey, e.secretKey)

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Handle response
		resp.Body.Close()

		// Success
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		// Retryable errors: 429 (rate limit), 500 (internal error), 503 (service unavailable)
		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError ||
			resp.StatusCode == http.StatusServiceUnavailable {
			lastErr = fmt.Errorf("HTTP %d: retryable error", resp.StatusCode)
			continue
		}

		// Non-retryable error
		return NewExporterConnectionError(url, fmt.Errorf("HTTP %d: non-retryable error", resp.StatusCode))
	}

	// All retries exhausted
	return NewExporterConnectionError(url, fmt.Errorf("max retries exceeded: %w", lastErr))
}

// convertSpans converts OpenTelemetry spans to Langfuse ingestion event format.
// The Langfuse API expects events with a "type" field indicating the event type
// (trace-create, span-create, generation-create, etc.) and a "body" field with the data.
func (e *LangfuseExporter) convertSpans(spans []sdktrace.ReadOnlySpan) []map[string]interface{} {
	e.sessionMu.RLock()
	sessionID := e.sessionID
	userID := e.userID
	e.sessionMu.RUnlock()

	events := make([]map[string]interface{}, 0, len(spans)*2)

	// Group spans by trace ID to create trace-create events
	tracesSeen := make(map[string]bool)

	for _, span := range spans {
		spanCtx := span.SpanContext()
		parentCtx := span.Parent()
		traceID := spanCtx.TraceID().String()

		// Create trace-create event for new traces (root spans without parent)
		if !tracesSeen[traceID] {
			tracesSeen[traceID] = true

			traceEvent := map[string]interface{}{
				"type":      "trace-create",
				"id":        "trace-" + traceID[:8], // Use first 8 chars of trace ID as unique event ID
				"timestamp": span.StartTime().Format(time.RFC3339Nano),
				"body": map[string]interface{}{
					"id":        traceID,
					"name":      span.Name(),
					"timestamp": span.StartTime().Format(time.RFC3339Nano),
				},
			}

			// Add session ID if set
			if sessionID != "" {
				traceEvent["body"].(map[string]interface{})["sessionId"] = sessionID
			}

			// Add user ID if set
			if userID != "" {
				traceEvent["body"].(map[string]interface{})["userId"] = userID
			}

			events = append(events, traceEvent)
		}

		// Create span or generation event based on span type
		eventType := "span-create"
		if span.SpanKind() == trace.SpanKindClient {
			eventType = "generation-create"
		}

		body := map[string]interface{}{
			"id":        spanCtx.SpanID().String(),
			"traceId":   traceID,
			"name":      span.Name(),
			"startTime": span.StartTime().Format(time.RFC3339Nano),
			"endTime":   span.EndTime().Format(time.RFC3339Nano),
		}

		// Add parent span if exists
		if parentCtx.IsValid() {
			body["parentObservationId"] = parentCtx.SpanID().String()
		}

		// Add attributes as metadata
		metadata := make(map[string]interface{})
		for _, attr := range span.Attributes() {
			metadata[string(attr.Key)] = attr.Value.AsInterface()
		}
		body["metadata"] = metadata

		// Add status
		status := span.Status()
		body["level"] = statusCodeToLevel(status.Code)
		if status.Description != "" {
			body["statusMessage"] = status.Description
		}

		spanEvent := map[string]interface{}{
			"type":      eventType,
			"id":        "span-" + spanCtx.SpanID().String()[:8],
			"timestamp": span.StartTime().Format(time.RFC3339Nano),
			"body":      body,
		}

		events = append(events, spanEvent)
	}

	return events
}

// spanKindToLangfuse converts OpenTelemetry span kind to Langfuse observation type.
func spanKindToLangfuse(kind trace.SpanKind) string {
	switch kind {
	case trace.SpanKindClient:
		return "GENERATION"
	case trace.SpanKindServer:
		return "SPAN"
	case trace.SpanKindInternal:
		return "SPAN"
	default:
		return "SPAN"
	}
}

// statusCodeToLevel converts OpenTelemetry status code to Langfuse level.
func statusCodeToLevel(code codes.Code) string {
	switch code {
	case codes.Error:
		return "ERROR"
	case codes.Ok:
		return "DEFAULT"
	default:
		return "DEFAULT"
	}
}
