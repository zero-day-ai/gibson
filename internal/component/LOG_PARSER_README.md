# Log Parser

The log parser provides efficient parsing and filtering of component log files to extract recent error-level entries for debugging and status reporting.

## Overview

The `ParseRecentErrors` function reads log files and extracts ERROR and FATAL level log entries, returning them in reverse chronological order (newest first). It supports two common log formats and handles various edge cases gracefully.

## Features

- **Dual Format Support**: Parses both JSON and key=value log formats
- **Efficient Processing**: Handles large log files efficiently (tested with 10,000+ entries)
- **Error Filtering**: Only extracts ERROR and FATAL level entries
- **Chronological Ordering**: Returns errors newest-first for immediate relevance
- **Graceful Degradation**: Missing files, empty files, and malformed lines don't cause failures
- **Timestamp Flexibility**: Supports multiple timestamp formats (RFC3339, RFC3339Nano, custom formats)

## Supported Log Formats

### JSON Format
```json
{"level":"ERROR","msg":"connection failed","time":"2025-01-01T12:01:00Z"}
{"level":"FATAL","msg":"system crash","time":"2025-01-01T12:03:00Z"}
```

### Key=Value Format
```
time=2025-01-01T12:01:00Z level=ERROR msg="connection failed"
time=2025-01-01T12:03:00Z level=FATAL msg="system crash"
```

Both formats can be mixed in the same file.

## Usage

```go
import "github.com/zero-day-ai/gibson/internal/component"

// Parse the 5 most recent errors from a component log
errors, err := component.ParseRecentErrors("/path/to/component.log", 5)
if err != nil {
    // Handle error
}

for _, logErr := range errors {
    fmt.Printf("[%s] %s at %s\n",
        logErr.Level,
        logErr.Message,
        logErr.Timestamp.Format(time.RFC3339))
}
```

## API

### ParseRecentErrors

```go
func ParseRecentErrors(logPath string, count int) ([]LogError, error)
```

**Parameters:**
- `logPath`: Absolute path to the log file to parse
- `count`: Maximum number of recent errors to return

**Returns:**
- `[]LogError`: Slice of error log entries, newest first (up to `count` entries)
- `error`: Always returns `nil` (graceful error handling)

**Edge Cases:**
- Missing file: Returns empty slice, no error
- Empty file: Returns empty slice, no error
- No errors found: Returns empty slice, no error
- Malformed lines: Skipped silently
- Count of 0: Returns empty slice

### LogError Type

```go
type LogError struct {
    Timestamp time.Time  // When the error occurred
    Message   string     // Error message content
    Level     string     // Log level (ERROR or FATAL)
}
```

## Performance

The parser is optimized for performance:

- Parses 10,000 log entries (~1MB) in approximately 15-25ms
- Memory efficient with buffered reading (64KB buffer)
- Protects against malformed logs (1MB max line length)
- Suitable for real-time status reporting

## Integration with Status Command

The log parser is designed to be used by the `gibson agent status`, `gibson tool status`, and `gibson plugin status` commands:

```go
// In the status command implementation
logPath := filepath.Join(componentDir, "logs", componentName + ".log")
recentErrors, _ := component.ParseRecentErrors(logPath, 5)

statusResult := &component.StatusResult{
    Component: comp,
    ProcessState: state,
    HealthCheck: healthResult,
    RecentErrors: recentErrors,  // Attach parsed errors
    Uptime: uptime,
}
```

## Error Levels

The parser filters for these log levels (case-insensitive):
- **ERROR**: General error conditions
- **FATAL**: Critical errors that may cause component termination

Other levels (INFO, DEBUG, WARN, TRACE) are ignored.

## Testing

Comprehensive test coverage includes:

- **Unit Tests**: Format parsing, edge cases, error handling
- **Integration Tests**: Realistic component log scenarios
- **Performance Tests**: Large file handling (10,000+ entries)
- **Example Tests**: Usage demonstrations
- **Benchmark Tests**: Performance measurement

Run tests:
```bash
go test -tags fts5 -v -run TestParseRecentErrors ./internal/component/
```

Run benchmarks:
```bash
go test -tags fts5 -bench=BenchmarkParseRecentErrors ./internal/component/
```

## Implementation Details

### Parsing Strategy

1. **Read entire file**: Uses buffered reader for efficiency
2. **Parse each line**: Try JSON first, fall back to key=value
3. **Filter by level**: Only keep ERROR and FATAL entries
4. **Sort by timestamp**: Newest first using reverse chronological order
5. **Limit results**: Return only the most recent `count` errors

### Timestamp Parsing

Supports multiple formats:
- RFC3339: `2025-01-01T12:00:00Z`
- RFC3339Nano: `2025-01-01T12:00:00.123456789Z`
- No timezone: `2025-01-01T12:00:00`
- Space separator: `2025-01-01 12:00:00`
- RFC1123/RFC1123Z

If timestamp parsing fails, a zero time is used (these entries still appear but sort to the end).

### Quoted Value Handling

The key=value parser handles quoted values with spaces and special characters:

```
msg="error with spaces and = signs"
```

## Future Enhancements

Potential improvements for extremely large log files:
- True tail-reading from end of file (seek to end, read backwards)
- Streaming parser for files larger than available memory
- Log rotation awareness (reading from .log.1, .log.2, etc.)
- Binary format support (protobuf, msgpack)

## Files

- `log_parser.go`: Main implementation
- `log_parser_test.go`: Unit tests
- `log_parser_example_test.go`: Usage examples
- `log_parser_integration_test.go`: Integration tests
- `status_types.go`: LogError type definition

## Dependencies

- Standard library only (no external dependencies)
- Uses `time`, `bufio`, `encoding/json`, `os`, `sort`, `strings`
