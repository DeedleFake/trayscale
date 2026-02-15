package tsutil

import (
	"cmp"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

// DNSOrQuoteHostname returns a nicely printable version of a peer's name. The function is copied from
// https://github.com/tailscale/tailscale/blob/b0ed863d55d6b51569ce5c6bd0b7021338ce6a82/cmd/tailscale/cli/status.go#L285
//func DNSOrQuoteHostname(st *ipnstate.Status, ps *ipnstate.PeerStatus) string {
//	baseName := ps.DNSName
//	if st.CurrentTailnet != nil {
//		baseName = dnsname.TrimSuffix(baseName, st.CurrentTailnet.MagicDNSSuffix)
//	}
//	if baseName != "" {
//		if strings.HasPrefix(baseName, "xn-") {
//			if u, err := idna.ToUnicode(baseName); err == nil {
//				return fmt.Sprintf("%s (%s)", baseName, u)
//			}
//		}
//		return baseName
//	}
//	return fmt.Sprintf("(%q)", dnsname.SanitizeHostname(ps.HostName))
//}

const AdminDashboardURL = "https://tailscale.com/admin"

// IsMullvad returns true if peer is a Mullvad exit node.
func IsMullvad(peer tailcfg.NodeView) bool {
	return peer.Tags().ContainsFunc(func(tag string) bool {
		return tag == "tag:mullvad-exit-node"
	})
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

// ComparePeers compares two peers. It does so by location if
// available, then by hostname. It returns the peers in a
// deterministic order if their locations or hostnames are identical,
// so the result of calling this is never 0. To determine if peers are
// the same, compare their IDs manually.
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

// CompareWaitingFiles compares two incoming files first by name and
// then by size.
func CompareWaitingFiles(f1, f2 apitype.WaitingFile) int {
	return cmp.Or(
		cmp.Compare(f1.Name, f2.Name),
		cmp.Compare(f1.Size, f2.Size),
	)
}

// CanReceiveFiles returns true if peer can be sent files via
// Taildrop.
//func CanReceiveFiles(peer tailcfg.NodeView) bool {
//	return peer.NoFileSharingReason == ""
//}
