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
	"tailscale.com/ipn/ipnstate"
)

const (
	appID                 = "dev.deedles-trayscale"
	prefShowWindowAtStart = "showWindowAtStart"
)

//go:embed trayscale.ui
var uiXML string

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

	peers  state.State[[]*ipnstate.PeerStatus]
	status state.State[bool]
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

// initState initializes internal App state, starts the pollStatus
// loop, and other similar initializations.
func (a *App) initState(ctx context.Context) {
	a.poll = make(chan struct{}, 1)

	rawpeers := state.Mutable[[]*ipnstate.PeerStatus](nil)
	a.peers = state.UniqFunc(rawpeers, (peers, old) => {
		slices.EqualFunc(peers, old, (p1, p2) => {
			p1.HostName == p2.HostName && slices.Equal(p1.TailscaleIPs, p2.TailscaleIPs)
		})
	})
	a.status = state.Uniq[bool](state.Derived(a.peers, (peers) => { len(peers) != 0 }))
	go a.pollStatus(ctx, rawpeers)
}

// initUI initializes the App's UI, loading the builder XML, creating
// a window, and so on.
func (a *App) initUI(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)

	a.app.ConnectStartup(() => {
		a.app.Hold()

		a.status.Listen((status) => {
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

	a.app.ConnectActivate(() => {
		if a.win != nil {
			a.win.Present()
			return
		}

		var cg CancelGroup

		aboutAction := gio.NewSimpleAction("about", nil)
		aboutAction.ConnectActivate((p) => { a.showAboutDialog() })
		a.app.AddAction(aboutAction)

		quitAction := gio.NewSimpleAction("quit", nil)
		quitAction.ConnectActivate((p) => { a.Quit() })
		a.app.AddAction(quitAction)
		a.app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

		builder := gtk.NewBuilderFromString(uiXML, len(uiXML))

		// I'm kind of torn on this one, since the type has to be
		// specified somewhere anyways. I think is better in case some
		// other derived type needs to be added to the closure or
		// something.
		withWidget[*gtk.Switch](builder, "StatusSwitch", (w) => {
			var handler glib.SignalHandle
			handler = w.ConnectStateSet((status) => {
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
			cg.Add(a.status.Listen((status) => {
				w.HandlerBlock(handler)
				defer w.HandlerUnblock(handler)
				w.SetState(status)
			}))
		})

		withWidget[*gtk.ScrolledWindow](builder, "MainContent", (w) => {
			cg.Add(a.status.Listen(w.SetVisible))
		})

		withWidget[*adw.PreferencesGroup](builder, "PeersList", (w) => {
			var children []gtk.Widgetter
			cg.Add(a.peers.Listen((peers) => {
				for _, child := range children {
					w.Remove(child)
				}
				children = children[:0]

				for i, p := range peers {
					row := adw.NewExpanderRow()
					row.SetTitle(p.HostName)
					if i == 0 {
						row.SetSubtitle("This machine")
					}

					for _, ip := range p.TailscaleIPs {
						str := ip.String()

						copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
						copyButton.ConnectClicked(() => {
							copyButton.Clipboard().Set(glib.NewValue(str))

							t := adw.NewToast("Copied to clipboard")
							t.SetTimeout(3)
							a.toaster.AddToast(t)
						})

						iplabel := gtk.NewLabel(str)
						iplabel.SetSelectable(true)

						iprow := adw.NewActionRow()
						iprow.AddPrefix(iplabel)
						iprow.AddSuffix(copyButton)

						row.AddRow(iprow)
					}

					w.Add(row)
					children = append(children, row)
				}
			}))
		})

		withWidget[*adw.StatusPage](builder, "NotConnectedStatusPage", (w) => {
			cg.Add(a.status.Listen((status) => { w.SetVisible(!status) }))
		})

		a.toaster = builder.GetObject("ToastOverlay").Cast().(*adw.ToastOverlay)

		a.win = builder.GetObject("MainWindow").Cast().(*adw.ApplicationWindow)
		a.app.AddWindow(&a.win.Window)
		a.win.ConnectCloseRequest(() => {
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
