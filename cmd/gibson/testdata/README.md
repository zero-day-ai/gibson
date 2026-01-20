# Test Fixtures for CLI Commands

This directory contains test fixtures (YAML workflow files) used by CLI command integration tests.

## Files

### simple-workflow.yaml
A basic workflow with a single agent node. Used for testing basic parsing and validation.

### workflow-with-dependencies.yaml
Workflow with explicit dependencies section that declares required agents, tools, and plugins.
Tests dependency resolution from the mission-level dependencies block.

### multi-type-workflow.yaml
Workflow demonstrating all three component types (agents, tools, plugins) with proper dependency chaining.
Tests mixed node types and sequential dependencies.

### parallel-workflow.yaml
Workflow with parallel execution pattern using a parallel node with multiple sub-nodes.
Tests parallel execution planning and dependency resolution.

### invalid-workflow.yaml
Intentionally malformed YAML file for testing error handling.
Tests parser error handling and user-friendly error messages.

## Usage in Tests

These fixtures are used by:
- `mission_validate_test.go` - Tests for `gibson mission validate` command
- `mission_plan_test.go` - Tests for `gibson mission plan` command

Tests create temporary copies of these files or generate their own inline to avoid coupling tests to static files.
