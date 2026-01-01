package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// MockCall represents a recorded method call on the mock embedder.
type MockCall struct {
	Method    string
	Args      []interface{}
	Timestamp time.Time
}

// MockEmbedder is a mock implementation of Embedder for testing.
// It generates deterministic embeddings based on text hash, ensuring
// the same text always produces the same embedding.
type MockEmbedder struct {
	mu           sync.RWMutex
	dimensions   int
	model        string
	calls        []MockCall
	embedError   error
	batchError   error
	healthStatus types.HealthStatus
}

// NewMockEmbedder creates a new mock embedder for testing.
func NewMockEmbedder() *MockEmbedder {
	return &MockEmbedder{
		dimensions:   1536, // Default to OpenAI text-embedding-3-small dimensions
		model:        "mock-embedder",
		calls:        make([]MockCall, 0),
		healthStatus: types.NewHealthStatus(types.HealthStateHealthy, "mock embedder"),
	}
}

// Embed generates a deterministic embedding for a single text.
// The embedding is derived from a SHA256 hash of the text, ensuring
// consistency across calls with the same input.
func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, MockCall{
		Method:    "Embed",
		Args:      []interface{}{text},
		Timestamp: time.Now(),
	})

	if m.embedError != nil {
		return nil, m.embedError
	}

	return m.generateEmbedding(text), nil
}

// EmbedBatch generates deterministic embeddings for multiple texts.
func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, MockCall{
		Method:    "EmbedBatch",
		Args:      []interface{}{texts},
		Timestamp: time.Now(),
	})

	if m.batchError != nil {
		return nil, m.batchError
	}

	embeddings := make([][]float64, len(texts))
	for i, text := range texts {
		embeddings[i] = m.generateEmbedding(text)
	}

	return embeddings, nil
}

// generateEmbedding creates a deterministic embedding from text using SHA256.
// The hash is used to seed a pseudo-random number generator, which produces
// consistent float64 values in the range [-1, 1].
func (m *MockEmbedder) generateEmbedding(text string) []float64 {
	// Hash the text to get deterministic seed
	hash := sha256.Sum256([]byte(text))

	// Use first 8 bytes of hash as seed
	seed := int64(binary.BigEndian.Uint64(hash[:8]))
	rng := rand.New(rand.NewSource(seed))

	// Generate embedding vector
	embedding := make([]float64, m.dimensions)
	for i := 0; i < m.dimensions; i++ {
		// Generate value in range [-1, 1]
		embedding[i] = (rng.Float64() * 2) - 1
	}

	// Normalize the vector to unit length (optional, but more realistic)
	return normalizeVector(embedding)
}

// Dimensions returns the dimensionality of the embedding vectors.
func (m *MockEmbedder) Dimensions() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dimensions
}

// Model returns the name of the mock embedding model.
func (m *MockEmbedder) Model() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.model
}

// Health returns the configured health status.
func (m *MockEmbedder) Health(ctx context.Context) types.HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, MockCall{
		Method:    "Health",
		Args:      []interface{}{},
		Timestamp: time.Now(),
	})

	return m.healthStatus
}

// SetDimensions allows changing the embedding dimensions for testing.
func (m *MockEmbedder) SetDimensions(dims int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dimensions = dims
}

// SetModel allows changing the model name for testing.
func (m *MockEmbedder) SetModel(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.model = model
}

// SetEmbedError configures Embed() to return an error.
func (m *MockEmbedder) SetEmbedError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.embedError = err
}

// SetBatchError configures EmbedBatch() to return an error.
func (m *MockEmbedder) SetBatchError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batchError = err
}

// SetHealthStatus configures what Health() should return.
func (m *MockEmbedder) SetHealthStatus(status types.HealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthStatus = status
}

// GetCalls returns all recorded method calls.
func (m *MockEmbedder) GetCalls() []MockCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent race conditions
	calls := make([]MockCall, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// GetCallsByMethod returns all calls to a specific method.
func (m *MockEmbedder) GetCallsByMethod(method string) []MockCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]MockCall, 0)
	for _, call := range m.calls {
		if call.Method == method {
			calls = append(calls, call)
		}
	}
	return calls
}

// CallCount returns the total number of method calls.
func (m *MockEmbedder) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.calls)
}

// Reset clears all recorded calls and resets the mock to its initial state.
func (m *MockEmbedder) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = make([]MockCall, 0)
	m.embedError = nil
	m.batchError = nil
	m.dimensions = 1536
	m.model = "mock-embedder"
	m.healthStatus = types.NewHealthStatus(types.HealthStateHealthy, "mock embedder")
}

// normalizeVector normalizes a vector to unit length.
func normalizeVector(v []float64) []float64 {
	var sum float64
	for _, val := range v {
		sum += val * val
	}

	if sum == 0 {
		return v
	}

	norm := math.Sqrt(sum)
	normalized := make([]float64, len(v))
	for i, val := range v {
		normalized[i] = val / norm
	}

	return normalized
}
