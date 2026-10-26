package ui

import (
	"testing"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

// TestTemplates builds every template class through its constructor,
// so a template child whose type doesn't match its field fails here
// instead of panicking at runtime.
func TestTemplates(t *testing.T) {
	requireGTK(t)

	build := func(t *testing.T, build func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatal(r)
			}
		}()
		build()
	}

	running := &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()}

	t.Run("MainWindow", func(t *testing.T) {
		app := &App{app: adw.NewApplication("dev.deedles.Trayscale.test.templates", gio.GApplicationNonUniqueValue)}
		defer app.app.Unref()
		build(t, func() {
			win := NewMainWindow(app)
			win.MainWindow.Destroy()
			win.Widget().Unref()
		})
	})
	t.Run("OfflinePage", func(t *testing.T) {
		build(t, func() { offlinePageClass.New().Widget().Unref() })
	})
	t.Run("PeerPage", func(t *testing.T) {
		peer := layoutPeer(t, "peer", "peer", true, false, false)
		status := layoutStatus([]tailcfg.NodeView{peer})
		status.Prefs = ipn.NewPrefs().View()
		build(t, func() { NewPeerPage(&App{}, status, peer).Widget().Unref() })
	})
	t.Run("SelfPage", func(t *testing.T) {
		build(t, func() { NewSelfPage(&App{}, running).Widget().Unref() })
	})
	t.Run("MullvadPage", func(t *testing.T) {
		build(t, func() { NewMullvadPage(&App{}, running).Widget().Unref() })
	})
	t.Run("PreferencesDialog", func(t *testing.T) {
		build(t, func() { NewPreferencesDialog().Widget().Unref() })
	})
}
