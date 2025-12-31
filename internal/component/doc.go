// Package component provides external component management for the Gibson Framework.
//
// # Overview
//
// The component package enables Gibson to discover, install, manage, and monitor
// external components (agents, tools, and plugins). It provides a complete lifecycle
// management system including installation from git repositories, building, starting,
// health monitoring, and graceful shutdown.
//
// # Main Interfaces
//
// The package defines three primary interfaces for component management:
//
// ## ComponentDAO (database.ComponentDAO)
//
// ComponentDAO manages the registration and persistence of components in SQLite:
//
//	type ComponentDAO interface {
//	    Create(ctx context.Context, comp *Component) error
//	    GetByID(ctx context.Context, id int64) (*Component, error)
//	    GetByName(ctx context.Context, kind ComponentKind, name string) (*Component, error)
//	    List(ctx context.Context, kind ComponentKind) ([]*Component, error)
//	    ListAll(ctx context.Context) (map[ComponentKind][]*Component, error)
//	    Update(ctx context.Context, comp *Component) error
//	    UpdateStatus(ctx context.Context, id int64, status ComponentStatus, pid, port int) error
//	    Delete(ctx context.Context, kind ComponentKind, name string) error
//	}
//
// The DAO is used by the Installer and LifecycleManager to persist component metadata.
// Components are organized by kind (agent, tool, plugin) and name, with unique constraints
// enforced by the database schema.
//
// ## Installer
//
// Installer handles component installation, updates, and removal:
//
//	type Installer interface {
//	    Install(ctx context.Context, repoURL string, kind ComponentKind, opts InstallOptions) (*InstallResult, error)
//	    Update(ctx context.Context, kind ComponentKind, name string, opts UpdateOptions) (*UpdateResult, error)
//	    UpdateAll(ctx context.Context, kind ComponentKind, opts UpdateOptions) ([]UpdateResult, error)
//	    Uninstall(ctx context.Context, kind ComponentKind, name string) (*UninstallResult, error)
//	}
//
// The installer clones git repositories, validates manifests, checks dependencies,
// builds components, and registers them in the component registry. The component kind
// is provided by the command context (e.g., 'gibson agent install' vs 'gibson tool install'),
// rather than being embedded in the manifest file.
//
// ## LifecycleManager
//
// LifecycleManager controls component execution and process management:
//
//	type LifecycleManager interface {
//	    StartComponent(ctx context.Context, comp *Component) (int, error)
//	    StopComponent(ctx context.Context, comp *Component) error
//	    RestartComponent(ctx context.Context, comp *Component) (int, error)
//	    GetStatus(ctx context.Context, comp *Component) (ComponentStatus, error)
//	}
//
// The lifecycle manager starts components as processes, assigns ports, monitors
// process health, and performs graceful shutdown (SIGTERM followed by SIGKILL).
//
// ## HealthMonitor
//
// HealthMonitor performs periodic health checks on running components:
//
//	type HealthMonitor interface {
//	    Start(ctx context.Context) error
//	    Stop() error
//	    CheckComponent(ctx context.Context, healthEndpoint string) error
//	    OnStatusChange(callback StatusChangeCallback)
//	    GetHealth(componentName string) HealthStatus
//	    RegisterComponent(name string, healthEndpoint string)
//	    UnregisterComponent(name string)
//	}
//
// The health monitor runs in the background, periodically checking registered
// components and invoking callbacks when health status changes.
//
// # Manifest Format
//
// Components are described by a manifest file (component.yaml) that defines
// their metadata, build configuration, runtime requirements, and dependencies.
//
// Example manifest:
//
//	name: scanner
//	version: 1.0.0
//	description: Network vulnerability scanner agent
//	author: Security Team
//	license: MIT
//	repository: https://github.com/org/gibson-agent-scanner
//
// Note: Component kind is no longer stored in the manifest. Instead, it is determined
// by the installation command (e.g., 'gibson agent install' for agents). This allows
// the same component to be used in different contexts if needed.
//
//	build:
//	  command: make build
//	  artifacts:
//	    - bin/scanner
//	  workdir: .
//	  env:
//	    CGO_ENABLED: "0"
//	    GOOS: linux
//
//	runtime:
//	  type: go
//	  entrypoint: ./bin/scanner
//	  args:
//	    - --verbose
//	  env:
//	    LOG_LEVEL: info
//	  port: 50000
//	  health_url: /health
//	  workdir: /opt/scanner
//
//	dependencies:
//	  gibson: ">=1.0.0"
//	  components:
//	    - nmap-tool@2.0.0
//	  system:
//	    - docker
//	    - nmap
//	  env:
//	    SCANNER_API_KEY: required
//
// # Component Lifecycle
//
// Components follow a well-defined lifecycle from installation to removal:
//
//  1. Install: Clone git repository, validate manifest, check dependencies, build component
//  2. Register: Add component to registry and persist to disk
//  3. Start: Launch component process, assign port, wait for health check
//  4. Monitor: Periodic health checks, status change notifications
//  5. Stop: Graceful shutdown (SIGTERM) with timeout, force kill (SIGKILL) if needed
//  6. Update: Pull latest changes, rebuild, optionally restart
//  7. Uninstall: Stop component, unregister, remove files
//
// # Component Kinds
//
// Gibson supports three component types:
//
//   - Agent (ComponentKindAgent): Autonomous services that perform specific tasks
//   - Tool (ComponentKindTool): Utilities and external programs invoked by agents
//   - Plugin (ComponentKindPlugin): Extensions that add functionality to Gibson
//
// # Component Sources
//
// Components can originate from different sources:
//
//   - Internal (ComponentSourceInternal): Built-in components distributed with Gibson
//   - External (ComponentSourceExternal): Third-party components installed from git
//   - Remote (ComponentSourceRemote): Network services accessed via HTTP/gRPC
//   - Config (ComponentSourceConfig): Components defined in configuration files
//
// # Component Statuses
//
// Components transition through various statuses during their lifecycle:
//
//   - Available (ComponentStatusAvailable): Installed and ready to start
//   - Running (ComponentStatusRunning): Currently executing with active process
//   - Stopped (ComponentStatusStopped): Previously running, now stopped
//   - Error (ComponentStatusError): Encountered error, health check failing
//
// # Configuration Options
//
// The ComponentConfig type (defined in manifest) supports various runtime options:
//
//   - Runtime Type: Execution environment (go, python, node, docker, binary, http, grpc)
//   - Entrypoint: Command or executable to run
//   - Arguments: Command-line arguments passed to the component
//   - Environment: Environment variables for the component
//   - Working Directory: Execution directory
//   - Port: Network port for HTTP/gRPC components
//   - Health URL: Endpoint for health checks
//   - Volumes: Docker volume mounts (for container runtime)
//
// # Usage Examples
//
// ## Installing a Component
//
// Install a component from a git repository:
//
//	import (
//	    "context"
//	    "github.com/zero-day-ai/gibson/internal/component"
//	    "github.com/zero-day-ai/gibson/internal/component/git"
//	    "github.com/zero-day-ai/gibson/internal/component/build"
//	)
//
//	func main() {
//	    // Create dependencies
//	    gitOps := git.NewDefaultGitOperations()
//	    builder := build.NewDefaultBuildExecutor()
//	    dao := database.NewComponentDAO(db)
//
//	    // Create installer
//	    installer := component.NewDefaultInstaller(gitOps, builder, dao)
//
//	    // Install component
//	    ctx := context.Background()
//	    repoURL := "https://github.com/org/gibson-agent-scanner"
//	    kind := component.ComponentKindAgent  // Kind determined by command context
//	    opts := component.InstallOptions{
//	        Force:        false,
//	        SkipBuild:    false,
//	        SkipRegister: false,
//	        Timeout:      5 * time.Minute,
//	    }
//
//	    result, err := installer.Install(ctx, repoURL, kind, opts)
//	    if err != nil {
//	        log.Fatalf("Installation failed: %v", err)
//	    }
//
//	    fmt.Printf("Installed %s v%s in %s\n",
//	        result.Component.Name,
//	        result.Component.Version,
//	        result.Duration)
//	}
//
// ## Managing Component Lifecycle
//
// Start, monitor, and stop a component:
//
//	func manageComponent() {
//	    // Create lifecycle manager and health monitor
//	    healthMonitor := component.NewHealthMonitor()
//	    lifecycleManager := component.NewLifecycleManager(healthMonitor)
//
//	    // Start health monitoring
//	    ctx := context.Background()
//	    if err := healthMonitor.Start(ctx); err != nil {
//	        log.Fatalf("Failed to start health monitor: %v", err)
//	    }
//	    defer healthMonitor.Stop()
//
//	    // Get component from database
//	    dao := database.NewComponentDAO(db)
//	    comp, err := dao.GetByName(ctx, component.ComponentKindAgent, "scanner")
//	    if err != nil || comp == nil {
//	        log.Fatal("Component not found")
//	    }
//
//	    // Start component
//	    port, err := lifecycleManager.StartComponent(ctx, comp)
//	    if err != nil {
//	        log.Fatalf("Failed to start component: %v", err)
//	    }
//	    fmt.Printf("Component started on port %d with PID %d\n", port, comp.PID)
//
//	    // Register for health monitoring
//	    healthURL := fmt.Sprintf("http://localhost:%d/health", port)
//	    healthMonitor.RegisterComponent(comp.Name, healthURL)
//
//	    // Monitor status changes
//	    healthMonitor.OnStatusChange(func(name string, oldStatus, newStatus component.HealthStatus) {
//	        fmt.Printf("Component %s status changed: %s -> %s\n", name, oldStatus, newStatus)
//	    })
//
//	    // Do work...
//	    time.Sleep(5 * time.Minute)
//
//	    // Stop component
//	    if err := lifecycleManager.StopComponent(ctx, comp); err != nil {
//	        log.Fatalf("Failed to stop component: %v", err)
//	    }
//	    fmt.Println("Component stopped successfully")
//	}
//
// ## Using the Component Registry
//
// Register, query, and persist components:
//
//	func useComponentDAO() {
//	    dao := database.NewComponentDAO(db)
//	    ctx := context.Background()
//
//	    // Create and register a component
//	    comp := &component.Component{
//	        Kind:      component.ComponentKindAgent,
//	        Name:      "scanner",
//	        Version:   "1.0.0",
//	        Path:      "/home/user/.gibson/agents/scanner",
//	        Source:    component.ComponentSourceExternal,
//	        Status:    component.ComponentStatusAvailable,
//	        CreatedAt: time.Now(),
//	        UpdatedAt: time.Now(),
//	    }
//
//	    // Register the component
//	    if err := dao.Create(ctx, comp); err != nil {
//	        log.Fatalf("Failed to register component: %v", err)
//	    }
//
//	    // Query components
//	    scanner, err := dao.GetByName(ctx, component.ComponentKindAgent, "scanner")
//	    if err != nil {
//	        log.Fatalf("Failed to get component: %v", err)
//	    }
//	    agents, err := dao.List(ctx, component.ComponentKindAgent)
//	    if err != nil {
//	        log.Fatalf("Failed to list agents: %v", err)
//	    }
//
//	    fmt.Printf("Found component: %s v%s\n", scanner.Name, scanner.Version)
//	    fmt.Printf("Total agents: %d\n", len(agents))
//
//	    // List all components
//	    allComponents, err := dao.ListAll(ctx)
//	    if err != nil {
//	        log.Fatalf("Failed to list components: %v", err)
//	    }
//	    totalCount := 0
//	    for _, components := range allComponents {
//	        totalCount += len(components)
//	    }
//	    fmt.Printf("Total components: %d\n", totalCount)
//	}
//
// ## Loading and Validating Manifests
//
// Load component manifests and access their configuration:
//
//	func loadManifest() {
//	    manifestPath := "/path/to/component.yaml"
//	    manifest, err := component.LoadManifest(manifestPath)
//	    if err != nil {
//	        log.Fatalf("Failed to load manifest: %v", err)
//	    }
//
//	    // Access manifest fields
//	    fmt.Printf("Component: %s v%s\n",
//	        manifest.Name,
//	        manifest.Version)
//	    // Note: Kind is not stored in manifest, it's provided during installation
//
//	    // Check runtime configuration
//	    if manifest.Runtime.IsNetworkBased() {
//	        fmt.Printf("Network component on port %d\n", manifest.Runtime.Port)
//	    }
//
//	    // Check dependencies
//	    if manifest.Dependencies != nil && manifest.Dependencies.HasDependencies() {
//	        fmt.Printf("Dependencies:\n")
//	        fmt.Printf("  Gibson: %s\n", manifest.Dependencies.Gibson)
//	        for _, dep := range manifest.Dependencies.GetComponents() {
//	            fmt.Printf("  Component: %s\n", dep)
//	        }
//	        for _, dep := range manifest.Dependencies.GetSystem() {
//	            fmt.Printf("  System: %s\n", dep)
//	        }
//	    }
//
//	    // Access build configuration
//	    if manifest.Build != nil {
//	        fmt.Printf("Build command: %s\n", manifest.Build.Command)
//	        for _, artifact := range manifest.Build.GetBuildArtifacts() {
//	            fmt.Printf("  Artifact: %s\n", artifact)
//	        }
//	    }
//	}
//
// ## Handling Errors
//
// The package provides structured error handling with ComponentError:
//
//	func handleErrors() {
//	    dao := database.NewComponentDAO(db)
//	    comp, err := dao.GetByName(ctx, component.ComponentKindAgent, "scanner")
//
//	    if err != nil || comp == nil {
//	        // Component not found - not an error, just nil
//	        fmt.Println("Component not found")
//	        return
//	    }
//
//	    // Operations that may return ComponentError
//	    err := registry.Register(comp)
//	    if err != nil {
//	        // Check if it's a ComponentError
//	        var compErr *component.ComponentError
//	        if errors.As(err, &compErr) {
//	            // Access error details
//	            fmt.Printf("Error code: %s\n", compErr.Code)
//	            fmt.Printf("Component: %s\n", compErr.Component)
//	            fmt.Printf("Retryable: %t\n", compErr.Retryable)
//
//	            // Check error context
//	            for key, value := range compErr.Context {
//	                fmt.Printf("  %s: %v\n", key, value)
//	            }
//
//	            // Check for specific error types
//	            switch compErr.Code {
//	            case component.ErrCodeComponentExists:
//	                fmt.Println("Component already exists, use Force option")
//	            case component.ErrCodeComponentNotFound:
//	                fmt.Println("Component not found in registry")
//	            case component.ErrCodeValidationFailed:
//	                fmt.Println("Component validation failed")
//	            case component.ErrCodeTimeout:
//	                if compErr.Retryable {
//	                    fmt.Println("Operation timed out, retrying...")
//	                }
//	            }
//	        }
//	    }
//	}
//
// ## Updating Components
//
// Update installed components to their latest versions:
//
//	func updateComponents() {
//	    gitOps := git.NewDefaultGitOperations()
//	    builder := build.NewDefaultBuildExecutor()
//	    dao := database.NewComponentDAO(db)
//	    installer := component.NewDefaultInstaller(gitOps, builder, dao)
//
//	    ctx := context.Background()
//	    opts := component.UpdateOptions{
//	        Restart:   true,
//	        SkipBuild: false,
//	        Timeout:   5 * time.Minute,
//	    }
//
//	    // Update single component
//	    result, err := installer.Update(ctx, component.ComponentKindAgent, "scanner", opts)
//	    if err != nil {
//	        log.Fatalf("Update failed: %v", err)
//	    }
//
//	    if result.Updated {
//	        fmt.Printf("Updated %s: %s -> %s\n",
//	            result.Component.Name,
//	            result.OldVersion,
//	            result.NewVersion)
//	    } else {
//	        fmt.Println("Component is already up to date")
//	    }
//
//	    // Update all agents
//	    results, err := installer.UpdateAll(ctx, component.ComponentKindAgent, opts)
//	    if err != nil {
//	        log.Fatalf("Batch update failed: %v", err)
//	    }
//
//	    for _, result := range results {
//	        if result.Updated {
//	            fmt.Printf("Updated %s\n", result.Component.Name)
//	        }
//	    }
//	}
//
// # Thread Safety
//
// The ComponentDAO uses SQLite for persistence, which provides ACID guarantees
// through transactions. Multiple processes can safely access the database
// concurrently using SQLite's WAL mode.
//
// The HealthMonitor also uses proper synchronization for concurrent access to
// monitored components and callbacks.
//
// # Error Handling
//
// The package uses structured errors (ComponentError) that include:
//   - Error codes for programmatic handling
//   - Human-readable messages
//   - Underlying cause errors (unwrappable)
//   - Component context information
//   - Retryable flag for transient errors
//
// All errors can be unwrapped using errors.Is() and errors.As() for proper
// error chain inspection.
//
// # Best Practices
//
//  1. Always use context.Context for cancellation and timeouts
//  2. Check ComponentError.Retryable before implementing retry logic
//  3. Validate manifests before attempting installation
//  4. Monitor component health in production environments
//  5. Implement graceful shutdown handlers for component processes
//  6. Component state is automatically persisted to SQLite
//  7. Handle dependency failures gracefully
//  8. Set appropriate timeouts for install/build operations
//  9. Clean up resources on installation failures
//  10. Use structured logging with component context
//
// # Testing
//
// The package includes comprehensive test coverage including unit tests,
// integration tests, and concurrent operation tests. Mock implementations
// are provided for git operations and build execution to facilitate testing
// without external dependencies.
package component
