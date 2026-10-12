package ui

import (
	"testing"

	"deedles.dev/trayscale/internal/gutil"
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
		var page OfflinePage
		fill(t, func() { gutil.FillFromUI(&page, offlinePageXML); gutil.UnrefFields(&page) })
	})
	t.Run("PeerPage", func(t *testing.T) {
		var page PeerPage
		fill(t, func() { gutil.FillFromUI(&page, peerPageXML); gutil.UnrefFields(&page) })
	})
	t.Run("SelfPage", func(t *testing.T) {
		var page SelfPage
		fill(t, func() { gutil.FillFromUI(&page, selfPageXML); gutil.UnrefFields(&page) })
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
