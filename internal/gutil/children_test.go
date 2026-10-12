package gutil_test

import (
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

// TestWidgetChildrenRemove removes each child as it is yielded, which
// frees it, and checks that iteration still reaches every child.
func TestWidgetChildrenRemove(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}

	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	defer box.Unref()
	for range 3 {
		l := gtk.NewLabel("x")
		box.Append(&l.Widget)
		l.Unref()
	}

	var n int
	for w := range gutil.WidgetChildren(&box.Widget) {
		box.Remove(w)
		n++
	}
	if n != 3 {
		t.Fatalf("yielded %d of 3 children", n)
	}
}
