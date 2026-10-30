package ui

import (
	"net/netip"
	"slices"
	"testing"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func TestSelfPageExitNodeRow(t *testing.T) {
	requireGTK(t)

	gateway := layoutPeer(t, "gateway", "gateway", true, true, false)
	sharee := layoutPeer(t, "sharee", "shared", true, true, true)

	loc := &tailcfg.Location{Country: "Sweden", CountryCode: "SE", City: "Stockholm"}
	mullvadNode := gateway.AsStruct()
	mullvadNode.StableID = "mullvad"
	mullvadNode.Tags = []string{"tag:mullvad-exit-node"}
	hi := mullvadNode.Hostinfo.AsStruct()
	hi.Location = loc
	mullvadNode.Hostinfo = hi.View()
	mullvad := mullvadNode.View()

	status := func(prefs ipn.Prefs, peers ...tailcfg.NodeView) *tsutil.IPNStatus {
		s := layoutStatus(peers)
		s.Prefs = prefs.View()
		return s
	}

	tests := []struct {
		name            string
		status          *tsutil.IPNStatus
		wantSubtitle    string
		wantActivatable bool
	}{
		{
			name:         "no exit node",
			status:       status(ipn.Prefs{}, gateway),
			wantSubtitle: "None",
		},
		{
			name:            "peer",
			status:          status(ipn.Prefs{ExitNodeID: "gateway"}, gateway),
			wantSubtitle:    peerName(gateway),
			wantActivatable: true,
		},
		{
			name:         "sharee has no page",
			status:       status(ipn.Prefs{ExitNodeID: "sharee"}, sharee),
			wantSubtitle: peerName(sharee),
		},
		{
			name:         "IP not among peers",
			status:       status(ipn.Prefs{ExitNodeIP: netip.MustParseAddr("100.64.0.9")}, gateway),
			wantSubtitle: "100.64.0.9",
		},
		{
			name:         "peer not in netmap",
			status:       status(ipn.Prefs{ExitNodeID: "removed"}, gateway),
			wantSubtitle: "Unavailable",
		},
		{
			name:         "automatic selection without a node yet",
			status:       status(ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "auto:any"}, gateway),
			wantSubtitle: "Waiting for an exit node",
		},
		{
			name:         "automatic selection after its node left the netmap",
			status:       status(ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "removed"}, gateway),
			wantSubtitle: "Waiting for an exit node",
		},
		{
			name:            "automatic selection resolved",
			status:          status(ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "gateway"}, gateway),
			wantSubtitle:    peerName(gateway),
			wantActivatable: true,
		},
		{
			name:            "Mullvad",
			status:          status(ipn.Prefs{ExitNodeID: "mullvad"}, mullvad),
			wantSubtitle:    mullvadLongLocationName(loc.View()),
			wantActivatable: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := NewSelfPage(&App{}, &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()})
			defer page.Widget().Unref()

			page.updateExitNodeRow(test.status)
			require.Equal(t, test.wantSubtitle, page.ExitNodeRow.GetSubtitle())
			require.Equal(t, test.wantActivatable, page.ExitNodeRow.GetActivatable())
			require.Equal(t, test.wantActivatable, page.ExitNodeIcon.GetVisible())
		})
	}
}

// TestSelfPageStartsWithWaitingFiles checks that a self page created
// after the waiting files arrived lists them, as nothing fetches them
// again for it.
func TestSelfPageStartsWithWaitingFiles(t *testing.T) {
	requireGTK(t)

	waiting := []apitype.WaitingFile{{Name: "a.txt", Size: 1}, {Name: "b.txt", Size: 2}}
	page := NewSelfPage(&App{waiting: &waiting}, &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()})
	defer page.Widget().Unref()

	require.Equal(t, waiting, slices.Collect(page.fileModel.All()))
	require.Equal(t, len(waiting), page.incomingFiles)
}
