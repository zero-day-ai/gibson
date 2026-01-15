package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	defaultMaxRetries    = 3
	defaultRetryDelay    = 1 * time.Second
	langfuseAPIPath      = "/api/public/ingestion"
)

// Trace represents a Langfuse trace for tracking a complete interaction.
type Trace struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	UserID    string                 `json:"userId,omitempty"`
	SessionID string                 `json:"sessionId,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Public    bool                   `json:"public,omitempty"`
}

// Generation represents an LLM generation event in Langfuse.
type Generation struct {
	ID                   string                 `json:"id"`
	TraceID              string                 `json:"traceId"`
	Name                 string                 `json:"name,omitempty"`
	StartTime            time.Time              `json:"startTime"`
	EndTime              time.Time              `json:"endTime,omitempty"`
	CompletionStartTime  time.Time              `json:"completionStartTime,omitempty"`
	Model                string                 `json:"model,omitempty"`
	ModelParameters      map[string]interface{} `json:"modelParameters,omitempty"`
	Input                interface{}            `json:"input,omitempty"`
	Output               interface{}            `json:"output,omitempty"`
	Usage                *Usage                 `json:"usage,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	ParentObservationID  string                 `json:"parentObservationId,omitempty"`
	Level                string                 `json:"level,omitempty"` // DEBUG, DEFAULT, WARNING, ERROR
	StatusMessage        string                 `json:"statusMessage,omitempty"`
	Version              string                 `json:"version,omitempty"`
	PromptName           string                 `json:"promptName,omitempty"`
	PromptVersion        int                    `json:"promptVersion,omitempty"`
}

// Span represents a generic span event in Langfuse.
type Span struct {
	ID                  string                 `json:"id"`
	TraceID             string                 `json:"traceId"`
	Name                string                 `json:"name,omitempty"`
	StartTime           time.Time              `json:"startTime"`
	EndTime             time.Time              `json:"endTime,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	Input               interface{}            `json:"input,omitempty"`
	Output              interface{}            `json:"output,omitempty"`
	ParentObservationID string                 `json:"parentObservationId,omitempty"`
	Level               string                 `json:"level,omitempty"` // DEBUG, DEFAULT, WARNING, ERROR
	StatusMessage       string                 `json:"statusMessage,omitempty"`
	Version             string                 `json:"version,omitempty"`
}

// Usage represents token usage information for LLM generations.
type Usage struct {
	PromptTokens     int64 `json:"promptTokens,omitempty"`
	CompletionTokens int64 `json:"completionTokens,omitempty"`
	TotalTokens      int64 `json:"totalTokens,omitempty"`
}

// LangfuseEvent represents a single event in the Langfuse ingestion batch.
type LangfuseEvent struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Body      map[string]interface{} `json:"body"`
}

// LangfuseBatch represents a batch of events to send to Langfuse.
type LangfuseBatch struct {
	Batch []LangfuseEvent `json:"batch"`
}

// LangfuseClient provides a client for sending telemetry data to Langfuse.
// It supports batched, async sending of traces, generations, and spans.
type LangfuseClient struct {
	host          string
	publicKey     string
	secretKey     string
	httpClient    *http.Client
	batchSize     int
	flushInterval time.Duration
	maxRetries    int
	retryDelay    time.Duration

	// Buffer for async batch sending
	buffer   []LangfuseEvent
	bufferMu sync.Mutex

	// Background flusher control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// LangfuseClientConfig holds configuration for creating a LangfuseClient.
type LangfuseClientConfig struct {
	Host          string
	PublicKey     string
	SecretKey     string
	BatchSize     int
	FlushInterval time.Duration
	MaxRetries    int
	RetryDelay    time.Duration
	HTTPTimeout   time.Duration
}

// NewLangfuseClient creates a new Langfuse client with the specified configuration.
// It starts a background goroutine for periodic flushing of buffered events.
//
// Parameters:
//   - config: Client configuration including API credentials and batch settings
//
// Returns:
//   - *LangfuseClient: The initialized client
//   - error: Any error encountered during initialization
func NewLangfuseClient(config LangfuseClientConfig) (*LangfuseClient, error) {
	// Validate required fields
	if config.Host == "" {
		return nil, NewObservabilityError(ErrAuthenticationFailed, "langfuse host is required")
	}
	if config.PublicKey == "" {
		return nil, NewObservabilityError(ErrAuthenticationFailed, "langfuse public key is required")
	}
	if config.SecretKey == "" {
		return nil, NewObservabilityError(ErrAuthenticationFailed, "langfuse secret key is required")
	}

	// Set defaults for optional fields
	if config.BatchSize <= 0 {
		config.BatchSize = defaultBatchSize
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = defaultFlushInterval
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = defaultRetryDelay
	}
	if config.HTTPTimeout <= 0 {
		config.HTTPTimeout = 30 * time.Second
	}

	client := &LangfuseClient{
		host:          config.Host,
		publicKey:     config.PublicKey,
		secretKey:     config.SecretKey,
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		maxRetries:    config.MaxRetries,
		retryDelay:    config.RetryDelay,
		httpClient: &http.Client{
			Timeout: config.HTTPTimeout,
		},
		buffer: make([]LangfuseEvent, 0, config.BatchSize),
		stopCh: make(chan struct{}),
	}

	// Start background flusher
	client.wg.Add(1)
	go client.backgroundFlusher()

	return client, nil
}

// CreateTrace creates a trace in Langfuse asynchronously.
// The trace is buffered and sent in the next batch.
//
// Parameters:
//   - trace: The trace to create
//
// Returns:
//   - error: Any error encountered during buffering
func (c *LangfuseClient) CreateTrace(trace *Trace) error {
	if trace == nil {
		return NewObservabilityError(ErrExporterConnection, "trace cannot be nil")
	}
	if trace.ID == "" {
		return NewObservabilityError(ErrExporterConnection, "trace ID is required")
	}

	// Convert to event body
	body := map[string]interface{}{
		"id":        trace.ID,
		"timestamp": trace.Timestamp.Format(time.RFC3339Nano),
	}
	if trace.Name != "" {
		body["name"] = trace.Name
	}
	if trace.UserID != "" {
		body["userId"] = trace.UserID
	}
	if trace.SessionID != "" {
		body["sessionId"] = trace.SessionID
	}
	if trace.Metadata != nil {
		body["metadata"] = trace.Metadata
	}
	if trace.Tags != nil {
		body["tags"] = trace.Tags
	}
	if trace.Public {
		body["public"] = trace.Public
	}

	event := LangfuseEvent{
		Type:      "trace-create",
		ID:        generateClientEventID("trace", trace.ID),
		Timestamp: trace.Timestamp,
		Body:      body,
	}

	return c.addEvent(event)
}

// CreateGeneration creates a generation (LLM completion) in Langfuse asynchronously.
// The generation is buffered and sent in the next batch.
//
// Parameters:
//   - gen: The generation to create
//
// Returns:
//   - error: Any error encountered during buffering
func (c *LangfuseClient) CreateGeneration(gen *Generation) error {
	if gen == nil {
		return NewObservabilityError(ErrExporterConnection, "generation cannot be nil")
	}
	if gen.ID == "" {
		return NewObservabilityError(ErrExporterConnection, "generation ID is required")
	}
	if gen.TraceID == "" {
		return NewObservabilityError(ErrExporterConnection, "generation trace ID is required")
	}

	// Convert to event body
	body := map[string]interface{}{
		"id":        gen.ID,
		"traceId":   gen.TraceID,
		"startTime": gen.StartTime.Format(time.RFC3339Nano),
	}
	if gen.Name != "" {
		body["name"] = gen.Name
	}
	if !gen.EndTime.IsZero() {
		body["endTime"] = gen.EndTime.Format(time.RFC3339Nano)
	}
	if !gen.CompletionStartTime.IsZero() {
		body["completionStartTime"] = gen.CompletionStartTime.Format(time.RFC3339Nano)
	}
	if gen.Model != "" {
		body["model"] = gen.Model
	}
	if gen.ModelParameters != nil {
		body["modelParameters"] = gen.ModelParameters
	}
	if gen.Input != nil {
		body["input"] = gen.Input
	}
	if gen.Output != nil {
		body["output"] = gen.Output
	}
	if gen.Usage != nil {
		usage := make(map[string]interface{})
		if gen.Usage.PromptTokens > 0 {
			usage["promptTokens"] = gen.Usage.PromptTokens
		}
		if gen.Usage.CompletionTokens > 0 {
			usage["completionTokens"] = gen.Usage.CompletionTokens
		}
		if gen.Usage.TotalTokens > 0 {
			usage["totalTokens"] = gen.Usage.TotalTokens
		}
		if len(usage) > 0 {
			body["usage"] = usage
		}
	}
	if gen.Metadata != nil {
		body["metadata"] = gen.Metadata
	}
	if gen.ParentObservationID != "" {
		body["parentObservationId"] = gen.ParentObservationID
	}
	if gen.Level != "" {
		body["level"] = gen.Level
	}
	if gen.StatusMessage != "" {
		body["statusMessage"] = gen.StatusMessage
	}
	if gen.Version != "" {
		body["version"] = gen.Version
	}
	if gen.PromptName != "" {
		body["promptName"] = gen.PromptName
	}
	if gen.PromptVersion > 0 {
		body["promptVersion"] = gen.PromptVersion
	}

	event := LangfuseEvent{
		Type:      "generation-create",
		ID:        generateClientEventID("generation", gen.ID),
		Timestamp: gen.StartTime,
		Body:      body,
	}

	return c.addEvent(event)
}

// CreateSpan creates a span in Langfuse asynchronously.
// The span is buffered and sent in the next batch.
//
// Parameters:
//   - span: The span to create
//
// Returns:
//   - error: Any error encountered during buffering
func (c *LangfuseClient) CreateSpan(span *Span) error {
	if span == nil {
		return NewObservabilityError(ErrExporterConnection, "span cannot be nil")
	}
	if span.ID == "" {
		return NewObservabilityError(ErrExporterConnection, "span ID is required")
	}
	if span.TraceID == "" {
		return NewObservabilityError(ErrExporterConnection, "span trace ID is required")
	}

	// Convert to event body
	body := map[string]interface{}{
		"id":        span.ID,
		"traceId":   span.TraceID,
		"startTime": span.StartTime.Format(time.RFC3339Nano),
	}
	if span.Name != "" {
		body["name"] = span.Name
	}
	if !span.EndTime.IsZero() {
		body["endTime"] = span.EndTime.Format(time.RFC3339Nano)
	}
	if span.Metadata != nil {
		body["metadata"] = span.Metadata
	}
	if span.Input != nil {
		body["input"] = span.Input
	}
	if span.Output != nil {
		body["output"] = span.Output
	}
	if span.ParentObservationID != "" {
		body["parentObservationId"] = span.ParentObservationID
	}
	if span.Level != "" {
		body["level"] = span.Level
	}
	if span.StatusMessage != "" {
		body["statusMessage"] = span.StatusMessage
	}
	if span.Version != "" {
		body["version"] = span.Version
	}

	event := LangfuseEvent{
		Type:      "span-create",
		ID:        generateClientEventID("span", span.ID),
		Timestamp: span.StartTime,
		Body:      body,
	}

	return c.addEvent(event)
}

// Flush immediately sends all buffered events to Langfuse.
// This blocks until the flush completes or fails.
//
// Returns:
//   - error: Any error encountered during flushing
func (c *LangfuseClient) Flush() error {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	return c.flush(context.Background())
}

// Close gracefully shuts down the client, flushing any pending events.
// It stops the background flusher and waits for it to complete.
//
// Returns:
//   - error: Any error encountered during shutdown
func (c *LangfuseClient) Close() error {
	// Signal background flusher to stop
	close(c.stopCh)

	// Wait for background flusher to finish with timeout
	doneCh := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(doneCh)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	select {
	case <-doneCh:
		// Background flusher stopped
	case <-ctx.Done():
		return WrapObservabilityError(ErrShutdownTimeout, "timeout waiting for background flusher", ctx.Err())
	}

	// Flush remaining events
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	if len(c.buffer) > 0 {
		if err := c.flush(ctx); err != nil {
			return WrapObservabilityError(ErrShutdownTimeout, "failed to flush remaining events", err)
		}
	}

	return nil
}

// addEvent adds an event to the buffer and triggers flush if buffer is full.
// Must be called without holding bufferMu.
func (c *LangfuseClient) addEvent(event LangfuseEvent) error {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	c.buffer = append(c.buffer, event)

	// Flush if buffer is full
	if len(c.buffer) >= c.batchSize {
		return c.flush(context.Background())
	}

	return nil
}

// backgroundFlusher periodically flushes buffered events.
func (c *LangfuseClient) backgroundFlusher() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.bufferMu.Lock()
			if len(c.buffer) > 0 {
				// Ignore errors in background flush, just log them
				_ = c.flush(context.Background())
			}
			c.bufferMu.Unlock()

		case <-c.stopCh:
			return
		}
	}
}

// flush sends buffered events to Langfuse with retry logic.
// Must be called with bufferMu locked.
func (c *LangfuseClient) flush(ctx context.Context) error {
	if len(c.buffer) == 0 {
		return nil
	}

	// Create batch payload
	batch := LangfuseBatch{
		Batch: c.buffer,
	}

	// Marshal to JSON
	payload, err := json.Marshal(batch)
	if err != nil {
		return WrapObservabilityError(ErrExporterConnection, "failed to marshal events", err)
	}

	// Send with retry logic
	if err := c.sendWithRetry(ctx, payload); err != nil {
		return err
	}

	// Clear buffer after successful send
	c.buffer = c.buffer[:0]

	return nil
}

// sendWithRetry sends a payload to Langfuse with exponential backoff retry.
func (c *LangfuseClient) sendWithRetry(ctx context.Context, payload []byte) error {
	url := c.host + langfuseAPIPath

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * c.retryDelay
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
		req.SetBasicAuth(c.publicKey, c.secretKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Read and close response body
		_, _ = io.Copy(io.Discard, resp.Body)
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

// generateClientEventID generates a unique event ID for Langfuse ingestion.
// It combines the event type and entity ID to create a deterministic ID.
func generateClientEventID(eventType, entityID string) string {
	// Use first 8 characters of entity ID for brevity
	if len(entityID) > 8 {
		entityID = entityID[:8]
	}
	return fmt.Sprintf("%s-%s-%d", eventType, entityID, time.Now().UnixNano())
}
