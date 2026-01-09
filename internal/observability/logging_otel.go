package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/pkg/version"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"
)

const (
	defaultLogBatchTimeout = 5 * time.Second
)

// OTelLoggingConfig configures OpenTelemetry Logs API integration.
// Supports OTLP exporters (gRPC and HTTP) and stdout for CLI verbose output.
type OTelLoggingConfig struct {
	Enabled        bool   // Enable OTel logging
	Endpoint       string // OTLP endpoint (e.g., "localhost:4317")
	Protocol       string // "grpc" or "http"
	Insecure       bool   // Use insecure connection for local development
	StdoutEnabled  bool   // Enable stdout exporter for CLI verbose output
	ServiceName    string // Service name for resource attributes
	ServiceVersion string // Service version for resource attributes
	TLSCertFile    string // Client TLS certificate file (optional)
	TLSKeyFile     string // Client TLS key file (optional)
}

// Validate validates the OTelLoggingConfig fields.
func (c *OTelLoggingConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Validate protocol
	if c.Protocol != "" {
		protocol := strings.ToLower(c.Protocol)
		if protocol != "grpc" && protocol != "http" {
			return fmt.Errorf("invalid protocol: %s (must be 'grpc' or 'http')", c.Protocol)
		}
	}

	// Validate endpoint is not empty when OTLP is configured
	if c.Endpoint != "" && c.Protocol == "" {
		return fmt.Errorf("protocol is required when endpoint is specified")
	}

	// Validate TLS configuration
	if c.TLSCertFile != "" && c.TLSKeyFile == "" {
		return fmt.Errorf("tls_key_file is required when tls_cert_file is specified")
	}
	if c.TLSKeyFile != "" && c.TLSCertFile == "" {
		return fmt.Errorf("tls_cert_file is required when tls_key_file is specified")
	}

	return nil
}

// LoggingShutdown is a function that shuts down the logging provider.
// It should be called before application exit to ensure all logs are flushed.
type LoggingShutdown func(context.Context) error

// InitLogging initializes OpenTelemetry Logs API with the specified configuration.
// It mirrors the pattern from InitTracing for consistency.
//
// Parameters:
//   - ctx: Context for initialization and potential cancellation
//   - cfg: Logging configuration including endpoint, protocol, and options
//
// Returns:
//   - *LoggingShutdown: Shutdown function for graceful cleanup
//   - error: Any error encountered during initialization
//
// When cfg.Enabled is false, returns a no-op shutdown function.
func InitLogging(ctx context.Context, cfg OTelLoggingConfig) (*LoggingShutdown, error) {
	// Return no-op shutdown if logging is disabled
	if !cfg.Enabled {
		noopShutdown := LoggingShutdown(func(context.Context) error { return nil })
		return &noopShutdown, nil
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, WrapObservabilityError(ErrExporterConnection, "invalid logging configuration", err)
	}

	// Create resource attributes
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	serviceVersion := cfg.ServiceVersion
	if serviceVersion == "" {
		serviceVersion = version.Version
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
		resource.WithFromEnv(),      // Include environment variables
		resource.WithTelemetrySDK(), // Include SDK info
	)
	if err != nil {
		return nil, WrapObservabilityError(ErrExporterConnection, "failed to create resource", err)
	}

	// Create log provider with exporters
	var exporters []sdklog.Exporter

	// Add OTLP exporter if configured
	if cfg.Endpoint != "" && cfg.Protocol != "" {
		var otlpExporter sdklog.Exporter
		var otlpErr error

		protocol := strings.ToLower(cfg.Protocol)
		switch protocol {
		case "grpc":
			otlpExporter, otlpErr = createOTLPGRPCExporter(ctx, cfg)
		case "http":
			otlpExporter, otlpErr = createOTLPHTTPExporter(ctx, cfg)
		default:
			return nil, NewObservabilityError(ErrExporterConnection, fmt.Sprintf("unsupported protocol: %s", cfg.Protocol))
		}

		if otlpErr != nil {
			return nil, NewExporterConnectionError(cfg.Endpoint, otlpErr)
		}
		exporters = append(exporters, otlpExporter)
	}

	// Add stdout exporter if configured (for CLI verbose output)
	if cfg.StdoutEnabled {
		stdoutExporter, err := stdoutlog.New(
			stdoutlog.WithPrettyPrint(),
		)
		if err != nil {
			return nil, WrapObservabilityError(ErrExporterConnection, "failed to create stdout exporter", err)
		}
		exporters = append(exporters, stdoutExporter)
	}

	// Require at least one exporter
	if len(exporters) == 0 {
		return nil, NewObservabilityError(ErrExporterConnection, "at least one exporter must be configured (endpoint or stdout)")
	}

	// Create logger provider with batch processors
	var opts []sdklog.LoggerProviderOption
	opts = append(opts, sdklog.WithResource(res))

	// Add a processor for each exporter
	for _, exporter := range exporters {
		processor := sdklog.NewBatchProcessor(exporter,
			sdklog.WithExportTimeout(defaultLogBatchTimeout),
		)
		opts = append(opts, sdklog.WithProcessor(processor))
	}

	// Create logger provider
	loggerProvider := sdklog.NewLoggerProvider(opts...)

	// Set as global logger provider
	global.SetLoggerProvider(loggerProvider)

	// Return shutdown function
	shutdown := LoggingShutdown(func(ctx context.Context) error {
		if err := loggerProvider.Shutdown(ctx); err != nil {
			return WrapObservabilityError(ErrShutdownTimeout, "failed to shutdown logger provider", err)
		}
		return nil
	})

	return &shutdown, nil
}

// createOTLPGRPCExporter creates an OTLP gRPC log exporter with the specified configuration.
func createOTLPGRPCExporter(ctx context.Context, cfg OTelLoggingConfig) (sdklog.Exporter, error) {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
	}

	// Configure TLS
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		// Use TLS with client certificate
		creds, err := credentials.NewClientTLSFromFile(cfg.TLSCertFile, "")
		if err != nil {
			return nil, WrapObservabilityError(ErrExporterConnection,
				"failed to load TLS credentials", err)
		}
		opts = append(opts, otlploggrpc.WithTLSCredentials(creds))
	} else if cfg.Insecure {
		// Use insecure connection (only if explicitly opted in)
		opts = append(opts, otlploggrpc.WithInsecure())
	} else {
		// Default: Use system TLS (no client cert, but verify server)
		creds := credentials.NewTLS(nil)
		opts = append(opts, otlploggrpc.WithTLSCredentials(creds))
	}

	return otlploggrpc.New(ctx, opts...)
}

// createOTLPHTTPExporter creates an OTLP HTTP log exporter with the specified configuration.
func createOTLPHTTPExporter(ctx context.Context, cfg OTelLoggingConfig) (sdklog.Exporter, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(cfg.Endpoint),
	}

	// Configure TLS
	if cfg.Insecure {
		opts = append(opts, otlploghttp.WithInsecure())
	}
	// Note: OTLP HTTP exporter doesn't support custom TLS credentials in the same way as gRPC

	return otlploghttp.New(ctx, opts...)
}

// GibsonLogger wraps the OpenTelemetry log.Logger with mission context and sensitive field redaction.
// It automatically includes mission_id, agent_name, trace_id, and span_id in all log records.
type GibsonLogger struct {
	logger    log.Logger
	missionID string
	agentName string
}

// NewGibsonLogger creates a new GibsonLogger with mission context extracted from ctx.
// The logger automatically correlates logs with traces and includes mission context.
//
// Parameters:
//   - ctx: Context containing mission context and trace context
//   - logger: The OpenTelemetry log.Logger to wrap
//
// Returns:
//   - *GibsonLogger: A configured logger ready for use
func NewGibsonLogger(ctx context.Context, logger log.Logger) *GibsonLogger {
	// Extract mission context from ctx (using well-known context keys)
	// These would be set by the mission orchestrator
	missionID := ""
	agentName := ""

	// Try to extract mission_id from context
	if val := ctx.Value("mission_id"); val != nil {
		if id, ok := val.(string); ok {
			missionID = id
		}
	}

	// Try to extract agent_name from context
	if val := ctx.Value("agent_name"); val != nil {
		if name, ok := val.(string); ok {
			agentName = name
		}
	}

	return &GibsonLogger{
		logger:    logger,
		missionID: missionID,
		agentName: agentName,
	}
}

// Debug emits a debug-level log record with automatic trace correlation and mission context.
// Debug logs include all fields without redaction.
func (l *GibsonLogger) Debug(ctx context.Context, msg string, attrs ...log.KeyValue) {
	l.emit(ctx, log.SeverityDebug, msg, attrs...)
}

// Info emits an info-level log record with automatic trace correlation and mission context.
// Sensitive data in attrs is redacted at info level and above.
func (l *GibsonLogger) Info(ctx context.Context, msg string, attrs ...log.KeyValue) {
	attrs = l.redactSensitiveFields(attrs)
	l.emit(ctx, log.SeverityInfo, msg, attrs...)
}

// Warn emits a warning-level log record with automatic trace correlation and mission context.
// Sensitive data in attrs is redacted at warn level and above.
func (l *GibsonLogger) Warn(ctx context.Context, msg string, attrs ...log.KeyValue) {
	attrs = l.redactSensitiveFields(attrs)
	l.emit(ctx, log.SeverityWarn, msg, attrs...)
}

// Error emits an error-level log record with automatic trace correlation and mission context.
// Sensitive data in attrs is redacted at error level.
func (l *GibsonLogger) Error(ctx context.Context, msg string, attrs ...log.KeyValue) {
	attrs = l.redactSensitiveFields(attrs)
	l.emit(ctx, log.SeverityError, msg, attrs...)
}

// emit is the internal method that emits a log record with all context fields.
func (l *GibsonLogger) emit(ctx context.Context, severity log.Severity, msg string, attrs ...log.KeyValue) {
	// Build record with context
	record := log.Record{}
	record.SetTimestamp(time.Now())
	record.SetSeverity(severity)
	record.SetBody(log.StringValue(msg))

	// Add mission context
	if l.missionID != "" {
		record.AddAttributes(log.String("mission_id", l.missionID))
	}
	if l.agentName != "" {
		record.AddAttributes(log.String("agent_name", l.agentName))
	}

	// Add trace context from OpenTelemetry
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		record.AddAttributes(
			log.String("trace_id", spanCtx.TraceID().String()),
			log.String("span_id", spanCtx.SpanID().String()),
		)
	}

	// Add user-provided attributes
	record.AddAttributes(attrs...)

	// Emit the record
	l.logger.Emit(ctx, record)
}

// redactSensitiveFields redacts sensitive field values BEFORE log emission.
// This ensures sensitive data never appears in exported logs.
//
// Sensitive fields: prompt, prompts, api_key, apikey, secret, secretkey, password, token, credential
func (l *GibsonLogger) redactSensitiveFields(attrs []log.KeyValue) []log.KeyValue {
	// List of sensitive field names to redact
	sensitiveFields := map[string]bool{
		"prompt":     true,
		"prompts":    true,
		"api_key":    true,
		"apikey":     true,
		"secret":     true,
		"secretkey":  true,
		"password":   true,
		"token":      true,
		"credential": true,
	}

	redacted := make([]log.KeyValue, len(attrs))
	for i, attr := range attrs {
		// Normalize key for comparison
		normalizedKey := strings.ToLower(strings.ReplaceAll(attr.Key, "_", ""))

		// Redact if sensitive
		if sensitiveFields[normalizedKey] {
			redacted[i] = log.String(attr.Key, "[REDACTED]")
		} else {
			redacted[i] = attr
		}
	}

	return redacted
}

// GetLogger creates a new logger from the global logger provider.
// This is a convenience function for getting a logger instance.
func GetLogger(name string) log.Logger {
	return global.GetLoggerProvider().Logger(
		name,
		log.WithInstrumentationVersion(version.Version),
	)
}
