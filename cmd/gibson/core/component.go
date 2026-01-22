//go:build fts5

package core

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/component"
)

// ComponentList lists components of a given kind
func ComponentList(cc *CommandContext, kind component.ComponentKind, opts ListOptions) (*CommandResult, error) {
	components, err := cc.ComponentStore.List(cc.Ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	return &CommandResult{
		Success: true,
		Data:    components,
	}, nil
}

// ComponentShow shows details of a specific component
func ComponentShow(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	comp, err := cc.ComponentStore.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	return &CommandResult{
		Success: true,
		Data:    comp,
	}, nil
}

// ComponentInstall installs a component from a source
func ComponentInstall(cc *CommandContext, kind component.ComponentKind, source string, opts InstallOptions) (*CommandResult, error) {
	installOpts := component.InstallOptions{
		Branch: opts.Branch,
		Tag:    opts.Tag,
	}

	if cc.Installer == nil {
		return nil, fmt.Errorf("installer not available")
	}

	result, err := cc.Installer.Install(cc.Ctx, source, kind, installOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to install component: %w", err)
	}

	return &CommandResult{
		Success: true,
		Data:    result,
	}, nil
}

// ComponentInstallAll installs all components from a manifest
func ComponentInstallAll(cc *CommandContext, kind component.ComponentKind, repoURL string, opts InstallOptions) (*CommandResult, error) {
	if cc.Installer == nil {
		return nil, fmt.Errorf("installer not available")
	}

	installOpts := component.InstallOptions{
		Branch: opts.Branch,
		Tag:    opts.Tag,
	}

	results, err := cc.Installer.InstallAll(cc.Ctx, repoURL, kind, installOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to install components: %w", err)
	}

	return &CommandResult{
		Success: true,
		Data:    results,
	}, nil
}

// ComponentUninstall uninstalls a component
func ComponentUninstall(cc *CommandContext, kind component.ComponentKind, name string, opts UninstallOptions) (*CommandResult, error) {
	if cc.Installer == nil {
		return nil, fmt.Errorf("installer not available")
	}

	result, err := cc.Installer.Uninstall(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to uninstall component: %w", err)
	}

	return &CommandResult{
		Success: true,
		Data:    result,
	}, nil
}

// ComponentUpdate updates a component
func ComponentUpdate(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	if cc.Installer == nil {
		return nil, fmt.Errorf("installer not available")
	}

	result, err := cc.Installer.Update(cc.Ctx, kind, name, component.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update component: %w", err)
	}

	return &CommandResult{
		Success: true,
		Data:    result,
	}, nil
}

// ComponentBuild builds a component from a local directory
func ComponentBuild(cc *CommandContext, kind component.ComponentKind, dir string) (*CommandResult, error) {
	// BuildLocal doesn't exist in the Installer interface
	// For now, just return an error
	return nil, fmt.Errorf("build local not yet implemented")
}

// ComponentLogs retrieves logs for a component
func ComponentLogs(cc *CommandContext, kind component.ComponentKind, name string, opts LogsOptions) (*CommandResult, error) {
	// For now, return empty logs
	// In a real implementation, this would fetch logs from the daemon or filesystem
	return &CommandResult{
		Success: true,
		Data:    []string{},
	}, nil
}
