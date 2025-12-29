package agent

import (
	"context"
	"fmt"
	"time"
)

// DelegationHarness implements AgentHarness for delegated agent execution.
// This is used internally by the registry when executing delegated tasks.
type DelegationHarness struct {
	registry AgentRegistry
	logger   Logger
	toolExec ToolExecutor
	pluginExec PluginExecutor
}

// Logger interface for structured logging
type Logger interface {
	Log(level, message string, fields map[string]any)
}

// ToolExecutor interface for executing tools
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, name string, input map[string]any) (map[string]any, error)
}

// PluginExecutor interface for querying plugins
type PluginExecutor interface {
	QueryPlugin(ctx context.Context, plugin, method string, params map[string]any) (any, error)
}

// NewDelegationHarness creates a new delegation harness
func NewDelegationHarness(registry AgentRegistry) *DelegationHarness {
	return &DelegationHarness{
		registry: registry,
		logger:   &defaultLogger{},
		toolExec: &noopToolExecutor{},
		pluginExec: &noopPluginExecutor{},
	}
}

// WithLogger sets the logger for this harness
func (h *DelegationHarness) WithLogger(logger Logger) *DelegationHarness {
	h.logger = logger
	return h
}

// WithToolExecutor sets the tool executor for this harness
func (h *DelegationHarness) WithToolExecutor(exec ToolExecutor) *DelegationHarness {
	h.toolExec = exec
	return h
}

// WithPluginExecutor sets the plugin executor for this harness
func (h *DelegationHarness) WithPluginExecutor(exec PluginExecutor) *DelegationHarness {
	h.pluginExec = exec
	return h
}

// ExecuteTool executes a tool and returns its output
func (h *DelegationHarness) ExecuteTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	h.Log("debug", "executing tool", map[string]any{
		"tool": name,
	})

	result, err := h.toolExec.ExecuteTool(ctx, name, input)
	if err != nil {
		h.Log("error", "tool execution failed", map[string]any{
			"tool":  name,
			"error": err.Error(),
		})
		return nil, err
	}

	return result, nil
}

// QueryPlugin queries a plugin for data or executes a plugin method
func (h *DelegationHarness) QueryPlugin(ctx context.Context, plugin, method string, params map[string]any) (any, error) {
	h.Log("debug", "querying plugin", map[string]any{
		"plugin": plugin,
		"method": method,
	})

	result, err := h.pluginExec.QueryPlugin(ctx, plugin, method, params)
	if err != nil {
		h.Log("error", "plugin query failed", map[string]any{
			"plugin": plugin,
			"method": method,
			"error":  err.Error(),
		})
		return nil, err
	}

	return result, nil
}

// DelegateToAgent delegates a task to another agent
func (h *DelegationHarness) DelegateToAgent(ctx context.Context, agentName string, task Task) (Result, error) {
	h.Log("info", "delegating to agent", map[string]any{
		"agent":       agentName,
		"task":        task.ID.String(),
		"task_name":   task.Name,
	})

	startTime := time.Now()
	result, err := h.registry.DelegateToAgent(ctx, agentName, task, h)
	duration := time.Since(startTime)

	if err != nil {
		h.Log("error", "agent delegation failed", map[string]any{
			"agent":    agentName,
			"task":     task.ID.String(),
			"error":    err.Error(),
			"duration": duration.String(),
		})
		return Result{}, err
	}

	h.Log("info", "agent delegation completed", map[string]any{
		"agent":    agentName,
		"task":     task.ID.String(),
		"status":   result.Status,
		"duration": duration.String(),
		"findings": len(result.Findings),
	})

	return result, nil
}

// Log writes a structured log message
func (h *DelegationHarness) Log(level, message string, fields map[string]any) {
	if h.logger != nil {
		h.logger.Log(level, message, fields)
	}
}

// defaultLogger is a simple console logger
type defaultLogger struct{}

func (l *defaultLogger) Log(level, message string, fields map[string]any) {
	// Simple console output for now
	// Full logging implementation will be added in Stage 4
	fmt.Printf("[%s] %s %v\n", level, message, fields)
}

// noopToolExecutor is a placeholder tool executor
type noopToolExecutor struct{}

func (e *noopToolExecutor) ExecuteTool(ctx context.Context, name string, input map[string]any) (map[string]any, error) {
	return nil, fmt.Errorf("tool execution not yet implemented (Stage 3)")
}

// noopPluginExecutor is a placeholder plugin executor
type noopPluginExecutor struct{}

func (e *noopPluginExecutor) QueryPlugin(ctx context.Context, plugin, method string, params map[string]any) (any, error) {
	return nil, fmt.Errorf("plugin execution not yet implemented (Stage 4)")
}
