package audit

import "context"

// Multi fans one operation's Begin/End out to several Auditors, so the same
// destructive-op seam feeds both the security audit log and the telemetry
// recorder without the app layer wiring two hooks. Nil entries are skipped, and
// Multi of zero live auditors is itself a no-op (returns nil from New).
type Multi struct {
	auditors []Auditor
}

// NewMulti returns an Auditor that broadcasts to all non-nil auditors, or nil if
// none are live (so callers keep the nil-is-no-op contract).
func NewMulti(auditors ...Auditor) Auditor {
	live := auditors[:0]
	for _, a := range auditors {
		if a != nil {
			live = append(live, a)
		}
	}
	if len(live) == 0 {
		return nil
	}
	return &Multi{auditors: live}
}

func (m *Multi) Begin(ctx context.Context, ev Event) Handle {
	hs := make([]Handle, 0, len(m.auditors))
	for _, a := range m.auditors {
		if h := a.Begin(ctx, ev); h != nil {
			hs = append(hs, h)
		}
	}
	return &multiHandle{handles: hs}
}

type multiHandle struct{ handles []Handle }

func (h *multiHandle) End(err error) {
	for _, inner := range h.handles {
		inner.End(err)
	}
}
