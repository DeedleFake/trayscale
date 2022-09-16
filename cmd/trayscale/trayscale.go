package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"time"

	"deedles.dev/trayscale"
	"deedles.dev/trayscale/tailscale"
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

func peerName(peer *ipnstate.PeerStatus, self bool) string {
	const maxNameLength = 30
	name := peer.HostName
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

func main() {
	if prof, ok := os.LookupEnv("PPROF"); ok {
		file, err := os.Create(prof)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		err = pprof.StartCPUProfile(file)
		if err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

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
