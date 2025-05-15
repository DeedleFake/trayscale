package ui

import (
	"cmp"
	"errors"
	"io"
	"iter"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/efogdev/gotk4-adwaita/pkg/adw"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
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

	peersListSorter = gtk.NewCustomSorter(glib.NewObjectComparer(func(p1, p2 *adw.ViewStackPage) int {
		if v, ok := prioritize("self", p1.Name(), p2.Name()); ok {
			return v
		}
		if v, ok := prioritize("mullvad", p1.Name(), p2.Name()); ok {
			return v
		}
		return strings.Compare(p1.Title(), p2.Title())
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

func peerName(status tsutil.Status, peer *ipnstate.PeerStatus) string {
	const maxNameLength = 30
	self := peer.ID == status.Status.Self.ID

	var buf strings.Builder

	name := tsutil.DNSOrQuoteHostname(status.Status, peer)
	if len(name) > maxNameLength {
		name = name[:maxNameLength-3] + "..."
	}
	buf.WriteString(name)

	if self {
		buf.WriteString(" [This machine]")
	}
	if peer.ExitNode {
		buf.WriteString(" [Exit node]")
	}
	if peer.ExitNodeOption {
		buf.WriteString(" [Exit node option]")
	}

	return buf.String()
}

func peerIcon(peer *ipnstate.PeerStatus) string {
	if peer.ExitNode {
		return "network-workgroup-symbolic"
	}
	if !peer.Online {
		return "network-offline-symbolic"
	}
	if peer.ExitNodeOption {
		return "network-server-symbolic"
	}

	return "folder-remote-symbolic"
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

func listModelObjects(list *gio.ListModel) iter.Seq[*glib.Object] {
	return func(yield func(*glib.Object) bool) {
		length := list.NItems()
		for i := uint(0); i < length; i++ {
			item := list.Item(i)
			if !yield(item) {
				return
			}
		}
	}
}

func errHasCode(err error, code int) bool {
	var gerr *gerror.GError
	if !errors.As(err, &gerr) {
		return false
	}
	return gerr.ErrorCode() == code
}

func listModelBackward[T any](m *gioutil.ListModel[T]) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := int(m.NItems()) - 1; i >= 0; i-- {
			if !yield(i, m.At(i)) {
				return
			}
		}
	}
}

func stringListBackward(m *gtk.StringList) iter.Seq2[uint, string] {
	return func(yield func(uint, string) bool) {
		for i := m.NItems(); i > 0; i-- {
			if !yield(i-1, m.String(i-1)) {
				return
			}
		}
	}
}

func listModelIndex(m gio.ListModeller, f func(obj *glib.Object) bool) (uint, bool) {
	length := m.NItems()
	for i := uint(0); i < length; i++ {
		if f(m.Item(i)) {
			return i, true
		}
	}
	return 0, false
}

func updateStringList(m *gtk.StringList, s iter.Seq[string]) {
	m.FreezeNotify()
	defer m.ThawNotify()

	for i, v := range stringListBackward(m) {
		if !xiter.Contains(s, v) {
			m.Remove(i)
		}
	}

	for v := range s {
		if !xiter.Contains(xiter.V2(stringListBackward(m)), v) {
			m.Append(v)
		}
	}
}

func updateListModel[T comparable](m *gioutil.ListModel[T], s iter.Seq[T]) {
	m.FreezeNotify()
	defer m.ThawNotify()

	for i, v := range listModelBackward(m) {
		if !xiter.Contains(s, v) {
			m.Remove(i)
		}
	}

	for v := range s {
		if !xiter.Contains(m.All(), v) {
			m.Append(v)
		}
	}
}

func NewObjectComparer[T any](f func(T, T) int) glib.CompareDataFunc {
	return glib.NewObjectComparer(func(o1, o2 *glib.Object) int {
		v1 := gioutil.ObjectValue[T](o1)
		v2 := gioutil.ObjectValue[T](o2)
		return f(v1, v2)
	})
}

func BindListBoxModel[T any](lb *gtk.ListBox, m gio.ListModeller, f func(T) gtk.Widgetter) {
	lb.BindModel(m, func(obj *glib.Object) gtk.Widgetter {
		if obj, ok := obj.Cast().(T); ok {
			return f(obj)
		}

		return f(gioutil.ObjectValue[T](obj))
	})
}

func BindModel[T any](
	add func(int, gtk.Widgetter),
	remove func(int, gtk.Widgetter),
	m gio.ListModeller,
	f func(T) gtk.Widgetter,
) func() {
	widgets := make([]gtk.Widgetter, 0, m.NItems())
	h := m.ConnectItemsChanged(func(index, removed, added uint) {
		for i, w := range widgets[index : index+removed] {
			remove(int(index)+i, w)
		}

		new := make([]gtk.Widgetter, 0, added)
		for i := index; i < added; i++ {
			item := m.Item(i)
			new = append(new, f(gioutil.ObjectValue[T](item)))
		}
		widgets = slices.Replace(widgets, int(index), int(removed), new...)

		for i, w := range new {
			add(int(index)+i, w)
		}
	})

	return func() {
		m.HandlerDisconnect(h)
	}
}
