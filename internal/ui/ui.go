package ui

import (
	"io"
	"reflect"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/opt"
)

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

func peerName(status tsutil.Status, peer *ipnstate.PeerStatus, self bool) string {
	const maxNameLength = 30
	name := tsutil.DNSOrQuoteHostname(status.Status, peer)
	if len(name) > maxNameLength {
		name = name[:maxNameLength-3] + "..."
	}

	if self {
		return name + " [This machine]"
	}
	if peer.ExitNode {
		return name + " [Exit node]"
	}
	if peer.ExitNodeOption {
		return name + " [Exit node option]"
	}
	return name
}

func peerIcon(peer *ipnstate.PeerStatus) string {
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

	for ft := range structFields(t) {
		fv := v.FieldByIndex(ft.Index)

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

func newFromBuilder[T any](xml ...string) *T {
	builder := gtk.NewBuilder()
	for _, v := range xml {
		builder.AddFromString(v, len(v))
	}

	var t T
	fillObjects(&t, builder)

	return &t
}

func modelItems(model *gio.ListModel) xiter.Seq[*glib.Object] {
	return func(yield func(*glib.Object) bool) bool {
		for i := uint(0); i < model.NItems(); i++ {
			if !yield(model.Item(i)) {
				return false
			}
		}
		return false
	}
}

func structFields(t reflect.Type) xiter.Seq[reflect.StructField] {
	return func(yield func(reflect.StructField) bool) bool {
		for i := range t.NumField() {
			if !yield(t.Field(i)) {
				return false
			}
		}
		return false
	}
}
