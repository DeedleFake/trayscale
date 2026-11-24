package ui

import (
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
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

// TestHoldDisablesActions checks that the actions that act on
// Tailscale's state can't be used during a profile switch hold, as the
// state shown then is the old profile's, and that they come back after.
func TestHoldDisablesActions(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "hold-actions"), operatorCheck: true}
	useExitNode := gio.NewSimpleActionStateful("use_exit_node", nil, glib.NewVariantBoolean(false))
	useExitNode.Ref()
	defer useExitNode.Unref()
	a.addAction(useExitNode, nil)
	search := gio.NewSimpleAction("search-peers", nil)
	search.Ref()
	defer search.Unref()
	a.addAction(search, nil)

	check := func(when string, exitNode, searchPeers bool) {
		t.Helper()
		if got := useExitNode.GetEnabled(); got != exitNode {
			t.Errorf("%v: use_exit_node enabled = %v, want %v", when, got, exitNode)
		}
		if got := search.GetEnabled(); got != searchPeers {
			t.Errorf("%v: search-peers enabled = %v, want %v", when, got, searchPeers)
		}
	}

	a.update(ipnStatus(ipn.Stopped, false))
	check("offline", false, true)

	a.update(ipnStatus(ipn.Running, true))
	check("online", true, true)

	a.update(ipnStatus(ipn.NoState, true))
	if !a.hold.active() {
		t.Fatal("NoState while online didn't start a hold")
	}
	check("held", false, false)
	a.update(ipnStatus(ipn.Starting, true))
	check("still held", false, false)

	a.update(ipnStatus(ipn.Running, true))
	check("after the hold", true, true)
}
