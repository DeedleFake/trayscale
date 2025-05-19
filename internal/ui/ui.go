package ui

import (
	"cmp"
	"errors"
	"io"
	"iter"
	"net/netip"
	"reflect"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
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

// must returns v if err is nil. If err is not nil, it panics with
// err's value.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// readAssetString returns the contents of the given embedded asset as
// a string. It panics if there are any errors.
func readAssetString(file string) string {
	var str strings.Builder
	f := must(trayscale.Assets().Open(file))
	must(io.Copy(&str, f))
	return str.String()
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

func fillObjects(dst any, builder *gtk.Builder) {
	v := reflect.ValueOf(dst).Elem()
	t := v.Type()

	for i := range t.NumField() {
		fv := v.Field(i)
		ft := t.Field(i)

		name := ft.Name
		if tag, ok := ft.Tag.Lookup("gtk"); ok {
			if tag == "-" {
				continue
			}
			name = tag
		}
		obj := builder.GetObject(name)
		if obj == nil {
			continue
		}

		fv.Set(reflect.ValueOf(obj.Cast()))
	}
}

func fillFromBuilder(into any, xml ...string) {
	builder := gtk.NewBuilder()
	for _, v := range xml {
		builder.AddFromString(v)
	}

	fillObjects(into, builder)
}

func errHasCode(err error, code int) bool {
	var gerr *gerror.GError
	if !errors.As(err, &gerr) {
		return false
	}
	return gerr.ErrorCode() == code
}

type widgetParent interface {
	FirstChild() gtk.Widgetter
}

func widgetChildren(w widgetParent) iter.Seq[gtk.Widgetter] {
	type siblingNexter interface{ NextSibling() gtk.Widgetter }
	return func(yield func(gtk.Widgetter) bool) {
		cur := w.FirstChild()
		for cur != nil {
			if !yield(cur) {
				return
			}

			for child := range widgetChildren(cur.(widgetParent)) {
				if !yield(child) {
					return
				}
			}

			cur = cur.(siblingNexter).NextSibling()
		}
	}
}

func expanderRowListBox(row *adw.ExpanderRow) *gtk.ListBox {
	type caster interface{ Cast() glib.Objector }
	for child := range widgetChildren(row) {
		if r, ok := child.(caster).Cast().(*gtk.Revealer); ok {
			for child := range widgetChildren(r) {
				if box, ok := child.(caster).Cast().(*gtk.ListBox); ok {
					return box
				}
			}
		}
	}
	panic("ExpanderRow ListBox not found")
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

func (row *PageRow) SetTitle(title string) {
	row.row.SetTitle(title)
}

func (row *PageRow) SetSubtitle(subtitle string) {
	row.row.SetSubtitle(subtitle)
}

func (row *PageRow) SetIconName(icon string) {
	row.icon.SetFromIconName(icon)
}
