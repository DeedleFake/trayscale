package ui

import (
	"cmp"
	"net/netip"
	"time"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/types/opt"
)

var (
	addrSorter        = gtk.NewCustomSorter(NewObjectComparer(netip.Addr.Compare))
	prefixSorter      = gtk.NewCustomSorter(NewObjectComparer(netip.Prefix.Compare))
	waitingFileSorter = gtk.NewCustomSorter(NewObjectComparer(func(f1, f2 apitype.WaitingFile) int {
		return cmp.Or(
			cmp.Compare(f1.Name, f2.Name),
			cmp.Compare(f1.Size, f2.Size),
		)
	}))

	stringListSorter = gtk.NewCustomSorter(glib.NewObjectComparer(func(s1, s2 *gtk.StringObject) int {
		return cmp.Compare(s1.String(), s2.String())
	}))

	boolTrueIcon    = gio.NewThemedIconWithDefaultFallbacks("emblem-ok-symbolic")
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

func NewObjectComparer[T any](f func(T, T) int) glib.CompareDataFunc {
	return glib.NewObjectComparer(func(o1, o2 *glib.Object) int {
		v1 := listmodels.Convert[T](o1)
		v2 := listmodels.Convert[T](o2)
		return f(v1, v2)
	})
}

// Page represents the UI for a single page of the app. This usually
// corresponds to information about a specific peer in the tailnet.
type Page interface {
	Widget() gtk.Widgetter
	Actions() gio.ActionGrouper

	Init(*adw.ViewStackPage)
	Update(tsutil.Status) bool
}
