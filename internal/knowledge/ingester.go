package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
)

// KnowledgeIngester handles ingesting content from various sources into the knowledge store.
// It coordinates chunking, embedding generation, and storage.
type KnowledgeIngester interface {
	// IngestPDF ingests a PDF file and stores its chunks
	IngestPDF(ctx context.Context, path string, opts IngestOptions) (*IngestResult, error)

	// IngestURL ingests content from a URL
	IngestURL(ctx context.Context, url string, opts IngestOptions) (*IngestResult, error)

	// IngestText ingests plain text content
	IngestText(ctx context.Context, text string, source string, opts IngestOptions) (*IngestResult, error)

	// IngestDirectory recursively ingests files from a directory
	IngestDirectory(ctx context.Context, path string, pattern string, opts IngestOptions) (*IngestResult, error)
}

// DefaultIngester implements KnowledgeIngester with a ChunkProcessor.
type DefaultIngester struct {
	processor ChunkProcessor
	storer    ChunkStorer // Interface for storing chunks (to be implemented in manager)
}

// ChunkStorer is an interface for storing knowledge chunks.
// This will be implemented by KnowledgeManager in a later phase.
type ChunkStorer interface {
	// StoreChunk stores a single knowledge chunk
	StoreChunk(ctx context.Context, chunk KnowledgeChunk) error

	// StoreBatch stores multiple chunks efficiently
	StoreBatch(ctx context.Context, chunks []KnowledgeChunk) error

	// GetSourceByHash checks if a source has been ingested
	GetSourceByHash(ctx context.Context, hash string) (*KnowledgeSource, error)

	// DeleteBySource removes all chunks from a source
	DeleteBySource(ctx context.Context, source string) error

	// StoreSource records a source as ingested
	StoreSource(ctx context.Context, source KnowledgeSource) error
}

// NewIngester creates a new DefaultIngester.
func NewIngester(processor ChunkProcessor, storer ChunkStorer) KnowledgeIngester {
	return &DefaultIngester{
		processor: processor,
		storer:    storer,
	}
}

// IngestPDF ingests a PDF file, extracting text by page and chunking it.
func (ing *DefaultIngester) IngestPDF(ctx context.Context, path string, opts IngestOptions) (*IngestResult, error) {
	start := time.Now()
	result := &IngestResult{
		Source:     path,
		SourceType: "pdf",
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("file not found: %s", path))
		return result, fmt.Errorf("PDF file not found: %s", path)
	}

	// Generate source hash for deduplication
	sourceHash, err := hashFile(path)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to hash file: %v", err))
		return result, fmt.Errorf("failed to hash file: %w", err)
	}

	// Check if already ingested
	if !opts.Force {
		existing, err := ing.storer.GetSourceByHash(ctx, sourceHash)
		if err == nil && existing != nil {
			result.ChunksSkipped = existing.ChunkCount
			result.TotalChunks = existing.ChunkCount
			result.DurationMs = time.Since(start).Milliseconds()
			return result, nil
		}
	}

	// Delete existing chunks if forcing overwrite
	if opts.Force {
		if err := ing.storer.DeleteBySource(ctx, path); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to delete existing chunks: %v", err))
		}
	}

	// Chunk the PDF
	chunkOpts := ChunkOptions{
		ChunkSize:    opts.ChunkSize,
		ChunkOverlap: opts.ChunkOverlap,
	}
	textChunks, err := ing.processor.ChunkPDF(path, chunkOpts)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to chunk PDF: %v", err))
		return result, fmt.Errorf("failed to chunk PDF: %w", err)
	}

	result.TotalChunks = len(textChunks)

	// Convert to KnowledgeChunks (without embeddings - those will be added by manager)
	var knowledgeChunks []KnowledgeChunk
	for _, tc := range textChunks {
		chunk := KnowledgeChunk{
			ID:         uuid.New().String(),
			Source:     path,
			SourceHash: sourceHash,
			Text:       tc.Text,
			Metadata:   tc.Metadata,
			CreatedAt:  time.Now(),
		}
		knowledgeChunks = append(knowledgeChunks, chunk)
	}

	// Store chunks
	if err := ing.storer.StoreBatch(ctx, knowledgeChunks); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to store chunks: %v", err))
		return result, fmt.Errorf("failed to store chunks: %w", err)
	}

	result.ChunksAdded = len(knowledgeChunks)

	// Record source
	source := KnowledgeSource{
		Source:     path,
		SourceType: "pdf",
		SourceHash: sourceHash,
		ChunkCount: len(knowledgeChunks),
		IngestedAt: time.Now(),
	}
	if err := ing.storer.StoreSource(ctx, source); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to record source: %v", err))
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// IngestURL ingests content from a URL, extracting text from HTML.
func (ing *DefaultIngester) IngestURL(ctx context.Context, url string, opts IngestOptions) (*IngestResult, error) {
	start := time.Now()
	result := &IngestResult{
		Source:     url,
		SourceType: "url",
	}

	// Fetch URL content with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create request: %v", err))
		return result, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to fetch URL: %v", err))
		return result, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Errors = append(result.Errors, fmt.Sprintf("HTTP error: %d", resp.StatusCode))
		return result, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to read response: %v", err))
		return result, fmt.Errorf("failed to read response: %w", err)
	}

	htmlContent := string(body)

	// Generate source hash for deduplication
	sourceHash := hashString(htmlContent)

	// Check if already ingested
	if !opts.Force {
		existing, err := ing.storer.GetSourceByHash(ctx, sourceHash)
		if err == nil && existing != nil {
			result.ChunksSkipped = existing.ChunkCount
			result.TotalChunks = existing.ChunkCount
			result.DurationMs = time.Since(start).Milliseconds()
			return result, nil
		}
	}

	// Delete existing chunks if forcing overwrite
	if opts.Force {
		if err := ing.storer.DeleteBySource(ctx, url); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to delete existing chunks: %v", err))
		}
	}

	// Extract page title using goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse HTML: %v", err))
		return result, fmt.Errorf("failed to parse HTML: %w", err)
	}

	title := doc.Find("title").First().Text()
	title = strings.TrimSpace(title)

	// Chunk the HTML
	chunkOpts := ChunkOptions{
		ChunkSize:    opts.ChunkSize,
		ChunkOverlap: opts.ChunkOverlap,
	}
	textChunks, err := ing.processor.ChunkHTML(htmlContent, chunkOpts)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to chunk HTML: %v", err))
		return result, fmt.Errorf("failed to chunk HTML: %w", err)
	}

	result.TotalChunks = len(textChunks)

	// Convert to KnowledgeChunks
	var knowledgeChunks []KnowledgeChunk
	for _, tc := range textChunks {
		// Add title to metadata if not already set
		if tc.Metadata.Title == "" {
			tc.Metadata.Title = title
		}

		chunk := KnowledgeChunk{
			ID:         uuid.New().String(),
			Source:     url,
			SourceHash: sourceHash,
			Text:       tc.Text,
			Metadata:   tc.Metadata,
			CreatedAt:  time.Now(),
		}
		knowledgeChunks = append(knowledgeChunks, chunk)
	}

	// Store chunks
	if err := ing.storer.StoreBatch(ctx, knowledgeChunks); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to store chunks: %v", err))
		return result, fmt.Errorf("failed to store chunks: %w", err)
	}

	result.ChunksAdded = len(knowledgeChunks)

	// Record source
	source := KnowledgeSource{
		Source:     url,
		SourceType: "url",
		SourceHash: sourceHash,
		ChunkCount: len(knowledgeChunks),
		IngestedAt: time.Now(),
		Metadata: map[string]any{
			"title": title,
		},
	}
	if err := ing.storer.StoreSource(ctx, source); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to record source: %v", err))
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// IngestText ingests plain text content directly.
func (ing *DefaultIngester) IngestText(ctx context.Context, text string, source string, opts IngestOptions) (*IngestResult, error) {
	start := time.Now()
	result := &IngestResult{
		Source:     source,
		SourceType: "text",
	}

	// Generate source hash for deduplication
	sourceHash := hashString(text)

	// Check if already ingested
	if !opts.Force {
		existing, err := ing.storer.GetSourceByHash(ctx, sourceHash)
		if err == nil && existing != nil {
			result.ChunksSkipped = existing.ChunkCount
			result.TotalChunks = existing.ChunkCount
			result.DurationMs = time.Since(start).Milliseconds()
			return result, nil
		}
	}

	// Delete existing chunks if forcing overwrite
	if opts.Force {
		if err := ing.storer.DeleteBySource(ctx, source); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to delete existing chunks: %v", err))
		}
	}

	// Chunk the text
	chunkOpts := ChunkOptions{
		ChunkSize:    opts.ChunkSize,
		ChunkOverlap: opts.ChunkOverlap,
	}
	textChunks, err := ing.processor.ChunkText(text, chunkOpts)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to chunk text: %v", err))
		return result, fmt.Errorf("failed to chunk text: %w", err)
	}

	result.TotalChunks = len(textChunks)

	// Convert to KnowledgeChunks
	var knowledgeChunks []KnowledgeChunk
	for _, tc := range textChunks {
		chunk := KnowledgeChunk{
			ID:         uuid.New().String(),
			Source:     source,
			SourceHash: sourceHash,
			Text:       tc.Text,
			Metadata:   tc.Metadata,
			CreatedAt:  time.Now(),
		}
		knowledgeChunks = append(knowledgeChunks, chunk)
	}

	// Store chunks
	if err := ing.storer.StoreBatch(ctx, knowledgeChunks); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to store chunks: %v", err))
		return result, fmt.Errorf("failed to store chunks: %w", err)
	}

	result.ChunksAdded = len(knowledgeChunks)

	// Record source
	src := KnowledgeSource{
		Source:     source,
		SourceType: "text",
		SourceHash: sourceHash,
		ChunkCount: len(knowledgeChunks),
		IngestedAt: time.Now(),
	}
	if err := ing.storer.StoreSource(ctx, src); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to record source: %v", err))
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// IngestDirectory recursively ingests files from a directory matching a pattern.
func (ing *DefaultIngester) IngestDirectory(ctx context.Context, dirPath string, pattern string, opts IngestOptions) (*IngestResult, error) {
	start := time.Now()
	result := &IngestResult{
		Source:     dirPath,
		SourceType: "directory",
	}

	// Check if directory exists
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("directory not found: %s", dirPath))
		return result, fmt.Errorf("directory not found: %s", dirPath)
	}
	if !info.IsDir() {
		result.Errors = append(result.Errors, fmt.Sprintf("not a directory: %s", dirPath))
		return result, fmt.Errorf("not a directory: %s", dirPath)
	}

	// Default pattern matches all files
	if pattern == "" {
		pattern = "*"
	}

	// Collect matching files
	var matchingFiles []string
	err = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("error walking %s: %v", path, err))
			return nil // Continue walking
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Check if file matches pattern
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid pattern %s: %v", pattern, err))
			return nil
		}

		if matched {
			matchingFiles = append(matchingFiles, path)
		}

		return nil
	})

	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to walk directory: %v", err))
		return result, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Ingest each file
	for _, filePath := range matchingFiles {
		// Check context cancellation
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, "operation cancelled")
			break
		}

		// Determine file type by extension
		ext := strings.ToLower(filepath.Ext(filePath))

		var fileResult *IngestResult
		var err error

		switch ext {
		case ".pdf":
			fileResult, err = ing.IngestPDF(ctx, filePath, opts)
		case ".txt", ".md", ".markdown":
			content, readErr := os.ReadFile(filePath)
			if readErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to read %s: %v", filePath, readErr))
				continue
			}
			fileResult, err = ing.IngestText(ctx, string(content), filePath, opts)
		case ".html", ".htm":
			content, readErr := os.ReadFile(filePath)
			if readErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to read %s: %v", filePath, readErr))
				continue
			}
			// Use ChunkHTML through IngestText by treating as HTML
			// For now, we'll ingest as text - HTML handling can be improved later
			fileResult, err = ing.IngestText(ctx, string(content), filePath, opts)
		default:
			// Skip unsupported file types
			result.Errors = append(result.Errors, fmt.Sprintf("unsupported file type: %s", filePath))
			continue
		}

		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to ingest %s: %v", filePath, err))
			continue
		}

		// Aggregate results
		result.ChunksAdded += fileResult.ChunksAdded
		result.ChunksSkipped += fileResult.ChunksSkipped
		result.TotalChunks += fileResult.TotalChunks
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// Helper functions

// hashFile generates a SHA-256 hash of a file's contents.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// hashString generates a SHA-256 hash of a string.
func hashString(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}
