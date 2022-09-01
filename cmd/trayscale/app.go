package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"time"

	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

var (
	//go:embed trayscale.ui
	uiXML string

	//go:embed page.ui
	pageXML string

	//go:embed menu.ui
	menuXML string
)

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	// TS is the Tailscale Client instance to use for interaction with
	// Tailscale.
	TS *tailscale.Client

	poll chan struct{}

	app     *adw.Application
	toaster *adw.ToastOverlay
	win     *adw.ApplicationWindow

	statusSwitch        *gtk.Switch
	statusSwitchHandler glib.SignalHandle
}

// pollStatus runs a loop that continues until ctx is cancelled. The
// loop polls Tailscale at regular intervals to determine the
// network's status, updating the App's state with the result.
func (a *App) pollStatus(ctx context.Context) {
	const ticklen = 5 * time.Second
	check := time.NewTicker(ticklen)

	for {
		peers, err := a.TS.Status(ctx)
		if err != nil {
			log.Printf("Error: Tailscale status: %v", err)
			continue
		}
		glib.IdleAdd(func() { a.updatePeers(peers) })

		select {
		case <-ctx.Done():
			return
		case <-check.C:
		case <-a.poll:
			check.Reset(ticklen)
		}
	}
}

// showAboutDialog shows the app's about dialog.
func (a *App) showAboutDialog() {
	dialog := gtk.NewAboutDialog()
	dialog.SetAuthors([]string{"DeedleFake"})
	dialog.SetComments("A simple, unofficial GUI wrapper for the Tailscale CLI client.")
	dialog.SetCopyright("Copyright (c) 2022 DeedleFake")
	dialog.SetLicense(readAssetString("LICENSE"))
	dialog.SetLogoIconName("com.tailscale-tailscale")
	dialog.SetProgramName("Trayscale")
	if v, ok := version.Get(); ok {
		dialog.SetVersion(v)
	}
	dialog.SetTransientFor(&a.win.Window)
	dialog.SetModal(true)
	dialog.Show()

	a.app.AddWindow(&dialog.Window)
}

//func (a *App) newPeerPage(p peerListInfo) (gtk.Widgetter, state.CancelFunc) {
//	builder := gtk.NewBuilderFromString(pageXML, len(pageXML))
//	withWidget(builder, "Container", func(w *adw.StatusPage) {
//		w.SetTitle(p.name)
//	})
//
//	var cg CancelGroup
//	peerStates := peerStates{s: state.Derived(a.peerInfo, func(peers map[string]*ipnstate.PeerStatus) *ipnstate.PeerStatus {
//		p := peers[p.id]
//		if p == nil {
//			p = new(ipnstate.PeerStatus)
//		}
//		return p
//	})}
//
//	var addrRows []gtk.Widgetter
//	cg.Add(peerStates.IPs().Listen(func(ips []netaddr.IP) {
//		withWidget(builder, "IPGroup", func(w *adw.PreferencesGroup) {
//			for _, row := range addrRows {
//				w.Remove(row)
//			}
//			addrRows = addrRows[:0]
//
//			for _, ip := range ips {
//				ipstr := ip.String()
//
//				copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
//				copyButton.SetTooltipText("Copy to Clipboard")
//				copyButton.ConnectClicked(func() {
//					copyButton.Clipboard().Set(glib.NewValue(ipstr))
//
//					t := adw.NewToast("Copied to clipboard")
//					t.SetTimeout(3)
//					a.toaster.AddToast(t)
//				})
//
//				iprow := adw.NewActionRow()
//				iprow.SetTitle(ipstr)
//				iprow.SetObjectProperty("title-selectable", true)
//				iprow.AddSuffix(copyButton)
//
//				w.Add(iprow)
//				addrRows = append(addrRows, iprow)
//			}
//		})
//	}))
//
//	cg.Add(peerStates.Misc().Listen(func(peer *ipnstate.PeerStatus) {
//		withWidget(builder, "ExitNodeRow", func(w *adw.ActionRow) {
//			w.SetVisible(peer.ExitNodeOption)
//		})
//		withWidget(builder, "ExitNodeSwitch", func(w *gtk.Switch) {
//		})
//		withWidget(builder, "RxBytes", func(w *gtk.Label) {
//			w.SetText(strconv.FormatInt(peer.RxBytes, 10))
//		})
//		withWidget(builder, "TxBytes", func(w *gtk.Label) {
//			w.SetText(strconv.FormatInt(peer.TxBytes, 10))
//		})
//	}))
//
//	return builder.GetObject("Container").Cast().(gtk.Widgetter), cg.Cancel
//}

// initState initializes internal App state, starts the pollStatus
// loop, and other similar initializations.
//func (a *App) initState(ctx context.Context) {
//	a.poll = make(chan struct{}, 1)
//
//	rawpeers := state.Mutable[[]*ipnstate.PeerStatus](nil)
//	a.peerList = state.UniqFunc(state.Derived(rawpeers, func(peers []*ipnstate.PeerStatus) (list []peerListInfo) {
//		for i, p := range peers {
//			name := p.HostName
//			if i == 0 {
//				name += " (This machine)"
//			}
//			if p.ExitNode {
//				name += " (Exit node)"
//			}
//
//			list = append(list, peerListInfo{
//				id:   string(p.ID),
//				name: name,
//			})
//		}
//		return list
//	}), slices.Equal[peerListInfo])
//	a.peerInfo = state.Derived(rawpeers, func(peers []*ipnstate.PeerStatus) map[string]*ipnstate.PeerStatus {
//		m := make(map[string]*ipnstate.PeerStatus)
//		for _, p := range peers {
//			m[string(p.ID)] = p
//		}
//		return m
//	})
//	a.status = state.Uniq[bool](state.Derived(a.peerList, func(peers []peerListInfo) bool {
//		return len(peers) != 0
//	}))
//	go a.pollStatus(ctx, rawpeers)
//}

func (a *App) notify(status bool) {
	body := "Tailscale is not connected."
	if status {
		body = "Tailscale is connected."
	}

	icon, iconerr := gio.NewIconForString("com.tailscale-tailscale")

	n := gio.NewNotification("Tailscale Status")
	n.SetBody(body)
	if iconerr == nil {
		n.SetIcon(icon)
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) updatePeers(peers []*ipnstate.PeerStatus) {
	status := len(peers) != 0
	a.notify(status)
	if a.win == nil {
		return
	}

	a.statusSwitch.SetState(status)
}

// init initializes the App, loading the builder XML, creating a
// window, and so on.
func (a *App) init(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)

	a.app.ConnectStartup(func() {
		a.app.Hold()
	})

	a.app.ConnectActivate(func() {
		if a.win != nil {
			a.win.Present()
			return
		}

		aboutAction := gio.NewSimpleAction("about", nil)
		aboutAction.ConnectActivate(func(p *glib.Variant) { a.showAboutDialog() })
		a.app.AddAction(aboutAction)

		quitAction := gio.NewSimpleAction("quit", nil)
		quitAction.ConnectActivate(func(p *glib.Variant) { a.Quit() })
		a.app.AddAction(quitAction)
		a.app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

		statusPage := adw.NewStatusPage()
		statusPage.SetTitle("Not Connected")
		statusPage.SetIconName("network-offline-symbolic")
		statusPage.SetDescription("Tailscale is not connected")

		builder := gtk.NewBuilder()
		builder.AddFromString(uiXML, len(uiXML))
		builder.AddFromString(menuXML, len(menuXML))

		// Workaround for Cambalache limitations.
		withWidget(builder, "MainMenuButton", func(w *gtk.MenuButton) {
			w.SetMenuModel(builder.GetObject("MainMenu").Cast().(gio.MenuModeller))
		})

		withWidget(builder, "StatusSwitch", func(w *gtk.Switch) {
			a.statusSwitch = w
			a.statusSwitchHandler = w.ConnectStateSet(func(s bool) bool {
				if s == w.State() {
					return false
				}

				f := a.TS.Stop
				if s {
					f = a.TS.Start
				}

				err := f(ctx)
				if err != nil {
					w.SetActive(!s)
					return true
				}
				a.poll <- struct{}{}
				return true
			})
		})

		//withWidget(builder, "PeersStack", func(w *gtk.Stack) {
		//	var pagesCG CancelGroup
		//	cg.Add(pagesCG.Cancel)

		//	var pages []*gtk.StackPage
		//	cg.Add(a.peerList.Listen(func(peers []peerListInfo) {
		//		pagesCG.Cancel()
		//		for _, page := range pages {
		//			w.Remove(page.Child())
		//		}
		//		pages = pages[:0]

		//		for _, p := range peers {
		//			pw, cancel := a.newPeerPage(p)
		//			pagesCG.Add(cancel)
		//			page := w.AddTitled(pw, string(p.id), p.name)
		//			pages = append(pages, page)
		//		}

		//		if len(pages) == 0 {
		//			page := w.AddTitled(statusPage, "status", "Not Connected")
		//			pages = append(pages, page)
		//		}
		//	}))
		//})

		a.toaster = builder.GetObject("ToastOverlay").Cast().(*adw.ToastOverlay)

		a.win = builder.GetObject("MainWindow").Cast().(*adw.ApplicationWindow)
		a.app.AddWindow(&a.win.Window)
		a.win.ConnectCloseRequest(func() bool {
			a.win = nil
			return false
		})
		a.win.Show()
	})
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	a.app.Quit()
}

// Run runs the app, initializing everything and then entering the
// main loop. It will return if either ctx is cancelled or Quit is
// called.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.init(ctx)

	a.poll = make(chan struct{}, 1)
	go a.pollStatus(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}
