package component

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/core"
)

// newLogsCommand creates a logs command for the specified component type.
func newLogsCommand(cfg Config, flags *LogsFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: fmt.Sprintf("View %s logs", cfg.DisplayName),
		Long: fmt.Sprintf(`Display logs for a %s.

Logs are stored in ~/.gibson/logs/%s/<name>.log

Use --follow (-f) to stream logs in real-time.
Use --lines (-n) to specify the number of lines to show.`,
			cfg.DisplayName, cfg.DisplayName),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogs(cmd, args, cfg, flags)
		},
	}

	// Register flags
	cmd.Flags().BoolVarP(&flags.Follow, "follow", "f", false, "Follow log output (like tail -f)")
	cmd.Flags().IntVarP(&flags.Lines, "lines", "n", 50, "Number of lines to show")

	return cmd
}

// runLogs executes the logs command for a component.
func runLogs(cmd *cobra.Command, args []string, cfg Config, flags *LogsFlags) error {
	componentName := args[0]

	// Build command context
	cc, err := buildCommandContext(cmd)
	if err != nil {
		return err
	}
	defer cc.Close()

	// Build logs options
	opts := core.LogsOptions{
		Follow: flags.Follow,
		Lines:  flags.Lines,
	}

	// Call core function
	result, err := core.ComponentLogs(cc, cfg.Kind, componentName, opts)
	if err != nil {
		return err
	}

	// Extract result data
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected result type")
	}

	// Check if this is follow mode
	if follow, _ := data["follow"].(bool); follow {
		logPath, _ := data["log_path"].(string)
		// Open log file for following
		file, err := os.Open(logPath)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		defer file.Close()
		return followLogs(cmd, file, logPath)
	}

	// Display lines
	lines, ok := data["lines"].([]string)
	if !ok {
		return fmt.Errorf("unexpected lines type")
	}

	for _, line := range lines {
		cmd.Println(line)
	}

	return nil
}


// followLogs streams log output in real-time (like tail -f).
func followLogs(cmd *cobra.Command, file *os.File, logPath string) error {
	// Seek to end of file
	_, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	cmd.Printf("Following logs for %s (Ctrl+C to stop)...\n\n", logPath)

	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// No new data, wait and try again
				// In a real implementation, we'd use fsnotify or similar
				continue
			}
			return fmt.Errorf("failed to read log: %w", err)
		}

		// Print the line without adding another newline
		cmd.Print(line)
	}
}
