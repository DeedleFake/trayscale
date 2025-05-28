package ui

/*
#include <adwaita.h>
#include "ui.h"
*/
import "C"

import (
	"log/slog"
	"runtime"

	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
)

var TypeApp = DefineType[AppClass, App](TypeAdwApplication, "UiApp")

type AppClass struct {
	AdwApplicationClass
}

func (class *AppClass) Init() {
	class.SetDispose(func(obj *GObject) {
		app := TypeApp.Cast(obj.AsGTypeInstance())
		app.unpin()
	})
}

type App struct {
	AdwApplication

	*appData
}

func (app *App) Init() {
}

type appData struct {
	p      runtime.Pinner
	tsApp  TSApp
	online bool
}

func (data *appData) pin() {
	data.p.Pin(data)
}

func (data *appData) unpin() {
	data.p.Unpin()
}

func NewApp(tsApp TSApp) *App {
	app := TypeApp.New(
		"application-id", GValueFromString(metadata.AppID),
	)
	app.appData = &appData{
		tsApp: tsApp,
	}
	app.pin()
	return app
}

func (app *App) Update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
		if app.online != status.Online() {
			app.online = status.Online()

			body := "Disconnected"
			if status.Online() {
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
	notification := GNotificationNew(title)
	defer notification.Unref()
	notification.SetBody(body)

	app.SendNotification("tailscale-status", notification)
}

////export ui_app_start_tray
//func ui_app_start_tray(ui_app *C.UiApp) C.gboolean {
//	tsApp := (*App)(ui_app).tsApp()
//
//	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
//	defer cancel()
//
//	status, ok := ctxutil.Recv(ctx, tsApp.Poller().GetIPN())
//	if !ok {
//		return C.FALSE
//	}
//
//	err := tsApp.Tray().Start(status)
//	if err != nil {
//		slog.Error("failed to start tray icon", "err", err)
//		return C.FALSE
//	}
//
//	return C.TRUE
//}
//
////export ui_app_stop_tray
//func ui_app_stop_tray(ui_app *C.UiApp) C.gboolean {
//	tsApp := (*App)(ui_app).tsApp()
//
//	err := tsApp.Tray().Close()
//	if err != nil {
//		slog.Error("failed to stop tray icon", "err", err)
//		return C.FALSE
//	}
//
//	return C.TRUE
//}
//
////export ui_app_set_polling_interval
//func ui_app_set_polling_interval(ui_app *C.UiApp, interval C.gdouble) {
//	tsApp := (*App)(ui_app).tsApp()
//
//	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
//	defer cancel()
//
//	select {
//	case <-ctx.Done():
//	case tsApp.Poller().SetInterval() <- time.Duration(interval * C.gdouble(time.Second)):
//	}
//}

type TSApp interface {
	Poller() *tsutil.Poller
	Tray() *tray.Tray
	Quit()
}
