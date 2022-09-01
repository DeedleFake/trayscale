package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	appID                 = "dev.deedles-trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
)

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

//func connectSwitch(w *gtk.Switch, status state.State[bool], f func(bool) error) state.CancelFunc {
//	var handler glib.SignalHandle
//	handler = w.ConnectStateSet(func(s bool) bool {
//		err := f(s)
//		if err != nil {
//			w.HandlerBlock(handler)
//			defer w.HandlerUnblock(handler)
//			w.SetActive(state.Get(status))
//		}
//		return true
//	})
//
//	return status.Listen(func(s bool) {
//		w.HandlerBlock(handler)
//		defer w.HandlerUnblock(handler)
//		w.SetState(s)
//	})
//}

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
