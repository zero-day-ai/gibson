package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
	"github.com/zero-day-ai/gibson/internal/database"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
	Long:  `Manage Gibson agents - install, update, start, stop, and monitor agent components`,
}

var agentLogsCmd = &cobra.Command{
	Use:   "logs NAME",
	Short: "View agent logs",
	Long:  `View process output logs for an agent`,
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentLogs,
}

// Flags for agent logs
var (
	logsFollow bool
	logsLines  int
)

func init() {
	// Register common component commands
	component.RegisterCommands(agentCmd, component.Config{
		Kind:          internalcomp.ComponentKindAgent,
		DisplayName:   "agent",
		DisplayPlural: "agents",
	})

	// Logs command flags
	agentLogsCmd.Flags().BoolVar(&logsFollow, "follow", false, "Follow log output")
	agentLogsCmd.Flags().IntVar(&logsLines, "lines", 100, "Number of lines to show")

	// Add logs subcommand
	agentCmd.AddCommand(agentLogsCmd)
}

// runAgentLogs executes the agent logs command
// NOTE: This command is deprecated as component data is now stored in etcd
func runAgentLogs(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	// Component data is now stored in etcd, not SQLite
	// This legacy command is no longer supported
	_, _, err := getComponentDAO()
	if err != nil {
		return fmt.Errorf("agent logs command is no longer supported (use 'gibson component logs %s' instead): %w", agentName, err)
	}

	// Unreachable
	return nil
}

// Helper function to get component DAO
// NOTE: This is deprecated - component data is now stored in etcd, not SQLite
func getComponentDAO() (interface{}, *database.DB, error) {
	return nil, nil, fmt.Errorf("component DAO is no longer available - component data is now stored in etcd")
}
