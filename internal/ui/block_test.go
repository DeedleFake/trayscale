package ui

import (
	"context"
	"testing"
	"time"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
)

// TestActionsDontBlockMainLoop checks that actions which talk to
// tailscaled leave the main loop running while it's unreachable. The
// poller is never run, so it never has a status, as when tailscaled
// has been down since launch.
func TestActionsDontBlockMainLoop(t *testing.T) {
	requireGTK(t)

	tests := []struct {
		name string
		run  func(a *App, ctx context.Context)
	}{
		{"toggle connection", func(a *App, ctx context.Context) { a.toggleConn(ctx) }},
		{"login", func(a *App, ctx context.Context) { gutil.IdleAdd(func() { a.login(ctx) }) }},
		{"change control server", func(a *App, ctx context.Context) { gutil.IdleAdd(func() { a.showChangeControlServer(ctx) }) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			a := &App{poller: new(tsutil.Poller)}
			tt.run(a, ctx)

			// A blocked main loop can't be interrupted from Go, so fail
			// loudly instead of hanging until go test's timeout.
			watchdog := time.AfterFunc(5*time.Second, func() {
				panic(tt.name + ": main loop blocked for 5s waiting on tailscaled")
			})
			defer watchdog.Stop()

			var ticked bool
			gutil.IdleAdd(func() { ticked = true })
			iterateUntil(t, func() bool { return ticked })
		})
	}
}
