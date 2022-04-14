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

//go:embed trayscale.ui
var uiXML string

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func readAssetString(file string) string {
	var str strings.Builder
	f := must(trayscale.Assets().Open(file))
	must(io.Copy(&str, f))
	return str.String()
}

func withWidget[T glib.Objector](b *gtk.Builder, name string, f func(T)) {
	w := b.GetObject(name).Cast().(T)
	f(w)
}

type App struct {
	TS *tailscale.Client

	poll chan struct{}

	app     *adw.Application
	toaster *adw.ToastOverlay
	win     *adw.ApplicationWindow

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

func (a *App) showAboutDialog() {
	dialog := gtk.NewAboutDialog()
	dialog.SetAuthors([]string{"DeedleFake"})
	dialog.SetComments("A simple, unofficial GUI wrapper for the Tailscale CLI client.")
	dialog.SetCopyright("Copyright (c) 2022 DeedleFake")
	dialog.SetLicense(readAssetString("LICENSE"))
	dialog.SetLogoIconName("com.tailscale-tailscale")
	dialog.SetProgramName("Trayscale")
	dialog.Show()

	a.app.AddWindow(&dialog.Window)
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
			a.win.Present()
			return
		}

		aboutAction := gio.NewSimpleAction("about", nil)
		aboutAction.ConnectActivate(func(p *glib.Variant) { a.showAboutDialog() })
		a.app.AddAction(aboutAction)

		quitAction := gio.NewSimpleAction("quit", nil)
		quitAction.ConnectActivate(func(p *glib.Variant) { a.Quit() })
		a.app.AddAction(quitAction)

		builder := gtk.NewBuilderFromString(uiXML, len(uiXML))

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
			a.status.Listen(func(status bool) {
				w.HandlerBlock(handler)
				defer w.HandlerUnblock(handler)
				w.SetState(status)
			})
		})

		withWidget(builder, "MainContent", func(w *gtk.ScrolledWindow) {
			a.status.Listen(w.SetVisible)
		})

		withWidget(builder, "PeersList", func(w *adw.PreferencesGroup) {
			var children []gtk.Widgetter
			a.peers.Listen(func(peers []*ipnstate.PeerStatus) {
				for _, child := range children {
					w.Remove(child)
				}
				children = children[:0]

				for _, p := range peers {
					row := adw.NewExpanderRow()
					row.SetTitle(p.HostName)

					for _, ip := range p.TailscaleIPs {
						str := ip.String()

						copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
						copyButton.ConnectClicked(func() {
							copyButton.Clipboard().Set(glib.NewValue(str))

							t := adw.NewToast("Copied to clipboard")
							t.SetTimeout(3)
							a.toaster.AddToast(t)
						})

						iprow := adw.NewActionRow()
						iprow.AddPrefix(gtk.NewLabel(str))
						iprow.AddSuffix(copyButton)

						row.AddRow(iprow)
					}

					w.Add(row)
					children = append(children, row)
				}
			})
		})

		withWidget(builder, "NotConnectedStatusPage", func(w *adw.StatusPage) {
			a.status.Listen(func(status bool) { w.SetVisible(!status) })
		})

		a.toaster = builder.GetObject("ToastOverlay").Cast().(*adw.ToastOverlay)

		a.win = builder.GetObject("MainWindow").Cast().(*adw.ApplicationWindow)
		a.app.AddWindow(&a.win.Window)
		a.win.Show()

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
