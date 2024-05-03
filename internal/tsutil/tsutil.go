package tsutil

import (
	"fmt"
	"strings"

	"golang.org/x/net/idna"
	"tailscale.com/ipn/ipnstate"
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
	return peer.CapMap.Contains("mullvad")
}
