package ui

import (
	"context"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
)

// TestActionsDontBlockMainLoop checks that actions which talk to
// tailscaled leave the main loop running while it's unreachable. The
// bus is never run, so it never has a status, as when tailscaled has
// been down since launch.
func TestActionsDontBlockMainLoop(t *testing.T) {
	requireGTK(t)

	tests := []struct {
		name string
		// run starts the action. If it returns a function, that's
		// called on the main thread after ctx is cancelled to check
		// how the action handled the failure.
		run func(a *App, ctx context.Context) func(t *testing.T)
	}{
		{"toggle connection", func(a *App, ctx context.Context) func(*testing.T) { a.toggleConn(ctx); return nil }},
		{"login", func(a *App, ctx context.Context) func(*testing.T) { gutil.IdleAdd(func() { a.login(ctx) }); return nil }},
		{"change control server", func(a *App, ctx context.Context) func(*testing.T) {
			gutil.IdleAdd(func() { a.showChangeControlServer(ctx) })
			return nil
		}},
		{"status switch", func(a *App, ctx context.Context) func(*testing.T) {
			win := &MainWindow{app: a, StatusSwitch: gtk.NewSwitch()}
			// The switch is turned on before its state-set handler runs.
			win.StatusSwitch.SetActive(true)
			gutil.IdleAdd(func() { win.setStatus(ctx, true) })
			return func(t *testing.T) {
				defer win.StatusSwitch.Unref()
				iterateUntil(t, func() bool { return !win.StatusSwitch.GetActive() })
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			a := &App{bus: new(tsutil.Bus)}
			check := tt.run(a, ctx)

			// A blocked main loop can't be interrupted from Go, so fail
			// loudly instead of hanging until go test's timeout.
			watchdog := time.AfterFunc(5*time.Second, func() {
				panic(tt.name + ": main loop blocked for 5s waiting on tailscaled")
			})
			defer watchdog.Stop()

			var ticked bool
			gutil.IdleAdd(func() { ticked = true })
			iterateUntil(t, func() bool { return ticked })
			watchdog.Stop()

			if check != nil {
				cancel()
				check(t)
			}
		})
	}
}
