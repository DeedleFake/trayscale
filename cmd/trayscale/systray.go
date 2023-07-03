package main

import (
	"context"
	_ "embed"

	"fyne.io/systray"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

type tray struct {
	showItem *systray.MenuItem
	quitItem *systray.MenuItem
}

func initTray(online bool) *tray {
	systray.SetIcon(statusIcon(online))
	systray.SetTitle("Trayscale")

	showWindow := systray.AddMenuItem("Show", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	return &tray{
		showItem: showWindow,
		quitItem: quit,
	}
}

func (t *tray) Run(ctx context.Context, a *App) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.ShowChan():
			glib.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})
		case <-t.QuitChan():
			a.Quit()
		}
	}
}

func (t *tray) QuitChan() <-chan struct{} {
	return t.quitItem.ClickedCh
}

func (t *tray) ShowChan() <-chan struct{} {
	return t.showItem.ClickedCh
}

func (t *tray) SetOnlineStatus(online bool) {
	if t == nil {
		return
	}

	systray.SetIcon(statusIcon(online))
}

var (
	//go:embed status-icon-active.png
	statusIconActive []byte

	//go:embed status-icon-inactive.png
	statusIconInactive []byte
)

func statusIcon(online bool) []byte {
	if online {
		return statusIconActive
	}
	return statusIconInactive
}

var systrayExit = make(chan func(), 1)

func startSystray(onStart func()) {
	start, stop := systray.RunWithExternalLoop(onStart, nil)
	select {
	case f := <-systrayExit:
		f()
	default:
	}

	start()
	systrayExit <- stop
}

func stopSystray() {
	select {
	case f := <-systrayExit:
		f()
	default:
	}
}
