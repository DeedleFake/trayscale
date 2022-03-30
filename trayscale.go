package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/DeedleFake/trayscale/tailscale"
	"github.com/getlantern/systray"
)

var (
	//go:embed tailscale.png
	tailscaleIcon []byte

	//go:embed tailscale-light.png
	tailscaleLightIcon []byte
)

type App struct {
	TS *tailscale.Client

	app fyne.App
	win fyne.Window

	status binding.Bool
}

func (a *App) pollStatus(ctx context.Context) {
	check := time.NewTicker(5 * time.Second)

	for {
		running, err := a.TS.Status(ctx)
		if err != nil {
			log.Printf("Error: Tailscale status: %v", err)
			continue
		}
		a.status.Set(running)

		select {
		case <-ctx.Done():
			return
		case <-check.C:
		}
	}
}

func (a *App) initUI(ctx context.Context) {
	a.app = app.NewWithID("trayscale")

	a.status = binding.NewBool()
	statusLabel := binding.BoolToStringWithFormat(a.status, "Running: %v")
	go a.pollStatus(ctx)

	a.win = a.app.NewWindow("Trayscale")
	a.win.SetContent(
		container.NewCenter(
			container.NewVBox(
				widget.NewRichTextFromMarkdown(`# Trayscale`),
				widget.NewCheck("Show Window at Start", func(bool) {}),
				widget.NewLabelWithData(statusLabel),
			),
		),
	)
	a.win.SetCloseIntercept(func() { a.win.Hide() })
}

func (a *App) initTray(ctx context.Context) {
	systray.SetIcon(tailscaleLightIcon)

	newTrayItem(ctx, "Show", func() { a.win.Show() })

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

	tscli, err := exec.LookPath("tailscale")
	if err != nil {
		log.Fatalf("Error: tailscale binary not found: %v", err)
	}
	ts := tailscale.Client{
		Command: tscli,
	}

	a := App{
		TS: &ts,
	}
	a.Run(ctx)
}
