package main

import (
	"github.com/spf13/cobra"
)

// OutputFormat represents the output format for CLI commands
type OutputFormat string

const (
	// FormatText is human-readable text output
	FormatText OutputFormat = "text"
	// FormatJSON is structured JSON output
	FormatJSON OutputFormat = "json"
)

// GlobalFlags holds global flags available to all commands
type GlobalFlags struct {
	Verbose      bool
	Quiet        bool
	OutputFormat string
	ConfigFile   string
	HomeDir      string
}

var globalFlags = &GlobalFlags{}

// RegisterGlobalFlags registers persistent flags on the root command
func RegisterGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&globalFlags.Verbose, "verbose", "v", false, "Enable verbose output")
	cmd.PersistentFlags().BoolVarP(&globalFlags.Quiet, "quiet", "q", false, "Suppress non-essential output")
	cmd.PersistentFlags().StringVarP(&globalFlags.OutputFormat, "output", "o", "text", "Output format (text|json)")
	cmd.PersistentFlags().StringVar(&globalFlags.ConfigFile, "config", "", "Path to config file (default: $GIBSON_HOME/config.yaml)")
	cmd.PersistentFlags().StringVar(&globalFlags.HomeDir, "home", "", "Gibson home directory (default: ~/.gibson)")
}

// ParseGlobalFlags parses and validates global flags from the command
func ParseGlobalFlags(cmd *cobra.Command) (*GlobalFlags, error) {
	// Validate output format
	format := globalFlags.OutputFormat
	if format != string(FormatText) && format != string(FormatJSON) {
		return nil, cmd.Help()
	}

	// Validate that verbose and quiet are not both set
	if globalFlags.Verbose && globalFlags.Quiet {
		cmd.PrintErrln("Error: --verbose and --quiet cannot be used together")
		return nil, cmd.Help()
	}

	return globalFlags, nil
}

// GetOutputFormat returns the parsed OutputFormat enum
func (f *GlobalFlags) GetOutputFormat() OutputFormat {
	if f.OutputFormat == string(FormatJSON) {
		return FormatJSON
	}
	return FormatText
}

// IsVerbose returns true if verbose mode is enabled
func (f *GlobalFlags) IsVerbose() bool {
	return f.Verbose && !f.Quiet
}

// IsQuiet returns true if quiet mode is enabled
func (f *GlobalFlags) IsQuiet() bool {
	return f.Quiet
}
