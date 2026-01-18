# Gibson Configuration Reference

This document provides a comprehensive reference for all Gibson configuration options including the config file (`config.yaml`), environment variables, and command-line flags.

## Table of Contents

- [Configuration File Location](#configuration-file-location)
- [Environment Variables](#environment-variables)
- [Global Command-Line Flags](#global-command-line-flags)
- [Core Configuration](#core-configuration)
- [Database Configuration](#database-configuration)
- [Security Configuration](#security-configuration)
- [LLM Provider Configuration](#llm-provider-configuration)
- [Memory Configuration](#memory-configuration)
- [GraphRAG Configuration](#graphrag-configuration)
- [Registry Configuration](#registry-configuration)
- [Callback Server Configuration](#callback-server-configuration)
- [Registration Server Configuration](#registration-server-configuration)
- [Daemon Configuration](#daemon-configuration)
- [Logging Configuration](#logging-configuration)
- [Tracing Configuration](#tracing-configuration)
- [Metrics Configuration](#metrics-configuration)
- [Langfuse Configuration](#langfuse-configuration)
- [Plugins Configuration](#plugins-configuration)
- [Command-Specific Flags](#command-specific-flags)
- [Complete Example Configuration](#complete-example-configuration)

---

## Configuration File Location

Gibson looks for configuration in the following locations (in order):

1. Path specified by `--config` flag
2. `$GIBSON_HOME/config.yaml`
3. `~/.gibson/config.yaml`

### Directory Structure

After running `gibson init`, the following structure is created:

```
~/.gibson/
├── config.yaml              # Main configuration file
├── master.key               # Encryption key for credentials
├── gibson.db                # SQLite database
├── data/                    # Data storage directory
├── cache/                   # Cache directory
├── etcd-data/               # Embedded etcd data (if using embedded registry)
├── components/              # Installed agents, tools, plugins
│   ├── agents/
│   ├── tools/
│   └── plugins/
└── logs/                    # Log files
```

---

## Environment Variables

### Core Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GIBSON_HOME` | string | `~/.gibson` | Override Gibson home directory |
| `GIBSON_VERBOSE` | flag | - | Enable verbose mode if set (any value) |
| `GIBSON_RUN_MODE` | string | `production` | Run mode: `production`, `development`, `test` |

### Callback Server

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GIBSON_CALLBACK_LISTEN_ADDRESS` | string | `0.0.0.0:50001` | Callback server listen address |
| `GIBSON_CALLBACK_ADVERTISE_ADDR` | string | - | Address advertised to agents (if different from listen) |

### GraphRAG / Neo4j

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GIBSON_NEO4J_URI` | string | - | Neo4j connection URI |
| `NEO4J_URI` | string | - | Neo4j URI (fallback) |
| `NEO4J_USER` | string | - | Neo4j username |
| `NEO4J_PASSWORD` | string | - | Neo4j password |

### LLM Providers

| Variable | Type | Description |
|----------|------|-------------|
| `ANTHROPIC_API_KEY` | string | Anthropic API key |
| `ANTHROPIC_MODEL` | string | Override default Anthropic model |
| `OPENAI_API_KEY` | string | OpenAI API key |
| `OPENAI_MODEL` | string | Override default OpenAI model |
| `GOOGLE_API_KEY` | string | Google AI API key |
| `GOOGLE_MODEL` | string | Override default Google model |
| `OLLAMA_URL` | string | Ollama server URL (default: `http://localhost:11434`) |
| `OLLAMA_MODEL` | string | Override default Ollama model |

### Testing

| Variable | Type | Description |
|----------|------|-------------|
| `GIBSON_TEST_LLMS` | flag | Enable LLM provider tests |

### Environment Variable Interpolation

Configuration values support `${VAR_NAME}` syntax for environment variable interpolation:

```yaml
llm:
  providers:
    anthropic:
      api_key: ${ANTHROPIC_API_KEY}
```

---

## Global Command-Line Flags

These flags are available on all Gibson commands:

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--config` | | string | `$GIBSON_HOME/config.yaml` | Path to config file |
| `--home` | | string | `~/.gibson` | Gibson home directory |
| `--verbose` | `-v` | bool | false | Enable verbose output |
| `--very-verbose` | | bool | false | Enable detailed verbose output |
| `--debug-verbose` | | bool | false | Enable debug-level verbose output |
| `--quiet` | `-q` | bool | false | Suppress non-essential output |
| `--output` | `-o` | string | `text` | Output format: `text`, `json` |
| `--taxonomy-path` | | string | - | Path to custom taxonomy YAML (extends embedded) |
| `--validate-taxonomy` | | bool | false | Validate taxonomy and exit |

### Verbosity Levels

Gibson has three verbosity levels for progressively detailed output:

1. **`--verbose` / `-v`**: Major operations (LLM requests, tool calls, agent lifecycle)
2. **`--very-verbose`**: Detailed operation data (token counts, durations, parameters)
3. **`--debug-verbose`**: All internal events (memory operations, component health)

---

## Core Configuration

```yaml
core:
  home_dir: ~/.gibson
  data_dir: ~/.gibson/data
  cache_dir: ~/.gibson/cache
  parallel_limit: 10
  timeout: 5m
  debug: false
```

| Key | Type | Default | Validation | Description |
|-----|------|---------|------------|-------------|
| `home_dir` | string | `~/.gibson` | Required | Root directory for Gibson installation |
| `data_dir` | string | `$home_dir/data` | Required | Data storage directory |
| `cache_dir` | string | `$home_dir/cache` | Required | Cache directory |
| `parallel_limit` | int | `10` | 1-100 | Maximum concurrent operations |
| `timeout` | duration | `5m` | ≥1s | Default operation timeout |
| `debug` | bool | `false` | | Enable debug mode |

---

## Database Configuration

Gibson uses SQLite for persistent storage with FTS5 full-text search.

```yaml
database:
  path: ~/.gibson/gibson.db
  max_connections: 10
  timeout: 30s
  wal_mode: true
  auto_vacuum: true
```

| Key | Type | Default | Validation | Description |
|-----|------|---------|------------|-------------|
| `path` | string | `$GIBSON_HOME/gibson.db` | Required | SQLite database file path |
| `max_connections` | int | `10` | 1-100 | Maximum connection pool size |
| `timeout` | duration | `30s` | ≥1s | Connection timeout |
| `wal_mode` | bool | `true` | | Enable WAL (Write-Ahead Logging) for concurrency |
| `auto_vacuum` | bool | `true` | | Enable automatic database vacuuming |

---

## Security Configuration

```yaml
security:
  encryption_algorithm: aes-256-gcm
  key_derivation: scrypt
  ssl_validation: true
  audit_logging: true
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `encryption_algorithm` | string | `aes-256-gcm` | Encryption algorithm for credentials |
| `key_derivation` | string | `scrypt` | Key derivation function |
| `ssl_validation` | bool | `true` | Validate TLS certificates |
| `audit_logging` | bool | `true` | Enable audit logging of security events |

---

## LLM Provider Configuration

Gibson supports multiple LLM providers with slot-based routing.

```yaml
llm:
  default_provider: anthropic

  providers:
    anthropic:
      type: anthropic
      api_key: ${ANTHROPIC_API_KEY}
      base_url: https://api.anthropic.com
      default_model: claude-sonnet-4-20250514
      models:
        claude-opus-4-5-20251101:
          context_window: 200000
          max_output: 32000
          features:
            - chat
            - tools
            - streaming
            - vision
          pricing_input: 0.015
          pricing_output: 0.075
        claude-sonnet-4-20250514:
          context_window: 200000
          max_output: 64000
          features:
            - chat
            - tools
            - streaming
            - vision
          pricing_input: 0.003
          pricing_output: 0.015
      options:
        anthropic_version: "2023-06-01"

    openai:
      type: openai
      api_key: ${OPENAI_API_KEY}
      base_url: https://api.openai.com/v1
      default_model: gpt-4o
      models:
        gpt-4o:
          context_window: 128000
          max_output: 16384
          features:
            - chat
            - tools
            - streaming
            - vision
            - json
          pricing_input: 0.005
          pricing_output: 0.015

    ollama:
      type: ollama
      base_url: http://localhost:11434
      default_model: llama3.2
      models:
        llama3.2:
          context_window: 128000
          max_output: 4096
          features:
            - chat
            - streaming
```

### Provider Configuration Fields

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `type` | enum | Yes | Provider type: `anthropic`, `openai`, `google`, `ollama`, `custom` |
| `api_key` | string | Varies | API key (required for cloud providers) |
| `base_url` | string | No | Custom API endpoint |
| `default_model` | string | Yes | Default model for this provider |
| `models` | map | No | Model-specific configurations |
| `options` | map | No | Provider-specific options |

### Model Configuration Fields

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `context_window` | int | Yes | Context window size in tokens |
| `max_output` | int | Yes | Maximum output tokens |
| `features` | []string | No | Model capabilities |
| `pricing_input` | float | No | Price per 1K input tokens (USD) |
| `pricing_output` | float | No | Price per 1K output tokens (USD) |

### Model Features

- `chat`: Conversational completions
- `completion`: Text completions
- `tools`: Function/tool calling
- `streaming`: Streaming responses
- `vision`: Image input support
- `json`: JSON mode / structured output

### Known Provider Base URLs

| Provider | Default Base URL |
|----------|-----------------|
| Anthropic | `https://api.anthropic.com` |
| OpenAI | `https://api.openai.com/v1` |
| Google | `https://generativelanguage.googleapis.com/v1beta` |
| Ollama | `http://localhost:11434` |

---

## Memory Configuration

Gibson uses a three-tier memory architecture.

```yaml
memory:
  working:
    max_tokens: 100000
    eviction_policy: lru

  mission:
    cache_size: 1000
    enable_fts: true

  long_term:
    backend: embedded
    connection_url: ""
    embedder:
      provider: openai
      model: text-embedding-3-small
      api_key: ${OPENAI_API_KEY}
```

### Working Memory (Ephemeral)

In-memory key-value store, cleared after task completion.

| Key | Type | Default | Validation | Description |
|-----|------|---------|------------|-------------|
| `max_tokens` | int | `100000` | >0 | Token budget for working memory |
| `eviction_policy` | string | `lru` | `lru` | Eviction policy (only LRU supported) |

### Mission Memory (Persistent within mission)

SQLite-backed with full-text search.

| Key | Type | Default | Validation | Description |
|-----|------|---------|------------|-------------|
| `cache_size` | int | `1000` | ≥0 | SQLite LRU cache entries |
| `enable_fts` | bool | `true` | | Enable FTS5 full-text search |

### Long-Term Memory (Vector-based, cross-mission)

| Key | Type | Default | Validation | Description |
|-----|------|---------|------------|-------------|
| `backend` | string | `embedded` | `embedded`, `qdrant`, `milvus` | Vector store backend |
| `connection_url` | string | - | Required if external | Connection URL for external backends |
| `embedder.provider` | string | `openai` | `openai`, `llm`, `mock` | Embedding provider |
| `embedder.model` | string | - | Required for non-mock | Embedding model name |
| `embedder.api_key` | string | - | Varies | Embedding provider API key |

---

## GraphRAG Configuration

Neo4j-backed knowledge graph with vector search.

```yaml
graphrag:
  enabled: true
  provider: neo4j

  neo4j:
    uri: bolt://localhost:7687
    username: neo4j
    password: password
    database: neo4j
    pool_size: 10

  vector:
    enabled: true
    index_type: hnsw
    dimensions: 384
    metric: cosine

  embedder:
    provider: native
    model: all-MiniLM-L6-v2
    dimensions: 384
    api_key: ""
    endpoint: ""

  cloud:
    provider: ""
    region: ""
    endpoint: ""
    aws_access_key_id: ""
    aws_secret_access_key: ""
    azure_subscription_id: ""
    azure_tenant_id: ""

  query:
    default_top_k: 10
    default_max_hops: 3
    min_score: 0.5
    vector_weight: 0.5
    graph_weight: 0.5
```

### GraphRAG Root

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable GraphRAG |
| `provider` | string | `neo4j` | Graph provider: `neo4j`, `neptune`, `memgraph` |

### Neo4j Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `uri` | string | - | Neo4j connection URI (e.g., `bolt://localhost:7687`) |
| `username` | string | - | Neo4j username |
| `password` | string | - | Neo4j password |
| `database` | string | `neo4j` | Database name |
| `pool_size` | int | `10` | Connection pool size |

### Vector Search Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable vector search |
| `index_type` | string | `hnsw` | Index type: `hnsw`, `ivfflat` |
| `dimensions` | int | - | Embedding vector dimensions |
| `metric` | string | `cosine` | Similarity metric: `cosine`, `euclidean`, `dot` |

### Embedder Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | - | Provider: `native`, `openai`, `huggingface`, `local` |
| `model` | string | - | Model name |
| `dimensions` | int | - | Embedding dimensions (must match vector.dimensions) |
| `api_key` | string | - | API key for cloud providers |
| `endpoint` | string | - | Custom endpoint for local/custom models |

**Note**: The `native` embedder uses `all-MiniLM-L6-v2` (384 dimensions) and runs fully offline with no API calls.

### Query Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `default_top_k` | int | `10` | Number of results per query |
| `default_max_hops` | int | `3` | Maximum graph traversal depth |
| `min_score` | float | `0.5` | Minimum relevance score threshold |
| `vector_weight` | float | `0.5` | Weight for vector similarity (0-1) |
| `graph_weight` | float | `0.5` | Weight for graph proximity (0-1) |

---

## Registry Configuration

Service discovery for agents, tools, and plugins via embedded or external etcd.

```yaml
registry:
  type: embedded
  data_dir: ~/.gibson/etcd-data
  listen_address: localhost:2379
  endpoints: []
  namespace: gibson
  ttl: 30s

  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `type` | string | `embedded` | Registry mode: `embedded` or `etcd` |
| `data_dir` | string | `$GIBSON_HOME/etcd-data` | Embedded etcd data directory |
| `listen_address` | string | `localhost:2379` | Embedded etcd listen address |
| `endpoints` | []string | `[]` | External etcd cluster endpoints (for type=etcd) |
| `namespace` | string | `gibson` | Key prefix for all registry entries |
| `ttl` | duration | `30s` | Lease time-to-live for registrations |

### TLS Configuration (for external etcd)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `tls.enabled` | bool | `false` | Enable TLS for etcd connections |
| `tls.cert_file` | string | - | Client certificate file path |
| `tls.key_file` | string | - | Client private key file path |
| `tls.ca_file` | string | - | Certificate authority file path |

---

## Callback Server Configuration

gRPC server for external agent communication.

```yaml
callback:
  enabled: true
  listen_address: 0.0.0.0:50001
  advertise_address: ""
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `true` | Enable callback server |
| `listen_address` | string | `0.0.0.0:50001` | Address to listen on |
| `advertise_address` | string | - | Address sent to agents (uses listen_address if empty) |

**Environment Overrides**:
- `GIBSON_CALLBACK_LISTEN_ADDRESS`
- `GIBSON_CALLBACK_ADVERTISE_ADDR`

---

## Registration Server Configuration

Allows agents to dynamically self-register their capabilities.

```yaml
registration:
  enabled: false
  port: 50100
  auth_token: ""
  heartbeat_timeout: 30s
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable registration server |
| `port` | int | `50100` | TCP port for registration gRPC server |
| `auth_token` | string | - | Optional authentication token for agents |
| `heartbeat_timeout` | duration | `30s` | Agent considered dead after this timeout |

---

## Daemon Configuration

Gibson daemon process for background operations.

```yaml
daemon:
  grpc_address: localhost:50002
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `grpc_address` | string | `localhost:50002` | Daemon gRPC API server address |

---

## Logging Configuration

```yaml
logging:
  level: info
  format: json
```

| Key | Type | Default | Options | Description |
|-----|------|---------|---------|-------------|
| `level` | string | `info` | `debug`, `info`, `warn`, `error` | Log level |
| `format` | string | `json` | `json`, `text` | Log output format |

---

## Tracing Configuration

Distributed tracing via OpenTelemetry.

```yaml
tracing:
  enabled: false
  endpoint: localhost:4317
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable distributed tracing |
| `endpoint` | string | - | OTLP exporter endpoint |

---

## Metrics Configuration

Prometheus metrics export.

```yaml
metrics:
  enabled: false
  port: 9090
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable metrics export |
| `port` | int | `9090` | Prometheus metrics port |

---

## Langfuse Configuration

LLM observability integration.

```yaml
langfuse:
  enabled: false
  host: ""
  public_key: ""
  secret_key: ""
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable Langfuse integration |
| `host` | string | - | Langfuse API host |
| `public_key` | string | - | Langfuse public key |
| `secret_key` | string | - | Langfuse secret key |

---

## Plugins Configuration

Plugin-specific configurations are nested under the plugin name:

```yaml
plugins:
  scope_ingestion:
    api_key: ${BUGCROWD_API_KEY}
    platform: bugcrowd
    refresh_interval: 1h

  custom_plugin:
    setting1: value1
    setting2: value2
```

Plugins access their configuration via the harness at runtime.

---

## Command-Specific Flags

### `gibson attack`

```bash
gibson attack [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--target` | string | - | Target name (required) |
| `--agent` | string | - | Agent name (required) |
| `--provider` | string | - | Override target provider |
| `--headers` | string | - | Additional HTTP headers (JSON) |
| `--credential` | string | - | Credential name/ID |
| `--max-turns` | int | `20` | Maximum agent turns |
| `--timeout` | duration | `10m` | Attack timeout |
| `--payloads` | []string | - | Filter to specific payload IDs |
| `--category` | string | - | Filter payloads by category |
| `--techniques` | []string | - | Filter by MITRE technique IDs |
| `--max-findings` | int | `0` | Stop after N findings (0=unlimited) |
| `--severity-threshold` | string | - | Minimum severity: `low`, `medium`, `high`, `critical` |
| `--rate-limit` | int | `0` | Requests per second (0=unlimited) |
| `--no-follow-redirects` | bool | `false` | Don't follow HTTP redirects |
| `--insecure` | bool | `false` | Skip TLS certificate verification |
| `--proxy` | string | - | HTTP/HTTPS proxy URL |
| `--persist` | bool | `false` | Always persist mission and findings |
| `--no-persist` | bool | `false` | Never persist |
| `--dry-run` | bool | `false` | Validate without executing |
| `--list-agents` | bool | `false` | List available agents and exit |

### `gibson config`

```bash
gibson config show [--output-format yaml|json]
gibson config get <key>
gibson config set <key> <value>
gibson config validate
```

### `gibson credential`

```bash
gibson credential add [flags]
gibson credential list
gibson credential show <name>
gibson credential rotate <name>
gibson credential delete <name>
```

| Flag | Type | Description |
|------|------|-------------|
| `--name` | string | Credential name |
| `--type` | string | Type: `api_key`, `bearer`, `basic`, `oauth`, `custom` |
| `--provider` | string | Provider: `openai`, `anthropic`, `aws`, etc. |
| `--from-env` | string | Read from environment variable |
| `--description` | string | Credential description |
| `--force` | bool | Skip confirmation prompt |

### `gibson mission`

```bash
gibson mission run -f <workflow.yaml> [--target <target>] [flags]
gibson mission list [--status <status>]
gibson mission status <mission-id>
gibson mission pause <mission-id>
gibson mission resume <mission-id>
gibson mission delete <mission-id>
gibson mission checkpoints <mission-id>
```

### `gibson finding`

```bash
gibson finding list [--mission <id>] [--severity <level>]
gibson finding show <finding-id>
gibson finding export [--format sarif|json|csv|html] [--output <file>]
```

---

## Complete Example Configuration

```yaml
# Gibson Configuration File
# Location: ~/.gibson/config.yaml

core:
  home_dir: ~/.gibson
  data_dir: ~/.gibson/data
  cache_dir: ~/.gibson/cache
  parallel_limit: 20
  timeout: 10m
  debug: false

database:
  path: ~/.gibson/gibson.db
  max_connections: 10
  timeout: 30s
  wal_mode: true
  auto_vacuum: true

security:
  encryption_algorithm: aes-256-gcm
  key_derivation: scrypt
  ssl_validation: true
  audit_logging: true

llm:
  default_provider: anthropic

  providers:
    anthropic:
      type: anthropic
      api_key: ${ANTHROPIC_API_KEY}
      default_model: claude-sonnet-4-20250514
      models:
        claude-opus-4-5-20251101:
          context_window: 200000
          max_output: 32000
          features: [chat, tools, streaming, vision]
          pricing_input: 0.015
          pricing_output: 0.075
        claude-sonnet-4-20250514:
          context_window: 200000
          max_output: 64000
          features: [chat, tools, streaming, vision]
          pricing_input: 0.003
          pricing_output: 0.015

    openai:
      type: openai
      api_key: ${OPENAI_API_KEY}
      default_model: gpt-4o
      models:
        gpt-4o:
          context_window: 128000
          max_output: 16384
          features: [chat, tools, streaming, vision, json]
          pricing_input: 0.005
          pricing_output: 0.015

    ollama:
      type: ollama
      base_url: http://localhost:11434
      default_model: llama3.2

memory:
  working:
    max_tokens: 100000
    eviction_policy: lru
  mission:
    cache_size: 1000
    enable_fts: true
  long_term:
    backend: embedded
    embedder:
      provider: openai
      model: text-embedding-3-small
      api_key: ${OPENAI_API_KEY}

graphrag:
  enabled: true
  provider: neo4j
  neo4j:
    uri: bolt://localhost:7687
    username: neo4j
    password: ${NEO4J_PASSWORD}
    database: neo4j
    pool_size: 10
  vector:
    enabled: true
    index_type: hnsw
    dimensions: 384
    metric: cosine
  embedder:
    provider: native
    model: all-MiniLM-L6-v2
    dimensions: 384
  query:
    default_top_k: 10
    default_max_hops: 3
    min_score: 0.5
    vector_weight: 0.5
    graph_weight: 0.5

registry:
  type: embedded
  data_dir: ~/.gibson/etcd-data
  listen_address: localhost:2379
  namespace: gibson
  ttl: 30s

callback:
  enabled: true
  listen_address: 0.0.0.0:50001

registration:
  enabled: false
  port: 50100
  heartbeat_timeout: 30s

daemon:
  grpc_address: localhost:50002

logging:
  level: info
  format: json

tracing:
  enabled: false
  endpoint: localhost:4317

metrics:
  enabled: false
  port: 9090

langfuse:
  enabled: false

plugins:
  scope_ingestion:
    api_key: ${BUGCROWD_API_KEY}
    platform: bugcrowd
```

---

## Duration Format Reference

All duration values use Go's duration format:

| Format | Meaning |
|--------|---------|
| `100ms` | 100 milliseconds |
| `1.5s` | 1.5 seconds |
| `30s` | 30 seconds |
| `1m` | 1 minute |
| `1m30s` | 1 minute 30 seconds |
| `5m` | 5 minutes |
| `1h` | 1 hour |
| `1h30m` | 1 hour 30 minutes |
| `24h` | 24 hours |

---

## See Also

- [MISSION.md](./MISSION.md) - Mission/Workflow YAML reference
- [SDK Documentation](../../sdk/CLAUDE.md) - Agent development guide
- [Gibson CLI Guide](../README.md) - Command reference
