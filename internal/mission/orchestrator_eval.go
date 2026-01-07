package mission

import (
	"context"

	"github.com/zero-day-ai/gibson/internal/eval"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
)

// NOTE: This file implements Task 7 (Orchestrator Integration) for the Gibson Eval Integration spec.
// It depends on Task 5 (EvalHarnessFactory) which currently has compilation errors that need to be
// resolved. Once Task 5's implementation is fixed, this integration will work correctly.
//
// Known issues in eval package:
// - internal/eval/factory.go: Type mismatches between Gibson and SDK harness interfaces
// - internal/eval/harness_adapter.go: Missing type definitions (MemoryStore, FunctionCall, Capability)
//
// The integration design is sound, but the eval factory implementation needs completion.

// WithEvalOptions sets eval options for the orchestrator.
// When eval options are provided, the orchestrator will wrap the harness factory
// with an EvalHarnessFactory that enables trajectory recording, real-time feedback,
// and result aggregation across all agents in the mission.
//
// The eval system provides:
//   - Automatic trajectory recording for all agent executions
//   - Real-time feedback and scoring during mission execution
//   - Aggregated evaluation results accessible via GetEvalResults()
//   - Optional export to Langfuse and OpenTelemetry
//
// Example usage:
//
//	evalOpts := eval.NewEvalOptions()
//	evalOpts.Enabled = true
//	evalOpts.FeedbackEnabled = true
//	evalOpts.GroundTruthPath = "/path/to/ground_truth.json"
//
//	orchestrator := mission.NewMissionOrchestrator(
//	    store,
//	    mission.WithHarnessFactory(baseFactory),
//	    mission.WithEvalOptions(evalOpts),
//	)
func WithEvalOptions(opts *eval.EvalOptions) OrchestratorOption {
	return func(o *DefaultMissionOrchestrator) {
		o.evalOptions = opts
	}
}

// wrapFactoryWithEval wraps a harness factory with evaluation capabilities.
// This creates an EvalHarnessFactory that transparently adds recording and feedback
// to all harnesses created during mission execution.
//
// Parameters:
//   - factory: The base harness factory to wrap (must not be nil)
//   - opts: Evaluation options controlling behavior (must not be nil)
//
// Returns:
//   - harness.HarnessFactoryInterface: Wrapped factory (or original if eval disabled)
//   - *eval.EvalResultCollector: Collector for aggregating eval results (or nil if eval disabled)
//   - error: Non-nil if wrapping fails
//
// If evaluation is disabled (opts.Enabled == false), this function returns the
// original factory unchanged with a nil collector and no error.
func wrapFactoryWithEval(
	factory harness.HarnessFactoryInterface,
	opts *eval.EvalOptions,
) (harness.HarnessFactoryInterface, *eval.EvalResultCollector, error) {
	// If evaluation is disabled, return the original factory
	if opts == nil || !opts.Enabled {
		return factory, nil, nil
	}

	// Validate options before wrapping
	if err := opts.Validate(); err != nil {
		return nil, nil, types.WrapError(
			types.CONFIG_VALIDATION_FAILED,
			"invalid evaluation options",
			err,
		)
	}

	// Create the eval harness factory
	evalFactory, err := eval.NewEvalHarnessFactory(factory, opts)
	if err != nil {
		return nil, nil, types.WrapError(
			types.CONFIG_VALIDATION_FAILED,
			"failed to create eval harness factory",
			err,
		)
	}

	// Return the wrapped factory and its collector
	return evalFactory, evalFactory.Results(), nil
}

// GetEvalResults returns the evaluation result collector if eval is enabled.
// This provides access to trajectories, feedback, and scores collected during
// mission execution.
//
// Returns:
//   - *eval.EvalResultCollector: Collector with aggregated results, or nil if eval disabled
//
// The collector can be used to:
//   - Call Finalize(ctx) to compute the final evaluation summary
//   - Call GetSummary() to get current evaluation state (safe during execution)
//   - Access individual agent trajectories and feedback
//   - Export results to JSONL, Langfuse, or OpenTelemetry
func (o *DefaultMissionOrchestrator) GetEvalResults() *eval.EvalResultCollector {
	return o.evalCollector
}

// FinalizeEvalResults computes and returns the final evaluation summary.
// This should be called after mission execution completes to get the complete
// evaluation metrics.
//
// Parameters:
//   - ctx: Context for the finalization operation
//
// Returns:
//   - *eval.EvalSummary: Complete evaluation summary with all metrics
//   - error: Non-nil if finalization fails or eval is not enabled
//
// The summary includes:
//   - Overall score and per-scorer scores
//   - Total steps executed and tokens used
//   - Alert counts (warnings and critical)
//   - Complete feedback history
//
// If evaluation is not enabled, this returns an error.
func (o *DefaultMissionOrchestrator) FinalizeEvalResults(ctx context.Context) (*eval.EvalSummary, error) {
	if o.evalCollector == nil {
		return nil, types.NewError(
			types.CONFIG_VALIDATION_FAILED,
			"evaluation is not enabled for this orchestrator",
		)
	}

	return o.evalCollector.Finalize(ctx)
}
