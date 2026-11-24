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

// TestBusReportsDaemonStopped checks that the bus publishes a
// not-running status when tailscaled goes away, instead of leaving the
// UI on the last status it saw.
func TestBusReportsDaemonStopped(t *testing.T) {
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
	b := &Bus{New: func(s *IPNStatus) { statuses <- s }}
	go b.Run(ctx)

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

// TestBusPublishesLatestAfterStart checks that the first status
// published after starting Tailscale is the newest one tailscaled sent
// while it was starting, not one it was holding from before, which made
// the switch flick off and back on.
func TestBusPublishesLatestAfterStart(t *testing.T) {
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
	b := &Bus{
		New: func(s *IPNStatus) { statuses <- s },
		setRunning: func(ctx context.Context, up bool) error {
			notes <- ipn.Starting
			notes <- ipn.Running
			// Give the watcher time to read both.
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}
	go b.Run(ctx)

	notes <- ipn.Stopped
	if s := <-statuses; s.State != ipn.Stopped {
		t.Fatalf("first status is %v, not Stopped", s.State)
	}

	go b.SetRunning(ctx, true)
	if s := <-statuses; s.State != ipn.Running {
		t.Fatalf("first status after starting is %v, not Running", s.State)
	}
}

var fakeSocket string

func TestMain(m *testing.M) {
	// Set before any test starts a bus, as buses from earlier
	// tests may still be dialing.
	dir, err := os.MkdirTemp("", "trayscale-test")
	if err != nil {
		panic(err)
	}
	fakeSocket = filepath.Join(dir, "tailscaled.sock")
	localClient.Socket, localClient.UseSocketOnly = fakeSocket, true
	// Each test serves the socket with its own server and closes it
	// afterward, so a pooled connection would outlive its server.
	localClient.Transport = &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return new(net.Dialer).DialContext(ctx, "unix", fakeSocket)
		},
	}
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

// TestBusDownStatusIsUsable checks that the status published when
// tailscaled stops can be read like any other, as the UI does.
func TestBusDownStatusIsUsable(t *testing.T) {
	l := listenLocalAPI(t)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	})}
	go srv.Serve(l)
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	statuses := make(chan *IPNStatus, 16)
	b := &Bus{New: func(s *IPNStatus) { statuses <- s }}
	go b.Run(ctx)

	s := <-statuses
	if s.Online() {
		t.Fatal("down status is online")
	}
	// What App.update, the self page, and the settings read.
	s.ExitNodeActive()
	s.ExitNode()
	s.OperatorIsCurrent()
	s.NeedsAuth()
	s.SelfAddr()
	s.Prefs.ControlURL()
	s.Prefs.AdvertisesExitNode()
	s.Prefs.AdvertiseRoutes()
}
