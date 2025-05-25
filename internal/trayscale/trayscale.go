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
		New:      func(s tsutil.Status) {},
	}
	go app.poller.Run(ctx)

	app.app = ui.NewApp()
	app.app.Run()
}

func (app *App) Quit() {
	app.app.Quit()
}
