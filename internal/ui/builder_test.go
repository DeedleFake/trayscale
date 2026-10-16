package ui

import (
	"testing"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

// TestFillFromBuilder builds every .ui file into its Go struct so that
// a type missing from gutil's registry fails here instead of panicking
// at runtime.
func TestFillFromBuilder(t *testing.T) {
	requireGTK(t)

	fill := func(t *testing.T, build func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatal(r)
			}
		}()
		build()
	}

	t.Run("MainWindow", func(t *testing.T) {
		var win MainWindow
		fill(t, func() { gutil.FillFromUI(&win, menuXML, mainWindowXML); gutil.UnrefFields(&win) })
	})
	t.Run("OfflinePage", func(t *testing.T) {
		fill(t, func() { offlinePageClass.New().Destroy() })
	})
	t.Run("PeerPage", func(t *testing.T) {
		peer := layoutPeer(t, "peer", "peer", true, false, false)
		status := layoutStatus([]tailcfg.NodeView{peer})
		status.Prefs = ipn.NewPrefs().View()
		fill(t, func() { NewPeerPage(&App{}, status, peer).Destroy() })
	})
	t.Run("SelfPage", func(t *testing.T) {
		status := &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()}
		fill(t, func() { NewSelfPage(&App{}, status).Destroy() })
	})
	t.Run("MullvadPage", func(t *testing.T) {
		var page MullvadPage
		fill(t, func() { gutil.FillFromUI(&page, mullvadPageXML); gutil.UnrefFields(&page) })
	})
	t.Run("PreferencesDialog", func(t *testing.T) {
		var dialog PreferencesDialog
		fill(t, func() { gutil.FillFromUI(&dialog, preferencesXML); gutil.UnrefFields(&dialog) })
	})
}
