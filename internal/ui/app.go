package ui

import (
	"context"
	"iter"
	"log/slog"
	"os"
	"slices"
	"time"

	"deedles.dev/mk"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/types/key"
)

const (
	appID                 = "dev.deedles.Trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
)

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	poller *tsutil.Poller
	online bool

	app      *adw.Application
	win      *MainWindow
	settings *gio.Settings
	tray     *tray.Tray

	statusPage    *adw.StatusPage
	selfPage      *stackPage
	mullvadPage   *stackPage
	peerPages     map[key.NodePublic]*stackPage
	spinnum       int
	operatorCheck bool
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
	stack := a.win.PeersStack

	for _, page := range a.peerPages {
		stack.Remove(page.page.Root())
	}
	clear(a.peerPages)

	if a.selfPage != nil {
		stack.Remove(a.selfPage.page.Root())
		a.selfPage = nil
	}

	if a.mullvadPage != nil {
		stack.Remove(a.mullvadPage.page.Root())
		a.mullvadPage = nil
	}

	if stack.Page(a.statusPage).Object == nil {
		stack.AddTitled(a.statusPage, "status", "Not Connected")
	}
}

func (a *App) updatePeers(status tsutil.Status) {
	if !status.Online() {
		a.updatePeersOffline()
		return
	}

	stack := a.win.PeersStack

	if a.selfPage == nil {
		a.selfPage = newStackPage(a, NewSelfPage(a, status.Status.Self, status))
	}
	a.selfPage.Update(a, status.Status.Self, status)

	switch {
	case tsutil.CanMullvad(status.Status.Self):
		if a.mullvadPage == nil {
			a.mullvadPage = newStackPage(a, NewMullvadPage(a, status))
		}
		a.mullvadPage.Update(a, nil, status)
	case a.mullvadPage != nil:
		stack.Remove(a.mullvadPage.page.Root())
		a.mullvadPage = nil
	}

	peerMap := status.Status.Peer
	peers := slices.SortedFunc(iter.Seq[key.NodePublic](xiter.Filter(xiter.MapKeys(status.Status.Peer),
		func(peer key.NodePublic) bool {
			return !tsutil.IsMullvad(peerMap[peer])
		})),
		key.NodePublic.Compare)

	for key, page := range a.peerPages {
		if _, ok := peerMap[key]; !ok {
			stack.Remove(page.page.Root())
			delete(a.peerPages, key)
		}
	}

	for _, p := range peers {
		peerStatus := peerMap[p]

		page, ok := a.peerPages[p]
		if !ok {
			page = newStackPage(a, NewPeerPage(a, peerStatus, status))
			a.peerPages[p] = page
		}

		page.Update(a, peerStatus, status)
	}

	if stack.Page(a.statusPage).Object != nil {
		stack.Remove(a.statusPage)
	}
}

func (a *App) update(s tsutil.Status) {
	online := s.Online()
	a.tray.Update(s)
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
				gtk.NewURILauncher(status.Status.AuthURL).Launch(ctx, &a.win.Window, nil)
			}
		})
		return nil
	}

	err := tsutil.Start(ctx)
	if err != nil {
		return err
	}
	a.poller.Poll() <- struct{}{}
	return nil
}

func (a *App) stopTS(ctx context.Context) error {
	err := tsutil.Stop(ctx)
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

	changeControlServerAction := gio.NewSimpleAction("change_control_server", nil)
	changeControlServerAction.ConnectActivate(func(p *glib.Variant) { a.showChangeControlServer() })
	a.app.AddAction(changeControlServerAction)

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
		a.mullvadPage = nil
		a.selfPage = nil
		a.win = nil
		return false
	})
	a.poller.Poll() <- struct{}{}
	a.win.SetVisible(true)
}

func (a *App) initTray(ctx context.Context) {
	if a.tray == nil {
		a.tray = tray.New(a.online)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-a.tray.ConnToggleChan():
			glib.IdleAdd(func() {
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				f := a.stopTS
				if !a.online {
					f = a.startTS
				}

				err := f(ctx)
				if err != nil {
					slog.Error("set Tailscale status from tray", "err", err)
					return
				}
			})

		case <-a.tray.SelfNodeChan():
			glib.IdleAdd(func() {
				s := <-a.poller.Get()
				addr, ok := s.SelfAddr()
				if !ok {
					return
				}
				a.clip(glib.NewValue(addr.String()))
				if a.win != nil {
					a.notify("Trayscale", "Copied address to clipboard")
				}
			})

		case <-a.tray.ShowChan():
			glib.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})

		case <-a.tray.QuitChan():
			a.Quit()
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
