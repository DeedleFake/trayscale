package tsutil

import (
	"slices"
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
	inProgress := incoming(ipn.PartialFile{Name: "a"})
	done := incoming(ipn.PartialFile{Name: "a", Done: true})

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
		{name: "engine while files wait", before: []ipn.Notify{filesWaiting(ipn.Notify{})}, notify: filesWaiting(ipn.Notify{Engine: engine}), want: 0},
		{name: "files start waiting", notify: filesWaiting(ipn.Notify{}), want: changeFiles},
		{name: "files stop waiting", before: []ipn.Notify{filesWaiting(ipn.Notify{})}, notify: ipn.Notify{Engine: engine}, want: changeFiles},
		{name: "transfer progress", notify: inProgress, want: 0},
		{name: "transfer done", before: []ipn.Notify{inProgress}, notify: done, want: changeFiles},
		{name: "small transfer done", notify: done, want: changeFiles},
		{name: "transfer gone", before: []ipn.Notify{inProgress}, notify: incoming(), want: changeFiles},
		{name: "no transfer news", before: []ipn.Notify{inProgress}, notify: ipn.Notify{State: &running}, want: changeIPN | changeState | changeTargets},
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

// filesWaiting returns n saying that files are waiting, as tailscaled
// adds to every notify while they are.
func filesWaiting(n ipn.Notify) ipn.Notify {
	n.FilesWaiting = &empty.Message{} //nolint:staticcheck
	return n
}

// incoming returns a notify listing transfers in progress.
func incoming(files ...ipn.PartialFile) ipn.Notify {
	if files == nil {
		files = []ipn.PartialFile{} // Means none, unlike nil.
	}
	return ipn.Notify{IncomingFiles: files} //nolint:staticcheck
}

// TestWatchStateArrived checks that each finished transfer is
// reported once, under its final name, even when tailscaled lists it
// in several notifies or the same name arrives again.
func TestWatchStateArrived(t *testing.T) {
	t.Parallel()

	first := time.Unix(1, 0)
	again := time.Unix(2, 0)
	copying := ipn.PartialFile{Name: "a.txt", Started: first}
	renaming := ipn.PartialFile{Name: "a.txt", Started: first, Done: true}
	placed := ipn.PartialFile{Name: "a.txt", Started: first, Done: true, FinalPath: "/files/a.txt"}
	resent := ipn.PartialFile{Name: "a.txt", Started: again, Done: true, FinalPath: "/files/a (1).txt"}

	for _, tt := range []struct {
		name     string
		notifies []ipn.Notify
		want     []string
	}{
		{name: "in progress", notifies: []ipn.Notify{incoming(copying)}},
		{name: "not in place yet", notifies: []ipn.Notify{incoming(renaming)}},
		{name: "in place", notifies: []ipn.Notify{incoming(copying), incoming(renaming), incoming(placed)}, want: []string{"a.txt"}},
		{name: "listed twice", notifies: []ipn.Notify{incoming(placed), incoming(placed), incoming()}, want: []string{"a.txt"}},
		{name: "sent again", notifies: []ipn.Notify{incoming(placed), incoming(), incoming(resent)}, want: []string{"a.txt", "a (1).txt"}},
		{name: "sent again while listed", notifies: []ipn.Notify{incoming(placed), incoming(placed, resent)}, want: []string{"a.txt", "a (1).txt"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var w watchState
			for _, n := range tt.notifies {
				w.apply(&n)
			}
			if !slices.Equal(w.arrived, tt.want) {
				t.Fatalf("arrived %q, want %q", w.arrived, tt.want)
			}
		})
	}
}
