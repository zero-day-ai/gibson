package console

import (
	"context"
	"time"
)

// OutputStyle represents the visual styling to apply to console output.
type OutputStyle int

const (
	// StyleNormal is the default output style with no special formatting.
	StyleNormal OutputStyle = iota
	// StyleError indicates error messages or failure states.
	StyleError
	// StyleSuccess indicates successful operations or positive states.
	StyleSuccess
	// StyleInfo indicates informational messages or system notifications.
	StyleInfo
	// StyleCommand indicates command input or command-related output.
	StyleCommand
	// StyleAgentOutput indicates agent reasoning or output text.
	StyleAgentOutput
	// StyleToolCall indicates a tool invocation by an agent.
	StyleToolCall
	// StyleToolResult indicates the response from a tool execution.
	StyleToolResult
	// StyleFinding indicates a security finding or vulnerability discovered.
	StyleFinding
	// StyleFindingCritical indicates a critical severity finding.
	StyleFindingCritical
	// StyleFindingHigh indicates a high severity finding.
	StyleFindingHigh
	// StyleFindingMedium indicates a medium severity finding.
	StyleFindingMedium
	// StyleFindingLow indicates a low severity finding.
	StyleFindingLow
	// StyleStatus indicates an agent status change or state transition.
	StyleStatus
	// StyleSteeringAck indicates acknowledgment of user steering input.
	StyleSteeringAck
	// StyleUserSteering indicates a user's steering message.
	StyleUserSteering
)

// String returns the string representation of an OutputStyle.
func (s OutputStyle) String() string {
	switch s {
	case StyleNormal:
		return "normal"
	case StyleError:
		return "error"
	case StyleSuccess:
		return "success"
	case StyleInfo:
		return "info"
	case StyleCommand:
		return "command"
	case StyleAgentOutput:
		return "agent_output"
	case StyleToolCall:
		return "tool_call"
	case StyleToolResult:
		return "tool_result"
	case StyleFinding:
		return "finding"
	case StyleFindingCritical:
		return "finding_critical"
	case StyleFindingHigh:
		return "finding_high"
	case StyleFindingMedium:
		return "finding_medium"
	case StyleFindingLow:
		return "finding_low"
	case StyleStatus:
		return "status"
	case StyleSteeringAck:
		return "steering_ack"
	case StyleUserSteering:
		return "user_steering"
	default:
		return "unknown"
	}
}

// OutputSource represents the origin of a console output line.
type OutputSource int

const (
	// SourceCommand indicates output from a slash command execution.
	SourceCommand OutputSource = iota
	// SourceAgent indicates output from an agent stream event.
	SourceAgent
	// SourceSystem indicates a system message or notification.
	SourceSystem
	// SourceUser indicates a user steering message.
	SourceUser
)

// String returns the string representation of an OutputSource.
func (s OutputSource) String() string {
	switch s {
	case SourceCommand:
		return "command"
	case SourceAgent:
		return "agent"
	case SourceSystem:
		return "system"
	case SourceUser:
		return "user"
	default:
		return "unknown"
	}
}

// OutputLine represents a single line of console output with styling and metadata.
type OutputLine struct {
	// Text is the content of the output line.
	Text string
	// Style determines how the line should be visually rendered.
	Style OutputStyle
	// Timestamp records when this line was created.
	Timestamp time.Time
	// Source indicates the origin of this output line.
	Source OutputSource
	// AgentName identifies which agent produced this output (if applicable).
	AgentName string
}

// ConsoleOutput represents a collection of output lines with pagination support.
type ConsoleOutput struct {
	// Lines contains the output lines to display.
	Lines []OutputLine
	// TotalLines tracks the total number of lines across all pages/history.
	TotalLines int
}

// ExecutionResult captures the complete result of a command execution.
type ExecutionResult struct {
	// Output contains the standard output (stdout) from the command.
	Output string
	// Error contains the standard error (stderr) from the command.
	Error string
	// IsError indicates whether the command execution resulted in an error.
	IsError bool
	// Duration records how long the command took to execute.
	Duration time.Duration
	// ExitCode contains the exit status code from the command.
	ExitCode int
}

// ParsedCommand represents a slash command that has been parsed into its components.
type ParsedCommand struct {
	// Name is the command name without the leading slash.
	Name string
	// Subcommand is an optional sub-command (e.g., "create" in "/resource create").
	Subcommand string
	// Args contains the positional arguments passed to the command.
	Args []string
	// Flags contains named flags/options as key-value pairs.
	Flags map[string]string
}

// CommandHandler is a function that executes a slash command and returns its result.
// The handler receives a context for cancellation/timeouts and command arguments.
type CommandHandler func(ctx context.Context, args []string) (*ExecutionResult, error)

// SlashCommand defines a console slash command with its metadata and execution handler.
type SlashCommand struct {
	// Name is the primary name of the command (without the leading slash).
	Name string
	// Aliases contains alternative names that can be used to invoke this command.
	Aliases []string
	// Description provides a brief explanation of what the command does.
	Description string
	// Usage shows the command syntax and available options.
	Usage string
	// Handler is the function that executes when this command is invoked.
	Handler CommandHandler
	// Subcommands contains nested commands organized by name.
	Subcommands map[string]*SlashCommand
}
