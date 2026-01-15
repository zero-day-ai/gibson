package observability

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLangfuseClient_ValidConfig(t *testing.T) {
	cfg := LangfuseClientConfig{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	}

	client, err := NewLangfuseClient(cfg)

	require.NoError(t, err)
	require.NotNil(t, client)

	// Clean up
	_ = client.Close()
}

func TestNewLangfuseClient_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  LangfuseClientConfig
	}{
		{
			name: "missing host",
			cfg: LangfuseClientConfig{
				PublicKey: "pk-test",
				SecretKey: "sk-test",
			},
		},
		{
			name: "missing public key",
			cfg: LangfuseClientConfig{
				Host:      "http://localhost:3000",
				SecretKey: "sk-test",
			},
		},
		{
			name: "missing secret key",
			cfg: LangfuseClientConfig{
				Host:      "http://localhost:3000",
				PublicKey: "pk-test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewLangfuseClient(tt.cfg)

			assert.Error(t, err)
			assert.Nil(t, client)
		})
	}
}

func TestNewLangfuseClient_WithCustomSettings(t *testing.T) {
	cfg := LangfuseClientConfig{
		Host:          "http://localhost:3000",
		PublicKey:     "pk-test",
		SecretKey:     "sk-test",
		BatchSize:     50,
		FlushInterval: 10 * time.Second,
		MaxRetries:    5,
		RetryDelay:    2 * time.Second,
		HTTPTimeout:   60 * time.Second,
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.Equal(t, 50, client.batchSize)
	assert.Equal(t, 10*time.Second, client.flushInterval)
	assert.Equal(t, 5, client.maxRetries)
	assert.Equal(t, 2*time.Second, client.retryDelay)
	assert.Equal(t, 60*time.Second, client.httpClient.Timeout)

	_ = client.Close()
}

func TestLangfuseClient_CreateTrace(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		assert.Equal(t, "/api/public/ingestion", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Read and validate request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var batch LangfuseBatch
		err = json.Unmarshal(body, &batch)
		require.NoError(t, err)

		assert.Len(t, batch.Batch, 1)
		assert.Equal(t, "trace-create", batch.Batch[0].Type)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BatchSize: 1, // Flush immediately
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	trace := &Trace{
		ID:        "trace-123",
		Name:      "Test Trace",
		Timestamp: time.Now(),
		UserID:    "user-1",
		SessionID: "session-1",
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	err = client.CreateTrace(trace)
	require.NoError(t, err)

	// Wait a bit for async sending
	time.Sleep(100 * time.Millisecond)
	assert.True(t, requestReceived, "Request should have been sent")
}

func TestLangfuseClient_CreateGeneration(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var batch LangfuseBatch
		err = json.Unmarshal(body, &batch)
		require.NoError(t, err)

		assert.Len(t, batch.Batch, 1)
		assert.Equal(t, "generation-create", batch.Batch[0].Type)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BatchSize: 1,
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	gen := &Generation{
		ID:        "gen-123",
		TraceID:   "trace-123",
		Name:      "Test Generation",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Second),
		Model:     "gpt-4",
		Input:     "What is the meaning of life?",
		Output:    "42",
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	err = client.CreateGeneration(gen)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	assert.True(t, requestReceived)
}

func TestLangfuseClient_CreateSpan(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var batch LangfuseBatch
		err = json.Unmarshal(body, &batch)
		require.NoError(t, err)

		assert.Len(t, batch.Batch, 1)
		assert.Equal(t, "span-create", batch.Batch[0].Type)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BatchSize: 1,
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	span := &Span{
		ID:        "span-123",
		TraceID:   "trace-123",
		Name:      "Test Span",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(1 * time.Second),
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	err = client.CreateSpan(span)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	assert.True(t, requestReceived)
}

func TestLangfuseClient_Batching(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var batch LangfuseBatch
		err = json.Unmarshal(body, &batch)
		require.NoError(t, err)

		// Should receive 2 events in the batch
		assert.Len(t, batch.Batch, 2)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BatchSize: 2, // Batch 2 events
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	// Create 2 traces - should trigger one batch send
	trace1 := &Trace{
		ID:        "trace-1",
		Timestamp: time.Now(),
	}
	trace2 := &Trace{
		ID:        "trace-2",
		Timestamp: time.Now(),
	}

	err = client.CreateTrace(trace1)
	require.NoError(t, err)
	err = client.CreateTrace(trace2)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, requestCount, "Should have sent exactly 1 batch request")
}

func TestLangfuseClient_Flush(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BatchSize: 100, // Large batch size to prevent auto-flush
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	trace := &Trace{
		ID:        "trace-123",
		Timestamp: time.Now(),
	}

	err = client.CreateTrace(trace)
	require.NoError(t, err)

	// Should not have sent yet
	time.Sleep(50 * time.Millisecond)
	assert.False(t, requestReceived)

	// Manual flush
	err = client.Flush()
	require.NoError(t, err)

	assert.True(t, requestReceived)
}

func TestLangfuseClient_BackgroundFlusher(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:          server.URL,
		PublicKey:     "pk-test",
		SecretKey:     "sk-test",
		BatchSize:     100,
		FlushInterval: 200 * time.Millisecond, // Short interval for testing
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	trace := &Trace{
		ID:        "trace-123",
		Timestamp: time.Now(),
	}

	err = client.CreateTrace(trace)
	require.NoError(t, err)

	// Wait for background flusher
	time.Sleep(300 * time.Millisecond)
	assert.True(t, requestReceived)
}

func TestLangfuseClient_RetryOnFailure(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:       server.URL,
		PublicKey:  "pk-test",
		SecretKey:  "sk-test",
		BatchSize:  1,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	trace := &Trace{
		ID:        "trace-123",
		Timestamp: time.Now(),
	}

	err = client.CreateTrace(trace)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, attemptCount, 3, "Should have retried at least 3 times")
}

func TestLangfuseClient_ValidationErrors(t *testing.T) {
	cfg := LangfuseClientConfig{
		Host:      "http://localhost:3000",
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	t.Run("nil trace", func(t *testing.T) {
		err := client.CreateTrace(nil)
		assert.Error(t, err)
	})

	t.Run("empty trace ID", func(t *testing.T) {
		err := client.CreateTrace(&Trace{})
		assert.Error(t, err)
	})

	t.Run("nil generation", func(t *testing.T) {
		err := client.CreateGeneration(nil)
		assert.Error(t, err)
	})

	t.Run("empty generation ID", func(t *testing.T) {
		err := client.CreateGeneration(&Generation{TraceID: "trace-123"})
		assert.Error(t, err)
	})

	t.Run("empty generation trace ID", func(t *testing.T) {
		err := client.CreateGeneration(&Generation{ID: "gen-123"})
		assert.Error(t, err)
	})

	t.Run("nil span", func(t *testing.T) {
		err := client.CreateSpan(nil)
		assert.Error(t, err)
	})

	t.Run("empty span ID", func(t *testing.T) {
		err := client.CreateSpan(&Span{TraceID: "trace-123"})
		assert.Error(t, err)
	})

	t.Run("empty span trace ID", func(t *testing.T) {
		err := client.CreateSpan(&Span{ID: "span-123"})
		assert.Error(t, err)
	})
}

func TestLangfuseClient_Close(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
		BatchSize: 100, // Large batch to prevent auto-flush
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)

	trace := &Trace{
		ID:        "trace-123",
		Timestamp: time.Now(),
	}

	err = client.CreateTrace(trace)
	require.NoError(t, err)

	// Close should flush pending events
	err = client.Close()
	require.NoError(t, err)

	assert.True(t, requestReceived, "Close should flush remaining events")
}

func TestLangfuseClient_AuthenticationHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "pk-test-key", username)
		assert.Equal(t, "sk-test-key", password)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := LangfuseClientConfig{
		Host:      server.URL,
		PublicKey: "pk-test-key",
		SecretKey: "sk-test-key",
		BatchSize: 1,
	}

	client, err := NewLangfuseClient(cfg)
	require.NoError(t, err)
	defer client.Close()

	trace := &Trace{
		ID:        "trace-123",
		Timestamp: time.Now(),
	}

	err = client.CreateTrace(trace)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
}
