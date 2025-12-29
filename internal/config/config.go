package config

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/memory"
	"github.com/zero-day-ai/gibson/internal/prompt"
)

// Config is the root configuration for the Gibson Framework.
type Config struct {
	Core     CoreConfig          `mapstructure:"core" yaml:"core" validate:"required"`
	Database DBConfig            `mapstructure:"database" yaml:"database" validate:"required"`
	Security SecurityConfig      `mapstructure:"security" yaml:"security" validate:"required"`
	LLM      LLMConfig           `mapstructure:"llm" yaml:"llm"`
	Memory   memory.MemoryConfig `mapstructure:"memory" yaml:"memory"`
	Prompt   prompt.PromptConfig `mapstructure:"prompt" yaml:"prompt"`
	Logging  LoggingConfig       `mapstructure:"logging" yaml:"logging"`
	Tracing  TracingConfig       `mapstructure:"tracing" yaml:"tracing"`
	Metrics  MetricsConfig       `mapstructure:"metrics" yaml:"metrics"`
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
