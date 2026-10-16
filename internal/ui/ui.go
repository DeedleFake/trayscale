package ui

import (
	"cmp"
	"time"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/types/opt"
)

// newStringSorter returns a new sorter, owned by the caller, that sorts
// GtkStringObjects by their strings. Its callbacks are freed when the
// sorter is destroyed.
func newStringSorter() *gtk.CustomSorter {
	return gutil.NewCustomSorter(func(a, b uintptr) int {
		return cmp.Compare(
			gtk.StringObjectNewFromInternalPtr(a).GetString(),
			gtk.StringObjectNewFromInternalPtr(b).GetString(),
		)
	})
}

// These live for the whole process.
var (
	boolTrueIcon    = gio.NewThemedIconWithDefaultFallbacks("object-select-symbolic")
	boolFalseIcon   = gio.NewThemedIconWithDefaultFallbacks("window-close-symbolic")
	boolUnknownIcon = gio.NewThemedIconWithDefaultFallbacks("dialog-question-symbolic")
)

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.StampMilli)
}

func boolIcon(v bool) *gio.ThemedIcon {
	if v {
		return boolTrueIcon
	}
	return boolFalseIcon
}

func optBoolIcon(v opt.Bool) *gio.ThemedIcon {
	b, ok := v.Get()
	if !ok {
		return boolUnknownIcon
	}
	return boolIcon(b)
}

// Page represents the UI for a single page of the app. This usually
// corresponds to information about a specific peer in the tailnet.
type Page interface {
	// Widget returns the page's widget, borrowed from the page. Whoever
	// holds the page owns one reference to it.
	Widget() *gtk.Widget
	// Actions returns the page's action group, borrowed from the page,
	// or nil.
	Actions() *gio.SimpleActionGroup

	// Bind attaches the page to a ViewStackPage. It is called each time
	// the page is inserted into the stack, including after restack, and
	// must be safe to call more than once. One-time widget setup belongs
	// in the constructor, not here.
	Bind(*adw.ViewStackPage)
	Update(tsutil.Status) bool
}
