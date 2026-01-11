# Building Intelligent Agents in Gibson

This guide explains how to build sophisticated, autonomous agents in Gibson that leverage state-of-the-art (SOTA) AI agent patterns from 2025.

## Table of Contents

1. [The Problem: Dumb vs Smart Agents](#the-problem-dumb-vs-smart-agents)
2. [SOTA Agent Architectures (2025)](#sota-agent-architectures-2025)
3. [The ReAct Pattern](#the-react-pattern)
4. [Gibson's Architecture](#gibsons-architecture)
5. [Memory System](#memory-system)
6. [Plugin System](#plugin-system)
7. [GraphRAG Integration](#graphrag-integration)
8. [Multi-Agent Orchestration](#multi-agent-orchestration)
9. [Building a Smart Agent](#building-a-smart-agent)
10. [Best Practices](#best-practices)

---

## The Problem: Dumb vs Smart Agents

### What Makes an Agent "Dumb"

A dumb agent follows a **fixed, procedural sequence**:

```
1. Do step A
2. Do step B
3. Do step C
4. Return result
```

The LLM is only used for analysis, not decision-making. The agent cannot:
- Adapt to unexpected results
- Choose which tools to use
- Decide when it has enough information
- Reason about what to do next

### What Makes an Agent "Smart"

A smart agent uses the LLM as the **decision-making engine**:

```
Loop:
  1. LLM reasons about current state
  2. LLM decides what action to take
  3. Execute the action (tool call)
  4. LLM observes the result
  5. LLM decides: continue or done?
```

The key difference: **The LLM controls the flow, not hard-coded logic.**

---

## SOTA Agent Architectures (2025)

### The Evolution of AI Agents

The field has evolved from simple prompt-response patterns to sophisticated autonomous systems:

| Era | Pattern | Characteristics |
|-----|---------|-----------------|
| 2023 | Basic RAG | Retrieve → Generate |
| 2024 | ReAct | Reason → Act → Observe loop |
| 2025 | Agentic AI | Multi-agent, memory, tools, planning |

### Key Industry Trends

1. **Multi-Agent Systems** - Gartner reports 1,445% surge in multi-agent inquiries (Q1 2024 → Q2 2025)
2. **Heterogeneous Models** - Expensive models for reasoning, cheap models for execution
3. **Tool Ecosystems** - MCP (Model Context Protocol) as standard for tool integration
4. **Memory-Augmented Generation** - Beyond RAG to true persistent memory

**Sources:**
- [MarkTechPost - Definitive Guide to AI Agents 2025](https://www.marktechpost.com/2025/07/19/the-definitive-guide-to-ai-agents-architectures-frameworks-and-real-world-applications-2025/)
- [Lindy - AI Agent Architecture Guide](https://www.lindy.ai/blog/ai-agent-architecture)

---

## The ReAct Pattern

### Overview

ReAct (Reasoning + Acting) is the foundational pattern for intelligent agents. It interleaves:
- **Reasoning**: Chain-of-thought about what to do
- **Acting**: Tool execution
- **Observing**: Processing results

### The Loop

```
┌─────────────────────────────────────────┐
│              THOUGHT                     │
│  "I need to scan the target network.    │
│   Let me use nmap to find open ports."  │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│              ACTION                      │
│  Tool: nmap                             │
│  Args: {target: "192.168.1.0/24"}       │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│            OBSERVATION                   │
│  Found 5 hosts with open ports:         │
│  - 192.168.1.1: 22, 80, 443            │
│  - 192.168.1.10: 22, 3306              │
│  ...                                    │
└─────────────────┬───────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────┐
│              THOUGHT                     │
│  "I see MySQL on .10. This could be    │
│   vulnerable. Let me check for weak    │
│   credentials..."                       │
└─────────────────┴───────────────────────┘
                  │
                  ▼
              (loop continues)
```

### ReAct vs Function Calling

| Approach | How It Works | Best For |
|----------|--------------|----------|
| **ReAct** | LLM outputs reasoning + action in text | Complex reasoning, traceability |
| **Function Calling** | LLM outputs structured JSON tool calls | Speed, reliability, simple tasks |

Modern agents typically use **function calling** (native to Claude, GPT-4, etc.) with ReAct-style prompting for reasoning.

**Sources:**
- [IBM - What is a ReAct Agent?](https://www.ibm.com/think/topics/react-agent)
- [Prompt Engineering Guide - ReAct](https://www.promptingguide.ai/techniques/react)

---

## Gibson's Architecture

Gibson provides all the primitives needed for SOTA agents. **You compose the agent loop.**

### The Harness API

```
┌─────────────────────────────────────────────────────────────┐
│                      YOUR AGENT                              │
│                                                              │
│   Execute(ctx, task, harness) → Result                      │
│                                                              │
│   You implement:                                             │
│   - System prompt construction                               │
│   - The ReAct/reasoning loop                                │
│   - Tool result processing                                   │
│   - Termination conditions                                   │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                      HARNESS                                 │
│                                                              │
│   LLM Methods:                                               │
│   ├─ Complete()              → Simple completion             │
│   ├─ CompleteWithTools()     → Tool-calling completion       │
│   ├─ CompleteStructured()    → Structured output             │
│   └─ Stream()                → Streaming completion          │
│                                                              │
│   Tool Methods:                                              │
│   ├─ ListTools()             → Discover available tools      │
│   ├─ CallTool()              → Execute a tool                │
│   └─ CallToolsParallel()     → Batch tool execution          │
│                                                              │
│   Memory Methods:                                            │
│   └─ Memory()                → Three-tier memory access      │
│       ├─ .Working()          → Ephemeral (this execution)    │
│       ├─ .Mission()          → Persistent (this mission)     │
│       └─ .LongTerm()         → Vector search (all missions)  │
│                                                              │
│   GraphRAG Methods:                                          │
│   ├─ QueryGraphRAG()         → Semantic graph search         │
│   ├─ StoreGraphNode()        → Store knowledge               │
│   ├─ FindSimilarFindings()   → Find related vulnerabilities  │
│   └─ GetAttackChains()       → Discover attack paths         │
│                                                              │
│   Agent Methods:                                             │
│   ├─ ListAgents()            → Discover available agents     │
│   └─ DelegateToAgent()       → Delegate to specialist        │
│                                                              │
│   Plugin Methods:                                            │
│   ├─ ListPlugins()           → Discover plugins              │
│   └─ QueryPlugin()           → Invoke plugin method          │
│                                                              │
│   Finding Methods:                                           │
│   ├─ SubmitFinding()         → Report vulnerability          │
│   ├─ GetFindings()           → Query current findings        │
│   └─ GetPreviousRunFindings()→ Learn from history            │
│                                                              │
│   Context:                                                   │
│   ├─ Mission()               → Mission metadata              │
│   ├─ Target()                → Target information            │
│   ├─ Logger()                → Structured logging            │
│   └─ Tracer()                → OpenTelemetry tracing         │
└─────────────────────────────────────────────────────────────┘
```

### Gibson vs Other Frameworks

| Feature | Gibson | LangChain | AutoGen | Claude SDK |
|---------|--------|-----------|---------|------------|
| Tool discovery | `ListTools()` | ✓ | ✓ | ✓ |
| Tool calling | `CompleteWithTools()` | ✓ | ✓ | ✓ |
| Three-tier memory | ✓ Built-in | Partial | Partial | ✗ |
| GraphRAG/Knowledge Graph | ✓ Neo4j built-in | ✗ (add-on) | ✗ | ✗ |
| Multi-agent delegation | `DelegateToAgent()` | ✓ | ✓ | ✓ |
| Plugin system | `QueryPlugin()` | ✗ | ✗ | ✗ |
| Security-focused | ✓ Findings, MITRE | ✗ | ✗ | ✗ |
| Built-in ReAct loop | ✗ **You build it** | ✓ | ✓ | ✓ |

**Key Insight**: Gibson gives you powerful primitives. You compose the intelligence.

---

## Memory System

Gibson implements a **three-tier memory architecture** aligned with SOTA research (Mem0, MAGMA).

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    MEMORY TIERS                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   WORKING MEMORY (Ephemeral)                                │
│   ├─ Scope: Single agent execution                          │
│   ├─ Storage: In-memory                                     │
│   ├─ Use: Temporary state, scratchpad                       │
│   └─ API: Set(), Get(), Delete(), List()                    │
│                                                              │
│   MISSION MEMORY (Persistent)                               │
│   ├─ Scope: Entire mission (across agent runs)              │
│   ├─ Storage: Persistent (file/DB)                          │
│   ├─ Use: Shared state, discovered assets, credentials      │
│   └─ API: Set(), Get(), Search(), History()                 │
│                                                              │
│   LONG-TERM MEMORY (Semantic)                               │
│   ├─ Scope: All missions (organizational knowledge)         │
│   ├─ Storage: Vector database + embeddings                  │
│   ├─ Use: Similar findings, attack patterns, learning       │
│   └─ API: Store(), Search(), Recall()                       │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Usage Patterns

```go
// Working Memory - temporary scratchpad
harness.Memory().Working().Set(ctx, "current_target", "192.168.1.10")
target, _ := harness.Memory().Working().Get(ctx, "current_target")

// Mission Memory - shared across agents in this mission
harness.Memory().Mission().Set(ctx, "discovered_hosts", hosts, nil)
hosts, _ := harness.Memory().Mission().Get(ctx, "discovered_hosts")

// Long-Term Memory - semantic search across all missions
results, _ := harness.Memory().LongTerm().Search(ctx, "SQL injection in login forms", 10)
harness.Memory().LongTerm().Store(ctx, "Found SQLi in /api/login endpoint", metadata)
```

### Why Three Tiers?

| Tier | Speed | Persistence | Semantic | Use Case |
|------|-------|-------------|----------|----------|
| Working | Fast | None | No | Loop variables, temp state |
| Mission | Medium | Mission-scoped | No | Discovered assets, shared state |
| Long-Term | Slower | Forever | Yes | Learning, similar findings |

**Research Alignment:**
- [Mem0 - Scalable Long-Term Memory](https://arxiv.org/pdf/2504.19413)
- [Beyond Vector Databases](https://vardhmanandroid2015.medium.com/beyond-vector-databases-architectures-for-true-long-term-ai-memory-0d4629d1a006)
- [IBM - AI Agent Memory](https://www.ibm.com/think/topics/ai-agent-memory)

---

## Plugin System

Plugins extend agent capabilities with domain-specific functionality.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      PLUGINS                                 │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   Plugin = Collection of Methods                             │
│                                                              │
│   ┌─────────────────────────────────────────┐               │
│   │  Plugin: "vulnerability-db"              │               │
│   │  Methods:                                │               │
│   │  ├─ search(cve_id) → details            │               │
│   │  ├─ lookup_by_product(name) → cves      │               │
│   │  └─ get_exploits(cve_id) → exploits     │               │
│   └─────────────────────────────────────────┘               │
│                                                              │
│   ┌─────────────────────────────────────────┐               │
│   │  Plugin: "threat-intel"                  │               │
│   │  Methods:                                │               │
│   │  ├─ check_ip(ip) → reputation           │               │
│   │  ├─ check_domain(domain) → intel        │               │
│   │  └─ get_iocs(campaign) → indicators     │               │
│   └─────────────────────────────────────────┘               │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Plugin vs Tool

| Aspect | Tool | Plugin |
|--------|------|--------|
| Purpose | Execute actions (scan, exploit) | Query data/services |
| Stateless | Yes | Can maintain state |
| Methods | Single function | Multiple methods |
| Example | `nmap`, `sqlmap` | `vuln-db`, `threat-intel` |

### Usage

```go
// List available plugins
plugins, _ := harness.ListPlugins(ctx)

// Query a plugin
result, _ := harness.QueryPlugin(ctx, "vulnerability-db", "search", map[string]any{
    "cve_id": "CVE-2024-1234",
})

// Use in agent reasoning
cveDetails := result.(map[string]any)
if cveDetails["severity"] == "critical" {
    // Prioritize exploitation
}
```

### Comparison to MCP

Gibson's plugin system predates but aligns with [Model Context Protocol (MCP)](https://modelcontextprotocol.io/):

| Feature | Gibson Plugins | MCP |
|---------|---------------|-----|
| Discovery | `ListPlugins()` | Tool search |
| Invocation | `QueryPlugin(method, params)` | Function calls |
| Schema | JSON Schema | JSON Schema |
| Transport | gRPC | stdio/SSE |

**Sources:**
- [Anthropic - MCP Introduction](https://www.anthropic.com/engineering/code-execution-with-mcp)
- [MCP Wikipedia](https://en.wikipedia.org/wiki/Model_Context_Protocol)

---

## GraphRAG Integration

Gibson includes built-in GraphRAG powered by Neo4j for relationship-aware knowledge retrieval.

### Why GraphRAG?

Traditional RAG retrieves chunks based on vector similarity. GraphRAG adds:
- **Relationship traversal** - Follow connections between entities
- **Multi-hop reasoning** - Answer questions requiring multiple steps
- **Structured knowledge** - Entities, relationships, properties

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      GRAPHRAG                                │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   Vector Search (Implicit Semantics)                         │
│   └─ "Find similar vulnerabilities"                         │
│       → Embedding similarity                                 │
│                                                              │
│   Graph Traversal (Explicit Semantics)                       │
│   └─ "What hosts are affected by this CVE?"                 │
│       → HOST -[HAS_VULN]-> VULNERABILITY                    │
│       → VULNERABILITY -[REFERENCES]-> CVE                   │
│                                                              │
│   Hybrid (Best of Both)                                      │
│   └─ Vector search finds entry point                        │
│   └─ Graph traversal expands context                        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Usage

```go
// Store knowledge in graph
nodeID, _ := harness.StoreGraphNode(ctx, graphrag.GraphNode{
    Type:       "Host",
    Properties: map[string]any{"ip": "192.168.1.10", "os": "Linux"},
    Content:    "Web server running Apache 2.4.49",  // For embeddings
})

// Create relationships
harness.CreateGraphRelationship(ctx, graphrag.Relationship{
    FromID: hostNodeID,
    ToID:   vulnNodeID,
    Type:   "HAS_VULNERABILITY",
})

// Semantic search
results, _ := harness.QueryGraphRAG(ctx, graphrag.Query{
    Text:  "hosts vulnerable to path traversal",
    TopK:  10,
})

// Find attack chains
chains, _ := harness.GetAttackChains(ctx, "T1190", 3)  // Initial Access

// Find similar findings (avoid duplicate work)
similar, _ := harness.FindSimilarFindings(ctx, findingID, 5)
```

### Security-Specific Graph Patterns

```
(Mission) -[HAS_TARGET]-> (Target)
(Target) -[HAS_HOST]-> (Host)
(Host) -[HAS_SERVICE]-> (Service)
(Service) -[HAS_VULNERABILITY]-> (Vulnerability)
(Vulnerability) -[EXPLOITED_BY]-> (Technique)
(Technique) -[PART_OF]-> (TacticPhase)
(Finding) -[AFFECTS]-> (Host)
(Finding) -[SIMILAR_TO]-> (Finding)
```

**Sources:**
- [Neo4j - GraphRAG and Agentic Architecture](https://neo4j.com/blog/developer/graphrag-and-agentic-architecture-with-neoconverse/)
- [Neo4j - Temporal Knowledge Graphs](https://neo4j.com/nodes-2025/agenda/building-evolving-ai-agents-via-dynamic-memory-representations-using-temporal-knowledge-graphs/)
- [DeepLearning.AI - Knowledge Graphs for RAG](https://www.deeplearning.ai/short-courses/knowledge-graphs-rag/)

---

## Multi-Agent Orchestration

Complex tasks benefit from specialized agents working together.

### Patterns

```
┌─────────────────────────────────────────────────────────────┐
│              ORCHESTRATION PATTERNS                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   1. COORDINATOR (Hub-and-Spoke)                            │
│      ┌─────────────┐                                        │
│      │ Coordinator │                                        │
│      └──────┬──────┘                                        │
│         ┌───┼───┐                                           │
│         ▼   ▼   ▼                                           │
│      [Recon] [Scan] [Exploit]                               │
│                                                              │
│   2. PIPELINE (Sequential)                                   │
│      [Recon] → [Scan] → [Exploit] → [Report]                │
│                                                              │
│   3. HIERARCHICAL (Layered)                                  │
│      [Strategic Planner]                                     │
│            ▼                                                 │
│      [Tactical Coordinator]                                  │
│         ┌──┴──┐                                             │
│         ▼     ▼                                             │
│      [Recon] [Scan]                                         │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Usage

```go
// Discover available agents
agents, _ := harness.ListAgents(ctx)

// Delegate to specialist
reconTask := agent.NewTask("recon-1", "Enumerate subdomains for target")
reconTask.SetContext("domain", "example.com")

result, _ := harness.DelegateToAgent(ctx, "subdomain-enum-agent", reconTask)

// Use results
subdomains := result.Output.([]string)
for _, subdomain := range subdomains {
    // Delegate scanning to another specialist
    scanTask := agent.NewTask("scan-1", "Scan for vulnerabilities")
    scanTask.SetContext("target", subdomain)
    harness.DelegateToAgent(ctx, "vuln-scanner-agent", scanTask)
}
```

### VulnBot Pattern (Research)

The VulnBot research demonstrates effective multi-agent pentesting:

1. **Reconnaissance Agent** - Asset discovery, fingerprinting
2. **Scanning Agent** - Vulnerability identification
3. **Exploitation Agent** - Proof-of-concept validation
4. **Coordinator** - Task graph, inter-agent communication

**Sources:**
- [VulnBot - Multi-Agent Penetration Testing](https://arxiv.org/pdf/2501.13411)
- [Escape - Agentic Pentesting Architecture](https://escape.tech/blog/best-agentic-pentesting-tools/)
- [ISC2 - Multi-Agent Intelligence](https://www.isc2.org/Insights/2025/07/Multi-Agent-Intelligence-Meets-Automation)

---

## Building a Smart Agent

### Step 1: Define the Agent

```go
myAgent, _ := sdk.NewAgent(
    sdk.WithName("web-recon-agent"),
    sdk.WithVersion("1.0.0"),
    sdk.WithDescription("Intelligent web reconnaissance agent"),
    sdk.WithTargetTypes(types.TargetTypeWebApplication),
    sdk.WithCapabilities(
        agent.CapabilityReconnaissance,
        agent.CapabilityVulnerabilityScanning,
    ),
    sdk.WithLLMSlot("primary", llm.SlotRequirements{
        MinContextWindow: 128000,
        RequiredFeatures: []string{"function_calling"},
    }),
    sdk.WithExecuteFunc(executeWebRecon),
)
```

### Step 2: Build the System Prompt

```go
func buildSystemPrompt(harness agent.Harness, task agent.Task) string {
    // Get available tools dynamically
    tools, _ := harness.ListTools(ctx)
    toolDescriptions := formatToolsForPrompt(tools)

    // Get target context
    target := harness.Target()

    // Check for previous findings
    prevFindings, _ := harness.GetPreviousRunFindings(ctx, finding.Filter{})

    return fmt.Sprintf(`You are an expert web reconnaissance agent.

TARGET: %s
TYPE: %s

YOUR GOAL: %s

AVAILABLE TOOLS:
%s

PREVIOUS FINDINGS (avoid duplicates):
%s

APPROACH:
1. Think step-by-step about what information you need
2. Use tools to gather data about the target
3. After each tool result, analyze what you learned
4. Decide what to investigate next based on findings
5. When you have sufficient information, summarize your findings

IMPORTANT:
- Start with passive reconnaissance before active scanning
- Note potential vulnerabilities as you discover them
- Use memory to track what you've already checked
- Submit findings for confirmed vulnerabilities

When you have completed reconnaissance, respond with "RECONNAISSANCE COMPLETE"
and provide a structured summary.`,
        target.URL,
        target.Type,
        task.Goal,
        toolDescriptions,
        formatPreviousFindings(prevFindings),
    )
}
```

### Step 3: Implement the ReAct Loop

```go
func executeWebRecon(ctx context.Context, h agent.Harness, task agent.Task) (agent.Result, error) {
    logger := h.Logger()

    // Build tool definitions for LLM
    tools, _ := h.ListTools(ctx)
    toolDefs := convertToLLMTools(tools)

    // Initialize conversation
    systemPrompt := buildSystemPrompt(h, task)
    messages := []llm.Message{
        llm.NewSystemMessage(systemPrompt),
        llm.NewUserMessage(fmt.Sprintf("Begin reconnaissance of %s", h.Target().URL)),
    }

    // ReAct loop
    maxTurns := 20
    for turn := 0; turn < maxTurns; turn++ {
        logger.Info("Agent turn", "turn", turn+1, "max", maxTurns)

        // Ask LLM what to do next (with tools)
        resp, err := h.CompleteWithTools(ctx, "primary", messages, toolDefs)
        if err != nil {
            return agent.NewFailedResult(err), err
        }

        // Process tool calls
        if len(resp.ToolCalls) > 0 {
            for _, toolCall := range resp.ToolCalls {
                logger.Info("Executing tool", "tool", toolCall.Name)

                // Execute the tool
                result, err := h.CallTool(ctx, toolCall.Name, toolCall.Arguments)

                // Store in working memory for reference
                h.Memory().Working().Set(ctx,
                    fmt.Sprintf("tool_result_%d", turn),
                    result)

                // Add tool result to conversation
                if err != nil {
                    messages = append(messages, llm.NewToolResultMessage(
                        toolCall.ID,
                        fmt.Sprintf("Error: %v", err),
                    ))
                } else {
                    messages = append(messages, llm.NewToolResultMessage(
                        toolCall.ID,
                        formatToolResult(result),
                    ))
                }
            }
        }

        // Add assistant response to conversation
        messages = append(messages, llm.NewAssistantMessage(resp.Content))

        // Check for completion
        if strings.Contains(resp.Content, "RECONNAISSANCE COMPLETE") {
            logger.Info("Agent completed reconnaissance")
            break
        }

        // Check for findings to submit
        findings := extractFindings(resp.Content)
        for _, f := range findings {
            h.SubmitFinding(ctx, f)
        }
    }

    // Extract final summary
    summary := extractSummary(messages)

    // Store results in mission memory for other agents
    h.Memory().Mission().Set(ctx, "recon_results", summary, nil)

    return agent.Result{
        Status: agent.StatusSuccess,
        Output: summary,
    }, nil
}
```

### Step 4: Convert Tools to LLM Format

```go
func convertToLLMTools(tools []tool.Descriptor) []llm.ToolDef {
    var defs []llm.ToolDef
    for _, t := range tools {
        defs = append(defs, llm.ToolDef{
            Name:        t.Name,
            Description: t.Description,
            Parameters:  t.InputSchema,
        })
    }
    return defs
}
```

---

## Best Practices

### 1. Dynamic Tool Discovery

Don't hardcode tools. Use `ListTools()` to discover at runtime:

```go
// Good: Dynamic discovery
tools, _ := harness.ListTools(ctx)
toolDefs := convertToLLMTools(tools)

// Bad: Hardcoded
tools := []llm.ToolDef{
    {Name: "nmap", ...},  // What if nmap isn't available?
}
```

### 2. Use Memory Appropriately

| Data Type | Memory Tier |
|-----------|-------------|
| Loop counters, temp vars | Working |
| Discovered hosts, credentials | Mission |
| Attack patterns, learned techniques | Long-Term |

### 3. Avoid Duplicate Work

Check previous findings before starting:

```go
prevFindings, _ := harness.GetPreviousRunFindings(ctx, finding.Filter{})
similarFindings, _ := harness.FindSimilarFindings(ctx, currentFinding.ID, 5)
```

### 4. Submit Findings Early

Don't wait until the end. Submit as you discover:

```go
// Good: Submit immediately
if isVulnerable {
    harness.SubmitFinding(ctx, finding)
}

// Bad: Collect and submit at end
// (What if agent crashes?)
```

### 5. Use Structured Output for Analysis

For complex reasoning, use structured output:

```go
type VulnerabilityAnalysis struct {
    Severity      string   `json:"severity"`
    Exploitable   bool     `json:"exploitable"`
    Impact        string   `json:"impact"`
    Remediation   string   `json:"remediation"`
}

analysis, _ := harness.CompleteStructured(ctx, "primary", messages, VulnerabilityAnalysis{})
```

### 6. Delegate to Specialists

Don't build one mega-agent. Delegate:

```go
// Coordinator pattern
if needsDeepWebScan {
    harness.DelegateToAgent(ctx, "web-scanner-agent", webTask)
}
if needsNetworkRecon {
    harness.DelegateToAgent(ctx, "network-recon-agent", netTask)
}
```

### 7. Use GraphRAG for Context

Leverage the knowledge graph:

```go
// Find similar attack patterns
patterns, _ := harness.FindSimilarAttacks(ctx, "SQL injection in login form", 5)

// Store discovered knowledge
harness.StoreGraphNode(ctx, graphrag.GraphNode{
    Type: "Endpoint",
    Properties: map[string]any{
        "path": "/api/login",
        "vulnerable": true,
    },
})
```

### 8. Handle Errors Gracefully

Tools fail. Handle it:

```go
result, err := harness.CallTool(ctx, toolCall.Name, toolCall.Arguments)
if err != nil {
    // Tell the LLM what happened so it can adapt
    messages = append(messages, llm.NewToolResultMessage(
        toolCall.ID,
        fmt.Sprintf("Tool failed: %v. Try a different approach.", err),
    ))
    continue  // Don't crash, let LLM adapt
}
```

---

## Summary

Building intelligent agents in Gibson:

1. **Use the ReAct pattern** - Let the LLM reason and decide
2. **Discover tools dynamically** - `ListTools()` at runtime
3. **Leverage three-tier memory** - Working → Mission → Long-Term
4. **Use GraphRAG** - Relationship-aware knowledge
5. **Delegate to specialists** - Multi-agent coordination
6. **Submit findings early** - Don't wait until the end

Gibson provides the primitives. You compose the intelligence.

---

## References

### Agent Architectures
- [IBM - What is a ReAct Agent?](https://www.ibm.com/think/topics/react-agent)
- [MarkTechPost - Definitive Guide to AI Agents 2025](https://www.marktechpost.com/2025/07/19/the-definitive-guide-to-ai-agents-architectures-frameworks-and-real-world-applications-2025/)
- [Letta - Rearchitecting Agent Loops](https://www.letta.com/blog/letta-v1-agent)

### Memory Systems
- [Mem0 - Scalable Long-Term Memory](https://arxiv.org/pdf/2504.19413)
- [IBM - AI Agent Memory](https://www.ibm.com/think/topics/ai-agent-memory)
- [Beyond Vector Databases](https://vardhmanandroid2015.medium.com/beyond-vector-databases-architectures-for-true-long-term-ai-memory-0d4629d1a006)

### Tool Integration
- [Anthropic - Advanced Tool Use](https://www.anthropic.com/engineering/advanced-tool-use)
- [MCP - Model Context Protocol](https://modelcontextprotocol.io/)
- [Anthropic - Code Execution with MCP](https://www.anthropic.com/engineering/code-execution-with-mcp)

### GraphRAG
- [Neo4j - GraphRAG and Agentic Architecture](https://neo4j.com/blog/developer/graphrag-and-agentic-architecture-with-neoconverse/)
- [DeepLearning.AI - Knowledge Graphs for RAG](https://www.deeplearning.ai/short-courses/knowledge-graphs-rag/)

### Multi-Agent Systems
- [VulnBot - Multi-Agent Penetration Testing](https://arxiv.org/pdf/2501.13411)
- [Escape - Agentic Pentesting Architecture](https://escape.tech/blog/best-agentic-pentesting-tools/)
- [Anthropic - Building Agents with Claude SDK](https://www.anthropic.com/engineering/building-agents-with-the-claude-agent-sdk)
