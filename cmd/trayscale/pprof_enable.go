//go:build pprof
// +build pprof

package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
)

func pprof() {
	go func() {
		addr := os.Getenv("PPROF_ADDR")
		if addr == "" {
			addr = ":6060"
		}

		slog.Info("start pprof HTTP server", "addr", addr)
		slog.Error("start pprof HTTP server", "err", http.ListenAndServe(addr, nil))
	}()
}
