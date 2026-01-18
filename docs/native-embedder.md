# Native Embedder

The native embedder provides offline, CPU-based embedding generation for Gibson using the all-MiniLM-L6-v2 model. It runs entirely locally without requiring external API calls, making it ideal for air-gapped environments and reducing operational costs.

## Overview

| Property | Value |
|----------|-------|
| Model | all-MiniLM-L6-v2 (sentence-transformers) |
| Dimensions | 384 |
| Output Type | float64 vectors |
| Runtime | ONNX Runtime (CPU) |
| Performance | ~100ms per embedding |
| Thread Safety | Yes (all methods are concurrent-safe) |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Gibson Daemon                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────┐    ┌─────────────────┐                    │
│  │  Memory Manager │───▶│ Native Embedder │                    │
│  │  (Long-Term)    │    │ (all-MiniLM-L6) │                    │
│  └─────────────────┘    └────────┬────────┘                    │
│                                  │                              │
│  ┌─────────────────┐    ┌────────▼────────┐                    │
│  │  GraphRAG Store │───▶│   ONNX Runtime  │                    │
│  │                 │    │   (libonnxruntime)                   │
│  └─────────────────┘    └─────────────────┘                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Systems Using Native Embedder

### 1. Memory Manager (Long-Term Memory)

**Location:** `internal/memory/manager.go`

The MemoryManager uses the native embedder to generate embeddings for long-term memory storage. When agents store information for later retrieval, the embedder converts text into vectors for semantic similarity search.

```go
// Memory manager initialization (simplified)
emb, err := embedder.CreateNativeEmbedder()
longTermMem := NewLongTermMemory(vectorStore, emb)
```

**Use Cases:**
- Agent memory persistence across sessions
- Semantic search over historical observations
- Context retrieval for agent reasoning

### 2. GraphRAG Store

**Location:** `internal/graphrag/store.go`

The GraphRAG store uses the embedder for semantic search over the knowledge graph. When storing nodes (findings, attack patterns, techniques), the embedder generates embeddings that enable vector-based similarity queries.

```go
// GraphRAG store operations that use embeddings
store.Store(ctx, record)           // Auto-generates embedding from EmbedContent
store.StoreBatch(ctx, records)     // Batch embedding generation
store.FindSimilarAttacks(ctx, content, topK)
store.FindSimilarFindings(ctx, findingID, topK)
```

**Use Cases:**
- Finding similar attack patterns (MITRE ATT&CK)
- Security finding correlation and deduplication
- Attack chain discovery
- Semantic search over knowledge graph nodes

### 3. Daemon Infrastructure

**Location:** `internal/daemon/infrastructure.go`

The daemon initializes the embedder during startup and injects it into both the memory subsystem and GraphRAG bridges.

```go
// Daemon creates embedder and injects into components
emb, err := embedder.CreateEmbedder(d.config.Embedder)
vectorStore := vector.NewEmbeddedVectorStore(emb.Dimensions())
store, err := graphrag.NewGraphRAGStoreWithProvider(config, emb, prov)
```

## Configuration

The native embedder is the default provider. Configuration is defined in `internal/memory/embedder/embedder.go`:

```yaml
# ~/.gibson/config.yaml
embedder:
  provider: native    # "native" or "openai"
  # No additional config needed for native embedder
```

Or programmatically:

```go
config := embedder.DefaultEmbedderConfig()
// Returns:
// EmbedderConfig{
//   Provider:   "native",
//   Model:      "",        // Not needed for native
//   APIKey:     "",        // Not needed for native
//   MaxRetries: 3,
//   Timeout:    30,
// }
```

## Requirements

### ONNX Runtime

The native embedder requires ONNX Runtime to be installed on the system.

**Linux (Debian/Ubuntu):**
```bash
# Download and install ONNX Runtime
wget https://github.com/microsoft/onnxruntime/releases/download/v1.16.3/onnxruntime-linux-x64-1.16.3.tgz
tar -xzf onnxruntime-linux-x64-1.16.3.tgz
sudo cp onnxruntime-linux-x64-1.16.3/lib/* /usr/local/lib/
sudo ldconfig
```

**macOS:**
```bash
brew install onnxruntime
```

**Windows:**
Download the ONNX Runtime release and add the DLL directory to your PATH.

### Model Weights

The all-MiniLM-L6-v2 model weights are embedded in the binary via the `all-minilm-l6-v2-go` library. No external model files are required.

## API Reference

### Embedder Interface

All embedders implement this interface (`internal/memory/embedder/embedder.go`):

```go
type Embedder interface {
    // Embed generates an embedding vector for a single text.
    Embed(ctx context.Context, text string) ([]float64, error)

    // EmbedBatch generates embeddings for multiple texts efficiently.
    EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

    // Dimensions returns the dimensionality of embedding vectors.
    Dimensions() int

    // Model returns the name of the embedding model being used.
    Model() string

    // Health returns the health status of the embedder.
    Health(ctx context.Context) types.HealthStatus
}
```

### Creating a Native Embedder

```go
import "github.com/zero-day-ai/gibson/internal/memory/embedder"

// Direct creation
emb, err := embedder.CreateNativeEmbedder()
if err != nil {
    // ONNX Runtime may be unavailable
    return err
}

// Via factory (recommended)
config := embedder.EmbedderConfig{Provider: "native"}
emb, err := embedder.CreateEmbedder(config)
```

### Generating Embeddings

```go
// Single text
embedding, err := emb.Embed(ctx, "SQL injection vulnerability found in login form")
// embedding: []float64 with 384 dimensions

// Batch processing (more efficient for multiple texts)
texts := []string{
    "SQL injection in login",
    "XSS in search field",
    "CSRF in payment form",
}
embeddings, err := emb.EmbedBatch(ctx, texts)
// embeddings: [][]float64, each with 384 dimensions
```

### Health Checks

```go
health := emb.Health(ctx)
if health.IsUnhealthy() {
    log.Error("embedder unhealthy", "message", health.Message)
}
```

## Error Handling

The embedder uses typed error codes for programmatic handling:

| Error Code | Description |
|------------|-------------|
| `EMBEDDER_UNAVAILABLE` | ONNX Runtime not found or model initialization failed |
| `EMBEDDING_FAILED` | Single embedding generation failed |
| `EMBEDDING_BATCH_FAILED` | Batch embedding generation failed |
| `INVALID_EMBEDDER_CONFIG` | Invalid configuration provided |

```go
import "github.com/zero-day-ai/gibson/internal/types"

_, err := emb.Embed(ctx, text)
if err != nil {
    if gerr, ok := types.AsGibsonError(err); ok {
        switch gerr.Code {
        case embedder.ErrCodeEmbedderUnavailable:
            // Handle missing runtime
        case embedder.ErrCodeEmbeddingFailed:
            // Handle generation failure
        }
    }
}
```

## Performance Considerations

### Batch Processing

For multiple texts, always use `EmbedBatch()` instead of multiple `Embed()` calls. While the current implementation processes texts sequentially, it keeps the model loaded in memory, avoiding repeated initialization overhead.

### Context Cancellation

The embedder checks for context cancellation between operations but cannot interrupt an in-progress embedding computation (ONNX Runtime limitation). For very long texts, consider setting appropriate timeouts.

### Memory Usage

The model stays loaded in memory after first use (~100MB). This is intentional for performance - cold start would add ~500ms latency per embedding.

## Comparison with OpenAI Embedder

| Feature | Native | OpenAI |
|---------|--------|--------|
| Dimensions | 384 | 1536 (small) / 3072 (large) |
| Latency | ~100ms | ~200-500ms (network dependent) |
| Cost | Free | $0.02-0.13 per 1M tokens |
| Offline | Yes | No |
| API Key Required | No | Yes |
| Quality | Good (general purpose) | Better (specialized training) |

For most Gibson use cases (security findings, attack patterns), the native embedder provides sufficient quality while enabling offline operation.

## Troubleshooting

### "ONNX Runtime may be unavailable"

Ensure ONNX Runtime is installed and in the library path:
```bash
# Check if library is findable
ldconfig -p | grep onnxruntime

# If not found, add to LD_LIBRARY_PATH
export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH
```

### Slow First Embedding

The first embedding call loads the model (~500ms). Subsequent calls are ~100ms. This is normal behavior.

### High Memory Usage

The model uses ~100MB RAM. This is expected and required for CPU inference.
