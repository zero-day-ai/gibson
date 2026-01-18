package main

import (
	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
	Long:  `Manage Gibson agents - install, update, start, stop, and monitor agent components`,
}

func init() {
	// Register common component commands
	component.RegisterCommands(agentCmd, component.Config{
		Kind:          internalcomp.ComponentKindAgent,
		DisplayName:   "agent",
		DisplayPlural: "agents",
	})
}
