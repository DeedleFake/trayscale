package ui

import (
	"cmp"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/types/opt"
)

var (
	stringListSorter = gtk.NewCustomSorter(glib.NewObjectComparer(func(s1, s2 *gtk.StringObject) int {
		return cmp.Compare(s1.String(), s2.String())
	}))

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

func boolIcon(v bool) gio.Iconner {
	if v {
		return boolTrueIcon
	}
	return boolFalseIcon
}

func optBoolIcon(v opt.Bool) gio.Iconner {
	b, ok := v.Get()
	if !ok {
		return boolUnknownIcon
	}
	return boolIcon(b)
}

// Page represents the UI for a single page of the app. This usually
// corresponds to information about a specific peer in the tailnet.
type Page interface {
	Widget() gtk.Widgetter
	Actions() gio.ActionGrouper

	// Bind attaches the page to a ViewStackPage. It is called each time
	// the page is inserted into the stack, including after restack, and
	// must be safe to call more than once. One-time widget setup belongs
	// in the constructor, not here.
	Bind(*adw.ViewStackPage)
	Update(tsutil.Status) bool
}
