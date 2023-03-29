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
	New func(Status)

	ts *Client

	poll chan struct{}
	get  chan Status
}

func NewPoller(ts *Client) *Poller {
	return &Poller{
		ts: ts,

		poll: make(chan struct{}),
		get:  make(chan Status),
	}
}

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

func (p *Poller) Poll() chan<- struct{} {
	return p.poll
}

func (p *Poller) Get() <-chan Status {
	return p.get
}

type Status struct {
	Status *ipnstate.Status
	Prefs  *ipn.Prefs
}

func (s Status) Online() bool {
	return s.Status != nil
}
