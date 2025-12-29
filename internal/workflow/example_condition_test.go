package workflow

import (
	"fmt"
	"time"
)

func ExampleConditionEvaluator() {
	// Create a new condition evaluator
	evaluator := NewConditionEvaluator()

	// Create evaluation context with variables
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables: map[string]any{
			"count":   float64(42),
			"enabled": true,
		},
	}

	// Evaluate simple expression
	result, err := evaluator.Evaluate("count > 40 && enabled", ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Result: %v\n", result)
	// Output: Result: true
}

func ExampleConditionEvaluator_nodeResults() {
	evaluator := NewConditionEvaluator()

	// Create context with node results
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"recon": {
				NodeID: "recon",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"found_endpoint":  true,
					"endpoint_count":  float64(5),
					"vulnerabilities": float64(0),
				},
				Duration: 2 * time.Second,
			},
		},
		Variables: make(map[string]any),
	}

	// Check if recon was successful and found endpoints
	result, err := evaluator.Evaluate(
		`nodes.recon.status == "completed" && nodes.recon.output.found_endpoint && nodes.recon.output.endpoint_count > 0`,
		ctx,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Recon successful: %v\n", result)
	// Output: Recon successful: true
}

func ExampleConditionEvaluator_functions() {
	evaluator := NewConditionEvaluator()

	// Create context with node results
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"scan": {
				NodeID: "scan",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"endpoints": []any{
						"http://example.com/api/v1",
						"http://example.com/api/v2",
						"http://example.com/admin",
					},
				},
			},
		},
		Variables: make(map[string]any),
	}

	// Check if scan found endpoints
	result, err := evaluator.Evaluate(
		`nodes.scan.status == "completed" && !empty(nodes.scan.output.endpoints) && len(nodes.scan.output.endpoints) >= 3`,
		ctx,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found multiple endpoints: %v\n", result)
	// Output: Found multiple endpoints: true
}

func ExampleConditionEvaluator_customFunction() {
	evaluator := NewConditionEvaluator()

	// Register a custom function
	evaluator.RegisterFunction("max", func(args []any) (any, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("max() requires exactly 2 arguments")
		}
		a, ok1 := args[0].(float64)
		b, ok2 := args[1].(float64)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("max() requires numeric arguments")
		}
		if a > b {
			return a, nil
		}
		return b, nil
	})

	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables: map[string]any{
			"threshold": float64(50),
			"value1":    float64(42),
			"value2":    float64(67),
		},
	}

	// Use custom function in expression
	result, err := evaluator.Evaluate("max(value1, value2) > threshold", ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Max exceeds threshold: %v\n", result)
	// Output: Max exceeds threshold: true
}

func ExampleConditionEvaluator_complexWorkflow() {
	evaluator := NewConditionEvaluator()

	// Simulate a complete workflow scenario
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"discovery": {
				NodeID: "discovery",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"targets_found": float64(3),
				},
			},
			"vulnerability_scan": {
				NodeID: "vulnerability_scan",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"critical": float64(2),
					"high":     float64(5),
					"medium":   float64(10),
				},
			},
		},
		Variables: map[string]any{
			"critical_threshold": float64(1),
			"auto_exploit":       true,
		},
	}

	// Decide whether to proceed with exploitation
	shouldExploit, err := evaluator.Evaluate(
		`auto_exploit && nodes.vulnerability_scan.status == "completed" && nodes.vulnerability_scan.output.critical >= critical_threshold`,
		ctx,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Should exploit: %v\n", shouldExploit)

	// Decide whether to do deeper analysis
	shouldAnalyze, err := evaluator.Evaluate(
		`nodes.discovery.output.targets_found > 0 && (nodes.vulnerability_scan.output.critical > 0 || nodes.vulnerability_scan.output.high > 3)`,
		ctx,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Should analyze: %v\n", shouldAnalyze)

	// Output:
	// Should exploit: true
	// Should analyze: true
}
