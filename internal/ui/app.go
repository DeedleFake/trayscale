package ui

/*
#include <adwaita.h>

#include "ui.h"
#include "app.h"
#include "main_window.h"
*/
import "C"

import (
	"log/slog"
	"os"
	"runtime/cgo"
	"time"
	"unsafe"

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

func (app *App) tsApp() TSApp {
	return cgo.Handle(app.ts_app).Value().(TSApp)
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
	if app == nil {
		return
	}

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

	h := cgo.NewHandle(status)
	defer h.Delete()
	C.ui_app_update(app.c(), C.TsutilStatus(h))
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
func ui_app_start_tray(ui_app *C.UiApp) C.gboolean {
	tsApp := (*App)(ui_app).tsApp()

	err := tsApp.Tray().Start(<-tsApp.Poller().GetIPN())
	if err != nil {
		slog.Error("failed to start tray icon", "err", err)
		return C.FALSE
	}

	return C.TRUE
}

//export ui_app_stop_tray
func ui_app_stop_tray(ui_app *C.UiApp) C.gboolean {
	tsApp := (*App)(ui_app).tsApp()

	err := tsApp.Tray().Close()
	if err != nil {
		slog.Error("failed to stop tray icon", "err", err)
		return C.FALSE
	}

	return C.TRUE
}

//export ui_app_set_polling_interval
func ui_app_set_polling_interval(ui_app *C.UiApp, interval C.gdouble) {
	tsApp := (*App)(ui_app).tsApp()

	tsApp.Poller().SetInterval() <- time.Duration(interval * C.gdouble(time.Second))
}
