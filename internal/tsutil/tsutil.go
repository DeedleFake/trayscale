package tsutil

import (
	"cmp"

	"tailscale.com/tailcfg"
)

const AdminDashboardURL = "https://tailscale.com/admin"

// IsMullvad returns true if peer is a Mullvad exit node.
func IsMullvad(peer tailcfg.NodeView) bool {
	return peer.Tags().ContainsFunc(func(tag string) bool {
		return tag == "tag:mullvad-exit-node"
	})
}

// IsShareeNode reports whether peer is in the netmap only because it
// belongs to a user that a device was shared to. These are hidden by
// tailscale status and should not appear in the peer list.
func IsShareeNode(peer tailcfg.NodeView) bool {
	if !peer.Valid() {
		return false
	}
	hi := peer.Hostinfo()
	return hi.Valid() && hi.ShareeNode()
}

// CanMullvad returns true if peer is allowed to access Mullvad exit
// nodes.
func CanMullvad(peer tailcfg.NodeView) bool {
	return peer.HasCap("mullvad")
}

// CompareLocations alphabestically compares the countries and then,
// if necessary, cities of two Locations.
func CompareLocations(loc1, loc2 tailcfg.LocationView) int {
	return cmp.Or(
		cmp.Compare(loc1.Country(), loc2.Country()),
		cmp.Compare(loc1.City(), loc2.City()),
	)
}

// ComparePeers orders two peers by location if both have one, then by
// hostname, then by node ID. Distinct IDs always compare as non-zero.
func ComparePeers(p1, p2 tailcfg.NodeView) int {
	i1 := p1.Hostinfo()
	i2 := p2.Hostinfo()

	loc := 0
	if i1.Location().Valid() && i2.Location().Valid() {
		loc = CompareLocations(i1.Location(), i2.Location())
	}
	return cmp.Or(
		loc,
		cmp.Compare(i1.Hostname(), i2.Hostname()),
		cmp.Compare(p1.ID(), p2.ID()),
	)
}
