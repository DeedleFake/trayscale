package ui

import (
	"log/slog"

	"deedles.dev/state"
	"deedles.dev/trayscale/internal/tsutil"
)

func (a *App) connectState() {
	a.status().Listen(a.update)
	a.online().Listen(func(online bool) {
		slog.Info("online status changed", "online", online)
		a.notify(online) // TODO: Notify on startup if not connected?
		a.tray.SetOnlineStatus(online)

		if a.win != nil {
			a.win.StatusSwitch.SetState(online)
			a.win.StatusSwitch.SetActive(online)
		}
	})
}

func (a *App) status() state.State[tsutil.Status] {
	return a.poller.State()
}

func (a *App) online() state.State[bool] {
	return state.Uniq(state.Derived(a.status(), tsutil.Status.Online))
}
