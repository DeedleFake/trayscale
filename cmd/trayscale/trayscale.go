package main

import (
	"context"
	_ "embed"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"deedles.dev/state"
	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/slices"
	"inet.af/netaddr"
	"tailscale.com/ipn/ipnstate"
)

const (
	appID                 = "dev.deedles-trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
)

var (
	//go:embed trayscale.ui
	uiXML string

	//go:embed page.ui
	pageXML string

	//go:embed menu.ui
	menuXML string
)

// must returns v if err is nil. If err is not nil, it panics with
// err's value.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// readAssetString returns the contents of the given embedded asset as
// a string. It panics if there are any errors.
func readAssetString(file string) string {
	var str strings.Builder
	f := must(trayscale.Assets().Open(file))
	must(io.Copy(&str, f))
	return str.String()
}

// withWidget gets the widget with the given name from b, asserts it
// to T, and then calls f with it.
func withWidget[T glib.Objector](b *gtk.Builder, name string, f func(T)) {
	w := b.GetObject(name).Cast().(T)
	f(w)
}

type peerListInfo struct {
	id   string
	name string
}

type peerStates struct {
	s state.State[*ipnstate.PeerStatus]
}

func (ps peerStates) IPs() state.State[[]netaddr.IP] {
	return state.UniqFunc(state.Derived(ps.s, func(peer *ipnstate.PeerStatus) []netaddr.IP {
		return peer.TailscaleIPs
	}), slices.Equal[netaddr.IP])
}

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

	peerInfo state.State[map[string]*ipnstate.PeerStatus]
	peerList state.State[[]peerListInfo]
	status   state.State[bool]
}

// pollStatus runs a loop that continues until ctx is cancelled. The
// loop polls Tailscale at regular intervals to determine the
// network's status, updating the App's state with the result.
func (a *App) pollStatus(ctx context.Context, rawpeers state.MutableState[[]*ipnstate.PeerStatus]) {
	const ticklen = 5 * time.Second
	check := time.NewTicker(ticklen)

	for {
		peers, err := a.TS.Status(ctx)
		if err != nil {
			log.Printf("Error: Tailscale status: %v", err)
			continue
		}
		rawpeers.Set(peers)

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

func (a *App) newPeerPage(p peerListInfo) (gtk.Widgetter, state.CancelFunc) {
	builder := gtk.NewBuilderFromString(pageXML, len(pageXML))
	withWidget(builder, "Container", func(w *adw.StatusPage) {
		w.SetTitle(p.name)
	})

	var cg CancelGroup
	peerStates := peerStates{s: state.Derived(a.peerInfo, func(peers map[string]*ipnstate.PeerStatus) *ipnstate.PeerStatus {
		return peers[p.id]
	})}

	var addrRows []gtk.Widgetter
	cg.Add(peerStates.IPs().Listen(func(ips []netaddr.IP) {
		withWidget(builder, "IPGroup", func(w *adw.PreferencesGroup) {
			for _, row := range addrRows {
				w.Remove(row)
			}
			addrRows = addrRows[:0]

			for _, ip := range ips {
				ipstr := ip.String()

				copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
				copyButton.SetTooltipText("Copy to Clipboard")
				copyButton.ConnectClicked(func() {
					copyButton.Clipboard().Set(glib.NewValue(ipstr))

					t := adw.NewToast("Copied to clipboard")
					t.SetTimeout(3)
					a.toaster.AddToast(t)
				})

				iprow := adw.NewActionRow()
				iprow.SetTitle(ipstr)
				iprow.SetObjectProperty("title-selectable", true)
				iprow.AddSuffix(copyButton)

				w.Add(iprow)
				addrRows = append(addrRows, iprow)
			}
		})
	}))
	return builder.GetObject("Container").Cast().(gtk.Widgetter), cg.Cancel
}

// initState initializes internal App state, starts the pollStatus
// loop, and other similar initializations.
func (a *App) initState(ctx context.Context) {
	a.poll = make(chan struct{}, 1)

	rawpeers := state.Mutable[[]*ipnstate.PeerStatus](nil)
	a.peerInfo = state.Derived(rawpeers, func(peers []*ipnstate.PeerStatus) map[string]*ipnstate.PeerStatus {
		m := make(map[string]*ipnstate.PeerStatus)
		for _, p := range peers {
			m[string(p.ID)] = p
		}
		return m
	})
	a.peerList = state.UniqFunc(state.Derived(rawpeers, func(peers []*ipnstate.PeerStatus) (list []peerListInfo) {
		for i, p := range peers {
			name := p.HostName
			if i == 0 {
				name += " (This machine)"
			}
			if p.ExitNode {
				name += " (Exit node)"
			}

			list = append(list, peerListInfo{
				id:   string(p.ID),
				name: name,
			})
		}
		return list
	}), slices.Equal[peerListInfo])
	a.status = state.Uniq[bool](state.Derived(a.peerList, func(peers []peerListInfo) bool {
		return len(peers) != 0
	}))
	go a.pollStatus(ctx, rawpeers)
}

// initUI initializes the App's UI, loading the builder XML, creating
// a window, and so on.
func (a *App) initUI(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)

	a.app.ConnectStartup(func() {
		a.app.Hold()

		a.status.Listen(func(status bool) {
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
		})
	})

	a.app.ConnectActivate(func() {
		if a.win != nil {
			a.win.Present()
			return
		}

		var cg CancelGroup

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
			var handler glib.SignalHandle
			handler = w.ConnectStateSet(func(status bool) bool {
				var err error
				defer func() {
					if err != nil {
						w.HandlerBlock(handler)
						defer w.HandlerUnblock(handler)
						w.SetActive(state.Get(a.status))
					}
				}()

				if status {
					err = a.TS.Start(ctx)
					a.poll <- struct{}{}
					return true
				}
				err = a.TS.Stop(ctx)
				a.poll <- struct{}{}
				return true
			})
			cg.Add(a.status.Listen(func(status bool) {
				w.HandlerBlock(handler)
				defer w.HandlerUnblock(handler)
				w.SetState(status)
			}))
		})

		withWidget(builder, "PeersStack", func(w *gtk.Stack) {
			var pagesCG CancelGroup
			cg.Add(pagesCG.Cancel)

			var pages []*gtk.StackPage
			cg.Add(a.peerList.Listen(func(peers []peerListInfo) {
				pagesCG.Cancel()
				for _, page := range pages {
					w.Remove(page.Child())
				}
				pages = pages[:0]

				for _, p := range peers {
					pw, cancel := a.newPeerPage(p)
					pagesCG.Add(cancel)
					page := w.AddTitled(pw, string(p.id), p.name)
					pages = append(pages, page)
				}

				if len(pages) == 0 {
					page := w.AddTitled(statusPage, "status", "Not Connected")
					pages = append(pages, page)
				}
			}))
		})

		//withWidget(builder, "PeersList", func(w *adw.PreferencesGroup) {
		//	var children []gtk.Widgetter
		//	cg.Add(a.peers.Listen(func(peers []*ipnstate.PeerStatus) {
		//		for _, child := range children {
		//			w.Remove(child)
		//		}
		//		children = children[:0]

		//		for i, p := range peers {
		//			row := adw.NewExpanderRow()
		//			row.SetTitle(p.HostName)
		//			if i == 0 {
		//				row.SetSubtitle("This machine")
		//			}
		//			if p.ExitNode {
		//				row.SetSubtitle("Exit node")
		//			}

		//			if p.ExitNodeOption && (i > 0) {
		//				exitLabel := gtk.NewLabel("Use as Exit Node")

		//				exitSwitch := gtk.NewSwitch()
		//				exitSwitch.SetVExpand(false)
		//				exitSwitch.ConnectStateSet(func(status bool) bool {
		//					var exit *ipnstate.PeerStatus
		//					if status {
		//						exit = p
		//					}
		//					a.TS.ExitNode(ctx, exit)
		//					return true
		//				})

		//				exitRow := adw.NewActionRow()
		//				exitRow.AddPrefix(exitLabel)
		//				exitRow.AddSuffix(exitSwitch)

		//				row.AddRow(exitRow)
		//			}

		//			for _, ip := range p.TailscaleIPs {
		//				str := ip.String()

		//				copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
		//				copyButton.SetTooltipText("Copy to Clipboard")
		//				copyButton.SetVExpand(false)
		//				copyButton.ConnectClicked(func() {
		//					copyButton.Clipboard().Set(glib.NewValue(str))

		//					t := adw.NewToast("Copied to clipboard")
		//					t.SetTimeout(3)
		//					a.toaster.AddToast(t)
		//				})

		//				iplabel := gtk.NewLabel(str)
		//				iplabel.SetSelectable(true)

		//				iprow := adw.NewActionRow()
		//				iprow.AddPrefix(iplabel)
		//				iprow.AddSuffix(copyButton)

		//				row.AddRow(iprow)
		//			}

		//			w.Add(row)
		//			children = append(children, row)
		//		}
		//	}))
		//})

		//withWidget(builder, "NotConnectedStatusPage", func(w *adw.StatusPage) {
		//	cg.Add(a.status.Listen(func(status bool) { w.SetVisible(!status) }))
		//})

		a.toaster = builder.GetObject("ToastOverlay").Cast().(*adw.ToastOverlay)

		a.win = builder.GetObject("MainWindow").Cast().(*adw.ApplicationWindow)
		a.app.AddWindow(&a.win.Window)
		a.win.ConnectCloseRequest(func() bool {
			cg.Cancel()
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

	a.initState(ctx)
	a.initUI(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	ts := tailscale.Client{
		Command: "tailscale",
	}

	a := App{
		TS: &ts,
	}
	a.Run(ctx)
}
