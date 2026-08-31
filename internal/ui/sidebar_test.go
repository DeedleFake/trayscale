package ui

import (
	"net/netip"
	"slices"
	"testing"

	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
	"tailscale.com/net/tsaddr"
	"tailscale.com/tailcfg"
)

func TestSidebarLayout(t *testing.T) {
	online := layoutPeer(t, "online", "alpha", true, false, false)
	online2 := layoutPeer(t, "online2", "bravo", true, false, false)
	exit := layoutPeer(t, "exit", "gateway", true, true, false)
	offlinePeer := layoutPeer(t, "offline-peer", "zeta", false, false, false)
	offlineExit := layoutPeer(t, "offline-exit", "old-gw", false, true, false)
	sharee := layoutPeer(t, "sharee", "shared", true, false, true)
	workstation := layoutPeer(t, "workstation", "workstation", true, false, false)
	server := layoutPeer(t, "server", "server", true, false, false)
	srvBackup := layoutPeer(t, "srv-backup", "srv-backup", true, false, false)

	peers := []tailcfg.NodeView{online, online2, exit, offlinePeer, offlineExit, sharee, workstation, server, srvBackup}
	running := layoutStatus(peers)
	stopped := layoutStatus(peers)
	stopped.State = ipn.Stopped

	pages := []string{"self", "mullvad", "offline", "online", "online2", "exit", "offline-peer", "offline-exit", "sharee", "workstation", "server", "srv-backup"}

	tests := []struct {
		name        string
		pages       []string
		status      *tsutil.IPNStatus
		showOffline bool
		query       string
		want        []sidebarPage
	}{
		{
			name:   "offline daemon",
			pages:  pages,
			status: stopped,
			want:   []sidebarPage{{name: "offline"}},
		},
		{
			name:        "sections",
			pages:       pages,
			status:      running,
			showOffline: true,
			want: []sidebarPage{
				{name: "self", section: "This machine"},
				{name: "mullvad", section: "Exit Nodes"},
				{name: "exit"},
				{name: "offline-exit"},
				{name: "online", section: "Online"},
				{name: "online2"},
				{name: "server"},
				{name: "srv-backup"},
				{name: "workstation"},
				{name: "offline-peer", section: "Offline"},
			},
		},
		{
			name:        "hide offline",
			pages:       pages,
			status:      running,
			showOffline: false,
			want: []sidebarPage{
				{name: "self", section: "This machine"},
				{name: "mullvad", section: "Exit Nodes"},
				{name: "exit"},
				{name: "offline-exit"},
				{name: "online", section: "Online"},
				{name: "online2"},
				{name: "server"},
				{name: "srv-backup"},
				{name: "workstation"},
			},
		},
		{
			name:        "no mullvad",
			pages:       []string{"self", "exit", "online"},
			status:      running,
			showOffline: true,
			want: []sidebarPage{
				{name: "self", section: "This machine"},
				{name: "exit", section: "Exit Nodes"},
				{name: "online", section: "Online"},
			},
		},
		{
			name:        "search ranked no sections",
			pages:       pages,
			status:      running,
			showOffline: true,
			query:       "srv",
			want: []sidebarPage{
				{name: "srv-backup"},
				{name: "server"},
			},
		},
		{
			name:        "search omits self mullvad offline",
			pages:       pages,
			status:      running,
			showOffline: true,
			query:       "workstation",
			want: []sidebarPage{
				{name: "workstation"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sidebarLayout(slices.Values(tt.pages), tt.status, tt.showOffline, tt.query)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func layoutStatus(peers []tailcfg.NodeView) *tsutil.IPNStatus {
	s := &tsutil.IPNStatus{
		State: ipn.Running,
		Peers: make(map[tailcfg.StableNodeID]tailcfg.NodeView, len(peers)),
	}
	for _, p := range peers {
		s.Peers[p.StableID()] = p
	}
	return s
}

func layoutPeer(t *testing.T, id, hostname string, online, exit, sharee bool) tailcfg.NodeView {
	t.Helper()
	hi := &tailcfg.Hostinfo{Hostname: hostname, ShareeNode: sharee}
	n := &tailcfg.Node{
		ID:       tailcfg.NodeID(len(id)),
		StableID: tailcfg.StableNodeID(id),
		Name:     hostname + ".example.ts.net.",
		Hostinfo: hi.View(),
		Online:   &online,
	}
	if exit {
		n.AllowedIPs = []netip.Prefix{tsaddr.AllIPv4(), tsaddr.AllIPv6()}
	}
	n.InitDisplayNames("example.ts.net")
	return n.View()
}
