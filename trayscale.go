package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/getlantern/systray"
	"tailscale.com/cmd/tailscale/cli"
)

var (
	//go:embed tailscale.png
	tailscaleIcon []byte

	//go:embed tailscale-light.png
	tailscaleLightIcon []byte
)

func addItem(ctx context.Context, label string, onClick func()) *systray.MenuItem {
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

func initTray(ctx context.Context) fyne.App {
	app := app.NewWithID("trayscale")
	win := app.NewWindow("Trayscale")
	win.SetContent(
		container.NewCenter(
			container.NewVBox(
				widget.NewRichTextFromMarkdown(`# Trayscale`),
				widget.NewCheck("Show Window at Start", func(bool) {}),
			),
		),
	)
	win.SetCloseIntercept(func() { win.Hide() })

	systray.SetIcon(tailscaleLightIcon)

	addItem(ctx, "Show", func() { win.Show() })

	systray.AddSeparator()

	addItem(ctx, "Exit", func() {
		systray.Quit()
	})

	return app
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cli.Fatalf = func(format string, a ...any) {
		log.Printf("Tailscale error: %v", fmt.Sprintf(format, a...))
	}

	app := initTray(ctx)

	log.Println("Displaying icon...")
	go systray.Run(
		func() {
			log.Println("Icon ready.")

			go func() {
				<-ctx.Done()
				systray.Quit()
			}()
		},
		func() {
			log.Println("Exiting...")
			app.Quit()
		},
	)

	app.Run()
}
