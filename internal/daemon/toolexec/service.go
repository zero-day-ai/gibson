package toolexec

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// toolExecutorServiceImpl is the concrete implementation of ToolExecutorService.
// It orchestrates tool discovery, caching, and subprocess-based execution.
type toolExecutorServiceImpl struct {
	// toolsDir is the directory containing tool binaries
	toolsDir string

	// defaultTimeout is the default execution timeout for tools
	defaultTimeout time.Duration

	// logger is the structured logger for service operations
	logger *slog.Logger

	// scanner is responsible for discovering tool binaries
	scanner BinaryScanner

	// executor is responsible for running tool subprocesses
	executor *SubprocessExecutor

	// mu protects concurrent access to the tool registry and metrics
	mu sync.RWMutex

	// tools is the internal registry mapping tool name to tool info and metrics
	tools map[string]*toolEntry
}

// toolEntry represents a tool in the internal registry with its metadata and metrics.
type toolEntry struct {
	// info contains the tool binary information from scanning
	info ToolBinaryInfo

	// metrics tracks execution statistics
	metrics ToolMetrics

	// lastError records the most recent execution error
	lastError error
}

// NewToolExecutorService creates a new ToolExecutorService instance.
//
// Parameters:
//   - toolsDir: Directory containing tool binaries (supports ~ expansion)
//   - defaultTimeout: Default timeout for tool execution (e.g., 5*time.Minute)
//   - logger: Structured logger for service operations
//
// The service must be started with Start() before use.
func NewToolExecutorService(toolsDir string, defaultTimeout time.Duration, logger *slog.Logger) *toolExecutorServiceImpl {
	if logger == nil {
		logger = slog.Default()
	}

	return &toolExecutorServiceImpl{
		toolsDir:       toolsDir,
		defaultTimeout: defaultTimeout,
		logger:         logger,
		scanner:        NewBinaryScanner(),
		executor:       NewSubprocessExecutor(),
		tools:          make(map[string]*toolEntry),
	}
}

// Start initializes the service by scanning the tools directory and populating
// the internal registry with discovered tool binaries and their schemas.
func (s *toolExecutorServiceImpl) Start(ctx context.Context) error {
	s.logger.Info("starting tool executor service", "tools_dir", s.toolsDir)

	// Expand home directory if needed
	toolsDir := s.toolsDir
	if len(toolsDir) > 0 && toolsDir[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		toolsDir = filepath.Join(homeDir, toolsDir[1:])
	}

	// Scan for tool binaries
	toolInfos, err := s.scanner.Scan(ctx, toolsDir)
	if err != nil {
		return fmt.Errorf("failed to scan tools directory: %w", err)
	}

	// Populate the internal registry
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tools = make(map[string]*toolEntry)
	for _, info := range toolInfos {
		s.tools[info.Name] = &toolEntry{
			info: info,
			metrics: ToolMetrics{
				TotalExecutions:      0,
				SuccessfulExecutions: 0,
				FailedExecutions:     0,
				TotalDuration:        0,
				AverageDuration:      0,
				LastExecutedAt:       nil,
			},
		}
	}

	s.logger.Info("tool executor service started",
		"total_tools", len(s.tools),
		"ready_tools", s.countReadyTools(),
		"error_tools", s.countErrorTools())

	return nil
}

// Stop gracefully shuts down the service, cleaning up any resources.
// Currently, this is a no-op as tool executions are short-lived subprocesses
// that are cleaned up by the OS when they complete.
func (s *toolExecutorServiceImpl) Stop(ctx context.Context) error {
	s.logger.Info("stopping tool executor service")
	return nil
}

// Execute runs a tool subprocess with the given input and timeout.
// Input and output are marshaled as JSON via stdin/stdout.
//
// The tool must be registered in the service (discovered via Start or RefreshTools).
// If timeout is 0, the default timeout is used.
//
// Metrics are updated after each execution attempt.
func (s *toolExecutorServiceImpl) Execute(ctx context.Context, name string, input map[string]any, timeout time.Duration) (map[string]any, error) {
	// Look up the tool
	s.mu.RLock()
	entry, exists := s.tools[name]
	s.mu.RUnlock()

	if !exists {
		return nil, types.WrapError(
			ErrToolNotFound,
			fmt.Sprintf("tool not found: %s", name),
			nil,
		)
	}

	// Check if tool has a schema error (not usable)
	if entry.info.Error != nil {
		return nil, types.WrapError(
			ErrToolSchemaFetchError,
			fmt.Sprintf("tool %s has schema error", name),
			entry.info.Error,
		)
	}

	// Use default timeout if not specified
	if timeout == 0 {
		timeout = s.defaultTimeout
	}

	s.logger.Debug("executing tool",
		"name", name,
		"path", entry.info.Path,
		"timeout", timeout)

	// Prepare execution request
	req := &ExecuteRequest{
		BinaryPath: entry.info.Path,
		Input:      input,
		Timeout:    timeout,
		Env:        os.Environ(),
	}

	// Execute the tool
	startTime := time.Now()
	result, err := s.executor.Execute(ctx, req)
	duration := time.Since(startTime)

	// Update metrics
	s.updateMetrics(name, duration, err)

	if err != nil {
		s.logger.Error("tool execution failed",
			"name", name,
			"duration", duration,
			"error", err)
		return nil, err
	}

	// Check for non-zero exit code (already handled by executor, but double-check)
	if result.ExitCode != 0 {
		s.logger.Error("tool exited with non-zero code",
			"name", name,
			"exit_code", result.ExitCode,
			"stderr", result.Stderr)
		return nil, types.WrapError(
			ErrToolExecutionFailed,
			fmt.Sprintf("tool %s exited with code %d", name, result.ExitCode),
			nil,
		)
	}

	s.logger.Info("tool executed successfully",
		"name", name,
		"duration", duration)

	return result.Output, nil
}

// ListTools returns metadata for all discovered tools, including their
// current status and execution metrics.
func (s *toolExecutorServiceImpl) ListTools() []ToolDescriptor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	descriptors := make([]ToolDescriptor, 0, len(s.tools))

	for _, entry := range s.tools {
		descriptor := ToolDescriptor{
			Name:         entry.info.Name,
			Description:  entry.info.Description,
			Version:      entry.info.Version,
			Tags:         entry.info.Tags,
			InputSchema:  entry.info.InputSchema,
			OutputSchema: entry.info.OutputSchema,
			BinaryPath:   entry.info.Path,
			Metrics:      s.cloneMetrics(&entry.metrics),
		}

		// Determine status
		if entry.info.Error != nil {
			descriptor.Status = "error"
			descriptor.ErrorMessage = entry.info.Error.Error()
		} else if entry.info.InputSchema.Type == "" || entry.info.OutputSchema.Type == "" {
			descriptor.Status = "schema-unknown"
		} else {
			descriptor.Status = "ready"
		}

		descriptors = append(descriptors, descriptor)
	}

	return descriptors
}

// GetToolSchema retrieves the input and output schemas for a specific tool.
// Returns ErrToolNotFound if the tool is not registered.
func (s *toolExecutorServiceImpl) GetToolSchema(name string) (*ToolSchema, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.tools[name]
	if !exists {
		return nil, types.WrapError(
			ErrToolNotFound,
			fmt.Sprintf("tool not found: %s", name),
			nil,
		)
	}

	return &ToolSchema{
		InputSchema:  entry.info.InputSchema,
		OutputSchema: entry.info.OutputSchema,
	}, nil
}

// RefreshTools rescans the tools directory and updates the internal registry.
// This enables hot-reload of tools without restarting the daemon.
//
// Metrics for existing tools are preserved. New tools are added with zero metrics.
// Removed tools are deleted from the registry.
func (s *toolExecutorServiceImpl) RefreshTools(ctx context.Context) error {
	s.logger.Info("refreshing tools registry")

	// Expand home directory if needed
	toolsDir := s.toolsDir
	if len(toolsDir) > 0 && toolsDir[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		toolsDir = filepath.Join(homeDir, toolsDir[1:])
	}

	// Scan for tool binaries
	toolInfos, err := s.scanner.Scan(ctx, toolsDir)
	if err != nil {
		return fmt.Errorf("failed to scan tools directory: %w", err)
	}

	// Update the registry while preserving metrics for existing tools
	s.mu.Lock()
	defer s.mu.Unlock()

	newTools := make(map[string]*toolEntry)

	for _, info := range toolInfos {
		if existingEntry, exists := s.tools[info.Name]; exists {
			// Preserve existing metrics, update info
			existingEntry.info = info
			newTools[info.Name] = existingEntry
		} else {
			// New tool - initialize with zero metrics
			newTools[info.Name] = &toolEntry{
				info: info,
				metrics: ToolMetrics{
					TotalExecutions:      0,
					SuccessfulExecutions: 0,
					FailedExecutions:     0,
					TotalDuration:        0,
					AverageDuration:      0,
					LastExecutedAt:       nil,
				},
			}
		}
	}

	s.tools = newTools

	s.logger.Info("tools registry refreshed",
		"total_tools", len(s.tools),
		"ready_tools", s.countReadyTools(),
		"error_tools", s.countErrorTools())

	return nil
}

// updateMetrics updates execution metrics for a tool after execution.
// This method is thread-safe and can be called concurrently.
func (s *toolExecutorServiceImpl) updateMetrics(name string, duration time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.tools[name]
	if !exists {
		return
	}

	now := time.Now()

	// Update counters
	entry.metrics.TotalExecutions++
	if err != nil {
		entry.metrics.FailedExecutions++
		entry.lastError = err
	} else {
		entry.metrics.SuccessfulExecutions++
		entry.lastError = nil
	}

	// Update duration metrics
	entry.metrics.TotalDuration += duration
	entry.metrics.AverageDuration = entry.metrics.TotalDuration / time.Duration(entry.metrics.TotalExecutions)
	entry.metrics.LastExecutedAt = &now
}

// countReadyTools counts the number of tools with valid schemas.
// Must be called with lock held.
func (s *toolExecutorServiceImpl) countReadyTools() int {
	count := 0
	for _, entry := range s.tools {
		if entry.info.Error == nil &&
			entry.info.InputSchema.Type != "" &&
			entry.info.OutputSchema.Type != "" {
			count++
		}
	}
	return count
}

// countErrorTools counts the number of tools with schema errors.
// Must be called with lock held.
func (s *toolExecutorServiceImpl) countErrorTools() int {
	count := 0
	for _, entry := range s.tools {
		if entry.info.Error != nil {
			count++
		}
	}
	return count
}

// cloneMetrics creates a copy of ToolMetrics to prevent external mutation.
func (s *toolExecutorServiceImpl) cloneMetrics(m *ToolMetrics) *ToolMetrics {
	clone := &ToolMetrics{
		TotalExecutions:      m.TotalExecutions,
		SuccessfulExecutions: m.SuccessfulExecutions,
		FailedExecutions:     m.FailedExecutions,
		TotalDuration:        m.TotalDuration,
		AverageDuration:      m.AverageDuration,
	}
	if m.LastExecutedAt != nil {
		t := *m.LastExecutedAt
		clone.LastExecutedAt = &t
	}
	return clone
}
