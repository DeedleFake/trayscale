package main

import (
	"fyne.io/systray"
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

func (t *tray) NotfiyQuit() <-chan struct{} {
	return t.quitItem.ClickedCh
}

func (t *tray) NotfiyShow() <-chan struct{} {
	return t.showItem.ClickedCh
}

func (t *tray) SetOnlineStatus(online bool) {
	if t == nil {
		return
	}

	systray.SetIcon(statusIcon(online))
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
