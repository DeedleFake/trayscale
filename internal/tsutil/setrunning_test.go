package tsutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn"
)

// TestBusSetRunning checks that the bus doesn't publish while it's
// starting Tailscale, returns the error, and publishes the last status
// again afterward so that the UI can undo what it did.
func TestBusSetRunning(t *testing.T) {
	d := startFakeDaemon(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errTest := errors.New("test")
	started := make(chan bool)
	release := make(chan struct{})
	statuses := make(chan *IPNStatus, 16)
	b := &Bus{
		New: func(s *IPNStatus) { statuses <- s },
		setRunning: func(ctx context.Context, up bool) error {
			started <- up
			<-release
			return errTest
		},
	}
	go b.Run(ctx)

	stopped := ipn.Stopped
	d.notes <- ipn.Notify{State: &stopped}
	waitState(t, statuses, ipn.Stopped)

	errc := make(chan error, 1)
	go func() { errc <- b.SetRunning(ctx, true) }()
	require.True(t, <-started)
	select {
	case s := <-statuses:
		t.Fatalf("published %v while starting", s.State)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	require.ErrorIs(t, <-errc, errTest)
	waitState(t, statuses, ipn.Stopped)
}
