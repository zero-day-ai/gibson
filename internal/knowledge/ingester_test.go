package knowledge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockChunkStorer is a mock implementation of ChunkStorer for testing
type MockChunkStorer struct {
	chunks     []KnowledgeChunk
	sources    map[string]KnowledgeSource
	sourceHash map[string]*KnowledgeSource
}

func NewMockChunkStorer() *MockChunkStorer {
	return &MockChunkStorer{
		chunks:     make([]KnowledgeChunk, 0),
		sources:    make(map[string]KnowledgeSource),
		sourceHash: make(map[string]*KnowledgeSource),
	}
}

func (m *MockChunkStorer) StoreChunk(ctx context.Context, chunk KnowledgeChunk) error {
	m.chunks = append(m.chunks, chunk)
	return nil
}

func (m *MockChunkStorer) StoreBatch(ctx context.Context, chunks []KnowledgeChunk) error {
	m.chunks = append(m.chunks, chunks...)
	return nil
}

func (m *MockChunkStorer) GetSourceByHash(ctx context.Context, hash string) (*KnowledgeSource, error) {
	if source, ok := m.sourceHash[hash]; ok {
		return source, nil
	}
	return nil, fmt.Errorf("source not found")
}

func (m *MockChunkStorer) DeleteBySource(ctx context.Context, source string) error {
	// Remove chunks with matching source
	filtered := make([]KnowledgeChunk, 0)
	for _, chunk := range m.chunks {
		if chunk.Source != source {
			filtered = append(filtered, chunk)
		}
	}
	m.chunks = filtered

	// Remove source
	delete(m.sources, source)
	return nil
}

func (m *MockChunkStorer) StoreSource(ctx context.Context, source KnowledgeSource) error {
	m.sources[source.Source] = source
	m.sourceHash[source.SourceHash] = &source
	return nil
}

func TestIngestText(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	testText := `# Introduction

This is a test document for knowledge ingestion.
It contains multiple paragraphs that should be chunked appropriately.

## Code Example

Here is some code:

` + "```go" + `
package main

func main() {
    fmt.Println("Hello, World!")
}
` + "```" + `

## Conclusion

This document tests text chunking capabilities.`

	result, err := ingester.IngestText(ctx, testText, "test-doc.txt", opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "test-doc.txt", result.Source)
	assert.Equal(t, "text", result.SourceType)
	assert.Greater(t, result.ChunksAdded, 0)
	assert.Equal(t, result.TotalChunks, result.ChunksAdded)
	assert.Len(t, result.Errors, 0)

	// Verify chunks were stored
	assert.Greater(t, len(storer.chunks), 0)

	// Verify at least one code block was detected
	hasCodeChunk := false
	for _, chunk := range storer.chunks {
		if chunk.Metadata.HasCode {
			hasCodeChunk = true
			assert.Equal(t, "go", chunk.Metadata.Language)
		}
	}
	assert.True(t, hasCodeChunk, "Should have at least one code chunk")
}

func TestIngestTextDeduplication(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	testText := "This is a test document."

	// First ingestion
	result1, err := ingester.IngestText(ctx, testText, "test-doc.txt", opts)
	require.NoError(t, err)
	assert.Greater(t, result1.ChunksAdded, 0)

	// Second ingestion with same content (should skip)
	result2, err := ingester.IngestText(ctx, testText, "test-doc.txt", opts)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.ChunksAdded)
	assert.Equal(t, result1.ChunksAdded, result2.ChunksSkipped)
}

func TestIngestTextForceOverwrite(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	testText := "This is a test document."

	// First ingestion
	opts1 := IngestOptions{Force: false, ChunkSize: 512, ChunkOverlap: 50}
	_, err := ingester.IngestText(ctx, testText, "test-doc.txt", opts1)
	require.NoError(t, err)
	initialCount := len(storer.chunks)
	assert.Greater(t, initialCount, 0)

	// Second ingestion with force
	opts2 := IngestOptions{Force: true, ChunkSize: 512, ChunkOverlap: 50}
	result2, err := ingester.IngestText(ctx, testText, "test-doc.txt", opts2)
	require.NoError(t, err)
	assert.Greater(t, result2.ChunksAdded, 0)

	// Should still have same number of chunks (old ones deleted, new ones added)
	assert.Equal(t, initialCount, len(storer.chunks))
}

func TestIngestURL(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
</head>
<body>
    <h1>Welcome</h1>
    <p>This is a test page for URL ingestion.</p>
    <pre><code class="language-python">
def hello():
    print("Hello, World!")
    </code></pre>
    <p>More content here.</p>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	result, err := ingester.IngestURL(ctx, server.URL, opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, server.URL, result.Source)
	assert.Equal(t, "url", result.SourceType)
	assert.Greater(t, result.ChunksAdded, 0)
	assert.Len(t, result.Errors, 0)

	// Verify chunks were stored
	assert.Greater(t, len(storer.chunks), 0)

	// Verify title was extracted
	hasTitle := false
	for _, chunk := range storer.chunks {
		if chunk.Metadata.Title == "Test Page" {
			hasTitle = true
			break
		}
	}
	assert.True(t, hasTitle, "Should have extracted page title")
}

func TestIngestURLError(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	// Test with invalid URL
	_, err := ingester.IngestURL(ctx, "http://this-domain-does-not-exist-12345.com", opts)
	assert.Error(t, err)
}

func TestIngestPDF(t *testing.T) {
	// Create a temporary test PDF
	tmpDir := t.TempDir()
	_ = filepath.Join(tmpDir, "test.pdf") // pdfPath would be used for creating a valid PDF

	// Create a simple PDF for testing
	// Note: In a real test, you'd use pdfcpu to create a proper PDF
	// For this test, we'll skip if pdfcpu fails (since we need a valid PDF)

	// Skip this test for now as creating a valid PDF requires more setup
	t.Skip("PDF test requires a valid PDF file - implement with pdfcpu generation")
}

func TestIngestPDFNotFound(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	// Test with non-existent file
	_, err := ingester.IngestPDF(ctx, "/nonexistent/path/test.pdf", opts)
	assert.Error(t, err)
}

func TestIngestDirectory(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	// Create temporary directory with test files
	tmpDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"doc1.txt": "This is the first document.",
		"doc2.txt": "This is the second document.",
		"doc3.md":  "# Markdown Document\n\nThis is a markdown file.",
		"skip.log": "This should be skipped.",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	// Ingest directory with pattern
	result, err := ingester.IngestDirectory(ctx, tmpDir, "*.txt", opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, tmpDir, result.Source)
	assert.Equal(t, "directory", result.SourceType)
	assert.Greater(t, result.ChunksAdded, 0)

	// Verify only .txt files were ingested
	txtSources := 0
	for _, chunk := range storer.chunks {
		if filepath.Ext(chunk.Source) == ".txt" {
			txtSources++
		}
	}
	assert.Greater(t, txtSources, 0)
}

func TestIngestDirectoryNotFound(t *testing.T) {
	ctx := context.Background()
	processor := NewChunkProcessor()
	storer := NewMockChunkStorer()
	ingester := NewIngester(processor, storer)

	opts := IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}

	// Test with non-existent directory
	_, err := ingester.IngestDirectory(ctx, "/nonexistent/directory", "*", opts)
	assert.Error(t, err)
}

func TestChunkProcessor(t *testing.T) {
	processor := NewChunkProcessor()

	t.Run("ChunkText", func(t *testing.T) {
		text := `# Introduction

This is a test document with multiple paragraphs.
Each paragraph should be chunked appropriately.

## Section 1

More content here.

` + "```python" + `
def test():
    pass
` + "```" + `

## Section 2

Final content.`

		opts := ChunkOptions{
			ChunkSize:    100,
			ChunkOverlap: 20,
		}

		chunks, err := processor.ChunkText(text, opts)
		require.NoError(t, err)
		assert.Greater(t, len(chunks), 0)

		// Verify code chunk exists
		hasCode := false
		for _, chunk := range chunks {
			if chunk.Metadata.HasCode {
				hasCode = true
				assert.Equal(t, "python", chunk.Metadata.Language)
			}
		}
		assert.True(t, hasCode)
	})

	t.Run("ChunkHTML", func(t *testing.T) {
		html := `<!DOCTYPE html>
<html>
<head><title>Test HTML</title></head>
<body>
    <h1>Header</h1>
    <p>Paragraph 1</p>
    <pre><code class="language-go">package main</code></pre>
    <p>Paragraph 2</p>
</body>
</html>`

		opts := ChunkOptions{
			ChunkSize:    100,
			ChunkOverlap: 20,
		}

		chunks, err := processor.ChunkHTML(html, opts)
		require.NoError(t, err)
		assert.Greater(t, len(chunks), 0)

		// Verify title was extracted
		hasTitle := false
		for _, chunk := range chunks {
			if chunk.Metadata.Title == "Test HTML" {
				hasTitle = true
			}
		}
		assert.True(t, hasTitle)
	})

	t.Run("EmptyText", func(t *testing.T) {
		opts := ChunkOptions{
			ChunkSize:    512,
			ChunkOverlap: 50,
		}

		chunks, err := processor.ChunkText("", opts)
		require.NoError(t, err)
		assert.Len(t, chunks, 0)
	})
}

func TestHashFunctions(t *testing.T) {
	t.Run("hashString", func(t *testing.T) {
		hash1 := hashString("test content")
		hash2 := hashString("test content")
		hash3 := hashString("different content")

		assert.Equal(t, hash1, hash2, "Same content should produce same hash")
		assert.NotEqual(t, hash1, hash3, "Different content should produce different hash")
		assert.Len(t, hash1, 64, "SHA-256 hash should be 64 hex characters")
	})

	t.Run("hashFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test files
		file1 := filepath.Join(tmpDir, "file1.txt")
		file2 := filepath.Join(tmpDir, "file2.txt")

		err := os.WriteFile(file1, []byte("test content"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(file2, []byte("test content"), 0644)
		require.NoError(t, err)

		hash1, err := hashFile(file1)
		require.NoError(t, err)

		hash2, err := hashFile(file2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "Files with same content should have same hash")
		assert.Len(t, hash1, 64, "SHA-256 hash should be 64 hex characters")
	})

	t.Run("hashFile nonexistent", func(t *testing.T) {
		_, err := hashFile("/nonexistent/file.txt")
		assert.Error(t, err)
	})
}
