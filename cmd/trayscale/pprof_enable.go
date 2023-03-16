//go:build pprof
// +build pprof

package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"

	"golang.org/x/exp/slog"
)

func pprof() {
	go func() {
		addr := os.Getenv("PPROF_ADDR")
		if addr == "" {
			addr = ":6060"
		}

		slog.Info("start pprof HTTP server", "addr", addr)
		slog.Error("start pprof HTTP server", http.ListenAndServe(addr, nil))
	}()
}
