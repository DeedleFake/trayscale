package tsutil

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"tailscale.com/feature/taildrop"
	"tailscale.com/ipn"
)

// Files keeps the list of Taildrop files waiting to be saved. It
// fetches the list when Bus reports a reconnect, a state change, or a
// finished transfer, after Delete, and when Refresh is called.
// Nothing reports a waiting file deleted outside of Trayscale while
// others are still waiting, so call Refresh before showing the list.
//
// Set the exported fields before calling Run and don't change them
// afterwards.
type Files struct {
	// Bus is where the triggers come from. Required. Run doesn't run
	// it.
	Bus *Bus

	// New is called with each fetched list, from one goroutine.
	// Required.
	New func(*FileStatus)

	once    sync.Once
	refresh chan struct{}
}

const filesTriggers = changeConnected | changeState | changeFiles

func (f *Files) init() {
	f.once.Do(func() { f.refresh = make(chan struct{}, 1) })
}

// Run fetches the list until ctx is done and returns ctx's error.
func (f *Files) Run(ctx context.Context) error {
	f.init()
	var arrived []string
	return fetchLoop(ctx, f.Bus, filesTriggers, f.refresh, "get waiting files", func(ctx context.Context) error {
		// Taken before the list, so that every file in it is in the
		// list unless it's already gone.
		arrived = append(arrived, f.Bus.takeArrived()...)
		files, err := waitingFiles(ctx)
		if err != nil && !errors.Is(err, taildrop.ErrNoTaildrop) {
			return err
		}
		f.New(&FileStatus{Files: files, Arrived: arrived})
		arrived = nil
		return nil
	})
}

// Refresh asks Files to fetch the list again. It never blocks.
func (f *Files) Refresh() {
	f.init()
	kick(f.refresh)
}

// Delete deletes a waiting file and then fetches the list again, even
// if deleting failed, as that most likely means the list was stale.
func (f *Files) Delete(ctx context.Context, name string) error {
	defer f.Refresh()
	return deleteWaitingFile(ctx, name)
}

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
// fetch started.
func fetchLoop(ctx context.Context, bus *Bus, mask change, refresh chan struct{}, what string, fetch func(context.Context) error) error {
	// Taken before the first fetch so that a signal fired during it
	// causes another fetch instead of being missed.
	s := bus.signals()
	failing := false
	for {
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := fetch(fetchCtx)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch {
		case err != nil && !failing:
			// Logged once until a fetch works again, as every fetch
			// fails while tailscaled is down.
			slog.Error(what, "err", err)
			failing = true
		case err == nil:
			failing = false
		}

		var ok bool
		s, ok = await(ctx, s, mask, refresh)
		if !ok {
			return ctx.Err()
		}
	}
}

// await waits until a signal in mask fires or refresh is received,
// and then takes everything else that already fired or was sent, so
// that one return means one fetch. It returns the link to wait on
// next.
func await(ctx context.Context, s *busSignal, mask change, refresh chan struct{}) (*busSignal, bool) {
	var got change
	for got&mask == 0 {
		select {
		case <-ctx.Done():
			return s, false
		case <-refresh:
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
