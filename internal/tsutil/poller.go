package tsutil

import (
	"context"
	"log/slog"
	"maps"
	"net/netip"
	"os/user"
	"time"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/util/set"
)

type Status interface {
	status()
}

type IPNStatus struct {
	State       ipn.State
	Prefs       ipn.PrefsView
	self        tailcfg.NodeView
	Peers       map[tailcfg.StableNodeID]tailcfg.NodeView
	FileTargets set.Set[tailcfg.StableNodeID]
	BrowseToURL string
}

func (*IPNStatus) status() {}

// copy returns a copy of s to publish. Published statuses always have
// valid Prefs, empty if tailscaled hasn't sent any, so that readers
// don't have to check.
func (s IPNStatus) copy() *IPNStatus {
	if !s.Prefs.Valid() {
		s.Prefs = new(ipn.Prefs).View()
	}
	s.Peers = maps.Clone(s.Peers)
	s.FileTargets = maps.Clone(s.FileTargets)
	return &s
}

func (s *IPNStatus) ensurePeers() {
	if s.Peers == nil {
		s.Peers = make(map[tailcfg.StableNodeID]tailcfg.NodeView)
	}
}

func (s *IPNStatus) applyInitialStatus(st *ipnstate.Status) {
	var suffix string
	if st.CurrentTailnet != nil {
		suffix = st.CurrentTailnet.MagicDNSSuffix
	}

	if st.Self != nil {
		s.self = nodeViewFromPeerStatus(st.Self, suffix)
	}

	s.ensurePeers()
	clear(s.Peers)
	for _, peer := range st.Peer {
		if peer == nil {
			continue
		}
		s.Peers[peer.ID] = nodeViewFromPeerStatus(peer, suffix)
	}
}

func (s *IPNStatus) applySelfChange(self *tailcfg.Node) (identityChanged bool) {
	nv := self.View()
	if s.self.Valid() && nv.Valid() && !sameSelfNode(s.self, nv) {
		// Profile switches reuse the watch session and send a complete
		// PeersChanged list without PeersRemoved for the old nodes.
		clear(s.Peers)
		clear(s.FileTargets)
		identityChanged = true
	}
	s.self = nv
	return identityChanged
}

func sameSelfNode(a, b tailcfg.NodeView) bool {
	return a.ID() == b.ID() && a.StableID() == b.StableID() && a.User() == b.User()
}

func (s *IPNStatus) applyPeersChanged(peers []*tailcfg.Node) (added bool) {
	s.ensurePeers()
	for _, peer := range peers {
		if _, ok := s.Peers[peer.StableID]; !ok {
			added = true
		}
		s.Peers[peer.StableID] = peer.View()
	}
	return added
}

func (s *IPNStatus) applyPeersRemoved(ids []tailcfg.NodeID) (removed bool) {
	for _, id := range ids {
		for stableID, peer := range s.Peers {
			if peer.ID() == id {
				delete(s.Peers, stableID)
				removed = true
				break
			}
		}
	}
	return removed
}

func (s *IPNStatus) refreshFileTargets(ctx context.Context) {
	// This is a lot longer than it probably should be. It's basically
	// just to make sure that the bus doesn't get completely stuck. If
	// this is getting hit, though, the UI is going to be updating
	// horribly slow.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	targets, err := FileTargets(ctx)
	if err != nil {
		slog.Error("failed to get file targets", "err", err)
		return
	}
	s.FileTargets.Make()
	clear(s.FileTargets)
	for _, target := range targets {
		s.FileTargets.Add(target.Node.StableID)
	}
}

// nodeViewFromPeerStatus builds a NodeView from an ipnstate.PeerStatus for
// the InitialStatus bootstrap. Subsequent updates use full Nodes from
// SelfChange / PeersChanged.
func nodeViewFromPeerStatus(ps *ipnstate.PeerStatus, magicDNSSuffix string) tailcfg.NodeView {
	if ps == nil {
		return tailcfg.NodeView{}
	}

	online := ps.Online
	n := &tailcfg.Node{
		ID:       ps.NodeID,
		StableID: ps.ID,
		Name:     ps.DNSName,
		User:     ps.UserID,
		Sharer:   ps.AltSharerUserID,
		Key:      ps.PublicKey,
		Created:  ps.Created,
		Online:   &online,
		Expired:  ps.Expired,
		CapMap:   maps.Clone(ps.CapMap),
	}
	if ps.KeyExpiry != nil {
		n.KeyExpiry = *ps.KeyExpiry
	}
	if !ps.LastSeen.IsZero() {
		lastSeen := ps.LastSeen
		n.LastSeen = &lastSeen
	}
	for _, ip := range ps.TailscaleIPs {
		n.Addresses = append(n.Addresses, netip.PrefixFrom(ip, ip.BitLen()))
	}
	if ps.AllowedIPs != nil {
		n.AllowedIPs = ps.AllowedIPs.AsSlice()
	}
	if ps.Tags != nil {
		n.Tags = ps.Tags.AsSlice()
	}
	if ps.PrimaryRoutes != nil {
		n.PrimaryRoutes = ps.PrimaryRoutes.AsSlice()
	}

	hi := &tailcfg.Hostinfo{
		Hostname:   ps.HostName,
		OS:         ps.OS,
		ShareeNode: ps.ShareeNode,
	}
	if ps.Location != nil {
		loc := *ps.Location
		hi.Location = &loc
	}
	n.Hostinfo = hi.View()
	n.InitDisplayNames(magicDNSSuffix)
	return n.View()
}

// Online returns true if s indicates that the local node is online
// and connected to the tailnet.
func (s *IPNStatus) Online() bool {
	return s.State == ipn.Running
}

func (s *IPNStatus) NeedsAuth() bool {
	return s.State == ipn.NeedsLogin
}

func (s *IPNStatus) ExitNodeActive() bool {
	return s.Prefs.ExitNodeID() != "" || s.Prefs.ExitNodeIP().IsValid()
}

func (s *IPNStatus) ExitNode() tailcfg.NodeView {
	if node, ok := s.Peers[s.Prefs.ExitNodeID()]; ok {
		return node
	}
	if addr := s.Prefs.ExitNodeIP(); addr.IsValid() {
		return s.peerByTailscaleIP(addr)
	}
	return tailcfg.NodeView{}
}

func (s *IPNStatus) peerByTailscaleIP(ip netip.Addr) tailcfg.NodeView {
	for _, peer := range s.Peers {
		for _, a := range peer.Addresses().All() {
			if a.Addr() == ip {
				return peer
			}
		}
	}
	return tailcfg.NodeView{}
}

func (s *IPNStatus) OperatorIsCurrent() bool {
	current, err := user.Current()
	if err != nil {
		slog.Error("get current user", "err", err)
		return false
	}

	return s.Prefs.OperatorUser() == current.Username
}

// Self returns the local node. The boolean is false when the node is
// missing or invalid.
func (s *IPNStatus) Self() (tailcfg.NodeView, bool) {
	n := s.self
	return n, n.Valid()
}

func (s *IPNStatus) SelfAddr() netip.Addr {
	self, ok := s.Self()
	if !ok {
		return netip.Addr{}
	}

	addrs := self.Addresses()
	if addrs.Len() == 0 {
		return netip.Addr{}
	}

	addr := addrs.At(0)
	for _, a := range addrs.SliceFrom(1).All() {
		if a.Compare(addr) < 0 {
			addr = a
		}
	}
	return addr.Addr()
}

type FileStatus struct {
	Files []apitype.WaitingFile

	// Arrived holds the names of the files received since the last
	// FileStatus, as tailscaled reported their transfers finishing.
	// Some may no longer be waiting.
	Arrived []string
}

func (*FileStatus) status() {}

type ProfileStatus struct {
	Profile  ipn.LoginProfile
	Profiles []ipn.LoginProfile
}

func (*ProfileStatus) status() {}
