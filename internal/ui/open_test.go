package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

// newTestApp returns a registered app that isn't unique, so that
// notifications can be sent without a session bus.
func newTestApp(t *testing.T, id string) *adw.Application {
	t.Helper()
	app := adw.NewApplication("dev.deedles.Trayscale.test."+id, gio.GApplicationNonUniqueValue)
	t.Cleanup(app.Unref)
	if _, err := app.Register(nil); gutil.Err(err) != nil {
		t.Fatal(err)
	}
	return app
}

// openedFile returns a file as GApplication::open passes it, owned by
// the caller, and counts its finalization into *finalized.
func openedFile(t *testing.T, finalized *int) *gio.FileBase {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := gio.FileNewForPath(path)
	track(finalized, file)
	return file
}

// hasDialog reports whether a dialog is open.
func hasDialog() bool {
	windows := gtk.WindowGetToplevels()
	defer gutil.Obj(windows).Unref()
	for _, obj := range listmodels.Objects(windows) {
		w := gutil.MustCast[gtk.Widget](obj, gtk.WidgetGLibType())
		for c := range gutil.WidgetChildren(w) {
			if gutil.IsA(c, adw.DialogGLibType()) {
				return true
			}
		}
	}
	return false
}

// TestOpenFilesWaitsForOnline checks that opening files before there's
// an online status, as when launching with files, leaves the main loop
// running and offers to send them once one arrives.
func TestOpenFilesWaitsForOnline(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "open-wait")}
	var finalized int
	files := []*gio.FileBase{openedFile(t, &finalized)}

	statuses := make(chan *tsutil.IPNStatus)
	online := func(ctx context.Context) (*tsutil.IPNStatus, error) {
		select {
		case s := <-statuses:
			return s, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	watchdog := time.AfterFunc(5*time.Second, func() { panic("openFiles blocked the main loop") })
	a.openFiles(t.Context(), files, online)
	watchdog.Stop()
	iterateFor(50 * time.Millisecond)
	if hasDialog() {
		t.Fatal("offered to send files before an online status")
	}

	statuses <- &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()}
	iterateUntil(t, hasDialog)
	dialog := findDialog(t)
	dialog.Close()
	dialog.Unref()
	iterateUntil(t, func() bool { return finalized == 1 })
}

// TestOpenFilesGivesUp checks that opened files are released if no
// online status arrives.
func TestOpenFilesGivesUp(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "open-give-up")}
	var finalized int
	files := []*gio.FileBase{openedFile(t, &finalized)}

	ctx, cancel := context.WithCancel(t.Context())
	a.openFiles(ctx, files, func(ctx context.Context) (*tsutil.IPNStatus, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	iterateFor(50 * time.Millisecond)
	if finalized != 0 {
		t.Fatal("released the files while waiting")
	}

	cancel()
	iterateUntil(t, func() bool { return finalized == 1 })
	if hasDialog() {
		t.Fatal("offered to send files without an online status")
	}
}
