package mission

import (
	"context"
	"fmt"

	"github.com/zero-day-ai/gibson/internal/component"
)

// componentStoreAdapter wraps component.ComponentStore to implement mission.ComponentStore
type componentStoreAdapter struct {
	store component.ComponentStore
}

// NewComponentStoreAdapter creates an adapter that wraps a component.ComponentStore
func NewComponentStoreAdapter(store component.ComponentStore) ComponentStore {
	return &componentStoreAdapter{store: store}
}

// GetByName retrieves a component by kind and name
func (a *componentStoreAdapter) GetByName(ctx context.Context, kind ComponentKind, name string) (*Component, error) {
	// Convert mission.ComponentKind to component.ComponentKind
	var compKind component.ComponentKind
	switch kind {
	case ComponentKindAgent:
		compKind = component.ComponentKindAgent
	case ComponentKindTool:
		compKind = component.ComponentKindTool
	case ComponentKindPlugin:
		compKind = component.ComponentKindPlugin
	default:
		return nil, fmt.Errorf("invalid component kind: %s", kind)
	}

	// Get component from underlying store
	comp, err := a.store.GetByName(ctx, compKind, name)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return nil, nil
	}

	// Convert component.Component to mission.Component
	return &Component{
		Name:    comp.Name,
		Kind:    kind,
		Version: comp.Version,
	}, nil
}

// componentInstallerAdapter wraps component.Installer to implement mission.ComponentInstaller
type componentInstallerAdapter struct {
	installer component.Installer
}

// NewComponentInstallerAdapter creates an adapter that wraps a component.Installer
func NewComponentInstallerAdapter(installer component.Installer) ComponentInstaller {
	return &componentInstallerAdapter{installer: installer}
}

// Install installs a component from a URL or name
func (a *componentInstallerAdapter) Install(ctx context.Context, url string, kind ComponentKind, opts ComponentInstallOptions) (*ComponentInstallResult, error) {
	// Convert mission.ComponentKind to component.ComponentKind
	var compKind component.ComponentKind
	switch kind {
	case ComponentKindAgent:
		compKind = component.ComponentKindAgent
	case ComponentKindTool:
		compKind = component.ComponentKindTool
	case ComponentKindPlugin:
		compKind = component.ComponentKindPlugin
	default:
		return nil, fmt.Errorf("invalid component kind: %s", kind)
	}

	// Convert mission install options to component install options
	compOpts := component.InstallOptions{
		Force:   opts.Force,
		Timeout: opts.Timeout,
	}

	// Install via underlying installer
	result, err := a.installer.Install(ctx, url, compKind, compOpts)
	if err != nil {
		return nil, err
	}

	// Convert result
	return &ComponentInstallResult{
		Name:       result.Component.Name,
		Version:    result.Component.Version,
		DurationMs: result.Duration.Milliseconds(),
	}, nil
}
