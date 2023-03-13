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
	if addr, ok := os.LookupEnv("PPROF_ADDR"); ok {
		go func() { slog.Error("start pprof HTTP server", http.ListenAndServe(addr, nil)) }()
	}
}
