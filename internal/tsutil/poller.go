package tsutil

import (
	"context"
	"fmt"
	"sync"

	"deedles.dev/mk"
	"tailscale.com/ipn"
	"tailscale.com/types/netmap"
)

// A Poller gets the latest Tailscale status at regular intervals or
// when manually triggered.
//
// A zero-value of a Poller is ready to use.
//
// It is a race condition to change any exported fields of Poller
// while Run is running.
type Poller struct {
	// TS is the Client to use to interact with Tailscale.
	//
	// If it is nil, a default client will be used.
	TS *Client

	// If non-nil, New will be called when a new status is received from
	// Tailscale.
	New func(Status)

	once sync.Once
	get  chan Status
}

func (p *Poller) init() {
	p.once.Do(func() {
		mk.Chan(&p.get, 0)
	})
}

func (p *Poller) client() *Client {
	if p.TS == nil {
		return &defaultClient
	}
	return p.TS
}

// Run runs the poller. It blocks until polling is done, which is
// generally a result of the given Context being cancelled.
//
// The behavior of two calls to Run running concurrently is undefined.
// Don't do it.
func (p *Poller) Run(ctx context.Context) error {
	p.init()

	w, err := p.TS.Watch(ctx)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	defer w.Close()

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	status := make(chan *ipn.Notify)
	go func() {
		for {
			n, err := w.Next()
			if err != nil {
				cancel(fmt.Errorf("next notification: %w", err))
				return
			}

			select {
			case <-ctx.Done():
				return
			case status <- &n:
			}
		}
	}()

	var latest Status
	var get chan Status
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case s := <-status:
			latest.update(s)
			if p.New != nil {
				p.New(latest)
			}
			get = p.get
		case get <- latest:
		}
	}
}

// Get returns a channel that will yield the latest Status fetched. If
// a new Status is in the process of being fetched, it will wait for
// that to finish and then yield that.
func (p *Poller) Get() <-chan Status {
	p.init()

	return p.get
}

// Status is a type that wraps various status-related types that
// Tailscale provides.
type Status struct {
	State  ipn.State
	Prefs  ipn.PrefsView
	NetMap netmap.NetworkMap
}

func (s *Status) update(n *ipn.Notify) {
	if n.State != nil {
		s.State = *n.State
	}
	if (n.Prefs != nil) && n.Prefs.Valid() {
		s.Prefs = *n.Prefs
	}
	if n.NetMap != nil {
		s.NetMap = *n.NetMap
	}
}

// Online returns true if s indicates that the local node is online
// and connected to the tailnet.
func (s Status) Online() bool {
	return s.State == ipn.Running
}

func (s Status) NeedsAuth() bool {
	return s.State == ipn.NeedsLogin
}
