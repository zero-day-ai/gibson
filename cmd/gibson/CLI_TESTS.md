# CLI Command Integration Tests

This document describes the CLI integration tests for mission dependency resolution commands.

## Overview

The tests validate CLI command parsing, flag handling, and output formatting for the following commands:
- `gibson mission validate` - Validates mission dependencies
- `gibson mission plan` - Analyzes mission dependencies without execution
- `gibson mission run` - Tests flag handling for the run command

## Test Files

### mission_validate_test.go
Tests for `gibson mission validate -f workflow.yaml` command.

**Test Coverage:**
- ✅ Missing file flag error handling
- ✅ File not found error handling
- ✅ Invalid YAML parsing errors
- ✅ Valid workflow file parsing
- ✅ Output format validation (text, json, yaml)
- ✅ Invalid output format error
- ✅ Auto-install flag (not yet implemented error)
- ✅ Workflows with explicit dependencies section
- ✅ Workflows with multiple node types

**Key Tests:**
- `TestMissionValidateCommand` - Comprehensive command tests with various scenarios
- `TestMissionValidateOutputFormats` - Tests all output format options

### mission_plan_test.go
Tests for `gibson mission plan -f workflow.yaml` command.

**Test Coverage:**
- ✅ Missing file flag error handling
- ✅ File not found error handling
- ✅ Invalid YAML parsing errors
- ✅ Valid workflow dependency tree generation
- ✅ Output formats (text, json, yaml)
- ✅ Invalid output format error
- ✅ Workflows with explicit dependencies
- ✅ Workflows with multiple node types (agent, tool, plugin)
- ✅ Workflows with parallel execution patterns
- ✅ Empty workflows (minimal dependencies)
- ✅ Relative vs absolute file paths
- ✅ Complex dependency trees

**Key Tests:**
- `TestMissionPlanCommand` - Main command test suite
- `TestMissionPlanOutputFormats` - Validates JSON/YAML/text output
- `TestMissionPlanEmptyWorkflow` - Tests workflows with minimal dependencies
- `TestMissionPlanRelativeVsAbsolutePath` - Path resolution tests
- `TestMissionPlanWithComplexDependencies` - Multi-level dependency chains

### mission_run_test.go
Tests for `gibson mission run` command flags.

**Test Coverage:**
- ✅ `--start-dependencies` flag parsing (enabled/disabled/not set)
- ✅ `--memory-continuity` flag validation (isolated/inherit/shared/invalid)
- ✅ `-f/--file` flag vs positional argument precedence
- ✅ Missing file argument error handling

**Key Tests:**
- `TestMissionRunStartDependenciesFlag` - Tests dependency auto-start flag
- `TestMissionRunMemoryContinuityFlag` - Tests memory continuity modes
- `TestMissionRunFileFlag` - Tests file argument handling

## Test Fixtures

Test fixtures are located in `testdata/`:

```
testdata/
├── README.md                          # Fixture documentation
├── simple-workflow.yaml               # Basic single-node workflow
├── workflow-with-dependencies.yaml    # Explicit dependencies section
├── multi-type-workflow.yaml           # Agent, tool, and plugin nodes
├── parallel-workflow.yaml             # Parallel execution pattern
└── invalid-workflow.yaml              # Malformed YAML for error testing
```

## Running Tests

```bash
# Run all CLI tests
cd /home/anthony/Code/zero-day.ai/opensource/gibson
go test -v ./cmd/gibson/... -run "Test.*(Validate|Plan)"

# Run specific test suites
go test -v ./cmd/gibson/... -run "TestMissionValidateCommand"
go test -v ./cmd/gibson/... -run "TestMissionPlanCommand"
go test -v ./cmd/gibson/... -run "TestMissionRun"

# Run with coverage
go test -cover ./cmd/gibson/... -run "Test.*(Validate|Plan)"
```

## Test Patterns

### CLI Command Test Pattern

Tests follow this pattern:

1. **Setup**: Create temp directory and Gibson home
2. **Fixture**: Create or load test workflow YAML
3. **Command**: Create fresh cobra.Command instance
4. **Flags**: Initialize and set command flags
5. **Execute**: Run command and capture output
6. **Assert**: Verify error handling or output formatting

### Example Test Structure

```go
func TestMissionValidateCommand(t *testing.T) {
    tests := []struct {
        name          string
        setupFixtures func(t *testing.T, tmpDir string) string
        flags         map[string]string
        wantError     bool
        errorContains string
        outputChecks  []string
    }{
        // Test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation...
        })
    }
}
```

## Important Notes

### Flag Isolation

Tests save and restore global flags to prevent test pollution:

```go
oldGlobalFlags := *globalFlags
defer func() { *globalFlags = oldGlobalFlags }()
```

### Daemon Not Required

These are **CLI unit tests**, not E2E tests. They test:
- Command parsing
- Flag validation
- Output formatting
- Error handling

They do NOT test:
- Actual daemon communication
- Real component resolution
- Mission execution

Tests expect daemon/dependency errors when components aren't installed. The important validation is that CLI arguments are parsed correctly before reaching daemon logic.

### Mock Implementations

Tests use stub implementations for components that require daemon:
- `stubComponentStore` - Returns empty component lists
- `stubLifecycleManager` - No-op lifecycle operations
- `storeManifestLoader` - Loads from local component directories

## Test Results

All tests pass:
- ✅ 8 test cases in `TestMissionValidateCommand`
- ✅ 5 test cases in `TestMissionValidateOutputFormats`
- ✅ 10 test cases in `TestMissionPlanCommand`
- ✅ 3 test cases in `TestMissionPlanOutputFormats`
- ✅ 1 test case in `TestMissionPlanEmptyWorkflow`
- ✅ 2 test cases in `TestMissionPlanRelativeVsAbsolutePath`
- ✅ 1 test case in `TestMissionPlanWithComplexDependencies`
- ✅ 3 test cases in `TestMissionRunStartDependenciesFlag`
- ✅ 5 test cases in `TestMissionRunMemoryContinuityFlag`
- ✅ 3 test cases in `TestMissionRunFileFlag`

**Total: 41 passing test cases**

## Future Improvements

Potential enhancements:
- [ ] Add E2E tests with actual daemon running
- [ ] Test component manifest resolution with real manifests
- [ ] Test transitive dependency resolution
- [ ] Test version constraint validation
- [ ] Test health check integration
- [ ] Add benchmark tests for large workflows
