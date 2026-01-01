package observability

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestTracingConfig_Validate tests TracingConfig validation logic.
func TestTracingConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    TracingConfig
		wantError bool
		errMsg    string
	}{
		{
			name: "invalid jaeger config - no longer supported",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "jaeger",
				Endpoint:    "http://localhost:14268",
				ServiceName: "gibson-test",
				SampleRate:  1.0,
			},
			wantError: true,
			errMsg:    "invalid tracing provider",
		},
		{
			name: "valid otlp config",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "otlp",
				Endpoint:    "http://localhost:4318",
				ServiceName: "gibson",
				SampleRate:  0.5,
			},
			wantError: false,
		},
		{
			name: "valid zipkin config",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "zipkin",
				Endpoint:    "http://localhost:9411",
				ServiceName: "gibson",
				SampleRate:  0.0,
			},
			wantError: false,
		},
		{
			name: "disabled config always valid",
			config: TracingConfig{
				Enabled:     false,
				Provider:    "invalid",
				Endpoint:    "",
				ServiceName: "",
				SampleRate:  2.0,
			},
			wantError: false,
		},
		{
			name: "invalid provider",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "datadog",
				Endpoint:    "http://localhost:8126",
				ServiceName: "gibson",
				SampleRate:  1.0,
			},
			wantError: true,
			errMsg:    "invalid tracing provider",
		},
		{
			name: "sample rate too low",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "otlp",
				Endpoint:    "http://localhost:4318",
				ServiceName: "gibson",
				SampleRate:  -0.1,
			},
			wantError: true,
			errMsg:    "invalid sample rate",
		},
		{
			name: "sample rate too high",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "otlp",
				Endpoint:    "http://localhost:4318",
				ServiceName: "gibson",
				SampleRate:  1.5,
			},
			wantError: true,
			errMsg:    "invalid sample rate",
		},
		{
			name: "missing endpoint",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "otlp",
				Endpoint:    "",
				ServiceName: "gibson",
				SampleRate:  1.0,
			},
			wantError: true,
			errMsg:    "endpoint is required",
		},
		{
			name: "missing service name",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "otlp",
				Endpoint:    "http://localhost:4318",
				ServiceName: "",
				SampleRate:  1.0,
			},
			wantError: true,
			errMsg:    "service name is required",
		},
		{
			name: "case insensitive provider",
			config: TracingConfig{
				Enabled:     true,
				Provider:    "OTLP",
				Endpoint:    "http://localhost:4318",
				ServiceName: "gibson",
				SampleRate:  1.0,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLangfuseConfig_Validate tests LangfuseConfig validation logic.
func TestLangfuseConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    LangfuseConfig
		wantError bool
		errMsg    string
	}{
		{
			name: "valid config",
			config: LangfuseConfig{
				PublicKey: "pk-lf-1234567890",
				SecretKey: "sk-lf-0987654321",
				Host:      "https://cloud.langfuse.com",
			},
			wantError: false,
		},
		{
			name: "missing public key",
			config: LangfuseConfig{
				PublicKey: "",
				SecretKey: "sk-lf-0987654321",
				Host:      "https://cloud.langfuse.com",
			},
			wantError: true,
			errMsg:    "public key is required",
		},
		{
			name: "missing secret key",
			config: LangfuseConfig{
				PublicKey: "pk-lf-1234567890",
				SecretKey: "",
				Host:      "https://cloud.langfuse.com",
			},
			wantError: true,
			errMsg:    "secret key is required",
		},
		{
			name: "missing host",
			config: LangfuseConfig{
				PublicKey: "pk-lf-1234567890",
				SecretKey: "sk-lf-0987654321",
				Host:      "",
			},
			wantError: true,
			errMsg:    "host is required",
		},
		{
			name: "all fields empty",
			config: LangfuseConfig{
				PublicKey: "",
				SecretKey: "",
				Host:      "",
			},
			wantError: true,
			errMsg:    "public key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMetricsConfig_Validate tests MetricsConfig validation logic.
func TestMetricsConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    MetricsConfig
		wantError bool
		errMsg    string
	}{
		{
			name: "valid prometheus config",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "prometheus",
				Port:     9090,
			},
			wantError: false,
		},
		{
			name: "valid otlp config",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "otlp",
				Port:     4318,
			},
			wantError: false,
		},
		{
			name: "disabled config always valid",
			config: MetricsConfig{
				Enabled:  false,
				Provider: "invalid",
				Port:     0,
			},
			wantError: false,
		},
		{
			name: "invalid provider",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "statsd",
				Port:     8125,
			},
			wantError: true,
			errMsg:    "invalid metrics provider",
		},
		{
			name: "port too low",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "prometheus",
				Port:     0,
			},
			wantError: true,
			errMsg:    "invalid port",
		},
		{
			name: "port too high",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "prometheus",
				Port:     65536,
			},
			wantError: true,
			errMsg:    "invalid port",
		},
		{
			name: "minimum valid port",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "prometheus",
				Port:     1,
			},
			wantError: false,
		},
		{
			name: "maximum valid port",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "prometheus",
				Port:     65535,
			},
			wantError: false,
		},
		{
			name: "case insensitive provider",
			config: MetricsConfig{
				Enabled:  true,
				Provider: "PROMETHEUS",
				Port:     9090,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestLoggingConfig_Validate tests LoggingConfig validation logic.
func TestLoggingConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    LoggingConfig
		wantError bool
		errMsg    string
	}{
		{
			name: "valid json to stdout",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "stdout",
			},
			wantError: false,
		},
		{
			name: "valid text to stderr",
			config: LoggingConfig{
				Level:  "debug",
				Format: "text",
				Output: "stderr",
			},
			wantError: false,
		},
		{
			name: "valid file output",
			config: LoggingConfig{
				Level:  "warn",
				Format: "json",
				Output: "/var/log/gibson.log",
			},
			wantError: false,
		},
		{
			name: "all valid log levels",
			config: LoggingConfig{
				Level:  "error",
				Format: "json",
				Output: "stdout",
			},
			wantError: false,
		},
		{
			name: "fatal level",
			config: LoggingConfig{
				Level:  "fatal",
				Format: "json",
				Output: "stdout",
			},
			wantError: false,
		},
		{
			name: "invalid log level",
			config: LoggingConfig{
				Level:  "trace",
				Format: "json",
				Output: "stdout",
			},
			wantError: true,
			errMsg:    "invalid log level",
		},
		{
			name: "invalid format",
			config: LoggingConfig{
				Level:  "info",
				Format: "xml",
				Output: "stdout",
			},
			wantError: true,
			errMsg:    "invalid log format",
		},
		{
			name: "empty output",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "",
			},
			wantError: true,
			errMsg:    "output is required",
		},
		{
			name: "invalid output (relative path)",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "logs/gibson.log",
			},
			wantError: true,
			errMsg:    "invalid log output",
		},
		{
			name: "case insensitive level",
			config: LoggingConfig{
				Level:  "INFO",
				Format: "json",
				Output: "stdout",
			},
			wantError: false,
		},
		{
			name: "case insensitive format",
			config: LoggingConfig{
				Level:  "info",
				Format: "JSON",
				Output: "stdout",
			},
			wantError: false,
		},
		{
			name: "case insensitive output",
			config: LoggingConfig{
				Level:  "info",
				Format: "json",
				Output: "STDOUT",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTracingConfig_YAMLSerialization tests YAML marshaling and unmarshaling.
func TestTracingConfig_YAMLSerialization(t *testing.T) {
	original := TracingConfig{
		Enabled:     true,
		Provider:    "otlp",
		Endpoint:    "http://localhost:4318",
		ServiceName: "gibson-test",
		SampleRate:  0.75,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify YAML contains expected fields
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "enabled: true")
	assert.Contains(t, yamlStr, "provider: otlp")
	assert.Contains(t, yamlStr, "endpoint: http://localhost:4318")
	assert.Contains(t, yamlStr, "service_name: gibson-test")
	assert.Contains(t, yamlStr, "sample_rate: 0.75")

	// Unmarshal back
	var unmarshaled TracingConfig
	err = yaml.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify fields match
	assert.Equal(t, original.Enabled, unmarshaled.Enabled)
	assert.Equal(t, original.Provider, unmarshaled.Provider)
	assert.Equal(t, original.Endpoint, unmarshaled.Endpoint)
	assert.Equal(t, original.ServiceName, unmarshaled.ServiceName)
	assert.Equal(t, original.SampleRate, unmarshaled.SampleRate)
}

// TestLangfuseConfig_YAMLSerialization tests YAML marshaling and unmarshaling.
func TestLangfuseConfig_YAMLSerialization(t *testing.T) {
	original := LangfuseConfig{
		PublicKey: "pk-lf-test-key",
		SecretKey: "sk-lf-secret-key",
		Host:      "https://cloud.langfuse.com",
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify YAML contains expected fields
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "public_key: pk-lf-test-key")
	assert.Contains(t, yamlStr, "secret_key: sk-lf-secret-key")
	assert.Contains(t, yamlStr, "host: https://cloud.langfuse.com")

	// Unmarshal back
	var unmarshaled LangfuseConfig
	err = yaml.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify fields match
	assert.Equal(t, original.PublicKey, unmarshaled.PublicKey)
	assert.Equal(t, original.SecretKey, unmarshaled.SecretKey)
	assert.Equal(t, original.Host, unmarshaled.Host)
}

// TestMetricsConfig_YAMLSerialization tests YAML marshaling and unmarshaling.
func TestMetricsConfig_YAMLSerialization(t *testing.T) {
	original := MetricsConfig{
		Enabled:  true,
		Provider: "prometheus",
		Port:     9090,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify YAML contains expected fields
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "enabled: true")
	assert.Contains(t, yamlStr, "provider: prometheus")
	assert.Contains(t, yamlStr, "port: 9090")

	// Unmarshal back
	var unmarshaled MetricsConfig
	err = yaml.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify fields match
	assert.Equal(t, original.Enabled, unmarshaled.Enabled)
	assert.Equal(t, original.Provider, unmarshaled.Provider)
	assert.Equal(t, original.Port, unmarshaled.Port)
}

// TestLoggingConfig_YAMLSerialization tests YAML marshaling and unmarshaling.
func TestLoggingConfig_YAMLSerialization(t *testing.T) {
	original := LoggingConfig{
		Level:  "debug",
		Format: "json",
		Output: "/var/log/gibson.log",
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify YAML contains expected fields
	yamlStr := string(data)
	assert.Contains(t, yamlStr, "level: debug")
	assert.Contains(t, yamlStr, "format: json")
	assert.Contains(t, yamlStr, "output: /var/log/gibson.log")

	// Unmarshal back
	var unmarshaled LoggingConfig
	err = yaml.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	// Verify fields match
	assert.Equal(t, original.Level, unmarshaled.Level)
	assert.Equal(t, original.Format, unmarshaled.Format)
	assert.Equal(t, original.Output, unmarshaled.Output)
}

// TestYAMLDeserialization_CompleteConfig tests unmarshaling a complete YAML config.
func TestYAMLDeserialization_CompleteConfig(t *testing.T) {
	yamlContent := `
tracing:
  enabled: true
  provider: otlp
  endpoint: http://localhost:4318
  service_name: gibson
  sample_rate: 0.5

langfuse:
  public_key: pk-lf-abc123
  secret_key: sk-lf-xyz789
  host: https://cloud.langfuse.com

metrics:
  enabled: true
  provider: prometheus
  port: 9090

logging:
  level: info
  format: json
  output: stdout
`

	// Parse as a composite struct
	type ObservabilityConfig struct {
		Tracing  TracingConfig  `yaml:"tracing"`
		Langfuse LangfuseConfig `yaml:"langfuse"`
		Metrics  MetricsConfig  `yaml:"metrics"`
		Logging  LoggingConfig  `yaml:"logging"`
	}

	var config ObservabilityConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)
	require.NoError(t, err)

	// Validate tracing
	assert.True(t, config.Tracing.Enabled)
	assert.Equal(t, "otlp", config.Tracing.Provider)
	assert.Equal(t, "http://localhost:4318", config.Tracing.Endpoint)
	assert.Equal(t, "gibson", config.Tracing.ServiceName)
	assert.Equal(t, 0.5, config.Tracing.SampleRate)
	assert.NoError(t, config.Tracing.Validate())

	// Validate langfuse
	assert.Equal(t, "pk-lf-abc123", config.Langfuse.PublicKey)
	assert.Equal(t, "sk-lf-xyz789", config.Langfuse.SecretKey)
	assert.Equal(t, "https://cloud.langfuse.com", config.Langfuse.Host)
	assert.NoError(t, config.Langfuse.Validate())

	// Validate metrics
	assert.True(t, config.Metrics.Enabled)
	assert.Equal(t, "prometheus", config.Metrics.Provider)
	assert.Equal(t, 9090, config.Metrics.Port)
	assert.NoError(t, config.Metrics.Validate())

	// Validate logging
	assert.Equal(t, "info", config.Logging.Level)
	assert.Equal(t, "json", config.Logging.Format)
	assert.Equal(t, "stdout", config.Logging.Output)
	assert.NoError(t, config.Logging.Validate())
}

// TestYAMLDeserialization_InvalidConfig tests unmarshaling with invalid values.
func TestYAMLDeserialization_InvalidConfig(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		validateFunc func(*testing.T, interface{})
	}{
		{
			name: "invalid tracing provider",
			yaml: `
enabled: true
provider: invalid-provider
endpoint: http://localhost:4318
service_name: gibson
sample_rate: 1.0
`,
			validateFunc: func(t *testing.T, v interface{}) {
				config := v.(*TracingConfig)
				err := config.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid tracing provider")
			},
		},
		{
			name: "invalid sample rate",
			yaml: `
enabled: true
provider: otlp
endpoint: http://localhost:4318
service_name: gibson
sample_rate: 1.5
`,
			validateFunc: func(t *testing.T, v interface{}) {
				config := v.(*TracingConfig)
				err := config.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid sample rate")
			},
		},
		{
			name: "invalid metrics port",
			yaml: `
enabled: true
provider: prometheus
port: 99999
`,
			validateFunc: func(t *testing.T, v interface{}) {
				config := v.(*MetricsConfig)
				err := config.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid port")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine config type based on YAML content
			if strings.Contains(tt.yaml, "sample_rate") {
				var config TracingConfig
				err := yaml.Unmarshal([]byte(tt.yaml), &config)
				require.NoError(t, err)
				tt.validateFunc(t, &config)
			} else if strings.Contains(tt.yaml, "port") {
				var config MetricsConfig
				err := yaml.Unmarshal([]byte(tt.yaml), &config)
				require.NoError(t, err)
				tt.validateFunc(t, &config)
			}
		})
	}
}

// Benchmark validation performance
func BenchmarkTracingConfig_Validate(b *testing.B) {
	config := TracingConfig{
		Enabled:     true,
		Provider:    "otlp",
		Endpoint:    "http://localhost:4318",
		ServiceName: "gibson",
		SampleRate:  1.0,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkMetricsConfig_Validate(b *testing.B) {
	config := MetricsConfig{
		Enabled:  true,
		Provider: "prometheus",
		Port:     9090,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkLoggingConfig_Validate(b *testing.B) {
	config := LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}
