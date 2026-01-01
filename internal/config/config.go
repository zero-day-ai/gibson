package config

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/prompt"
)

// Config is the root configuration for the Gibson Framework.
type Config struct {
	Core          CoreConfig                                 `mapstructure:"core" yaml:"core" validate:"required"`
	Database      DBConfig                                   `mapstructure:"database" yaml:"database" validate:"required"`
	Security      SecurityConfig                             `mapstructure:"security" yaml:"security" validate:"required"`
	LLM           LLMConfig                                  `mapstructure:"llm" yaml:"llm"`
	Memory        memory.MemoryConfig                        `mapstructure:"memory" yaml:"memory"`
	Prompt        prompt.PromptConfig                        `mapstructure:"prompt" yaml:"prompt"`
	Logging       LoggingConfig                              `mapstructure:"logging" yaml:"logging"`
	Tracing       TracingConfig                              `mapstructure:"tracing" yaml:"tracing"`
	Metrics       MetricsConfig                              `mapstructure:"metrics" yaml:"metrics"`
	RemoteAgents  map[string]component.RemoteComponentConfig `mapstructure:"remote_agents" yaml:"remote_agents,omitempty"`
	RemoteTools   map[string]component.RemoteComponentConfig `mapstructure:"remote_tools" yaml:"remote_tools,omitempty"`
	RemotePlugins map[string]component.RemoteComponentConfig `mapstructure:"remote_plugins" yaml:"remote_plugins,omitempty"`
	Registration  RegistrationConfig                         `mapstructure:"registration" yaml:"registration,omitempty"`
}

// CoreConfig contains core application settings.
type CoreConfig struct {
	HomeDir       string        `mapstructure:"home_dir" yaml:"home_dir"`
	DataDir       string        `mapstructure:"data_dir" yaml:"data_dir"`
	CacheDir      string        `mapstructure:"cache_dir" yaml:"cache_dir"`
	ParallelLimit int           `mapstructure:"parallel_limit" yaml:"parallel_limit" validate:"min=1,max=100"`
	Timeout       time.Duration `mapstructure:"timeout" yaml:"timeout" validate:"min=1s"`
	Debug         bool          `mapstructure:"debug" yaml:"debug"`
}

// DBConfig contains database configuration.
type DBConfig struct {
	Path           string        `mapstructure:"path" yaml:"path"`
	MaxConnections int           `mapstructure:"max_connections" yaml:"max_connections" validate:"min=1,max=100"`
	Timeout        time.Duration `mapstructure:"timeout" yaml:"timeout" validate:"min=1s"`
	WALMode        bool          `mapstructure:"wal_mode" yaml:"wal_mode"`
	AutoVacuum     bool          `mapstructure:"auto_vacuum" yaml:"auto_vacuum"`
}

// SecurityConfig contains security-related settings.
type SecurityConfig struct {
	EncryptionAlgorithm string `mapstructure:"encryption_algorithm" yaml:"encryption_algorithm"`
	KeyDerivation       string `mapstructure:"key_derivation" yaml:"key_derivation"`
	SSLValidation       bool   `mapstructure:"ssl_validation" yaml:"ssl_validation"`
	AuditLogging        bool   `mapstructure:"audit_logging" yaml:"audit_logging"`
}

// LLMConfig contains LLM provider configuration (stub for future stages).
type LLMConfig struct {
	DefaultProvider string `mapstructure:"default_provider" yaml:"default_provider"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
}

// TracingConfig contains distributed tracing configuration.
type TracingConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`
}

// MetricsConfig contains metrics export configuration.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	Port    int  `mapstructure:"port" yaml:"port"`
}

// RegistrationConfig contains configuration for the optional agent self-announcement server.
// When enabled, agents can dynamically register themselves with Gibson by sending heartbeat
// announcements with their capabilities and network addresses.
//
// This is an optional feature - the primary mechanism for remote component discovery is
// static configuration via remote_agents, remote_tools, and remote_plugins.
type RegistrationConfig struct {
	// Enabled controls whether the registration server is started
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`

	// Port is the TCP port for the registration gRPC server (default: 50100)
	// Validation only applies when Enabled is true
	Port int `mapstructure:"port" yaml:"port"`

	// AuthToken is an optional authentication token that agents must provide when registering
	// If empty, no authentication is required (not recommended for production)
	AuthToken string `mapstructure:"auth_token" yaml:"auth_token,omitempty"`

	// HeartbeatTimeout is the duration after which an agent is considered dead if no heartbeat
	// is received (default: 30s)
	HeartbeatTimeout time.Duration `mapstructure:"heartbeat_timeout" yaml:"heartbeat_timeout,omitempty"`
}
