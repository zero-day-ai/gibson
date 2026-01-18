package orchestrator

import (
	"fmt"
	"strings"
)

// InventoryPromptFormatter formats ComponentInventory for inclusion in LLM prompts.
// It supports configurable token budgets and progressive summarization to ensure
// the inventory section doesn't exceed context limits while maintaining visibility
// of all available components.
type InventoryPromptFormatter struct {
	maxTokenBudget int  // Maximum tokens to allocate for inventory section (default: 500)
	verboseMode    bool // Include full schemas and detailed information (default: false)
}

// FormatterOption is a functional option for configuring InventoryPromptFormatter.
type FormatterOption func(*InventoryPromptFormatter)

// WithMaxTokenBudget sets the maximum token budget for inventory formatting.
// The formatter will use progressive summarization if the full format exceeds this budget.
// Default is 500 tokens. Set to 0 for unlimited (not recommended).
func WithMaxTokenBudget(tokens int) FormatterOption {
	return func(f *InventoryPromptFormatter) {
		f.maxTokenBudget = tokens
	}
}

// WithVerboseMode enables verbose formatting with full schemas and detailed information.
// This produces more detailed output but consumes more tokens.
// Default is false. Only recommended when token budget is generous.
func WithVerboseMode(verbose bool) FormatterOption {
	return func(f *InventoryPromptFormatter) {
		f.verboseMode = verbose
	}
}

// NewInventoryPromptFormatter creates a new formatter with sensible defaults.
// Default configuration:
//   - maxTokenBudget: 500 tokens
//   - verboseMode: false
//
// Use functional options to customize behavior:
//
//	formatter := NewInventoryPromptFormatter(
//	    WithMaxTokenBudget(1000),
//	    WithVerboseMode(true),
//	)
func NewInventoryPromptFormatter(opts ...FormatterOption) *InventoryPromptFormatter {
	f := &InventoryPromptFormatter{
		maxTokenBudget: 500, // Default budget
		verboseMode:    false,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

// Format generates a formatted markdown representation of the component inventory
// suitable for LLM prompts. It automatically applies progressive summarization
// when the output would exceed the configured token budget.
//
// Progressive summarization levels:
//  1. Full table format (default) - Complete tables with all metadata
//  2. Condensed format - Brief lists with component names grouped by type
//  3. Names-only format - Minimal format showing only component names (future enhancement)
//
// The missionTargetType parameter is used to highlight agents that match the
// mission's target type (e.g., "llm_chat", "kubernetes") with bold formatting.
//
// Returns empty string if inventory is nil. Handles empty component lists gracefully.
func (f *InventoryPromptFormatter) Format(inv *ComponentInventory, missionTargetType string) string {
	if inv == nil {
		return ""
	}

	// Estimate tokens for full format
	estimatedTokens := f.EstimateTokens(inv)

	// If budget allows (or no budget set), use full table format
	if f.maxTokenBudget == 0 || estimatedTokens <= f.maxTokenBudget {
		return f.formatFullTables(inv, missionTargetType)
	}

	// Otherwise, use condensed format
	return f.FormatCondensed(inv)
}

// formatFullTables generates the full table format with complete metadata.
func (f *InventoryPromptFormatter) formatFullTables(inv *ComponentInventory, missionTargetType string) string {
	var sb strings.Builder

	sb.WriteString("## Available Components\n\n")

	// Include stale warning if applicable
	if inv.IsStale {
		sb.WriteString("*Note: Component inventory may be stale (registry unavailable). Using cached data.*\n\n")
	}

	// Format agents
	if len(inv.Agents) > 0 {
		sb.WriteString(f.FormatAgents(inv.Agents, missionTargetType))
		sb.WriteString("\n")
	}

	// Format tools
	if len(inv.Tools) > 0 {
		sb.WriteString(f.FormatTools(inv.Tools))
		sb.WriteString("\n")
	}

	// Format plugins
	if len(inv.Plugins) > 0 {
		sb.WriteString(f.FormatPlugins(inv.Plugins))
		sb.WriteString("\n")
	}

	// Add usage guidance
	if len(inv.Agents) > 0 {
		sb.WriteString("*Use `spawn_agent` action only with agent names from the list above.*\n")
	}

	return sb.String()
}

// FormatAgents generates a markdown table for agent summaries.
// Agents matching the missionTargetType are highlighted with bold formatting.
//
// Table columns:
//   - Name: Agent identifier
//   - Capabilities: Comma-separated list of capabilities
//   - Target Types: Comma-separated list of target types
//   - Health: Health status (healthy, degraded, unhealthy, unavailable)
//   - Instances: Number of running instances
//
// Example output:
//
//	### Agents (for spawn_agent)
//	| Name | Capabilities | Target Types | Health | Instances |
//	|------|-------------|--------------|--------|-----------|
//	| **davinci** | prompt_injection, jailbreak | **llm_chat**, llm_api | healthy | 2 |
//	| k8skiller | container_escape, rbac_abuse | kubernetes | healthy | 1 |
func (f *InventoryPromptFormatter) FormatAgents(agents []AgentSummary, missionTargetType string) string {
	if len(agents) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("### Agents (for spawn_agent)\n")
	sb.WriteString("| Name | Capabilities | Target Types | Health | Instances |\n")
	sb.WriteString("|------|-------------|--------------|--------|-----------|")

	for _, agent := range agents {
		sb.WriteString("\n| ")

		// Name - bold if agent matches mission target type
		name := agent.Name
		if missionTargetType != "" && f.agentMatchesTargetType(agent, missionTargetType) {
			name = fmt.Sprintf("**%s**", name)
		}
		sb.WriteString(name)
		sb.WriteString(" | ")

		// Capabilities
		caps := f.joinAndTruncate(agent.Capabilities, ", ", 50)
		sb.WriteString(caps)
		sb.WriteString(" | ")

		// Target Types - bold matching target types
		targetTypes := f.formatTargetTypes(agent.TargetTypes, missionTargetType)
		sb.WriteString(targetTypes)
		sb.WriteString(" | ")

		// Health Status
		sb.WriteString(agent.HealthStatus)
		sb.WriteString(" | ")

		// Instances
		sb.WriteString(fmt.Sprintf("%d", agent.Instances))
		sb.WriteString(" |")
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatTools generates a markdown table for tool summaries.
//
// Table columns:
//   - Name: Tool identifier
//   - Tags: Comma-separated list of tags
//   - Description: Brief tool description (truncated if too long)
//   - Health: Health status
//
// Example output:
//
//	### Tools
//	| Name | Tags | Description | Health |
//	|------|------|-------------|--------|
//	| nmap | network, enumeration | Port scanning and service detection | healthy |
//	| sqlmap | web, injection | SQL injection testing | healthy |
func (f *InventoryPromptFormatter) FormatTools(tools []ToolSummary) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("### Tools\n")
	sb.WriteString("| Name | Tags | Description | Health |\n")
	sb.WriteString("|------|------|-------------|--------|")

	for _, tool := range tools {
		sb.WriteString("\n| ")

		// Name
		sb.WriteString(tool.Name)
		sb.WriteString(" | ")

		// Tags
		tags := f.joinAndTruncate(tool.Tags, ", ", 30)
		sb.WriteString(tags)
		sb.WriteString(" | ")

		// Description (truncated)
		desc := f.truncateString(tool.Description, 50)
		sb.WriteString(desc)
		sb.WriteString(" | ")

		// Health Status
		sb.WriteString(tool.HealthStatus)
		sb.WriteString(" |")
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatPlugins generates a markdown table for plugin summaries.
//
// Table columns:
//   - Name: Plugin identifier
//   - Methods: Comma-separated list of method names
//   - Health: Health status
//
// Example output:
//
//	### Plugins
//	| Name | Methods | Health |
//	|------|---------|--------|
//	| mitre-lookup | getTechnique, mapToTactic | healthy |
//	| report-gen | generateHTML, generatePDF | healthy |
func (f *InventoryPromptFormatter) FormatPlugins(plugins []PluginSummary) string {
	if len(plugins) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("### Plugins\n")
	sb.WriteString("| Name | Methods | Health |\n")
	sb.WriteString("|------|---------|--------|")

	for _, plugin := range plugins {
		sb.WriteString("\n| ")

		// Name
		sb.WriteString(plugin.Name)
		sb.WriteString(" | ")

		// Methods (extract names from MethodSummary)
		methodNames := make([]string, len(plugin.Methods))
		for i, method := range plugin.Methods {
			methodNames[i] = method.Name
		}
		methods := f.joinAndTruncate(methodNames, ", ", 50)
		sb.WriteString(methods)
		sb.WriteString(" | ")

		// Health Status
		sb.WriteString(plugin.HealthStatus)
		sb.WriteString(" |")
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatCondensed generates a condensed format suitable for when token budget is tight.
// This format lists component names grouped by type without detailed metadata.
//
// Condensed format includes:
//   - Total component count summary
//   - Component names grouped by type (agents, tools, plugins)
//   - Brief usage guidance
//
// Example output:
//
//	## Available Components (23 total)
//
//	**Agents (8):** davinci, k8skiller, nuclei-runner, web-crawler, prompt-injector, model-extractor, llm-fuzzer, credential-harvester
//
//	**Tools (12):** nmap, sqlmap, ffuf, nikto, gobuster, hydra, john, hashcat, burp-proxy, zap, nuclei, httpx
//
//	**Plugins (3):** mitre-lookup, report-gen, cve-lookup
//
//	*Use spawn_agent only with agent names from the list above.*
func (f *InventoryPromptFormatter) FormatCondensed(inv *ComponentInventory) string {
	if inv == nil {
		return ""
	}

	var sb strings.Builder

	totalComponents := len(inv.Agents) + len(inv.Tools) + len(inv.Plugins)
	sb.WriteString(fmt.Sprintf("## Available Components (%d total)\n\n", totalComponents))

	// Include stale warning if applicable
	if inv.IsStale {
		sb.WriteString("*(Cached - registry unavailable)*\n\n")
	}

	// Agents
	if len(inv.Agents) > 0 {
		agentNames := make([]string, len(inv.Agents))
		for i, agent := range inv.Agents {
			agentNames[i] = agent.Name
		}
		sb.WriteString(fmt.Sprintf("**Agents (%d):** %s\n\n",
			len(inv.Agents), strings.Join(agentNames, ", ")))
	}

	// Tools
	if len(inv.Tools) > 0 {
		toolNames := make([]string, len(inv.Tools))
		for i, tool := range inv.Tools {
			toolNames[i] = tool.Name
		}
		sb.WriteString(fmt.Sprintf("**Tools (%d):** %s\n\n",
			len(inv.Tools), strings.Join(toolNames, ", ")))
	}

	// Plugins
	if len(inv.Plugins) > 0 {
		pluginNames := make([]string, len(inv.Plugins))
		for i, plugin := range inv.Plugins {
			pluginNames[i] = plugin.Name
		}
		sb.WriteString(fmt.Sprintf("**Plugins (%d):** %s\n\n",
			len(inv.Plugins), strings.Join(pluginNames, ", ")))
	}

	// Usage guidance
	if len(inv.Agents) > 0 {
		sb.WriteString("*Use spawn_agent only with agent names from the list above.*\n")
	}

	return sb.String()
}

// EstimateTokens estimates the number of tokens the full inventory format would consume.
// Uses the heuristic: ~4 characters per token (standard LLM tokenization approximation).
//
// This estimation is intentionally conservative (may overestimate) to ensure we don't
// exceed token budgets. The estimate includes the full table format with all metadata.
//
// Returns 0 if inventory is nil.
func (f *InventoryPromptFormatter) EstimateTokens(inv *ComponentInventory) int {
	if inv == nil {
		return 0
	}

	// Generate full format and measure
	fullFormat := f.formatFullTables(inv, "")
	charCount := len(fullFormat)

	// Conservative heuristic: 4 characters per token
	// (This is approximate - actual tokenization may vary by model)
	return charCount / 4
}

// Helper functions

// agentMatchesTargetType checks if an agent supports the given target type.
func (f *InventoryPromptFormatter) agentMatchesTargetType(agent AgentSummary, targetType string) bool {
	for _, tt := range agent.TargetTypes {
		if tt == targetType {
			return true
		}
	}
	return false
}

// formatTargetTypes formats target types with bold highlighting for matches.
func (f *InventoryPromptFormatter) formatTargetTypes(targetTypes []string, missionTargetType string) string {
	if len(targetTypes) == 0 {
		return ""
	}

	formatted := make([]string, len(targetTypes))
	for i, tt := range targetTypes {
		if missionTargetType != "" && tt == missionTargetType {
			formatted[i] = fmt.Sprintf("**%s**", tt)
		} else {
			formatted[i] = tt
		}
	}

	return f.joinAndTruncate(formatted, ", ", 50)
}

// joinAndTruncate joins strings and truncates the result if too long.
func (f *InventoryPromptFormatter) joinAndTruncate(items []string, sep string, maxLen int) string {
	if len(items) == 0 {
		return ""
	}

	joined := strings.Join(items, sep)
	return f.truncateString(joined, maxLen)
}

// truncateString truncates a string to maxLen, adding "..." if truncated.
func (f *InventoryPromptFormatter) truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	if maxLen <= 3 {
		return "..."
	}

	return s[:maxLen-3] + "..."
}

// ComponentInventory, AgentSummary, ToolSummary, PluginSummary, SlotSummary,
// and MethodSummary types are now defined in inventory.go
