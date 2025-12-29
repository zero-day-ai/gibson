package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines all keyboard bindings for the TUI.
type KeyMap struct {
	// Global keys
	Quit    key.Binding
	Help    key.Binding
	Command key.Binding
	Tab     key.Binding
	Escape  key.Binding

	// Navigation keys
	ViewDashboard key.Binding
	ViewConsole   key.Binding
	ViewMission   key.Binding
	ViewFindings  key.Binding

	// List navigation
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	PageUp   key.Binding
	PageDown key.Binding

	// Action keys
	Refresh key.Binding
	Run     key.Binding
	Pause   key.Binding
	Stop    key.Binding
	Delete  key.Binding
	Approve key.Binding
	Filter  key.Binding
	Export  key.Binding
}

// DefaultKeyMap returns a KeyMap with default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		// Global keys
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command mode"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "cycle focus"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),

		// Navigation keys
		ViewDashboard: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "dashboard view"),
		),
		ViewConsole: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "console view"),
		),
		ViewMission: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "mission view"),
		),
		ViewFindings: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "findings view"),
		),

		// List navigation
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdn", "page down"),
		),

		// Action keys
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Run: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "run"),
		),
		Pause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause"),
		),
		Stop: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Approve: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "approve"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter"),
		),
		Export: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "export"),
		),
	}
}

// HelpText returns a structured help text for all key bindings.
func (k KeyMap) HelpText() map[string][]key.Binding {
	return map[string][]key.Binding{
		"Global": {
			k.Quit,
			k.Help,
			k.Tab,
			k.Escape,
		},
		"Views": {
			k.ViewDashboard,
			k.ViewConsole,
			k.ViewMission,
			k.ViewFindings,
		},
		"Navigation": {
			k.Up,
			k.Down,
			k.Enter,
			k.PageUp,
			k.PageDown,
		},
		"Actions": {
			k.Refresh,
			k.Run,
			k.Pause,
			k.Stop,
			k.Delete,
			k.Approve,
			k.Filter,
			k.Export,
		},
	}
}
