package tsutil

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tailscale.com/ipn"
)

// TestPollerReportsDaemonStopped checks that the poller publishes a
// not-running status when tailscaled goes away, instead of leaving the
// UI on the last status it saw.
func TestPollerReportsDaemonStopped(t *testing.T) {
	l := listenLocalAPI(t)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/localapi/v0/watch-ipn-bus" {
			http.NotFound(w, r)
			return
		}
		running := ipn.Running
		json.NewEncoder(w).Encode(ipn.Notify{State: &running})
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})}
	go srv.Serve(l)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	statuses := make(chan *IPNStatus, 16)
	p := &Poller{Interval: 50 * time.Millisecond, New: func(s Status) {
		if s, ok := s.(*IPNStatus); ok {
			statuses <- s
		}
	}}
	go p.Run(ctx)

	if s := <-statuses; !s.Online() {
		t.Fatalf("first status not online: %v", s.State)
	}

	srv.Close() // tailscaled stops.

	timeout := time.After(3 * time.Second)
	for {
		select {
		case s := <-statuses:
			if !s.Online() {
				return
			}
		case <-timeout:
			t.Fatal("still showing the old status 3s after tailscaled stopped")
		}
	}
}

// TestPollerPublishesLatestAfterStart checks that the first status
// published after starting Tailscale is the newest one tailscaled sent
// while it was starting, not one it was holding from before, which made
// the switch flick off and back on.
func TestPollerPublishesLatestAfterStart(t *testing.T) {
	l := listenLocalAPI(t)
	notes := make(chan ipn.State, 4)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/localapi/v0/watch-ipn-bus" {
			http.NotFound(w, r)
			return
		}
		enc := json.NewEncoder(w)
		for {
			select {
			case <-r.Context().Done():
				return
			case state := <-notes:
				enc.Encode(ipn.Notify{State: &state})
				w.(http.Flusher).Flush()
			}
		}
	})}
	go srv.Serve(l)
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	statuses := make(chan *IPNStatus, 16)
	p := &Poller{
		Interval: time.Hour,
		New: func(s Status) {
			if s, ok := s.(*IPNStatus); ok {
				statuses <- s
			}
		},
		setRunning: func(ctx context.Context, up bool) error {
			notes <- ipn.Starting
			notes <- ipn.Running
			// Give the watcher time to read both.
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}
	go p.Run(ctx)

	notes <- ipn.Stopped
	if s := <-statuses; s.State != ipn.Stopped {
		t.Fatalf("first status is %v, not Stopped", s.State)
	}

	p.SetRunning() <- true
	if s := <-statuses; s.State != ipn.Running {
		t.Fatalf("first status after starting is %v, not Running", s.State)
	}
}

var fakeSocket string

func TestMain(m *testing.M) {
	// Set before any test starts a poller, as pollers from earlier
	// tests may still be dialing.
	dir, err := os.MkdirTemp("", "trayscale-test")
	if err != nil {
		panic(err)
	}
	fakeSocket = filepath.Join(dir, "tailscaled.sock")
	localClient.Socket, localClient.UseSocketOnly = fakeSocket, true
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// listenLocalAPI listens on the socket that localClient uses.
func listenLocalAPI(t *testing.T) net.Listener {
	os.Remove(fakeSocket)
	l, err := net.Listen("unix", fakeSocket)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
