package ui

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

func (a *App) initSettings(ctx context.Context) {
	nonreloc, reloc := gio.SettingsSchemaSourceGetDefault().ListSchemas(true)
	if !slices.Contains(nonreloc, appID) && !slices.Contains(reloc, appID) {
		goto init
	}

	a.settings = gio.NewSettings(appID)
	a.settings.ConnectChanged(func(key string) {
		switch key {
		case "tray-icon":
			if a.settings.Boolean("tray-icon") {
				go tray.Start(func() { a.initTray(ctx) })
				return
			}
			tray.Stop()

		case "polling-interval":
			a.poller.SetInterval() <- a.getInterval()

		case "control-plane-server":
			url := a.settings.String("control-plane-server")
			err := tsutil.SetControlURL(ctx, url)
			if err != nil {
				slog.Error("update control plane server URL", "err", err, "url", url)
				return
			}
			a.poller.Poll() <- struct{}{}
		}
	})

init:
	if (a.settings == nil) || a.settings.Boolean("tray-icon") {
		go tray.Start(func() { a.initTray(ctx) })
	}
}

func (a *App) showChangeControlServer() {
	slog.Info("change control server clicked")
}

func (a *App) showPreferences() {
	if a.settings == nil {
		a.toast("Settings schema not found")
		return
	}

	win := NewPreferencesWindow()
	a.settings.Bind("tray-icon", win.UseTrayIconRow.Object, "active", gio.SettingsBindDefault)
	a.settings.Bind("polling-interval", win.PollingIntervalAdjustment.Object, "value", gio.SettingsBindDefault)
	a.settings.Bind("control-plane-server", win.ControlURLRow.Object, "text", gio.SettingsBindGet)
	win.ControlURLRow.ConnectApply(func() {
		a.settings.SetString("control-plane-server", win.ControlURLRow.Text())
	})
	win.SetTransientFor(&a.win.Window)
	win.Show()

	a.app.AddWindow(&win.Window.Window)
}

func (a *App) getInterval() time.Duration {
	if a.settings == nil {
		return 5 * time.Second
	}
	return time.Duration(a.settings.Double("polling-interval") * float64(time.Second))
}
