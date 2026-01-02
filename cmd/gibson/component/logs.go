package component

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
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

	// Get Gibson home directory
	homeDir, err := getGibsonHome()
	if err != nil {
		return fmt.Errorf("failed to get Gibson home: %w", err)
	}

	// Build log path
	logPath := filepath.Join(homeDir, "logs", string(cfg.Kind), fmt.Sprintf("%s.log", componentName))

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no logs found for %s '%s' (expected at %s)", cfg.DisplayName, componentName, logPath)
	}

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	if flags.Follow {
		return followLogs(cmd, file, logPath)
	}

	return tailLogs(cmd, file, flags.Lines)
}

// tailLogs shows the last N lines of the log file.
func tailLogs(cmd *cobra.Command, file *os.File, lines int) error {
	// Read the entire file and get the last N lines
	// For large files, a more efficient implementation would seek from the end
	scanner := bufio.NewScanner(file)
	var allLines []string

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	// Get last N lines
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	for _, line := range allLines[start:] {
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
