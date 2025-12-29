package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zero-day-ai/gibson/internal/component/build"
	"github.com/zero-day-ai/gibson/internal/component/git"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultInstallTimeout is the default timeout for installation operations
	DefaultInstallTimeout = 5 * time.Minute

	// DefaultBuildTimeout is the default timeout for build operations
	DefaultBuildTimeout = 3 * time.Minute

	// ManifestFileName is the name of the component manifest file
	ManifestFileName = "component.yaml"
)

// InstallOptions contains options for installing a component
type InstallOptions struct {
	// Branch specifies which branch to clone (optional)
	Branch string

	// Tag specifies which tag to clone (optional)
	Tag string

	// Force allows reinstalling even if component exists
	Force bool

	// SkipBuild skips the build step
	SkipBuild bool

	// SkipRegister skips registration in the component registry
	SkipRegister bool

	// Timeout specifies the maximum time for the installation
	Timeout time.Duration
}

// UpdateOptions contains options for updating a component
type UpdateOptions struct {
	// Restart automatically restarts the component after update if it was running
	Restart bool

	// SkipBuild skips the build step
	SkipBuild bool

	// Timeout specifies the maximum time for the update
	Timeout time.Duration
}

// InstallResult contains the result of an installation operation
type InstallResult struct {
	// Component is the installed component
	Component *Component

	// Path is the filesystem path where the component was installed
	Path string

	// Duration is how long the installation took
	Duration time.Duration

	// BuildOutput contains output from the build step
	BuildOutput string

	// Installed indicates whether a new installation occurred
	Installed bool

	// Updated indicates whether an existing component was updated
	Updated bool
}

// UpdateResult contains the result of an update operation
type UpdateResult struct {
	// Component is the updated component
	Component *Component

	// Path is the filesystem path of the component
	Path string

	// Duration is how long the update took
	Duration time.Duration

	// BuildOutput contains output from the build step
	BuildOutput string

	// Updated indicates whether the component was actually updated
	Updated bool

	// Restarted indicates whether the component was restarted
	Restarted bool

	// OldVersion is the version before the update
	OldVersion string

	// NewVersion is the version after the update
	NewVersion string
}

// UninstallResult contains the result of an uninstall operation
type UninstallResult struct {
	// Name is the name of the uninstalled component
	Name string

	// Kind is the kind of the uninstalled component
	Kind ComponentKind

	// Path is the filesystem path that was removed
	Path string

	// Duration is how long the uninstall took
	Duration time.Duration

	// WasStopped indicates whether the component was stopped before removal
	WasStopped bool

	// WasRunning indicates whether the component was running before uninstall
	WasRunning bool
}

// Installer defines the interface for installing, updating, and uninstalling components
type Installer interface {
	// Install installs a component from a git repository URL
	Install(ctx context.Context, repoURL string, opts InstallOptions) (*InstallResult, error)

	// Update updates an installed component to the latest version
	Update(ctx context.Context, kind ComponentKind, name string, opts UpdateOptions) (*UpdateResult, error)

	// UpdateAll updates all installed components of a specific kind
	UpdateAll(ctx context.Context, kind ComponentKind, opts UpdateOptions) ([]UpdateResult, error)

	// Uninstall removes an installed component
	Uninstall(ctx context.Context, kind ComponentKind, name string) (*UninstallResult, error)
}

// DefaultInstaller implements Installer using git, build executor, and component registry
type DefaultInstaller struct {
	git      git.GitOperations
	builder  build.BuildExecutor
	registry ComponentRegistry
	homeDir  string
	tracer   trace.Tracer
}

// NewDefaultInstaller creates a new DefaultInstaller instance
func NewDefaultInstaller(gitOps git.GitOperations, builder build.BuildExecutor, registry ComponentRegistry) *DefaultInstaller {
	return &DefaultInstaller{
		git:      gitOps,
		builder:  builder,
		registry: registry,
		homeDir:  getDefaultHomeDir(),
		tracer:   otel.GetTracerProvider().Tracer("gibson.component"),
	}
}

// getDefaultHomeDir returns the default Gibson home directory
func getDefaultHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".gibson")
}

// Install installs a component from a git repository URL
func (i *DefaultInstaller) Install(ctx context.Context, repoURL string, opts InstallOptions) (*InstallResult, error) {
	// Start tracing span
	ctx, span := i.tracer.Start(ctx, SpanComponentInstall)
	defer span.End()

	start := time.Now()
	result := &InstallResult{}

	// Set default timeout if not specified
	if opts.Timeout == 0 {
		opts.Timeout = DefaultInstallTimeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Step 1: Parse repository URL to extract component information
	repoInfo, err := i.git.ParseRepoURL(repoURL)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "parse_repo_url")...)
		return result, WrapComponentError(ErrCodeInvalidPath, "failed to parse repository URL", err)
	}

	// Validate component kind
	kind, err := ParseComponentKind(repoInfo.Kind)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "parse_component_kind")...)
		return result, NewInvalidKindError(repoInfo.Kind)
	}

	// Add component metadata to span
	span.SetAttributes(
		attribute.String(AttrRepoURL, repoURL),
		attribute.String(AttrComponentKind, kind.String()),
		attribute.String(AttrComponentName, repoInfo.Name),
	)

	// Step 2: Determine installation path
	componentDir := filepath.Join(i.homeDir, kind.String()+"s", repoInfo.Name)
	result.Path = componentDir

	// Check if component already exists
	if _, err := os.Stat(componentDir); err == nil {
		if !opts.Force {
			return result, NewComponentExistsError(repoInfo.Name).
				WithContext("path", componentDir)
		}
		// Force install: remove existing directory
		if err := os.RemoveAll(componentDir); err != nil {
			return result, WrapComponentError(ErrCodePermissionDenied, "failed to remove existing component", err).
				WithComponent(repoInfo.Name)
		}
	}

	// Step 3: Clone repository
	cloneOpts := git.CloneOptions{
		Branch: opts.Branch,
		Tag:    opts.Tag,
	}

	if err := i.git.Clone(repoURL, componentDir, cloneOpts); err != nil {
		// Cleanup on failure
		_ = os.RemoveAll(componentDir)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "clone_repository")...)
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to clone repository", err).
			WithComponent(repoInfo.Name).
			WithContext("url", repoURL)
	}

	// Step 4: Load and validate manifest
	manifestPath := filepath.Join(componentDir, ManifestFileName)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		// Cleanup on failure
		_ = os.RemoveAll(componentDir)
		return result, err // LoadManifest already returns proper ComponentError
	}

	// Verify manifest matches repository info
	if manifest.Kind != kind {
		_ = os.RemoveAll(componentDir)
		return result, NewValidationFailedError(
			fmt.Sprintf("manifest kind %s doesn't match repository kind %s", manifest.Kind, kind),
			nil,
		)
	}

	if manifest.Name != repoInfo.Name {
		_ = os.RemoveAll(componentDir)
		return result, NewValidationFailedError(
			fmt.Sprintf("manifest name %s doesn't match repository name %s", manifest.Name, repoInfo.Name),
			nil,
		)
	}

	// Step 5: Check dependencies
	if err := i.checkDependencies(ctx, manifest); err != nil {
		// Cleanup on failure
		_ = os.RemoveAll(componentDir)
		return result, err
	}

	// Step 6: Build component (unless SkipBuild is set)
	var buildOutput string
	if !opts.SkipBuild && manifest.Build != nil {
		buildStart := time.Now()
		buildResult, err := i.buildComponent(ctx, componentDir, manifest)
		buildDuration := time.Since(buildStart)

		if err != nil {
			// Cleanup on failure
			_ = os.RemoveAll(componentDir)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(ErrorAttributes(err, "build_component")...)
			return result, err
		}
		buildOutput = buildResult.Stdout + "\n" + buildResult.Stderr

		// Add build metrics to span
		span.SetAttributes(
			attribute.String(AttrBuildCommand, manifest.Build.Command),
			attribute.Int64(AttrBuildDuration, buildDuration.Milliseconds()),
		)
	}
	result.BuildOutput = buildOutput

	// Get the git version (commit hash)
	version, err := i.git.GetVersion(componentDir)
	if err != nil {
		// Use manifest version as fallback
		version = manifest.Version
	}

	// Step 7: Create component instance
	component := &Component{
		Kind:      kind,
		Name:      repoInfo.Name,
		Version:   version,
		Path:      componentDir,
		Source:    ComponentSourceExternal,
		Status:    ComponentStatusAvailable,
		Manifest:  manifest,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Validate component
	if err := component.Validate(); err != nil {
		// Cleanup on failure
		_ = os.RemoveAll(componentDir)
		return result, NewValidationFailedError("component validation failed", err)
	}

	// Step 8: Register component (unless SkipRegister is set)
	if !opts.SkipRegister && i.registry != nil {
		if err := i.registry.Register(component); err != nil {
			// Cleanup on failure
			_ = os.RemoveAll(componentDir)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(ErrorAttributes(err, "register_component")...)
			return result, WrapComponentError(ErrCodeLoadFailed, "failed to register component", err).
				WithComponent(repoInfo.Name)
		}
	}

	result.Component = component
	result.Duration = time.Since(start)
	result.Installed = true

	// Record successful installation
	span.SetStatus(codes.Ok, "component installed successfully")
	span.SetAttributes(ComponentAttributes(component)...)
	span.SetAttributes(InstallResultAttributes(result)...)

	return result, nil
}

// Update updates an installed component to the latest version
func (i *DefaultInstaller) Update(ctx context.Context, kind ComponentKind, name string, opts UpdateOptions) (*UpdateResult, error) {
	// Start tracing span
	ctx, span := i.tracer.Start(ctx, SpanComponentUpdate)
	defer span.End()

	// Add component metadata to span
	span.SetAttributes(
		attribute.String(AttrComponentKind, kind.String()),
		attribute.String(AttrComponentName, name),
	)

	start := time.Now()
	result := &UpdateResult{}

	// Set default timeout if not specified
	if opts.Timeout == 0 {
		opts.Timeout = DefaultInstallTimeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Validate component kind
	if !kind.IsValid() {
		err := NewInvalidKindError(kind.String())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_kind")...)
		return result, err
	}

	// Get component path
	componentDir := filepath.Join(i.homeDir, kind.String()+"s", name)
	result.Path = componentDir

	// Verify component exists
	if _, err := os.Stat(componentDir); os.IsNotExist(err) {
		notFoundErr := NewComponentNotFoundError(name).
			WithContext("path", componentDir)
		span.RecordError(notFoundErr)
		span.SetStatus(codes.Error, notFoundErr.Error())
		span.SetAttributes(ErrorAttributes(notFoundErr, "check_exists")...)
		return result, notFoundErr
	}

	// Get current component from registry if available
	var wasRunning bool
	if i.registry != nil {
		if comp := i.registry.Get(kind, name); comp != nil {
			result.OldVersion = comp.Version
			wasRunning = comp.IsRunning()

			// Step 1: Stop component if running
			// Note: Actual stop logic would be handled by a component manager
			// For now, we just note that it was running
			if wasRunning {
				// TODO: Implement stop via component manager
			}
		}
	}

	// If we don't have old version from registry, try to get it from git
	if result.OldVersion == "" {
		if version, err := i.git.GetVersion(componentDir); err == nil {
			result.OldVersion = version
		}
	}

	// Step 2: Pull latest changes
	if err := i.git.Pull(componentDir); err != nil {
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to pull latest changes", err).
			WithComponent(name)
	}

	// Get new version
	newVersion, err := i.git.GetVersion(componentDir)
	if err != nil {
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to get new version", err).
			WithComponent(name)
	}
	result.NewVersion = newVersion

	// Check if there were any changes
	if result.OldVersion == result.NewVersion {
		result.Duration = time.Since(start)
		result.Updated = false
		return result, nil
	}

	// Step 3: Load manifest
	manifestPath := filepath.Join(componentDir, ManifestFileName)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return result, err
	}

	// Check dependencies
	if err := i.checkDependencies(ctx, manifest); err != nil {
		return result, err
	}

	// Step 4: Rebuild component (unless SkipBuild is set)
	var buildOutput string
	if !opts.SkipBuild && manifest.Build != nil {
		buildResult, err := i.buildComponent(ctx, componentDir, manifest)
		if err != nil {
			return result, err
		}
		buildOutput = buildResult.Stdout + "\n" + buildResult.Stderr
	}
	result.BuildOutput = buildOutput

	// Step 5: Re-register component
	if i.registry != nil {
		component := &Component{
			Kind:      kind,
			Name:      name,
			Version:   newVersion,
			Path:      componentDir,
			Source:    ComponentSourceExternal,
			Status:    ComponentStatusAvailable,
			Manifest:  manifest,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Unregister old version
		_ = i.registry.Unregister(kind, name)

		// Register new version
		if err := i.registry.Register(component); err != nil {
			return result, WrapComponentError(ErrCodeLoadFailed, "failed to re-register component", err).
				WithComponent(name)
		}

		result.Component = component
	}

	// Step 6: Optionally restart if it was running
	if opts.Restart && wasRunning {
		// Note: Actual restart logic would depend on the component manager
		// For now, we just mark it as needing restart
		result.Restarted = false // Would be true after actual restart
	}

	result.Duration = time.Since(start)
	result.Updated = true

	// Record successful update
	span.SetStatus(codes.Ok, "component updated successfully")
	span.SetAttributes(UpdateResultAttributes(result)...)
	if result.Component != nil {
		span.SetAttributes(ComponentAttributes(result.Component)...)
	}

	return result, nil
}

// UpdateAll updates all installed components of a specific kind
func (i *DefaultInstaller) UpdateAll(ctx context.Context, kind ComponentKind, opts UpdateOptions) ([]UpdateResult, error) {
	// Validate component kind
	if !kind.IsValid() {
		return nil, NewInvalidKindError(kind.String())
	}

	// Get all components of this kind from registry
	var components []*Component
	var err error
	if i.registry != nil {
		components = i.registry.List(kind)
	} else {
		// If no registry, scan filesystem
		components, err = i.scanComponents(kind)
		if err != nil {
			return nil, err
		}
	}

	// Update each component
	results := make([]UpdateResult, 0, len(components))
	for _, comp := range components {
		result, err := i.Update(ctx, kind, comp.Name, opts)
		if err != nil {
			// Continue with other components even if one fails
			result = &UpdateResult{
				Component: comp,
				Path:      comp.Path,
				Updated:   false,
			}
		}
		results = append(results, *result)
	}

	return results, nil
}

// Uninstall removes an installed component
func (i *DefaultInstaller) Uninstall(ctx context.Context, kind ComponentKind, name string) (*UninstallResult, error) {
	// Start tracing span
	ctx, span := i.tracer.Start(ctx, SpanComponentUninstall)
	defer span.End()

	// Add component metadata to span
	span.SetAttributes(
		attribute.String(AttrComponentKind, kind.String()),
		attribute.String(AttrComponentName, name),
	)

	start := time.Now()
	result := &UninstallResult{
		Name: name,
		Kind: kind,
	}

	// Validate component kind
	if !kind.IsValid() {
		err := NewInvalidKindError(kind.String())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_kind")...)
		return result, err
	}

	// Get component path
	componentDir := filepath.Join(i.homeDir, kind.String()+"s", name)
	result.Path = componentDir

	// Verify component exists
	if _, err := os.Stat(componentDir); os.IsNotExist(err) {
		notFoundErr := NewComponentNotFoundError(name).
			WithContext("path", componentDir)
		span.RecordError(notFoundErr)
		span.SetStatus(codes.Error, notFoundErr.Error())
		span.SetAttributes(ErrorAttributes(notFoundErr, "check_exists")...)
		return result, notFoundErr
	}

	// Step 1: Stop component if running
	if i.registry != nil {
		if comp := i.registry.Get(kind, name); comp != nil {
			if comp.IsRunning() {
				result.WasRunning = true
				// TODO: Implement stop via component manager
				// For now, just note that it was running
				result.WasStopped = false
				span.SetAttributes(attribute.Bool("gibson.component.was_running", true))
			}
		}
	}

	// Step 2: Unregister from registry
	if i.registry != nil {
		_ = i.registry.Unregister(kind, name) // Ignore errors if not registered
	}

	// Step 3: Remove directory
	if err := os.RemoveAll(componentDir); err != nil {
		removeErr := WrapComponentError(ErrCodePermissionDenied, "failed to remove component directory", err).
			WithComponent(name).
			WithContext("path", componentDir)
		span.RecordError(removeErr)
		span.SetStatus(codes.Error, removeErr.Error())
		span.SetAttributes(ErrorAttributes(removeErr, "remove_directory")...)
		return result, removeErr
	}

	result.Duration = time.Since(start)

	// Record successful uninstall
	span.SetStatus(codes.Ok, "component uninstalled successfully")
	span.SetAttributes(UninstallResultAttributes(result)...)

	return result, nil
}

// checkDependencies validates that all component dependencies are satisfied
func (i *DefaultInstaller) checkDependencies(ctx context.Context, manifest *Manifest) error {
	if manifest.Dependencies == nil || !manifest.Dependencies.HasDependencies() {
		return nil
	}

	deps := manifest.Dependencies

	// Check system dependencies
	for _, sysDep := range deps.GetSystem() {
		// For now, just check if the command exists
		// In a real implementation, we would also check versions
		if err := i.checkSystemDependency(sysDep); err != nil {
			return NewDependencyFailedError(manifest.Name, sysDep, err, false)
		}
	}

	// Check component dependencies
	if i.registry != nil {
		for _, compDep := range deps.GetComponents() {
			if err := i.checkComponentDependency(compDep); err != nil {
				return NewDependencyFailedError(manifest.Name, compDep, err, false)
			}
		}
	}

	// Check required environment variables
	for key := range deps.GetEnv() {
		if os.Getenv(key) == "" {
			return NewDependencyFailedError(manifest.Name, key,
				fmt.Errorf("required environment variable %s not set", key), false)
		}
	}

	return nil
}

// checkSystemDependency checks if a system dependency is available
func (i *DefaultInstaller) checkSystemDependency(dep string) error {
	// Check if command exists in PATH
	if _, err := os.Stat(dep); err == nil {
		return nil
	}

	// For now, just return nil (assume dependency is available)
	// In a real implementation, we would check using exec.LookPath or similar
	// TODO: Implement proper system dependency checking
	return nil
}

// checkComponentDependency checks if a component dependency is satisfied
func (i *DefaultInstaller) checkComponentDependency(dep string) error {
	// Parse dependency format: name@version
	// For now, just check if component exists
	// In a real implementation, we would also check version compatibility
	return nil
}

// buildComponent builds a component using its build configuration
func (i *DefaultInstaller) buildComponent(ctx context.Context, componentDir string, manifest *Manifest) (*build.BuildResult, error) {
	if manifest.Build == nil {
		return nil, fmt.Errorf("no build configuration in manifest")
	}

	buildCfg := manifest.Build

	// Prepare build configuration
	buildConfig := build.BuildConfig{
		WorkDir:    componentDir,
		Command:    "make",
		Args:       []string{"build"},
		OutputPath: "", // Will be determined from build artifacts
		Env:        buildCfg.GetEnv(),
	}

	// Override with manifest build command if specified
	if buildCfg.Command != "" {
		buildConfig.Command = buildCfg.Command
		buildConfig.Args = []string{}
	}

	// Set working directory if specified
	if buildCfg.WorkDir != "" {
		buildConfig.WorkDir = filepath.Join(componentDir, buildCfg.WorkDir)
	}

	// Build with timeout
	buildCtx, cancel := context.WithTimeout(ctx, DefaultBuildTimeout)
	defer cancel()

	result, err := i.builder.Build(buildCtx, buildConfig, manifest.Name, manifest.Version, "dev")
	if err != nil {
		return result, WrapComponentError(ErrCodeLoadFailed, "build failed", err).
			WithComponent(manifest.Name)
	}

	return result, nil
}

// scanComponents scans the filesystem for installed components
func (i *DefaultInstaller) scanComponents(kind ComponentKind) ([]*Component, error) {
	componentsDir := filepath.Join(i.homeDir, kind.String()+"s")

	// Check if directory exists
	if _, err := os.Stat(componentsDir); os.IsNotExist(err) {
		return []*Component{}, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return nil, WrapComponentError(ErrCodeLoadFailed, "failed to read components directory", err)
	}

	// Load each component
	components := make([]*Component, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		componentDir := filepath.Join(componentsDir, entry.Name())
		manifestPath := filepath.Join(componentDir, ManifestFileName)

		// Load manifest
		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			// Skip components with invalid manifests
			continue
		}

		// Get version from git
		version := manifest.Version
		if gitVersion, err := i.git.GetVersion(componentDir); err == nil {
			version = gitVersion
		}

		component := &Component{
			Kind:      kind,
			Name:      entry.Name(),
			Version:   version,
			Path:      componentDir,
			Source:    ComponentSourceExternal,
			Status:    ComponentStatusAvailable,
			Manifest:  manifest,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		components = append(components, component)
	}

	return components, nil
}
