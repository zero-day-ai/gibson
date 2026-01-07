# EvalConfigLoader Implementation

This document describes the YAML configuration loader for Gibson's evaluation system, implemented as part of Task 2 of the Gibson Eval Integration spec.

## Overview

The EvalConfigLoader provides a clean, declarative way to configure evaluation settings through YAML files instead of programmatic configuration. This makes it easier to version control evaluation configurations, share them across teams, and modify settings without code changes.

## Files

### Core Implementation

- **`config.go`** - Main implementation containing:
  - `EvalConfig` struct - represents the YAML configuration structure
  - `ScorerConfig` struct - configuration for individual scorers
  - `ThresholdsConfig` struct - warning and critical thresholds
  - `ExportConfig` struct - export destination configuration
  - `LoadEvalConfig(path string)` - loads and validates YAML config
  - `ToOptions()` - converts EvalConfig to EvalOptions
  - `BuildScorers()` - instantiates StreamingScorer instances

### Tests

- **`config_test.go`** - Unit tests covering:
  - Successful config loading
  - File not found errors
  - Invalid YAML parsing errors
  - Validation failures (missing fields, invalid values)
  - ToOptions conversion
  - BuildScorers for all scorer types
  - Edge cases

- **`config_integration_test.go`** - Integration tests covering:
  - Full workflow (load → validate → convert → build)
  - Minimal configuration
  - All scorers disabled scenario
  - Real example config file
  - Error handling scenarios

### Example

- **`../../testdata/eval_config_example.yaml`** - Complete example configuration demonstrating all features

## Usage

### Loading a Configuration

```go
import "github.com/zero-day-ai/gibson/internal/eval"

// Load configuration from YAML file
config, err := eval.LoadEvalConfig("./eval_config.yaml")
if err != nil {
    return fmt.Errorf("failed to load eval config: %w", err)
}

// Convert to runtime options
opts := config.ToOptions()

// Build scorer instances
scorers, err := config.BuildScorers()
if err != nil {
    return fmt.Errorf("failed to build scorers: %w", err)
}
```

### YAML Structure

```yaml
scorers:
  - name: tool_correctness
    enabled: true
    options:
      order_matters: true
      numeric_tolerance: 0.001

  - name: trajectory
    enabled: true
    options:
      mode: ordered_subset
      penalize_extra: 0.05

  - name: finding_accuracy
    enabled: true
    options:
      match_by_severity: true
      match_by_category: false
      fuzzy_title_threshold: 0.8

thresholds:
  warning: 0.5
  critical: 0.2

export:
  langfuse: true
  otel: false
  jsonl: "./eval_results.jsonl"

ground_truth: "./testdata/ground_truth.json"
expected_tools: "./testdata/expected_tools.json"
```

## Supported Scorers

### tool_correctness

Evaluates whether the correct tools were called with correct arguments.

**Options:**
- `order_matters` (bool) - Whether tool call order is significant
- `numeric_tolerance` (float64) - Tolerance for comparing numeric arguments

### trajectory

Evaluates the agent's execution path against expected steps.

**Options:**
- `mode` (string) - Matching mode: `exact_match`, `subset_match`, or `ordered_subset`
- `penalize_extra` (float64) - Penalty per extra step (0.0 - 1.0)

### finding_accuracy

Evaluates the accuracy of discovered vulnerabilities.

**Options:**
- `match_by_severity` (bool) - Weight findings by severity level
- `match_by_category` (bool) - Require category matching
- `fuzzy_title_threshold` (float64) - Similarity threshold for fuzzy title matching (0.0 - 1.0)

## Validation

The loader performs comprehensive validation:

1. **File existence** - Config file must exist and be readable
2. **YAML syntax** - Must be valid YAML
3. **Required fields** - `ground_truth` path is required
4. **Threshold ranges** - Warning and critical thresholds must be in [0.0, 1.0]
5. **Threshold ordering** - Critical ≤ Warning
6. **Scorer names** - Must be one of: `tool_correctness`, `trajectory`, `finding_accuracy`
7. **Scorer options** - Type-checked and validated for each scorer type

## Error Handling

All errors use Gibson's typed error system with clear error codes:

- `CONFIG_NOT_FOUND` - Config file doesn't exist
- `CONFIG_LOAD_FAILED` - Failed to read config file
- `CONFIG_PARSE_FAILED` - Invalid YAML syntax
- `CONFIG_VALIDATION_FAILED` - Validation errors (detailed messages provided)

## Design Decisions

### Why YAML over JSON?

YAML was chosen because:
1. More human-readable and writable
2. Supports comments for documentation
3. Less verbose than JSON
4. Already used throughout Gibson codebase
5. Better for configuration files

### Scorer Option Flexibility

Scorer options use `map[string]any` to allow:
1. Each scorer type to have unique options
2. Future extensibility without breaking changes
3. Type conversion for numeric values (int → float64)

### Builder Pattern

The `BuildScorers()` method:
1. Only instantiates enabled scorers
2. Validates option types at build time
3. Returns clear error messages for invalid configurations
4. Creates SDK scorer instances directly

## Testing

All tests pass with 100% coverage of critical paths:

```bash
cd <gibson-root>/internal/eval
go test -run "TestLoadEvalConfig|TestEvalConfig|TestConfigIntegration" -v
```

**Test coverage:**
- ✅ Successful config loading
- ✅ File not found errors
- ✅ YAML parsing errors
- ✅ All validation rules
- ✅ ToOptions conversion
- ✅ BuildScorers for all scorer types
- ✅ Invalid scorer options
- ✅ Numeric type conversion (int/float64)
- ✅ Edge cases (empty options, disabled scorers)
- ✅ Full integration workflow

## Future Enhancements

Potential improvements for future versions:

1. **Schema validation** - Use JSON Schema for YAML validation
2. **Config merging** - Support multiple config files with inheritance
3. **Environment variables** - Interpolate env vars in config (e.g., `${GROUND_TRUTH_PATH}`)
4. **Scorer plugins** - Support custom scorers defined in config
5. **Presets** - Predefined scorer configurations (e.g., "strict", "permissive")
6. **Config validation CLI** - `gibson eval validate-config` command

## Dependencies

- `gopkg.in/yaml.v3` - YAML parsing (already used in Gibson)
- `github.com/zero-day-ai/sdk/eval` - Scorer interfaces and implementations
- `github.com/zero-day-ai/gibson/internal/types` - Error handling

## Related Files

- `options.go` - EvalOptions struct (Task 1)
- `../../testdata/eval_config_example.yaml` - Example configuration
- Spec: Gibson Eval Integration - Task 2

## Author

Implementation: Claude Code (Sonnet 4.5)
Date: 2026-01-05
Spec: Gibson Eval Integration (R8)
