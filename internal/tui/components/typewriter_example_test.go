package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Example demonstrates basic TypeWriter usage
func ExampleTypeWriter() {
	// Create a new TypeWriter
	tw := NewTypeWriter()

	// Set the text to type
	tw.SetText("Hello, World!")

	// Optionally customize the speed (in milliseconds)
	tw.SetSpeed(100) // 100ms between characters

	// Start the typing animation
	cmd := tw.Start()
	if cmd != nil {
		// In a BubbleTea app, you would return this command
		// from your Init() or Update() function
		_ = cmd
	}

	// In your Update() function, pass messages to the TypeWriter
	tw, cmd = tw.Update(typewriterTickMsg{})

	// Render the current state
	output := tw.View()
	fmt.Println(output)

	// Check if typing is complete
	if tw.Done() {
		fmt.Println("Typing animation complete!")
	}

	// Skip to the end immediately
	tw.Skip()
}

// ExampleTypeWriter_integration shows how to integrate TypeWriter into a BubbleTea model
func ExampleTypeWriter_integration() {
	type model struct {
		typewriter *TypeWriter
	}

	// Init function
	initFn := func() tea.Cmd {
		tw := NewTypeWriter()
		tw.SetText("Initializing system...")
		tw.SetSpeed(50) // Fast typing
		return tw.Start()
	}

	// Update function
	updateFn := func(m model, msg tea.Msg) (model, tea.Cmd) {
		switch msg.(type) {
		case typewriterTickMsg, cursorBlinkMsg:
			updatedTW, cmd := m.typewriter.Update(msg)
			m.typewriter = updatedTW
			return m, cmd
		}
		return m, nil
	}

	// View function
	viewFn := func(m model) string {
		return m.typewriter.View()
	}

	// These would be used in your BubbleTea program
	_ = initFn
	_ = updateFn
	_ = viewFn
}
