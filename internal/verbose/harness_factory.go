package verbose

import (
	"github.com/zero-day-ai/gibson/internal/harness"
)

// WrapHarnessFactory wraps a harness.HarnessFactoryInterface to create harnesses
// that emit verbose events via a VerboseEventBus.
//
// If level == LevelNone, the inner factory is returned unchanged (no wrapping).
// Otherwise, all harnesses created by the returned factory will be wrapped with
// VerboseHarnessWrapper to emit events at the specified verbosity level.
//
// Parameters:
//   - inner: The underlying HarnessFactoryInterface to wrap
//   - bus: The event bus to emit events to
//   - level: The verbosity level for emitted events
//
// Returns:
//   - harness.HarnessFactoryInterface: A factory that creates verbose-wrapped harnesses
func WrapHarnessFactory(inner harness.HarnessFactoryInterface, bus VerboseEventBus, level VerboseLevel) harness.HarnessFactoryInterface {
	// If verbose level is none, return inner unchanged
	if level == LevelNone {
		return inner
	}

	// Otherwise, return a wrapped factory
	return &verboseHarnessFactory{
		inner: inner,
		bus:   bus,
		level: level,
	}
}

// verboseHarnessFactory implements harness.HarnessFactoryInterface and wraps
// all created harnesses with VerboseHarnessWrapper.
type verboseHarnessFactory struct {
	inner harness.HarnessFactoryInterface
	bus   VerboseEventBus
	level VerboseLevel
}

// Create creates a new AgentHarness wrapped with VerboseHarnessWrapper.
func (f *verboseHarnessFactory) Create(agentName string, missionCtx harness.MissionContext, target harness.TargetInfo) (harness.AgentHarness, error) {
	// Create the inner harness
	innerHarness, err := f.inner.Create(agentName, missionCtx, target)
	if err != nil {
		return nil, err
	}

	// Wrap it with VerboseHarnessWrapper
	return NewVerboseHarnessWrapper(innerHarness, f.bus, f.level), nil
}

// CreateChild creates a child harness wrapped with VerboseHarnessWrapper.
func (f *verboseHarnessFactory) CreateChild(parent harness.AgentHarness, agentName string) (harness.AgentHarness, error) {
	// If parent is a VerboseHarnessWrapper, unwrap it to get the inner harness
	// This ensures we call CreateChild on the actual factory, not the wrapper
	var innerParent harness.AgentHarness = parent
	if wrapper, ok := parent.(*VerboseHarnessWrapper); ok {
		innerParent = wrapper.inner
	}

	// Create the child harness
	childHarness, err := f.inner.CreateChild(innerParent, agentName)
	if err != nil {
		return nil, err
	}

	// Wrap it with VerboseHarnessWrapper
	return NewVerboseHarnessWrapper(childHarness, f.bus, f.level), nil
}

// Ensure verboseHarnessFactory implements HarnessFactoryInterface at compile time
var _ harness.HarnessFactoryInterface = (*verboseHarnessFactory)(nil)
