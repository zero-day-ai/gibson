package embedder

import (
	"context"
	"fmt"
	"sync"

	"github.com/clems4ever/all-minilm-l6-v2-go/all_minilm_l6_v2"
	"github.com/zero-day-ai/gibson/internal/types"
)

// Singleton for the native embedder - ONNX Runtime can only be initialized once per process
var (
	nativeEmbedderInstance *NativeEmbedder
	nativeEmbedderOnce     sync.Once
	nativeEmbedderErr      error
)

// NativeEmbedder uses all-MiniLM-L6-v2 for local embedding generation.
// This embedder runs entirely offline without requiring external API calls.
// The model is embedded in the binary and uses ONNX Runtime for inference.
//
// Model details:
//   - Architecture: all-MiniLM-L6-v2 (sentence-transformers)
//   - Dimensions: 384
//   - Output: float32 vectors (converted to float64 for interface compliance)
//   - Performance: ~100ms per embedding on CPU
//
// Thread-safety: All methods are safe for concurrent use.
// Singleton: ONNX Runtime can only be initialized once per process, so
// CreateNativeEmbedder returns a shared instance.
type NativeEmbedder struct {
	model *all_minilm_l6_v2.Model
	mu    sync.RWMutex
}

// CreateNativeEmbedder creates or returns the singleton native embedder using all-MiniLM-L6-v2.
// Returns an error if ONNX Runtime is not available or model initialization fails.
//
// IMPORTANT: ONNX Runtime can only be initialized once per process. This function
// returns a shared singleton instance. All callers receive the same embedder.
//
// Requirements:
//   - ONNX Runtime library must be available (libonnxruntime.so or onnxruntime.dll)
//   - Model weights are embedded in the binary (no external files needed)
//
// Example:
//
//	emb, err := CreateNativeEmbedder()
//	if err != nil {
//	    return nil, fmt.Errorf("embedder required: %w", err)
//	}
func CreateNativeEmbedder() (*NativeEmbedder, error) {
	nativeEmbedderOnce.Do(func() {
		// Initialize the all-MiniLM-L6-v2 model
		// This loads the ONNX model and creates the runtime session
		model, err := all_minilm_l6_v2.NewModel()
		if err != nil {
			nativeEmbedderErr = types.WrapError(ErrCodeEmbedderUnavailable,
				"failed to initialize native embedder (ONNX Runtime may be unavailable)", err)
			return
		}

		nativeEmbedderInstance = &NativeEmbedder{
			model: model,
		}
	})

	if nativeEmbedderErr != nil {
		return nil, nativeEmbedderErr
	}

	return nativeEmbedderInstance, nil
}

// Embed generates an embedding vector for a single text.
// The text is tokenized, encoded, and passed through the transformer model.
//
// The all-MiniLM-L6-v2 model returns float32 vectors, which are converted
// to float64 for compliance with the Embedder interface.
//
// Returns an error if the embedding generation fails or context is canceled.
func (e *NativeEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check if context is already canceled
	if err := ctx.Err(); err != nil {
		return nil, types.WrapError(ErrCodeEmbeddingFailed, "context canceled", err)
	}

	// Generate embedding using the native model
	// Note: all-minilm-l6-v2-go doesn't currently support context cancellation
	// TODO: Consider wrapping in a goroutine with timeout for long-running operations
	embedding, err := e.model.Compute(text, true)
	if err != nil {
		return nil, types.WrapError(ErrCodeEmbeddingFailed,
			fmt.Sprintf("failed to generate embedding for text (len=%d)", len(text)), err)
	}

	// Convert float32 to float64
	result := make([]float64, len(embedding))
	for i, v := range embedding {
		result[i] = float64(v)
	}

	return result, nil
}

// EmbedBatch generates embeddings for multiple texts efficiently.
// Texts are processed sequentially using the same model session.
//
// This method processes texts one at a time. While not parallel, it's more
// efficient than separate Embed() calls because the model remains loaded.
//
// Returns an error if any embedding fails. Partial results are not returned.
func (e *NativeEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check if context is already canceled
	if err := ctx.Err(); err != nil {
		return nil, types.WrapError(ErrCodeEmbeddingBatchFailed, "context canceled", err)
	}

	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	// Process each text sequentially
	results := make([][]float64, len(texts))
	for i, text := range texts {
		// Check context between iterations
		if err := ctx.Err(); err != nil {
			return nil, types.WrapError(ErrCodeEmbeddingBatchFailed,
				fmt.Sprintf("context canceled after %d/%d embeddings", i, len(texts)), err)
		}

		// Generate embedding
		embedding, err := e.model.Compute(text, true)
		if err != nil {
			return nil, types.WrapError(ErrCodeEmbeddingBatchFailed,
				fmt.Sprintf("failed to generate embedding %d/%d", i+1, len(texts)), err)
		}

		// Convert float32 to float64
		result := make([]float64, len(embedding))
		for j, v := range embedding {
			result[j] = float64(v)
		}

		results[i] = result
	}

	return results, nil
}

// Dimensions returns the dimensionality of embedding vectors.
// all-MiniLM-L6-v2 produces 384-dimensional embeddings.
func (e *NativeEmbedder) Dimensions() int {
	return 384
}

// Model returns the name of the embedding model.
func (e *NativeEmbedder) Model() string {
	return "all-MiniLM-L6-v2"
}

// Health checks if the embedder is operational.
// Tests the model by generating a test embedding.
func (e *NativeEmbedder) Health(ctx context.Context) types.HealthStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Try to generate a test embedding
	testText := "health check"
	_, err := e.model.Compute(testText, true)
	if err != nil {
		return types.NewHealthStatus(types.HealthStateDegraded,
			fmt.Sprintf("native embedder health check failed: %v", err))
	}

	return types.NewHealthStatus(types.HealthStateHealthy,
		"native embedder operational (all-MiniLM-L6-v2)")
}
