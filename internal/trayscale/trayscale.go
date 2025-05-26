package trayscale

import (
	"context"
	"log/slog"
	"time"

	"deedles.dev/trayscale/internal/ctxutil"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/ui"
	"github.com/atotto/clipboard"
)

type App struct {
	poller *tsutil.Poller
	tray   *tray.Tray
	app    *ui.App
}

func (app *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	context.AfterFunc(ctx, app.Quit)

	app.poller = &tsutil.Poller{
		Interval: 5 * time.Second,
		New:      app.Update,
	}

	app.tray = &tray.Tray{
		OnShow:       app.app.ShowWindow,
		OnConnToggle: func() { app.toggleConn(ctx) },
		OnExitToggle: func() { app.toggleExit(ctx) },
		OnSelfNode:   func() { app.copySelf(ctx) },
		OnQuit:       app.Quit,
	}

	app.app = ui.NewApp(app)

	go app.poller.Run(ctx)
	app.app.Run()
}

func (app *App) Quit() {
	app.app.Quit()
}

func (app *App) Update(status tsutil.Status) {
	app.tray.Update(status)
	app.app.Update(status)
}

func (app *App) toggleConn(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	status, ok := ctxutil.Recv(ctx, app.poller.GetIPN())
	if !ok {
		return
	}

	f := tsutil.Start
	if status.Online() {
		f = tsutil.Stop
	}

	err := f(ctx)
	if err != nil {
		slog.Error("failed to toggle Tailscale", "source", "tray icon", "err", err)
		return
	}

	ctxutil.Recv(ctx, app.poller.Poll())
}

func (app *App) toggleExit(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	status, ok := ctxutil.Recv(ctx, app.poller.GetIPN())
	if !ok {
		return
	}

	exitNodeActive := status.ExitNodeActive()
	err := tsutil.SetUseExitNode(ctx, !exitNodeActive)
	if err != nil {
		app.app.Notify("Toggle exit node", err.Error())
		slog.Error("failed to toggle Tailscale", "source", "tray icon", "err", err)
		return
	}

	if exitNodeActive {
		app.app.Notify("Tailscale exit node", "Disabled")
		return
	}
	app.app.Notify("Exit node", "Enabled")
}

func (app *App) copySelf(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	status, ok := ctxutil.Recv(ctx, app.poller.GetIPN())
	if !ok {
		return
	}

	addr := status.SelfAddr()
	if !addr.IsValid() {
		slog.Error("self address was invalid")
		return
	}

	err := clipboard.WriteAll(addr.String())
	if err != nil {
		slog.Error("failed to copy self address", "err", err)
		app.app.Notify("Copy address to clipboard", err.Error())
		return
	}

	app.app.Notify("Trayscale", "Copied address to clipboard")
}

func (app *App) Poller() *tsutil.Poller {
	return app.poller
}

func (app *App) Tray() *tray.Tray {
	return app.tray
}
