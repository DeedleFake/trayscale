package ui

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/version"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/efogdev/gotk4-adwaita/pkg/adw"
	"tailscale.com/ipn"
)

func (a *App) initSettings(ctx context.Context) {
	nonreloc, reloc := gio.SettingsSchemaSourceGetDefault().ListSchemas(true)
	schemas := xiter.Concat(slices.Values(nonreloc), slices.Values(reloc))
	if !xiter.Contains(schemas, appID) {
		a.runSettings(ctx)
		return
	}

	a.settings = gio.NewSettings(appID)
	a.settings.ConnectChanged(func(key string) {
		switch key {
		case "tray-icon":
			if a.settings.Boolean("tray-icon") {
				glib.IdleAdd(func() {
					a.initTray(ctx)
				})
				return
			}
			glib.IdleAdd(func() {
				a.tray.Close()
				a.tray = nil
			})

		case "polling-interval":
			a.poller.SetInterval() <- a.getInterval()
		}
	})

	a.runSettings(ctx)
}

func (a *App) runSettings(ctx context.Context) {
	if (a.settings == nil) || a.settings.Boolean("tray-icon") {
		glib.IdleAdd(func() {
			a.initTray(ctx)
		})
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
		switch response {
		case "default":
			val = ipn.DefaultControlURL
			fallthrough // Oh my.
		case "set":
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := tsutil.SetControlURL(ctx, val)
			if err != nil {
				slog.Error("update control plane server URL", "err", err, "url", val)
				a.toast(fmt.Sprintf("Error setting control URL: %v", err))
				return
			}
			a.poller.Poll() <- struct{}{}
		}
	})
}

func (a *App) showPreferences() {
	if a.settings == nil {
		a.toast("Settings schema not found")
		return
	}

	win := NewPreferencesWindow()
	a.settings.Bind("tray-icon", win.UseTrayIconRow.Object, "active", gio.SettingsBindDefault)
	a.settings.Bind("show-notification-on-startup", win.ShowNotificationOnStartupRow.Object, "active", gio.SettingsBindDefault)
	a.settings.Bind("polling-interval", win.PollingIntervalAdjustment.Object, "value", gio.SettingsBindDefault)
	win.SetTransientFor(&a.win.Window)
	win.SetVisible(true)

	a.app.AddWindow(&win.Window.Window)
}

// showAbout shows the app's about dialog.
func (a *App) showAbout() {
	dialog := adw.NewAboutDialog()
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
	dialog.SetVisible(true)

	dialog.Present(a.window())
}

func (a *App) getInterval() time.Duration {
	if a.settings == nil {
		return 5 * time.Second
	}
	return time.Duration(a.settings.Double("polling-interval") * float64(time.Second))
}
