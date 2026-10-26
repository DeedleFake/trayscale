package tsutil

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/ipn"
)

// fakeDaemon serves the LocalAPI calls that Bus, Files, and Profiles
// make. The bus sends whatever is sent to notes.
type fakeDaemon struct {
	notes chan ipn.Notify

	files    atomic.Int32
	profiles atomic.Int32

	// If holdFiles is set, that files fetch, counting from 1, waits
	// for release to be closed.
	holdFiles atomic.Int32
	release   chan struct{}
}

func startFakeDaemon(t *testing.T) *fakeDaemon {
	d := &fakeDaemon{notes: make(chan ipn.Notify), release: make(chan struct{})}
	l := listenLocalAPI(t)
	srv := &http.Server{Handler: http.HandlerFunc(d.serve)}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return d
}

func (d *fakeDaemon) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/localapi/v0/watch-ipn-bus":
		enc := json.NewEncoder(w)
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case n := <-d.notes:
				enc.Encode(n)
				w.(http.Flusher).Flush()
			}
		}
	case "/localapi/v0/files/":
		if d.files.Add(1) == d.holdFiles.Load() {
			<-d.release
		}
		w.Write([]byte("[]"))
	case "/localapi/v0/profiles/current":
		d.profiles.Add(1)
		w.Write([]byte("{}"))
	case "/localapi/v0/profiles/":
		w.Write([]byte("[]"))
	case "/localapi/v0/file-targets":
		w.Write([]byte("[]"))
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// run starts a Bus, Files, and Profiles against the fake daemon. It
// returns a channel that gets each published status.
func (d *fakeDaemon) run(t *testing.T) (*Files, *Profiles, <-chan *IPNStatus) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	statuses := make(chan *IPNStatus, 16)
	bus := &Bus{New: func(s *IPNStatus) { statuses <- s }}
	files := &Files{Bus: bus, New: func(*FileStatus) {}}
	profiles := &Profiles{Bus: bus, New: func(*ProfileStatus) {}}
	go bus.Run(ctx)
	go files.Run(ctx)
	go profiles.Run(ctx)
	return files, profiles, statuses
}

// settle waits until the fetch counts stop changing.
func (d *fakeDaemon) settle() (files, profiles int32) {
	files, profiles = d.files.Load(), d.profiles.Load()
	for {
		time.Sleep(200 * time.Millisecond)
		f, p := d.files.Load(), d.profiles.Load()
		if f == files && p == profiles {
			return files, profiles
		}
		files, profiles = f, p
	}
}

func waitState(t *testing.T, statuses <-chan *IPNStatus, state ipn.State) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case s := <-statuses:
			if s.State == state {
				return
			}
		case <-timeout:
			t.Fatalf("no %v status", state)
		}
	}
}

// TestSourcesIgnoreEngineUpdates checks that tailscaled's engine
// updates, which it sends about every 2 seconds, don't cause fetches.
func TestSourcesIgnoreEngineUpdates(t *testing.T) {
	d := startFakeDaemon(t)
	_, _, statuses := d.run(t)

	running := ipn.Running
	d.notes <- ipn.Notify{State: &running}
	waitState(t, statuses, ipn.Running)
	files, profiles := d.settle()

	for range 20 {
		d.notes <- ipn.Notify{Engine: &ipn.EngineStatus{}}
	}
	if f, p := d.settle(); f != files || p != profiles {
		t.Fatalf("engine updates caused %v files and %v profiles fetches", f-files, p-profiles)
	}

	stopped := ipn.Stopped
	d.notes <- ipn.Notify{State: &stopped}
	waitState(t, statuses, ipn.Stopped)
	if f, p := d.settle(); f != files+1 || p != profiles+1 {
		t.Fatalf("state change caused %v files and %v profiles fetches, not 1 each", f-files, p-profiles)
	}
}

// TestFilesFetchCoalesces checks that triggers that arrive during a
// fetch cause exactly one more fetch.
func TestFilesFetchCoalesces(t *testing.T) {
	bus := func(d *fakeDaemon, statuses <-chan *IPNStatus) {
		d.notes <- filesWaiting(ipn.Notify{})
		d.notes <- ipn.Notify{}
		stopped := ipn.Stopped
		d.notes <- filesWaiting(ipn.Notify{State: &stopped})
		// Published only after everything before it fired.
		waitState(t, statuses, ipn.Stopped)
	}
	refresh := func(files *Files) {
		for range 5 {
			files.Refresh()
		}
	}

	for _, tt := range []struct {
		name    string
		trigger func(*fakeDaemon, *Files, <-chan *IPNStatus)
	}{
		{name: "bus", trigger: func(d *fakeDaemon, _ *Files, statuses <-chan *IPNStatus) { bus(d, statuses) }},
		{name: "refresh", trigger: func(_ *fakeDaemon, files *Files, _ <-chan *IPNStatus) { refresh(files) }},
		{name: "both", trigger: func(d *fakeDaemon, files *Files, statuses <-chan *IPNStatus) {
			bus(d, statuses)
			refresh(files)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := startFakeDaemon(t)
			files, _, statuses := d.run(t)
			before, _ := d.settle()

			d.holdFiles.Store(before + 1)
			files.Refresh()
			for d.files.Load() == before {
				time.Sleep(10 * time.Millisecond)
			}
			tt.trigger(d, files, statuses)
			close(d.release)

			if f, _ := d.settle(); f != before+2 {
				t.Fatalf("%v fetches after the held one, not 1", f-before-1)
			}
		})
	}
}

// TestFilesFirstFetchKeepsTriggers checks that triggers that arrive
// during the first fetch, before Files could be waiting for them,
// aren't lost.
func TestFilesFirstFetchKeepsTriggers(t *testing.T) {
	d := startFakeDaemon(t)
	d.holdFiles.Store(1)
	_, _, statuses := d.run(t)
	for d.files.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	stopped := ipn.Stopped
	d.notes <- ipn.Notify{State: &stopped}
	waitState(t, statuses, ipn.Stopped)
	close(d.release)

	if f, _ := d.settle(); f != 2 {
		t.Fatalf("%v fetches, not 2", f)
	}
}

// TestProfilesSwitchRefreshes checks that Switch fetches the profiles
// again even if switching fails.
func TestProfilesSwitchRefreshes(t *testing.T) {
	d := startFakeDaemon(t)
	_, profiles, _ := d.run(t)
	_, before := d.settle()

	err := profiles.Switch(t.Context(), "missing")
	if err == nil {
		t.Fatal("switching to a missing profile worked")
	}
	if _, p := d.settle(); p != before+1 {
		t.Fatalf("%v profile fetches after Switch, not 1", p-before)
	}
}
