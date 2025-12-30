# Gibson Observability Guide

Gibson provides comprehensive observability through OpenTelemetry integration, including distributed tracing, metrics collection, structured logging, and LLM-specific monitoring via Langfuse.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Tracing](#tracing)
  - [Configuration](#tracing-configuration)
  - [Providers](#tracing-providers)
  - [TracedAgentHarness](#tracedagentharness)
  - [Span Names & Attributes](#span-names--attributes)
- [Metrics](#metrics)
  - [Configuration](#metrics-configuration)
  - [Available Metrics](#available-metrics)
  - [Recording Metrics](#recording-metrics)
- [Logging](#logging)
  - [TracedLogger](#tracedlogger)
  - [Trace Correlation](#trace-correlation)
  - [Sensitive Data Redaction](#sensitive-data-redaction)
- [Langfuse Integration](#langfuse-integration)
- [Cost Tracking](#cost-tracking)
- [Health Monitoring](#health-monitoring)
- [Error Handling](#error-handling)
- [Configuration Reference](#configuration-reference)

---

## Overview

Gibson's observability stack is built on OpenTelemetry standards with extensions for AI/LLM workloads:

| Component | Purpose |
|-----------|---------|
| **Tracing** | Distributed traces for LLM calls, tool executions, agent delegations |
| **Metrics** | Counters, histograms for tokens, latency, costs, findings |
| **Logging** | Structured logs with automatic trace correlation |
| **Langfuse** | LLM-specific observability platform integration |
| **Cost Tracking** | Per-mission and per-agent cost monitoring with thresholds |
| **Health Monitoring** | Component health checks with automatic alerting |

---

## Quick Start

### Basic Setup

```go
import (
    "github.com/zero-day-ai/gibson/internal/observability"
)

// Initialize tracing
tracingCfg := observability.TracingConfig{
    Enabled:     true,
    Provider:    "otlp",
    Endpoint:    "localhost:4317",
    ServiceName: "gibson",
    SampleRate:  1.0, // 100% sampling for dev
}

tp, err := observability.InitTracing(ctx, tracingCfg, nil)
if err != nil {
    log.Fatal(err)
}
defer observability.ShutdownTracing(ctx, tp)

// Initialize metrics
metricsCfg := observability.MetricsConfig{
    Enabled:  true,
    Provider: "prometheus",
    Port:     9090,
}

mp, err := observability.InitMetrics(ctx, metricsCfg)
if err != nil {
    log.Fatal(err)
}
defer mp.Shutdown(ctx)

// Create traced harness (wraps your existing harness)
tracedHarness := observability.NewTracedAgentHarness(
    innerHarness,
    observability.WithTracer(tp.Tracer("gibson")),
    observability.WithMetrics(observability.NewOpenTelemetryMetricsRecorder(mp.Meter("gibson"))),
)

// All operations are now automatically traced!
resp, err := tracedHarness.Complete(ctx, "primary", messages)
```

### YAML Configuration

```yaml
logging:
  level: info        # debug, info, warn, error
  format: json       # json or text
  output: stdout     # stdout, stderr, or file path

tracing:
  enabled: true
  provider: otlp     # otlp, jaeger, langfuse, noop
  endpoint: "localhost:4317"
  service_name: gibson
  sample_rate: 0.1   # 10% sampling for production

metrics:
  enabled: true
  provider: prometheus  # prometheus or otlp
  port: 9090

langfuse:
  public_key: "pk-lf-..."
  secret_key: "sk-lf-..."
  host: "https://cloud.langfuse.com"
```

---

## Tracing

### Tracing Configuration

```go
type TracingConfig struct {
    Enabled     bool    `yaml:"enabled"`
    Provider    string  `yaml:"provider"`      // otlp, jaeger, langfuse, noop
    Endpoint    string  `yaml:"endpoint"`      // e.g., localhost:4317
    ServiceName string  `yaml:"service_name"`  // e.g., gibson
    SampleRate  float64 `yaml:"sample_rate"`   // 0.0 to 1.0
}
```

### Tracing Providers

| Provider | Use Case | Endpoint Format |
|----------|----------|-----------------|
| `otlp` | Production standard, OpenTelemetry Collector | `host:4317` (gRPC) |
| `jaeger` | Direct Jaeger integration | `http://host:14268/api/traces` |
| `langfuse` | LLM-specific observability | Configured via LangfuseConfig |
| `noop` | Testing, disabled tracing | N/A |

### Initialization Options

```go
tp, err := observability.InitTracing(ctx, cfg, langfuseCfg,
    // Custom sampler
    observability.WithSampler(sdktrace.AlwaysSample()),

    // Custom resource attributes
    observability.WithResource(resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceNameKey.String("my-service"),
    )),

    // Batch export timeout (default: 5s)
    observability.WithBatchTimeout(10 * time.Second),
)
```

### TracedAgentHarness

The `TracedAgentHarness` wraps any `AgentHarness` implementation to automatically instrument all operations:

```go
tracedHarness := observability.NewTracedAgentHarness(
    innerHarness,
    observability.WithTracer(tracer),
    observability.WithMetrics(recorder),
    observability.WithLogger(logger),
    observability.WithPromptCapture(true), // Capture full prompts (default: true)
)
```

**Automatically Traced Operations:**

| Method | Span Name | Description |
|--------|-----------|-------------|
| `Complete()` | `gen_ai.chat` | Synchronous LLM completion |
| `CompleteWithTools()` | `gen_ai.chat` | LLM completion with tool definitions |
| `Stream()` | `gen_ai.chat.stream` | Streaming LLM completion |
| `CallTool()` | `gen_ai.tool` | Tool/function execution |
| `QueryPlugin()` | `gibson.plugin.query` | Plugin method invocation |
| `DelegateToAgent()` | `gibson.agent.delegate` | Agent-to-agent delegation |
| `SubmitFinding()` | `gibson.finding.submit` | Security finding submission |

**Automatic Instrumentation Includes:**
- Span creation with proper parent-child relationships
- GenAI semantic convention attributes
- Token usage tracking
- Latency measurement
- Error recording with stack traces
- Turn counter increment
- Cost calculation (when configured)

### Span Names & Attributes

#### GenAI Semantic Conventions (OpenTelemetry Standard)

**Span Names:**
```go
gen_ai.chat           // Synchronous chat completion
gen_ai.chat.stream    // Streaming chat completion
gen_ai.tool           // Tool/function call
gen_ai.embeddings     // Embedding generation
```

**Standard Attributes:**
```go
gen_ai.system              // Provider (anthropic, openai, etc.)
gen_ai.request.model       // Requested model name
gen_ai.request.temperature // Temperature parameter
gen_ai.request.max_tokens  // Max tokens requested
gen_ai.request.top_p       // Top-P sampling parameter
gen_ai.response.model      // Model that generated response
gen_ai.response.finish_reason // Why model stopped
gen_ai.usage.input_tokens  // Prompt tokens
gen_ai.usage.output_tokens // Completion tokens
gen_ai.prompt              // Full prompt text (when enabled)
gen_ai.completion          // Full response text
```

#### Gibson-Specific Spans

```go
gibson.agent.delegate   // Agent delegation
gibson.finding.submit   // Finding submission
gibson.plugin.query     // Plugin method call
gibson.memory.get       // Memory retrieval
gibson.memory.set       // Memory storage
gibson.memory.search    // Memory search
```

#### Gibson-Specific Attributes

**Agent/Mission Context:**
```go
gibson.agent.name       // Agent identifier
gibson.agent.version    // Agent version
gibson.mission.id       // Mission UUID
gibson.mission.name     // Mission name
gibson.mission.phase    // Current phase
gibson.turn.number      // Turn counter (auto-incremented)
```

**Operations:**
```go
gibson.tool.name              // Tool identifier
gibson.plugin.name            // Plugin name
gibson.plugin.method          // Plugin method
gibson.delegation.target_agent // Target agent
gibson.delegation.task_id     // Task UUID
```

**Findings:**
```go
gibson.finding.id         // Finding UUID
gibson.finding.severity   // critical, high, medium, low, info
gibson.finding.category   // sqli, xss, rce, etc.
gibson.finding.target_id  // Target UUID
gibson.finding.confidence // 0.0 to 1.0
```

**Cost & Metrics:**
```go
gibson.llm.cost           // Cost in USD
gibson.llm.slot           // LLM slot name
gibson.metrics.llm_calls  // Total LLM calls
gibson.metrics.tool_calls // Total tool calls
gibson.metrics.tokens_used // Total tokens
gibson.metrics.findings_count // Findings submitted
```

#### Attribute Helper Functions

```go
// Mission context
attrs := observability.MissionAttributes(missionCtx)

// Agent info
attrs := observability.AgentAttributes("recon", "1.0.0")

// Finding details
attrs := observability.FindingAttributes(finding)

// Tool call
attrs := observability.ToolAttributes("nmap_scan")

// Delegation
attrs := observability.DelegationAttributes("exploit_agent", taskID)

// Builder pattern
attrs := observability.NewAttributeSet().
    AddString("gibson.agent.name", "recon").
    AddInt("gibson.turn.number", 5).
    AddFloat64("gibson.llm.cost", 0.015).
    Build()
```

---

## Metrics

### Metrics Configuration

```go
type MetricsConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Provider string `yaml:"provider"`  // prometheus or otlp
    Port     int    `yaml:"port"`      // 1-65535
}
```

**Providers:**
- `prometheus`: Exposes HTTP endpoint for scraping at `/metrics`
- `otlp`: Pushes metrics to OpenTelemetry Collector via gRPC

### Available Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gibson.llm.completions` | Counter | slot, provider, model, status | LLM completion count |
| `gibson.llm.tokens.input` | Counter | slot, provider, model, status | Input tokens consumed |
| `gibson.llm.tokens.output` | Counter | slot, provider, model, status | Output tokens generated |
| `gibson.llm.latency` | Histogram | slot, provider, model, status | Completion latency (ms) |
| `gibson.llm.cost` | Counter | slot, provider, model | Cost in USD |
| `gibson.tool.calls` | Counter | tool, status | Tool execution count |
| `gibson.tool.duration` | Histogram | tool, status | Tool duration (ms) |
| `gibson.findings.submitted` | Counter | severity, category | Findings submitted |
| `gibson.agent.delegations` | Counter | source_agent, target_agent, status | Agent delegations |
| `gibson.health.status` | Gauge | component, state | Health status (1.0/0.0) |

### Recording Metrics

```go
recorder := observability.NewOpenTelemetryMetricsRecorder(meter)

// Generic recording
recorder.RecordCounter("gibson.custom.count", 1, map[string]string{
    "type": "example",
})
recorder.RecordHistogram("gibson.custom.latency", 125.5, labels)
recorder.RecordGauge("gibson.custom.gauge", 42.0, labels)

// Convenience methods
recorder.RecordLLMCompletion(
    "primary",      // slot
    "anthropic",    // provider
    "claude-3-opus", // model
    "success",      // status
    1000,           // input tokens
    500,            // output tokens
    1250.0,         // latency ms
    0.045,          // cost USD
)

recorder.RecordToolCall("nmap_scan", "success", 5000.0)
recorder.RecordFindingSubmitted("high", "sqli")
recorder.RecordAgentDelegation("recon", "exploit", "success")
```

---

## Logging

### TracedLogger

The `TracedLogger` provides structured logging with automatic trace correlation:

```go
handler := observability.NewJSONHandler(os.Stdout, slog.LevelInfo)
logger := observability.NewTracedLogger(handler, missionID, "recon_agent")

// Logs automatically include trace_id, span_id, mission_id, agent_name
logger.Info(ctx, "Starting reconnaissance",
    slog.String("target", "example.com"),
    slog.Int("port", 443),
)

// Output:
// {
//   "time": "2024-01-15T10:30:00Z",
//   "level": "INFO",
//   "msg": "Starting reconnaissance",
//   "trace_id": "abc123...",
//   "span_id": "def456...",
//   "mission_id": "mission-uuid",
//   "agent_name": "recon_agent",
//   "target": "example.com",
//   "port": 443
// }
```

### Trace Correlation

All logs automatically include OpenTelemetry trace context when available:

```go
// Extract trace context from span
ctx, span := tracer.Start(ctx, "my-operation")
defer span.End()

// Logger extracts trace_id and span_id from context
logger.Info(ctx, "Operation started")
```

### Sensitive Data Redaction

At `Info`, `Warn`, and `Error` levels, sensitive fields are automatically redacted:

**Redacted Fields:**
- `prompt`, `prompts`
- `api_key`, `apikey`
- `secret`, `secretkey`
- `password`
- `token`
- `credential`

```go
// At Info level, sensitive data is redacted
logger.Info(ctx, "API call",
    slog.String("api_key", "sk-abc123"),  // Logged as "[REDACTED]"
    slog.String("prompt", "hack the planet"), // Logged as "[REDACTED]"
)

// At Debug level, no redaction (use only in development)
logger.Debug(ctx, "Debug info",
    slog.String("prompt", "hack the planet"), // Logged as-is
)
```

---

## Langfuse Integration

[Langfuse](https://langfuse.com) provides LLM-specific observability with prompt management, evaluation, and cost tracking.

### Configuration

```go
langfuseCfg := &observability.LangfuseConfig{
    PublicKey: "pk-lf-...",
    SecretKey: "sk-lf-...",
    Host:      "https://cloud.langfuse.com",
}

tp, err := observability.InitTracing(ctx, tracingCfg, langfuseCfg)
```

Or use Langfuse as the primary provider:

```go
tracingCfg := observability.TracingConfig{
    Enabled:     true,
    Provider:    "langfuse",
    ServiceName: "gibson",
    SampleRate:  1.0,
}
```

### Langfuse Exporter Options

```go
exporter, err := observability.NewLangfuseExporter(langfuseCfg,
    // Batch size before flush (default: 100)
    observability.WithBatchSize(50),

    // Flush interval (default: 5s)
    observability.WithFlushInterval(3 * time.Second),

    // Retry policy (default: 3 retries, 1s delay)
    observability.WithRetryPolicy(5, 2*time.Second),
)

// Set session/user for filtering in Langfuse UI
exporter.SetSession("session-123")
exporter.SetUser("user-456")
```

### Span Mapping

OpenTelemetry spans are converted to Langfuse observations:

| OTel Span Kind | Langfuse Type |
|----------------|---------------|
| `SpanKindClient` | GENERATION |
| `SpanKindServer` | SPAN |
| `SpanKindInternal` | SPAN |

---

## Cost Tracking

Track LLM costs per mission and agent with configurable thresholds:

```go
costTracker := observability.NewCostTracker(tokenTracker, logger)

// Set cost threshold for a mission
costTracker.SetThreshold(missionID, 100.0) // $100 limit

// Calculate cost for a completion
cost := costTracker.CalculateCost(
    "anthropic",     // provider
    "claude-3-opus", // model
    1000,            // input tokens
    500,             // output tokens
)

// Record cost on span
costTracker.RecordCostOnSpan(span, cost)

// Check threshold
if costTracker.CheckThreshold(missionID, totalCost) {
    // Threshold exceeded - logs warning with overage percentage
}

// Get costs
missionCost := costTracker.GetMissionCost(missionID)
agentCost := costTracker.GetAgentCost(missionID, "recon_agent")
```

---

## Health Monitoring

Monitor component health with automatic metric emission:

```go
monitor := observability.NewHealthMonitor(metricsRecorder, logger)

// Register components
monitor.Register("database", dbHealthChecker)
monitor.Register("llm_provider", llmHealthChecker)
monitor.Register("tool_executor", toolHealthChecker)

// Start periodic health checks (background goroutine)
go monitor.StartPeriodicCheck(ctx, 30*time.Second)

// Manual health check
status, err := monitor.Check(ctx, "database")
if status.State != observability.HealthStateHealthy {
    log.Warn("Database unhealthy", "message", status.Message)
}

// Check all components
allStatuses, err := monitor.CheckAll(ctx)
```

### Health States

| State | Description |
|-------|-------------|
| `HealthStateHealthy` | Component operating normally |
| `HealthStateDegraded` | Operational but with reduced performance |
| `HealthStateUnhealthy` | Component not operational |

### Implementing HealthChecker

```go
type DatabaseHealthChecker struct {
    db *sql.DB
}

func (c *DatabaseHealthChecker) Health(ctx context.Context) types.HealthStatus {
    if err := c.db.PingContext(ctx); err != nil {
        return types.HealthStatus{
            State:   observability.HealthStateUnhealthy,
            Message: fmt.Sprintf("ping failed: %v", err),
        }
    }
    return types.HealthStatus{
        State:   observability.HealthStateHealthy,
        Message: "connected",
    }
}
```

---

## Error Handling

Structured observability errors with retry information:

```go
// Error types
observability.ErrExporterConnection     // Retryable - connection failed
observability.ErrAuthenticationFailed   // Non-retryable - bad credentials
observability.ErrSpanContextMissing     // Non-retryable - no span in context
observability.ErrMetricsRegistration    // Non-retryable - metric setup failed
observability.ErrBufferOverflow         // Retryable - buffer full
observability.ErrShutdownTimeout        // Non-retryable - shutdown timed out

// Error constructors
err := observability.NewExporterConnectionError("localhost:4317", cause)
err := observability.NewAuthenticationError("langfuse", cause)
err := observability.NewShutdownTimeoutError("tracer")

// Check if retryable
if obsErr, ok := err.(*observability.ObservabilityError); ok {
    if obsErr.Retryable {
        // Implement retry logic
    }
}
```

---

## Configuration Reference

### Complete YAML Example

```yaml
logging:
  level: info           # debug, info, warn, error
  format: json          # json, text
  output: stdout        # stdout, stderr, /path/to/file

tracing:
  enabled: true
  provider: otlp        # otlp, jaeger, langfuse, noop
  endpoint: "otel-collector:4317"
  service_name: gibson
  sample_rate: 0.1      # 10% sampling

metrics:
  enabled: true
  provider: prometheus  # prometheus, otlp
  port: 9090

langfuse:
  public_key: "pk-lf-..."
  secret_key: "sk-lf-..."
  host: "https://cloud.langfuse.com"
```

### Environment Variables

While Gibson primarily uses YAML configuration, you can use environment variable expansion in your config:

```yaml
langfuse:
  public_key: ${LANGFUSE_PUBLIC_KEY}
  secret_key: ${LANGFUSE_SECRET_KEY}
  host: ${LANGFUSE_HOST:-https://cloud.langfuse.com}
```

### Docker Compose Stack

Example observability stack:

```yaml
version: '3.8'
services:
  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    command: ["--config=/etc/otel/config.yaml"]
    volumes:
      - ./otel-config.yaml:/etc/otel/config.yaml
    ports:
      - "4317:4317"   # OTLP gRPC
      - "4318:4318"   # OTLP HTTP
      - "8888:8888"   # Prometheus metrics

  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686" # UI
      - "14268:14268" # Collector

  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yaml:/etc/prometheus/prometheus.yml
    ports:
      - "9091:9090"

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
```

### OpenTelemetry Collector Config

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  batch:
    timeout: 5s
    send_batch_size: 1000

exporters:
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true
  prometheus:
    endpoint: "0.0.0.0:8888"

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [jaeger]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
```

---

## Best Practices

### Sampling Strategy

| Environment | Sample Rate | Rationale |
|-------------|-------------|-----------|
| Development | 1.0 (100%) | Full visibility for debugging |
| Staging | 0.5 (50%) | Balance visibility and overhead |
| Production | 0.1 (10%) | Reduce storage/cost, statistical sampling |

### Prompt Capture

- **Development**: Enable `WithPromptCapture(true)` for debugging
- **Production**: Consider disabling if handling sensitive user data
- **Compliance**: Check data retention policies before enabling

### Cost Monitoring

1. Set conservative thresholds initially
2. Monitor `gibson.llm.cost` metric
3. Alert on threshold breaches
4. Review high-cost missions for optimization

### Log Levels

| Level | Use Case |
|-------|----------|
| `debug` | Development, troubleshooting |
| `info` | Normal operations, audit trail |
| `warn` | Recoverable issues, degraded performance |
| `error` | Failures requiring attention |

---

## Troubleshooting

### No Traces Appearing

1. Check `tracing.enabled: true` in config
2. Verify endpoint connectivity: `nc -zv <host> 4317`
3. Check sample rate > 0
4. Verify collector is receiving: check collector logs

### Missing Metrics

1. Check `metrics.enabled: true`
2. For Prometheus: verify scrape target in prometheus.yml
3. Check port availability: `lsof -i :<port>`
4. Verify metric registration: check for `ErrMetricsRegistration` errors

### Langfuse Not Receiving Data

1. Verify API keys are correct
2. Check network connectivity to Langfuse host
3. Review batch size and flush interval settings
4. Check for `ErrAuthenticationFailed` errors

### High Memory Usage

1. Reduce batch sizes
2. Increase flush intervals
3. Lower sample rate
4. Check for span/metric accumulation (shutdown not called)
