package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime/pprof"

	"deedles.dev/trayscale/internal/ui"
)

func profile() func() {
	path, ok := os.LookupEnv("PPROF")
	if !ok {
		return func() {}
	}

	slog.Info("profiling enabled", "path", path)

	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}

	err = pprof.StartCPUProfile(file)
	if err != nil {
		panic(err)
	}

	return func() {
		pprof.StopCPUProfile()
		err := file.Close()
		if err != nil {
			panic(err)
		}

		slog.Info("profiling stopped", "path", path)
	}
}

func main() {
	defer profile()()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var a ui.App
	a.Run(ctx)
}
