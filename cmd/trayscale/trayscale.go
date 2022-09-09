package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

const (
	appID                 = "dev.deedles-trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
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

// withWidget gets the widget with the given name from b, asserts it
// to T, and then calls f with it.
func withWidget[T glib.Objector](b *gtk.Builder, name string, f func(T)) {
	w := b.GetObject(name).Cast().(T)
	f(w)
}

// getObject is a simple helper to avoid unnecessary type repetition.
// It calls builder.GetObject(name) and, casts the type to T, and then
// stores it in w. By using a pointer for the output, Go's generic
// type inference can be exploited.
func getObject[T any](w *T, builder *gtk.Builder, name string) {
	*w = builder.GetObject(name).Cast().(T)
}

func makeMap[M ~map[K]V, K comparable, V any](m *M, c int) {
	*m = make(M, c)
}

func makeChan[C ~chan E, E any](c *C, b int) {
	*c = make(C, b)
}

func peerName(peer *ipnstate.PeerStatus, self bool) string {
	const maxNameLength = 30
	name := peer.HostName
	if len(name) > maxNameLength {
		name = name[:maxNameLength-3] + "..."
	}

	if self {
		return name + " [Self]"
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

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ts := tailscale.Client{
		Command: "tailscale",
	}

	a := App{
		TS: &ts,
	}
	a.Run(ctx)
}
