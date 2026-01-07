package verbose

// VerboseFormatter formats verbose events for output.
// Implementations can provide text, JSON, or other formats.
type VerboseFormatter interface {
	// Format converts a VerboseEvent into a formatted string for output.
	// The returned string should be ready to write directly to output
	// (including newlines if appropriate for the format).
	Format(event VerboseEvent) string
}
