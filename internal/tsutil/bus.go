package tsutil

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
	"tailscale.com/util/set"
)

// A Bus follows tailscaled's IPN bus and publishes what it says. It's
// the only reader of the bus in Trayscale. It owns the connection,
// reconnects when it drops, and tells Files and Profiles when the bus
// said something that could change their data.
//
// Set the exported fields before calling Run and don't change them
// afterwards.
type Bus struct {
	// New is called with each new status, in order, from a single
	// goroutine. Required.
	New func(*IPNStatus)

	// NewEngine, if non-nil, is called with each engine update.
	// tailscaled sends these about every 2 seconds while it's running.
	// It's called from the same goroutine as New.
	NewEngine func(*EngineStatus)

	once    sync.Once
	head    atomic.Pointer[busSignal] // the next signal to fire
	latest  atomic.Pointer[IPNStatus] // the last status passed to New
	ready   chan struct{}             // closed when latest is first set
	holds   chan int
	resend  chan struct{}
	stopped chan struct{}
	runMu   sync.Mutex

	// setRunning is a test seam. Only tests set it, to fake starting
	// and stopping Tailscale. It's nil in normal use, in which case
	// SetRunning calls Start and Stop.
	setRunning func(context.Context, bool) error
}

func (b *Bus) init() {
	b.once.Do(func() {
		b.head.Store(newBusSignal())
		b.ready = make(chan struct{})
		b.holds = make(chan int)
		b.resend = make(chan struct{}, 1)
		b.stopped = make(chan struct{})
	})
}

// EngineStatus is tailscaled's live per-peer connection data.
type EngineStatus struct {
	LivePeers map[key.NodePublic]ipnstate.PeerStatusLite
}

func (*EngineStatus) status() {}

// busEvent is what one notify, or the connection itself, produced.
type busEvent struct {
	status  *IPNStatus // to publish, if non-nil
	engine  *EngineStatus
	changes change
}

// Run follows the bus until ctx is done, reconnecting every 5 seconds
// while tailscaled is unreachable. It returns ctx's error.
//
// The behavior of two calls to Run running concurrently is undefined.
func (b *Bus) Run(ctx context.Context) error {
	b.init()
	defer close(b.stopped)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	events := make(chan busEvent)
	go b.watch(ctx, events)

	var pending *IPNStatus
	holds := 0
	for {
		if pending != nil && holds == 0 {
			b.publish(pending)
			pending = nil
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if ev.engine != nil && b.NewEngine != nil {
				b.NewEngine(ev.engine)
			}
			if ev.status != nil {
				pending = ev.status
			}
			// Signals aren't held while starting or stopping. Only
			// publishing is.
			if ev.changes != 0 {
				b.fire(ev.changes)
			}
		case d := <-b.holds:
			holds += d
		case <-b.resend:
			if latest := b.latest.Load(); pending == nil && holds == 0 && latest != nil {
				b.New(latest)
			}
		}
	}
}

func (b *Bus) publish(s *IPNStatus) {
	first := b.latest.Swap(s) == nil
	b.New(s)
	if first {
		close(b.ready)
	}
	b.fire(changePublished)
}

// watch follows the bus, one connection at a time, and sends what it
// says to events.
func (b *Bus) watch(ctx context.Context, events chan<- busEvent) {
	send := func(ev busEvent) bool {
		select {
		case <-ctx.Done():
			return false
		case events <- ev:
			return true
		}
	}

	down := false
	for {
		connected, err := b.watchOnce(ctx, send)
		if ctx.Err() != nil {
			return
		}
		slog.Error("IPN bus watcher stopped", "err", err)
		if connected || !down {
			// tailscaled is gone or unreachable, so its last status no
			// longer applies.
			down = true
			if !send(busEvent{status: IPNStatus{State: ipn.NoState}.copy()}) {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// watchOnce follows one bus connection until it fails. connected
// reports whether the connection was ever made.
func (b *Bus) watchOnce(ctx context.Context, send func(busEvent) bool) (connected bool, err error) {
	const watcherOpts = ipn.NotifyInitialState |
		ipn.NotifyInitialPrefs |
		ipn.NotifyWatchEngineUpdates |
		ipn.NotifyInitialStatus |
		ipn.NotifyPeerChanges

	watcher, err := localClient.WatchIPNBus(ctx, watcherOpts)
	if err != nil {
		return false, err
	}
	defer watcher.Close()

	if !send(busEvent{changes: changeConnected}) {
		return true, ctx.Err()
	}

	var w watchState
	for {
		notify, err := watcher.Next()
		if err != nil {
			// Any error ends the stream; Next would keep failing.
			return true, err
		}

		if notify.ErrMessage != nil {
			var state ipn.State
			if notify.State != nil {
				state = *notify.State
			}
			slog.Error("watcher got error message", "state", state, "err", notify.ErrMessage)
		}

		c := w.apply(&notify)
		if c&changeTargets != 0 {
			w.ipn.refreshFileTargets(ctx)
		}

		// TODO: Handle health warnings.
		ev := busEvent{changes: c &^ (changeIPN | changeTargets)}
		if c&changeIPN != 0 {
			ev.status = w.ipn.copy()
		}
		if notify.Engine != nil {
			ev.engine = &EngineStatus{LivePeers: notify.Engine.LivePeers}
		}
		if ev == (busEvent{}) {
			continue
		}
		if !send(ev) {
			return true, ctx.Err()
		}
	}
}

// Latest returns the last status passed to New, waiting for the
// first one if there hasn't been one yet.
func (b *Bus) Latest(ctx context.Context) (*IPNStatus, error) {
	b.init()
	if s := b.latest.Load(); s != nil {
		return s, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.ready:
		return b.latest.Load(), nil
	}
}

// Next waits for the next status passed to New after Next is called.
func (b *Bus) Next(ctx context.Context) (*IPNStatus, error) {
	s := b.signals()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.done:
			if s.c&changePublished != 0 {
				return b.latest.Load(), nil
			}
			s = s.next
		}
	}
}

// SetRunning starts Tailscale if up is true and stops it otherwise.
// It blocks until that's done, so don't call it from the main thread.
// Calls run one at a time.
//
// While it runs, Bus keeps reading the bus but doesn't pass statuses
// to New, so that the UI sees the state that tailscaled settles on
// instead of the steps on the way. If it fails, the last status is
// passed to New again so that the UI can undo whatever it did in
// anticipation.
func (b *Bus) SetRunning(ctx context.Context, up bool) error {
	b.init()

	b.runMu.Lock()
	defer b.runMu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.stopped:
		return context.Canceled
	case b.holds <- 1:
	}

	err := b.run(ctx, up)

	// Not ctx, as the hold has to be released even if it's done.
	select {
	case <-b.stopped:
	case b.holds <- -1:
	}

	if err != nil {
		select {
		case b.resend <- struct{}{}:
		default:
		}
	}
	return err
}

func (b *Bus) run(ctx context.Context, up bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if b.setRunning != nil {
		return b.setRunning(ctx, up)
	}
	if up {
		return Start(ctx)
	}
	return Stop(ctx)
}

// change says what a bus notify could have changed. watchState.apply
// decides it for each notify, and each source decides which changes
// it cares about.
type change uint16

const (
	// changeConnected: the bus connection was just made or remade.
	changeConnected change = 1 << iota

	// changeState: the IPN state differs from the previous one.
	// tailscaled resets the state when switching, adding, logging out
	// of, or deleting the current profile.
	changeState

	// changeIdentity: something that names the current login changed:
	// a finished login, the profile name or control URL in the prefs,
	// or the self node's identity.
	changeIdentity

	// changeFiles: a Taildrop transfer finished, or files started or
	// stopped waiting.
	changeFiles

	// changePublished: Bus passed a new status to New.
	changePublished

	// changeIPN: the status has something new to publish. Only Bus
	// uses it.
	changeIPN

	// changeTargets: Taildrop targets may have changed. Only Bus uses
	// it.
	changeTargets
)

// watchState is what one bus connection remembers between notifies.
type watchState struct {
	ipn IPNStatus

	// incoming holds the names of Taildrop transfers in progress.
	incoming set.Set[string]

	// filesWaiting is whether the last notify said that files are
	// waiting. tailscaled sets it on every notify while they are.
	filesWaiting bool
}

// apply folds notify into w and reports what it changed. It's the one
// place that maps bus fields to changes.
func (w *watchState) apply(notify *ipn.Notify) (c change) {
	s := &w.ipn

	if notify.State != nil {
		c |= changeIPN
		if *notify.State != s.State {
			c |= changeState
			if *notify.State == ipn.Running {
				// FileTargets fails unless the state is Running.
				c |= changeTargets
			}
		}
		s.State = *notify.State
	}
	if notify.Prefs != nil && notify.Prefs.Valid() {
		c |= changeIPN
		if s.Prefs.Valid() && (notify.Prefs.ProfileName() != s.Prefs.ProfileName() || notify.Prefs.ControlURL() != s.Prefs.ControlURL()) {
			c |= changeIdentity
		}
		s.Prefs = *notify.Prefs
	}
	if notify.LoginFinished != nil {
		c |= changeIdentity
	}
	if notify.BrowseToURL != nil {
		s.BrowseToURL = *notify.BrowseToURL
		c |= changeIPN
	}

	if notify.InitialStatus != nil {
		s.applyInitialStatus(notify.InitialStatus)
		c |= changeIPN | changeTargets
	}
	if notify.SelfChange != nil {
		c |= changeIPN
		if s.applySelfChange(notify.SelfChange) {
			c |= changeTargets | changeIdentity
		}
	}
	if len(notify.PeersChanged) != 0 {
		c |= changeIPN
		if s.applyPeersChanged(notify.PeersChanged) {
			c |= changeTargets
		}
	}
	if len(notify.PeersRemoved) != 0 {
		c |= changeIPN
		if s.applyPeersRemoved(notify.PeersRemoved) {
			c |= changeTargets
		}
	}

	if w.applyFiles(notify) {
		c |= changeFiles
	}
	return c
}

// applyFiles reports whether notify says that the waiting files may
// have changed. Nothing reports a waiting file being deleted, except
// that deleting the last one ends FilesWaiting.
func (w *watchState) applyFiles(notify *ipn.Notify) (changed bool) {
	// Both fields are deprecated in favor of AwaitWaitingFiles, but that
	// only returns once files are waiting, so it can't report changes.
	incomingFiles := notify.IncomingFiles //nolint:staticcheck
	waiting := notify.FilesWaiting != nil //nolint:staticcheck

	if incomingFiles != nil {
		incoming := make(set.Set[string], len(incomingFiles))
		for _, f := range incomingFiles {
			if f.Done {
				changed = true
				continue
			}
			incoming.Add(f.Name)
		}
		for name := range w.incoming {
			if !incoming.Contains(name) {
				// It finished or failed without being reported as done.
				changed = true
			}
		}
		w.incoming = incoming
	}

	if waiting != w.filesWaiting {
		w.filesWaiting = waiting
		changed = true
	}
	return changed
}

// busSignal is one link of a broadcast chain. Bus.fire sets c and
// next and then closes done, so readers see both after <-done. A
// reader that falls behind follows the links it missed, so none are
// lost.
type busSignal struct {
	done chan struct{}
	c    change
	next *busSignal
}

func newBusSignal() *busSignal {
	return &busSignal{done: make(chan struct{})}
}

// signals returns the link that fires next.
func (b *Bus) signals() *busSignal {
	b.init()
	return b.head.Load()
}

// fire is only called from Run's goroutine.
func (b *Bus) fire(c change) {
	s := b.head.Load()
	s.c = c
	s.next = newBusSignal()
	b.head.Store(s.next)
	close(s.done)
}
