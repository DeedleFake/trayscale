package tsutil

import (
	"context"
	"time"

	"golang.org/x/exp/slog"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

// A Poller gets the latest Tailscale status at regular intervals or
// when manually triggered.
type Poller struct {
	// If non-nil, New will be called when a new status is received from
	// Tailscale. It should not be changed while Run is running.
	New func(Status)

	ts *Client

	poll chan struct{}
	get  chan Status
}

// NewPoller returns a new Poller that uses the given Client to
// interact with Tailscale.
func NewPoller(ts *Client) *Poller {
	return &Poller{
		ts: ts,

		poll: make(chan struct{}),
		get:  make(chan Status),
	}
}

// Run runs the poller. It blocks until polling is done, which is
// generally a result of the given Context being cancelled.
//
// The behavior of two calls to Run running concurrently is undefined.
// Don't do it.
func (p *Poller) Run(ctx context.Context) {
	const ticklen = 5 * time.Second
	check := time.NewTicker(ticklen)
	defer check.Stop()

	for {
		status, err := p.ts.Status(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale status", err)
			continue
		}

		prefs, err := p.ts.Prefs(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale prefs", err)
			continue
		}

		s := Status{Status: status, Prefs: prefs}
		if p.New != nil {
			// TODO: Only call this if the status changed from the previous
			// poll.
			p.New(s)
		}

	send:
		select {
		case <-ctx.Done():
			return
		case <-check.C:
		case <-p.poll:
			check.Reset(ticklen)
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
	return p.poll
}

// Get returns a channel that will yield the latest Status fetched. If
// a new Status is in the process of being fetched, it will wait for
// that to finish and then yield that.
func (p *Poller) Get() <-chan Status {
	return p.get
}

// Status is a type that wraps various status-related types that
// Tailscale provides.
type Status struct {
	Status *ipnstate.Status
	Prefs  *ipn.Prefs
}

// Online returns true if s indicates that the local node is online
// and connected to the tailnet.
func (s Status) Online() bool {
	return s.Status != nil
}
