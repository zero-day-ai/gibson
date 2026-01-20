package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
	dclient "github.com/zero-day-ai/gibson/internal/daemon/client"
	"github.com/zero-day-ai/gibson/internal/mission"
)

var missionValidateCmd = &cobra.Command{
	Use:   "validate -f WORKFLOW_FILE",
	Short: "Validate mission dependencies",
	Long: `Validate that all dependencies required by a mission workflow are installed, running, and healthy.

This command loads a workflow YAML file, resolves all component dependencies (agents, tools, plugins),
and checks that each component is:
  - Installed in the Gibson component registry
  - Currently running
  - Passing health checks
  - Satisfying version constraints (if specified)

The validation process:
  1. Parses the workflow YAML file
  2. Extracts direct dependencies (from nodes and dependencies section)
  3. Recursively resolves transitive dependencies (from component manifests)
  4. Queries the daemon to check component state
  5. Reports any issues (missing, stopped, unhealthy, or version mismatches)

Exit codes:
  0 - All dependencies satisfied (workflow is ready to run)
  1 - Validation failed (missing or unhealthy components)
  2 - Invalid arguments or file not found

Examples:
  # Validate a workflow file
  gibson mission validate -f workflow.yaml

  # Validate with JSON output
  gibson mission validate -f workflow.yaml --output json

  # Validate with YAML output
  gibson mission validate -f workflow.yaml --output yaml

  # Auto-install missing components (not yet implemented)
  gibson mission validate -f workflow.yaml --auto-install`,
	RunE: runMissionValidate,
}

// Flags for mission validate command
var (
	missionValidateFile        string
	missionValidateOutput      string
	missionValidateAutoInstall bool
)

func init() {
	// Add validate subcommand to mission command
	missionCmd.AddCommand(missionValidateCmd)

	// Add flags
	missionValidateCmd.Flags().StringVarP(&missionValidateFile, "file", "f", "", "Path to workflow YAML file (required)")
	missionValidateCmd.Flags().StringVar(&missionValidateOutput, "output", "text", "Output format: text, json, yaml")
	missionValidateCmd.Flags().BoolVar(&missionValidateAutoInstall, "auto-install", false, "Attempt to install missing components before validation (not yet implemented)")

	// Mark file flag as required
	_ = missionValidateCmd.MarkFlagRequired("file")
}

// runMissionValidate executes the mission validate command
func runMissionValidate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Parse global flags for verbose logging
	flags, err := ParseGlobalFlags(cmd)
	if err != nil {
		return internal.WrapError(internal.ExitConfigError, "failed to parse flags", err)
	}

	verbose := flags.IsVerbose()

	// Validate output format flag
	switch missionValidateOutput {
	case "text", "json", "yaml":
		// valid
	default:
		return internal.WrapError(internal.ExitConfigError,
			fmt.Sprintf("invalid output format: %s (must be text, json, or yaml)", missionValidateOutput), nil)
	}

	// Check if auto-install flag was used
	if missionValidateAutoInstall {
		return internal.WrapError(internal.ExitConfigError,
			"--auto-install is not yet implemented", nil)
	}

	// Validate that file flag was provided and file exists
	if missionValidateFile == "" {
		return internal.WrapError(internal.ExitConfigError,
			"workflow file is required (use -f or --file flag)", nil)
	}

	// Check if file exists
	if _, err := os.Stat(missionValidateFile); os.IsNotExist(err) {
		return internal.WrapError(internal.ExitConfigError,
			fmt.Sprintf("workflow file not found: %s", missionValidateFile), nil)
	}

	// Validation requires daemon connection
	if verbose {
		fmt.Fprintf(os.Stderr, "Connecting to Gibson daemon...\n")
	}

	client, err := dclient.RequireDaemon(ctx)
	if err != nil {
		return internal.WrapError(internal.ExitError, "mission validation requires daemon", err)
	}
	defer client.Close()

	// Load and parse the workflow file to validate it's a valid mission
	if verbose {
		fmt.Fprintf(os.Stderr, "Loading workflow file: %s\n", missionValidateFile)
	}

	// Use mission.ParseDefinition to validate YAML format
	_, err = mission.ParseDefinition(missionValidateFile)
	if err != nil {
		return internal.WrapError(internal.ExitError,
			fmt.Sprintf("failed to parse workflow file: %v", err), err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Resolving dependencies...\n")
	}

	// Call daemon to validate mission dependencies
	result, err := client.ValidateMissionDependencies(ctx, missionValidateFile)
	if err != nil {
		return internal.WrapError(internal.ExitError,
			"failed to validate mission dependencies", err)
	}

	// Format and display the validation result
	if err := formatValidationResult(cmd, result, verbose); err != nil {
		return err
	}

	// Return non-zero exit code if validation failed
	if !result.Valid {
		return internal.WrapError(internal.ExitError, "validation failed", nil)
	}

	return nil
}

// formatValidationResult formats and prints the validation result based on output format
func formatValidationResult(cmd *cobra.Command, result *dclient.ValidationResult, verbose bool) error {
	switch missionValidateOutput {
	case "json":
		return formatValidationJSON(cmd, result)
	case "yaml":
		return formatValidationYAML(cmd, result)
	default: // "text"
		return formatValidationText(cmd, result, verbose)
	}
}

// formatValidationJSON outputs validation result as JSON
func formatValidationJSON(cmd *cobra.Command, result *dclient.ValidationResult) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// formatValidationYAML outputs validation result as YAML
func formatValidationYAML(cmd *cobra.Command, result *dclient.ValidationResult) error {
	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	defer encoder.Close()
	return encoder.Encode(result)
}

// formatValidationText outputs validation result as human-readable text
func formatValidationText(cmd *cobra.Command, result *dclient.ValidationResult, verbose bool) error {
	out := cmd.OutOrStdout()

	// Print header
	fmt.Fprintln(out, "Mission Dependency Validation")
	fmt.Fprintln(out, "=============================")
	fmt.Fprintln(out)

	// Print summary
	if result.Valid {
		fmt.Fprintf(out, "✓ %s\n", result.Summary)
	} else {
		fmt.Fprintf(out, "✗ %s\n", result.Summary)
	}
	fmt.Fprintln(out)

	// Print statistics
	fmt.Fprintln(out, "Component Statistics:")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Total:\t%d\n", result.TotalComponents)
	fmt.Fprintf(tw, "  Installed:\t%d\n", result.InstalledCount)
	fmt.Fprintf(tw, "  Running:\t%d\n", result.RunningCount)
	fmt.Fprintf(tw, "  Healthy:\t%d\n", result.HealthyCount)
	tw.Flush()
	fmt.Fprintln(out)

	// Print validation time
	fmt.Fprintf(out, "Validated in: %v\n", result.Duration)
	fmt.Fprintln(out)

	// If validation passed and not verbose, we're done
	if result.Valid && !verbose {
		return nil
	}

	// Print detailed issues if validation failed
	hasIssues := false

	if len(result.NotInstalled) > 0 {
		hasIssues = true
		fmt.Fprintln(out, "Components Not Installed:")
		fmt.Fprintln(out, "-------------------------")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tNAME\tREQUIRED VERSION\tSOURCE")
		for _, node := range result.NotInstalled {
			version := node.Version
			if version == "" {
				version = "any"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				node.Kind, node.Name, version, formatSource(node.Source, node.SourceRef))
		}
		tw.Flush()
		fmt.Fprintln(out)

		// Print installation hints
		fmt.Fprintln(out, "To install missing components:")
		for _, node := range result.NotInstalled {
			fmt.Fprintf(out, "  gibson %s install <url>\n", node.Kind)
		}
		fmt.Fprintln(out)
	}

	if len(result.NotRunning) > 0 {
		hasIssues = true
		fmt.Fprintln(out, "Components Not Running:")
		fmt.Fprintln(out, "----------------------")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tNAME\tVERSION\tSOURCE")
		for _, node := range result.NotRunning {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				node.Kind, node.Name, node.ActualVersion, formatSource(node.Source, node.SourceRef))
		}
		tw.Flush()
		fmt.Fprintln(out)

		// Print start hints
		fmt.Fprintln(out, "To start stopped components:")
		for _, node := range result.NotRunning {
			fmt.Fprintf(out, "  gibson %s start %s\n", node.Kind, node.Name)
		}
		fmt.Fprintln(out)
	}

	if len(result.Unhealthy) > 0 {
		hasIssues = true
		fmt.Fprintln(out, "Components Unhealthy:")
		fmt.Fprintln(out, "--------------------")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tNAME\tVERSION\tSOURCE")
		for _, node := range result.Unhealthy {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				node.Kind, node.Name, node.ActualVersion, formatSource(node.Source, node.SourceRef))
		}
		tw.Flush()
		fmt.Fprintln(out)

		// Print health check hints
		fmt.Fprintln(out, "To check component health:")
		for _, node := range result.Unhealthy {
			fmt.Fprintf(out, "  gibson %s status %s\n", node.Kind, node.Name)
			fmt.Fprintf(out, "  gibson %s logs %s\n", node.Kind, node.Name)
		}
		fmt.Fprintln(out)
	}

	if len(result.VersionMismatch) > 0 {
		hasIssues = true
		fmt.Fprintln(out, "Version Mismatches:")
		fmt.Fprintln(out, "------------------")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tNAME\tREQUIRED\tINSTALLED\tSOURCE")
		for _, mismatch := range result.VersionMismatch {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				mismatch.Node.Kind,
				mismatch.Node.Name,
				mismatch.RequiredVersion,
				mismatch.ActualVersion,
				formatSource(mismatch.Node.Source, mismatch.Node.SourceRef))
		}
		tw.Flush()
		fmt.Fprintln(out)

		// Print update hints
		fmt.Fprintln(out, "To update components:")
		for _, mismatch := range result.VersionMismatch {
			fmt.Fprintf(out, "  gibson %s update %s\n", mismatch.Node.Kind, mismatch.Node.Name)
		}
		fmt.Fprintln(out)
	}

	// If verbose mode and validation passed, show all components
	if verbose && result.Valid && !hasIssues {
		fmt.Fprintln(out, "All Components:")
		fmt.Fprintln(out, "---------------")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "KIND\tNAME\tVERSION\tSTATUS\tSOURCE")

		// Collect all nodes from the result (would need to add this to the result struct)
		// For now, we can infer from the installed/running/healthy nodes
		// This is a simplification - ideally ValidationResult would include all nodes
		fmt.Fprintln(tw, "(Detailed component list available with JSON/YAML output)")
		tw.Flush()
		fmt.Fprintln(out)
	}

	return nil
}

// formatSource formats the dependency source and reference for display
func formatSource(source, sourceRef string) string {
	switch source {
	case "mission_explicit":
		return "dependencies"
	case "mission_node":
		return fmt.Sprintf("node:%s", sourceRef)
	case "manifest":
		return fmt.Sprintf("dep:%s", sourceRef)
	default:
		return source
	}
}
