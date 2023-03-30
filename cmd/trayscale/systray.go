package main

import "fyne.io/systray"

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
