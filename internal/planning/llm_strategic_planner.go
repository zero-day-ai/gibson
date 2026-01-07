package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/types"
	"github.com/zero-day-ai/gibson/internal/workflow"
)

// LLMStrategicPlanner implements StrategicPlanner using an LLM for intelligent plan generation.
// It creates a comprehensive planning prompt that includes workflow structure, bounds constraints,
// target information, memory context, and the original goal, then parses the LLM's JSON response
// into a validated strategic plan.
//
// The planner enforces:
//   - Planning token budget (terminates if exceeded)
//   - Workflow bounds (rejects steps not in workflow)
//   - Resource constraints (respects max parallelism, budgets)
//   - Memory-informed decisions (prioritizes successful techniques)
//
// Implementation notes:
//   - Uses harness.Complete() for LLM access (slot-based model selection)
//   - Tracks token usage and enforces budget limits
//   - Falls back gracefully on LLM failures (returns error, caller can use DefaultOrderPlanner)
//   - Validates every generated step against PlanningBounds
//   - Includes original goal verbatim in prompt to prevent drift
type LLMStrategicPlanner struct {
	// harness provides access to LLM capabilities and token tracking
	harness harness.AgentHarness

	// slotName is the LLM slot to use for planning (e.g., "primary", "reasoning")
	slotName string

	// budgetManager tracks token usage against planning budget
	budgetManager BudgetManager

	// pricingConfig provides model pricing information for cost calculations
	pricingConfig *llm.PricingConfig
}

// NewLLMStrategicPlanner creates a new LLM-based strategic planner.
//
// Parameters:
//   - h: Agent harness providing LLM access
//   - slotName: LLM slot to use for planning (typically "primary")
//   - budgetMgr: Budget manager for tracking planning token usage
//
// Returns a configured planner ready to generate plans.
func NewLLMStrategicPlanner(h harness.AgentHarness, slotName string, budgetMgr BudgetManager) *LLMStrategicPlanner {
	return &LLMStrategicPlanner{
		harness:       h,
		slotName:      slotName,
		budgetManager: budgetMgr,
		pricingConfig: llm.DefaultPricing(),
	}
}

// Plan generates an intelligent execution strategy using LLM reasoning.
//
// The planning process:
//  1. Builds a comprehensive prompt with workflow structure, bounds, target, memory context
//  2. Queries the LLM for a strategic plan in JSON format
//  3. Parses the JSON response into a StrategicPlan struct
//  4. Validates all generated steps against PlanningBounds
//  5. Validates budget allocation doesn't exceed constraints
//  6. Records token usage against planning budget
//
// The LLM prompt instructs the model to:
//   - Analyze the target and available attack strategies
//   - Consider memory context for what worked/failed before
//   - Order steps by likelihood of success
//   - Skip nodes unlikely to yield findings
//   - Identify parallel execution opportunities
//   - Allocate token budgets based on step complexity
//
// Returns an error if:
//   - Planning budget is exhausted
//   - LLM call fails
//   - Response cannot be parsed as valid JSON
//   - Generated plan violates bounds or constraints
//   - Context is cancelled
func (p *LLMStrategicPlanner) Plan(ctx context.Context, input StrategicPlanInput) (*StrategicPlan, error) {
	// Validate inputs
	if err := p.validateInput(input); err != nil {
		return nil, err
	}

	// Check if we have sufficient planning budget
	if input.Budget != nil && input.Budget.StrategicPlanning <= 0 {
		return nil, NewPlanningError(ErrorTypeBudgetExhausted,
			"no strategic planning budget available")
	}

	// Build planning prompt
	prompt, err := p.buildPlanningPrompt(input)
	if err != nil {
		return nil, WrapPlanningError(ErrorTypeInternal,
			"failed to build planning prompt", err)
	}

	// Prepare LLM messages
	messages := []llm.Message{
		llm.NewSystemMessage(p.buildSystemPrompt()),
		llm.NewUserMessage(prompt),
	}

	// Check if we can afford the LLM call (rough estimate: prompt is ~2k tokens)
	estimatedPromptTokens := len(prompt) / 4 // Rough heuristic: ~4 chars per token
	if input.Budget != nil && !p.budgetManager.CanAfford(PhaseStrategicPlanning, estimatedPromptTokens) {
		return nil, NewPlanningError(ErrorTypeBudgetExhausted,
			"insufficient budget for planning LLM call").
			WithContext("estimated_tokens", estimatedPromptTokens).
			WithContext("remaining", input.Budget.StrategicPlanning)
	}

	// Call LLM for plan generation
	resp, err := p.harness.Complete(ctx, p.slotName, messages,
		harness.WithTemperature(0.3), // Lower temperature for more deterministic planning
		harness.WithMaxTokens(4000),  // Allow substantial response for detailed plans
	)
	if err != nil {
		return nil, WrapPlanningError(ErrorTypeInternal,
			"LLM completion failed during planning", err)
	}

	// Record token usage against planning budget
	tokensUsed := resp.Usage.TotalTokens
	if input.Budget != nil && p.budgetManager != nil {
		// Calculate cost from token usage
		costUsed := 0.0
		if p.pricingConfig != nil {
			// Extract provider from model string (e.g., "anthropic/claude-3" -> "anthropic")
			provider, model := parseProviderModel(resp.Model)
			usage := llm.TokenUsage{
				InputTokens:  resp.Usage.PromptTokens,
				OutputTokens: resp.Usage.CompletionTokens,
			}
			if cost, err := p.pricingConfig.CalculateCost(provider, model, usage); err == nil {
				costUsed = cost
			}
		}
		if err := p.budgetManager.Consume(PhaseStrategicPlanning, tokensUsed, costUsed); err != nil {
			// Budget exhausted during planning
			return nil, WrapPlanningError(ErrorTypeBudgetExhausted,
				"planning budget exhausted during LLM call", err).
				WithContext("tokens_used", tokensUsed).
				WithContext("cost_used", costUsed)
		}
	}

	// Parse LLM response into plan structure
	plan, err := p.parsePlanResponse(resp.Message.Content, input)
	if err != nil {
		return nil, WrapPlanningError(ErrorTypeInternal,
			"failed to parse LLM planning response", err).
			WithContext("response_preview", truncate(resp.Message.Content, 200))
	}

	// Validate generated plan against bounds
	if err := p.validatePlan(plan, input.Bounds, input.Budget); err != nil {
		return nil, WrapPlanningError(ErrorTypeConstraintViolation,
			"generated plan violates constraints", err)
	}

	// Set plan metadata
	now := time.Now()
	plan.ID = types.NewID()
	plan.Status = PlanStatusDraft
	plan.ReplanCount = 0
	plan.CreatedAt = now
	plan.UpdatedAt = now

	return plan, nil
}

// validateInput checks that all required input parameters are valid.
func (p *LLMStrategicPlanner) validateInput(input StrategicPlanInput) error {
	if input.Workflow == nil {
		return NewInvalidParameterError("workflow cannot be nil")
	}

	if input.Bounds == nil {
		return NewInvalidParameterError("bounds cannot be nil")
	}

	if len(input.Workflow.Nodes) == 0 {
		return NewInvalidParameterError("workflow must contain at least one node")
	}

	if input.OriginalGoal == "" {
		return NewInvalidParameterError("original goal cannot be empty")
	}

	return nil
}

// buildSystemPrompt creates the system message that defines the planner's role.
func (p *LLMStrategicPlanner) buildSystemPrompt() string {
	return `You are a strategic security testing planner for an AI security framework.

Your role is to analyze a workflow definition, target information, and historical data to generate an optimized execution plan that maximizes the likelihood of discovering security vulnerabilities.

You must:
1. Only include workflow nodes that are defined in the workflow (no invented steps)
2. Consider historical memory data about what worked and failed on similar targets
3. Order steps by likelihood of success based on target characteristics
4. Allocate token budgets proportionally to step complexity and importance
5. Identify nodes that can run in parallel without dependencies
6. Skip nodes that are unlikely to yield findings given the target type
7. Provide clear rationale for all decisions

Your response must be valid JSON matching the schema provided in the user message.`
}

// buildPlanningPrompt constructs the detailed planning prompt with all context.
func (p *LLMStrategicPlanner) buildPlanningPrompt(input StrategicPlanInput) (string, error) {
	var b strings.Builder

	// Original goal (verbatim for anchoring)
	b.WriteString("## Mission Goal\n\n")
	b.WriteString(input.OriginalGoal)
	b.WriteString("\n\n")

	// Target information
	b.WriteString("## Target Information\n\n")
	b.WriteString(fmt.Sprintf("- **Type**: %s\n", input.Target.Type))
	b.WriteString(fmt.Sprintf("- **Name**: %s\n", input.Target.Name))
	if input.Target.URL != "" {
		b.WriteString(fmt.Sprintf("- **URL**: %s\n", input.Target.URL))
	}
	if input.Target.Provider != "" {
		b.WriteString(fmt.Sprintf("- **Provider**: %s\n", input.Target.Provider))
	}
	b.WriteString("\n")

	// Workflow structure
	b.WriteString("## Available Workflow Nodes\n\n")
	b.WriteString(p.formatWorkflowNodes(input.Workflow))
	b.WriteString("\n")

	// Planning constraints
	b.WriteString("## Planning Constraints\n\n")
	b.WriteString(p.formatConstraints(input.Bounds, input.Budget))
	b.WriteString("\n")

	// Memory context (if available)
	if input.MemoryContext != nil && input.MemoryContext.ResultCount > 0 {
		b.WriteString("## Historical Memory Context\n\n")
		b.WriteString(p.formatMemoryContext(input.MemoryContext))
		b.WriteString("\n")
	}

	// Response schema
	b.WriteString("## Required Response Format\n\n")
	b.WriteString(p.formatResponseSchema(input.Budget))

	return b.String(), nil
}

// formatWorkflowNodes formats workflow nodes for the prompt.
func (p *LLMStrategicPlanner) formatWorkflowNodes(wf *workflow.Workflow) string {
	var b strings.Builder

	b.WriteString("The following nodes are available in the workflow:\n\n")

	for nodeID, node := range wf.Nodes {
		if node == nil {
			continue
		}

		b.WriteString(fmt.Sprintf("**%s** (%s)\n", nodeID, node.Type))

		switch node.Type {
		case workflow.NodeTypeAgent:
			b.WriteString(fmt.Sprintf("  - Agent: %s\n", node.AgentName))
		case workflow.NodeTypeTool:
			b.WriteString(fmt.Sprintf("  - Tool: %s\n", node.ToolName))
		case workflow.NodeTypePlugin:
			b.WriteString(fmt.Sprintf("  - Plugin: %s\n", node.PluginName))
		}

		if len(node.Dependencies) > 0 {
			b.WriteString(fmt.Sprintf("  - Dependencies: %s\n", strings.Join(node.Dependencies, ", ")))
		}

		if node.Timeout > 0 {
			b.WriteString(fmt.Sprintf("  - Timeout: %s\n", node.Timeout))
		}

		b.WriteString("\n")
	}

	return b.String()
}

// formatConstraints formats planning bounds and budget constraints.
func (p *LLMStrategicPlanner) formatConstraints(bounds *PlanningBounds, budget *BudgetAllocation) string {
	var b strings.Builder

	b.WriteString("You must respect these constraints:\n\n")

	if bounds.MaxParallel > 1 {
		b.WriteString(fmt.Sprintf("- **Max Parallel Execution**: %d nodes can run concurrently\n", bounds.MaxParallel))
	} else {
		b.WriteString("- **Execution Mode**: Sequential only (no parallelism)\n")
	}

	if bounds.MaxDuration > 0 {
		b.WriteString(fmt.Sprintf("- **Max Duration**: %s total execution time\n", bounds.MaxDuration))
	}

	if budget != nil && budget.Execution > 0 {
		b.WriteString(fmt.Sprintf("- **Execution Token Budget**: %d tokens available for step execution\n", budget.Execution))
	}

	if len(bounds.ApprovalGates) > 0 {
		b.WriteString(fmt.Sprintf("- **Approval Gates**: These nodes require approval: %s\n", strings.Join(bounds.ApprovalGates, ", ")))
	}

	return b.String()
}

// formatMemoryContext formats historical memory data for the prompt.
func (p *LLMStrategicPlanner) formatMemoryContext(mem *MemoryQueryResult) string {
	var b strings.Builder

	b.WriteString("Based on analysis of similar past missions:\n\n")

	// Successful techniques
	if len(mem.SuccessfulTechniques) > 0 {
		b.WriteString("**Successful Techniques** (prioritize these):\n")
		for i, tech := range mem.SuccessfulTechniques {
			if i >= 5 { // Limit to top 5
				break
			}
			successPct := int(tech.SuccessRate * 100)
			b.WriteString(fmt.Sprintf("- %s (success rate: %d%%, avg cost: %d tokens)\n",
				tech.Technique, successPct, tech.AvgTokenCost))
		}
		b.WriteString("\n")
	}

	// Failed approaches
	if len(mem.FailedApproaches) > 0 {
		b.WriteString("**Failed Approaches** (avoid these):\n")
		for i, fail := range mem.FailedApproaches {
			if i >= 3 { // Limit to top 3 failures
				break
			}
			b.WriteString(fmt.Sprintf("- %s: %s\n", fail.Approach, fail.FailureReason))
		}
		b.WriteString("\n")
	}

	// Similar missions summary
	if len(mem.SimilarMissions) > 0 {
		successCount := 0
		totalFindings := 0
		for _, mission := range mem.SimilarMissions {
			if mission.Success {
				successCount++
			}
			totalFindings += mission.FindingsCount
		}
		b.WriteString(fmt.Sprintf("**Similar Missions**: %d past missions analyzed, %d successful, %d total findings discovered\n\n",
			len(mem.SimilarMissions), successCount, totalFindings))
	}

	return b.String()
}

// formatResponseSchema defines the expected JSON response structure.
func (p *LLMStrategicPlanner) formatResponseSchema(budget *BudgetAllocation) string {
	executionBudget := 0
	if budget != nil {
		executionBudget = budget.Execution
	}

	schema := fmt.Sprintf(`Respond with a JSON object matching this schema:

{
  "ordered_steps": [
    {
      "node_id": "string (must match a workflow node ID)",
      "priority": "number (lower executes first, equal priorities can run in parallel)",
      "allocated_tokens": "number (budget for this step, sum must not exceed %d)",
      "rationale": "string (why this step is included and prioritized this way)"
    }
  ],
  "skipped_nodes": [
    {
      "node_id": "string (workflow node being skipped)",
      "reason": "string (why this node is skipped)",
      "condition": "string (when to reconsider this node, or empty if permanent skip)"
    }
  ],
  "parallel_groups": [
    ["node_id1", "node_id2"]  // Groups of nodes that can run concurrently
  ],
  "rationale": "string (overall strategy explanation)",
  "memory_influences": ["string (which memory insights influenced decisions)"]
}

IMPORTANT:
- Every node_id must exactly match a workflow node ID from the available nodes
- Sum of allocated_tokens across all steps must not exceed %d tokens
- Nodes with the same priority can execute in parallel (if dependencies allow)
- Consider memory context to prioritize proven successful techniques
- Skip nodes that are unlikely to succeed given the target type
- Allocate more tokens to complex/important steps, fewer to simple reconnaissance`, executionBudget, executionBudget)

	return schema
}

// parsePlanResponse parses the LLM's JSON response into a StrategicPlan.
func (p *LLMStrategicPlanner) parsePlanResponse(content string, input StrategicPlanInput) (*StrategicPlan, error) {
	// The response might be wrapped in markdown code blocks, extract JSON
	jsonContent := extractJSON(content)

	// Parse into intermediate structure
	var response struct {
		OrderedSteps []struct {
			NodeID          string `json:"node_id"`
			Priority        int    `json:"priority"`
			AllocatedTokens int    `json:"allocated_tokens"`
			Rationale       string `json:"rationale"`
		} `json:"ordered_steps"`
		SkippedNodes []struct {
			NodeID    string `json:"node_id"`
			Reason    string `json:"reason"`
			Condition string `json:"condition"`
		} `json:"skipped_nodes"`
		ParallelGroups   [][]string `json:"parallel_groups"`
		Rationale        string     `json:"rationale"`
		MemoryInfluences []string   `json:"memory_influences"`
	}

	if err := json.Unmarshal([]byte(jsonContent), &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Build StrategicPlan from response
	plan := &StrategicPlan{
		OrderedSteps:     make([]PlannedStep, 0, len(response.OrderedSteps)),
		SkippedNodes:     make([]SkipDecision, 0, len(response.SkippedNodes)),
		ParallelGroups:   response.ParallelGroups,
		StepBudgets:      make(map[string]int),
		Rationale:        response.Rationale,
		MemoryInfluences: response.MemoryInfluences,
	}

	// Convert ordered steps
	totalTokens := 0
	for _, step := range response.OrderedSteps {
		plannedStep := PlannedStep{
			NodeID:          step.NodeID,
			Priority:        step.Priority,
			AllocatedTokens: step.AllocatedTokens,
			Rationale:       step.Rationale,
		}
		plan.OrderedSteps = append(plan.OrderedSteps, plannedStep)
		plan.StepBudgets[step.NodeID] = step.AllocatedTokens
		totalTokens += step.AllocatedTokens
	}

	// Convert skipped nodes
	for _, skip := range response.SkippedNodes {
		skipDecision := SkipDecision{
			NodeID:    skip.NodeID,
			Reason:    skip.Reason,
			Condition: skip.Condition,
		}
		plan.SkippedNodes = append(plan.SkippedNodes, skipDecision)
	}

	// Set estimated tokens and duration
	plan.EstimatedTokens = totalTokens
	plan.EstimatedDuration = p.calculateEstimatedDuration(plan, input.Workflow)

	return plan, nil
}

// validatePlan ensures the generated plan respects all constraints.
func (p *LLMStrategicPlanner) validatePlan(plan *StrategicPlan, bounds *PlanningBounds, budget *BudgetAllocation) error {
	// Validate all steps reference workflow nodes
	for i, step := range plan.OrderedSteps {
		if !bounds.ContainsNode(step.NodeID) {
			return fmt.Errorf("step %d references invalid node %q (not in workflow)", i, step.NodeID)
		}
	}

	// Validate all skipped nodes reference workflow nodes
	for i, skip := range plan.SkippedNodes {
		if !bounds.ContainsNode(skip.NodeID) {
			return fmt.Errorf("skip decision %d references invalid node %q (not in workflow)", i, skip.NodeID)
		}
	}

	// Validate parallel groups
	for i, group := range plan.ParallelGroups {
		if len(group) > bounds.MaxParallel {
			return fmt.Errorf("parallel group %d contains %d nodes, exceeds max parallelism of %d",
				i, len(group), bounds.MaxParallel)
		}

		for _, nodeID := range group {
			if !bounds.ContainsNode(nodeID) {
				return fmt.Errorf("parallel group %d references invalid node %q", i, nodeID)
			}
		}
	}

	// Validate token budget
	if budget != nil && budget.Execution > 0 {
		if plan.EstimatedTokens > budget.Execution {
			return fmt.Errorf("plan estimated tokens (%d) exceeds execution budget (%d)",
				plan.EstimatedTokens, budget.Execution)
		}
	}

	return nil
}

// calculateEstimatedDuration estimates total execution time from planned steps.
func (p *LLMStrategicPlanner) calculateEstimatedDuration(plan *StrategicPlan, wf *workflow.Workflow) time.Duration {
	var totalDuration time.Duration

	// Sum timeouts of all planned steps
	for _, step := range plan.OrderedSteps {
		if node, ok := wf.Nodes[step.NodeID]; ok && node != nil {
			totalDuration += node.Timeout
		}
	}

	return totalDuration
}

// extractJSON extracts JSON content from a string that might be wrapped in markdown code blocks.
func extractJSON(content string) string {
	// Check for markdown code blocks with json language hint
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json")
		if start != -1 {
			start += len("```json")
			end := strings.Index(content[start:], "```")
			if end != -1 {
				return strings.TrimSpace(content[start : start+end])
			}
		}
	}

	// Check for plain markdown code blocks
	if strings.Contains(content, "```") {
		start := strings.Index(content, "```")
		if start != -1 {
			start += len("```")
			end := strings.Index(content[start:], "```")
			if end != -1 {
				extracted := strings.TrimSpace(content[start : start+end])
				// Only use this if it looks like JSON (starts with { or [)
				if strings.HasPrefix(extracted, "{") || strings.HasPrefix(extracted, "[") {
					return extracted
				}
			}
		}
	}

	// No code blocks found, return content as-is (might already be plain JSON)
	return strings.TrimSpace(content)
}

// truncate truncates a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseProviderModel extracts provider and model from a model string.
// Handles formats like "anthropic/claude-3", "claude-3", "openai/gpt-4", "gpt-4".
func parseProviderModel(modelStr string) (provider, model string) {
	parts := strings.SplitN(modelStr, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// Try to infer provider from model name
	if strings.Contains(modelStr, "claude") {
		return "anthropic", modelStr
	} else if strings.Contains(modelStr, "gpt") {
		return "openai", modelStr
	} else if strings.Contains(modelStr, "gemini") {
		return "google", modelStr
	}
	// Default to returning empty provider and full model string
	return "", modelStr
}
