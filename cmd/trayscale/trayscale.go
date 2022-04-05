package main

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/data/binding"
	"github.com/DeedleFake/fyner"
	"github.com/DeedleFake/fyner/fstate"
	"github.com/DeedleFake/state"
	"github.com/DeedleFake/trayscale/fyneutil"
	"github.com/DeedleFake/trayscale/stray"
	"github.com/DeedleFake/trayscale/tailscale"
	"github.com/getlantern/systray"
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

	app fyne.App
	win fyne.Window

	peers  state.MutableState[[]*ipnstate.PeerStatus]
	status state.State[bool]
}

func (a *App) pollStatus(ctx context.Context) {
	const ticklen = 5 * time.Second
	check := time.NewTicker(ticklen)

	for {
		peers, err := a.TS.Status(ctx)
		if err != nil {
			log.Printf("Error: Tailscale status: %v", err)
			continue
		}
		a.peers.Set(peers)

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

func (a *App) initUI(ctx context.Context) {
	a.app = app.NewWithID("trayscale")

	a.peers = state.Mutable[[]*ipnstate.PeerStatus](nil)
	a.status = state.Derived(a.peers, func(peers []*ipnstate.PeerStatus) bool {
		return len(peers) != 0
	})
	go a.pollStatus(ctx)

	icon := state.Derived(a.status, func(running bool) fyne.Resource {
		return fyneutil.NewMemoryResource("icon", a.updateIcon(running))
	})

	showWindowAtStart := fstate.FromBinding[bool](
		binding.BindPreferenceBool(
			prefShowWindowAtStart,
			a.app.Preferences(),
		),
	)

	a.win = a.app.NewWindow("Trayscale")
	a.win.SetContent(fyner.Content(
		&fyner.Border{
			Top: &fyner.Box{
				Children: []fyner.Component{
					&fyner.Center{
						Child: &fyner.Box{
							Horizontal: state.Static(true),
							Children: []fyner.Component{
								&fyner.Icon{Resource: icon},
								&fyner.RichText{Markdown: state.Static(`# Trayscale`)},
							},
						},
					},
					&fyner.Container{
						Layout: state.Static(fyneutil.NewMaxHBoxLayout()),
						Children: []fyner.Component{
							&fyner.Button{
								Text:     state.Static("Start"),
								Disabled: a.status,
								OnTapped: func() { a.TS.Start(ctx) },
							},
							&fyner.Button{
								Text:     state.Static("Stop"),
								Disabled: state.Derived(a.status, func(running bool) bool { return !running }),
								OnTapped: func() { a.TS.Stop(ctx) },
							},
						},
					},
				},
			},
			Bottom: &fyner.Box{
				Children: []fyner.Component{
					&fyner.Check{
						Text:    state.Static("Show Window at Start"),
						Checked: showWindowAtStart,
					},
					&fyner.Button{
						Text:     state.Static("Quit"),
						OnTapped: func() { a.Quit() },
					},
				},
			},
			Center: &fyner.List[*ipnstate.PeerStatus, *fyner.Label]{
				Items: fstate.ToSliceOfStates[*ipnstate.PeerStatus, []*ipnstate.PeerStatus](a.peers),
				Binder: func(s state.State[*ipnstate.PeerStatus], label *fyner.Label) {
					label.Text = state.Derived(s, func(peer *ipnstate.PeerStatus) string {
						return fmt.Sprintf("%v - %v", peer.HostName, peer.TailscaleIPs)
					})
				},
			},
		},
	))
	a.win.SetCloseIntercept(func() { a.win.Hide() })
	a.win.Resize(fyne.NewSize(300, 500))

	if a.app.Preferences().Bool(prefShowWindowAtStart) {
		a.win.Show()
	}
}

func (a *App) initTray(ctx context.Context) (start, stop func()) {
	// This implementation is a placeholder until fyne-io/systray#2 is
	// fixed.

	go func() {
		stray.Run(&stray.Stray{
			Icon: state.Derived(a.status, a.updateIcon),
			Items: []stray.Item{
				&stray.MenuItem{
					Text:    state.Static("Show"),
					OnClick: func() { a.win.Show() },
				},
				&stray.Separator{},
				&stray.MenuItem{
					Text:     state.Static("Start"),
					Disabled: a.status,
					OnClick:  func() { a.TS.Start(ctx) },
				},
				&stray.MenuItem{
					Text:     state.Static("Stop"),
					Disabled: state.Derived(a.status, func(running bool) bool { return !running }),
					OnClick:  func() { a.TS.Stop(ctx) },
				},
				&stray.Separator{},
				&stray.MenuItem{
					Text:    state.Static("Quit"),
					OnClick: func() { a.Quit() },
				},
			},
		})
	}()

	return func() {}, func() { systray.Quit() }
}

func (a *App) Quit() {
	a.app.Quit()
}

func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.poll = make(chan struct{}, 1)

	a.initUI(ctx)
	startTray, stopTray := a.initTray(ctx)

	go func() {
		<-ctx.Done()
		a.app.Quit()
	}()

	startTray()
	defer stopTray()
	a.app.Run()
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
