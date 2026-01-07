package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
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
func runAgentLogs(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	agentName := args[0]

	// Get component DAO
	dao, db, err := getComponentDAO()
	if err != nil {
		return fmt.Errorf("failed to get component DAO: %w", err)
	}
	defer internal.CloseWithLog(db, nil, "component database")

	// Get agent
	agent, err := dao.GetByName(ctx, internalcomp.ComponentKindAgent, agentName)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent '%s' not found", agentName)
	}

	// Construct log file path (assuming logs are written to ~/.gibson/logs/<agent-name>.log)
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	logPath := fmt.Sprintf("%s/logs/%s.log", homeDir, agentName)

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no logs found for agent '%s'", agentName)
	}

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer internal.CloseWithLog(file, nil, "agent log file")

	if logsFollow {
		// Follow mode: tail -f style
		cmd.Printf("Following logs for agent '%s' (Ctrl+C to stop)...\n\n", agentName)

		// Read existing lines first
		scanner := bufio.NewScanner(file)
		lineCount := 0
		lines := make([]string, 0, logsLines)

		// Read all lines
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			lineCount++
		}

		// Print last N lines
		start := 0
		if len(lines) > logsLines {
			start = len(lines) - logsLines
		}
		for i := start; i < len(lines); i++ {
			cmd.Println(lines[i])
		}

		// Now follow for new lines
		// In a real implementation, this would use inotify or similar
		// For now, we'll just poll the file
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-cmd.Context().Done():
				return nil
			case <-ticker.C:
				for scanner.Scan() {
					cmd.Println(scanner.Text())
				}
			}
		}
	} else {
		// Regular mode: show last N lines
		scanner := bufio.NewScanner(file)
		lines := make([]string, 0, logsLines)

		// Read all lines
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		// Print last N lines
		start := 0
		if len(lines) > logsLines {
			start = len(lines) - logsLines
		}

		for i := start; i < len(lines); i++ {
			cmd.Println(lines[i])
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("error reading log file: %w", err)
		}
	}

	return nil
}

// Helper function to get component DAO
func getComponentDAO() (database.ComponentDAO, *database.DB, error) {
	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Open database
	dbPath := homeDir + "/gibson.db"
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create DAO
	dao := database.NewComponentDAO(db)
	return dao, db, nil
}
