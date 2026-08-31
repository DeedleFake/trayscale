package tsutil_test

import (
	"net/netip"
	"testing"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netmap"
)

func TestSelfAndSelfAddr(t *testing.T) {
	t.Run("nil netmap", func(t *testing.T) {
		var s tsutil.IPNStatus
		self, ok := s.Self()
		require.False(t, ok)
		require.False(t, self.Valid())
		require.False(t, s.SelfAddr().IsValid())
	})

	t.Run("invalid self node", func(t *testing.T) {
		s := tsutil.IPNStatus{NetMap: &netmap.NetworkMap{}}
		self, ok := s.Self()
		require.False(t, ok)
		require.False(t, self.Valid())
		require.False(t, s.SelfAddr().IsValid())
	})

	t.Run("valid self node", func(t *testing.T) {
		high := netip.MustParsePrefix("100.64.1.2/32")
		low := netip.MustParsePrefix("100.64.0.1/32")
		n := &tailcfg.Node{Addresses: []netip.Prefix{high, low}}
		s := tsutil.IPNStatus{NetMap: &netmap.NetworkMap{SelfNode: n.View()}}

		self, ok := s.Self()
		require.True(t, ok)
		require.True(t, self.Valid())
		require.Equal(t, low.Addr(), s.SelfAddr())
	})

	t.Run("valid self node with no addresses", func(t *testing.T) {
		n := &tailcfg.Node{}
		s := tsutil.IPNStatus{NetMap: &netmap.NetworkMap{SelfNode: n.View()}}

		self, ok := s.Self()
		require.True(t, ok)
		require.True(t, self.Valid())
		require.False(t, s.SelfAddr().IsValid())
	})
}

func TestIsShareeNode(t *testing.T) {
	require.False(t, tsutil.IsShareeNode(tailcfg.NodeView{}))
	require.False(t, tsutil.IsShareeNode((&tailcfg.Node{}).View()))

	n := &tailcfg.Node{
		Hostinfo: (&tailcfg.Hostinfo{ShareeNode: true}).View(),
	}
	require.True(t, tsutil.IsShareeNode(n.View()))
}
