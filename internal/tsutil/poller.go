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
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/taildrop"
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
	New func(*Status)

	once     sync.Once
	poll     chan struct{}
	get      chan *Status
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

	interval := p.Interval
	if interval < 0 {
		interval = 5 * time.Second
	}
	retry := interval

	check := time.NewTicker(interval)
	defer check.Stop()

	for {
		status, err := GetStatus(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale status", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry):
				if retry < 30*time.Second {
					retry *= 2
				}
				continue
			}
		}

		prefs, err := Prefs(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale prefs", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry):
				if retry < 30*time.Second {
					retry *= 2
				}
				continue
			}
		}

		profile, profiles, err := ProfileStatus(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get profile status", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry):
				if retry < 30*time.Second {
					retry *= 2
				}
				continue
			}
		}

		retry = interval

		var files []apitype.WaitingFile
		if status.Self.HasCap(tailcfg.CapabilityFileSharing) {
			files, err = WaitingFiles(ctx)
			if err != nil && !errors.Is(err, taildrop.ErrNoTaildrop) {
				if ctx.Err() != nil {
					return
				}
				slog.Error("get waiting files", "err", err)
			}
		}

		s := &Status{Status: status, Prefs: prefs, Files: files, Profile: profile, Profiles: profiles}
		if p.New != nil {
			// TODO: Only call this if the status changed from the previous
			// poll? Is that remotely feasible?
			p.New(s)
		}

	send:
		select {
		case <-ctx.Done():
			return
		case <-check.C:
		case <-p.poll:
			check.Reset(interval)
		case interval = <-p.interval:
			check.Reset(interval)
			goto send
		case p.get <- s:
			goto send // I've never used a goto before.
		}
	}
}

// Poll returns a channel that, when sent to, causes a new status to
// be fetched from Tailscale. A send to the channel does not resolve
// until the poller begins to fetch the status, meaning that a send to
// Poll followed immediately by a receive from Get will always result
// in the new Status.
//
// Do not close the returned channel. Doing so will result in
// undefined behavior.
func (p *Poller) Poll() chan<- struct{} {
	p.init()

	return p.poll
}

// Get returns a channel that will yield the latest Status fetched. If
// a new Status is in the process of being fetched, it will wait for
// that to finish and then yield that.
func (p *Poller) Get() <-chan *Status {
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

// Status is a type that wraps various status-related types that
// Tailscale provides.
type Status struct {
	Status   *ipnstate.Status
	Prefs    *ipn.Prefs
	Files    []apitype.WaitingFile
	Profile  ipn.LoginProfile
	Profiles []ipn.LoginProfile
}

// Online returns true if s indicates that the local node is online
// and connected to the tailnet.
func (s *Status) Online() bool {
	return (s.Status != nil) && (s.Status.BackendState == ipn.Running.String())
}

func (s *Status) NeedsAuth() bool {
	return (s.Status != nil) && (s.Status.BackendState == ipn.NeedsLogin.String())
}

func (s *Status) OperatorIsCurrent() bool {
	current, err := user.Current()
	if err != nil {
		slog.Error("get current user", "err", err)
		return false
	}

	return s.Prefs.OperatorUser == current.Username
}

func (s *Status) SelfAddr() (netip.Addr, bool) {
	if s.Status == nil {
		return netip.Addr{}, false
	}
	if s.Status.Self == nil {
		return netip.Addr{}, false
	}
	if len(s.Status.Self.TailscaleIPs) == 0 {
		return netip.Addr{}, false
	}

	return slices.MinFunc(s.Status.Self.TailscaleIPs, netip.Addr.Compare), true
}
