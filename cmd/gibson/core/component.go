package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/component/build"
)

// ComponentList returns a list of all installed components of the specified kind.
func ComponentList(cc *CommandContext, kind component.ComponentKind, opts ListOptions) (*CommandResult, error) {
	// Validate flags - cannot use both --local and --remote
	if opts.Local && opts.Remote {
		return nil, fmt.Errorf("cannot use both local and remote filters")
	}

	// Get all components of the specified kind from database
	components, err := cc.DAO.List(cc.Ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	// Apply source filters if specified
	var filtered []*component.Component
	for _, comp := range components {
		if opts.Local && comp.Source != "local" {
			continue
		}
		if opts.Remote && comp.Source != "remote" {
			continue
		}
		filtered = append(filtered, comp)
	}

	return &CommandResult{
		Data: filtered,
	}, nil
}

// ComponentShow returns detailed information about a specific component.
func ComponentShow(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	// Get component
	comp, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	return &CommandResult{
		Data: comp,
	}, nil
}

// ComponentInstall installs a component from a git repository.
func ComponentInstall(cc *CommandContext, kind component.ComponentKind, source string, opts InstallOptions) (*CommandResult, error) {
	// Prepare install options
	installOpts := component.InstallOptions{
		Branch:       opts.Branch,
		Tag:          opts.Tag,
		Force:        opts.Force,
		SkipBuild:    opts.SkipBuild,
		SkipRegister: opts.SkipRegister,
	}

	// Install the component
	start := time.Now()
	result, err := cc.Installer.Install(cc.Ctx, source, kind, installOpts)
	if err != nil {
		return nil, err
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentInstallAll installs all components from a mono-repo.
func ComponentInstallAll(cc *CommandContext, kind component.ComponentKind, source string, opts InstallOptions) (*CommandResult, error) {
	// Prepare install options
	installOpts := component.InstallOptions{
		Branch:       opts.Branch,
		Tag:          opts.Tag,
		Force:        opts.Force,
		SkipBuild:    opts.SkipBuild,
		SkipRegister: opts.SkipRegister,
	}

	// Install all components
	start := time.Now()
	result, err := cc.Installer.InstallAll(cc.Ctx, source, kind, installOpts)
	if err != nil {
		return nil, err
	}

	// Fail if no components were found
	if result.ComponentsFound == 0 {
		return nil, fmt.Errorf("no components found in repository %s", source)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentUninstall removes an installed component.
func ComponentUninstall(cc *CommandContext, kind component.ComponentKind, name string, opts UninstallOptions) (*CommandResult, error) {
	// Check if component exists
	existing, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	// Uninstall the component
	start := time.Now()
	result, err := cc.Installer.Uninstall(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("uninstall failed: %w", err)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentUpdate updates a component to the latest version.
func ComponentUpdate(cc *CommandContext, kind component.ComponentKind, name string, opts UpdateOptions) (*CommandResult, error) {
	// Check if component exists
	existing, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	// Prepare update options
	updateOpts := component.UpdateOptions{
		Restart:   opts.Restart,
		SkipBuild: opts.SkipBuild,
	}

	// Update the component
	start := time.Now()
	result, err := cc.Installer.Update(cc.Ctx, kind, name, updateOpts)
	if err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentBuild builds a component locally.
func ComponentBuild(cc *CommandContext, kind component.ComponentKind, name string) (*CommandResult, error) {
	// Get component
	comp, err := cc.DAO.GetByName(cc.Ctx, kind, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get component: %w", err)
	}
	if comp == nil {
		return nil, fmt.Errorf("component '%s' not found", name)
	}

	if comp.Manifest == nil || comp.Manifest.Build == nil {
		return nil, fmt.Errorf("component '%s' has no build configuration", name)
	}

	// Create builder
	builder := build.NewDefaultBuildExecutor()

	// Prepare build configuration
	buildCfg := comp.Manifest.Build
	workDir := comp.RepoPath
	if workDir == "" {
		return nil, fmt.Errorf("component '%s' has no repository path configured", name)
	}
	buildConfig := build.BuildConfig{
		WorkDir: workDir,
		Command: "make",
		Args:    []string{"build"},
		Env:     buildCfg.GetEnv(),
	}

	// Override with manifest build command if specified
	if buildCfg.Command != "" {
		// Split command into executable and args
		parts := strings.Fields(buildCfg.Command)
		if len(parts) > 0 {
			buildConfig.Command = parts[0]
			buildConfig.Args = parts[1:]
		}
	}

	// Set working directory if specified
	if buildCfg.WorkDir != "" {
		buildConfig.WorkDir = workDir + "/" + buildCfg.WorkDir
	}

	// Build the component
	start := time.Now()
	result, err := builder.Build(cc.Ctx, buildConfig, comp.Name, comp.Version, "dev")
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}

	return &CommandResult{
		Data:     result,
		Duration: time.Since(start),
	}, nil
}

// ComponentStatus returns the status of a component from the registry.
func ComponentStatus(cc *CommandContext, kind component.ComponentKind, name string, opts StatusOptions) (*CommandResult, error) {
	if cc.RegManager == nil {
		return nil, fmt.Errorf("registry not available (run 'gibson init' first)")
	}

	reg := cc.RegManager.Registry()
	if reg == nil {
		return nil, fmt.Errorf("registry not started")
	}

	// If name is provided, get specific component status
	if name != "" {
		// Discover instances of this specific component
		instances, err := reg.Discover(cc.Ctx, kind.String(), name)
		if err != nil {
			return nil, fmt.Errorf("failed to discover component '%s': %w", name, err)
		}

		if len(instances) == 0 {
			return nil, fmt.Errorf("component '%s' not found in registry", name)
		}

		return &CommandResult{
			Data: map[string]interface{}{
				"name":      name,
				"instances": instances,
				"json":      opts.JSON,
			},
		}, nil
	}

	// Otherwise, show all components of this kind
	instances, err := reg.DiscoverAll(cc.Ctx, kind.String())
	if err != nil {
		return nil, fmt.Errorf("failed to discover components: %w", err)
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"instances": instances,
			"json":      opts.JSON,
		},
	}, nil
}

// ComponentLogs retrieves logs for a component.
func ComponentLogs(cc *CommandContext, kind component.ComponentKind, name string, opts LogsOptions) (*CommandResult, error) {
	// Build log path
	logPath := filepath.Join(cc.HomeDir, "logs", string(kind), fmt.Sprintf("%s.log", name))

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no logs found for component '%s' (expected at %s)", name, logPath)
	}

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read logs based on options
	var lines []string
	if opts.Follow {
		// For follow mode, return the file handle and path for streaming
		return &CommandResult{
			Data: map[string]interface{}{
				"follow":   true,
				"log_path": logPath,
			},
		}, nil
	}

	// Read the last N lines
	lines, err = tailFile(file, opts.Lines)
	if err != nil {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	return &CommandResult{
		Data: map[string]interface{}{
			"lines":    lines,
			"log_path": logPath,
		},
	}, nil
}

// Helper functions

// tailFile reads the last N lines from a file.
func tailFile(file *os.File, lines int) ([]string, error) {
	scanner := bufio.NewScanner(file)
	var allLines []string

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Get last N lines
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	return allLines[start:], nil
}
