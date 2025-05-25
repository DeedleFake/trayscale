package ui

/*
#include <adwaita.h>

#include "ui.h"
#include "app.h"
*/
import "C"

import (
	"log/slog"
	"os"
	"runtime/cgo"
	"unsafe"

	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
)

type App C.UiApp

func NewApp(tsApp TSApp) *App {
	h := C.TsApp(cgo.NewHandle(tsApp))
	return (*App)(C.ui_app_new(h))
}

func (app *App) c() *C.UiApp {
	return (*C.UiApp)(app)
}

func (app *App) Run() {
	args := toCStrings(os.Args)
	defer freeAll(args)

	C.ui_app_run(app.c(), C.int(len(args)), unsafe.SliceData(args))
}

func (app *App) Quit() {
	idle(func() {
		C.ui_app_quit(app.c())
	})
}

func (app *App) Update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
		online := app.online != C.FALSE
		if online != status.Online() {
			app.online = C.FALSE
			body := "Disconnected"
			if status.Online() {
				app.online = C.TRUE
				body = "Connected"
			}

			app.Notify("Tailscale", body) // TODO: Notify on startup if not connected?
		}
	}
}

func (app *App) ShowWindow() {
	slog.Info("show window")
}

func (app *App) Notify(title, body string) {
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))

	cbody := C.CString(body)
	defer C.free(unsafe.Pointer(cbody))

	C.ui_app_notify(app.c(), ctitle, cbody)
}

//export ui_app_start_tray
func ui_app_start_tray(app *C.UiApp) C.gboolean {
	tsApp := cgo.Handle(app.ts_app).Value().(TSApp)

	err := tsApp.Tray().Start(<-tsApp.Poller().GetIPN())
	if err != nil {
		slog.Error("failed to start tray icon", "err", err)
		return C.FALSE
	}

	return C.TRUE
}

type TSApp interface {
	Poller() *tsutil.Poller
	Tray() *tray.Tray
	Quit()
}
