package main

import (
	"context"
	"os"
	"time"

	"deedles.dev/mk"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/internal/xslices"
	"fyne.io/systray"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

//go:generate go run deedles.dev/trayscale/cmd/gtkbuildergen -out ui.go mainwindow.ui peerpage.ui preferences.ui menu.ui

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	// TS is the Tailscale Client instance to use for interaction with
	// Tailscale.
	TS *tsutil.Client

	poller *tsutil.Poller
	online bool

	app      *adw.Application
	win      *MainWindow
	settings *gio.Settings

	statusPage *adw.StatusPage
	peerPages  map[key.NodePublic]*peerPage
}

func (a *App) showPreferences() {
	if a.settings == nil {
		a.toast("Settings schema not found")
		return
	}

	win := NewPreferencesWindow()
	a.settings.Bind("tray-icon", win.UseTrayIcon.Object, "active", gio.SettingsBindDefault)
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

func (a *App) toast(msg string) *adw.Toast {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	a.win.ToastOverlay.AddToast(toast)
	return toast
}

func (a *App) updatePeers(status tsutil.Status) {
	const statusPageName = "status"

	w := a.win.PeersStack

	var peerMap map[key.NodePublic]*ipnstate.PeerStatus
	var peers []key.NodePublic

	if status.Online() {
		if c := w.ChildByName(statusPageName); c != nil {
			w.Remove(c)
		}

		peerMap = status.Status.Peer

		peers = slices.Insert(status.Status.Peers(), 0, status.Status.Self.PublicKey) // Add this manually to guarantee ordering.
		peerMap[status.Status.Self.PublicKey] = status.Status.Self
	}

	oldPeers, newPeers := xslices.Partition(peers, func(peer key.NodePublic) bool {
		_, ok := a.peerPages[peer]
		return ok
	})

	for id, page := range a.peerPages {
		_, ok := peerMap[id]
		if !ok {
			w.Remove(page.container)
			delete(a.peerPages, id)
		}
	}

	for _, p := range newPeers {
		peerStatus := peerMap[p]
		peerPage := a.newPeerPage(status, peerStatus)
		peerPage.page = w.AddTitled(
			peerPage.container,
			p.String(),
			peerName(status, peerStatus, peerPage.self),
		)
		a.updatePeerPage(peerPage, peerStatus, status)
		a.peerPages[p] = peerPage
	}

	for _, p := range oldPeers {
		page := a.peerPages[p]
		a.updatePeerPage(page, peerMap[p], status)
	}

	if w.Pages().NItems() == 0 {
		w.AddTitled(a.statusPage, statusPageName, "Not Connected")
		return
	}
}

func (a *App) update(s tsutil.Status) {
	online := s.Online()
	if a.online != online {
		a.online = online
		a.notify(online) // TODO: Notify on startup if not connected?
		systray.SetIcon(statusIcon(online))
	}
	if a.win == nil {
		return
	}

	a.win.StatusSwitch.SetState(online)
	a.win.StatusSwitch.SetActive(online)
	a.updatePeers(s)
}

func (a *App) init(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)
	mk.Map(&a.peerPages, 0)

	var hideWindow bool
	a.app.AddMainOption("hide-window", 0, glib.OptionFlagNone, glib.OptionArgNone, "Hide window on initial start", "")
	a.app.ConnectHandleLocalOptions(func(options *glib.VariantDict) int {
		if options.Contains("hide-window") {
			hideWindow = true
		}

		return -1
	})

	a.app.ConnectStartup(func() {
		a.app.Hold()
	})

	a.app.ConnectActivate(func() {
		if hideWindow {
			hideWindow = false
			return
		}
		a.onAppActivate(ctx)
	})

	a.initSettings(ctx)
}

func (a *App) initSettings(ctx context.Context) {
	if !slices.Contains(gio.SettingsListSchemas(), appID) {
		goto init
	}

	a.settings = gio.NewSettings(appID)
	a.settings.ConnectChanged(func(key string) {
		switch key {
		case "tray-icon":
			if a.settings.Boolean("tray-icon") {
				go startSystray(func() { a.initTray(ctx) })
				return
			}
			stopSystray()
		}
	})

init:
	if (a.settings == nil) || a.settings.Boolean("tray-icon") {
		go startSystray(func() { a.initTray(ctx) })
	}
}

func (a *App) startTS(ctx context.Context) error {
	status := <-a.poller.Get()
	if status.NeedsAuth() {
		Confirmation{
			Heading: "Login Required",
			Body:    "Open a browser to authenticate with Tailscale?",
			Accept:  "_Open Browser",
			Reject:  "_Cancel",
		}.Show(a, func(accept bool) {
			if accept {
				gtk.ShowURI(&a.win.Window, status.Status.AuthURL, gdk.CURRENT_TIME)
			}
		})
		return nil
	}

	err := a.TS.Start(ctx)
	if err != nil {
		return err
	}
	a.poller.Poll() <- struct{}{}
	return nil
}

func (a *App) stopTS(ctx context.Context) error {
	err := a.TS.Stop(ctx)
	if err != nil {
		return err
	}
	a.poller.Poll() <- struct{}{}
	return nil
}

func (a *App) onAppActivate(ctx context.Context) {
	if a.win != nil {
		a.win.Present()
		return
	}

	preferencesAction := gio.NewSimpleAction("preferences", nil)
	preferencesAction.ConnectActivate(func(p *glib.Variant) { a.showPreferences() })
	a.app.AddAction(preferencesAction)

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(p *glib.Variant) { a.showAbout() })
	a.app.AddAction(aboutAction)

	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(p *glib.Variant) { a.Quit() })
	a.app.AddAction(quitAction)
	a.app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

	a.statusPage = adw.NewStatusPage()
	a.statusPage.SetTitle("Not Connected")
	a.statusPage.SetIconName("network-offline-symbolic")
	a.statusPage.SetDescription("Tailscale is not connected")

	a.win = NewMainWindow(&a.app.Application)

	a.win.StatusSwitch.ConnectStateSet(func(s bool) bool {
		if s == a.win.StatusSwitch.State() {
			return false
		}

		// TODO: Handle this, and other switches, asynchrounously instead
		// of freezing the entire UI.
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		f := a.stopTS
		if s {
			f = a.startTS
		}

		err := f(ctx)
		if err != nil {
			slog.Error("set Tailscale status", "err", err)
			a.win.StatusSwitch.SetActive(!s)
			return true
		}
		return true
	})

	a.win.PeersStack.NotifyProperty("visible-child", func() {
		if a.win.PeersStack.VisibleChild() != nil {
			a.win.Leaflet.Navigate(adw.NavigationDirectionForward)
		}
	})

	a.win.BackButton.ConnectClicked(func() {
		a.win.Leaflet.Navigate(adw.NavigationDirectionBack)
	})

	a.win.ConnectCloseRequest(func() bool {
		maps.Clear(a.peerPages)
		a.win = nil
		return false
	})
	a.poller.Poll() <- struct{}{}
	a.win.Show()
}

func (a *App) initTray(ctx context.Context) {
	systray.SetIcon(statusIcon(a.online))
	systray.SetTitle("Trayscale")

	showWindow := systray.AddMenuItem("Show", "").ClickedCh
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "").ClickedCh

	for {
		select {
		case <-ctx.Done():
			return
		case <-showWindow:
			glib.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})
		case <-quit:
			a.Quit()
		}
	}
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	stopSystray()
	a.app.Quit()
}

// Run runs the app, initializing everything and then entering the
// main loop. It will return if either ctx is cancelled or Quit is
// called.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.init(ctx)

	err := a.app.Register(ctx)
	if err != nil {
		slog.Error("register application", "err", err)
		return
	}

	a.poller = &tsutil.Poller{
		TS:  a.TS,
		New: func(s tsutil.Status) { glib.IdleAdd(func() { a.update(s) }) },
	}
	go a.poller.Run(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}
