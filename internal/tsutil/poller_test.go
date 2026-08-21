package tsutil

import (
	"context"
	"errors"
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

func TestWatcherOptsFor(t *testing.T) {
	t.Parallel()

	legacy := watcherOptsFor("1.98.9")
	modern := watcherOptsFor("1.102.2")

	for _, opts := range []ipn.NotifyWatchOpt{legacy, modern} {
		require.NoError(t, ipn.ValidateNotifyWatchOpt(opts))
		require.Equal(t, ipn.NotifyWatchOpt(0), opts&ipn.NotifyRateLimit)
		require.Equal(t, ipn.NotifyNoNetMap, opts&ipn.NotifyNoNetMap)
		require.Equal(t, ipn.NotifyInitialStatus, opts&ipn.NotifyInitialStatus)
		require.Error(t, ipn.ValidateNotifyWatchOpt(opts|ipn.NotifyRateLimit))
	}

	require.Equal(t, ipn.NotifyWatchOpt(0), legacy&ipn.NotifyPeerChanges)
	require.Equal(t, ipn.NotifyPeerChanges, modern&ipn.NotifyPeerChanges)

	tests := []struct {
		name            string
		ver             string
		wantPeerChanges bool
	}{
		{name: "empty", ver: "", wantPeerChanges: false},
		{name: "1.98.9", ver: "1.98.9", wantPeerChanges: false},
		{name: "1.99.0", ver: "1.99.0", wantPeerChanges: false},
		{name: "1.100", ver: "1.100", wantPeerChanges: true},
		{name: "1.100.0", ver: "1.100.0", wantPeerChanges: true},
		{name: "1.102.2", ver: "1.102.2", wantPeerChanges: true},
		{name: "1.102 long", ver: "1.102.2-t6cac91817-g6ff0ddc72", wantPeerChanges: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := watcherOptsFor(tt.ver)&ipn.NotifyPeerChanges != 0
			require.Equal(t, tt.wantPeerChanges, got)
		})
	}
}

func TestApplyInitialStatusKeepsPeersWhenEmpty(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	var s IPNStatus
	s.applyNetMap(testNetMap(id, 1, false))
	require.Contains(t, s.Peers, id)
	require.False(t, s.Peers[id].Online().Get())

	s.applyInitialStatus(&ipnstate.Status{Peer: nil})
	require.Contains(t, s.Peers, id)
	require.False(t, s.Peers[id].Online().Get())

	s.applyInitialStatus(&ipnstate.Status{Peer: map[key.NodePublic]*ipnstate.PeerStatus{}})
	require.Contains(t, s.Peers, id)
	require.False(t, s.Peers[id].Online().Get())
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

func TestApplyInitialStatusOverlaysNetMapOnline(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	lastSeen := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	var s IPNStatus

	s.applyInitialStatus(testStatus(testPeerStatus(id, 1, true, time.Time{})))
	require.True(t, s.Peers[id].Online().Get())

	s.applyNetMap(testNetMap(id, 1, false))
	require.False(t, s.Peers[id].Online().Get())
	require.True(t, s.Peers[id].LastSeen().Get().IsZero())

	s.applyInitialStatus(testStatus(testPeerStatus(id, 1, true, lastSeen)))
	require.Len(t, s.Peers, 1)
	got := s.Peers[id]
	require.True(t, got.Online().Get())
	require.Equal(t, lastSeen, got.LastSeen().Get())
}

func TestApplyNotifyStatusOverlay(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	lastSeen := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	stale := testNotify(testNetMap(id, 1, false), testStatus(testPeerStatus(id, 1, true, time.Time{})))

	t.Run("overlay error keeps netmap peers", func(t *testing.T) {
		assertFailedOverlayConsumed(t, id, stale, func(context.Context) (*ipnstate.Status, error) {
			return nil, errors.New("status failed")
		})
	})

	t.Run("nil overlay keeps netmap peers", func(t *testing.T) {
		assertFailedOverlayConsumed(t, id, stale, func(context.Context) (*ipnstate.Status, error) {
			return nil, nil
		})
	})

	t.Run("successful overlay wins over netmap", func(t *testing.T) {
		for _, wantOnline := range []bool{true, false} {
			sess := &ipnSession{
				needOverlay: true,
				getStatus: func(context.Context) (*ipnstate.Status, error) {
					return testStatus(testPeerStatus(id, 1, wantOnline, lastSeen)), nil
				},
			}
			sess.applyNotify(context.Background(), stale)
			require.Len(t, sess.status.Peers, 1)
			got := sess.status.Peers[id]
			require.Equal(t, wantOnline, got.Online().Get())
			require.Equal(t, lastSeen, got.LastSeen().Get())
		}
	})
}

func TestApplyNotifyOverlayOncePerSession(t *testing.T) {
	id := tailcfg.StableNodeID("n1")
	var calls int
	get := func(context.Context) (*ipnstate.Status, error) {
		calls++
		return testStatus(testPeerStatus(id, 1, true, time.Time{})), nil
	}

	sess := &ipnSession{needOverlay: true, getStatus: get}
	sess.applyNotify(context.Background(), testNotify(testNetMap(id, 1, false), nil))
	require.Equal(t, 1, calls)
	require.True(t, sess.status.Peers[id].Online().Get())

	sess.applyNotify(context.Background(), testNotify(testNetMap(id, 1, false), nil))
	require.Equal(t, 1, calls)
	require.False(t, sess.status.Peers[id].Online().Get())

	sess2 := &ipnSession{needOverlay: true, getStatus: get}
	sess2.applyNotify(context.Background(), testNotify(testNetMap(id, 1, false), nil))
	require.Equal(t, 2, calls)
	require.True(t, sess2.status.Peers[id].Online().Get())
}

func assertFailedOverlayConsumed(t *testing.T, id tailcfg.StableNodeID, n *ipn.Notify, first func(context.Context) (*ipnstate.Status, error)) {
	t.Helper()
	var calls int
	sess := &ipnSession{
		needOverlay: true,
		getStatus: func(ctx context.Context) (*ipnstate.Status, error) {
			calls++
			if calls == 1 {
				return first(ctx)
			}
			return testStatus(testPeerStatus(id, 1, true, time.Time{})), nil
		},
	}
	sess.applyNotify(context.Background(), n)
	require.Equal(t, 1, calls)
	require.False(t, sess.needOverlay)
	require.Len(t, sess.status.Peers, 1)
	require.False(t, sess.status.Peers[id].Online().Get())

	sess.applyNotify(context.Background(), n)
	require.Equal(t, 1, calls)
	require.False(t, sess.status.Peers[id].Online().Get())
}

func testPeer(id tailcfg.StableNodeID, nodeID tailcfg.NodeID, online bool) tailcfg.NodeView {
	o := online
	return (&tailcfg.Node{
		ID:       nodeID,
		StableID: id,
		Name:     "peer.tailnet.ts.net.",
		Online:   &o,
	}).View()
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
		//lint:ignore SA1019 applyNotify still reads initial NetMap
		n.NetMap = nm
	}
	return &n
}
