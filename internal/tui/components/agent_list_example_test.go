package components_test

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/tui/components"
)

// Example demonstrates creating and rendering an AgentListSidebar
func Example_agentListSidebar() {
	sidebar := components.NewAgentListSidebar()
	sidebar.SetSize(40, 12)

	// Add some example agents
	agents := []components.AgentListItem{
		{
			Name:           "recon-agent",
			Status:         database.AgentStatusRunning,
			Mode:           database.AgentModeAutonomous,
			CurrentAction:  "Scanning network",
			NeedsAttention: false,
		},
		{
			Name:           "exploit-agent",
			Status:         database.AgentStatusPaused,
			Mode:           database.AgentModeInteractive,
			CurrentAction:  "Waiting for approval",
			NeedsAttention: false,
		},
		{
			Name:           "pivot-agent",
			Status:         database.AgentStatusWaitingInput,
			Mode:           database.AgentModeInteractive,
			CurrentAction:  "Needs credentials",
			NeedsAttention: true,
		},
		{
			Name:           "exfil-agent",
			Status:         database.AgentStatusCompleted,
			Mode:           database.AgentModeAutonomous,
			CurrentAction:  "Task complete",
			NeedsAttention: false,
		},
	}

	sidebar.SetAgents(agents)

	// Select the second agent
	sidebar.SelectNext()

	// Render the sidebar
	view := sidebar.View()
	fmt.Println(view)

	// Get summary
	summary := sidebar.GetSummary()
	fmt.Printf("\nSummary: %s\n", summary)

	// Get agents needing attention
	needsAttention := sidebar.GetNeedingAttention()
	fmt.Printf("Agents needing attention: %d\n", len(needsAttention))
}

// Example_agentListSidebar_empty demonstrates rendering an empty sidebar
func Example_agentListSidebar_empty() {
	sidebar := components.NewAgentListSidebar()
	sidebar.SetSize(30, 5)

	view := sidebar.View()
	fmt.Println(view)
	fmt.Println("\nSummary:", sidebar.GetSummary())
}

// Example_agentListSidebar_navigation demonstrates keyboard navigation
func Example_agentListSidebar_navigation() {
	sidebar := components.NewAgentListSidebar()
	sidebar.SetAgents([]components.AgentListItem{
		{Name: "agent1", Status: database.AgentStatusRunning},
		{Name: "agent2", Status: database.AgentStatusPaused},
		{Name: "agent3", Status: database.AgentStatusCompleted},
	})

	fmt.Printf("Initial selection: %s\n", sidebar.Selected().Name)

	sidebar.SelectNext()
	fmt.Printf("After SelectNext: %s\n", sidebar.Selected().Name)

	sidebar.SelectNext()
	fmt.Printf("After SelectNext: %s\n", sidebar.Selected().Name)

	// Wrap around
	sidebar.SelectNext()
	fmt.Printf("After SelectNext (wrap): %s\n", sidebar.Selected().Name)

	// Go backwards
	sidebar.SelectPrev()
	fmt.Printf("After SelectPrev: %s\n", sidebar.Selected().Name)

	// Select by name
	found := sidebar.SelectByName("agent2")
	fmt.Printf("Selected by name 'agent2': %v, current: %s\n", found, sidebar.Selected().Name)
}

// Example_agentListSidebar_filtering demonstrates filtering capabilities
func Example_agentListSidebar_filtering() {
	sidebar := components.NewAgentListSidebar()
	sidebar.SetAgents([]components.AgentListItem{
		{Name: "agent1", Status: database.AgentStatusRunning, Mode: database.AgentModeAutonomous},
		{Name: "agent2", Status: database.AgentStatusPaused, Mode: database.AgentModeInteractive},
		{Name: "agent3", Status: database.AgentStatusRunning, Mode: database.AgentModeAutonomous},
		{Name: "agent4", Status: database.AgentStatusCompleted, Mode: database.AgentModeInteractive},
		{Name: "agent5", Status: database.AgentStatusRunning, Mode: database.AgentModeAutonomous, NeedsAttention: true},
	})

	fmt.Printf("Total agents: %d\n", sidebar.Count())

	running := sidebar.FilterByStatus(database.AgentStatusRunning)
	fmt.Printf("Running agents: %d\n", len(running))

	autonomous := sidebar.FilterByMode(database.AgentModeAutonomous)
	fmt.Printf("Autonomous agents: %d\n", len(autonomous))

	needsAttention := sidebar.GetNeedingAttention()
	fmt.Printf("Agents needing attention: %d\n", len(needsAttention))

	// Get agent by name
	agent := sidebar.GetAgentByName("agent3")
	if agent != nil {
		fmt.Printf("Found agent: %s, status: %s\n", agent.Name, agent.Status)
	}
}
