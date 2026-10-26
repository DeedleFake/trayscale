package tsutil

import (
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
