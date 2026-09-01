package tsutil

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/netmap"
)

func TestSelfAndSelfAddr(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		var s IPNStatus
		self, ok := s.Self()
		require.False(t, ok)
		require.False(t, self.Valid())
		require.False(t, s.SelfAddr().IsValid())
	})

	t.Run("invalid self node", func(t *testing.T) {
		s := IPNStatus{self: tailcfg.NodeView{}}
		self, ok := s.Self()
		require.False(t, ok)
		require.False(t, self.Valid())
		require.False(t, s.SelfAddr().IsValid())
	})

	t.Run("valid self node", func(t *testing.T) {
		high := netip.MustParsePrefix("100.64.1.2/32")
		low := netip.MustParsePrefix("100.64.0.1/32")
		n := &tailcfg.Node{Addresses: []netip.Prefix{high, low}}
		s := IPNStatus{self: n.View()}

		self, ok := s.Self()
		require.True(t, ok)
		require.True(t, self.Valid())
		require.Equal(t, low.Addr(), s.SelfAddr())
	})

	t.Run("valid self node with no addresses", func(t *testing.T) {
		n := &tailcfg.Node{}
		s := IPNStatus{self: n.View()}

		self, ok := s.Self()
		require.True(t, ok)
		require.True(t, self.Valid())
		require.False(t, s.SelfAddr().IsValid())
	})
}

func TestIsShareeNode(t *testing.T) {
	require.False(t, IsShareeNode(tailcfg.NodeView{}))
	require.False(t, IsShareeNode((&tailcfg.Node{}).View()))

	n := &tailcfg.Node{
		Hostinfo: (&tailcfg.Hostinfo{ShareeNode: true}).View(),
	}
	require.True(t, IsShareeNode(n.View()))
}

func TestApplyInitialStatusClearsPeersWhenEmpty(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	var s IPNStatus
	s.applyNetMap(testNetMap(id, 1, false))
	require.Contains(t, s.Peers, id)

	s.applyInitialStatus(&ipnstate.Status{Peer: nil})
	require.Empty(t, s.Peers)

	s.applyNetMap(testNetMap(id, 1, false))
	s.applyInitialStatus(&ipnstate.Status{Peer: map[key.NodePublic]*ipnstate.PeerStatus{}})
	require.Empty(t, s.Peers)
}

func TestApplyInitialStatusReplacesNonEmptyPeerMap(t *testing.T) {
	keep := tailcfg.StableNodeID("keep")
	drop := tailcfg.StableNodeID("drop")
	var s IPNStatus
	s.applyNetMap(&netmap.NetworkMap{
		Peers: []tailcfg.NodeView{
			testPeer(keep, 1, false),
			testPeer(drop, 2, true),
		},
	})
	require.Len(t, s.Peers, 2)

	s.applyInitialStatus(testStatus(testPeerStatus(keep, 1, true, time.Time{})))
	require.Len(t, s.Peers, 1)
	require.Contains(t, s.Peers, keep)
	require.True(t, s.Peers[keep].Online().Get())
}

func TestApplyNotifyPrefersInitialStatusOverNetMap(t *testing.T) {
	keep := tailcfg.StableNodeID("keep")
	drop := tailcfg.StableNodeID("drop")
	lastSeen := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	n := testNotify(
		&netmap.NetworkMap{
			Peers: []tailcfg.NodeView{
				testPeer(keep, 1, false),
				testPeer(drop, 2, true),
			},
		},
		testStatus(testPeerStatus(keep, 1, true, lastSeen)),
	)

	var s IPNStatus
	s.applyNotify(n)
	require.Len(t, s.Peers, 1)
	require.Contains(t, s.Peers, keep)
	require.NotContains(t, s.Peers, drop)
	require.True(t, s.Peers[keep].Online().Get())
	require.Equal(t, lastSeen, s.Peers[keep].LastSeen().Get())
}

func TestApplyNotifyFallsBackToNetMap(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	var s IPNStatus
	s.applyNotify(testNotify(testNetMap(id, 1, false), nil))
	require.Len(t, s.Peers, 1)
	require.Contains(t, s.Peers, id)
	require.False(t, s.Peers[id].Online().Get())
}

func TestApplyNotifyDeltasAfterBootstrap(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	var s IPNStatus
	s.applyNotify(&ipn.Notify{InitialStatus: testStatus(testPeerStatus(id, 1, false, time.Time{}))})
	require.False(t, s.Peers[id].Online().Get())

	s.applyNotify(&ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode(id, 1, true)}})
	require.True(t, s.Peers[id].Online().Get())
}

func TestApplyNotifyTargetsDirty(t *testing.T) {
	t.Parallel()

	id := tailcfg.StableNodeID("n1")
	selfA := testSelfNode(10, "self-a", 100)

	t.Run("bootstrap initial status", func(t *testing.T) {
		var s IPNStatus
		_, targetsDirty := s.applyNotify(&ipn.Notify{InitialStatus: testStatus(testPeerStatus(id, 1, true, time.Time{}))})
		require.True(t, targetsDirty)
	})

	t.Run("bootstrap netmap", func(t *testing.T) {
		var s IPNStatus
		_, targetsDirty := s.applyNotify(testNotify(testNetMap(id, 1, false), nil))
		require.True(t, targetsDirty)
	})

	t.Run("new peer", func(t *testing.T) {
		var s IPNStatus
		_, targetsDirty := s.applyNotify(&ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode(id, 1, false)}})
		require.True(t, targetsDirty)
	})

	t.Run("online upsert", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode(id, 1, false)}})
		_, targetsDirty := s.applyNotify(&ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode(id, 1, true)}})
		require.False(t, targetsDirty)
		require.True(t, s.Peers[id].Online().Get())
	})

	t.Run("remove", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode(id, 1, true)}})
		_, targetsDirty := s.applyNotify(&ipn.Notify{PeersRemoved: []tailcfg.NodeID{1}})
		require.True(t, targetsDirty)
		require.NotContains(t, s.Peers, id)
	})

	t.Run("remove unknown", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{PeersChanged: []*tailcfg.Node{testPeerNode(id, 1, true)}})
		_, targetsDirty := s.applyNotify(&ipn.Notify{PeersRemoved: []tailcfg.NodeID{99}})
		require.False(t, targetsDirty)
		require.Contains(t, s.Peers, id)
	})

	t.Run("first self", func(t *testing.T) {
		var s IPNStatus
		_, targetsDirty := s.applyNotify(&ipn.Notify{SelfChange: selfA})
		require.False(t, targetsDirty)
	})

	t.Run("same identity self", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{SelfChange: selfA})
		renamed := testSelfNode(selfA.ID, selfA.StableID, selfA.User)
		renamed.Name = "renamed.tailnet.ts.net."
		_, targetsDirty := s.applyNotify(&ipn.Notify{SelfChange: renamed})
		require.False(t, targetsDirty)
	})

	t.Run("identity change", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{SelfChange: selfA})
		_, targetsDirty := s.applyNotify(&ipn.Notify{SelfChange: testSelfNode(11, "self-b", 200)})
		require.True(t, targetsDirty)
	})
}

func TestApplyNotifySelfIdentityChange(t *testing.T) {
	t.Parallel()

	oldPeer := tailcfg.StableNodeID("old")
	newPeer := tailcfg.StableNodeID("new")
	selfA := testSelfNode(10, "self-a", 100)

	t.Run("first self keeps existing peers", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{
			PeersChanged: []*tailcfg.Node{testPeerNode(oldPeer, 1, true)},
		})
		s.applyNotify(&ipn.Notify{SelfChange: selfA})
		require.Contains(t, s.Peers, oldPeer)
	})

	t.Run("same identity keeps peers", func(t *testing.T) {
		var s IPNStatus
		s.applyNotify(&ipn.Notify{
			SelfChange:   selfA,
			PeersChanged: []*tailcfg.Node{testPeerNode(oldPeer, 1, true)},
		})
		renamed := testSelfNode(selfA.ID, selfA.StableID, selfA.User)
		renamed.Name = "renamed.tailnet.ts.net."
		s.applyNotify(&ipn.Notify{SelfChange: renamed})
		require.Contains(t, s.Peers, oldPeer)
		got, ok := s.Self()
		require.True(t, ok)
		require.Equal(t, renamed.Name, got.Name())
	})

	t.Run("identity change drops old peers", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			next *tailcfg.Node
		}{
			{name: "node ID", next: testSelfNode(11, selfA.StableID, selfA.User)},
			{name: "stable ID", next: testSelfNode(selfA.ID, "self-b", selfA.User)},
			{name: "user", next: testSelfNode(selfA.ID, selfA.StableID, 200)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				var s IPNStatus
				s.applyNotify(&ipn.Notify{
					SelfChange:   selfA,
					PeersChanged: []*tailcfg.Node{testPeerNode(oldPeer, 1, true)},
				})
				s.FileTargets.Make()
				s.FileTargets.Add(oldPeer)

				s.applyNotify(&ipn.Notify{
					SelfChange:   tt.next,
					PeersChanged: []*tailcfg.Node{testPeerNode(newPeer, 2, true)},
				})
				require.Len(t, s.Peers, 1)
				require.Contains(t, s.Peers, newPeer)
				require.NotContains(t, s.Peers, oldPeer)
				require.False(t, s.FileTargets.Contains(oldPeer))
			})
		}
	})
}

func testPeer(id tailcfg.StableNodeID, nodeID tailcfg.NodeID, online bool) tailcfg.NodeView {
	return testPeerNode(id, nodeID, online).View()
}

func testPeerNode(id tailcfg.StableNodeID, nodeID tailcfg.NodeID, online bool) *tailcfg.Node {
	o := online
	return &tailcfg.Node{
		ID:       nodeID,
		StableID: id,
		Name:     "peer.tailnet.ts.net.",
		Online:   &o,
	}
}

func testSelfNode(id tailcfg.NodeID, stable tailcfg.StableNodeID, user tailcfg.UserID) *tailcfg.Node {
	return &tailcfg.Node{
		ID:       id,
		StableID: stable,
		User:     user,
		Name:     "self.tailnet.ts.net.",
	}
}

func testNetMap(id tailcfg.StableNodeID, nodeID tailcfg.NodeID, online bool) *netmap.NetworkMap {
	return &netmap.NetworkMap{
		Peers: []tailcfg.NodeView{testPeer(id, nodeID, online)},
	}
}

func testPeerStatus(id tailcfg.StableNodeID, nodeID tailcfg.NodeID, online bool, lastSeen time.Time) *ipnstate.PeerStatus {
	return &ipnstate.PeerStatus{
		ID:       id,
		NodeID:   nodeID,
		DNSName:  "peer.tailnet.ts.net.",
		Online:   online,
		LastSeen: lastSeen,
	}
}

func testStatus(ps *ipnstate.PeerStatus) *ipnstate.Status {
	var pub key.NodePublic
	return &ipnstate.Status{
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{pub: ps},
	}
}

func testNotify(nm *netmap.NetworkMap, initial *ipnstate.Status) *ipn.Notify {
	n := ipn.Notify{InitialStatus: initial}
	if nm != nil {
		n.NetMap = nm //nolint:staticcheck // fallback bootstrap when InitialStatus is absent
	}
	return &n
}
