package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines every key in the dashboard. Keep this exhaustive and
// editable — the help modal renders directly from this struct.
type KeyMap struct {
	Quit       key.Binding
	NextPanel  key.Binding
	PrevPanel  key.Binding
	Backup     key.Binding
	Restore    key.Binding
	Sync       key.Binding
	NewProfile key.Binding
	Delete     key.Binding
	Help       key.Binding
	Refresh    key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		NextPanel:  key.NewBinding(key.WithKeys("tab", "l", "right"), key.WithHelp("tab/l/→", "next panel")),
		PrevPanel:  key.NewBinding(key.WithKeys("shift+tab", "h", "left"), key.WithHelp("shift-tab/h/←", "prev panel")),
		Backup:     key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backup")),
		Restore:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restore")),
		Sync:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		NewProfile: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new profile")),
		Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Refresh:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
	}
}
