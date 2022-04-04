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
	"github.com/DeedleFake/fyner/state"
	"github.com/DeedleFake/trayscale/fyneutil"
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

	showWindowAtStart := state.FromBinding[bool](
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
				Items: state.ToSliceOfStates[*ipnstate.PeerStatus, []*ipnstate.PeerStatus](a.peers),
				Builder: func() *fyner.Label {
					return new(fyner.Label)
				},
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

func (a *App) updateIcon(active bool) []byte {
	icon := "assets/icon-active.png"
	if !active {
		icon = "assets/icon-inactive.png"
	}

	data, _ := assets.ReadFile(icon)
	return data
}

func (a *App) initTray(ctx context.Context) {
	a.status.Listen(func(running bool) { systray.SetIcon(a.updateIcon(running)) })

	newTrayItem(ctx, "Show", func() { a.win.Show() })

	systray.AddSeparator()

	start := newTrayItem(ctx, "Start", func() {
		err := a.TS.Start(ctx)
		if err != nil {
			log.Printf("Error: start tailscale: %v", err)
		}
		a.poll <- struct{}{}
	})
	a.status.Listen(func(active bool) {
		if active {
			start.Disable()
			return
		}
		start.Enable()
	})

	stop := newTrayItem(ctx, "Stop", func() {
		err := a.TS.Stop(ctx)
		if err != nil {
			log.Printf("Error: stop tailscale: %v", err)
		}
		a.poll <- struct{}{}
	})
	a.status.Listen(func(active bool) {
		if !active {
			stop.Disable()
			return
		}
		stop.Enable()
	})

	systray.AddSeparator()

	newTrayItem(ctx, "Exit", func() {
		a.Quit()
	})
}

func (a *App) Quit() {
	a.app.Quit()
	systray.Quit()
}

func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.poll = make(chan struct{}, 1)

	a.initUI(ctx)
	a.initTray(ctx)

	go systray.Run(
		func() {
			go func() {
				<-ctx.Done()
				systray.Quit()
			}()
		},
		nil,
	)

	go func() {
		<-ctx.Done()
		a.app.Quit()
	}()

	a.app.Run()
}

func newTrayItem(ctx context.Context, label string, onClick func()) *systray.MenuItem {
	item := systray.AddMenuItem(label, "")
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-item.ClickedCh:
				onClick()
			}
		}
	}()
	return item
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
