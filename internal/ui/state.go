package ui

import (
	"fmt"
	"log/slog"
	"reflect"

	"deedles.dev/state"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

func (a *App) connectState() {
	a.status().Listen(a.update)
	deriveOnline(a.status()).Listen(func(online bool) {
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

func deriveOnline(s state.State[tsutil.Status]) state.State[bool] {
	return state.Uniq(state.Derived(s, tsutil.Status.Online))
}

func derivePrefs(s state.State[tsutil.Status]) state.State[*ipn.Prefs] {
	return state.Derived(s, func(s tsutil.Status) *ipn.Prefs { return s.Prefs })
}

func deriveField[T any](s state.State[T], name string) state.State[reflect.Value] {
	return state.Derived(s, func(v T) reflect.Value {
		rv := reflect.Indirect(reflect.ValueOf(v)).FieldByName(name)
		if !rv.IsValid() {
			panic(fmt.Errorf("invalid field: %q", name))
		}
		return rv
	})
}

func deriveMethod[T any](s state.State[T], name string) state.State[reflect.Value] {
	return state.Derived(s, func(v T) reflect.Value {
		rv := reflect.ValueOf(v).MethodByName(name)
		if !rv.IsValid() {
			panic(fmt.Errorf("invalid method: %q", name))
		}
		return rv
	})
}
