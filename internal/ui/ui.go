package ui

import (
	"cmp"
	"net/netip"
	"time"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/types/opt"
)

var (
	addrSorter        = gtk.NewCustomSorter(NewObjectComparer(netip.Addr.Compare))
	prefixSorter      = gtk.NewCustomSorter(NewObjectComparer(xnetip.ComparePrefixes))
	waitingFileSorter = gtk.NewCustomSorter(NewObjectComparer(func(f1, f2 apitype.WaitingFile) int {
		return cmp.Or(
			cmp.Compare(f1.Name, f2.Name),
			cmp.Compare(f1.Size, f2.Size),
		)
	}))

	stringListSorter = gtk.NewCustomSorter(glib.NewObjectComparer(func(s1, s2 *gtk.StringObject) int {
		return cmp.Compare(s1.String(), s2.String())
	}))
)

func prioritize[T comparable](target, v1, v2 T) (int, bool) {
	if v1 == target {
		if v1 == v2 {
			return 0, true
		}
		return -1, true
	}
	if v2 == target {
		return 1, true
	}
	return 0, false
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.StampMilli)
}

func boolIcon(v bool) string {
	if v {
		return "emblem-ok-symbolic"
	}
	return "window-close-symbolic"
}

func optBoolIcon(v opt.Bool) string {
	b, ok := v.Get()
	if !ok {
		return "dialog-question-symbolic"
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

	Init(*PageRow)
	Update(tsutil.Status) bool
}

type PageRow struct {
	page *adw.ViewStackPage
	row  *adw.ActionRow
	icon *gtk.Image
}

func NewPageRow(page *adw.ViewStackPage) *PageRow {
	icon := gtk.NewImage()
	icon.NotifyProperty("icon-name", func() {
		page.SetIconName(icon.IconName())
	})
	icon.SetVExpand(false)
	icon.SetVAlign(gtk.AlignCenter)

	row := adw.NewActionRow()
	row.AddPrefix(icon)
	row.NotifyProperty("title", func() {
		page.SetTitle(row.Title())
	})

	return &PageRow{
		page: page,
		row:  row,
		icon: icon,
	}
}

func (row *PageRow) Page() *adw.ViewStackPage {
	return row.page
}

func (row *PageRow) Row() *adw.ActionRow {
	return row.row
}

func (row *PageRow) Icon() *gtk.Image {
	return row.icon
}

func (row *PageRow) SetTitle(title string) {
	row.row.SetTitle(title)
}

func (row *PageRow) SetSubtitle(subtitle string) {
	row.row.SetSubtitle(subtitle)
}

func (row *PageRow) SetIcon(icon gio.Iconner) {
	row.icon.SetFromGIcon(icon)
}

func (row *PageRow) SetIconName(name string) {
	row.icon.SetFromIconName(name)
}
