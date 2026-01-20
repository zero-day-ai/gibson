package knowledge

import (
	"time"
)

// KnowledgeChunk represents a single chunk of knowledge stored in the vector store.
// It extends the basic VectorRecord with source tracking and richer metadata.
type KnowledgeChunk struct {
	// ID is a unique identifier for this chunk (typically UUID or hash-based)
	ID string `json:"id"`

	// Source is the origin of this chunk (file path, URL, or identifier)
	Source string `json:"source"`

	// SourceHash is a hash of the source content for deduplication
	SourceHash string `json:"source_hash"`

	// Text is the actual text content of this chunk
	Text string `json:"text"`

	// Embedding is the vector representation of the text (typically 384 dimensions)
	// This will be populated automatically during storage
	Embedding []float64 `json:"embedding,omitempty"`

	// Metadata contains additional structured information about the chunk
	Metadata ChunkMetadata `json:"metadata"`

	// CreatedAt is when this chunk was stored
	CreatedAt time.Time `json:"created_at"`
}

// ChunkMetadata contains structured metadata about a knowledge chunk.
type ChunkMetadata struct {
	// Section is the header or section name containing this chunk
	Section string `json:"section,omitempty"`

	// PageNumber is the page in the original document (for PDFs)
	PageNumber int `json:"page_number,omitempty"`

	// StartChar is the starting character position in the original document
	StartChar int `json:"start_char,omitempty"`

	// HasCode indicates if this chunk contains code blocks
	HasCode bool `json:"has_code,omitempty"`

	// Language is the programming language (if HasCode is true)
	Language string `json:"language,omitempty"`

	// Title is the document title (for web pages)
	Title string `json:"title,omitempty"`

	// Additional custom metadata fields
	Custom map[string]any `json:"custom,omitempty"`
}

// KnowledgeSource represents a source document that has been ingested into the knowledge store.
// This tracks what content has been added for management and deduplication.
type KnowledgeSource struct {
	// Source is the identifier (file path, URL, etc.)
	Source string `json:"source"`

	// SourceType indicates the type of source (pdf, url, text, etc.)
	SourceType string `json:"source_type"`

	// SourceHash is a hash of the source content for deduplication
	SourceHash string `json:"source_hash"`

	// ChunkCount is the number of chunks created from this source
	ChunkCount int `json:"chunk_count"`

	// IngestedAt is when this source was first added
	IngestedAt time.Time `json:"ingested_at"`

	// Metadata contains additional information about the source
	Metadata map[string]any `json:"metadata,omitempty"`
}

// KnowledgeResult represents a single search result with relevance score.
type KnowledgeResult struct {
	// Chunk is the knowledge chunk that matched the search
	Chunk KnowledgeChunk `json:"chunk"`

	// Score is the similarity score (0-1, higher is better)
	Score float64 `json:"score"`
}

// KnowledgeStats provides aggregate statistics about the knowledge store.
type KnowledgeStats struct {
	// TotalSources is the number of unique sources ingested
	TotalSources int `json:"total_sources"`

	// TotalChunks is the total number of knowledge chunks stored
	TotalChunks int `json:"total_chunks"`

	// StorageBytes is the approximate storage size in bytes
	StorageBytes int64 `json:"storage_bytes"`

	// LastIngestTime is when the most recent source was ingested
	LastIngestTime time.Time `json:"last_ingest_time"`

	// SourcesByType provides a breakdown of sources by type
	SourcesByType map[string]int `json:"sources_by_type,omitempty"`
}

// SearchOptions configures a knowledge search query.
type SearchOptions struct {
	// Limit is the maximum number of results to return (default: 10)
	Limit int `json:"limit"`

	// Threshold is the minimum similarity score (0-1, default: 0.0)
	Threshold float64 `json:"threshold"`

	// Source filters results to only this source (optional)
	Source string `json:"source,omitempty"`

	// SourceType filters results to only this source type (optional)
	SourceType string `json:"source_type,omitempty"`

	// Filters contains additional metadata filters
	Filters map[string]any `json:"filters,omitempty"`
}

// NewSearchOptions creates a SearchOptions with sensible defaults.
func NewSearchOptions() *SearchOptions {
	return &SearchOptions{
		Limit:     10,
		Threshold: 0.0,
		Filters:   make(map[string]any),
	}
}

// WithLimit sets the maximum number of results.
func (o *SearchOptions) WithLimit(limit int) *SearchOptions {
	o.Limit = limit
	return o
}

// WithThreshold sets the minimum similarity score.
func (o *SearchOptions) WithThreshold(threshold float64) *SearchOptions {
	o.Threshold = threshold
	return o
}

// WithSource filters results to a specific source.
func (o *SearchOptions) WithSource(source string) *SearchOptions {
	o.Source = source
	return o
}

// WithSourceType filters results to a specific source type.
func (o *SearchOptions) WithSourceType(sourceType string) *SearchOptions {
	o.SourceType = sourceType
	return o
}

// WithFilter adds a custom metadata filter.
func (o *SearchOptions) WithFilter(key string, value any) *SearchOptions {
	if o.Filters == nil {
		o.Filters = make(map[string]any)
	}
	o.Filters[key] = value
	return o
}

// ChunkOptions configures chunking behavior for content processing.
type ChunkOptions struct {
	// ChunkSize is the target size in tokens per chunk (default: 512)
	ChunkSize int `json:"chunk_size"`

	// ChunkOverlap is the number of overlapping tokens between chunks (default: 50)
	ChunkOverlap int `json:"chunk_overlap"`
}

// TextChunk represents a chunk of text with metadata before conversion to KnowledgeChunk.
type TextChunk struct {
	// Text is the chunk content
	Text string `json:"text"`

	// Metadata contains structured information about the chunk
	Metadata ChunkMetadata `json:"metadata"`
}

// IngestOptions configures ingestion behavior.
type IngestOptions struct {
	// Force replaces existing content if true
	Force bool `json:"force"`

	// ChunkSize is the target size in tokens per chunk (default: 512)
	ChunkSize int `json:"chunk_size"`

	// ChunkOverlap is the number of overlapping tokens between chunks (default: 50)
	ChunkOverlap int `json:"chunk_overlap"`
}

// NewIngestOptions creates IngestOptions with sensible defaults.
func NewIngestOptions() *IngestOptions {
	return &IngestOptions{
		Force:        false,
		ChunkSize:    512,
		ChunkOverlap: 50,
	}
}

// IngestResult contains the outcome of an ingestion operation.
type IngestResult struct {
	// Source is the identifier of the ingested source
	Source string `json:"source"`

	// SourceType is the type of source (pdf, url, text, directory)
	SourceType string `json:"source_type"`

	// TotalChunks is the total number of chunks created/processed
	TotalChunks int `json:"total_chunks"`

	// ChunksAdded is the number of new chunks stored
	ChunksAdded int `json:"chunks_added"`

	// ChunksSkipped is the number of chunks that already existed
	ChunksSkipped int `json:"chunks_skipped"`

	// Errors contains any errors encountered during ingestion
	Errors []string `json:"errors,omitempty"`

	// DurationMs is the time taken in milliseconds
	DurationMs int64 `json:"duration_ms"`
}
