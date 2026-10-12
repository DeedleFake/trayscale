package ui

import (
	"sync"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

var initGTK = sync.OnceValue(func() bool {
	if !gtk.InitCheck() {
		return false
	}
	adw.Init()
	return true
})

// requireGTK initializes GTK and libadwaita once for the package's
// tests, skipping t if there is no display.
func requireGTK(t *testing.T) {
	t.Helper()
	if !initGTK() {
		t.Skip("no display")
	}
}
