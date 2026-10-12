package ui

import (
	"context"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
)

// TestStatusSwitchUpdates checks that status updates with the old
// state neither stop the switch's toggle animation partway nor move it
// back while Tailscale is starting.
func TestStatusSwitchUpdates(t *testing.T) {
	requireGTK(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	w := gtk.NewWindow()
	defer w.Destroy()
	win := &MainWindow{app: &App{poller: new(tsutil.Poller)}, StatusSwitch: gtk.NewSwitch()}
	sw := win.StatusSwitch
	gutil.Connect(sw, sw.ConnectStateSet, func(_ gtk.Switch, s bool) bool {
		if s == sw.GetState() {
			return false
		}
		win.setStatus(ctx, s)
		return true
	})
	w.SetChild(&sw.Widget)
	w.Present()
	iterateFor(500 * time.Millisecond)

	sw.Activate() // Starts the toggle animation, as a click does.
	iterateFor(40 * time.Millisecond)
	win.updateStatusSwitch(false)
	iterateFor(300 * time.Millisecond)
	if !sw.GetActive() {
		t.Fatal("update stopped the toggle animation")
	}

	win.updateStatusSwitch(false)
	iterateFor(100 * time.Millisecond)
	if !sw.GetActive() {
		t.Fatal("update turned the switch back off while starting")
	}
}

// iterateFor runs the default main context for d.
func iterateFor(d time.Duration) {
	ctx := glib.MainContextDefault()
	for end := time.Now().Add(d); time.Now().Before(end); {
		if !ctx.Iteration(false) {
			time.Sleep(time.Millisecond)
		}
	}
}
