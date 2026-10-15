package ui

import (
	"net/netip"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func TestSelfPageAutomaticExitNode(t *testing.T) {
	requireGTK(t)

	exit := layoutPeer(t, "exit", "gateway", true, true, false)
	status := layoutStatus([]tailcfg.NodeView{exit})
	status.Prefs = ipn.NewPrefs().View()
	page := NewSelfPage(&App{}, status)
	defer page.Destroy()
	stack := adw.NewViewStack()
	defer stack.Unref()
	stackPage := stack.AddNamed(page.Widget(), "self")
	defer stackPage.Unref()
	page.Bind(stackPage)
	var advertising ipn.Prefs
	advertising.SetAdvertiseExitNode(true)

	tests := []struct {
		name          string
		prefs         ipn.Prefs
		wantAutomatic bool
		wantSensitive bool
		wantNode      string
	}{
		{name: "off", wantSensitive: true, wantNode: "None"},
		{name: "manual", prefs: ipn.Prefs{ExitNodeID: "exit"}, wantSensitive: true, wantNode: "gateway"},
		{name: "automatic", prefs: ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "exit"}, wantAutomatic: true, wantSensitive: true, wantNode: "gateway"},
		{name: "waiting", prefs: ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "auto:any"}, wantAutomatic: true, wantSensitive: true, wantNode: "Waiting for an exit node"},
		{name: "manual choice after automatic", prefs: ipn.Prefs{ExitNodeID: "exit"}, wantSensitive: true, wantNode: "gateway"},
		{name: "missing peer", prefs: ipn.Prefs{ExitNodeID: "missing"}, wantSensitive: true, wantNode: "Unavailable"},
		{name: "legacy IP", prefs: ipn.Prefs{ExitNodeIP: netip.MustParseAddr("100.64.0.1")}, wantSensitive: true, wantNode: "100.64.0.1"},
		{name: "advertising", prefs: advertising, wantNode: "None"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status.Prefs = tt.prefs.View()
			page.updateExitNode(status)
			sw := rowSwitch(page.AutomaticExitNodeRow)
			if sw.GetActive() != tt.wantAutomatic || sw.GetState() != tt.wantAutomatic {
				t.Errorf("automatic active=%v state=%v, want %v", sw.GetActive(), sw.GetState(), tt.wantAutomatic)
			}
			if got := page.AutomaticExitNodeRow.GetSensitive(); got != tt.wantSensitive {
				t.Errorf("sensitive = %v, want %v", got, tt.wantSensitive)
			}
			if got := page.CurrentExitNodeRow.GetSubtitle(); got != tt.wantNode {
				t.Errorf("current exit node = %q, want %q", got, tt.wantNode)
			}
		})
	}
}

// An old daemon status must not cancel a click during the toggle animation,
// before the switch has emitted state-set and started the request.
func TestAutomaticExitNodeSwitchUpdates(t *testing.T) {
	requireGTK(t)

	page := &SelfPage{AutomaticExitNodeRow: adw.NewSwitchRow(), CurrentExitNodeRow: adw.NewActionRow()}
	defer page.AutomaticExitNodeRow.Unref()
	defer page.CurrentExitNodeRow.Unref()
	w := gtk.NewWindow()
	defer w.Destroy()
	list := gtk.NewListBox()
	defer list.Unref()
	list.Append(&page.AutomaticExitNodeRow.Widget)
	w.SetChild(&list.Widget)
	w.Present()
	iterateFor(500 * time.Millisecond)
	sw := rowSwitch(page.AutomaticExitNodeRow)
	sw.Activate()
	iterateFor(40 * time.Millisecond)
	page.updateExitNode(&tsutil.IPNStatus{Prefs: ipn.NewPrefs().View()})
	iterateFor(300 * time.Millisecond)
	if !sw.GetActive() {
		t.Fatal("old status cancelled the automatic exit-node toggle")
	}
}
