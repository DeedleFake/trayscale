package ui

import (
	"cmp"
	"errors"
	"io"
	"net/netip"
	"reflect"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

	name := tsutil.DNSOrQuoteHostname(status.Status, peer)
	if len(name) > maxNameLength {
		return name[:maxNameLength-3] + "..."
	}
	return name
}

func peerIcon(peer *ipnstate.PeerStatus) string {
	if peer.ExitNode {
		if !peer.Online {
			return "network-vpn-acquiring-symbolic"
		}
		return "network-vpn-symbolic"
	}
	if !peer.Online {
		return "network-wired-offline-symbolic"
	}
	if peer.ExitNodeOption {
		return "folder-remote-symbolic"
	}

	return "network-wired-symbolic"
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

func convertObject[T any](obj *glib.Object) T {
	if v, ok := obj.Cast().(T); ok {
		return v
	}
	return gioutil.ObjectValue[T](obj)
}

func NewObjectComparer[T any](f func(T, T) int) glib.CompareDataFunc {
	return glib.NewObjectComparer(func(o1, o2 *glib.Object) int {
		v1 := convertObject[T](o1)
		v2 := convertObject[T](o2)
		return f(v1, v2)
	})
}

// Page represents the UI for a single page of the app. This usually
// corresponds to information about a specific peer in the tailnet.
type Page interface {
	Widget() gtk.Widgetter
	Update(*App, *adw.ViewStackPage, tsutil.Status) bool
}
