package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"os"
	"os/signal"
	"time"

	"deedles.dev/state"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/slices"
	"tailscale.com/ipn/ipnstate"
)

//go:embed assets
var assets embed.FS

const (
	prefShowWindowAtStart = "showWindowAtStart"
)

type App struct {
	TS *tailscale.Client

	poll chan struct{}

	app *adw.Application
	win *adw.ApplicationWindow

	peers  state.State[[]*ipnstate.PeerStatus]
	status state.State[bool]
}

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

func (a *App) updateIcon(active bool) []byte {
	icon := "assets/icon-active.png"
	if !active {
		icon = "assets/icon-inactive.png"
	}

	data, _ := assets.ReadFile(icon)
	return data
}

func (a *App) initState(ctx context.Context) {
	rawpeers := state.Mutable[[]*ipnstate.PeerStatus](nil)
	a.peers = state.UniqFunc(rawpeers, func(peers, old []*ipnstate.PeerStatus) bool {
		return slices.EqualFunc(peers, old, func(p1, p2 *ipnstate.PeerStatus) bool {
			return p1.HostName == p2.HostName && slices.Equal(p1.TailscaleIPs, p2.TailscaleIPs)
		})
	})
	a.status = state.Derived(a.peers, func(peers []*ipnstate.PeerStatus) bool {
		return len(peers) != 0
	})
	go a.pollStatus(ctx, rawpeers)
}

func (a *App) initUI(ctx context.Context) {
	//statusIconActive := gdkpixbuf.NewPixbufFromXPMData(iconActiveXPM)
	//statusIconInactive := gdkpixbuf.NewPixbufFromXPMData(iconInactiveXPM)
	//statusIconState := state.Derived(a.status, func(running bool) *gdkpixbuf.Pixbuf {
	//	if running {
	//		return statusIconActive
	//	}
	//	return statusIconInactive
	//})

	a.app = adw.NewApplication("dev.deedles.trayscale", 0)
	a.app.ConnectActivate(func() {
		//statusIcon := gtk.NewImageFromPixbuf(state.Get(statusIconState))
		//statusIconState.Listen(statusIcon.SetFromPixbuf)

		statusSwitch := gtk.NewSwitch()
		a.status.Listen(statusSwitch.SetState)
		statusSwitch.ConnectStateSet(func(status bool) bool {
			if status == state.Get(a.status) {
				return false
			}

			var err error
			defer func() {
				if err != nil {
					statusSwitch.SetActive(state.Get(a.status))
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

		header := adw.NewHeaderBar()
		header.PackStart(statusSwitch)
		//header.PackStart(statusIcon)

		peersList := adw.NewPreferencesGroup()
		var peersWidgets []gtk.Widgetter
		a.peers.Listen(func(peers []*ipnstate.PeerStatus) {
			for _, w := range peersWidgets {
				peersList.Remove(w)
			}
			peersWidgets = peersWidgets[:0]

			for _, p := range peers {
				row := adw.NewExpanderRow()
				row.SetTitle(p.HostName)

				for _, ip := range p.TailscaleIPs {
					str := ip.String()

					copyButton := gtk.NewButtonFromIconName("edit-copy")
					copyButton.ConnectClicked(func() {
						copyButton.Clipboard().Set(glib.NewValue(str))
					})

					iprow := adw.NewActionRow()
					iprow.AddPrefix(gtk.NewLabel(str))
					iprow.AddSuffix(copyButton)

					row.AddRow(iprow)
				}

				peersList.Add(row)
				peersWidgets = append(peersWidgets, row)
			}
		})

		peersListFrame := gtk.NewFrame("")
		peersListFrame.SetMarginTop(30)
		peersListFrame.SetMarginBottom(30)
		peersListFrame.SetMarginStart(30)
		peersListFrame.SetMarginEnd(30)
		peersListFrame.SetChild(peersList)

		scroller := gtk.NewScrolledWindow()
		scroller.SetVExpand(true)
		scroller.SetMinContentWidth(400)
		scroller.SetChild(peersListFrame)

		windowBox := gtk.NewBox(gtk.OrientationVertical, 0)
		windowBox.Append(header)
		windowBox.Append(scroller)

		a.win = adw.NewApplicationWindow(&a.app.Application)
		a.win.SetTitle("Trayscale")
		a.win.SetContent(windowBox)
		a.win.SetDefaultSize(-1, 400)
		//a.win.SetHideOnClose(true)
		a.win.Show() // TODO: Make this configurable.
	})
}

//func (a *App) initTray(ctx context.Context) (start, stop func()) {
//	// This implementation is a placeholder until fyne-io/systray#2 is
//	// fixed.
//
//	go func() {
//		stray.Run(&stray.Stray{
//			Icon: state.Derived(a.status, a.updateIcon),
//			Items: []stray.Item{
//				&stray.MenuItem{
//					Text:    state.Static("Show"),
//					OnClick: func() { a.win.Show() },
//				},
//				&stray.Separator{},
//				&stray.MenuItem{
//					Text:     state.Static("Start"),
//					Disabled: a.status,
//					OnClick:  func() { a.TS.Start(ctx) },
//				},
//				&stray.MenuItem{
//					Text:     state.Static("Stop"),
//					Disabled: state.Derived(a.status, func(running bool) bool { return !running }),
//					OnClick:  func() { a.TS.Stop(ctx) },
//				},
//				&stray.Separator{},
//				&stray.MenuItem{
//					Text:    state.Static("Quit"),
//					OnClick: func() { a.Quit() },
//				},
//			},
//		})
//	}()
//
//	return func() {}, func() { systray.Quit() }
//}

func (a *App) Quit() {
	a.app.Quit()
}

func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.poll = make(chan struct{}, 1)

	a.initState(ctx)
	a.initUI(ctx)
	//startTray, stopTray := a.initTray(ctx)

	go func() {
		<-ctx.Done()
		a.app.Quit()
	}()

	//startTray()
	//defer stopTray()
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
