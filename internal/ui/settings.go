package ui

import (
	"context"
	"log/slog"
	"time"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

func (a *App) initSettings(ctx context.Context) {
	// The default source can be NULL when no schemas are installed.
	source := gio.SettingsSchemaSourceGetDefault()
	if source == nil {
		a.runSettings(ctx)
		return
	}
	schema := source.Lookup(metadata.AppID, true)
	if schema == nil {
		a.runSettings(ctx)
		return
	}
	schema.Unref()

	// Lives as long as the app.
	a.settings = gio.NewSettings(metadata.AppID)
	gutil.Connect(a.settings, a.settings.ConnectChanged, func(_ gio.Settings, key string) {
		switch key {
		case "tray-icon":
			gutil.IdleAdd(func() {
				if a.settings.GetBoolean("tray-icon") {
					a.initTray(ctx)
					return
				}
				a.stopTray()
				a.tray.Close()
				a.tray = nil
			})

		case "polling-interval":
			a.poller.SetInterval() <- a.getInterval()

		case "taildrop-auto-save", "taildrop-auto-save-dir":
			gutil.IdleAdd(func() {
				// New enable/dir selection should retry files that failed
				// against the previous configuration.
				a.clearAutoSaveFailures()
				a.maybeAutoSaveFiles()
			})

		case "show-offline-peers":
			gutil.IdleAdd(func() {
				if a.win != nil {
					a.win.SetShowOffline(a.settings.GetBoolean("show-offline-peers"))
				}
			})
		}
	})

	a.runSettings(ctx)
}

func (a *App) runSettings(ctx context.Context) {
	if (a.settings == nil) || a.settings.GetBoolean("tray-icon") {
		gutil.IdleAdd(func() {
			a.initTray(ctx)
		})
	}
}

func (a *App) showChangeControlServer(ctx context.Context) {
	go func() {
		getCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		status, err := a.getIPN(getCtx)
		cancel()
		if err != nil {
			slog.Error("get control plane server URL", "err", err)
			gutil.IdleAdd(func() {
				if a.win != nil {
					a.win.Toast(locale.Get("Error setting control URL: %v", err))
				}
			})
			return
		}
		gutil.IdleAdd(func() { a.promptControlServer(ctx, status.Prefs.ControlURL()) })
	}()
}

func (a *App) promptControlServer(ctx context.Context, current string) {
	Prompt{
		Heading: locale.Get("Control Server URL"),
		Purpose: gtk.InputPurposeUrlValue,
		Responses: []PromptResponse{
			{ID: "cancel", Label: locale.Get("_Cancel")},
			{ID: "default", Label: locale.Get("Use _Default")},
			{ID: "set", Label: locale.Get("_Set URL"), Appearance: adw.ResponseSuggestedValue, Default: true},
		},
	}.Show(a, current, func(response, val string) {
		switch response {
		case "default":
			val = ipn.DefaultControlURL
		case "set":
		default:
			return
		}

		go func() {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			err := tsutil.SetControlURL(ctx, val)
			if err != nil {
				slog.Error("update control plane server URL", "err", err, "url", val)
				gutil.IdleAdd(func() {
					if a.win != nil {
						a.win.Toast(locale.Get("Error setting control URL: %v", err))
					}
				})
				return
			}
			<-a.poller.Poll()
		}()
	})
}

func (a *App) showPreferences() {
	if a.settings == nil {
		a.win.Toast(locale.Get("Settings schema not found"))
		return
	}

	dialog := NewPreferencesDialog()
	a.settings.Bind("tray-icon", &dialog.UseTrayIconRow.Object, "active", gio.GSettingsBindDefaultValue)
	a.settings.Bind("polling-interval", &dialog.PollingIntervalAdjustment.Object, "value", gio.GSettingsBindDefaultValue)
	a.settings.Bind("taildrop-auto-save", &dialog.TaildropAutoSaveRow.Object, "active", gio.GSettingsBindDefaultValue)

	// The row's items are in ExitNodeSelection order.
	dialog.ExitNodeSelectionRow.SetSelected(uint32(a.exitNodeSelection()))
	gutil.ConnectNotify(dialog.ExitNodeSelectionRow, "selected", func() {
		sel := tsutil.ExitNodeSelection(dialog.ExitNodeSelectionRow.GetSelected())
		a.settings.SetString("exit-node-selection", sel.String())
	})

	updateAutoSaveSubtitle := func() {
		dir := gutil.SettingsString(a.settings, "taildrop-auto-save-dir")
		if dir == "" {
			dialog.TaildropAutoSaveRow.SetSubtitle(locale.Get("No folder selected"))
			return
		}
		dialog.TaildropAutoSaveRow.SetSubtitle(dir)
	}
	updateAutoSaveSubtitle()

	// Location can only be changed while auto-save is enabled; the
	// switch is the single enable control for the combined row.
	syncFolderButton := func() {
		dialog.TaildropAutoSaveFolderButton.SetSensitive(dialog.TaildropAutoSaveRow.GetActive())
	}
	syncFolderButton()
	gutil.ConnectNotify(dialog.TaildropAutoSaveRow, "active", syncFolderButton)

	selectFolder := func(onCancel func()) {
		fileDialog := gtk.NewFileDialog()
		defer fileDialog.Unref() // The operation keeps it alive.
		fileDialog.SetModal(true)
		fileDialog.SetTitle(locale.Get("Select Auto-save Folder"))
		if dir := gutil.SettingsString(a.settings, "taildrop-auto-save-dir"); dir != "" {
			initial := gio.FileNewForPath(dir)
			fileDialog.SetInitialFolder(initial)
			gutil.Obj(initial).Unref()
		}
		fileDialog.SelectFolder(a.window(), nil, &gutil.AsyncCallback, gutil.Async(func(res *gio.AsyncResultBase) {
			folder, err := fileDialog.SelectFolderFinish(res)
			if err = gutil.Err(err); err != nil || folder == nil {
				if !gutil.ErrHasCode(err, int(gtk.DialogErrorDismissedValue)) {
					slog.Error("select auto-save folder", "err", err)
				}
				if onCancel != nil {
					onCancel()
				}
				return
			}
			a.settings.SetString("taildrop-auto-save-dir", gutil.FilePath(folder))
			gutil.Obj(folder).Unref()
			updateAutoSaveSubtitle()
		}))
	}

	gutil.Connect(dialog.TaildropAutoSaveFolderButton, dialog.TaildropAutoSaveFolderButton.ConnectClicked, func(gtk.Button) {
		selectFolder(nil)
	})

	// Enabling without a directory prompts for a folder; cancel leaves
	// auto-save off so the combined row never ends half-configured.
	gutil.ConnectNotify(dialog.TaildropAutoSaveRow, "active", func() {
		if !dialog.TaildropAutoSaveRow.GetActive() {
			return
		}
		if gutil.SettingsString(a.settings, "taildrop-auto-save-dir") != "" {
			return
		}
		selectFolder(func() {
			a.settings.SetBoolean("taildrop-auto-save", false)
		})
	})

	// The presented dialog keeps itself alive; drop the fields'
	// references once it's closed.
	gutil.Connect(dialog.PreferencesDialog, dialog.PreferencesDialog.ConnectClosed, func(adw.Dialog) {
		gutil.IdleAdd(func() { gutil.UnrefFields(dialog) })
	})
	dialog.PreferencesDialog.Present(a.parent())
}

// showAbout shows the app's about dialog.
func (a *App) showAbout() {
	dialog := adw.NewAboutDialog()
	defer dialog.Unref() // Presented dialogs keep themselves alive.
	dialog.SetDeveloperName("DeedleFake")
	dialog.SetCopyright("Copyright (c) 2025 DeedleFake")
	dialog.SetLicense(metadata.License())
	dialog.SetLicenseType(gtk.LicenseCustomValue)
	dialog.SetApplicationIcon(metadata.AppID)
	dialog.SetApplicationName("Trayscale")
	dialog.SetWebsite("https://github.com/DeedleFake/trayscale")
	dialog.SetIssueUrl("https://github.com/DeedleFake/trayscale/issues")

	rnv, rn := metadata.ReleaseNotes()
	dialog.SetReleaseNotesVersion(rnv)
	dialog.SetReleaseNotes(rn)

	if v, ok := metadata.Version(); ok {
		dialog.SetVersion(v)
	}

	dialog.Present(a.parent())
}

func (a *App) getInterval() time.Duration {
	if a.settings == nil {
		return 5 * time.Second
	}
	return time.Duration(a.settings.GetDouble("polling-interval") * float64(time.Second))
}

// exitNodeSelection returns how to pick an exit node when exit node
// use is turned on.
func (a *App) exitNodeSelection() tsutil.ExitNodeSelection {
	if a.settings == nil {
		return tsutil.SelectPreviousExitNode
	}
	return tsutil.ParseExitNodeSelection(gutil.SettingsString(a.settings, "exit-node-selection"))
}
