package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Subdir specifies a subdirectory within the repository where the component is located.
	// This is useful for mono-repos that contain multiple components.
	// Can also be specified in the repoURL using the fragment syntax: repo.git#path/to/component
	Subdir string
}

// ParsedRepoURL contains the parsed components of a repository URL
type ParsedRepoURL struct {
	// RepoURL is the base repository URL without the subdirectory fragment
	RepoURL string

	// Subdir is the subdirectory path extracted from the URL fragment (if any)
	Subdir string
}

// ParseRepoURL parses a repository URL that may contain a subdirectory fragment.
// Supports the syntax: https://github.com/user/repo.git#path/to/component
// or: git@github.com:user/repo.git#path/to/component
func ParseRepoURL(fullURL string) ParsedRepoURL {
	result := ParsedRepoURL{RepoURL: fullURL}

	// Look for the fragment separator
	if idx := strings.LastIndex(fullURL, "#"); idx != -1 {
		result.RepoURL = fullURL[:idx]
		result.Subdir = fullURL[idx+1:]
	}

	return result
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

// InstallAllResult contains the result of installing all components from a mono-repo
type InstallAllResult struct {
	// RepoURL is the repository that was cloned
	RepoURL string

	// Successful contains results for components that installed successfully
	Successful []InstallResult

	// Failed contains information about components that failed to install
	Failed []InstallFailure

	// Duration is the total time for the operation
	Duration time.Duration

	// ComponentsFound is the total number of components discovered
	ComponentsFound int
}

// InstallFailure contains information about a failed installation
type InstallFailure struct {
	// Path is the subdirectory path where the component was found
	Path string

	// Name is the component name (if manifest was readable)
	Name string

	// Error is the error that occurred
	Error error
}

// Installer defines the interface for installing, updating, and uninstalling components
type Installer interface {
	// Install installs a component from a git repository URL
	Install(ctx context.Context, repoURL string, kind ComponentKind, opts InstallOptions) (*InstallResult, error)

	// InstallAll clones a mono-repo and installs all components of the specified kind found within it
	InstallAll(ctx context.Context, repoURL string, kind ComponentKind, opts InstallOptions) (*InstallAllResult, error)

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
func (i *DefaultInstaller) Install(ctx context.Context, repoURL string, kind ComponentKind, opts InstallOptions) (*InstallResult, error) {
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

	// Step 1: Validate component kind
	if !kind.IsValid() {
		err := NewInvalidKindError(kind.String())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "validate_kind")...)
		return result, err
	}

	// Parse the repository URL for subdirectory fragment (e.g., repo.git#path/to/component)
	parsed := ParseRepoURL(repoURL)
	actualRepoURL := parsed.RepoURL

	// Subdirectory can come from URL fragment or from options (options take precedence)
	subdir := parsed.Subdir
	if opts.Subdir != "" {
		subdir = opts.Subdir
	}

	// Add component metadata to span (we'll add component name after reading manifest)
	span.SetAttributes(
		attribute.String(AttrRepoURL, actualRepoURL),
		attribute.String(AttrComponentKind, kind.String()),
	)
	if subdir != "" {
		span.SetAttributes(attribute.String("gibson.component.subdir", subdir))
	}

	// Step 2: Clone to temporary directory first
	tempDir, err := os.MkdirTemp("", "gibson-install-*")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "create_temp_dir")...)
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to create temporary directory", err)
	}
	defer os.RemoveAll(tempDir) // Clean up temp dir when done

	cloneOpts := git.CloneOptions{
		Branch: opts.Branch,
		Tag:    opts.Tag,
	}

	if err := i.git.Clone(actualRepoURL, tempDir, cloneOpts); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(ErrorAttributes(err, "clone_repository")...)
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to clone repository", err).
			WithContext("url", actualRepoURL)
	}

	// Step 3: Determine the component directory (apply subdirectory if specified)
	componentSourceDir := tempDir
	if subdir != "" {
		componentSourceDir = filepath.Join(tempDir, subdir)
		// Verify the subdirectory exists
		if _, err := os.Stat(componentSourceDir); os.IsNotExist(err) {
			return result, WrapComponentError(ErrCodeManifestNotFound, "subdirectory not found in repository", err).
				WithContext("subdir", subdir).
				WithContext("url", actualRepoURL)
		}
	}

	// Step 4: Load manifest from the component source directory
	manifestPath := filepath.Join(componentSourceDir, ManifestFileName)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return result, err // LoadManifest already returns proper ComponentError
	}

	// Add component name to span now that we have it
	span.SetAttributes(attribute.String(AttrComponentName, manifest.Name))

	// Step 5: Determine final installation path
	componentDir := filepath.Join(i.homeDir, kind.String()+"s", manifest.Name)
	result.Path = componentDir

	// Check if component already exists at final location
	if _, err := os.Stat(componentDir); err == nil {
		if !opts.Force {
			return result, NewComponentExistsError(manifest.Name).
				WithContext("path", componentDir)
		}
		// Force install: remove existing directory
		if err := os.RemoveAll(componentDir); err != nil {
			return result, WrapComponentError(ErrCodePermissionDenied, "failed to remove existing component", err).
				WithComponent(manifest.Name)
		}
	}

	// Step 6: Ensure parent directory exists
	parentDir := filepath.Dir(componentDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return result, WrapComponentError(ErrCodePermissionDenied, "failed to create parent directory", err).
			WithContext("path", parentDir)
	}

	// Step 7: Move component from source to final location
	// When using a subdirectory, we only move that subdirectory (not the whole repo)
	if err := os.Rename(componentSourceDir, componentDir); err != nil {
		// If rename fails (e.g., cross-device link), fall back to copy
		if err := copyDir(componentSourceDir, componentDir); err != nil {
			return result, WrapComponentError(ErrCodeLoadFailed, "failed to move component to final location", err).
				WithComponent(manifest.Name).
				WithContext("from", componentSourceDir).
				WithContext("to", componentDir)
		}
	}

	// Step 8: Check dependencies
	if err := i.checkDependencies(ctx, manifest); err != nil {
		// Cleanup on failure
		_ = os.RemoveAll(componentDir)
		return result, err
	}

	// Step 9: Build component (unless SkipBuild is set)
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

	// Step 10: Create component instance
	component := &Component{
		Kind:      kind,
		Name:      manifest.Name,
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

	// Step 11: Register component (unless SkipRegister is set)
	if !opts.SkipRegister && i.registry != nil {
		if err := i.registry.Register(component); err != nil {
			// Cleanup on failure
			_ = os.RemoveAll(componentDir)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(ErrorAttributes(err, "register_component")...)
			return result, WrapComponentError(ErrCodeLoadFailed, "failed to register component", err).
				WithComponent(manifest.Name)
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

// InstallAll clones a mono-repo and installs all components of the specified kind found within it.
// It recursively walks the repository looking for component.yaml files and installs each one.
func (i *DefaultInstaller) InstallAll(ctx context.Context, repoURL string, kind ComponentKind, opts InstallOptions) (*InstallAllResult, error) {
	// Start tracing span
	ctx, span := i.tracer.Start(ctx, "component.install_all")
	defer span.End()

	start := time.Now()
	result := &InstallAllResult{
		RepoURL:    repoURL,
		Successful: make([]InstallResult, 0),
		Failed:     make([]InstallFailure, 0),
	}

	// Set default timeout if not specified (longer for bulk operations)
	if opts.Timeout == 0 {
		opts.Timeout = DefaultInstallTimeout * 3 // 15 minutes for bulk install
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Step 1: Validate component kind
	if !kind.IsValid() {
		err := NewInvalidKindError(kind.String())
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return result, err
	}

	// Parse the repository URL (strip any subdirectory fragment for bulk install)
	parsed := ParseRepoURL(repoURL)
	actualRepoURL := parsed.RepoURL

	span.SetAttributes(
		attribute.String(AttrRepoURL, actualRepoURL),
		attribute.String(AttrComponentKind, kind.String()),
	)

	// Step 2: Clone to temporary directory
	tempDir, err := os.MkdirTemp("", "gibson-install-all-*")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to create temporary directory", err)
	}
	defer os.RemoveAll(tempDir)

	cloneOpts := git.CloneOptions{
		Branch: opts.Branch,
		Tag:    opts.Tag,
	}

	if err := i.git.Clone(actualRepoURL, tempDir, cloneOpts); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to clone repository", err).
			WithContext("url", actualRepoURL)
	}

	// Step 3: Walk the repository and find all component.yaml files
	componentPaths, err := i.findComponentManifests(tempDir)
	if err != nil {
		return result, WrapComponentError(ErrCodeLoadFailed, "failed to scan repository for components", err)
	}

	result.ComponentsFound = len(componentPaths)
	span.SetAttributes(attribute.Int("gibson.components_found", len(componentPaths)))

	if len(componentPaths) == 0 {
		result.Duration = time.Since(start)
		return result, nil
	}

	// Step 4: Install each component
	// Note: Repositories should contain only one type of component (tools-only, agents-only, etc.)
	// The kind is determined by the CLI subcommand, not the manifest.
	for _, manifestPath := range componentPaths {
		// Get the subdirectory relative to the temp dir
		subdir := filepath.Dir(manifestPath)
		relSubdir, err := filepath.Rel(tempDir, subdir)
		if err != nil {
			relSubdir = subdir
		}

		// Load manifest to get component name for error reporting
		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			result.Failed = append(result.Failed, InstallFailure{
				Path:  relSubdir,
				Error: err,
			})
			continue
		}

		// Install this component using the subdirectory syntax
		installURL := actualRepoURL + "#" + relSubdir
		installResult, err := i.Install(ctx, installURL, kind, opts)
		if err != nil {
			result.Failed = append(result.Failed, InstallFailure{
				Path:  relSubdir,
				Name:  manifest.Name,
				Error: err,
			})
			continue
		}

		result.Successful = append(result.Successful, *installResult)
	}

	result.Duration = time.Since(start)

	// Set span status based on results
	if len(result.Failed) == 0 {
		span.SetStatus(codes.Ok, fmt.Sprintf("installed %d components successfully", len(result.Successful)))
	} else if len(result.Successful) > 0 {
		span.SetStatus(codes.Ok, fmt.Sprintf("installed %d components, %d failed", len(result.Successful), len(result.Failed)))
	} else {
		span.SetStatus(codes.Error, fmt.Sprintf("all %d components failed to install", len(result.Failed)))
	}

	span.SetAttributes(
		attribute.Int("gibson.components_successful", len(result.Successful)),
		attribute.Int("gibson.components_failed", len(result.Failed)),
	)

	return result, nil
}

// findComponentManifests recursively walks a directory and returns paths to all component.yaml files
func (i *DefaultInstaller) findComponentManifests(rootDir string) ([]string, error) {
	var manifests []string

	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}

		// Skip hidden directories (like .git)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// Skip common non-component directories
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "vendor", "__pycache__", ".venv", "venv", "pkg", "build", "dist", "target":
				return filepath.SkipDir
			}
		}

		// Check if this is a component manifest
		if !d.IsDir() && (d.Name() == "component.yaml" || d.Name() == "component.json") {
			manifests = append(manifests, path)
		}

		return nil
	})

	return manifests, err
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

// copyDir copies a directory recursively from src to dst
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	// Get source file info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := dstFile.ReadFrom(srcFile); err != nil {
		return err
	}

	// Set permissions
	return os.Chmod(dst, srcInfo.Mode())
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
