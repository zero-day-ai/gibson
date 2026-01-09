package toolexec

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/schema"
)

func TestNewToolExecutorService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService("~/.gibson/tools/bin", 5*time.Minute, logger)

	assert.NotNil(t, service)
	assert.Equal(t, "~/.gibson/tools/bin", service.toolsDir)
	assert.Equal(t, 5*time.Minute, service.defaultTimeout)
	assert.NotNil(t, service.scanner)
	assert.NotNil(t, service.executor)
	assert.NotNil(t, service.tools)
}

func TestNewToolExecutorService_NilLogger(t *testing.T) {
	service := NewToolExecutorService("~/.gibson/tools/bin", 5*time.Minute, nil)

	assert.NotNil(t, service)
	assert.NotNil(t, service.logger) // Should use default logger
}

func TestToolExecutorService_StartStop(t *testing.T) {
	// Create temporary tools directory
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	require.NoError(t, os.MkdirAll(toolsDir, 0755))

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(toolsDir, 5*time.Minute, logger)

	ctx := context.Background()

	// Start should succeed even with empty directory
	err := service.Start(ctx)
	assert.NoError(t, err)

	// Stop should always succeed
	err = service.Stop(ctx)
	assert.NoError(t, err)
}

func TestToolExecutorService_StartNonexistentDirectory(t *testing.T) {
	// Use a directory that doesn't exist
	toolsDir := "/tmp/nonexistent-tools-dir-" + time.Now().Format("20060102150405")

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(toolsDir, 5*time.Minute, logger)

	ctx := context.Background()

	// Start should succeed but return empty tools list
	err := service.Start(ctx)
	assert.NoError(t, err)

	tools := service.ListTools()
	assert.Empty(t, tools)
}

func TestToolExecutorService_ListToolsEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(tmpDir, 5*time.Minute, logger)

	ctx := context.Background()
	require.NoError(t, service.Start(ctx))

	tools := service.ListTools()
	assert.Empty(t, tools)
}

func TestToolExecutorService_GetToolSchemaNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(tmpDir, 5*time.Minute, logger)

	ctx := context.Background()
	require.NoError(t, service.Start(ctx))

	schema, err := service.GetToolSchema("nonexistent-tool")
	assert.Error(t, err)
	assert.Nil(t, schema)
	assert.Contains(t, err.Error(), "tool not found")
}

func TestToolExecutorService_ExecuteNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(tmpDir, 5*time.Minute, logger)

	ctx := context.Background()
	require.NoError(t, service.Start(ctx))

	output, err := service.Execute(ctx, "nonexistent-tool", map[string]any{}, 0)
	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), "tool not found")
}

func TestToolExecutorService_RefreshTools(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	require.NoError(t, os.MkdirAll(toolsDir, 0755))

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(toolsDir, 5*time.Minute, logger)

	ctx := context.Background()
	require.NoError(t, service.Start(ctx))

	// Initially empty
	tools := service.ListTools()
	assert.Empty(t, tools)

	// Refresh should succeed even with empty directory
	err := service.RefreshTools(ctx)
	assert.NoError(t, err)

	tools = service.ListTools()
	assert.Empty(t, tools)
}

func TestToolExecutorService_MetricsClone(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(tmpDir, 5*time.Minute, logger)

	now := time.Now()
	original := &ToolMetrics{
		TotalExecutions:      10,
		SuccessfulExecutions: 8,
		FailedExecutions:     2,
		TotalDuration:        10 * time.Second,
		AverageDuration:      1 * time.Second,
		LastExecutedAt:       &now,
	}

	clone := service.cloneMetrics(original)

	// Verify all fields are copied
	assert.Equal(t, original.TotalExecutions, clone.TotalExecutions)
	assert.Equal(t, original.SuccessfulExecutions, clone.SuccessfulExecutions)
	assert.Equal(t, original.FailedExecutions, clone.FailedExecutions)
	assert.Equal(t, original.TotalDuration, clone.TotalDuration)
	assert.Equal(t, original.AverageDuration, clone.AverageDuration)

	// Verify LastExecutedAt is cloned (not same pointer)
	assert.NotNil(t, clone.LastExecutedAt)
	assert.Equal(t, *original.LastExecutedAt, *clone.LastExecutedAt)
	assert.NotSame(t, original.LastExecutedAt, clone.LastExecutedAt)
}

func TestToolExecutorService_MetricsCloneNilLastExecutedAt(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(tmpDir, 5*time.Minute, logger)

	original := &ToolMetrics{
		TotalExecutions:      5,
		SuccessfulExecutions: 5,
		FailedExecutions:     0,
		TotalDuration:        5 * time.Second,
		AverageDuration:      1 * time.Second,
		LastExecutedAt:       nil,
	}

	clone := service.cloneMetrics(original)

	assert.Nil(t, clone.LastExecutedAt)
}

func TestToolExecutorService_CountReadyTools(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewToolExecutorService(tmpDir, 5*time.Minute, logger)

	// Manually populate tools for testing
	service.tools = map[string]*toolEntry{
		"tool1": {
			info: ToolBinaryInfo{
				Name: "tool1",
				InputSchema: schema.JSONSchema{
					Type: "object",
				},
				OutputSchema: schema.JSONSchema{
					Type: "object",
				},
			},
		},
		"tool2": {
			info: ToolBinaryInfo{
				Name:  "tool2",
				Error: assert.AnError,
			},
		},
		"tool3": {
			info: ToolBinaryInfo{
				Name: "tool3",
				InputSchema: schema.JSONSchema{
					Type: "",
				},
				OutputSchema: schema.JSONSchema{
					Type: "",
				},
			},
		},
	}

	readyCount := service.countReadyTools()
	assert.Equal(t, 1, readyCount) // Only tool1 is ready

	errorCount := service.countErrorTools()
	assert.Equal(t, 1, errorCount) // Only tool2 has an error
}
