package prompt

import (
	"testing"
)

// TestPromptTransformer_Interface verifies that our mock transformer implements the interface
func TestPromptTransformer_Interface(t *testing.T) {
	var _ PromptTransformer = (*mockTransformer)(nil)
}

// TestPromptTransformer_Name verifies transformer name is returned correctly
func TestPromptTransformer_Name(t *testing.T) {
	transformer := &mockTransformer{
		name: "TestTransformer",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			return prompts, nil
		},
	}

	if transformer.Name() != "TestTransformer" {
		t.Errorf("Expected name 'TestTransformer', got '%s'", transformer.Name())
	}
}

// TestPromptTransformer_Transform verifies basic transformation
func TestPromptTransformer_Transform(t *testing.T) {
	callCount := 0
	transformer := &mockTransformer{
		name: "CountingTransformer",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			callCount++
			return prompts, nil
		},
	}

	ctx := &RelayContext{
		SourceAgent: "parent",
		TargetAgent: "child",
		Task:        "test",
	}

	prompts := []Prompt{
		{ID: "test1", Position: PositionSystem, Content: "content"},
	}

	_, err := transformer.Transform(ctx, prompts)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected transform to be called once, called %d times", callCount)
	}
}

// TestPromptTransformer_ChainedTransformations tests multiple transformers in sequence
func TestPromptTransformer_ChainedTransformations(t *testing.T) {
	// First transformer adds metadata
	transformer1 := &mockTransformer{
		name: "MetadataAdder",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			result := make([]Prompt, len(prompts))
			for i, p := range prompts {
				if p.Metadata == nil {
					p.Metadata = make(map[string]any)
				}
				p.Metadata["step1"] = "done"
				result[i] = p
			}
			return result, nil
		},
	}

	// Second transformer adds more metadata
	transformer2 := &mockTransformer{
		name: "MoreMetadataAdder",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			result := make([]Prompt, len(prompts))
			for i, p := range prompts {
				if p.Metadata == nil {
					p.Metadata = make(map[string]any)
				}
				p.Metadata["step2"] = "done"
				result[i] = p
			}
			return result, nil
		},
	}

	ctx := &RelayContext{
		SourceAgent: "parent",
		TargetAgent: "child",
		Task:        "test",
	}

	prompts := []Prompt{
		{ID: "test1", Position: PositionSystem, Content: "content"},
	}

	// Apply first transformer
	prompts, err := transformer1.Transform(ctx, prompts)
	if err != nil {
		t.Fatalf("First transform failed: %v", err)
	}

	// Apply second transformer
	prompts, err = transformer2.Transform(ctx, prompts)
	if err != nil {
		t.Fatalf("Second transform failed: %v", err)
	}

	// Verify both metadata entries exist
	if prompts[0].Metadata["step1"] != "done" {
		t.Errorf("Expected step1=done, got %v", prompts[0].Metadata["step1"])
	}

	if prompts[0].Metadata["step2"] != "done" {
		t.Errorf("Expected step2=done, got %v", prompts[0].Metadata["step2"])
	}
}

// TestPromptTransformer_ErrorHandling tests error propagation
func TestPromptTransformer_ErrorHandling(t *testing.T) {
	transformer := &mockTransformer{
		name: "ErrorTransformer",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			return nil, NewInvalidPromptError("test error")
		},
	}

	ctx := &RelayContext{
		SourceAgent: "parent",
		TargetAgent: "child",
		Task:        "test",
	}

	prompts := []Prompt{
		{ID: "test1", Position: PositionSystem, Content: "content"},
	}

	_, err := transformer.Transform(ctx, prompts)
	if err == nil {
		t.Fatal("Expected error from transformer, got nil")
	}
}

// TestPromptTransformer_EmptyPrompts tests behavior with empty prompt list
func TestPromptTransformer_EmptyPrompts(t *testing.T) {
	transformer := &mockTransformer{
		name: "PassthroughTransformer",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			return prompts, nil
		},
	}

	ctx := &RelayContext{
		SourceAgent: "parent",
		TargetAgent: "child",
		Task:        "test",
	}

	result, err := transformer.Transform(ctx, []Prompt{})
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d prompts", len(result))
	}
}

// TestPromptTransformer_ContextAccess tests that transformers can access relay context
func TestPromptTransformer_ContextAccess(t *testing.T) {
	var capturedContext *RelayContext

	transformer := &mockTransformer{
		name: "ContextCapture",
		transform: func(ctx *RelayContext, prompts []Prompt) ([]Prompt, error) {
			capturedContext = ctx
			return prompts, nil
		},
	}

	ctx := &RelayContext{
		SourceAgent: "parent",
		TargetAgent: "child",
		Task:        "test task",
		Memory: map[string]any{
			"key": "value",
		},
		Constraints: []string{"constraint1"},
	}

	prompts := []Prompt{
		{ID: "test1", Position: PositionSystem, Content: "content"},
	}

	_, err := transformer.Transform(ctx, prompts)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}

	if capturedContext == nil {
		t.Fatal("Context was not captured")
	}

	if capturedContext.SourceAgent != "parent" {
		t.Errorf("Expected SourceAgent 'parent', got '%s'", capturedContext.SourceAgent)
	}

	if capturedContext.Task != "test task" {
		t.Errorf("Expected Task 'test task', got '%s'", capturedContext.Task)
	}

	if len(capturedContext.Memory) != 1 {
		t.Errorf("Expected 1 memory entry, got %d", len(capturedContext.Memory))
	}
}
