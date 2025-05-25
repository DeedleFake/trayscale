package ui

/*
#include <adwaita.h>

#include "app.h"

char *APP_ID = NULL;
*/
import "C"

import (
	"context"
	"os"
	"time"
	"unsafe"

	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tsutil"
)

func init() {
	C.APP_ID = C.CString(metadata.AppID)
}

type App struct {
	poller *tsutil.Poller

	app *C.App
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

	args := toCStrings(os.Args)
	defer freeAll(args)

	app.app = C.app_new()
	C.app_run(app.app, C.int(len(args)), unsafe.SliceData(args))
}

func (app *App) Quit() {
	C.app_quit(app.app)
}
