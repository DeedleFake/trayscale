package ui

import (
	"testing"

	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

func ipnStatus(state ipn.State, wantRunning bool) *tsutil.IPNStatus {
	return &tsutil.IPNStatus{State: state, Prefs: (&ipn.Prefs{WantRunning: wantRunning}).View()}
}

func TestSwitchHold(t *testing.T) {
	var h switchHold

	if hold, _, _ := h.offer(false, ipnStatus(ipn.Starting, true)); hold {
		t.Fatal("held a connect from offline")
	}

	hold, start, gen := h.offer(true, ipnStatus(ipn.NoState, true))
	if !hold || !start || !h.active() {
		t.Fatal("didn't hold NoState during a switch")
	}
	if hold, start, _ := h.offer(true, ipnStatus(ipn.Starting, true)); !hold || start {
		t.Fatalf("Starting: hold = %v, start = %v, want true, false", hold, start)
	}
	if hold, _, _ := h.offer(true, ipnStatus(ipn.Running, true)); hold || h.active() {
		t.Fatal("held Running")
	}
	if s := h.expire(gen); s != nil {
		t.Fatal("an ended hold expired")
	}

	if hold, _, _ := h.offer(true, ipnStatus(ipn.Stopped, false)); hold {
		t.Fatal("held a deliberate disconnect")
	}
	if hold, _, _ := h.offer(true, ipnStatus(ipn.NoState, false)); hold {
		t.Fatal("held NoState without WantRunning")
	}

	_, _, gen = h.offer(true, ipnStatus(ipn.NoState, true))
	last := ipnStatus(ipn.Starting, true)
	h.offer(true, last)
	if s := h.expire(gen); s != last {
		t.Fatalf("expire = %v, want the latest held status", s)
	}
	if h.active() {
		t.Fatal("still holding after expiring")
	}
}
