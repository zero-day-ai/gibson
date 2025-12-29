package plugin

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/schema"
)

// PluginConfig holds plugin initialization configuration
type PluginConfig struct {
	Name       string         `json:"name"`
	Settings   map[string]any `json:"settings"`
	Timeout    time.Duration  `json:"timeout"`
	RetryCount int            `json:"retry_count"`
}

// MethodDescriptor describes a plugin method
type MethodDescriptor struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	InputSchema  schema.JSONSchema `json:"input_schema"`
	OutputSchema schema.JSONSchema `json:"output_schema"`
}

// PluginDescriptor contains plugin metadata
type PluginDescriptor struct {
	Name       string             `json:"name"`
	Version    string             `json:"version"`
	Methods    []MethodDescriptor `json:"methods"`
	IsExternal bool               `json:"is_external"`
	Status     PluginStatus       `json:"status"`
}

// PluginStatus represents plugin lifecycle status
type PluginStatus string

const (
	PluginStatusUninitialized PluginStatus = "uninitialized"
	PluginStatusInitializing  PluginStatus = "initializing"
	PluginStatusRunning       PluginStatus = "running"
	PluginStatusStopping      PluginStatus = "stopping"
	PluginStatusStopped       PluginStatus = "stopped"
	PluginStatusError         PluginStatus = "error"
)

// String returns the string representation of PluginStatus
func (s PluginStatus) String() string {
	return string(s)
}

// IsValid checks if the PluginStatus is a valid value
func (s PluginStatus) IsValid() bool {
	switch s {
	case PluginStatusUninitialized, PluginStatusInitializing, PluginStatusRunning,
		PluginStatusStopping, PluginStatusStopped, PluginStatusError:
		return true
	default:
		return false
	}
}
