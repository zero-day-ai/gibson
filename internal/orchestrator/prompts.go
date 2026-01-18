package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SystemPrompt establishes the orchestrator's role and capabilities.
// This is the core instruction set that guides the LLM in making intelligent
// orchestration decisions about workflow execution.
const SystemPrompt = `You are Gibson's Mission Orchestrator - the brain that coordinates
penetration testing missions. You make intelligent decisions about what to execute next
based on the current state of the knowledge graph.

## Your Role
- Analyze graph state to understand what has been discovered
- Decide which workflow nodes to execute next
- Modify parameters based on discoveries
- Handle failures gracefully
- Know when the mission objective is complete

## Decision Guidelines
- ALWAYS respect DAG dependencies (never execute a node before its dependencies)
- Consider parallelization when multiple nodes are ready
- Prioritize high-value targets based on findings
- Be conservative with dynamic node spawning
- Stop when the objective is met, not when all nodes are done

## Output Format
Respond with a JSON Decision object. Always include reasoning.

## Decision Actions Available

1. **execute_agent** - Run the specified workflow node/agent
   - Use when a node is ready (dependencies satisfied)
   - Requires: target_node_id
   - Consider: Is this the highest priority node?

2. **skip_agent** - Skip execution of a workflow node
   - Use when a node is no longer needed based on findings
   - Requires: target_node_id
   - Reasoning: Explain why skipping is appropriate

3. **modify_params** - Modify parameters for a target node before execution
   - Use when discoveries suggest parameter changes
   - Requires: target_node_id, modifications
   - Example: Adjust target URL based on recon findings

4. **retry** - Retry execution of a failed node
   - Use when a failure is transient or can be overcome
   - Requires: target_node_id
   - Consider: Retry policies, error type, remaining attempts

5. **spawn_agent** - Dynamically create and add a new node to the workflow
   - Use SPARINGLY - only when truly needed
   - Requires: spawn_config (agent_name, description, task_config, depends_on)
   - Example: Spawn specialized agent after discovering unexpected vulnerability

6. **complete** - Mark the workflow as complete and stop orchestration
   - Use when the mission objective is achieved
   - Requires: stop_reason
   - Consider: Are all critical paths explored?

## Available Components Guidelines

You have access to the registered agents, tools, and plugins listed in the "Available Components"
section of each observation. Follow these rules:

1. **spawn_agent**: The agent_name MUST be one of the agents listed in "Available Components".
   Do NOT invent or guess agent names. Only use registered agents.

2. **Capability Matching**: When spawning agents, prefer agents whose capabilities match the
   mission objective. Check the "Capabilities" and "Target Types" columns.

3. **Health Status**: Avoid using components marked as "unhealthy" or "unavailable" unless
   no alternatives exist.

4. **No Hallucination**: If no suitable agent exists for a task, use the "complete" action
   with an appropriate stop_reason rather than inventing an agent name.

## Confidence Scoring

Provide a confidence score (0.0 to 1.0) for each decision:
- 0.9-1.0: Very confident (clear choice, strong signal)
- 0.7-0.9: Confident (good reasoning, some uncertainty)
- 0.5-0.7: Moderate (multiple valid options, choosing based on heuristics)
- 0.3-0.5: Uncertain (ambiguous state, making best guess)
- 0.0-0.3: Very uncertain (insufficient data, exploratory choice)

## Chain-of-Thought Reasoning

Always provide detailed reasoning that covers:
1. Current state assessment (what's been done, what's pending)
2. Dependency analysis (what's ready to execute)
3. Priority evaluation (which tasks are most valuable now)
4. Risk assessment (potential issues or blockers)
5. Decision rationale (why this specific action)

## Example Decision

{
  "reasoning": "Node 'recon' has completed successfully and discovered 3 open ports. Nodes 'port-scan-80' and 'port-scan-443' are now ready (dependencies satisfied). Both have equal priority, but port 443 likely has HTTPS which is higher value for credential extraction. Choosing to execute port-scan-443 first.",
  "action": "execute_agent",
  "target_node_id": "port-scan-443",
  "confidence": 0.85
}

Remember: Your decisions drive the entire mission. Be thoughtful, conservative with spawning, and always explain your reasoning clearly.`

// BuildObservationPrompt constructs a detailed context prompt from the current
// observation state. This prompt is sent to the LLM along with the system prompt
// to help it make informed orchestration decisions.
//
// The prompt is designed to be concise yet comprehensive, typically staying under
// 2k tokens to leave room for the system prompt and LLM response.
func BuildObservationPrompt(state *ObservationState) string {
	if state == nil {
		return "No observation state available. Cannot make decisions."
	}

	var sb strings.Builder
	sb.WriteString("# Current Mission State\n\n")

	// Mission overview
	sb.WriteString("## Mission Overview\n")
	sb.WriteString(fmt.Sprintf("**Objective**: %s\n", state.MissionInfo.Objective))
	sb.WriteString(fmt.Sprintf("**Mission ID**: %s\n", state.MissionInfo.ID))
	sb.WriteString(fmt.Sprintf("**Status**: %s\n", state.MissionInfo.Status))
	sb.WriteString(fmt.Sprintf("**Elapsed Time**: %s\n", state.MissionInfo.TimeElapsed))
	sb.WriteString(fmt.Sprintf("**Progress**: %d/%d nodes completed (%d failed)\n\n",
		state.GraphSummary.CompletedNodes, state.GraphSummary.TotalNodes, state.GraphSummary.FailedNodes))

	// Component inventory (if available) - show before ready nodes so LLM sees available components first
	if state.ComponentInventory != nil {
		formatter := NewInventoryPromptFormatter(WithMaxTokenBudget(500))
		// Extract mission target type from mission info if available (future enhancement)
		missionTargetType := "" // TODO: Extract from mission metadata when available
		inventorySection := formatter.Format(state.ComponentInventory, missionTargetType)
		sb.WriteString(inventorySection)
		sb.WriteString("\n")
	}

	// Ready nodes (highest priority)
	if len(state.ReadyNodes) > 0 {
		sb.WriteString("## Ready Nodes (Dependencies Satisfied)\n")
		sb.WriteString("These nodes are ready to execute immediately:\n\n")
		for _, node := range state.ReadyNodes {
			sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", node.ID, node.Type))
			if node.Name != "" {
				sb.WriteString(fmt.Sprintf("  - Name: %s\n", node.Name))
			}
			if node.AgentName != "" {
				sb.WriteString(fmt.Sprintf("  - Agent: %s\n", node.AgentName))
			}
			if node.ToolName != "" {
				sb.WriteString(fmt.Sprintf("  - Tool: %s\n", node.ToolName))
			}
			if node.Description != "" {
				sb.WriteString(fmt.Sprintf("  - Description: %s\n", node.Description))
			}
			if node.IsDynamic {
				sb.WriteString("  - (Dynamically spawned)\n")
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("## Ready Nodes\nNo nodes are currently ready to execute.\n\n")
	}

	// Running nodes
	if len(state.RunningNodes) > 0 {
		sb.WriteString("## Currently Running Nodes\n")
		for _, node := range state.RunningNodes {
			sb.WriteString(fmt.Sprintf("- **%s** (%s", node.ID, node.Type))
			if node.AgentName != "" {
				sb.WriteString(fmt.Sprintf(": %s", node.AgentName))
			}
			if node.Attempt > 1 {
				sb.WriteString(fmt.Sprintf(", attempt %d", node.Attempt))
			}
			sb.WriteString(")\n")
		}
		sb.WriteString("\n")
	}

	// Failed nodes
	if len(state.FailedNodes) > 0 {
		sb.WriteString("## Failed Nodes\n")
		for _, node := range state.FailedNodes {
			sb.WriteString(fmt.Sprintf("- **%s**", node.ID))
			if node.Name != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", node.Name))
			}
			if node.AgentName != "" {
				sb.WriteString(fmt.Sprintf(" - Agent: %s", node.AgentName))
			}
			if node.Attempt > 0 {
				sb.WriteString(fmt.Sprintf(" (attempt %d)", node.Attempt))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")

		// Include failed execution details if present
		if state.FailedExecution != nil {
			sb.WriteString("### Recent Failure Details\n")
			sb.WriteString(fmt.Sprintf("- Node: %s (%s)\n", state.FailedExecution.NodeName, state.FailedExecution.NodeID))
			if state.FailedExecution.AgentName != "" {
				sb.WriteString(fmt.Sprintf("- Agent: %s\n", state.FailedExecution.AgentName))
			}
			sb.WriteString(fmt.Sprintf("- Attempt: %d/%d\n", state.FailedExecution.Attempt, state.FailedExecution.MaxRetries))
			sb.WriteString(fmt.Sprintf("- Error: %s\n", truncate(state.FailedExecution.Error, 200)))
			sb.WriteString(fmt.Sprintf("- Can retry: %t\n", state.FailedExecution.CanRetry))
			sb.WriteString("\n")
		}
	}

	// Recent decisions for context
	if len(state.RecentDecisions) > 0 {
		sb.WriteString("## Recent Decisions\n")
		for _, dec := range state.RecentDecisions {
			sb.WriteString(fmt.Sprintf("Iteration %d: %s", dec.Iteration, dec.Action))
			if dec.Target != "" {
				sb.WriteString(fmt.Sprintf(" -> %s", dec.Target))
			}
			sb.WriteString(fmt.Sprintf(" (confidence: %.2f)\n", dec.Confidence))
			if dec.Reasoning != "" {
				sb.WriteString(fmt.Sprintf("  Reasoning: %s\n", dec.Reasoning))
			}
		}
		sb.WriteString("\n")
	}

	// Resource constraints
	sb.WriteString("## Resource Constraints\n")
	sb.WriteString(fmt.Sprintf("- Max concurrent: %d\n", state.ResourceConstraints.MaxConcurrent))
	sb.WriteString(fmt.Sprintf("- Currently running: %d\n", state.ResourceConstraints.CurrentRunning))
	sb.WriteString(fmt.Sprintf("- Total iterations: %d\n", state.ResourceConstraints.TotalIterations))
	if state.ResourceConstraints.RemainingRetries > 0 {
		sb.WriteString(fmt.Sprintf("- Failed nodes available for retry: %d\n", state.ResourceConstraints.RemainingRetries))
	}
	if state.ResourceConstraints.ExecutionBudget != nil {
		if state.ResourceConstraints.ExecutionBudget.MaxExecutions > 0 {
			sb.WriteString(fmt.Sprintf("- Remaining executions: %d/%d\n",
				state.ResourceConstraints.ExecutionBudget.RemainingExecutions,
				state.ResourceConstraints.ExecutionBudget.MaxExecutions))
		}
	}
	sb.WriteString("\n")

	// Decision prompt
	sb.WriteString("## What Should We Do Next?\n\n")
	sb.WriteString("Based on the current state:\n")
	sb.WriteString("1. Analyze what has been accomplished\n")
	sb.WriteString("2. Consider which ready nodes are highest priority\n")
	sb.WriteString("3. Evaluate if any failed nodes should be retried\n")
	sb.WriteString("4. Determine if any parameters should be modified based on findings\n")
	sb.WriteString("5. Assess if the mission objective has been achieved\n")
	sb.WriteString("6. Consider if dynamic agent spawning would be valuable (use sparingly)\n\n")
	sb.WriteString("Provide your decision as a JSON object with detailed reasoning.\n")

	return sb.String()
}

// BuildDecisionSchema returns the JSON schema for the Decision struct.
// This schema can be used with LLM providers that support structured output
// (e.g., Anthropic's Claude with response_format, OpenAI's function calling).
//
// The schema enforces proper structure and validation constraints at the LLM level.
func BuildDecisionSchema() string {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"title":   "Orchestrator Decision",
		"description": "The orchestrator's decision about what to execute next in the workflow. " +
			"Must include reasoning, action, and confidence. Additional fields depend on the action type.",
		"required": []string{"reasoning", "action", "confidence"},
		"properties": map[string]interface{}{
			"reasoning": map[string]interface{}{
				"type": "string",
				"description": "Chain-of-thought reasoning explaining why this decision was made. " +
					"Should cover: current state, dependencies, priorities, risks, and rationale. " +
					"Minimum 50 characters.",
				"minLength": 50,
			},
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{
					"execute_agent",
					"skip_agent",
					"modify_params",
					"retry",
					"spawn_agent",
					"complete",
				},
				"description": "The action to take. Must be one of the predefined actions.",
			},
			"target_node_id": map[string]interface{}{
				"type": "string",
				"description": "The workflow node ID to act upon. " +
					"Required for: execute_agent, skip_agent, modify_params, retry",
			},
			"modifications": map[string]interface{}{
				"type": "object",
				"description": "Parameter modifications for the target node. " +
					"Required for: modify_params action. " +
					"Keys are parameter names, values are new parameter values.",
				"additionalProperties": true,
			},
			"spawn_config": map[string]interface{}{
				"type":        "object",
				"description": "Configuration for spawning a new workflow node. Required for: spawn_agent action.",
				"required":    []string{"agent_name", "description", "task_config", "depends_on"},
				"properties": map[string]interface{}{
					"agent_name": map[string]interface{}{
						"type":        "string",
						"description": "The type of agent to spawn (must exist in agent registry)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Human-readable explanation of why this agent is being spawned",
						"minLength":   20,
					},
					"task_config": map[string]interface{}{
						"type":                 "object",
						"description":          "Configuration parameters for the spawned agent",
						"additionalProperties": true,
					},
					"depends_on": map[string]interface{}{
						"type": "array",
						"description": "List of node IDs that must complete before this spawned node runs. " +
							"Can be empty array if no dependencies.",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
			"confidence": map[string]interface{}{
				"type":        "number",
				"description": "Confidence score between 0.0 and 1.0. Higher = more confident in this decision.",
				"minimum":     0.0,
				"maximum":     1.0,
			},
			"stop_reason": map[string]interface{}{
				"type": "string",
				"description": "Explanation of why the workflow is being marked complete. " +
					"Required for: complete action. " +
					"Should explain that the objective was achieved.",
				"minLength": 20,
			},
		},
		"allOf": []interface{}{
			// Conditional validation: execute_agent, skip_agent, retry require target_node_id
			map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{
						"action": map[string]interface{}{
							"enum": []string{"execute_agent", "skip_agent", "retry"},
						},
					},
				},
				"then": map[string]interface{}{
					"required": []string{"target_node_id"},
				},
			},
			// Conditional validation: modify_params requires target_node_id and modifications
			map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{
						"action": map[string]interface{}{
							"const": "modify_params",
						},
					},
				},
				"then": map[string]interface{}{
					"required": []string{"target_node_id", "modifications"},
				},
			},
			// Conditional validation: spawn_agent requires spawn_config
			map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{
						"action": map[string]interface{}{
							"const": "spawn_agent",
						},
					},
				},
				"then": map[string]interface{}{
					"required": []string{"spawn_config"},
				},
			},
			// Conditional validation: complete requires stop_reason
			map[string]interface{}{
				"if": map[string]interface{}{
					"properties": map[string]interface{}{
						"action": map[string]interface{}{
							"const": "complete",
						},
					},
				},
				"then": map[string]interface{}{
					"required": []string{"stop_reason"},
				},
			},
		},
	}

	// Marshal to pretty JSON
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		// Should never happen with static schema, but handle gracefully
		return "{}"
	}

	return string(schemaJSON)
}

// FormatDecisionExample returns a formatted example of a valid Decision JSON.
// This can be included in prompts to guide the LLM toward proper formatting.
func FormatDecisionExample() string {
	example := map[string]interface{}{
		"reasoning": "Node 'recon' completed successfully and discovered 3 open ports. " +
			"Nodes 'port-scan-80' and 'port-scan-443' are ready (dependencies satisfied). " +
			"Port 443 (HTTPS) is higher priority for credential extraction. " +
			"No resource constraints. Executing port-scan-443.",
		"action":         "execute_agent",
		"target_node_id": "port-scan-443",
		"confidence":     0.85,
	}

	exampleJSON, _ := json.MarshalIndent(example, "", "  ")
	return string(exampleJSON)
}

// FormatCompleteExample returns an example of a complete decision.
func FormatCompleteExample() string {
	example := map[string]interface{}{
		"reasoning": "All reconnaissance, scanning, and exploitation nodes have completed. " +
			"We discovered 5 high-severity findings including prompt injection and data extraction vulnerabilities. " +
			"The mission objective to 'discover security vulnerabilities in target LLM' has been achieved. " +
			"No additional nodes would provide significant value. Marking mission complete.",
		"action":      "complete",
		"confidence":  0.95,
		"stop_reason": "Mission objective achieved: discovered 5 high-severity vulnerabilities in target system",
	}

	exampleJSON, _ := json.MarshalIndent(example, "", "  ")
	return string(exampleJSON)
}

// FormatSpawnExample returns an example of a spawn_agent decision.
func FormatSpawnExample() string {
	example := map[string]interface{}{
		"reasoning": "Recon discovered an unexpected GraphQL endpoint at /graphql. " +
			"This was not part of the original workflow. " +
			"GraphQL often has introspection vulnerabilities. " +
			"Spawning a specialized GraphQL introspection agent to explore this attack surface.",
		"action":     "spawn_agent",
		"confidence": 0.78,
		"spawn_config": map[string]interface{}{
			"agent_name":  "graphql-introspector",
			"description": "Spawned to analyze unexpected GraphQL endpoint discovered during reconnaissance",
			"task_config": map[string]interface{}{
				"endpoint": "https://target.example.com/graphql",
				"goal":     "Perform introspection and identify schema vulnerabilities",
			},
			"depends_on": []string{"recon"},
		},
	}

	exampleJSON, _ := json.MarshalIndent(example, "", "  ")
	return string(exampleJSON)
}

// BuildFullPrompt combines the system prompt, observation prompt, and examples
// into a complete prompt ready to send to the LLM.
//
// This is a convenience function that ensures proper formatting and ordering.
func BuildFullPrompt(state *ObservationState, includeExamples bool) string {
	var sb strings.Builder

	// System prompt
	sb.WriteString(SystemPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("---\n\n")

	// Observation state
	sb.WriteString(BuildObservationPrompt(state))
	sb.WriteString("\n")

	// Optional examples
	if includeExamples {
		sb.WriteString("---\n\n")
		sb.WriteString("## Example Decisions\n\n")
		sb.WriteString("### Example 1: Execute Agent\n```json\n")
		sb.WriteString(FormatDecisionExample())
		sb.WriteString("\n```\n\n")
		sb.WriteString("### Example 2: Complete Mission\n```json\n")
		sb.WriteString(FormatCompleteExample())
		sb.WriteString("\n```\n\n")
		sb.WriteString("### Example 3: Spawn Agent\n```json\n")
		sb.WriteString(FormatSpawnExample())
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("---\n\n")
	sb.WriteString("Now, provide your decision based on the current mission state above.\n")

	return sb.String()
}

// EstimatePromptTokens provides a rough estimate of token count for the prompt.
// This uses a simple heuristic: ~4 characters per token (GPT standard).
//
// This is useful for staying under context window limits and optimizing prompt size.
func EstimatePromptTokens(prompt string) int {
	return len(prompt) / 4
}

// truncate is a helper function to truncate strings to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}
