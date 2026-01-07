# Schema-Based Targets

Gibson uses a schema-driven target system that enables testing diverse attack surfaces beyond HTTP APIs. Each target type defines a JSON Schema for its connection parameters, allowing Gibson to handle Kubernetes clusters, smart contracts, custom protocols, and traditional HTTP endpoints through a unified interface.

## Table of Contents

- [Overview](#overview)
- [Target Types](#target-types)
- [Creating Targets](#creating-targets)
- [Running Attacks](#running-attacks)
- [Built-in Target Schemas](#built-in-target-schemas)
- [Migration Guide](#migration-guide)
- [Advanced Usage](#advanced-usage)

## Overview

### What are Schema-Based Targets?

Schema-based targets allow you to define connection parameters specific to each target type using JSON Schema validation. Instead of being limited to URL-based targets, you can now test:

- **HTTP APIs** - Traditional REST/GraphQL endpoints
- **LLM Chat Interfaces** - OpenAI, Anthropic, and other chat APIs
- **Kubernetes Clusters** - ML pipelines, AI deployments, cluster security
- **Smart Contracts** - Blockchain-based AI oracles and contracts
- **Custom Protocols** - Any protocol with a registered schema

### Key Benefits

- **Extensibility**: Add new target types without modifying core framework
- **Validation**: JSON Schema ensures required connection parameters are provided
- **Type Safety**: Connection parameters are validated before execution
- **Flexibility**: Support for diverse attack surfaces in AI security testing
- **Backward Compatibility**: Existing URL-based targets continue to work

## Target Types

Gibson includes five built-in target types:

| Type | Use Case | Required Fields |
|------|----------|-----------------|
| `http_api` | REST/GraphQL APIs, generic HTTP endpoints | `url` |
| `llm_chat` | Chat-based LLM interfaces (OpenAI, Anthropic) | `url` |
| `llm_api` | LLM API endpoints with specific providers | `url` |
| `kubernetes` | Kubernetes clusters and namespaces | `cluster` |
| `smart_contract` | Blockchain smart contracts | `chain`, `address` |

### Agent Compatibility

Agents declare which target types they support via the `TargetSchemas()` method. Before executing an attack, Gibson validates that the agent supports the target's type.

Example:
- `k8skiller` agent supports `kubernetes` targets only
- `prompt-injector` agent supports `http_api`, `llm_chat`, `llm_api` targets
- Agents without explicit schemas accept any target type (for backward compatibility)

## Creating Targets

### Basic Target Creation

Use `gibson target add` to create a named target:

```bash
gibson target add <name> --type <type> --connection <json>
```

**Example: HTTP API Target**
```bash
gibson target add my-api \
  --type http_api \
  --connection '{"url":"https://api.example.com/v1/chat"}' \
  --description "Production chat API"
```

**Example: Kubernetes Target**
```bash
gibson target add prod-cluster \
  --type kubernetes \
  --connection '{"cluster":"production","namespace":"ml-pipeline"}' \
  --description "Production ML cluster"
```

**Example: Smart Contract Target**
```bash
gibson target add my-contract \
  --type smart_contract \
  --connection '{"chain":"ethereum","address":"0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb"}' \
  --description "AI oracle contract"
```

### Interactive Target Creation

Use the `--interactive` flag to be prompted for connection fields:

```bash
gibson target add test-k8s --type kubernetes --interactive
```

This will prompt for each field defined in the schema:
```
cluster (required): minikube
namespace (optional, default: default): kube-system
kubeconfig (optional): ~/.kube/config
api_server (optional):
```

### Listing Targets

```bash
# List all targets
gibson target list

# Output shows:
# ID                     Name            Type         Connection
# abc123...              my-api          http_api     url: https://api.example.com
# def456...              prod-cluster    kubernetes   cluster: production, ns: ml-pipeline
```

### Viewing Target Details

```bash
gibson target show my-api
```

Output:
```yaml
ID: abc123-def456-789
Name: my-api
Type: http_api
Description: Production chat API
Connection:
  url: https://api.example.com/v1/chat
  method: POST
  timeout: 30
Created: 2025-01-15 10:30:00
Updated: 2025-01-15 10:30:00
```

**Note**: Sensitive fields like `api_key`, `token`, and `password` are masked in output.

## Running Attacks

### Using Stored Targets

Reference a stored target by name:

```bash
gibson attack --agent <agent-name> --target <target-name>
```

**Example: Attack Kubernetes Cluster**
```bash
gibson attack \
  --agent k8skiller \
  --target prod-cluster \
  --goal "Identify misconfigurations in ML pipeline namespace"
```

**Example: Attack HTTP API**
```bash
gibson attack \
  --agent prompt-injector \
  --target my-api \
  --goal "Test for prompt injection vulnerabilities"
```

### Using Inline Targets

For one-off tests, define the target inline without storing it:

```bash
gibson attack \
  --agent <agent-name> \
  --type <type> \
  --connection <json>
```

**Example: Inline Kubernetes Attack**
```bash
gibson attack \
  --agent k8skiller \
  --type kubernetes \
  --connection '{"cluster":"minikube","namespace":"default"}' \
  --goal "Test local development cluster"
```

**Example: Inline HTTP Attack**
```bash
gibson attack \
  --agent prompt-injector \
  --type http_api \
  --connection '{"url":"https://test.example.com/chat"}' \
  --goal "Quick injection test"
```

### Type Compatibility Validation

If you attempt to use an incompatible target type, Gibson will reject the attack:

```bash
gibson attack --agent k8skiller --target my-api
# Error: agent "k8skiller" does not support target type "http_api"
# Supported types: kubernetes
```

## Built-in Target Schemas

### HTTP API Schema

**Type**: `http_api`

**Required Fields**:
- `url` (string, URI format) - Target endpoint URL

**Optional Fields**:
- `method` (string, enum: GET/POST/PUT/DELETE) - HTTP method (default: POST)
- `headers` (object) - Additional HTTP headers
- `timeout` (integer, minimum: 1) - Request timeout in seconds (default: 30)

**Example**:
```json
{
  "url": "https://api.example.com/v1/completions",
  "method": "POST",
  "headers": {
    "Authorization": "Bearer sk-...",
    "Content-Type": "application/json"
  },
  "timeout": 60
}
```

**Create Command**:
```bash
gibson target add my-llm-api \
  --type http_api \
  --connection '{"url":"https://api.example.com/v1/completions","headers":{"Authorization":"Bearer sk-xxx"}}'
```

### LLM Chat Schema

**Type**: `llm_chat`

**Required Fields**:
- `url` (string, URI format) - Chat API endpoint

**Optional Fields**:
- `model` (string) - Model identifier (e.g., "gpt-4", "claude-3-opus")
- `headers` (object) - HTTP headers for authentication
- `provider` (string) - Provider hint (openai, anthropic, etc.)
- `timeout` (integer) - Request timeout

**Example**:
```json
{
  "url": "https://api.openai.com/v1/chat/completions",
  "model": "gpt-4",
  "headers": {
    "Authorization": "Bearer sk-...",
    "OpenAI-Organization": "org-..."
  },
  "provider": "openai"
}
```

**Create Command**:
```bash
gibson target add openai-chat \
  --type llm_chat \
  --connection '{"url":"https://api.openai.com/v1/chat/completions","model":"gpt-4","provider":"openai"}'
```

### LLM API Schema

**Type**: `llm_api`

Similar to `llm_chat` but for provider-specific API endpoints. Extends HTTP API schema with LLM-specific fields.

**Required Fields**:
- `url` (string, URI format)

**Optional Fields**:
- `model` (string)
- `headers` (object)
- `provider` (string)
- `timeout` (integer)

### Kubernetes Schema

**Type**: `kubernetes`

**Required Fields**:
- `cluster` (string) - Cluster name or kubeconfig context

**Optional Fields**:
- `namespace` (string) - Target namespace (default: "default")
- `kubeconfig` (string) - Path to kubeconfig file
- `api_server` (string, URI format) - Kubernetes API server URL

**Example**:
```json
{
  "cluster": "production-us-west",
  "namespace": "ml-pipeline",
  "kubeconfig": "~/.kube/config"
}
```

**Create Command**:
```bash
gibson target add prod-k8s \
  --type kubernetes \
  --connection '{"cluster":"production-us-west","namespace":"ml-pipeline"}'
```

**Example with API Server**:
```json
{
  "cluster": "eks-cluster",
  "namespace": "ai-services",
  "api_server": "https://ABC123.gr7.us-west-2.eks.amazonaws.com"
}
```

### Smart Contract Schema

**Type**: `smart_contract`

**Required Fields**:
- `chain` (string, enum) - Blockchain network (ethereum, polygon, arbitrum, base, solana)
- `address` (string, pattern: `^0x[a-fA-F0-9]{40}$`) - Contract address

**Optional Fields**:
- `rpc_url` (string, URI format) - RPC endpoint URL
- `abi` (string) - Contract ABI JSON

**Example**:
```json
{
  "chain": "ethereum",
  "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
  "rpc_url": "https://mainnet.infura.io/v3/YOUR-PROJECT-ID",
  "abi": "[{\"inputs\":[],\"name\":\"predict\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\"}]}]"
}
```

**Create Command**:
```bash
gibson target add ai-oracle \
  --type smart_contract \
  --connection '{"chain":"ethereum","address":"0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb"}'
```

## Migration Guide

### Migrating from URL-Based Attacks

**Old Syntax (Deprecated)**:
```bash
gibson attack https://api.example.com/v1/chat --agent prompt-injector
```

**New Syntax - Option 1 (Inline)**:
```bash
gibson attack \
  --agent prompt-injector \
  --type http_api \
  --connection '{"url":"https://api.example.com/v1/chat"}'
```

**New Syntax - Option 2 (Stored Target)**:
```bash
# Create target once
gibson target add my-api \
  --type http_api \
  --connection '{"url":"https://api.example.com/v1/chat"}'

# Use target in attacks
gibson attack --agent prompt-injector --target my-api
```

### Why Schema-Based Targets?

The URL-based approach worked for HTTP endpoints but couldn't express:
- Kubernetes cluster contexts and namespaces
- Blockchain network and contract addresses
- Custom protocol connection parameters
- Multi-parameter authentication schemes

Schema-based targets provide:
- **Type-specific parameters**: Each target type defines its own fields
- **Validation**: Required fields enforced before execution
- **Reusability**: Store targets once, use in multiple attacks
- **Clarity**: Connection parameters explicit and documented

### Backward Compatibility

During the migration period:
- Existing agents can still use `h.Target().URL()` method
- URL field is populated from `Connection["url"]` for HTTP-based targets
- Agents without `TargetSchemas()` accept any target type
- Database stores both legacy and new formats

### Updating Agents to Use Schemas

**Before (Legacy)**:
```go
func (a *MyAgent) Execute(ctx context.Context, h Harness, task Task) (Result, error) {
    targetURL := h.Target().URL
    // Use URL directly
}
```

**After (Schema-Based)**:
```go
import "github.com/zero-day-ai/sdk/input"

func (a *MyAgent) TargetSchemas() []types.TargetSchema {
    return []types.TargetSchema{target.HTTPAPISchema}
}

func (a *MyAgent) Execute(ctx context.Context, h Harness, task Task) (Result, error) {
    url := input.GetString(h.Target().Connection, "url", "")
    method := input.GetString(h.Target().Connection, "method", "POST")
    headers := input.GetMap(h.Target().Connection, "headers", nil)
    // Use connection parameters
}
```

## Advanced Usage

### Custom Target Schemas

To define a custom target type for your agents:

```go
package main

import (
    "github.com/zero-day-ai/sdk/schema"
    "github.com/zero-day-ai/sdk/types"
)

var CustomProtocolSchema = types.TargetSchema{
    Type:    "custom_protocol",
    Version: "1.0",
    Description: "Custom protocol for proprietary AI system",
    Schema: schema.New().
        Required("host").
        Required("port").
        Optional("protocol", "tcp").
        String("host", "Hostname or IP address").
        Integer("port", "Port number", 1, 65535).
        String("protocol", "Protocol type (tcp, udp)").
        Build(),
}

func (a *MyAgent) TargetSchemas() []types.TargetSchema {
    return []types.TargetSchema{CustomProtocolSchema}
}
```

### Schema Validation Errors

Gibson validates connection parameters against the schema before execution:

**Missing Required Field**:
```bash
gibson target add test --type kubernetes --connection '{"namespace":"default"}'
# Error: connection missing required field "cluster" for type "kubernetes"
```

**Invalid Field Type**:
```bash
gibson target add test --type kubernetes --connection '{"cluster":"prod","namespace":123}'
# Error: connection field "namespace" must be string, got number
```

**Invalid Enum Value**:
```bash
gibson target add test --type smart_contract --connection '{"chain":"bitcoin","address":"0x..."}'
# Error: connection field "chain" must be one of: ethereum, polygon, arbitrum, base, solana
```

### Target Management Best Practices

1. **Use Descriptive Names**: Name targets by environment and purpose
   ```bash
   gibson target add prod-api-us-west
   gibson target add staging-k8s-ml-pipeline
   gibson target add dev-smart-contract-testnet
   ```

2. **Add Descriptions**: Document the target's purpose
   ```bash
   gibson target add prod-api \
     --description "Production LLM API - requires VPN access"
   ```

3. **Use Tags for Organization**: Group related targets
   ```bash
   gibson target add prod-api --tags "production,http,llm"
   gibson target add staging-api --tags "staging,http,llm"
   ```

4. **Leverage Stored Targets**: Create targets once, reuse in multiple attacks
   ```bash
   # Create once
   gibson target add prod-k8s --type kubernetes --connection '...'

   # Use multiple times
   gibson attack --agent k8skiller --target prod-k8s --goal "Pod security"
   gibson attack --agent k8skiller --target prod-k8s --goal "RBAC audit"
   gibson attack --agent k8skiller --target prod-k8s --goal "Network policies"
   ```

5. **Protect Sensitive Data**: Use credential references instead of inline secrets
   ```bash
   # Store credential separately
   gibson credential add k8s-prod-token --type bearer --value "..."

   # Reference in target
   gibson target add prod-k8s \
     --type kubernetes \
     --connection '{"cluster":"prod"}' \
     --credential k8s-prod-token
   ```

### JSON Output for Automation

Use `--output json` for scripting:

```bash
# Export targets as JSON
gibson target list --output json > targets.json

# Show target in JSON format
gibson target show my-api --output json | jq '.connection.url'
```

### Deleting Targets

```bash
# Delete a specific target
gibson target delete my-api

# Delete multiple targets
gibson target delete prod-k8s staging-k8s dev-k8s
```

## Troubleshooting

### Error: "no schema registered for type"

**Cause**: Target type not recognized or agent with schema not registered.

**Solution**:
1. Ensure the agent that supports the target type is registered:
   ```bash
   gibson agent list | grep <agent-name>
   ```
2. Check for typos in target type:
   ```bash
   # Wrong
   --type k8s

   # Correct
   --type kubernetes
   ```

### Error: "agent does not support target type"

**Cause**: Agent explicitly declares supported types via `TargetSchemas()` and the target's type is not included.

**Solution**:
1. Check which types the agent supports:
   ```bash
   gibson agent show <agent-name>
   ```
2. Use a compatible agent or create a target with a supported type.

### Error: "connection missing required field"

**Cause**: Required schema field not provided in connection JSON.

**Solution**: Add the missing field. Check schema documentation for required fields:
- `http_api`: requires `url`
- `kubernetes`: requires `cluster`
- `smart_contract`: requires `chain` and `address`

### Warning: "no schema registered for type, skipping validation"

**Cause**: Target type is unknown (no agent declares a schema for it).

**Effect**: Target is created without validation (forward compatibility).

**Action**: This is usually safe. The target will be validated when an agent with the schema registers.

## See Also

- [Agent Development Guide](../sdk/docs/AGENT.md) - Creating agents with target schema support
- [CLI Reference](#cli-reference) - Complete command reference
- [Configuration Guide](#configuration) - Gibson configuration options
- [SDK Documentation](../sdk/README.md) - SDK API reference
