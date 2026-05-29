package panels

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nixrajput/siphon/internal/jobs"
	"github.com/nixrajput/siphon/internal/tui/styles"
)

type jobView struct {
	id       string
	stage    string
	phase    jobs.Phase
	message  string
	progress progress.Model
	bytes    int64
	total    int64
}

// Jobs is a panel that displays running job progress bars via a live event
// subscription pump.
type Jobs struct {
	jobs    map[string]*jobView
	focused bool
}

// NewJobs creates an empty Jobs panel.
func NewJobs() Jobs {
	return Jobs{jobs: map[string]*jobView{}}
}

func (p Jobs) Init() tea.Cmd { return nil }

// SubscribeCmd returns a tea.Cmd that reads one Event from ch and emits a
// JobEventInternal back into Update. The caller re-issues SubscribeCmd after
// each delivery to keep the pump running until ch closes.
func SubscribeCmd(ch <-chan jobs.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return jobsChannelClosedMsg{}
		}
		return JobEventInternal{Event: e, Ch: ch}
	}
}

// JobEventInternal is the message emitted by SubscribeCmd. Panel-internal pump
// message — distinct from tui.JobEventMsg / tui.JobChannelMsg in msg.go.
type JobEventInternal struct {
	Event jobs.Event
	Ch    <-chan jobs.Event
}

type jobsChannelClosedMsg struct{}

func (p Jobs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case JobEventInternal:
		p.absorb(msg.Event)
		// Re-issue the subscription to pump the next event.
		return p, SubscribeCmd(msg.Ch)
	case jobsChannelClosedMsg:
		// Channel closed — most recent state stays visible.
		return p, nil
	}
	return p, nil
}

func (p *Jobs) absorb(e jobs.Event) {
	view, ok := p.jobs[e.JobID]
	if !ok {
		view = &jobView{
			id:       e.JobID,
			stage:    e.Stage,
			progress: progress.New(progress.WithDefaultGradient()),
		}
		p.jobs[e.JobID] = view
	}
	view.phase = e.Phase
	view.message = e.Message
	if e.Progress != nil {
		view.bytes = e.Progress.BytesDone
		view.total = e.Progress.BytesTotal
	}
}

func (p Jobs) View() string {
	border := styles.Border
	if p.focused {
		border = styles.BorderFocus
	}
	if len(p.jobs) == 0 {
		return border.Render(styles.PanelTitle.Render("Jobs") + "\n" + styles.Dim.Render("(idle)"))
	}
	body := styles.PanelTitle.Render("Jobs") + "\n"
	for _, j := range p.jobs {
		bar := ""
		if j.total > 0 {
			bar = j.progress.ViewAs(float64(j.bytes) / float64(j.total))
		}
		body += fmt.Sprintf("%-6s  %s %s\n%s\n", j.stage, j.phase.String(), j.message, bar)
	}
	return border.Render(body)
}

// Title returns the panel's display name.
func (p Jobs) Title() string { return "Jobs" }

// SetSize is a no-op — progress bars use their default width in Phase C.
func (p *Jobs) SetSize(w, h int) { /* progress bars are width-aware via Update */ }

// SetFocus toggles keyboard focus on the panel.
func (p *Jobs) SetFocus(b bool) { p.focused = b }

// Compile-time check.
var _ tea.Model = Jobs{}
