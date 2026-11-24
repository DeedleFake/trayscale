package tsutil

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/feature/taildrop"
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

	// waiting, if set, is the files list to serve, as JSON.
	waiting atomic.Pointer[string]

	// filesErr, if set, is the error to fail files fetches with.
	filesErr atomic.Pointer[string]

	// longPolls counts the files fetches that asked to wait for a file,
	// and canceled the ones that were canceled while waiting.
	longPolls atomic.Int32
	canceled  atomic.Int32

	// failProfiles is how many more profile fetches to fail.
	failProfiles atomic.Int32

	// current, if set, is the current profile's ID. It's "pA" if not.
	current atomic.Pointer[string]

	// deleted gets the name of each deleted waiting file.
	deleted chan string
}

func startFakeDaemon(t *testing.T) *fakeDaemon {
	d := &fakeDaemon{notes: make(chan ipn.Notify), release: make(chan struct{}), deleted: make(chan string, 16)}
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
		if msg := d.filesErr.Load(); msg != nil {
			http.Error(w, *msg, http.StatusInternalServerError)
			return
		}
		if ws := r.FormValue("waitsec"); ws != "" && ws != "0" {
			// Like tailscaled, answer once files are waiting or after
			// waitsec.
			d.longPolls.Add(1)
			sec, _ := strconv.Atoi(ws)
			deadline := time.After(time.Duration(sec) * time.Second)
		wait:
			for d.waitingBody() == "[]" {
				select {
				case <-r.Context().Done():
					d.canceled.Add(1)
					return
				case <-deadline:
					break wait
				case <-time.After(5 * time.Millisecond):
				}
			}
		}
		w.Write([]byte(d.waitingBody()))
	case "/localapi/v0/profiles/current":
		json.NewEncoder(w).Encode(ipn.LoginProfile{ID: d.currentProfile()})
	case "/localapi/v0/profiles/":
		// Only ProfileStatus lists the profiles, so this counts its
		// fetches. Files only asks for the current one.
		d.profiles.Add(1)
		if d.failProfiles.Add(-1) >= 0 {
			http.Error(w, "busy", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("[]"))
	case "/localapi/v0/file-targets":
		w.Write([]byte("[]"))
	default:
		name, ok := strings.CutPrefix(r.URL.Path, "/localapi/v0/files/")
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch r.Method {
		case "DELETE":
			d.deleted <- name
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Content-Length", "4")
			w.Write([]byte("data"))
		}
	}
}

func (d *fakeDaemon) currentProfile() ipn.ProfileID {
	if p := d.current.Load(); p != nil {
		return ipn.ProfileID(*p)
	}
	return "pA"
}

// setProfile makes id the current profile.
func (d *fakeDaemon) setProfile(id ipn.ProfileID) {
	s := string(id)
	d.current.Store(&s)
}

func (d *fakeDaemon) waitingBody() string {
	if b := d.waiting.Load(); b != nil {
		return *b
	}
	return "[]"
}

// setWaiting sets the files list to serve, as JSON.
func (d *fakeDaemon) setWaiting(body string) {
	d.waiting.Store(&body)
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

	// Files doesn't check while Tailscale is offline.
	stopped := ipn.Stopped
	d.notes <- ipn.Notify{State: &stopped}
	waitState(t, statuses, ipn.Stopped)
	if f, p := d.settle(); f != files || p != profiles+1 {
		t.Fatalf("state change caused %v files and %v profiles fetches, not 0 and 1", f-files, p-profiles)
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

// TestIsNoTaildrop checks that isNoTaildrop recognizes tailscaled's
// error for having nowhere to keep files once it has come through the
// LocalAPI, and nothing else.
func TestIsNoTaildrop(t *testing.T) {
	d := startFakeDaemon(t)

	for _, tt := range []struct {
		name string
		err  string // "" to succeed
		want bool
	}{
		{name: "no error"},
		{name: "no taildrop", err: taildrop.ErrNoTaildrop.Error(), want: true},
		{name: "other", err: "file access denied"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d.filesErr.Store(nil)
			if tt.err != "" {
				d.filesErr.Store(&tt.err)
			}
			_, err := waitingFiles(t.Context(), 0)
			if (err != nil) != (tt.err != "") {
				t.Fatalf("waitingFiles err = %v", err)
			}
			if got := isNoTaildrop(err); got != tt.want {
				t.Fatalf("isNoTaildrop(%v) = %v, want %v", err, got, tt.want)
			}
		})
	}
}

func TestRetryDelay(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want time.Duration
		ok   bool
	}{
		{n: 0},
		{n: 1, want: time.Second, ok: true},
		{n: 2, want: 2 * time.Second, ok: true},
		{n: 3, want: 4 * time.Second, ok: true},
		{n: 5, want: 16 * time.Second, ok: true},
		{n: 6},
		{n: 100},
	} {
		if got, ok := retryDelay(tt.n); got != tt.want || ok != tt.ok {
			t.Errorf("retryDelay(%v) = %v, %v, want %v, %v", tt.n, got, ok, tt.want, tt.ok)
		}
	}
}

// fastRetries makes retries quick for the rest of the test.
func fastRetries(t *testing.T) {
	old := retryBase
	retryBase = 10 * time.Millisecond
	t.Cleanup(func() { retryBase = old })
}

// TestFetchRetries checks that a failed fetch is retried until it works
// and that a fetch that keeps failing is retried only so many times.
func TestFetchRetries(t *testing.T) {
	fastRetries(t)

	for _, tt := range []struct {
		name  string
		fails int32
		want  int32
	}{
		{name: "brief", fails: 2, want: 3},
		{name: "lasting", fails: 100, want: 1 + maxRetries},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := startFakeDaemon(t)
			d.failProfiles.Store(tt.fails)
			d.run(t)
			if _, p := d.settle(); p != tt.want {
				t.Fatalf("%v profile fetches, want %v", p, tt.want)
			}
		})
	}
}

// TestFetchRetryYieldsToTriggers checks that a trigger during the wait
// to retry fetches right away and that the retry it replaces doesn't
// fetch again.
func TestFetchRetryYieldsToTriggers(t *testing.T) {
	d := startFakeDaemon(t)
	_, profiles, _ := d.run(t)
	_, before := d.settle()

	d.failProfiles.Store(1)
	profiles.Refresh()
	for d.profiles.Load() == before {
		time.Sleep(10 * time.Millisecond)
	}
	profiles.Refresh()
	if _, p := d.settle(); p != before+2 {
		t.Fatalf("%v profile fetches, want 2", p-before)
	}
	time.Sleep(retryBase)
	if _, p := d.settle(); p != before+2 {
		t.Fatalf("%v profile fetches after the retry was due, want 2", p-before)
	}
}
