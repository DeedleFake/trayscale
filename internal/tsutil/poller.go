package tsutil

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/netip"
	"os/user"
	"sync"
	"time"

	"deedles.dev/mk"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/feature/taildrop"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netmap"
	"tailscale.com/util/set"
)

// A Poller gets the latest Tailscale status at regular intervals or
// when manually triggered.
//
// A zero-value of a Poller is ready to use.
//
// It is a race condition to change any exported fields of Poller
// while Run is running.
type Poller struct {
	// Interval is the default interval to use for polling.
	//
	// If it is a zero, a non-zero default will be used.
	Interval time.Duration

	// If non-nil, New will be called when a new status is received from
	// Tailscale.
	New func(Status)

	once sync.Once

	poll     chan struct{}
	getIPN   chan *IPNStatus
	nextIPN  chan *IPNStatus
	interval chan time.Duration
}

func (p *Poller) init() {
	p.once.Do(func() {
		mk.Chan(&p.poll, 0)
		mk.Chan(&p.getIPN, 0)
		mk.Chan(&p.nextIPN, 0)
		mk.Chan(&p.interval, 0)
	})
}

// Run runs the poller. It blocks until polling is done, which is
// generally a result of the given Context being cancelled.
//
// The behavior of two calls to Run running concurrently is undefined.
// Don't do it.
func (p *Poller) Run(ctx context.Context) {
	p.init()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	n := newNotifier()
	go p.watchIPN(ctx)
	go p.watchFiles(ctx, n)
	go p.watchProfiles(ctx, n)

	interval := p.Interval
	if interval < 0 {
		interval = 5 * time.Second
	}

	check := time.NewTicker(interval)
	defer check.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case p.poll <- struct{}{}:
			n = n.Notify()
			check.Reset(interval)
		case interval = <-p.interval:
			n = n.Notify()
			check.Reset(interval)
		case <-check.C:
			n = n.Notify()
		}
	}
}

func (p *Poller) watchIPN(ctx context.Context) {
	const watcherOpts = ipn.NotifyInitialState | ipn.NotifyInitialPrefs | ipn.NotifyInitialNetMap | ipn.NotifyNoPrivateKeys | ipn.NotifyWatchEngineUpdates | ipn.NotifyRateLimit

watch:
	watcher, err := localClient.WatchIPNBus(ctx, watcherOpts)
	if err != nil {
		slog.Error("start IPN bus watcher", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			goto watch
		}
	}
	defer watcher.Close()

	set := make(chan *IPNStatus)
	go func() {
		var get chan *IPNStatus
		var s *IPNStatus
		for {
			select {
			case <-ctx.Done():
				return
			case s = <-set:
				get = p.getIPN
				p.New(s)
			case get <- s:
			}
		}
	}()

	var s IPNStatus
	for {
		notify, err := watcher.Next()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				goto watch
			}
			slog.Error("get next IPN bus notification", "err", err)
			continue
		}

		if notify.ErrMessage != nil {
			var state ipn.State
			if notify.State != nil {
				state = *notify.State
			}
			slog.Error("watcher got error message", "state", state, "err", notify.ErrMessage)
		}

		var dirty bool
		if notify.State != nil {
			s.State = *notify.State
			dirty = true
		}
		if notify.Prefs != nil && notify.Prefs.Valid() {
			s.Prefs = *notify.Prefs
			dirty = true
		}
		if notify.NetMap != nil {
			s.NetMap = notify.NetMap
			s.rebuildPeers(ctx)
			dirty = true
		}
		if notify.Engine != nil {
			s.Engine = notify.Engine
			dirty = true
		}
		if notify.BrowseToURL != nil {
			s.BrowseToURL = *notify.BrowseToURL
			dirty = true
		}
		// TODO: Handle health warnings.
		if !dirty {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-p.poll:
		}

		c := s.copy()
		select {
		case <-ctx.Done():
			return
		case set <- c:
		}
		select {
		case p.nextIPN <- c:
		default:
		}
	}
}

func (p *Poller) watchFiles(ctx context.Context, n *notifier) {
	for {
		files, err := WaitingFiles(ctx)
		if err != nil && !errors.Is(err, taildrop.ErrNoTaildrop) {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get waiting files", "err", err)
			goto wait
		}

		p.New(&FileStatus{Files: files})

	wait:
		select {
		case <-ctx.Done():
			return
		case <-n.notify:
			n = n.next
		}
	}
}

func (p *Poller) watchProfiles(ctx context.Context, n *notifier) {
	for {
		profile, profiles, err := GetProfileStatus(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get profile status", "err", err)
			goto wait
		}

		p.New(&ProfileStatus{Profile: profile, Profiles: profiles})

	wait:
		select {
		case <-ctx.Done():
			return
		case <-n.notify:
			n = n.next
		}
	}
}

// Poll returns a channel that, when received from, causes a new
// status to be fetched from Tailscale.
func (p *Poller) Poll() <-chan struct{} {
	p.init()

	return p.poll
}

// GetIPN returns a channel that yields the most recently fetched
// network status. It will block until the network status has been
// fetched successfully once.
func (p *Poller) GetIPN() <-chan *IPNStatus {
	p.init()

	return p.getIPN
}

// NextIPN returns a channel that is sent the new IPNStatus each time
// it is available if anyone is receiving from it. Unlike [GetIPN],
// this channel does not yield the previous status, so it is useful if
// an update is expected to arrive soon. Most usages should use
// [GetIPN] instead as it significantly faster.
func (p *Poller) NextIPN() <-chan *IPNStatus {
	p.init()

	return p.nextIPN
}

// SetInterval returns a channel that modifies the polling interval of
// a running poller. This will delay the next poll until the new
// interval has elapsed.
func (p *Poller) SetInterval() chan<- time.Duration {
	p.init()

	return p.interval
}

type Status interface {
	status()
}

type IPNStatus struct {
	State       ipn.State
	Prefs       ipn.PrefsView
	NetMap      *netmap.NetworkMap
	Peers       map[tailcfg.StableNodeID]tailcfg.NodeView
	FileTargets set.Set[tailcfg.StableNodeID]
	Engine      *ipn.EngineStatus
	BrowseToURL string
}

func (*IPNStatus) status() {}

func (s IPNStatus) copy() *IPNStatus {
	s.Peers = maps.Clone(s.Peers)
	s.FileTargets = maps.Clone(s.FileTargets)
	return &s
}

func (s *IPNStatus) rebuildPeers(ctx context.Context) {
	// This is a lot longer than it probably should be. It's basically
	// just to make sure that the poller doesn't get completely stuck. If
	// this is getting hit, though, the UI is going to be updating
	// horribly slow.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if s.Peers == nil {
		mk.Map(&s.Peers, 0)
	}
	clear(s.Peers)
	for _, peer := range s.NetMap.Peers {
		s.Peers[peer.StableID()] = peer
	}

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
		peer, _ := s.NetMap.PeerByTailscaleIP(addr)
		return peer
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

func (s *IPNStatus) SelfAddr() netip.Addr {
	if s.NetMap == nil {
		return netip.Addr{}
	}

	addrs := s.NetMap.SelfNode.Addresses()
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
}

func (*FileStatus) status() {}

type ProfileStatus struct {
	Profile  ipn.LoginProfile
	Profiles []ipn.LoginProfile
}

func (*ProfileStatus) status() {}

type notifier struct {
	notify chan struct{}
	next   *notifier
}

func newNotifier() *notifier {
	return &notifier{
		notify: make(chan struct{}),
	}
}

func (n *notifier) Notify() *notifier {
	n.next = newNotifier()
	close(n.notify)
	return n.next
}
