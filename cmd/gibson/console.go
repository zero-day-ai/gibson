package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/agent"
	"golang.org/x/term"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Start interactive console mode",
	Long: `Start an interactive console for chatting with agents.

The console supports:
  - @agent mentions to switch between agents
  - /help - show available commands
  - /quit or /exit - exit console
  - /agents - list available agents
  - /target NAME - set current target
  - /clear - clear screen
  - Ctrl+C - cancel current operation (not exit)`,
	RunE: runConsole,
}

// Console flags
var (
	consoleAgentName   string
	consoleTargetName  string
)

func init() {
	consoleCmd.Flags().StringVar(&consoleAgentName, "agent", "", "Initial agent to chat with")
	consoleCmd.Flags().StringVar(&consoleTargetName, "target", "", "Target to use for agent operations")
}

// consoleState holds the state of the interactive console
type consoleState struct {
	ctx           context.Context
	cancel        context.CancelFunc
	activeAgent   string
	targetName    string
	registry      *agent.DefaultAgentRegistry
	history       []string
	historyIdx    int
	isTerminal    bool
	originalState *term.State
}

// runConsole executes the console command
func runConsole(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Create cancellable context for operations
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Initialize console state
	state := &consoleState{
		ctx:        ctx,
		cancel:     cancel,
		activeAgent: consoleAgentName,
		targetName: consoleTargetName,
		registry:   agent.NewAgentRegistry(),
		history:    []string{},
		historyIdx: 0,
	}

	// Check if we're in a terminal
	state.isTerminal = term.IsTerminal(int(os.Stdin.Fd()))

	// Setup signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Handle signals in a goroutine
	go func() {
		for {
			select {
			case <-sigChan:
				// On Ctrl+C, cancel current operation but don't exit
				fmt.Println("\n[Operation cancelled. Use /quit to exit]")
				cancel()
				// Create new context for next operation
				ctx, cancel = context.WithCancel(cmd.Context())
				state.ctx = ctx
				state.cancel = cancel
			case <-ctx.Done():
				return
			}
		}
	}()

	// Display welcome message
	printWelcome(cmd)

	// If agent specified, verify it exists
	if state.activeAgent != "" {
		_, err := state.registry.GetDescriptor(state.activeAgent)
		if err != nil {
			cmd.PrintErrf("Warning: agent '%s' not found. Use /agents to list available agents.\n", state.activeAgent)
			state.activeAgent = ""
		} else {
			cmd.Printf("Active agent: %s\n", state.activeAgent)
		}
	}

	// Main console loop
	return state.run(cmd)
}

// run is the main console loop
func (s *consoleState) run(cmd *cobra.Command) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		// Show prompt
		prompt := s.getPrompt()
		fmt.Print(prompt)

		// Read input
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nGoodbye!")
				return nil
			}
			return fmt.Errorf("failed to read input: %w", err)
		}

		// Trim whitespace
		input = strings.TrimSpace(input)

		// Skip empty lines
		if input == "" {
			continue
		}

		// Add to history
		s.history = append(s.history, input)
		s.historyIdx = len(s.history)

		// Handle slash commands
		if strings.HasPrefix(input, "/") {
			shouldExit := s.handleSlashCommand(cmd, input)
			if shouldExit {
				return nil
			}
			continue
		}

		// Handle @agent mentions
		if strings.HasPrefix(input, "@") {
			s.handleAgentMention(cmd, input)
			continue
		}

		// Send message to active agent
		if err := s.sendMessageToAgent(cmd, input); err != nil {
			cmd.PrintErrf("Error: %v\n", err)
		}
	}
}

// getPrompt returns the current prompt string
func (s *consoleState) getPrompt() string {
	if s.activeAgent != "" {
		return fmt.Sprintf("gibson:%s> ", s.activeAgent)
	}
	return "gibson> "
}

// handleSlashCommand processes slash commands
// Returns true if the console should exit
func (s *consoleState) handleSlashCommand(cmd *cobra.Command, input string) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "/quit", "/exit":
		fmt.Println("Goodbye!")
		return true

	case "/help":
		printHelp(cmd)

	case "/agents":
		s.listAgents(cmd)

	case "/target":
		if len(parts) < 2 {
			cmd.PrintErrf("Usage: /target <name>\n")
		} else {
			s.targetName = parts[1]
			cmd.Printf("Target set to: %s\n", s.targetName)
		}

	case "/clear":
		clearScreen()

	default:
		cmd.PrintErrf("Unknown command: %s (type /help for available commands)\n", command)
	}

	return false
}

// handleAgentMention processes @agent mentions for switching agents
func (s *consoleState) handleAgentMention(cmd *cobra.Command, input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	// Extract agent name (remove @ prefix)
	agentName := strings.TrimPrefix(parts[0], "@")

	// Verify agent exists
	_, err := s.registry.GetDescriptor(agentName)
	if err != nil {
		cmd.PrintErrf("Agent '%s' not found. Use /agents to list available agents.\n", agentName)
		return
	}

	// Switch to agent
	s.activeAgent = agentName
	cmd.Printf("Switched to agent: %s\n", agentName)

	// If there's a message after the mention, send it
	if len(parts) > 1 {
		message := strings.Join(parts[1:], " ")
		if err := s.sendMessageToAgent(cmd, message); err != nil {
			cmd.PrintErrf("Error: %v\n", err)
		}
	}
}

// sendMessageToAgent sends a message to the currently active agent
func (s *consoleState) sendMessageToAgent(cmd *cobra.Command, message string) error {
	if s.activeAgent == "" {
		return fmt.Errorf("no active agent. Use @agent to select an agent or use --agent flag")
	}

	// Create a task from the message
	task := agent.NewTask(
		"console-message",
		message,
		map[string]any{
			"message": message,
		},
	)

	// Add target if set
	if s.targetName != "" {
		// In a real implementation, we would look up the target ID
		// For now, just add it to the input
		task.Input["target"] = s.targetName
	}

	// Execute task with agent
	cmd.Printf("\n[Sending to %s...]\n", s.activeAgent)

	result, err := s.registry.DelegateToAgent(s.ctx, s.activeAgent, task, nil)
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	// Display response
	s.displayAgentResponse(cmd, result)

	return nil
}

// displayAgentResponse formats and displays the agent's response
func (s *consoleState) displayAgentResponse(cmd *cobra.Command, result agent.Result) {
	fmt.Println()

	// Display status
	switch result.Status {
	case agent.ResultStatusCompleted:
		cmd.Printf("[%s] Completed\n", s.activeAgent)
	case agent.ResultStatusFailed:
		cmd.Printf("[%s] Failed\n", s.activeAgent)
		if result.Error != nil {
			cmd.Printf("Error: %s\n", result.Error.Message)
		}
	case agent.ResultStatusCancelled:
		cmd.Printf("[%s] Cancelled\n", s.activeAgent)
	}

	// Display output
	if len(result.Output) > 0 {
		if response, ok := result.Output["response"].(string); ok {
			cmd.Printf("\n%s\n", response)
		} else {
			// Print all output as JSON-like format
			for k, v := range result.Output {
				cmd.Printf("%s: %v\n", k, v)
			}
		}
	}

	// Display findings if any
	if len(result.Findings) > 0 {
		cmd.Printf("\nFindings (%d):\n", len(result.Findings))
		for i, finding := range result.Findings {
			cmd.Printf("  %d. [%s] %s\n", i+1, finding.Severity, finding.Title)
		}
	}

	// Display metrics
	if result.Metrics.Duration > 0 {
		cmd.Printf("\nDuration: %v\n", result.Metrics.Duration)
	}

	fmt.Println()
}

// listAgents displays all available agents
func (s *consoleState) listAgents(cmd *cobra.Command) {
	agents := s.registry.List()

	if len(agents) == 0 {
		cmd.Println("No agents available. Install agents using 'gibson agent install'")
		return
	}

	cmd.Println("\nAvailable agents:")
	for _, agent := range agents {
		active := ""
		if agent.Name == s.activeAgent {
			active = " (active)"
		}
		cmd.Printf("  @%s - %s%s\n", agent.Name, agent.Description, active)
	}
	fmt.Println()
}

// printWelcome displays the welcome message
func printWelcome(cmd *cobra.Command) {
	cmd.Println("Gibson Interactive Console")
	cmd.Println("Type /help for available commands")
	cmd.Println()
}

// printHelp displays help information
func printHelp(cmd *cobra.Command) {
	cmd.Println("\nAvailable commands:")
	cmd.Println("  /help           - Show this help message")
	cmd.Println("  /quit, /exit    - Exit the console")
	cmd.Println("  /agents         - List available agents")
	cmd.Println("  /target <name>  - Set current target")
	cmd.Println("  /clear          - Clear the screen")
	cmd.Println()
	cmd.Println("Agent selection:")
	cmd.Println("  @agent          - Switch to an agent")
	cmd.Println("  @agent message  - Switch to an agent and send a message")
	cmd.Println()
	cmd.Println("Special keys:")
	cmd.Println("  Ctrl+C          - Cancel current operation")
	cmd.Println()
}

// clearScreen clears the terminal screen
func clearScreen() {
	// ANSI escape code to clear screen
	fmt.Print("\033[H\033[2J")
}
