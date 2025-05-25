package trayscale

import (
	"context"
	"time"

	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/ui"
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
	go app.poller.Run(ctx)

	app.tray = &tray.Tray{
		OnShow: app.app.ShowWindow,
		OnQuit: app.Quit,
	}

	app.app = ui.NewApp(app)
	app.app.Run()
}

func (app *App) Quit() {
	app.app.Quit()
}

func (app *App) Update(status tsutil.Status) {
	app.tray.Update(status)
	app.app.Update(status)
}

func (app *App) Poller() *tsutil.Poller {
	return app.poller
}

func (app *App) Tray() *tray.Tray {
	return app.tray
}
