package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestConditionEvaluator_Literals(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   make(map[string]any),
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"true literal", "true", true, false},
		{"false literal", "false", false, false},
		{"number equality", "42 == 42", true, false},
		{"number inequality", "42 != 43", true, false},
		{"string equality", `"hello" == "hello"`, true, false},
		{"string inequality", `"hello" != "world"`, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_Comparisons(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   make(map[string]any),
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"less than true", "5 < 10", true, false},
		{"less than false", "10 < 5", false, false},
		{"less than equal true", "5 <= 5", true, false},
		{"less than equal false", "10 <= 5", false, false},
		{"greater than true", "10 > 5", true, false},
		{"greater than false", "5 > 10", false, false},
		{"greater than equal true", "10 >= 10", true, false},
		{"greater than equal false", "5 >= 10", false, false},
		{"string less than", `"apple" < "banana"`, true, false},
		{"string greater than", `"zebra" > "apple"`, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_BooleanOperators(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   make(map[string]any),
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"and true", "true && true", true, false},
		{"and false", "true && false", false, false},
		{"or true", "true || false", true, false},
		{"or false", "false || false", false, false},
		{"not true", "!false", true, false},
		{"not false", "!true", false, false},
		{"complex and", "5 > 3 && 10 < 20", true, false},
		{"complex or", "5 < 3 || 10 < 20", true, false},
		{"complex not", "!(5 > 10)", true, false},
		{"precedence", "true || false && false", true, false},
		{"parentheses", "(true || false) && false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_Variables(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables: map[string]any{
			"count":   float64(42),
			"name":    "test",
			"enabled": true,
			"data": map[string]any{
				"value": float64(100),
				"flag":  true,
			},
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"variable number", "count == 42", true, false},
		{"variable string", `name == "test"`, true, false},
		{"variable bool", "enabled == true", true, false},
		{"nested variable", "data.value == 100", true, false},
		{"nested bool", "data.flag == true", true, false},
		{"variable comparison", "count > 40", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_NodeResults(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"recon": {
				NodeID: "recon",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"found_endpoint": true,
					"count":          float64(5),
					"message":        "success",
				},
				Duration: 2 * time.Second,
			},
			"scan": {
				NodeID: "scan",
				Status: NodeStatusFailed,
				Output: map[string]any{
					"vulnerabilities": float64(3),
				},
			},
		},
		Variables: make(map[string]any),
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"node output bool", "nodes.recon.output.found_endpoint == true", true, false},
		{"node output number", "nodes.recon.output.count == 5", true, false},
		{"node output string", `nodes.recon.output.message == "success"`, true, false},
		{"node status", `nodes.recon.status == "completed"`, true, false},
		{"node status failed", `nodes.scan.status == "failed"`, true, false},
		{"node output comparison", "nodes.scan.output.vulnerabilities > 0", true, false},
		{"complex expression", `nodes.recon.status == "completed" && nodes.recon.output.found_endpoint`, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_Functions(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables: map[string]any{
			"text":   "hello",
			"empty":  "",
			"items":  []any{"a", "b", "c"},
			"absent": nil,
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"len string", "len(text) == 5", true, false},
		{"len array", "len(items) == 3", true, false},
		{"len map", "len(data) == 1", true, false},
		{"empty string true", "empty(empty) == true", true, false},
		{"empty string false", "empty(text) == false", true, false},
		{"empty array", "empty(items) == false", true, false},
		{"exists true", "exists(text) == true", true, false},
		{"exists false", "exists(absent) == false", true, false},
		{"complex with function", "len(text) > 3 && !empty(text)", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_CustomFunctions(t *testing.T) {
	evaluator := NewConditionEvaluator()

	// Register a custom function
	evaluator.RegisterFunction("contains", func(args []any) (any, error) {
		if len(args) != 2 {
			return nil, nil
		}
		str, ok1 := args[0].(string)
		substr, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return false, nil
		}
		return strings.Contains(str, substr), nil
	})

	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables: map[string]any{
			"message": "hello world",
		},
	}

	result, err := evaluator.Evaluate(`contains(message, "world")`, ctx)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result {
		t.Errorf("Evaluate() = %v, expected true", result)
	}
}

func TestConditionEvaluator_Errors(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   make(map[string]any),
	}

	tests := []struct {
		name string
		expr string
	}{
		{"unclosed string", `"hello`},
		{"unclosed paren", "(true"},
		{"invalid operator", "5 + 3"},
		{"missing operand", "5 =="},
		{"unknown function", "unknown()"},
		{"undefined variable", "undefined_var == 5"},
		{"non-boolean result", "42"},
		{"invalid comparison", `true < false`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluator.Evaluate(tt.expr, ctx)
			if err == nil {
				t.Errorf("Evaluate() expected error but got none")
			}

			// Check that error is WorkflowError
			if _, ok := err.(*WorkflowError); !ok {
				t.Errorf("Evaluate() error type = %T, expected *WorkflowError", err)
			}
		})
	}
}

func TestConditionEvaluator_ComplexExpressions(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"recon": {
				NodeID: "recon",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"endpoints": []any{
						"http://example.com/api/v1",
						"http://example.com/api/v2",
					},
					"vulnerabilities": float64(0),
				},
			},
			"scan": {
				NodeID: "scan",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"critical": float64(2),
					"high":     float64(5),
					"medium":   float64(10),
				},
			},
		},
		Variables: map[string]any{
			"threshold": float64(3),
			"enabled":   true,
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			"complex node condition",
			`nodes.recon.status == "completed" && len(nodes.recon.output.endpoints) > 0`,
			true,
		},
		{
			"vulnerability check",
			`nodes.scan.output.critical > threshold || nodes.scan.output.high > 10`,
			false,
		},
		{
			"nested conditions",
			`(nodes.recon.status == "completed" && nodes.scan.status == "completed") && (nodes.scan.output.critical > 0 || nodes.scan.output.high > 0)`,
			true,
		},
		{
			"with variables",
			`enabled && nodes.scan.output.critical >= threshold`,
			false,
		},
		{
			"complex boolean logic",
			`!empty(nodes.recon.output.endpoints) && nodes.recon.output.vulnerabilities == 0`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_ArrayIndexing(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"test": {
				NodeID: "test",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"items": []any{"first", "second", "third"},
					"data": []any{
						map[string]any{"value": float64(1)},
						map[string]any{"value": float64(2)},
					},
				},
			},
		},
		Variables: map[string]any{
			"array": []any{float64(10), float64(20), float64(30)},
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"array index variable", "array[0] == 10", true, false},
		{"array index comparison", "array[1] > 15", true, false},
		{"nested array index", "nodes.test.output.items[0] == \"first\"", true, false},
		{"array of maps", "nodes.test.output.data[1].value == 2", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConditionEvaluator_NilHandling(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"test": {
				NodeID: "test",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"present": "value",
					"null":    nil,
				},
			},
		},
		Variables: map[string]any{
			"nullable": nil,
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{"nil equality", "nullable == nullable", true, false},
		{"exists with nil", "exists(nullable) == false", true, false},
		{"exists with value", "exists(nodes.test.output.present) == true", true, false},
		{"empty with nil", "empty(nullable) == true", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Evaluate() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkConditionEvaluator_Simple(b *testing.B) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   map[string]any{"count": float64(42)},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = evaluator.Evaluate("count > 40", ctx)
	}
}

func BenchmarkConditionEvaluator_Complex(b *testing.B) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"recon": {
				NodeID: "recon",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"found": true,
					"count": float64(5),
				},
			},
		},
		Variables: map[string]any{"threshold": float64(3)},
	}

	expr := `nodes.recon.status == "completed" && nodes.recon.output.found && nodes.recon.output.count > threshold`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = evaluator.Evaluate(expr, ctx)
	}
}
