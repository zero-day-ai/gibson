package component

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	dclient "github.com/zero-day-ai/gibson/internal/daemon/client"
	sdkregistry "github.com/zero-day-ai/sdk/registry"
)

// StatusOutput represents the JSON output structure for component status.
type StatusOutput struct {
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	Instances []InstanceOutput `json:"instances"`
	Count     int              `json:"count"`
}

// InstanceOutput represents a single instance in JSON output.
type InstanceOutput struct {
	InstanceID string            `json:"instance_id"`
	Endpoint   string            `json:"endpoint"`
	StartedAt  time.Time         `json:"started_at"`
	Uptime     string            `json:"uptime,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// AllStatusOutput represents the JSON output for all components.
type AllStatusOutput struct {
	Components []ComponentSummary `json:"components"`
	Total      int                `json:"total"`
}

// ComponentSummary represents a summary of a component with instance count.
type ComponentSummary struct {
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Instances int      `json:"instances"`
	Endpoints []string `json:"endpoints"`
}

// newStatusCommand creates a status command for the specified component type.
func newStatusCommand(cfg Config, flags *StatusFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [NAME]",
		Short: "Show component status from registry",
		Long: fmt.Sprintf(`Display status of registered %ss from the service registry.

Without arguments, shows all registered %ss with instance counts.
With a component name, shows detailed information about all instances of that %s.

The registry is the source of truth for all component discovery.
Components automatically register when started and deregister when stopped.`,
			cfg.DisplayName, cfg.DisplayName, cfg.DisplayName),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, cfg, flags)
		},
	}

	// Register flags
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "Output in JSON format")

	return cmd
}

// runStatus executes the status command for a component.
func runStatus(cmd *cobra.Command, args []string, cfg Config, flags *StatusFlags) error {
	var componentName string
	if len(args) > 0 {
		componentName = args[0]
	}

	// Check for daemon client first - prefer daemon registry for live status
	ctx := cmd.Context()
	if daemonClient := GetDaemonClient(ctx); daemonClient != nil {
		return queryComponentStatusViaDaemon(cmd, daemonClient, cfg, componentName, flags)
	}

	// Fall back to local registry when daemon unavailable
	fmt.Fprintln(os.Stderr, "[WARN] Daemon not running, showing local registry data")

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer internal.CloseWithLog(cc, nil, "gRPC connection")

	// Build status options
	opts := core.StatusOptions{
		Watch:      flags.Watch,
		Interval:   flags.Interval,
		ErrorCount: flags.ErrorCount,
		JSON:       flags.JSON,
	}

	// Call core function
	result, err := core.ComponentStatus(cc, cfg.Kind, componentName, opts)
	if err != nil {
		return err
	}

	// Extract result data
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	// Check if this is a specific component or all components
	if componentName != "" {
		return displayComponentInstances(cmd, cfg, flags, data)
	}

	return displayAllComponents(cmd, cfg, flags, data)
}

// displayAllComponents displays all registered components from the result data.
func displayAllComponents(cmd *cobra.Command, cfg Config, flags *StatusFlags, data map[string]interface{}) error {
	instances, ok := data["instances"].([]sdkregistry.ServiceInfo)
	if !ok {
		return fmt.Errorf("unexpected instances type")
	}

	if len(instances) == 0 {
		cmd.Printf("No %ss registered\n", cfg.DisplayName)
		return nil
	}

	// Group instances by name
	componentMap := make(map[string][]sdkregistry.ServiceInfo)
	for _, instance := range instances {
		componentMap[instance.Name] = append(componentMap[instance.Name], instance)
	}

	// Build summaries
	var summaries []ComponentSummary
	for name, insts := range componentMap {
		// Get version from first instance (all should have same version)
		version := ""
		if len(insts) > 0 {
			version = insts[0].Version
		}

		// Collect endpoints
		endpoints := make([]string, 0, len(insts))
		for _, inst := range insts {
			endpoints = append(endpoints, inst.Endpoint)
		}

		summaries = append(summaries, ComponentSummary{
			Name:      name,
			Version:   version,
			Instances: len(insts),
			Endpoints: endpoints,
		})
	}

	// Sort by name for consistent output
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})

	// Output JSON or table
	if flags.JSON {
		return outputAllJSON(cmd, summaries)
	}

	return displayAllTable(cmd, cfg, summaries)
}

// displayComponentInstances displays detailed information about all instances from the result data.
func displayComponentInstances(cmd *cobra.Command, cfg Config, flags *StatusFlags, data map[string]interface{}) error {
	componentName, ok := data["name"].(string)
	if !ok {
		return fmt.Errorf("unexpected name type")
	}

	instances, ok := data["instances"].([]sdkregistry.ServiceInfo)
	if !ok {
		return fmt.Errorf("unexpected instances type")
	}

	// Sort by instance ID for consistent output
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceID < instances[j].InstanceID
	})

	// Output JSON or detailed view
	if flags.JSON {
		return outputInstancesJSON(cmd, componentName, instances)
	}

	return displayInstancesDetail(cmd, cfg, componentName, instances)
}

// outputAllJSON outputs all components in JSON format.
func outputAllJSON(cmd *cobra.Command, summaries []ComponentSummary) error {
	output := AllStatusOutput{
		Components: summaries,
		Total:      len(summaries),
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	cmd.Println(string(jsonBytes))
	return nil
}

// outputInstancesJSON outputs component instances in JSON format.
func outputInstancesJSON(cmd *cobra.Command, componentName string, instances []sdkregistry.ServiceInfo) error {
	// Get version from first instance
	version := ""
	if len(instances) > 0 {
		version = instances[0].Version
	}

	// Build instance outputs
	instanceOutputs := make([]InstanceOutput, 0, len(instances))
	for _, inst := range instances {
		uptime := ""
		if !inst.StartedAt.IsZero() {
			uptime = formatDuration(time.Since(inst.StartedAt))
		}

		instanceOutputs = append(instanceOutputs, InstanceOutput{
			InstanceID: inst.InstanceID,
			Endpoint:   inst.Endpoint,
			StartedAt:  inst.StartedAt,
			Uptime:     uptime,
			Metadata:   inst.Metadata,
		})
	}

	output := StatusOutput{
		Name:      componentName,
		Version:   version,
		Instances: instanceOutputs,
		Count:     len(instanceOutputs),
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	cmd.Println(string(jsonBytes))
	return nil
}

// displayAllTable displays all components in table format.
func displayAllTable(cmd *cobra.Command, cfg Config, summaries []ComponentSummary) error {
	cmd.Printf("Registered %ss:\n\n", titleCaser.String(cfg.DisplayName))

	// Calculate column widths
	maxNameLen := len("NAME")
	maxVersionLen := len("VERSION")
	for _, s := range summaries {
		if len(s.Name) > maxNameLen {
			maxNameLen = len(s.Name)
		}
		if len(s.Version) > maxVersionLen {
			maxVersionLen = len(s.Version)
		}
	}

	// Print header
	nameHeader := padRight("NAME", maxNameLen)
	versionHeader := padRight("VERSION", maxVersionLen)
	cmd.Printf("%s  %s  INSTANCES  ENDPOINTS\n", nameHeader, versionHeader)
	cmd.Printf("%s  %s  %s  %s\n",
		strings.Repeat("-", maxNameLen),
		strings.Repeat("-", maxVersionLen),
		strings.Repeat("-", 9),
		strings.Repeat("-", 40))

	// Print rows
	for _, s := range summaries {
		name := padRight(s.Name, maxNameLen)
		version := padRight(s.Version, maxVersionLen)
		instances := fmt.Sprintf("%-9d", s.Instances)
		endpoints := strings.Join(s.Endpoints, ", ")

		// Truncate endpoints if too long
		if len(endpoints) > 40 {
			endpoints = endpoints[:37] + "..."
		}

		cmd.Printf("%s  %s  %s  %s\n", name, version, instances, endpoints)
	}

	cmd.Printf("\nTotal: %d %s(s)\n", len(summaries), cfg.DisplayName)
	return nil
}

// displayInstancesDetail displays detailed information about component instances.
func displayInstancesDetail(cmd *cobra.Command, cfg Config, componentName string, instances []sdkregistry.ServiceInfo) error {
	// Get version from first instance (all should have same version)
	version := ""
	if len(instances) > 0 {
		version = instances[0].Version
	}

	cmd.Printf("%s: %s\n", titleCaser.String(cfg.DisplayName), componentName)
	cmd.Printf("Version: %s\n", version)
	cmd.Printf("Instances: %d\n\n", len(instances))

	// Display each instance
	for i, inst := range instances {
		if i > 0 {
			cmd.Println()
		}

		cmd.Printf("Instance %d:\n", i+1)
		cmd.Printf("  Instance ID: %s\n", inst.InstanceID)
		cmd.Printf("  Endpoint: %s\n", inst.Endpoint)

		if !inst.StartedAt.IsZero() {
			cmd.Printf("  Started At: %s\n", inst.StartedAt.Format(time.RFC3339))
			uptime := time.Since(inst.StartedAt)
			cmd.Printf("  Uptime: %s\n", formatDuration(uptime))
		}

		// Display metadata if present
		if len(inst.Metadata) > 0 {
			cmd.Printf("  Metadata:\n")

			// Sort metadata keys for consistent output
			keys := make([]string, 0, len(inst.Metadata))
			for k := range inst.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				cmd.Printf("    %s: %s\n", k, inst.Metadata[k])
			}
		}
	}

	return nil
}

// formatDuration formats a duration in a human-readable format (e.g., "2h 15m 30s").
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if seconds > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dh", hours)
}

// padRight pads a string to the right with spaces to reach the specified width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// queryComponentStatusViaDaemon queries component status from the daemon registry.
func queryComponentStatusViaDaemon(cmd *cobra.Command, daemonClient interface{}, cfg Config, componentName string, flags *StatusFlags) error {
	// Type assert to daemon client
	client, ok := daemonClient.(*dclient.Client)
	if !ok {
		return fmt.Errorf("invalid daemon client type")
	}

	ctx := cmd.Context()

	// Query appropriate component type from daemon
	switch cfg.Kind {
	case "agent":
		agents, err := client.ListAgents(ctx)
		if err != nil {
			return fmt.Errorf("failed to list agents from daemon: %w", err)
		}
		return displayDaemonComponents(cmd, "AGENTS", agents, componentName)

	case "tool":
		tools, err := client.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tools from daemon: %w", err)
		}
		return displayDaemonTools(cmd, tools, componentName)

	case "plugin":
		plugins, err := client.ListPlugins(ctx)
		if err != nil {
			return fmt.Errorf("failed to list plugins from daemon: %w", err)
		}
		return displayDaemonPlugins(cmd, plugins, componentName)

	default:
		return fmt.Errorf("unknown component kind: %s", cfg.Kind)
	}
}

// displayDaemonComponents displays agent components from daemon.
func displayDaemonComponents(cmd *cobra.Command, title string, agents []dclient.AgentInfo, filter string) error {
	if len(agents) == 0 {
		fmt.Println("No components registered")
		return nil
	}

	// Filter if needed
	if filter != "" {
		filtered := make([]dclient.AgentInfo, 0)
		for _, a := range agents {
			if strings.Contains(a.Name, filter) {
				filtered = append(filtered, a)
			}
		}
		agents = filtered
	}

	// Display table
	fmt.Printf("\n%s (%d registered)\n\n", title, len(agents))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tADDRESS")
	for _, a := range agents {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Name, a.Version, a.Status, a.Address)
	}
	w.Flush()

	return nil
}

// displayDaemonTools displays tool components from daemon.
func displayDaemonTools(cmd *cobra.Command, tools []dclient.ToolInfo, filter string) error {
	if len(tools) == 0 {
		fmt.Println("No tools registered")
		return nil
	}

	// Filter if needed
	if filter != "" {
		filtered := make([]dclient.ToolInfo, 0)
		for _, t := range tools {
			if strings.Contains(t.Name, filter) {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	// Display table
	fmt.Printf("\nTOOLS (%d registered)\n\n", len(tools))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tADDRESS")
	for _, t := range tools {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Version, t.Status, t.Address)
	}
	w.Flush()

	return nil
}

// displayDaemonPlugins displays plugin components from daemon.
func displayDaemonPlugins(cmd *cobra.Command, plugins []dclient.PluginInfo, filter string) error {
	if len(plugins) == 0 {
		fmt.Println("No plugins registered")
		return nil
	}

	// Filter if needed
	if filter != "" {
		filtered := make([]dclient.PluginInfo, 0)
		for _, p := range plugins {
			if strings.Contains(p.Name, filter) {
				filtered = append(filtered, p)
			}
		}
		plugins = filtered
	}

	// Display table
	fmt.Printf("\nPLUGINS (%d registered)\n\n", len(plugins))
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tADDRESS")
	for _, p := range plugins {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Version, p.Status, p.Address)
	}
	w.Flush()

	return nil
}
