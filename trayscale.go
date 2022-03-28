package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

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

func initTray(ctx context.Context) {
	systray.SetIcon(tailscaleLightIcon)

	status := addItem(ctx, "Status: Down", func() {})

	systray.AddSeparator()

	addItem(ctx, "Exit", func() {
		systray.Quit()
	})

	go func() {
		for {
			check := time.NewTicker(time.Second)
			defer check.Stop()

			select {
			case <-ctx.Done():
				return

			case <-check.C:
				err := cli.Run([]string{"status"})
				if err != nil {
					status.SetTitle("Status: Down")
					continue
				}
				status.SetTitle("Status: Up")
			}
		}
	}()
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cli.Fatalf = func(format string, a ...any) {
		log.Printf("Tailscale error: %v", fmt.Sprintf(format, a...))
	}

	initTray(ctx)

	log.Println("Displaying icon...")
	systray.Run(
		func() {
			log.Println("Icon ready.")

			go func() {
				<-ctx.Done()
				systray.Quit()
			}()
		},
		func() {
			log.Println("Exiting...")
		},
	)
}
