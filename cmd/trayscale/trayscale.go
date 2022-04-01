package main

import (
	"context"
	"embed"
	_ "embed"
	"image/color"
	"log"
	"os"
	"os/signal"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
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

var (
	colorActive   = &color.NRGBA{0, 255, 0, 255}
	colorInactive = &color.NRGBA{255, 0, 0, 255}
)

type App struct {
	TS *tailscale.Client

	poll chan struct{}

	app fyne.App
	win fyne.Window

	peers  fyneutil.ListBinding[*ipnstate.PeerStatus, []*ipnstate.PeerStatus]
	status fyneutil.Binding[bool]
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

	a.peers = fyneutil.NewListBinding[*ipnstate.PeerStatus, []*ipnstate.PeerStatus]()
	a.status = binding.NewBool()
	fyneutil.Transform(a.status, a.peers, func(peers []*ipnstate.PeerStatus) bool {
		return len(peers) != 0
	})
	go a.pollStatus(ctx)

	icon := widget.NewIcon(fyneutil.NewMemoryResource("icon", a.updateIcon()))
	a.status.AddListener(binding.NewDataListener(func() {
		icon.SetResource(fyneutil.NewMemoryResource("icon", a.updateIcon()))
	}))

	startButton := widget.NewButton("Start", func() { a.TS.Start(ctx) })
	stopButton := widget.NewButton("Stop", func() { a.TS.Stop(ctx) })
	a.status.AddListener(binding.NewDataListener(func() {
		running, _ := a.status.Get()
		if running {
			startButton.Disable()
			stopButton.Enable()
			return
		}
		startButton.Enable()
		stopButton.Disable()
	}))

	a.win = a.app.NewWindow("Trayscale")
	a.win.SetContent(
		container.NewBorder(
			container.NewVBox(
				container.NewCenter(
					container.NewHBox(
						icon,
						widget.NewRichTextFromMarkdown(`# Trayscale`),
					),
				),
				container.New(
					fyneutil.NewMaxHBoxLayout(),
					startButton,
					stopButton,
				),
			),
			container.NewVBox(
				widget.NewCheckWithData(
					"Show Window at Start",
					binding.BindPreferenceBool(prefShowWindowAtStart, a.app.Preferences()),
				),
				widget.NewButton("Quit", func() { a.Quit() }),
			),
			nil,
			nil,
			container.NewGridWrap(
				fyne.NewSize(300, 500),
				widget.NewListWithData(
					a.peers,
					func() fyne.CanvasObject { return widget.NewLabel("") },
					func(data binding.DataItem, w fyne.CanvasObject) {
						str := binding.NewString()
						w.(*widget.Label).Bind(str)
						fyneutil.Transform(str, data.(binding.Untyped), func(u any) string {
							peer := u.(*ipnstate.PeerStatus)
							return peer.HostName
						})
					},
				),
			),
		),
	)
	a.win.SetCloseIntercept(func() { a.win.Hide() })

	if a.app.Preferences().Bool(prefShowWindowAtStart) {
		a.win.Show()
	}
}

func (a *App) updateIcon() []byte {
	icon := "assets/icon-active.png"
	active, _ := a.status.Get()
	if !active {
		icon = "assets/icon-inactive.png"
	}

	data, _ := assets.ReadFile(icon)
	return data
}

func (a *App) initTray(ctx context.Context) {
	a.status.AddListener(binding.NewDataListener(func() { systray.SetIcon(a.updateIcon()) }))

	newTrayItem(ctx, "Show", func() { a.win.Show() })

	systray.AddSeparator()

	start := newTrayItem(ctx, "Start", func() {
		err := a.TS.Start(ctx)
		if err != nil {
			log.Printf("Error: start tailscale: %v", err)
		}
		a.poll <- struct{}{}
	})
	a.status.AddListener(binding.NewDataListener(func() {
		active, _ := a.status.Get()
		if active {
			start.Disable()
			return
		}
		start.Enable()
	}))

	stop := newTrayItem(ctx, "Stop", func() {
		err := a.TS.Stop(ctx)
		if err != nil {
			log.Printf("Error: stop tailscale: %v", err)
		}
		a.poll <- struct{}{}
	})
	a.status.AddListener(binding.NewDataListener(func() {
		active, _ := a.status.Get()
		if !active {
			stop.Disable()
			return
		}
		stop.Enable()
	}))

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
