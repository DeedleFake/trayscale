package main

import (
	"context"
	"os"
	"os/signal"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/ui"
)

func main() {
	pprof()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ts := tsutil.Client{
		Command: "tailscale",
	}

	a := ui.UI{
		TS: &ts,
	}
	a.Run(ctx)
}
