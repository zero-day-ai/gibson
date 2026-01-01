package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/zero-day-ai/gibson/internal/component"
)

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() *Config {
	homeDir := getDefaultHomeDir()

	return &Config{
		Core: CoreConfig{
			HomeDir:       homeDir,
			DataDir:       filepath.Join(homeDir, "data"),
			CacheDir:      filepath.Join(homeDir, "cache"),
			ParallelLimit: 10,
			Timeout:       5 * time.Minute,
			Debug:         false,
		},
		Database: DBConfig{
			Path:           filepath.Join(homeDir, "gibson.db"),
			MaxConnections: 10,
			Timeout:        30 * time.Second,
			WALMode:        true,
			AutoVacuum:     true,
		},
		Security: SecurityConfig{
			EncryptionAlgorithm: "aes-256-gcm",
			KeyDerivation:       "scrypt",
			SSLValidation:       true,
			AuditLogging:        true,
		},
		LLM: LLMConfig{
			DefaultProvider: "",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Tracing: TracingConfig{
			Enabled:  false,
			Endpoint: "",
		},
		Metrics: MetricsConfig{
			Enabled: false,
			Port:    9090,
		},
		RemoteAgents:  make(map[string]component.RemoteComponentConfig),
		RemoteTools:   make(map[string]component.RemoteComponentConfig),
		RemotePlugins: make(map[string]component.RemoteComponentConfig),
		Registration: RegistrationConfig{
			Enabled:          false,
			Port:             50100,
			AuthToken:        "",
			HeartbeatTimeout: 30 * time.Second,
		},
	}
}

// getDefaultHomeDir returns the default Gibson home directory.
// It uses ~/.gibson or falls back to a temporary directory if user home cannot be determined.
func getDefaultHomeDir() string {
	userHome, err := os.UserHomeDir()
	if err != nil {
		// Fallback to temporary directory if user home cannot be determined
		return filepath.Join(os.TempDir(), ".gibson")
	}
	return filepath.Join(userHome, ".gibson")
}
