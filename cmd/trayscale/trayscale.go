package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"os/signal"
	"time"

	"deedles.dev/state"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/slices"
	"tailscale.com/ipn/ipnstate"
)

const (
	appID                 = "dev.deedles-trayscale"
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

func (a *App) initState(ctx context.Context) {
	a.poll = make(chan struct{}, 1)

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
	a.app = adw.NewApplication(appID, 0)
	a.app.ConnectActivate(func() {
		if a.win != nil {
			a.win.Show()
			return
		}

		statusSwitch := gtk.NewSwitch()
		var statusSwitchStateSet glib.SignalHandle
		statusSwitchStateSet = statusSwitch.ConnectStateSet(func(status bool) bool {
			var err error
			defer func() {
				if err != nil {
					statusSwitch.HandlerBlock(statusSwitchStateSet)
					defer statusSwitch.HandlerUnblock(statusSwitchStateSet)
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
		a.status.Listen(func(status bool) {
			statusSwitch.HandlerBlock(statusSwitchStateSet)
			defer statusSwitch.HandlerUnblock(statusSwitchStateSet)
			statusSwitch.SetState(status)
		})

		header := adw.NewHeaderBar()
		header.PackStart(statusSwitch)

		peersList := adw.NewPreferencesGroup()
		peersList.SetMarginTop(30)
		peersList.SetMarginBottom(30)
		peersList.SetMarginStart(30)
		peersList.SetMarginEnd(30)
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

					copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
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

		scroller := gtk.NewScrolledWindow()
		scroller.SetVExpand(true)
		scroller.SetMinContentWidth(480)
		scroller.SetChild(peersList)
		a.status.Listen(scroller.SetVisible)

		nopeersStatusPage := adw.NewStatusPage()
		nopeersStatusPage.SetIconName("com.tailscale-tailscale")
		nopeersStatusPage.SetTitle("Tailscale is not connected")
		nopeersStatusPage.SetVExpand(true)
		a.status.Listen(func(status bool) { nopeersStatusPage.SetVisible(!status) })

		windowBox := gtk.NewBox(gtk.OrientationVertical, 0)
		windowBox.Append(header)
		windowBox.Append(scroller)
		windowBox.Append(nopeersStatusPage)

		a.win = adw.NewApplicationWindow(&a.app.Application)
		a.win.SetTitle("Trayscale")
		a.win.SetIconName("com.tailscale-tailscale")
		a.win.SetContent(windowBox)
		a.win.SetDefaultSize(-1, 400)
		a.win.SetHideOnClose(true)
		a.win.Show() // TODO: Make this configurable.

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
}

func (a *App) Quit() {
	a.app.Quit()
}

func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.initState(ctx)
	a.initUI(ctx)

	go func() {
		<-ctx.Done()
		a.app.Quit()
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
