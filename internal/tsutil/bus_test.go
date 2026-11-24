package tsutil

import (
	"context"
	"testing"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
	"tailscale.com/types/empty"
)

func TestWatchStateApply(t *testing.T) {
	t.Parallel()

	running, stopped := ipn.Running, ipn.Stopped
	prefs := func(controlURL string) *ipn.PrefsView {
		v := (&ipn.Prefs{ControlURL: controlURL}).View()
		return &v
	}
	engine := &ipn.EngineStatus{}
	selfA := testSelfNode(10, "self-a", 100)
	renamed := testSelfNode(selfA.ID, selfA.StableID, selfA.User)
	renamed.Name = "renamed.tailnet.ts.net."

	for _, tt := range []struct {
		name   string
		before []ipn.Notify
		notify ipn.Notify
		want   change
	}{
		{name: "state", notify: ipn.Notify{State: &running}, want: changeIPN | changeState | changeTargets},
		{name: "same state", before: []ipn.Notify{{State: &running}}, notify: ipn.Notify{State: &running}, want: changeIPN},
		{name: "stopped", before: []ipn.Notify{{State: &running}}, notify: ipn.Notify{State: &stopped}, want: changeIPN | changeState},
		{name: "first prefs", notify: ipn.Notify{Prefs: prefs("a")}, want: changeIPN},
		{name: "same prefs", before: []ipn.Notify{{Prefs: prefs("a")}}, notify: ipn.Notify{Prefs: prefs("a")}, want: changeIPN},
		{name: "control URL", before: []ipn.Notify{{Prefs: prefs("a")}}, notify: ipn.Notify{Prefs: prefs("b")}, want: changeIPN | changeIdentity},
		{name: "login finished", notify: ipn.Notify{LoginFinished: &empty.Message{}}, want: changeIdentity},
		{name: "engine", before: []ipn.Notify{{State: &running}}, notify: ipn.Notify{Engine: engine}, want: 0},
		// Files asks tailscaled for the list instead of following these
		// deprecated fields.
		{name: "files waiting", notify: ipn.Notify{FilesWaiting: &empty.Message{}}, want: 0},                            //nolint:staticcheck
		{name: "transfer done", notify: ipn.Notify{IncomingFiles: []ipn.PartialFile{{Name: "a", Done: true}}}, want: 0}, //nolint:staticcheck
		{name: "first self", notify: ipn.Notify{SelfChange: selfA}, want: changeIPN},
		{name: "self identity", before: []ipn.Notify{{SelfChange: selfA}}, notify: ipn.Notify{SelfChange: testSelfNode(11, "self-b", 200)}, want: changeIPN | changeTargets | changeIdentity},
		{name: "initial status", notify: ipn.Notify{InitialStatus: testStatus(testPeerStatus("n1", 1, true, time.Time{}))}, want: changeIPN | changeTargets},
		{name: "new peer", notify: ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode("n1", 1, true)}}, want: changeIPN | changeTargets},
		{name: "peer removed", before: []ipn.Notify{{PeersChanged: []*tailcfg.Node{testPeerNode("n1", 1, true)}}}, notify: ipn.Notify{PeersRemoved: []tailcfg.NodeID{1}}, want: changeIPN | changeTargets},
		{name: "unknown peer removed", before: []ipn.Notify{{PeersChanged: []*tailcfg.Node{testPeerNode("n1", 1, true)}}}, notify: ipn.Notify{PeersRemoved: []tailcfg.NodeID{99}}, want: changeIPN},
		{name: "same self renamed", before: []ipn.Notify{{SelfChange: selfA}}, notify: ipn.Notify{SelfChange: renamed}, want: changeIPN},
		{name: "peer update", before: []ipn.Notify{{PeersChanged: []*tailcfg.Node{testPeerNode("n1", 1, false)}}}, notify: ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode("n1", 1, true)}}, want: changeIPN},
		{name: "browse URL", notify: ipn.Notify{BrowseToURL: new("https://example.com")}, want: changeIPN},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var w watchState
			for _, n := range tt.before {
				w.apply(&n)
			}
			if got := w.apply(&tt.notify); got != tt.want {
				t.Fatalf("got %b, want %b", got, tt.want)
			}
		})
	}
}

// TestBusOnline checks that Online returns an online status right
// away, waits through offline ones for the next online one, and gives
// up when its context is done.
func TestBusOnline(t *testing.T) {
	t.Parallel()

	b := &Bus{New: func(*IPNStatus) {}}
	b.init()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := b.Online(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Online with no status: err = %v, want %v", err, context.DeadlineExceeded)
	}

	b.publish(&IPNStatus{State: ipn.Starting})
	got := make(chan *IPNStatus, 1)
	go func() {
		s, err := b.Online(t.Context())
		if err != nil {
			t.Error(err)
		}
		got <- s
	}()

	select {
	case s := <-got:
		t.Fatalf("Online returned %v before an online status", s.State)
	case <-time.After(50 * time.Millisecond):
	}

	b.publish(&IPNStatus{State: ipn.NoState})
	running := &IPNStatus{State: ipn.Running}
	b.publish(running)
	select {
	case s := <-got:
		if s != running {
			t.Fatalf("Online returned %v, want the running status", s.State)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Online didn't return after an online status")
	}

	s, err := b.Online(t.Context())
	if err != nil || s != running {
		t.Fatalf("Online while online = %v, %v, want the running status", s, err)
	}
}

// TestWaitBrowseURLSeesPublishDuringStart checks that a BrowseToURL
// published while the start function runs, before WaitBrowseURL would
// have called Next, is still returned. App.login used to call StartLogin
// and then Next, which missed that URL and timed out.
func TestWaitBrowseURLSeesPublishDuringStart(t *testing.T) {
	t.Parallel()

	b := &Bus{New: func(*IPNStatus) {}}
	b.init()
	const url = "https://login.example/auth"
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	got, err := b.WaitBrowseURL(ctx, func(context.Context) error {
		b.publish(&IPNStatus{BrowseToURL: url})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != url {
		t.Fatalf("got %q, want %q", got, url)
	}
}
