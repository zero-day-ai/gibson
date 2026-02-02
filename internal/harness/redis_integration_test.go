//go:build integration
// +build integration

package harness

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/sdk/queue"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// Redis connection helper - uses localhost:6379 by default or REDIS_URL env var
func getRedisURL() string {
	if url := os.Getenv("REDIS_URL"); url != "" {
		return url
	}
	return "redis://localhost:6379"
}

// connectRedis creates a Redis client and tests connectivity
func connectRedis(t *testing.T) *queue.RedisClient {
	t.Helper()

	redisURL := getRedisURL()
	client, err := queue.NewRedisClient(queue.RedisOptions{
		URL:            redisURL,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   5 * time.Second,
	})

	if err != nil {
		t.Skipf("Redis not available at %s: %v", redisURL, err)
		return nil
	}

	// Test connectivity with a ping
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to list tools - this will fail if Redis isn't responsive
	_, err = client.ListTools(ctx)
	if err != nil && err.Error() != "failed to get available tools: redis: nil" {
		t.Skipf("Redis not responsive: %v", err)
		return nil
	}

	return client
}

// cleanupRedis removes test keys from Redis
func cleanupRedis(t *testing.T, client *queue.RedisClient, keyPrefix string) {
	t.Helper()

	// We can't easily delete keys by pattern in go-redis without using SCAN,
	// so we'll clean up specific known keys instead
	// For tests, we use predictable tool names like "test-tool-*"
	// Clean up the tools:available set and tool metadata
	if keyPrefix != "" {
		// This is best-effort cleanup - don't fail tests if cleanup fails
		_ = client.Close()
	}
}

// TestRedisIntegration_ToolRegistration tests the tool registration workflow
func TestRedisIntegration_ToolRegistration(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-registration-%s", uuid.New().String()[:8])

	// Create tool metadata
	meta := queue.ToolMeta{
		Name:              toolName,
		Version:           "1.0.0",
		Description:       "Test tool for integration testing",
		InputMessageType:  "google.protobuf.Struct",
		OutputMessageType: "google.protobuf.Struct",
		Schema:            `{"type": "object"}`,
		Tags:              []string{"test", "integration"},
		WorkerCount:       0,
	}

	// Register the tool
	err := client.RegisterTool(ctx, meta)
	require.NoError(t, err, "failed to register tool")

	// List all tools and verify our tool is present
	tools, err := client.ListTools(ctx)
	require.NoError(t, err, "failed to list tools")

	found := false
	for _, tool := range tools {
		if tool.Name == toolName {
			found = true
			assert.Equal(t, meta.Version, tool.Version)
			assert.Equal(t, meta.Description, tool.Description)
			assert.Equal(t, meta.InputMessageType, tool.InputMessageType)
			assert.Equal(t, meta.OutputMessageType, tool.OutputMessageType)
			assert.ElementsMatch(t, meta.Tags, tool.Tags)
			break
		}
	}

	assert.True(t, found, "tool not found in registered tools list")

	t.Logf("Successfully registered and retrieved tool: %s", toolName)
}

// TestRedisIntegration_PushPopWorkflow tests the work item push/pop flow
func TestRedisIntegration_PushPopWorkflow(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-pushpop-%s", uuid.New().String()[:8])
	queueName := fmt.Sprintf("tool:%s:queue", toolName)

	// Create a work item
	jobID := uuid.New().String()
	inputData := map[string]any{
		"target": "example.com",
		"port":   443,
	}
	inputProto, err := structpb.NewStruct(inputData)
	require.NoError(t, err)

	inputJSON, err := protojson.Marshal(inputProto)
	require.NoError(t, err)

	workItem := queue.WorkItem{
		JobID:       jobID,
		Index:       0,
		Total:       1,
		Tool:        toolName,
		InputJSON:   string(inputJSON),
		InputType:   "google.protobuf.Struct",
		OutputType:  "google.protobuf.Struct",
		TraceID:     "trace-123",
		SpanID:      "span-456",
		SubmittedAt: time.Now().UnixMilli(),
	}

	// Validate work item
	err = workItem.IsValid()
	require.NoError(t, err, "work item should be valid")

	// Push work item to queue
	err = client.Push(ctx, queueName, workItem)
	require.NoError(t, err, "failed to push work item")

	t.Logf("Pushed work item to queue: %s (job_id: %s)", queueName, jobID)

	// Pop work item from queue (simulating a worker)
	popCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	poppedItem, err := client.Pop(popCtx, queueName)
	require.NoError(t, err, "failed to pop work item")
	require.NotNil(t, poppedItem, "popped item should not be nil")

	// Verify work item fields
	assert.Equal(t, jobID, poppedItem.JobID)
	assert.Equal(t, 0, poppedItem.Index)
	assert.Equal(t, 1, poppedItem.Total)
	assert.Equal(t, toolName, poppedItem.Tool)
	assert.Equal(t, "google.protobuf.Struct", poppedItem.InputType)
	assert.Equal(t, "google.protobuf.Struct", poppedItem.OutputType)
	assert.Equal(t, string(inputJSON), poppedItem.InputJSON)
	assert.Equal(t, "trace-123", poppedItem.TraceID)
	assert.Equal(t, "span-456", poppedItem.SpanID)

	// Verify work item age is reasonable (should be < 1 second)
	age := poppedItem.Age()
	assert.Less(t, age, 2*time.Second, "work item age should be less than 2 seconds")

	t.Logf("Successfully popped work item from queue (age: %v)", age)
}

// TestRedisIntegration_PubSubResultDelivery tests the pub/sub result delivery
func TestRedisIntegration_PubSubResultDelivery(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	jobID := uuid.New().String()
	resultsChannel := fmt.Sprintf("results:%s", jobID)

	// Subscribe to results channel BEFORE publishing
	resultsChan, err := client.Subscribe(ctx, resultsChannel)
	require.NoError(t, err, "failed to subscribe to results channel")

	t.Logf("Subscribed to results channel: %s", resultsChannel)

	// Give the subscription a moment to establish
	time.Sleep(100 * time.Millisecond)

	// Create a result
	outputData := map[string]any{
		"status":  "success",
		"finding": "SQL injection detected",
		"details": map[string]any{
			"endpoint": "/api/users",
			"severity": "high",
		},
	}
	outputProto, err := structpb.NewStruct(outputData)
	require.NoError(t, err)

	outputJSON, err := protojson.Marshal(outputProto)
	require.NoError(t, err)

	startTime := time.Now().UnixMilli()
	result := queue.Result{
		JobID:       jobID,
		Index:       0,
		OutputJSON:  string(outputJSON),
		OutputType:  "google.protobuf.Struct",
		Error:       "",
		WorkerID:    "test-worker-1",
		StartedAt:   startTime,
		CompletedAt: startTime + 100,
	}

	// Validate result
	err = result.IsValid()
	require.NoError(t, err, "result should be valid")

	// Publish result
	err = client.Publish(ctx, resultsChannel, result)
	require.NoError(t, err, "failed to publish result")

	t.Logf("Published result to channel: %s", resultsChannel)

	// Wait for result with timeout
	select {
	case receivedResult, ok := <-resultsChan:
		require.True(t, ok, "results channel should not be closed")

		// Verify result fields
		assert.Equal(t, jobID, receivedResult.JobID)
		assert.Equal(t, 0, receivedResult.Index)
		assert.Equal(t, string(outputJSON), receivedResult.OutputJSON)
		assert.Equal(t, "google.protobuf.Struct", receivedResult.OutputType)
		assert.Empty(t, receivedResult.Error)
		assert.Equal(t, "test-worker-1", receivedResult.WorkerID)
		assert.Equal(t, startTime, receivedResult.StartedAt)
		assert.Equal(t, startTime+100, receivedResult.CompletedAt)
		assert.False(t, receivedResult.HasError())

		duration := receivedResult.Duration()
		assert.Equal(t, 100*time.Millisecond, duration)

		t.Logf("Successfully received result via pub/sub (duration: %v)", duration)

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for result")
	}
}

// TestRedisIntegration_WorkerCount tests worker count management
func TestRedisIntegration_WorkerCount(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-workers-%s", uuid.New().String()[:8])

	// Initial worker count should be 0
	count, err := client.GetWorkerCount(ctx, toolName)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "initial worker count should be 0")

	// Increment worker count
	err = client.IncrementWorkerCount(ctx, toolName)
	require.NoError(t, err)

	count, err = client.GetWorkerCount(ctx, toolName)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "worker count should be 1 after increment")

	// Increment again
	err = client.IncrementWorkerCount(ctx, toolName)
	require.NoError(t, err)

	count, err = client.GetWorkerCount(ctx, toolName)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "worker count should be 2 after second increment")

	// Decrement worker count
	err = client.DecrementWorkerCount(ctx, toolName)
	require.NoError(t, err)

	count, err = client.GetWorkerCount(ctx, toolName)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "worker count should be 1 after decrement")

	t.Logf("Worker count management working correctly")
}

// TestRedisIntegration_Heartbeat tests the heartbeat mechanism
func TestRedisIntegration_Heartbeat(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-heartbeat-%s", uuid.New().String()[:8])

	// Send heartbeat
	err := client.Heartbeat(ctx, toolName)
	require.NoError(t, err, "failed to send heartbeat")

	t.Logf("Heartbeat sent successfully for tool: %s", toolName)

	// Note: We can't easily test TTL expiration without waiting 30+ seconds,
	// so we just verify the heartbeat doesn't error
}

// TestRedisIntegration_ProxyExecution tests the full RedisToolProxy execution flow
func TestRedisIntegration_ProxyExecution(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-proxy-%s", uuid.New().String()[:8])

	// Register tool
	meta := queue.ToolMeta{
		Name:              toolName,
		Version:           "1.0.0",
		Description:       "Test tool for proxy execution",
		InputMessageType:  "google.protobuf.Struct",
		OutputMessageType: "google.protobuf.Struct",
		Schema:            `{"type": "object"}`,
		Tags:              []string{"test"},
		WorkerCount:       1,
	}

	err := client.RegisterTool(ctx, meta)
	require.NoError(t, err)

	// Increment worker count to indicate workers are available
	err = client.IncrementWorkerCount(ctx, toolName)
	require.NoError(t, err)

	// Create proxy
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	proxy := NewRedisToolProxy(client, meta, logger)
	proxy.SetTimeout(10 * time.Second)

	// Verify proxy metadata
	assert.Equal(t, toolName, proxy.Name())
	assert.Equal(t, "1.0.0", proxy.Version())
	assert.Equal(t, "Test tool for proxy execution", proxy.Description())
	assert.ElementsMatch(t, []string{"test"}, proxy.Tags())
	assert.Equal(t, "google.protobuf.Struct", proxy.InputMessageType())
	assert.Equal(t, "google.protobuf.Struct", proxy.OutputMessageType())

	// Simulate a worker that processes jobs
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)

		queueName := fmt.Sprintf("tool:%s:queue", toolName)

		// Pop work item
		workItem, err := client.Pop(workerCtx, queueName)
		if err != nil || workItem == nil {
			return
		}

		t.Logf("Worker received work item: %s", workItem.JobID)

		// Process and return result
		time.Sleep(50 * time.Millisecond) // Simulate work

		outputData := map[string]any{
			"status": "completed",
			"input":  workItem.InputJSON,
		}
		outputProto, _ := structpb.NewStruct(outputData)
		outputJSON, _ := protojson.Marshal(outputProto)

		startTime := time.Now().UnixMilli()
		result := queue.Result{
			JobID:       workItem.JobID,
			Index:       workItem.Index,
			OutputJSON:  string(outputJSON),
			OutputType:  workItem.OutputType,
			WorkerID:    "test-worker-proxy",
			StartedAt:   startTime,
			CompletedAt: startTime + 50,
		}

		resultsChannel := fmt.Sprintf("results:%s", workItem.JobID)
		_ = client.Publish(workerCtx, resultsChannel, result)

		t.Logf("Worker published result for job: %s", workItem.JobID)
	}()

	// Execute tool via proxy
	inputData := map[string]any{
		"target": "test.example.com",
	}
	inputProto, err := structpb.NewStruct(inputData)
	require.NoError(t, err)

	// Call proxy in a goroutine to avoid blocking if worker fails
	resultChan := make(chan struct {
		output proto.Message
		err    error
	}, 1)

	go func() {
		output, err := proxy.ExecuteProto(ctx, inputProto)
		resultChan <- struct {
			output proto.Message
			err    error
		}{output, err}
	}()

	// Wait for result
	select {
	case result := <-resultChan:
		require.NoError(t, result.err, "proxy execution should succeed")
		require.NotNil(t, result.output, "output should not be nil")

		// Verify output is a Struct
		outputStruct, ok := result.output.(*structpb.Struct)
		require.True(t, ok, "output should be a Struct")
		assert.Equal(t, "completed", outputStruct.Fields["status"].GetStringValue())

		t.Logf("Proxy execution completed successfully")

	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for proxy execution to complete")
	}

	// Wait for worker to finish
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Log("Warning: worker goroutine did not finish cleanly")
	}
}

// TestRedisIntegration_NoWorkersError tests the "no workers available" error case
func TestRedisIntegration_NoWorkersError(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-noworkers-%s", uuid.New().String()[:8])

	// Register tool but don't start any workers
	meta := queue.ToolMeta{
		Name:              toolName,
		Version:           "1.0.0",
		Description:       "Test tool with no workers",
		InputMessageType:  "google.protobuf.Struct",
		OutputMessageType: "google.protobuf.Struct",
		Schema:            `{"type": "object"}`,
		Tags:              []string{"test"},
		WorkerCount:       0,
	}

	err := client.RegisterTool(ctx, meta)
	require.NoError(t, err)

	// Create proxy
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	proxy := NewRedisToolProxy(client, meta, logger)

	// Check health - should be unhealthy with no workers
	health := proxy.Health(ctx)
	assert.Equal(t, "unhealthy", string(health.State))
	assert.Contains(t, health.Message, "no workers available")

	t.Logf("Health check correctly reports unhealthy: %s", health.Message)

	// Try to execute - this will timeout since no workers are available
	inputProto, err := structpb.NewStruct(map[string]any{"test": "data"})
	require.NoError(t, err)

	proxy.SetTimeout(2 * time.Second) // Short timeout for this test

	execCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	output, err := proxy.ExecuteProto(execCtx, inputProto)
	assert.Error(t, err, "execution should fail with no workers")
	assert.Nil(t, output)

	// Verify it's a timeout error
	assert.Contains(t, err.Error(), "timeout", "error should indicate timeout")

	t.Logf("Correctly handled no workers scenario with timeout error")
}

// TestRedisIntegration_RedisToolRegistry tests the RedisToolRegistry
func TestRedisIntegration_RedisToolRegistry(t *testing.T) {
	client := connectRedis(t)
	if client == nil {
		return
	}
	defer client.Close()

	ctx := context.Background()
	toolName := fmt.Sprintf("test-tool-registry-%s", uuid.New().String()[:8])

	// Register a tool
	meta := queue.ToolMeta{
		Name:              toolName,
		Version:           "1.0.0",
		Description:       "Test tool for registry",
		InputMessageType:  "google.protobuf.Struct",
		OutputMessageType: "google.protobuf.Struct",
		Schema:            `{"type": "object"}`,
		Tags:              []string{"test", "registry"},
		WorkerCount:       0,
	}

	err := client.RegisterTool(ctx, meta)
	require.NoError(t, err)

	// Create registry
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := NewRedisToolRegistry(client, logger)

	// Refresh registry
	err = registry.Refresh(ctx)
	require.NoError(t, err, "failed to refresh registry")

	// Verify tool is in registry
	toolsList := registry.List()
	assert.Contains(t, toolsList, toolName, "tool should be in registry")

	// Get tool
	tool, found := registry.Get(toolName)
	require.True(t, found, "tool should be found")
	require.NotNil(t, tool)

	assert.Equal(t, toolName, tool.Name())
	assert.Equal(t, "1.0.0", tool.Version())

	// Get metadata
	retrievedMeta, found := registry.GetMetadata(toolName)
	require.True(t, found, "metadata should be found")
	assert.Equal(t, meta.Name, retrievedMeta.Name)
	assert.Equal(t, meta.Version, retrievedMeta.Version)
	assert.ElementsMatch(t, meta.Tags, retrievedMeta.Tags)

	// Test health check with no workers
	isHealthy := registry.IsHealthy(ctx, toolName)
	assert.False(t, isHealthy, "tool should be unhealthy with no workers")

	// Add a worker
	err = client.IncrementWorkerCount(ctx, toolName)
	require.NoError(t, err)

	// Send heartbeat
	err = client.Heartbeat(ctx, toolName)
	require.NoError(t, err)

	// Check health again
	isHealthy = registry.IsHealthy(ctx, toolName)
	assert.True(t, isHealthy, "tool should be healthy with workers")

	// Get health status
	healthStatus, err := registry.GetHealthStatus(ctx, toolName)
	require.NoError(t, err)
	assert.Contains(t, healthStatus, "state=healthy")
	assert.Contains(t, healthStatus, "1 workers available")

	// Test count
	count := registry.Count()
	assert.GreaterOrEqual(t, count, 1, "registry should have at least one tool")

	t.Logf("Registry tests passed, count: %d", count)

	// Close registry
	err = registry.Close()
	require.NoError(t, err)
}
