package observability

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/types"
)

func TestBuildDecisionSpan(t *testing.T) {
	tests := []struct {
		name          string
		decision      *schema.Decision
		prompt        string
		traceID       string
		parentSpanID  string
		expectedName  string
		expectedLevel string
		checkMetadata bool
	}{
		{
			name:          "nil decision returns nil",
			decision:      nil,
			prompt:        "test prompt",
			traceID:       "trace-123",
			parentSpanID:  "",
			expectedName:  "",
			expectedLevel: "",
		},
		{
			name: "decision with high confidence",
			decision: &schema.Decision{
				ID:                types.NewID(),
				MissionID:         types.NewID(),
				Iteration:         5,
				Timestamp:         time.Now(),
				Action:            schema.DecisionActionExecuteAgent,
				TargetNodeID:      "node-abc",
				Reasoning:         "Node is ready and has no dependencies blocking",
				Confidence:        0.95,
				PromptTokens:      1500,
				CompletionTokens:  200,
				LatencyMs:         250,
				GraphStateSummary: "3 ready, 2 running, 5 completed",
			},
			prompt:        "What should we do next?",
			traceID:       "trace-123",
			parentSpanID:  "parent-456",
			expectedName:  "orchestrator.decision[5]: execute_agent",
			expectedLevel: "DEFAULT",
			checkMetadata: true,
		},
		{
			name: "decision with low confidence triggers warning",
			decision: &schema.Decision{
				ID:               types.NewID(),
				MissionID:        types.NewID(),
				Iteration:        3,
				Timestamp:        time.Now(),
				Action:           schema.DecisionActionSkipAgent,
				TargetNodeID:     "node-xyz",
				Reasoning:        "Uncertain about node readiness",
				Confidence:       0.3,
				PromptTokens:     1200,
				CompletionTokens: 150,
				LatencyMs:        180,
			},
			prompt:        "Should we skip this node?",
			traceID:       "trace-456",
			parentSpanID:  "",
			expectedName:  "orchestrator.decision[3]: skip_agent",
			expectedLevel: "WARNING",
			checkMetadata: true,
		},
		{
			name: "decision with modifications",
			decision: &schema.Decision{
				ID:               types.NewID(),
				MissionID:        types.NewID(),
				Iteration:        7,
				Timestamp:        time.Now(),
				Action:           schema.DecisionActionModifyParams,
				TargetNodeID:     "node-def",
				Reasoning:        "Adjusting timeout for better results",
				Confidence:       0.85,
				Modifications:    map[string]any{"timeout": 30, "retries": 3},
				PromptTokens:     1800,
				CompletionTokens: 250,
				LatencyMs:        300,
			},
			prompt:        "How should we modify parameters?",
			traceID:       "trace-789",
			parentSpanID:  "parent-abc",
			expectedName:  "orchestrator.decision[7]: modify_params",
			expectedLevel: "DEFAULT",
			checkMetadata: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := BuildDecisionSpan(tt.decision, tt.prompt, tt.traceID, tt.parentSpanID)

			if tt.decision == nil {
				assert.Nil(t, span)
				return
			}

			require.NotNil(t, span)
			assert.Equal(t, tt.decision.ID.String(), span.ID)
			assert.Equal(t, tt.traceID, span.TraceID)
			assert.Equal(t, tt.parentSpanID, span.ParentObservationID)
			assert.Equal(t, tt.expectedName, span.Name)
			assert.Equal(t, tt.expectedLevel, span.Level)
			assert.Equal(t, tt.decision.PromptTokens, span.PromptTokens)
			assert.Equal(t, tt.decision.CompletionTokens, span.CompletionTokens)
			assert.Equal(t, tt.decision.PromptTokens+tt.decision.CompletionTokens, span.TotalTokens())

			// Check input structure
			input, ok := span.Input.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.decision.Iteration, input["iteration"])
			assert.Equal(t, tt.prompt, input["prompt"])
			assert.Equal(t, tt.decision.Action.String(), input["requested_action"])

			// Check output structure
			output, ok := span.Output.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.decision.Action.String(), output["action"])
			assert.Equal(t, tt.decision.TargetNodeID, output["target_node"])
			assert.Equal(t, tt.decision.Reasoning, output["reasoning"])
			assert.Equal(t, tt.decision.Confidence, output["confidence"])

			// Check modifications if present
			if len(tt.decision.Modifications) > 0 {
				assert.Equal(t, tt.decision.Modifications, output["modifications"])
			}

			if tt.checkMetadata {
				assert.Equal(t, tt.decision.ID.String(), span.Metadata["decision_id"])
				assert.Equal(t, tt.decision.MissionID.String(), span.Metadata["mission_id"])
				assert.Equal(t, tt.decision.Iteration, span.Metadata["iteration"])
				assert.Equal(t, tt.decision.LatencyMs, span.Metadata["latency_ms"])
				assert.Equal(t, tt.decision.ID.String(), span.Metadata["neo4j_node_id"])
			}

			// Check that span can be serialized
			jsonStr, err := span.ToJSON()
			require.NoError(t, err)
			assert.NotEmpty(t, jsonStr)

			// Verify it's valid JSON
			var parsed map[string]any
			err = json.Unmarshal([]byte(jsonStr), &parsed)
			require.NoError(t, err)
		})
	}
}

func TestBuildAgentSpan(t *testing.T) {
	tests := []struct {
		name          string
		exec          *schema.AgentExecution
		traceID       string
		parentSpanID  string
		expectedName  string
		expectedLevel string
	}{
		{
			name:          "nil execution returns nil",
			exec:          nil,
			traceID:       "trace-123",
			parentSpanID:  "",
			expectedName:  "",
			expectedLevel: "",
		},
		{
			name: "successful agent execution",
			exec: &schema.AgentExecution{
				ID:             types.NewID(),
				WorkflowNodeID: "recon-agent",
				MissionID:      types.NewID(),
				Status:         schema.ExecutionStatusCompleted,
				StartedAt:      time.Now().Add(-5 * time.Minute),
				CompletedAt:    timePtr(time.Now()),
				Attempt:        1,
				ConfigUsed:     map[string]any{"timeout": 300, "depth": 3},
				Result:         map[string]any{"findings": 5, "status": "success"},
			},
			traceID:       "trace-456",
			parentSpanID:  "parent-789",
			expectedName:  "agent.execute: recon-agent",
			expectedLevel: "DEFAULT",
		},
		{
			name: "failed agent execution",
			exec: &schema.AgentExecution{
				ID:             types.NewID(),
				WorkflowNodeID: "exploit-agent",
				MissionID:      types.NewID(),
				Status:         schema.ExecutionStatusFailed,
				StartedAt:      time.Now().Add(-2 * time.Minute),
				CompletedAt:    timePtr(time.Now()),
				Attempt:        2,
				ConfigUsed:     map[string]any{"target": "api.example.com"},
				Result:         map[string]any{},
				Error:          "connection timeout after 30s",
			},
			traceID:       "trace-789",
			parentSpanID:  "parent-abc",
			expectedName:  "agent.execute: exploit-agent",
			expectedLevel: "ERROR",
		},
		{
			name: "running agent execution",
			exec: &schema.AgentExecution{
				ID:             types.NewID(),
				WorkflowNodeID: "analysis-agent",
				MissionID:      types.NewID(),
				Status:         schema.ExecutionStatusRunning,
				StartedAt:      time.Now().Add(-30 * time.Second),
				CompletedAt:    nil,
				Attempt:        1,
				ConfigUsed:     map[string]any{"model": "gpt-4"},
				Result:         map[string]any{},
			},
			traceID:       "trace-abc",
			parentSpanID:  "",
			expectedName:  "agent.execute: analysis-agent",
			expectedLevel: "DEFAULT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := BuildAgentSpan(tt.exec, tt.traceID, tt.parentSpanID)

			if tt.exec == nil {
				assert.Nil(t, span)
				return
			}

			require.NotNil(t, span)
			assert.Equal(t, tt.exec.ID.String(), span.ID)
			assert.Equal(t, tt.traceID, span.TraceID)
			assert.Equal(t, tt.parentSpanID, span.ParentObservationID)
			assert.Equal(t, tt.expectedName, span.Name)
			assert.Equal(t, tt.expectedLevel, span.Level)
			assert.Equal(t, tt.exec.StartedAt, span.StartTime)
			assert.Equal(t, tt.exec.CompletedAt, span.EndTime)

			// Check input structure
			input := span.Input
			assert.Equal(t, tt.exec.WorkflowNodeID, input["workflow_node_id"])
			assert.Equal(t, tt.exec.Attempt, input["attempt"])
			assert.Equal(t, tt.exec.ConfigUsed, input["config"])

			// Check output structure
			output := span.Output
			assert.Equal(t, tt.exec.Status.String(), output["status"])
			assert.Equal(t, tt.exec.Result, output["result"])

			if tt.exec.Error != "" {
				assert.Equal(t, tt.exec.Error, output["error"])
				assert.Equal(t, tt.exec.Error, span.StatusMessage)
			}

			// Check metadata
			assert.Equal(t, tt.exec.ID.String(), span.Metadata["execution_id"])
			assert.Equal(t, tt.exec.MissionID.String(), span.Metadata["mission_id"])
			assert.Equal(t, tt.exec.WorkflowNodeID, span.Metadata["workflow_node_id"])
			assert.Equal(t, tt.exec.Attempt, span.Metadata["attempt"])
			assert.Equal(t, tt.exec.ID.String(), span.Metadata["neo4j_node_id"])

			// Check duration if completed
			if tt.exec.CompletedAt != nil {
				assert.NotNil(t, span.Metadata["duration_ms"])
			}

			// Check that span can be serialized
			jsonStr, err := span.ToJSON()
			require.NoError(t, err)
			assert.NotEmpty(t, jsonStr)
		})
	}
}

func TestBuildToolSpan(t *testing.T) {
	tests := []struct {
		name          string
		tool          *schema.ToolExecution
		traceID       string
		parentSpanID  string
		expectedName  string
		expectedLevel string
	}{
		{
			name:          "nil tool execution returns nil",
			tool:          nil,
			traceID:       "trace-123",
			parentSpanID:  "",
			expectedName:  "",
			expectedLevel: "",
		},
		{
			name: "successful tool execution",
			tool: &schema.ToolExecution{
				ID:               types.NewID(),
				AgentExecutionID: types.NewID(),
				ToolName:         "nmap_scan",
				Input:            map[string]any{"target": "192.168.1.1", "ports": "1-1000"},
				Output:           map[string]any{"open_ports": []int{22, 80, 443}, "scan_time": 45},
				StartedAt:        time.Now().Add(-1 * time.Minute),
				CompletedAt:      timePtr(time.Now()),
				Status:           schema.ExecutionStatusCompleted,
			},
			traceID:       "trace-456",
			parentSpanID:  "agent-789",
			expectedName:  "tool.nmap_scan",
			expectedLevel: "DEFAULT",
		},
		{
			name: "failed tool execution",
			tool: &schema.ToolExecution{
				ID:               types.NewID(),
				AgentExecutionID: types.NewID(),
				ToolName:         "exploit_db_search",
				Input:            map[string]any{"query": "wordpress", "version": "5.0"},
				Output:           map[string]any{},
				StartedAt:        time.Now().Add(-30 * time.Second),
				CompletedAt:      timePtr(time.Now()),
				Status:           schema.ExecutionStatusFailed,
				Error:            "API rate limit exceeded",
			},
			traceID:       "trace-789",
			parentSpanID:  "agent-abc",
			expectedName:  "tool.exploit_db_search",
			expectedLevel: "ERROR",
		},
		{
			name: "running tool execution",
			tool: &schema.ToolExecution{
				ID:               types.NewID(),
				AgentExecutionID: types.NewID(),
				ToolName:         "burp_scan",
				Input:            map[string]any{"url": "https://example.com", "scan_type": "active"},
				Output:           map[string]any{},
				StartedAt:        time.Now().Add(-2 * time.Minute),
				CompletedAt:      nil,
				Status:           schema.ExecutionStatusRunning,
			},
			traceID:       "trace-def",
			parentSpanID:  "agent-def",
			expectedName:  "tool.burp_scan",
			expectedLevel: "DEFAULT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := BuildToolSpan(tt.tool, tt.traceID, tt.parentSpanID)

			if tt.tool == nil {
				assert.Nil(t, span)
				return
			}

			require.NotNil(t, span)
			assert.Equal(t, tt.tool.ID.String(), span.ID)
			assert.Equal(t, tt.traceID, span.TraceID)
			assert.Equal(t, tt.parentSpanID, span.ParentObservationID)
			assert.Equal(t, tt.expectedName, span.Name)
			assert.Equal(t, tt.expectedLevel, span.Level)
			assert.Equal(t, tt.tool.StartedAt, span.StartTime)
			assert.Equal(t, tt.tool.CompletedAt, span.EndTime)

			// Check input structure
			input := span.Input
			assert.Equal(t, tt.tool.ToolName, input["tool_name"])
			assert.Equal(t, tt.tool.Input, input["params"])

			// Check output structure
			output := span.Output
			assert.Equal(t, tt.tool.Status.String(), output["status"])
			assert.Equal(t, tt.tool.Output, output["result"])

			if tt.tool.Error != "" {
				assert.Equal(t, tt.tool.Error, output["error"])
				assert.Equal(t, tt.tool.Error, span.StatusMessage)
			}

			// Check metadata
			assert.Equal(t, tt.tool.ID.String(), span.Metadata["tool_execution_id"])
			assert.Equal(t, tt.tool.AgentExecutionID.String(), span.Metadata["agent_execution_id"])
			assert.Equal(t, tt.tool.ToolName, span.Metadata["tool_name"])
			assert.Equal(t, tt.tool.ID.String(), span.Metadata["neo4j_node_id"])

			// Check duration if completed
			if tt.tool.CompletedAt != nil {
				assert.NotNil(t, span.Metadata["duration_ms"])
			}

			// Check that span can be serialized
			jsonStr, err := span.ToJSON()
			require.NoError(t, err)
			assert.NotEmpty(t, jsonStr)
		})
	}
}

func TestBuildMissionSummarySpan(t *testing.T) {
	tests := []struct {
		name          string
		summary       *SpanMissionSummary
		startTime     time.Time
		endTime       time.Time
		traceID       string
		expectedName  string
		expectedLevel string
	}{
		{
			name:          "nil summary returns nil",
			summary:       nil,
			startTime:     time.Now(),
			endTime:       time.Now(),
			traceID:       "trace-123",
			expectedName:  "",
			expectedLevel: "",
		},
		{
			name: "successful mission summary",
			summary: &SpanMissionSummary{
				MissionID:      types.NewID(),
				Status:         "completed",
				TotalDecisions: 15,
				TotalTokens:    25000,
				Duration:       10 * time.Minute,
				CompletedNodes: 8,
				FailedNodes:    0,
				StopReason:     "all workflow nodes completed",
			},
			startTime:     time.Now().Add(-10 * time.Minute),
			endTime:       time.Now(),
			traceID:       "trace-456",
			expectedName:  "mission.summary",
			expectedLevel: "DEFAULT",
		},
		{
			name: "failed mission summary",
			summary: &SpanMissionSummary{
				MissionID:      types.NewID(),
				Status:         "failed",
				TotalDecisions: 8,
				TotalTokens:    12000,
				Duration:       3 * time.Minute,
				CompletedNodes: 3,
				FailedNodes:    2,
				Error:          "max retries exceeded for critical node",
				StopReason:     "fatal error",
			},
			startTime:     time.Now().Add(-3 * time.Minute),
			endTime:       time.Now(),
			traceID:       "trace-789",
			expectedName:  "mission.summary",
			expectedLevel: "ERROR",
		},
		{
			name: "budget exceeded mission summary",
			summary: &SpanMissionSummary{
				MissionID:      types.NewID(),
				Status:         "budget_exceeded",
				TotalDecisions: 20,
				TotalTokens:    50000,
				Duration:       15 * time.Minute,
				CompletedNodes: 10,
				FailedNodes:    1,
				StopReason:     "token budget exceeded",
			},
			startTime:     time.Now().Add(-15 * time.Minute),
			endTime:       time.Now(),
			traceID:       "trace-abc",
			expectedName:  "mission.summary",
			expectedLevel: "DEFAULT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := BuildMissionSummarySpan(tt.summary, tt.startTime, tt.endTime, tt.traceID)

			if tt.summary == nil {
				assert.Nil(t, span)
				return
			}

			require.NotNil(t, span)
			assert.Equal(t, tt.summary.MissionID.String()+"-summary", span.ID)
			assert.Equal(t, tt.traceID, span.TraceID)
			assert.Empty(t, span.ParentObservationID)
			assert.Equal(t, tt.expectedName, span.Name)
			assert.Equal(t, tt.expectedLevel, span.Level)
			assert.Equal(t, tt.startTime, span.StartTime)
			assert.Equal(t, &tt.endTime, span.EndTime)

			// Check input structure
			input := span.Input
			assert.Equal(t, tt.summary.MissionID.String(), input["mission_id"])

			// Check output structure
			output := span.Output
			assert.Equal(t, tt.summary.Status, output["status"])
			assert.Equal(t, tt.summary.TotalDecisions, output["total_decisions"])
			assert.Equal(t, tt.summary.TotalTokens, output["total_tokens"])
			assert.Equal(t, tt.summary.Duration.Milliseconds(), output["duration_ms"])
			assert.Equal(t, tt.summary.CompletedNodes, output["completed_nodes"])
			assert.Equal(t, tt.summary.FailedNodes, output["failed_nodes"])
			assert.Equal(t, tt.summary.StopReason, output["stop_reason"])

			if tt.summary.Error != "" {
				assert.Equal(t, tt.summary.Error, output["error"])
				assert.Equal(t, tt.summary.Error, span.StatusMessage)
			}

			// Check metadata
			assert.Equal(t, tt.summary.MissionID.String(), span.Metadata["mission_id"])
			assert.Equal(t, tt.summary.TotalDecisions, span.Metadata["total_decisions"])
			assert.Equal(t, tt.summary.TotalTokens, span.Metadata["total_tokens"])
			assert.Equal(t, tt.summary.Duration.Milliseconds(), span.Metadata["duration_ms"])
			assert.Equal(t, tt.summary.CompletedNodes, span.Metadata["completed_nodes"])
			assert.Equal(t, tt.summary.FailedNodes, span.Metadata["failed_nodes"])

			// Check that span can be serialized
			jsonStr, err := span.ToJSON()
			require.NoError(t, err)
			assert.NotEmpty(t, jsonStr)
		})
	}
}

func TestLangfuseSpanHelpers(t *testing.T) {
	t.Run("span duration calculation", func(t *testing.T) {
		startTime := time.Now()
		endTime := startTime.Add(5 * time.Second)

		span := &LangfuseSpan{
			StartTime: startTime,
			EndTime:   &endTime,
		}

		assert.Equal(t, 5*time.Second, span.Duration())
		assert.True(t, span.IsComplete())
	})

	t.Run("span without end time", func(t *testing.T) {
		span := &LangfuseSpan{
			StartTime: time.Now(),
			EndTime:   nil,
		}

		assert.Equal(t, time.Duration(0), span.Duration())
		assert.False(t, span.IsComplete())
	})

	t.Run("generation duration calculation", func(t *testing.T) {
		startTime := time.Now()
		endTime := startTime.Add(2 * time.Second)

		gen := &LangfuseGeneration{
			StartTime: startTime,
			EndTime:   &endTime,
		}

		assert.Equal(t, 2*time.Second, gen.Duration())
		assert.True(t, gen.IsComplete())
	})

	t.Run("generation total tokens", func(t *testing.T) {
		gen := &LangfuseGeneration{
			PromptTokens:     1500,
			CompletionTokens: 300,
		}

		assert.Equal(t, 1800, gen.TotalTokens())
	})
}

func TestSpanSerialization(t *testing.T) {
	t.Run("span JSON serialization", func(t *testing.T) {
		endTime := time.Now()
		span := &LangfuseSpan{
			ID:                  "span-123",
			TraceID:             "trace-456",
			ParentObservationID: "parent-789",
			Name:                "test.span",
			StartTime:           time.Now().Add(-1 * time.Second),
			EndTime:             &endTime,
			Input:               map[string]any{"key": "value"},
			Output:              map[string]any{"result": "success"},
			Metadata:            map[string]any{"test": true},
			Level:               "DEFAULT",
		}

		jsonStr, err := span.ToJSON()
		require.NoError(t, err)
		assert.NotEmpty(t, jsonStr)

		// Verify it's valid JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "span-123", parsed["id"])
		assert.Equal(t, "trace-456", parsed["traceId"])
	})

	t.Run("generation JSON serialization", func(t *testing.T) {
		endTime := time.Now()
		gen := &LangfuseGeneration{
			ID:               "gen-123",
			TraceID:          "trace-456",
			Name:             "test.generation",
			StartTime:        time.Now().Add(-2 * time.Second),
			EndTime:          &endTime,
			Model:            "gpt-4",
			Input:            "test prompt",
			Output:           "test completion",
			PromptTokens:     100,
			CompletionTokens: 50,
			Level:            "DEFAULT",
		}

		jsonStr, err := gen.ToJSON()
		require.NoError(t, err)
		assert.NotEmpty(t, jsonStr)

		// Verify it's valid JSON
		var parsed map[string]any
		err = json.Unmarshal([]byte(jsonStr), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "gen-123", parsed["id"])
		assert.Equal(t, "gpt-4", parsed["model"])
		assert.Equal(t, float64(100), parsed["promptTokens"])
		assert.Equal(t, float64(50), parsed["completionTokens"])
	})
}

// Helper function to create a time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}
