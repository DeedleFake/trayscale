package tsutil

import (
	"cmp"
	"fmt"
	"strings"

	"golang.org/x/net/idna"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/util/dnsname"
)

// DNSOrQuoteHostname returns a nicely printable version of a peer's name. The function is copied from
// https://github.com/tailscale/tailscale/blob/b0ed863d55d6b51569ce5c6bd0b7021338ce6a82/cmd/tailscale/cli/status.go#L285
func DNSOrQuoteHostname(st *ipnstate.Status, ps *ipnstate.PeerStatus) string {
	baseName := ps.DNSName
	if st.CurrentTailnet != nil {
		baseName = dnsname.TrimSuffix(baseName, st.CurrentTailnet.MagicDNSSuffix)
	}
	if baseName != "" {
		if strings.HasPrefix(baseName, "xn-") {
			if u, err := idna.ToUnicode(baseName); err == nil {
				return fmt.Sprintf("%s (%s)", baseName, u)
			}
		}
		return baseName
	}
	return fmt.Sprintf("(%q)", dnsname.SanitizeHostname(ps.HostName))
}

// IsMullvad returns true if peer is a Mullvad exit node.
func IsMullvad(peer *ipnstate.PeerStatus) bool {
	return (peer.Tags != nil) && peer.Tags.ContainsFunc(func(tag string) bool {
		return tag == "tag:mullvad-exit-node"
	})
}

// CanMullvad returns true if peer is allowed to access Mullvad exit
// nodes.
func CanMullvad(peer *ipnstate.PeerStatus) bool {
	return peer.HasCap("mullvad")
}

// CompareLocations alphabestically compares the countries and then,
// if necessary, cities of two Locations.
func CompareLocations(loc1, loc2 *tailcfg.Location) int {
	return cmp.Or(
		cmp.Compare(loc1.Country, loc2.Country),
		cmp.Compare(loc1.City, loc2.City),
	)
}

// ComparePeers compares two peers. It does so by location if
// available, then by hostname. It returns the peers in a
// deterministic order if their locations or hostnames are identical,
// so the result of calling this is never 0. To determine if peers are
// the same, compare their IDs manually.
func ComparePeers(p1, p2 *ipnstate.PeerStatus) int {
	loc := 0
	if p1.Location != nil && p2.Location != nil {
		loc = CompareLocations(p1.Location, p2.Location)
	}
	return cmp.Or(
		loc,
		cmp.Compare(p1.HostName, p2.HostName),
		cmp.Compare(p1.ID, p2.ID),
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
