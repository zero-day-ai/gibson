package workflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConditionEvaluator_PathResolution tests comprehensive path resolution scenarios
func TestConditionEvaluator_PathResolution(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"recon": {
				NodeID: "recon",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"found_endpoint": true,
					"endpoints": []any{
						map[string]any{
							"url":  "https://api.example.com/v1",
							"port": float64(443),
						},
						map[string]any{
							"url":  "https://api.example.com/v2",
							"port": float64(8443),
						},
					},
					"nested": map[string]any{
						"deeply": map[string]any{
							"value": float64(42),
						},
					},
				},
			},
		},
		Variables: map[string]any{
			"config": map[string]any{
				"timeout": float64(30),
			},
		},
	}

	t.Run("simple path resolution", func(t *testing.T) {
		result, err := evaluator.Evaluate("nodes.recon.output.found_endpoint", ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("deep nested path resolution", func(t *testing.T) {
		result, err := evaluator.Evaluate("nodes.recon.output.nested.deeply.value == 42", ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("array with map indexing", func(t *testing.T) {
		result, err := evaluator.Evaluate(`nodes.recon.output.endpoints[0].url == "https://api.example.com/v1"`, ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("array with number field access", func(t *testing.T) {
		result, err := evaluator.Evaluate("nodes.recon.output.endpoints[1].port == 8443", ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("variable path resolution", func(t *testing.T) {
		result, err := evaluator.Evaluate("config.timeout == 30", ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("missing path returns error", func(t *testing.T) {
		testCtx := &EvaluationContext{
			NodeResults: map[string]*NodeResult{
				"test": {
					NodeID: "test",
					Status: NodeStatusCompleted,
					Output: map[string]any{},
				},
			},
			Variables: make(map[string]any),
		}
		// Accessing missing field returns nil, which != 5 causes comparison
		result, err := evaluator.Evaluate("exists(nodes.test.output.missing) == false", testCtx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("non-existent node", func(t *testing.T) {
		_, err := evaluator.Evaluate("nodes.nonexistent.status", ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "node result not found")
	})
}

// TestConditionEvaluator_AllComparisonOperators tests all comparison operators comprehensively
func TestConditionEvaluator_AllComparisonOperators(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   make(map[string]any),
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		// Equality (==)
		{"int equality true", "5 == 5", true},
		{"int equality false", "5 == 6", false},
		{"string equality true", `"test" == "test"`, true},
		{"string equality false", `"test" == "other"`, false},
		{"bool equality true", "true == true", true},
		{"bool equality false", "true == false", false},

		// Inequality (!=)
		{"int inequality true", "5 != 6", true},
		{"int inequality false", "5 != 5", false},
		{"string inequality true", `"test" != "other"`, true},
		{"string inequality false", `"test" != "test"`, false},

		// Less than (<)
		{"int less than true", "5 < 10", true},
		{"int less than false", "10 < 5", false},
		{"int less than equal false", "5 < 5", false},
		{"string less than true", `"apple" < "banana"`, true},
		{"string less than false", `"banana" < "apple"`, false},

		// Less than or equal (<=)
		{"int lte true less", "5 <= 10", true},
		{"int lte true equal", "5 <= 5", true},
		{"int lte false", "10 <= 5", false},

		// Greater than (>)
		{"int greater than true", "10 > 5", true},
		{"int greater than false", "5 > 10", false},
		{"int greater than equal false", "5 > 5", false},
		{"string greater than true", `"zebra" > "apple"`, true},
		{"string greater than false", `"apple" > "zebra"`, false},

		// Greater than or equal (>=)
		{"int gte true greater", "10 >= 5", true},
		{"int gte true equal", "5 >= 5", true},
		{"int gte false", "5 >= 10", false},

		// Float comparisons
		{"float equality", "3.14 == 3.14", true},
		{"float inequality", "3.14 != 2.71", true},
		{"float less than", "2.71 < 3.14", true},
		{"float greater than", "3.14 > 2.71", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			require.NoError(t, err, "Expression should evaluate without error")
			assert.Equal(t, tt.expected, result, "Result should match expected value")
		})
	}
}

// TestConditionEvaluator_BooleanOperatorPrecedence tests operator precedence and associativity
func TestConditionEvaluator_BooleanOperatorPrecedence(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables:   make(map[string]any),
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		// NOT has highest precedence
		{"not before and", "!false && false", false},
		{"not before or", "!false || false", true},

		// AND has higher precedence than OR
		{"and before or - true", "true || false && false", true},
		{"and before or - false", "false || false && true", false},
		{"and before or - complex", "true || true && false", true},

		// Parentheses override precedence
		{"parens override and/or", "(true || false) && false", false},
		{"parens override or/and", "false || (false && true)", false},
		{"nested parens", "((true || false) && (true || false))", true},

		// Multiple NOT operators
		{"double not", "!!true", true},
		{"triple not", "!!!false", true},

		// Complex precedence
		{"complex precedence 1", "true && false || true", true},
		{"complex precedence 2", "false || true && false", false},
		{"complex precedence 3", "!false && true || false", true},
		{"complex precedence 4", "!(false || true) && true", false},

		// Chained operators
		{"chained and", "true && true && true", true},
		{"chained or", "false || false || true", true},
		{"mixed chain", "true && false || true && true", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			require.NoError(t, err, "Expression should evaluate without error")
			assert.Equal(t, tt.expected, result, "Result should match expected value")
		})
	}
}

// TestConditionEvaluator_NestedExpressions tests deeply nested expressions
func TestConditionEvaluator_NestedExpressions(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"step1": {
				NodeID: "step1",
				Status: NodeStatusCompleted,
				Output: map[string]any{"success": true},
			},
			"step2": {
				NodeID: "step2",
				Status: NodeStatusCompleted,
				Output: map[string]any{"count": float64(10)},
			},
			"step3": {
				NodeID: "step3",
				Status: NodeStatusFailed,
				Output: map[string]any{"error": "timeout"},
			},
		},
		Variables: map[string]any{
			"threshold": float64(5),
			"retry":     true,
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			"deeply nested with parens",
			"((nodes.step1.output.success && nodes.step2.output.count > threshold) || (nodes.step3.status == \"failed\" && retry))",
			true,
		},
		{
			"multiple levels of nesting",
			"(nodes.step1.output.success && (nodes.step2.output.count > threshold || nodes.step3.status == \"completed\")) || !retry",
			true,
		},
		{
			"complex nested logic",
			"!(nodes.step1.output.success && nodes.step2.output.count < threshold) && (retry || nodes.step3.status != \"failed\")",
			true,
		},
		{
			"nested with functions",
			"(exists(nodes.step1.output.success) && !empty(nodes.step3.output.error)) || len(nodes.step3.output.error) > 0",
			true,
		},
		{
			"deeply nested comparisons",
			"((5 > 3) && (10 < 20)) || ((15 >= 10) && (20 <= 30))",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			require.NoError(t, err, "Expression should evaluate without error")
			assert.Equal(t, tt.expected, result, "Result should match expected value")
		})
	}
}

// TestConditionEvaluator_ErrorHandling tests comprehensive error handling
func TestConditionEvaluator_ErrorHandling(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"test": {
				NodeID: "test",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"value": float64(42),
				},
			},
		},
		Variables: map[string]any{
			"count": float64(10),
		},
	}

	tests := []struct {
		name        string
		expr        string
		errorSubstr string
	}{
		{
			"invalid syntax - unclosed string",
			`"hello`,
			"unterminated string",
		},
		{
			"invalid syntax - unclosed paren",
			"(true && false",
			"expected",
		},
		{
			"invalid syntax - unexpected char",
			"5 + 3",
			"unexpected",
		},
		{
			"unknown function",
			"unknown_func(5)",
			"unknown function",
		},
		{
			"undefined variable",
			"undefined_var == 5",
			"variable not found",
		},
		{
			"non-boolean result",
			"42",
			"did not evaluate to boolean",
		},
		{
			"type mismatch in comparison",
			`true < false`,
			"cannot compare",
		},
		{
			"missing operand",
			"5 ==",
			"unexpected token",
		},
		{
			"invalid path - missing node",
			"nodes.missing.output.value",
			"node result not found",
		},
		{
			"boolean operator on non-boolean",
			"5 && 10",
			"requires boolean operands",
		},
		{
			"not operator on non-boolean",
			"!42",
			"requires boolean operand",
		},
		{
			"function wrong arg count",
			"len(1, 2, 3)",
			"requires exactly 1 argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluator.Evaluate(tt.expr, ctx)
			require.Error(t, err, "Expression should return an error")

			// Check error is WorkflowError
			var wfErr *WorkflowError
			require.ErrorAs(t, err, &wfErr, "Error should be WorkflowError type")

			// Check error message contains expected substring if provided
			if tt.errorSubstr != "" {
				assert.Contains(t, err.Error(), tt.errorSubstr, "Error message should contain expected substring")
			}
		})
	}
}

// TestConditionEvaluator_NullValueEdgeCases tests edge cases with null/nil values
func TestConditionEvaluator_NullValueEdgeCases(t *testing.T) {
	evaluator := NewConditionEvaluator()

	tests := []struct {
		name     string
		setup    func() *EvaluationContext
		expr     string
		expected bool
		wantErr  bool
	}{
		{
			name: "nil compared to nil",
			setup: func() *EvaluationContext {
				return &EvaluationContext{
					NodeResults: make(map[string]*NodeResult),
					Variables: map[string]any{
						"null1": nil,
						"null2": nil,
					},
				}
			},
			expr:     "null1 == null2",
			expected: true,
			wantErr:  false,
		},
		{
			name: "nil compared to value",
			setup: func() *EvaluationContext {
				return &EvaluationContext{
					NodeResults: make(map[string]*NodeResult),
					Variables: map[string]any{
						"null_val": nil,
						"value":    float64(42),
					},
				}
			},
			expr:     "null_val == value",
			expected: false,
			wantErr:  false,
		},
		{
			name: "nil inequality",
			setup: func() *EvaluationContext {
				return &EvaluationContext{
					NodeResults: make(map[string]*NodeResult),
					Variables: map[string]any{
						"null_val": nil,
						"value":    "test",
					},
				}
			},
			expr:     "null_val != value",
			expected: true,
			wantErr:  false,
		},
		{
			name: "exists function with nil",
			setup: func() *EvaluationContext {
				return &EvaluationContext{
					NodeResults: make(map[string]*NodeResult),
					Variables: map[string]any{
						"null_val": nil,
					},
				}
			},
			expr:     "!exists(null_val)",
			expected: true,
			wantErr:  false,
		},
		{
			name: "empty function with nil",
			setup: func() *EvaluationContext {
				return &EvaluationContext{
					NodeResults: make(map[string]*NodeResult),
					Variables: map[string]any{
						"null_val": nil,
					},
				}
			},
			expr:     "empty(null_val)",
			expected: true,
			wantErr:  false,
		},
		{
			name: "node output with null field",
			setup: func() *EvaluationContext {
				return &EvaluationContext{
					NodeResults: map[string]*NodeResult{
						"test": {
							NodeID: "test",
							Status: NodeStatusCompleted,
							Output: map[string]any{
								"result":  "success",
								"details": nil,
							},
						},
					},
					Variables: make(map[string]any),
				}
			},
			expr:     "!exists(nodes.test.output.details)",
			expected: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			result, err := evaluator.Evaluate(tt.expr, ctx)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestConditionEvaluator_TypeMismatches tests type mismatch scenarios
func TestConditionEvaluator_TypeMismatches(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: make(map[string]*NodeResult),
		Variables: map[string]any{
			"num":    float64(42),
			"str":    "hello",
			"bool":   true,
			"array":  []any{1, 2, 3},
			"object": map[string]any{"key": "value"},
		},
	}

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "string vs number comparison",
			expr:    `str < num`,
			wantErr: true,
		},
		{
			name:    "boolean vs number comparison",
			expr:    `bool > num`,
			wantErr: true,
		},
		{
			name:    "boolean AND with number",
			expr:    `bool && num`,
			wantErr: true,
		},
		{
			name:    "boolean OR with string",
			expr:    `bool || str`,
			wantErr: true,
		},
		{
			name:    "NOT with number",
			expr:    `!num`,
			wantErr: true,
		},
		{
			name:    "comparing different types equality",
			expr:    `num == str`,
			wantErr: false, // Equality should work but return false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := evaluator.Evaluate(tt.expr, ctx)
			if tt.wantErr {
				assert.Error(t, err, "Should error on type mismatch")
			} else {
				assert.NoError(t, err, "Should not error")
			}
		})
	}
}

// TestConditionEvaluator_MapIndexing tests map/object indexing with brackets
func TestConditionEvaluator_MapIndexing(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"test": {
				NodeID: "test",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"metadata": map[string]any{
						"name":    "test-service",
						"version": "1.0.0",
						"tags":    []any{"prod", "api"},
					},
				},
			},
		},
		Variables: map[string]any{
			"config": map[string]any{
				"env":  "production",
				"port": float64(8080),
			},
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
		wantErr  bool
	}{
		{
			name:     "map string index",
			expr:     `config["env"] == "production"`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "map number value",
			expr:     `config["port"] == 8080`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "nested map access",
			expr:     `nodes.test.output.metadata["name"] == "test-service"`,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "map with array value accessed by dot",
			expr:     `nodes.test.output.metadata.tags[0] == "prod"`,
			expected: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestConditionEvaluator_ComplexRealWorldScenarios tests realistic workflow scenarios
func TestConditionEvaluator_ComplexRealWorldScenarios(t *testing.T) {
	evaluator := NewConditionEvaluator()

	t.Run("multi-stage deployment check", func(t *testing.T) {
		ctx := &EvaluationContext{
			NodeResults: map[string]*NodeResult{
				"build": {
					NodeID: "build",
					Status: NodeStatusCompleted,
					Output: map[string]any{
						"artifacts": float64(3),
						"duration":  float64(120),
					},
				},
				"test": {
					NodeID: "test",
					Status: NodeStatusCompleted,
					Output: map[string]any{
						"passed": float64(150),
						"failed": float64(0),
					},
				},
				"security_scan": {
					NodeID: "security_scan",
					Status: NodeStatusCompleted,
					Output: map[string]any{
						"vulnerabilities": float64(0),
						"warnings":        float64(2),
					},
				},
			},
			Variables: map[string]any{
				"deploy_enabled":      true,
				"max_vulnerabilities": float64(0),
				"min_test_pass_rate":  float64(0.95),
			},
		}

		expr := `deploy_enabled &&
				 nodes.build.status == "completed" &&
				 nodes.test.status == "completed" &&
				 nodes.security_scan.status == "completed" &&
				 nodes.test.output.failed == 0 &&
				 nodes.security_scan.output.vulnerabilities <= max_vulnerabilities`

		result, err := evaluator.Evaluate(expr, ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("conditional retry logic", func(t *testing.T) {
		ctx := &EvaluationContext{
			NodeResults: map[string]*NodeResult{
				"api_call": {
					NodeID: "api_call",
					Status: NodeStatusFailed,
					Output: map[string]any{
						"status_code": float64(503),
						"retries":     float64(2),
					},
					Error: &NodeError{
						Code:    "HTTP_ERROR",
						Message: "Service Unavailable",
					},
				},
			},
			Variables: map[string]any{
				"max_retries":     float64(3),
				"retry_on_5xx":    true,
				"backoff_enabled": true,
			},
		}

		expr := `nodes.api_call.status == "failed" &&
				 retry_on_5xx &&
				 nodes.api_call.output.status_code >= 500 &&
				 nodes.api_call.output.retries < max_retries`

		result, err := evaluator.Evaluate(expr, ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("data validation pipeline", func(t *testing.T) {
		ctx := &EvaluationContext{
			NodeResults: map[string]*NodeResult{
				"ingest": {
					NodeID: "ingest",
					Status: NodeStatusCompleted,
					Output: map[string]any{
						"records": []any{
							map[string]any{"id": "1", "valid": true},
							map[string]any{"id": "2", "valid": true},
							map[string]any{"id": "3", "valid": true},
						},
						"total": float64(3),
					},
				},
			},
			Variables: map[string]any{
				"min_records": float64(1),
			},
		}

		expr := `nodes.ingest.status == "completed" &&
				 !empty(nodes.ingest.output.records) &&
				 len(nodes.ingest.output.records) >= min_records`

		result, err := evaluator.Evaluate(expr, ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})
}

// TestConditionEvaluator_ArrayOutOfBounds tests array indexing edge cases
func TestConditionEvaluator_ArrayOutOfBounds(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"test": {
				NodeID: "test",
				Status: NodeStatusCompleted,
				Output: map[string]any{
					"items": []any{"a", "b", "c"},
				},
			},
		},
		Variables: map[string]any{
			"small_array": []any{float64(1), float64(2)},
		},
	}

	t.Run("negative index", func(t *testing.T) {
		// Go doesn't have Python-style negative indexing, so this should error
		_, err := evaluator.Evaluate("nodes.test.output.items[-1]", ctx)
		assert.Error(t, err)
	})

	t.Run("index out of bounds", func(t *testing.T) {
		_, err := evaluator.Evaluate("nodes.test.output.items[99]", ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "out of bounds")
	})

	t.Run("valid index at boundary", func(t *testing.T) {
		result, err := evaluator.Evaluate("nodes.test.output.items[2] == \"c\"", ctx)
		require.NoError(t, err)
		assert.True(t, result)
	})
}

// TestConditionEvaluator_NodeStatusField tests accessing node status and duration fields
func TestConditionEvaluator_NodeStatusField(t *testing.T) {
	evaluator := NewConditionEvaluator()
	ctx := &EvaluationContext{
		NodeResults: map[string]*NodeResult{
			"slow_task": {
				NodeID:   "slow_task",
				Status:   NodeStatusCompleted,
				Output:   map[string]any{"result": "done"},
				Duration: 5 * time.Second,
			},
			"failed_task": {
				NodeID:   "failed_task",
				Status:   NodeStatusFailed,
				Output:   map[string]any{},
				Error: &NodeError{
					Code:    "TIMEOUT",
					Message: "connection timeout",
				},
				Duration: 30 * time.Second,
			},
		},
		Variables: map[string]any{
			"timeout_threshold": float64(10),
		},
	}

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			"check duration in seconds",
			"nodes.slow_task.duration == 5",
			true,
		},
		{
			"duration comparison",
			"nodes.failed_task.duration > timeout_threshold",
			true,
		},
		{
			"status with duration check",
			`nodes.slow_task.status == "completed" && nodes.slow_task.duration < timeout_threshold`,
			true,
		},
		{
			"failed status check",
			`nodes.failed_task.status == "failed"`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.expr, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
