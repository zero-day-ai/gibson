# ONNX Runtime & Native Embedder Setup

The native embedder provides offline, CPU-based embedding generation for Gibson using the all-MiniLM-L6-v2 model. It runs entirely locally without requiring external API calls, making it ideal for air-gapped environments and reducing operational costs.

## Quick Reference

| Property | Value |
|----------|-------|
| Model | all-MiniLM-L6-v2 (sentence-transformers) |
| Dimensions | 384 |
| Output Type | float64 vectors |
| Runtime | ONNX Runtime 1.23.0 |
| Performance | ~100ms per embedding |
| Thread Safety | Yes (all methods are concurrent-safe) |

## Installation

### Required ONNX Runtime Version

**IMPORTANT:** You MUST use ONNX Runtime **1.23.0**. Other versions have compatibility issues:
- Version 1.23.1+ has protobuf parsing incompatibilities with the embedded model
- Older versions may lack required features

### Linux (Debian/Ubuntu/Kali)

```bash
# Set version (MUST be 1.23.0)
ORT_VERSION=1.23.0

# Detect architecture
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ONNX_ARCH="linux-x64"
elif [ "$ARCH" = "aarch64" ]; then
    ONNX_ARCH="linux-aarch64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

# Download and install
cd /tmp
wget "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-${ONNX_ARCH}-${ORT_VERSION}.tgz"
tar -xzf onnxruntime-${ONNX_ARCH}-${ORT_VERSION}.tgz
sudo cp onnxruntime-${ONNX_ARCH}-${ORT_VERSION}/lib/libonnxruntime.so* /usr/local/lib/
sudo ldconfig

# Verify installation
ls -la /usr/local/lib/libonnxruntime.so*
```

### Set Environment Variable

Add to your `~/.bashrc` or `~/.zshrc`:

```bash
export ONNXRUNTIME_LIB_PATH=/usr/local/lib/libonnxruntime.so
```

### macOS

```bash
# Using Homebrew (check version compatibility)
brew install onnxruntime

# Or manual install for specific version
ORT_VERSION=1.23.0
curl -fSL "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-osx-universal2-${ORT_VERSION}.tgz" -o onnxruntime.tgz
tar -xzf onnxruntime.tgz
sudo cp onnxruntime-osx-universal2-${ORT_VERSION}/lib/libonnxruntime.* /usr/local/lib/

export ONNXRUNTIME_LIB_PATH=/usr/local/lib/libonnxruntime.dylib
```

### Windows

1. Download ONNX Runtime 1.23.0 from [GitHub Releases](https://github.com/microsoft/onnxruntime/releases/tag/v1.23.0)
2. Extract and add the DLL directory to your PATH
3. Set environment variable: `ONNXRUNTIME_LIB_PATH=C:\path\to\onnxruntime.dll`

## Model File Setup

The all-MiniLM-L6-v2 model weights are stored in the `all-minilm-l6-v2-go` Go module. **However**, the Go module proxy does not handle Git LFS files, so the model file is often just a pointer (133 bytes) instead of the actual model (~90MB).

### Check If Model Is Installed Correctly

```bash
# Find the model file
MODEL_PATH=$(find $(go env GOPATH)/pkg/mod -name "model.onnx" -path "*all-minilm*" 2>/dev/null | head -1)
ls -la "$MODEL_PATH"

# If it's ~133 bytes, it's a Git LFS pointer (broken)
# If it's ~90MB, it's the actual model (correct)
```

### Fix: Download the Actual Model

If the model file is only 133 bytes (Git LFS pointer):

```bash
# Download the actual model from HuggingFace
curl -fSL "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx" -o /tmp/model.onnx

# Find and replace the LFS pointer
MODEL_PATH=$(find $(go env GOPATH)/pkg/mod -name "model.onnx" -path "*all-minilm*" 2>/dev/null | head -1)
MODEL_DIR=$(dirname "$MODEL_PATH")
sudo chmod 644 "$MODEL_PATH"
sudo cp /tmp/model.onnx "$MODEL_PATH"

# Verify
ls -la "$MODEL_PATH"  # Should be ~90MB now
```

### Rebuild Gibson After Fixing Model

```bash
cd /path/to/gibson
make build
```

## Configuration

The native embedder is the **default** provider. No additional configuration is required.

```yaml
# ~/.gibson/config.yaml (optional - these are the defaults)
embedder:
  provider: native    # "native" or "openai"
  # No additional config needed for native embedder
```

## Verification

After installation, verify everything works:

```bash
# Start the daemon
gibson daemon start

# Check logs for successful embedder initialization
gibson daemon logs 2>&1 | grep -i embedder
# Should show: "created embedder for GraphRAG provider=native dimensions=384 model=all-MiniLM-L6-v2"

# Check status
gibson status
```

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
│  │                 │    │ (libonnxruntime)│                    │
│  └─────────────────┘    └─────────────────┘                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Troubleshooting

### Error: "cannot open shared object file: No such file or directory"

ONNX Runtime library not found. Check installation:

```bash
# Verify library exists
ls -la /usr/local/lib/libonnxruntime.so*

# Check if it's in ldconfig cache
ldconfig -p | grep onnxruntime

# If not found, run ldconfig
sudo ldconfig

# If still not working, set LD_LIBRARY_PATH
export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH
```

### Error: "Failed to load model because protobuf parsing failed"

This indicates one of two issues:

1. **Wrong ONNX Runtime version**: You must use exactly version 1.23.0
   ```bash
   # Check what version is installed
   ls -la /usr/local/lib/libonnxruntime.so*

   # Remove wrong version and install 1.23.0
   sudo rm /usr/local/lib/libonnxruntime.so*
   # Follow installation steps above
   ```

2. **Git LFS pointer instead of actual model**: See "Model File Setup" section above

### Error: "ONNX Runtime may be unavailable"

The library was found but couldn't initialize. Check:

```bash
# Verify the library is the right architecture
file /usr/local/lib/libonnxruntime.so

# Verify ONNXRUNTIME_LIB_PATH is set correctly
echo $ONNXRUNTIME_LIB_PATH
```

### Slow First Embedding

The first embedding call loads the model (~500ms). Subsequent calls are ~100ms. This is normal behavior.

### High Memory Usage

The model uses ~100MB RAM. This is expected and required for CPU inference.

## Docker Setup

The Gibson Dockerfiles handle ONNX Runtime setup automatically. See `build/Dockerfile.oss` and `build/Dockerfile.enterprise`.

Key points:
- ONNX Runtime 1.23.0 is installed to `/usr/local/lib/`
- `ONNXRUNTIME_LIB_PATH` environment variable is set
- The model file is downloaded from HuggingFace during build

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

## Error Codes

| Error Code | Description |
|------------|-------------|
| `EMBEDDER_UNAVAILABLE` | ONNX Runtime not found or model initialization failed |
| `EMBEDDING_FAILED` | Single embedding generation failed |
| `EMBEDDING_BATCH_FAILED` | Batch embedding generation failed |
| `INVALID_EMBEDDER_CONFIG` | Invalid configuration provided |

## Comparison: Native vs OpenAI Embedder

| Feature | Native | OpenAI |
|---------|--------|--------|
| Dimensions | 384 | 1536 (small) / 3072 (large) |
| Latency | ~100ms | ~200-500ms (network dependent) |
| Cost | Free | $0.02-0.13 per 1M tokens |
| Offline | Yes | No |
| API Key Required | No | Yes |
| Quality | Good (general purpose) | Better (specialized training) |

For most Gibson use cases (security findings, attack patterns), the native embedder provides sufficient quality while enabling offline operation.
