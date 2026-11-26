package tsutil

import (
	"cmp"
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/util/set"
)

// Files keeps the list of Taildrop files waiting to be saved and
// pushes it to New each time it changes. The IPN bus only reports
// waiting files with fields that are deprecated, so Files asks
// tailscaled for the list instead.
//
// While no files are waiting, it asks tailscaled to answer as soon as
// one is, for up to Wait at a time. While some are, tailscaled can't
// say when that changes, so it checks every Interval. It checks right
// away after Delete and when Refresh is called, and only while Bus's
// latest status is online.
//
// LocalAPI lists, opens, and deletes the current profile's files by
// name and never says whose they are, so Files asks for the current
// profile around each of those calls. Each list carries its profile,
// and Open and Delete only act on the profile they're given.
//
// Set the exported fields before calling Run and don't change them
// afterwards.
type Files struct {
	// Bus says when Tailscale is online and when the list could belong
	// to another login. Required. Run doesn't run it.
	Bus *Bus

	// New is called with each list that differs from the last one, and
	// with the first one after a reset, from one goroutine. Required.
	New func(*FileStatus)

	// Interval is how often to check the list while files are waiting.
	// Zero means 5 seconds.
	Interval time.Duration

	// Wait is the longest that one check waits for a file to arrive
	// while none are waiting, rounded down to seconds. Zero means a
	// minute.
	Wait time.Duration

	once    sync.Once
	refresh chan struct{}
}

// filesResets are the signals after which the list could belong to
// another login or come from another tailscaled, as when switching
// profiles, so that the next list isn't compared to the last.
const filesResets = changeConnected | changeState | changeIdentity

func (f *Files) init() {
	f.once.Do(func() { f.refresh = make(chan struct{}, 1) })
}

// Refresh asks Files to check the list again. It never blocks.
func (f *Files) Refresh() {
	f.init()
	kick(f.refresh)
}

// ErrProfileChanged means that another profile became current, so a
// file of the profile asked for can't be reached.
var ErrProfileChanged = errors.New("the current profile changed")

// checkProfile returns ErrProfileChanged if profile isn't current.
func checkProfile(ctx context.Context, profile ipn.ProfileID) error {
	cur, err := currentProfile(ctx)
	if err != nil {
		return err
	}
	if cur != profile {
		return ErrProfileChanged
	}
	return nil
}

// Open opens profile's waiting file name. It fails with
// ErrProfileChanged if profile isn't current before or after opening
// it, so the file read is profile's unless profiles switched away and
// back while it opened.
func (f *Files) Open(ctx context.Context, profile ipn.ProfileID, name string) (io.ReadCloser, int64, error) {
	if err := checkProfile(ctx, profile); err != nil {
		return nil, 0, err
	}
	r, size, err := getWaitingFile(ctx, name)
	if err != nil {
		return nil, 0, err
	}
	if err := checkProfile(ctx, profile); err != nil {
		r.Close()
		return nil, 0, err
	}
	return r, size, nil
}

// Delete deletes profile's waiting file name and then checks the list
// again, even if deleting failed, as that most likely means the list
// was stale. It fails with ErrProfileChanged, deleting nothing, if
// profile isn't current.
//
// LocalAPI only deletes by name from the current profile, so a switch
// between the check and the delete still deletes the new profile's
// file of that name. Nothing on Trayscale's side can close that.
func (f *Files) Delete(ctx context.Context, profile ipn.ProfileID, name string) error {
	defer f.Refresh()
	if err := checkProfile(ctx, profile); err != nil {
		return err
	}
	return deleteWaitingFile(ctx, name)
}

// Run checks the list until ctx is done and returns ctx's error.
func (f *Files) Run(ctx context.Context) error {
	f.init()
	r := filesRun{
		f:        f,
		s:        f.Bus.signals(),
		interval: cmp.Or(f.Interval, 5*time.Second),
		wait:     cmp.Or(f.Wait, time.Minute),
		seen:     make(map[ipn.ProfileID][]apitype.WaitingFile),
	}
	return r.run(ctx)
}

// filesRun is the state of one call to Files.Run.
type filesRun struct {
	f        *Files
	s        *busSignal // the next signal to look at
	interval time.Duration
	wait     time.Duration

	state    ipn.State             // the state of the latest status looked at
	profile  ipn.ProfileID         // the profile last is from
	last     []apitype.WaitingFile // the last list passed to New
	baseline bool                  // whether last was passed to New since the last reset
	failures int                   // checks failed in a row

	// seen is the last list passed to New for each profile. Resets
	// keep it, so the first list after one reports the files that
	// arrived since, such as one in a list that a reset dropped.
	seen map[ipn.ProfileID][]apitype.WaitingFile
}

type filesResult struct {
	profile ipn.ProfileID
	files   []apitype.WaitingFile
	err     error
}

// filesEvent is what ended a wait in filesRun.next.
type filesEvent int

const (
	filesDone    filesEvent = iota // ctx is done
	filesReset                     // the list could belong to another login
	filesRefresh                   // Refresh was called
	filesTimer                     // the timer fired
	filesFetched                   // the check finished
)

func (r *filesRun) run(ctx context.Context) error {
	for {
		if !r.online() {
			r.reset()
			if ev, _ := r.next(ctx, nil, nil); ev == filesDone {
				return ctx.Err()
			}
			continue
		}

		wait := time.Duration(0)
		if r.baseline && len(r.last) == 0 {
			// Nothing new to say until a file arrives.
			wait = r.wait
		}
		res, again, ev := r.check(ctx, wait)
		switch ev {
		case filesDone:
			return ctx.Err()
		case filesReset:
			continue
		}
		// A reset that fired while checking means that the list could
		// be from another login, which the bus can say a little after
		// tailscaled switched.
		if r.drain() {
			continue
		}

		var timer <-chan time.Time
		switch {
		case errors.Is(res.err, ErrProfileChanged):
			// The list could be either profile's. Ask again.
			r.reset()
			continue

		case res.err == nil:
			r.failures = 0
			if r.baseline && res.profile != r.profile {
				// Switched before the bus said so.
				r.reset()
			}
			r.push(res.profile, res.files)
			if len(r.last) == 0 {
				continue
			}
			timer = time.After(r.interval)

		case isNoTaildrop(res.err):
			// There's nowhere to keep files, so none can arrive until
			// something changes, and asking again would fail at once.
			r.failures = 0
			r.push("", nil)

		default:
			r.failures++
			if r.failures == 1 {
				// Logged once until a check works again.
				slog.Error("get waiting files", "err", res.err)
			}
			if d, ok := retryDelay(r.failures); ok {
				timer = time.After(d)
			}
		}
		if again {
			continue
		}
		if ev, _ := r.next(ctx, timer, nil); ev == filesDone {
			return ctx.Err()
		}
	}
}

// check gets the list, waiting up to wait for a file to arrive if none
// are waiting. It stops early if the bus resets the list or ctx is
// done. again reports whether Refresh was called while it ran.
func (r *filesRun) check(ctx context.Context, wait time.Duration) (res filesResult, again bool, ev filesEvent) {
	checkCtx, cancel := context.WithTimeout(ctx, wait+30*time.Second)
	defer cancel()

	done := make(chan filesResult, 1)
	go func() {
		profile, files, err := profileWaitingFiles(checkCtx, wait)
		done <- filesResult{profile: profile, files: files, err: err}
	}()

	for {
		ev, res := r.next(ctx, nil, done)
		switch ev {
		case filesFetched:
			return res, again, ev
		case filesRefresh:
			// A check that's waiting for a file would say nothing new
			// unless one arrives, so only one that isn't is repeated.
			again = again || wait == 0
			continue
		}
		cancel()
		<-done
		return filesResult{}, again, ev
	}
}

// profileWaitingFiles gets the waiting files and the profile they're
// from. It fails with ErrProfileChanged if the current profile changed
// while it asked, as the list could then be either profile's.
func profileWaitingFiles(ctx context.Context, wait time.Duration) (ipn.ProfileID, []apitype.WaitingFile, error) {
	before, err := currentProfile(ctx)
	if err != nil {
		return "", nil, err
	}
	files, err := waitingFiles(ctx, wait)
	if err != nil {
		return "", nil, err
	}
	after, err := currentProfile(ctx)
	if err != nil {
		return "", nil, err
	}
	if after != before {
		return "", nil, ErrProfileChanged
	}
	return after, files, nil
}

// next waits for ctx to be done, the bus to reset the list, Refresh,
// timer to fire, or a result on done, which it returns. Bus signals
// that don't reset the list don't end the wait. A nil timer or done
// never fires.
func (r *filesRun) next(ctx context.Context, timer <-chan time.Time, done <-chan filesResult) (filesEvent, filesResult) {
	for {
		select {
		case <-ctx.Done():
			return filesDone, filesResult{}
		case <-r.s.done:
			if r.drain() {
				return filesReset, filesResult{}
			}
		case <-r.f.refresh:
			return filesRefresh, filesResult{}
		case <-timer:
			return filesTimer, filesResult{}
		case res := <-done:
			return filesFetched, res
		}
	}
}

// drain takes the signals that have already fired and reports whether
// any of them reset the list, resetting it if so.
func (r *filesRun) drain() bool {
	var c change
	for {
		select {
		case <-r.s.done:
			c |= r.s.c
			r.s = r.s.next
			continue
		default:
		}
		break
	}

	state := ipn.NoState
	if latest := r.f.Bus.latest.Load(); latest != nil {
		state = latest.State
	}
	reset := c&filesResets != 0 || state != r.state
	r.state = state
	if reset {
		r.reset()
	}
	return reset
}

// reset forgets the last list, so that the next one is passed to New
// and compared only with the last one seen for its profile.
func (r *filesRun) reset() {
	r.profile = ""
	r.last = nil
	r.baseline = false
	r.failures = 0
}

func (r *filesRun) online() bool {
	latest := r.f.Bus.latest.Load()
	return latest != nil && latest.Online()
}

// push passes profile's files to New if they're the first since a
// reset or they differ from the last list. The first list after a
// reset reports what arrived since the last list seen for profile, or
// nothing if there was none, so that files that were already waiting
// when Files started aren't reported as new.
func (r *filesRun) push(profile ipn.ProfileID, files []apitype.WaitingFile) {
	if !r.baseline {
		var arrived []string
		if prev, ok := r.seen[profile]; ok {
			arrived = arrivedFiles(prev, files)
		}
		r.f.New(&FileStatus{Profile: profile, Files: files, Arrived: arrived})
		r.profile, r.last, r.baseline = profile, files, true
		r.remember(profile, files)
		return
	}
	if slices.Equal(files, r.last) {
		return
	}
	r.f.New(&FileStatus{Profile: profile, Files: files, Arrived: arrivedFiles(r.last, files)})
	r.profile, r.last = profile, files
	r.remember(profile, files)
}

// remember keeps files as the last list seen for profile. A list with
// no profile, as when Taildrop is off, isn't kept.
func (r *filesRun) remember(profile ipn.ProfileID, files []apitype.WaitingFile) {
	if profile != "" {
		r.seen[profile] = files
	}
}

// arrivedFiles returns the names of the files in cur that aren't in
// prev, or that are with another size.
func arrivedFiles(prev, cur []apitype.WaitingFile) []string {
	old := set.Of(prev...)
	var arrived []string
	for _, f := range cur {
		if !old.Contains(f) {
			arrived = append(arrived, f.Name)
		}
	}
	return arrived
}
