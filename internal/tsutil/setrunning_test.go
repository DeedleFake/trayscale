package tsutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPollerSetRunning checks that the poller doesn't poll while it's
// starting Tailscale, reports the error, and polls again right after.
func TestPollerSetRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errTest := errors.New("test")
	started := make(chan bool)
	release := make(chan struct{})
	errs := make(chan error, 1)
	p := &Poller{
		Interval: time.Hour,
		New:      func(Status) {},
		Error:    func(err error) { errs <- err },
		setRunning: func(ctx context.Context, up bool) error {
			started <- up
			<-release
			return errTest
		},
	}
	go p.Run(ctx)

	p.SetRunning() <- true
	require.True(t, <-started)
	select {
	case <-p.Poll():
		t.Fatal("polled while starting")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	require.ErrorIs(t, <-errs, errTest)
	select {
	case <-p.Poll():
	case <-time.After(time.Second):
		t.Fatal("didn't poll after starting")
	}
}
