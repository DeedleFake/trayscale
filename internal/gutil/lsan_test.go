//go:build asan

package gutil_test

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// TestLeakedLayoutReported checks that the LeakSanitizer suppressions
// for pango's and fontconfig's caches don't also hide a pango layout
// that we leak ourselves. It reruns itself to leak one, since the
// report only comes at exit.
func TestLeakedLayoutReported(t *testing.T) {
	if os.Getenv("TRAYSCALE_LEAK_LAYOUT") == "1" {
		gtk.InitCheck()
		leakLayout()
		// Collect the Go wrappers that still point at the layout.
		runtime.GC()
		return
	}
	if !gtk.InitCheck() {
		t.Skip("no display")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestLeakedLayoutReported$")
	cmd.Env = append(os.Environ(), "TRAYSCALE_LEAK_LAYOUT=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("leaked layout not reported:\n%s", out)
	}
	for _, want := range []string{"detected memory leaks", "pango_layout_new"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("leak report doesn't mention %q:\n%s", want, out)
		}
	}
}

// leakLayout creates a pango layout, lays it out, and never releases
// it.
//
//go:noinline
func leakLayout() {
	l := gtk.NewLabel("")
	layout := l.CreatePangoLayout("leaked")
	layout.GetSize(nil, nil)
	l.Unref()
	// Leave no pointer to it in the Go heap, which LeakSanitizer scans.
	layout.Ptr = 0
}
