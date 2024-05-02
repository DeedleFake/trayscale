package ui

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"time"

	"deedles.dev/mk"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn"
	"tailscale.com/types/key"
)

const (
	appID                 = "dev.deedles.Trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
)

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
	tray     *tray.Tray

	statusPage    *adw.StatusPage
	peerPages     map[key.NodePublic]*stackPage
	mullvad       *MullvadPage
	spinnum       int
	operatorCheck bool
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

func (a *App) clip(v *glib.Value) {
	gdk.DisplayGetDefault().Clipboard().Set(v)
}

func (a *App) notify(title, body string) {
	icon, iconerr := gio.NewIconForString(appID)

	n := gio.NewNotification(title)
	n.SetBody(body)
	if iconerr == nil {
		n.SetIcon(icon)
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) spin() {
	glib.IdleAdd(func() {
		a.spinnum++
		a.win.WorkSpinner.SetSpinning(a.spinnum > 0)
	})
}

func (a *App) stopSpin() {
	glib.IdleAdd(func() {
		a.spinnum--
		a.win.WorkSpinner.SetSpinning(a.spinnum > 0)
	})
}

func (a *App) toast(msg string) *adw.Toast {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	a.win.ToastOverlay.AddToast(toast)
	return toast
}

func (a *App) updatePeersOffline() {
	w := a.win.PeersStack

	for id, page := range a.peerPages {
		w.Remove(page.page.Root())
		delete(a.peerPages, id)
	}

	if w.Pages().NItems() == 0 {
		w.AddTitled(a.statusPage, "status", "Not Connected")
		return
	}
}

func (a *App) updatePeers(status tsutil.Status) {
	if !status.Online() {
		a.updatePeersOffline()
		return
	}

	w := a.win.PeersStack
	w.Remove(a.statusPage)

	peerMap := status.Status.Peer
	if peerMap == nil {
		mk.Map(&peerMap, 1)
	}

	peers := slices.Insert(status.Status.Peers(), 0, status.Status.Self.PublicKey) // Add this manually to guarantee ordering.
	peerMap[status.Status.Self.PublicKey] = status.Status.Self

	peers = slices.DeleteFunc(peers, func(peer key.NodePublic) bool {
		return tsutil.IsMullvad(peerMap[peer])
	})

	oldPeers, newPeers := xslices.Partition(peers, func(peer key.NodePublic) bool {
		_, ok := a.peerPages[peer]
		return ok
	})

	for _, p := range newPeers {
		peerStatus := peerMap[p]
		page := stackPage{page: NewPage(peerStatus, status)}
		page.Init(a, peerStatus, status)
		page.Update(a, peerStatus, status)
		a.peerPages[p] = &page
	}

	for _, p := range oldPeers {
		page := a.peerPages[p]
		page.Update(a, peerMap[p], status)
	}
}

func (a *App) update(s tsutil.Status) {
	online := s.Online()
	a.tray.Update(s, a.online)
	if a.online != online {
		a.online = online

		body := "Tailscale is not connected."
		if online {
			body = "Tailscale is connected."
		}
		a.notify("Tailscale Status", body) // TODO: Notify on startup if not connected?
	}
	if a.win == nil {
		return
	}

	a.win.StatusSwitch.SetState(online)
	a.win.StatusSwitch.SetActive(online)
	a.updatePeers(s)

	if a.settings != nil {
		controlURL := a.settings.String("control-plane-server")
		if controlURL == "" {
			controlURL = ipn.DefaultControlURL
		}
		if controlURL != s.Prefs.ControlURL {
			a.settings.SetString("control-plane-server", s.Prefs.ControlURL)
		}
	}

	if a.online && !a.operatorCheck {
		a.operatorCheck = true
		if !s.OperatorIsCurrent() {
			Info{
				Heading: "User is not Tailscale Operator",
				Body:    "Some functionality may not work as expected. To resolve, run\n<tt>sudo tailscale set --operator=$USER</tt>\nin the command-line.",
			}.Show(a, nil)
		}
	}
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

	contentVariant := glib.NewVariantString("content")
	a.win.PeersStack.NotifyProperty("visible-child", func() {
		a.win.SplitView.ActivateAction("navigation.push", contentVariant)
	})

	a.win.ConnectCloseRequest(func() bool {
		clear(a.peerPages)
		a.win = nil
		return false
	})
	a.poller.Poll() <- struct{}{}
	a.win.Show()
}

func (a *App) initTray(ctx context.Context) {
	if a.tray == nil {
		a.tray = tray.New(a.online)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-a.tray.ShowChan():
			glib.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})

		case <-a.tray.QuitChan():
			a.Quit()

		case <-a.tray.SelfNodeChan():
			s := <-a.poller.Get()
			addr, ok := s.SelfAddr()
			if !ok {
				continue
			}
			a.clip(glib.NewValue(addr.String()))
			if a.win != nil {
				a.notify("Trayscale", "Copied address to clipboard")
			}
		}
	}
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	tray.Stop()
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
		TS:       a.TS,
		Interval: a.getInterval(),
		New:      func(s tsutil.Status) { glib.IdleAdd(func() { a.update(s) }) },
	}
	go a.poller.Run(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}
