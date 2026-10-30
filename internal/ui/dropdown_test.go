package ui

import (
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

// TestDropDownPopup checks that onDropDownPopup's function runs each
// time the drop down's list pops up.
func TestDropDownPopup(t *testing.T) {
	requireGTK(t)

	w := gtk.NewWindow()
	defer w.Destroy()
	dd := gtk.NewDropDownFromStrings([]string{"a", "b"})
	w.SetChild(&dd.Widget)
	var popups int
	onDropDownPopup(dd, func() { popups++ })
	w.Present()
	iterateFor(300 * time.Millisecond)
	if popups != 0 {
		t.Fatalf("%v popups seen before any popped up", popups)
	}

	for want := 1; want <= 2; want++ {
		dd.Activate() // Pops the list up, as a click does.
		iterateUntil(t, func() bool { return popups >= want })
		popdown(dd)
		iterateFor(300 * time.Millisecond)
		if popups != want {
			t.Fatalf("%v popups seen, not %v", popups, want)
		}
	}
}

func popdown(dd *gtk.DropDown) {
	for child := range gutil.WidgetChildren(&dd.Widget) {
		if popover, ok := gutil.Cast[gtk.Popover](child, gtk.PopoverGLibType()); ok {
			popover.Popdown()
		}
	}
}
