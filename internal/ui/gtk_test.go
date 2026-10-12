package ui

import (
	"os"
	"runtime"
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

// TestMain collects garbage after the tests so that stale Go pointers
// to C objects don't hide leaks from go test -asan's check at exit.
func TestMain(m *testing.M) {
	code := m.Run()
	runtime.GC()
	os.Exit(code)
}
