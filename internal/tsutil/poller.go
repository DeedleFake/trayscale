package tsutil

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"os/user"
	"slices"
	"sync"
	"time"

	"deedles.dev/mk"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/feature/taildrop"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
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
	get      chan *NetStatus
	interval chan time.Duration
}

func (p *Poller) init() {
	p.once.Do(func() {
		mk.Chan(&p.poll, 0)
		mk.Chan(&p.get, 0)
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
	go p.watchStatus(ctx, n)
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
			check.Reset(interval)
		}
	}
}

func (p *Poller) watchStatus(ctx context.Context, n *notifier) {
	s := new(NetStatus)
	for {
		var status *ipnstate.Status
		var prefs *ipn.Prefs

		status, err := GetStatus(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale status", "err", err)
			goto wait
		}

		prefs, err = Prefs(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale prefs", "err", err)
			goto wait
		}

		s = &NetStatus{Status: status, Prefs: prefs}
		p.New(s)

	wait:
		select {
		case <-ctx.Done():
			return
		case p.get <- s:
			goto wait
		case <-n.notify:
			n = n.next
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

// GetNet returns a channel that yields the most recently fetched
// network status. It will block until the network status has been
// fetched successfully once.
func (p *Poller) GetNet() <-chan *NetStatus {
	p.init()

	return p.get
}

// SetInterval returns a channel that modifies the polling interval of
// a running poller. This will delay the next poll until the new
// interval has elapsed.
func (p *Poller) SetInterval() chan<- time.Duration {
	p.init()

	return p.interval
}

type Status any

type NetStatus struct {
	Status *ipnstate.Status
	Prefs  *ipn.Prefs
}

// Online returns true if s indicates that the local node is online
// and connected to the tailnet.
func (s *NetStatus) Online() bool {
	return s.Status.BackendState == ipn.Running.String()
}

func (s *NetStatus) NeedsAuth() bool {
	return s.Status.BackendState == ipn.NeedsLogin.String()
}

func (s *NetStatus) OperatorIsCurrent() bool {
	current, err := user.Current()
	if err != nil {
		slog.Error("get current user", "err", err)
		return false
	}

	return s.Prefs.OperatorUser == current.Username
}

func (s *NetStatus) SelfAddr() (netip.Addr, bool) {
	if s.Status.Self == nil {
		return netip.Addr{}, false
	}
	if len(s.Status.Self.TailscaleIPs) == 0 {
		return netip.Addr{}, false
	}

	return slices.MinFunc(s.Status.Self.TailscaleIPs, netip.Addr.Compare), true
}

type FileStatus struct {
	Files []apitype.WaitingFile
}

type ProfileStatus struct {
	Profile  ipn.LoginProfile
	Profiles []ipn.LoginProfile
}

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
