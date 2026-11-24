package tsutil

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"tailscale.com/ipn"
)

// Profiles keeps the current login profile and the list of profiles.
// It fetches them when Bus reports a reconnect, a state change, or an
// identity change, after Switch, and when Refresh is called. Nothing
// reports a profile that isn't the current one being deleted, so call
// Refresh before showing the list.
//
// Set the exported fields before calling Run and don't change them
// afterwards.
type Profiles struct {
	// Bus is where the triggers come from. Required. Run doesn't run
	// it.
	Bus *Bus

	// New is called with each fetched status, from one goroutine.
	// Required.
	New func(*ProfileStatus)

	once    sync.Once
	refresh chan struct{}
}

const profilesTriggers = changeConnected | changeState | changeIdentity

func (p *Profiles) init() {
	p.once.Do(func() { p.refresh = make(chan struct{}, 1) })
}

// Run fetches the profiles until ctx is done and returns ctx's error.
func (p *Profiles) Run(ctx context.Context) error {
	p.init()
	return fetchLoop(ctx, p.Bus, profilesTriggers, p.refresh, "get profile status", func(ctx context.Context) error {
		profile, profiles, err := getProfileStatus(ctx)
		if err != nil {
			return err
		}
		p.New(&ProfileStatus{Profile: profile, Profiles: profiles})
		return nil
	})
}

// Refresh asks Profiles to fetch the profiles again. It never blocks.
func (p *Profiles) Refresh() {
	p.init()
	kick(p.refresh)
}

// Switch switches to another login profile and then fetches the
// profiles again, even if switching failed, as that most likely means
// the list was stale.
func (p *Profiles) Switch(ctx context.Context, id ipn.ProfileID) error {
	defer p.Refresh()
	return switchProfile(ctx, id)
}

func kick(c chan<- struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// fetchLoop calls fetch once at the start and then once for each
// batch of signals in mask or refreshes that arrived since the last
// fetch started. A failed fetch is retried a few times with backoff, in
// case the failure was brief.
func fetchLoop(ctx context.Context, bus *Bus, mask change, refresh chan struct{}, what string, fetch func(context.Context) error) error {
	// Taken before the first fetch so that a signal fired during it
	// causes another fetch instead of being missed.
	s := bus.signals()
	failures := 0
	for {
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := fetch(fetchCtx)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var retry <-chan time.Time
		if err != nil {
			failures++
			if failures == 1 {
				// Logged once until a fetch works again, as every fetch
				// fails while tailscaled is down.
				slog.Error(what, "err", err)
			}
			if d, ok := retryDelay(failures); ok {
				retry = time.After(d)
			}
		} else {
			failures = 0
		}

		var ok bool
		s, ok = await(ctx, s, mask, refresh, retry)
		if !ok {
			return ctx.Err()
		}
	}
}

// retryBase is the delay before the first retry of a failed fetch.
var retryBase = time.Second

// maxRetries is how many times in a row a failed fetch is retried
// before waiting for something else to cause one, so that a lasting
// failure, such as being denied access, doesn't turn into polling.
const maxRetries = 5

// retryDelay returns how long to wait after the nth failure in a row
// before fetching again, doubling each time, or false if it's time to
// stop retrying.
func retryDelay(n int) (time.Duration, bool) {
	if n < 1 || n > maxRetries {
		return 0, false
	}
	return retryBase << (n - 1), true
}

// await waits until a signal in mask fires, refresh is received, or
// retry fires, and then takes everything else that already fired or was
// sent, so that one return means one fetch. It returns the link to wait
// on next. A nil retry never fires.
func await(ctx context.Context, s *busSignal, mask change, refresh chan struct{}, retry <-chan time.Time) (*busSignal, bool) {
	var got change
	for got&mask == 0 {
		select {
		case <-ctx.Done():
			return s, false
		case <-refresh:
			got |= mask
		case <-retry:
			got |= mask
		case <-s.done:
			got |= s.c
			s = s.next
		}
	}

	for {
		select {
		case <-s.done:
			s = s.next
		default:
			select {
			case <-refresh:
			default:
			}
			return s, true
		}
	}
}
