package ui

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/version"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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
	status := <-a.poller.Get()

	Prompt{
		Heading: "Control Server URL",
		Responses: []PromptResponse{
			{ID: "cancel", Label: "_Cancel"},
			{ID: "default", Label: "Use _Default"},
			{ID: "set", Label: "_Set URL", Appearance: adw.ResponseSuggested, Default: true},
		},
	}.Show(a, status.Prefs.ControlURL, func(response, val string) {
		slog.Info("control server URL dialog closed", "response", response, "val", val)
	})
}

func (a *App) showPreferences() {
	if a.settings == nil {
		a.toast("Settings schema not found")
		return
	}

	win := NewPreferencesWindow()
	a.settings.Bind("tray-icon", win.UseTrayIconRow.Object, "active", gio.SettingsBindDefault)
	a.settings.Bind("polling-interval", win.PollingIntervalAdjustment.Object, "value", gio.SettingsBindDefault)
	win.SetTransientFor(&a.win.Window)
	win.Show()

	a.app.AddWindow(&win.Window.Window)
}

// showAbout shows the app's about dialog.
func (a *App) showAbout() {
	dialog := adw.NewAboutWindow()
	dialog.SetDevelopers([]string{"DeedleFake"})
	dialog.SetCopyright("Copyright (c) 2023 DeedleFake")
	dialog.SetLicense(readAssetString("LICENSE"))
	dialog.SetLicenseType(gtk.LicenseCustom)
	dialog.SetApplicationIcon(appID)
	dialog.SetApplicationName("Trayscale")
	dialog.SetWebsite("https://github.com/DeedleFake/trayscale")
	dialog.SetIssueURL("https://github.com/DeedleFake/trayscale/issues")
	if v, ok := version.Get(); ok {
		dialog.SetVersion(v)
	}
	dialog.SetTransientFor(&a.win.Window)
	dialog.Show()

	a.app.AddWindow(&dialog.Window.Window)
}

func (a *App) getInterval() time.Duration {
	if a.settings == nil {
		return 5 * time.Second
	}
	return time.Duration(a.settings.Double("polling-interval") * float64(time.Second))
}
