package tui

import "github.com/nixrajput/siphon/internal/jobs"

// JobEventMsg carries a single jobs.Event from the runner into the TUI's
// Update cycle. The Jobs panel listens for this; other panels ignore it.
type JobEventMsg struct {
	Event jobs.Event
}

// JobChannelMsg wraps the channel returned by app.Backup/Restore/Sync so
// the Jobs panel can subscribe. Sent by modals when they complete.
type JobChannelMsg struct {
	Channel <-chan jobs.Event
	JobID   string
}

// RefreshMsg asks all panels to reload their data (profile list, dumps).
type RefreshMsg struct{}

// ErrorMsg pops the error modal with the given error and optional hint.
type ErrorMsg struct {
	Err  error
	Hint string
}

// FocusMsg moves focus to the named panel.
type FocusMsg struct{ Panel string }
