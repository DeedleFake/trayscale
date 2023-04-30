package main

import (
	"context"
	_ "embed"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/tailcfg"
	"tailscale.com/types/opt"
)

const (
	appID                 = "dev.deedles.Trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
)

var (
	//go:embed status-icon-active.png
	statusIconActive []byte

	//go:embed status-icon-inactive.png
	statusIconInactive []byte
)

func statusIcon(online bool) []byte {
	if online {
		return statusIconActive
	}
	return statusIconInactive
}

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

func peerName(status tsutil.Status, peer *tailcfg.Node, self bool) string {
	if peer.ComputedName == "" {
		peer.InitDisplayNames("")
	}

	const maxNameLength = 30
	name := peer.DisplayName(true)
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

func peerIcon(peer *tailcfg.Node) string {
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

func main() {
	pprof()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ts := tsutil.Client{
		Command: "tailscale",
	}

	a := App{
		TS: &ts,
	}
	a.Run(ctx)
}
