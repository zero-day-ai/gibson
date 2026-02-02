package main

import (
	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/cmd/gibson/component"
	internalcomp "github.com/zero-day-ai/gibson/internal/component"
)

var toolCmd = &cobra.Command{
	Use:   "tool",
	Short: "Manage Gibson tools",
	Long:  `Install, manage, and invoke tools for agent capabilities`,
}


func init() {
	// Register common component commands but exclude start/stop
	// Tools are now executed via Redis-based tool registry through the harness,
	// they don't need individual start/stop lifecycle management.
	commands := component.NewCommands(component.Config{
		Kind:          internalcomp.ComponentKindTool,
		DisplayName:   "tool",
		DisplayPlural: "tools",
	})

	// Register all commands EXCEPT start and stop
	// Tools are executed via Redis-based tool registry, not individual gRPC servers
	toolCmd.AddCommand(commands.List)
	toolCmd.AddCommand(commands.Install)
	toolCmd.AddCommand(commands.InstallAll)
	toolCmd.AddCommand(commands.Uninstall)
	toolCmd.AddCommand(commands.Update)
	toolCmd.AddCommand(commands.Show)
	toolCmd.AddCommand(commands.Build)
	// commands.Start - REMOVED: tools are executed via Redis-based tool registry
	// commands.Stop  - REMOVED: tools are executed via Redis-based tool registry
	toolCmd.AddCommand(commands.Status)
	toolCmd.AddCommand(commands.Logs)
}
