package plan

import (
	"fmt"
	"strings"
)

// RiskRule defines a rule for evaluating risk in an execution step.
// Each rule evaluates a specific aspect of risk and returns a risk level
// and explanation for that aspect.
type RiskRule struct {
	// Name is the identifier for this rule
	Name string

	// Description provides details about what this rule evaluates
	Description string

	// Evaluate performs the risk evaluation for a step.
	// It returns the risk level determined by this rule and an explanation.
	Evaluate func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string)
}

// RiskAssessor evaluates execution steps and plans to determine risk levels
// and approval requirements. It applies a configurable set of risk rules
// to make these determinations.
type RiskAssessor struct {
	rules []RiskRule
}

// RiskAssessorOption is a functional option for configuring a RiskAssessor.
type RiskAssessorOption func(*RiskAssessor)

// NewRiskAssessor creates a new RiskAssessor with the specified options.
// If no options are provided, the assessor will have no rules and will
// assess all steps as low risk. Use WithDefaultRules() to get a sensible
// default configuration.
func NewRiskAssessor(opts ...RiskAssessorOption) *RiskAssessor {
	assessor := &RiskAssessor{
		rules: make([]RiskRule, 0),
	}

	for _, opt := range opts {
		opt(assessor)
	}

	return assessor
}

// WithRule adds a custom risk rule to the assessor.
func WithRule(rule RiskRule) RiskAssessorOption {
	return func(a *RiskAssessor) {
		a.rules = append(a.rules, rule)
	}
}

// WithDefaultRules configures the assessor with a default set of risk rules
// that cover common risk scenarios for cybersecurity operations.
func WithDefaultRules() RiskAssessorOption {
	return func(a *RiskAssessor) {
		a.rules = append(a.rules, defaultRiskRules()...)
	}
}

// AssessStep evaluates a single execution step and returns a comprehensive
// risk assessment. It applies all configured rules to the step and aggregates
// their results to determine the overall risk level.
//
// The overall risk level is determined by taking the highest risk level
// reported by any rule. If any rule reports critical risk, the overall
// level is critical. If no rules report critical but any report high,
// the overall level is high, and so on.
//
// Steps with high or critical risk levels will have RequiresApproval set to true.
func (r *RiskAssessor) AssessStep(step *ExecutionStep, ctx RiskContext) RiskAssessment {
	assessment := RiskAssessment{
		Level:            RiskLevelLow,
		RequiresApproval: false,
		Rationale:        "",
		Factors:          make([]RiskFactor, 0, len(r.rules)),
	}

	// If there are no rules, return a low risk assessment
	if len(r.rules) == 0 {
		assessment.Rationale = "No risk rules configured"
		return assessment
	}

	// Evaluate each rule
	var rationales []string
	highestLevel := RiskLevelLow

	for _, rule := range r.rules {
		level, explanation := rule.Evaluate(step, ctx)

		// Update highest level if this rule found higher risk
		if riskLevelOrdinal(level) > riskLevelOrdinal(highestLevel) {
			highestLevel = level
		}

		// Create a risk factor for this rule
		factor := RiskFactor{
			Name:        rule.Name,
			Description: rule.Description,
			Weight:      1.0, // All rules weighted equally by default
			Value:       riskLevelToScore(level),
		}
		assessment.Factors = append(assessment.Factors, factor)

		// Collect explanation if this rule found elevated risk
		if level != RiskLevelLow {
			rationales = append(rationales, fmt.Sprintf("%s: %s", rule.Name, explanation))
		}
	}

	// Set the overall level
	assessment.Level = highestLevel

	// Determine if approval is required
	assessment.RequiresApproval = highestLevel.IsHighRisk()

	// Build the rationale string
	if len(rationales) > 0 {
		assessment.Rationale = strings.Join(rationales, "; ")
	} else {
		assessment.Rationale = "All risk rules passed with low risk"
	}

	return assessment
}

// AssessPlan evaluates an entire execution plan and returns a summary of
// the risk profile across all steps. It assesses each step individually
// and aggregates the results to provide an overall plan risk summary.
func (r *RiskAssessor) AssessPlan(plan *ExecutionPlan, ctx RiskContext) PlanRiskSummary {
	summary := PlanRiskSummary{
		OverallLevel:     RiskLevelLow,
		HighRiskSteps:    0,
		CriticalSteps:    0,
		ApprovalRequired: false,
		Factors:          make([]RiskFactor, 0),
	}

	// Track factor aggregation by name
	factorMap := make(map[string]*RiskFactor)
	highestLevel := RiskLevelLow

	// Assess each step
	for i := range plan.Steps {
		stepAssessment := r.AssessStep(&plan.Steps[i], ctx)

		// Update highest level
		if riskLevelOrdinal(stepAssessment.Level) > riskLevelOrdinal(highestLevel) {
			highestLevel = stepAssessment.Level
		}

		// Count high and critical risk steps
		if stepAssessment.Level == RiskLevelHigh {
			summary.HighRiskSteps++
		} else if stepAssessment.Level == RiskLevelCritical {
			summary.CriticalSteps++
			summary.HighRiskSteps++ // Critical is also counted as high risk
		}

		// Track if any step requires approval
		if stepAssessment.RequiresApproval {
			summary.ApprovalRequired = true
		}

		// Aggregate factors
		for _, factor := range stepAssessment.Factors {
			if existing, exists := factorMap[factor.Name]; exists {
				// Average the values
				existing.Value = (existing.Value + factor.Value) / 2
			} else {
				// Create a new factor
				factorCopy := factor
				factorMap[factor.Name] = &factorCopy
			}
		}
	}

	// Set overall level
	summary.OverallLevel = highestLevel

	// Convert factor map to slice
	summary.Factors = make([]RiskFactor, 0, len(factorMap))
	for _, factor := range factorMap {
		summary.Factors = append(summary.Factors, *factor)
	}

	return summary
}

// riskLevelOrdinal returns a numeric value for comparing risk levels.
// Higher numbers indicate higher risk.
func riskLevelOrdinal(level RiskLevel) int {
	switch level {
	case RiskLevelLow:
		return 0
	case RiskLevelMedium:
		return 1
	case RiskLevelHigh:
		return 2
	case RiskLevelCritical:
		return 3
	default:
		return 0
	}
}

// riskLevelToScore converts a risk level to a numeric score between 0.0 and 1.0.
func riskLevelToScore(level RiskLevel) float64 {
	switch level {
	case RiskLevelLow:
		return 0.25
	case RiskLevelMedium:
		return 0.50
	case RiskLevelHigh:
		return 0.75
	case RiskLevelCritical:
		return 1.0
	default:
		return 0.0
	}
}

// defaultRiskRules returns a set of default risk rules for common scenarios
// in cybersecurity operations.
func defaultRiskRules() []RiskRule {
	return []RiskRule{
		{
			Name:        "destructive_operation",
			Description: "Evaluates whether the step performs destructive operations",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				// Check for destructive keywords in step name or description
				destructiveKeywords := []string{
					"delete", "remove", "destroy", "wipe", "format",
					"drop", "truncate", "kill", "terminate", "shutdown",
				}

				text := strings.ToLower(step.Name + " " + step.Description)
				for _, keyword := range destructiveKeywords {
					if strings.Contains(text, keyword) {
						return RiskLevelHigh, fmt.Sprintf("step contains destructive operation: %s", keyword)
					}
				}

				return RiskLevelLow, ""
			},
		},
		{
			Name:        "data_exfiltration",
			Description: "Evaluates whether the step involves data exfiltration",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				exfilKeywords := []string{
					"exfiltrate", "extract", "download", "dump", "export",
					"copy", "transfer", "send", "upload",
				}

				text := strings.ToLower(step.Name + " " + step.Description)
				for _, keyword := range exfilKeywords {
					if strings.Contains(text, keyword) {
						return RiskLevelHigh, fmt.Sprintf("step may involve data exfiltration: %s", keyword)
					}
				}

				return RiskLevelLow, ""
			},
		},
		{
			Name:        "privilege_escalation",
			Description: "Evaluates whether the step attempts privilege escalation",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				privescKeywords := []string{
					"sudo", "root", "admin", "privilege", "escalate",
					"elevate", "bypass", "exploit",
				}

				text := strings.ToLower(step.Name + " " + step.Description)
				for _, keyword := range privescKeywords {
					if strings.Contains(text, keyword) {
						return RiskLevelCritical, fmt.Sprintf("step may involve privilege escalation: %s", keyword)
					}
				}

				return RiskLevelLow, ""
			},
		},
		{
			Name:        "network_modification",
			Description: "Evaluates whether the step modifies network configuration",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				networkKeywords := []string{
					"firewall", "iptables", "route", "network", "interface",
					"dns", "proxy", "gateway",
				}

				text := strings.ToLower(step.Name + " " + step.Description)
				for _, keyword := range networkKeywords {
					if strings.Contains(text, keyword) {
						return RiskLevelMedium, fmt.Sprintf("step may modify network configuration: %s", keyword)
					}
				}

				return RiskLevelLow, ""
			},
		},
		{
			Name:        "credential_access",
			Description: "Evaluates whether the step accesses credentials",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				credKeywords := []string{
					"password", "credential", "key", "secret", "token",
					"certificate", "private key", "api key",
				}

				text := strings.ToLower(step.Name + " " + step.Description)
				for _, keyword := range credKeywords {
					if strings.Contains(text, keyword) {
						return RiskLevelHigh, fmt.Sprintf("step may access credentials: %s", keyword)
					}
				}

				return RiskLevelLow, ""
			},
		},
		{
			Name:        "agent_delegation",
			Description: "Evaluates risk of delegating to another agent",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				// Agent steps inherit risk from the delegated task
				if step.Type == StepTypeAgent {
					return RiskLevelMedium, "delegation to another agent introduces additional complexity"
				}
				return RiskLevelLow, ""
			},
		},
		{
			Name:        "parallel_execution",
			Description: "Evaluates risk of parallel step execution",
			Evaluate: func(step *ExecutionStep, ctx RiskContext) (RiskLevel, string) {
				// Parallel execution can have race conditions
				if step.Type == StepTypeParallel && len(step.ParallelSteps) > 1 {
					return RiskLevelMedium, fmt.Sprintf("parallel execution of %d steps may have race conditions", len(step.ParallelSteps))
				}
				return RiskLevelLow, ""
			},
		},
	}
}
