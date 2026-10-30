package ui

import (
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

// switchHold holds back the NoState and Starting statuses that
// tailscaled reports while it switches profiles, so that the switch
// doesn't look like a disconnect.
type switchHold struct {
	held *tsutil.IPNStatus // The latest held status, nil when not holding.
	gen  int
}

// offer reports whether to hold s back instead of showing it, given
// whether the shown status is online. If s starts a hold, start is
// true and gen identifies the hold for expire.
func (h *switchHold) offer(online bool, s *tsutil.IPNStatus) (hold, start bool, gen int) {
	if online && switching(s) {
		start = h.held == nil
		if start {
			h.gen++
		}
		h.held = s
		return true, start, h.gen
	}

	h.held = nil
	h.gen++
	return false, false, h.gen
}

// active reports whether a hold is in progress.
func (h *switchHold) active() bool {
	return h.held != nil
}

// expire ends the hold identified by gen, returning its latest status
// to show, or nil if that hold has already ended.
func (h *switchHold) expire(gen int) *tsutil.IPNStatus {
	s := h.held
	if s == nil || gen != h.gen {
		return nil
	}
	h.held = nil
	return s
}

func switching(s *tsutil.IPNStatus) bool {
	return (s.State == ipn.NoState || s.State == ipn.Starting) && s.Prefs.WantRunning()
}
