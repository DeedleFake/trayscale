package ui

import (
	"errors"
	"io"
	"iter"
	"reflect"
	"slices"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/opt"
)

type enum[T any] struct {
	Index int
	Val   T
}

func enumerate[T any](i int, v T) enum[T] {
	return enum[T]{i, v}
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

	switch {
	case self, peer == nil:
		buf.WriteString("🔵 ")
	case peer.Online:
		buf.WriteString("🟢 ")
	default:
		buf.WriteString("🔴 ")
	}

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
	if peer == nil {
		return ""
	}

	if peer.ExitNode {
		return "network-workgroup-symbolic"
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

	for i := 0; i < t.NumField(); i++ {
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

func listModelContains[T comparable](m *gioutil.ListModel[T], val T) bool {
	for v := range m.All() {
		if v == val {
			return true
		}
	}
	return false
}

func updateListModel[T comparable](m *gioutil.ListModel[T], s []T) {
	for i, v := range listModelBackward(m) {
		if !slices.Contains(s, v) {
			m.Remove(i)
		}
	}

	for _, v := range s {
		if !listModelContains(m, v) {
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
