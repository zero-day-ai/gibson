package observability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/schema"
	"github.com/zero-day-ai/gibson/internal/llm"
	"github.com/zero-day-ai/gibson/internal/orchestrator"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestLangfuse_FullPromptEndToEnd is an integration test that verifies the complete
// data flow from orchestrator decisions to Langfuse with full prompt capture.
// This test uses a mock HTTP server to capture and validate Langfuse ingestion events.
func TestLangfuse_FullPromptEndToEnd(t *testing.T) {
	// Start mock Langfuse server
	server := newMockLangfuseServer()
	defer server.Close()

	// Create tracer pointing to mock server
	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	// Create test mission
	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Full Prompt Test Mission",
		"Integration test for full prompt tracing",
		"Verify complete prompt data flows to Langfuse",
		"target-test-123",
		"yaml: integration-test",
	)

	ctx := context.Background()

	// Create adapter
	adapter, err := NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	// Wait for trace initialization and reset server state
	time.Sleep(50 * time.Millisecond)
	server.reset()

	// Create a decision with full prompt data
	decision := &orchestrator.Decision{
		Reasoning:    "Execute reconnaissance to gather information about the target",
		Action:       orchestrator.ActionExecuteAgent,
		TargetNodeID: "recon-node",
		Confidence:   0.92,
	}

	// Create ThinkResult with complete prompt capture
	result := &orchestrator.ThinkResult{
		Decision:         decision,
		PromptTokens:     450,
		CompletionTokens: 120,
		TotalTokens:      570,
		Latency:          320 * time.Millisecond,
		Model:            "gpt-4",
		RawResponse:      `{"reasoning":"Execute recon","action":"execute_agent","target_node_id":"recon-node","confidence":0.92}`,
		RetryCount:       0,

		// Full prompt data
		SystemPrompt: "You are Gibson's Mission Orchestrator responsible for coordinating security testing operations.",
		UserPrompt:   "Current mission: Full Prompt Test Mission\nObjective: Verify complete prompt data flows to Langfuse\n\nReady nodes: recon-node\nDecide next action.",

		// Messages array
		Messages: []llm.Message{
			{
				Role:    llm.RoleSystem,
				Content: "You are Gibson's Mission Orchestrator responsible for coordinating security testing operations.",
			},
			{
				Role:    llm.RoleUser,
				Content: "Current mission: Full Prompt Test Mission\nObjective: Verify complete prompt data flows to Langfuse\n\nReady nodes: recon-node\nDecide next action.",
			},
		},

		// Request configuration
		RequestConfig: orchestrator.RequestConfig{
			Temperature: 0.2,
			MaxTokens:   2000,
			TopP:        0.0,
			SlotName:    "primary",
		},
	}

	// Log the decision
	err = adapter.LogDecision(ctx, decision, result, 1, missionID.String())
	require.NoError(t, err)

	// Close adapter
	summary := &MissionTraceSummary{
		Status:          schema.MissionStatusCompleted.String(),
		TotalDecisions:  1,
		TotalExecutions: 0,
		TotalTools:      0,
		TotalTokens:     570,
		Duration:        500 * time.Millisecond,
		Outcome:         "Integration test completed",
	}
	err = adapter.Close(ctx, summary)
	require.NoError(t, err)

	// Wait for async events to be sent
	time.Sleep(150 * time.Millisecond)

	// Verify events were captured
	events := server.getEvents()
	require.Greater(t, len(events), 0, "Should have captured events")

	// Find the generation-create event
	var generationEvent map[string]any
	for _, event := range events {
		if event["type"] == "generation-create" {
			generationEvent = event
			break
		}
	}
	require.NotNil(t, generationEvent, "Should have a generation-create event")

	// Verify event structure
	assert.Equal(t, "generation-create", generationEvent["type"])

	body, ok := generationEvent["body"].(map[string]any)
	require.True(t, ok, "Event body should be a map")

	// Verify full prompt is in input field
	input, ok := body["input"].(string)
	require.True(t, ok, "Input should be a string")
	assert.Contains(t, input, "[SYSTEM]:", "Input should contain system role delimiter")
	assert.Contains(t, input, "You are Gibson's Mission Orchestrator", "Input should contain system prompt content")
	assert.Contains(t, input, "[USER]:", "Input should contain user role delimiter")
	assert.Contains(t, input, "Full Prompt Test Mission", "Input should contain user prompt content")
	assert.Contains(t, input, "---", "Input should contain message delimiter")

	// Verify model and token counts
	assert.Equal(t, "gpt-4", body["model"])

	// Handle promptTokens as either int or float64 (JSON unmarshaling may vary)
	promptTokens := body["promptTokens"]
	if pt, ok := promptTokens.(float64); ok {
		assert.Equal(t, 450.0, pt)
	} else if pt, ok := promptTokens.(int); ok {
		assert.Equal(t, 450, pt)
	}

	completionTokens := body["completionTokens"]
	if ct, ok := completionTokens.(float64); ok {
		assert.Equal(t, 120.0, ct)
	} else if ct, ok := completionTokens.(int); ok {
		assert.Equal(t, 120, ct)
	}

	// Verify metadata contains structured data
	metadata, ok := body["metadata"].(map[string]any)
	require.True(t, ok, "Metadata should be a map")

	// Verify messages array is present in metadata
	messages, ok := metadata["messages"].([]any)
	require.True(t, ok, "Metadata should contain messages array")
	assert.Len(t, messages, 2, "Should have 2 messages (system + user)")

	// Verify message structure
	msg0, ok := messages[0].(map[string]any)
	require.True(t, ok, "First message should be a map")
	assert.Equal(t, "system", msg0["role"])
	assert.Contains(t, msg0["content"], "Mission Orchestrator")

	msg1, ok := messages[1].(map[string]any)
	require.True(t, ok, "Second message should be a map")
	assert.Equal(t, "user", msg1["role"])
	assert.Contains(t, msg1["content"], "Full Prompt Test Mission")

	// Verify request_config is present in metadata
	requestConfig, ok := metadata["request_config"].(map[string]any)
	require.True(t, ok, "Metadata should contain request_config")

	// Verify request config fields
	temp := requestConfig["temperature"]
	if tempFloat, ok := temp.(float64); ok {
		assert.Equal(t, 0.2, tempFloat)
	}

	maxTokens := requestConfig["max_tokens"]
	if mt, ok := maxTokens.(float64); ok {
		assert.Equal(t, 2000.0, mt)
	} else if mt, ok := maxTokens.(int); ok {
		assert.Equal(t, 2000, mt)
	}

	assert.Equal(t, "primary", requestConfig["slot_name"])

	t.Logf("Integration test complete: captured %d events, verified full prompt in generation event", len(events))
}

// TestLangfuse_FullPromptWithToolCalls tests prompt capture with tool call messages.
func TestLangfuse_FullPromptWithToolCalls(t *testing.T) {
	// Start mock Langfuse server
	server := newMockLangfuseServer()
	defer server.Close()

	// Create tracer
	tracer, err := NewMissionTracer(LangfuseConfig{
		Host:      server.URL,
		PublicKey: "test-public-key",
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)
	defer tracer.Close()

	// Create test mission
	missionID := types.NewID()
	mission := schema.NewMission(
		missionID,
		"Tool Call Test Mission",
		"Test prompt capture with tool calls",
		"Verify tool call messages in prompts",
		"target-tool-test",
		"yaml: tool-test",
	)

	ctx := context.Background()
	adapter, err := NewDecisionLogWriterAdapter(ctx, tracer, mission)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	server.reset()

	// Create decision
	decision := &orchestrator.Decision{
		Reasoning:    "Use scanner tool to discover services",
		Action:       orchestrator.ActionExecuteAgent,
		TargetNodeID: "scanner-node",
		Confidence:   0.88,
	}

	// Create ThinkResult with tool call in messages
	result := &orchestrator.ThinkResult{
		Decision:         decision,
		PromptTokens:     350,
		CompletionTokens: 90,
		TotalTokens:      440,
		Model:            "claude-3-5-sonnet-20241022",
		SystemPrompt:     "You are an orchestrator",
		UserPrompt:       "Scan the target",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You are an orchestrator"},
			{Role: llm.RoleUser, Content: "Scan the target"},
			{
				Role:    llm.RoleAssistant,
				Content: "I'll use the port scanner",
				ToolCalls: []llm.ToolCall{
					{ID: "call-1", Name: "port_scanner", Arguments: `{"target":"192.168.1.1","ports":"1-1024"}`},
				},
			},
			{
				Role:       llm.RoleTool,
				Content:    `{"open_ports":[22,80,443]}`,
				ToolCallID: "call-1",
			},
		},
		RequestConfig: orchestrator.RequestConfig{
			Temperature: 0.3,
			MaxTokens:   1500,
			SlotName:    "primary",
		},
	}

	err = adapter.LogDecision(ctx, decision, result, 2, missionID.String())
	require.NoError(t, err)

	err = adapter.Close(ctx, nil)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	// Verify events
	events := server.getEvents()
	require.Greater(t, len(events), 0)

	// Find generation event
	var generationEvent map[string]any
	for _, event := range events {
		if event["type"] == "generation-create" {
			generationEvent = event
			break
		}
	}
	require.NotNil(t, generationEvent)

	body := generationEvent["body"].(map[string]any)
	metadata := body["metadata"].(map[string]any)

	// Verify messages include tool calls
	messages := metadata["messages"].([]any)
	assert.Len(t, messages, 4, "Should have 4 messages including tool call and response")

	// Verify assistant message with tool call
	msg2 := messages[2].(map[string]any)
	assert.Equal(t, "assistant", msg2["role"])

	// Verify tool response message
	msg3 := messages[3].(map[string]any)
	assert.Equal(t, "tool", msg3["role"])
	assert.Equal(t, "call-1", msg3["tool_call_id"])
	assert.Contains(t, msg3["content"], "open_ports")

	t.Logf("Tool call test complete: verified %d messages including tool interactions", len(messages))
}
