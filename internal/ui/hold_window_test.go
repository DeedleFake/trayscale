package ui

import (
	"slices"
	"testing"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
)

// TestHoldGatesWindowUpdates checks that the engine status and waiting
// files that arrive during a profile switch hold stay out of the window,
// which still shows the old profile, and show once the hold ends.
func TestHoldGatesWindowUpdates(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "hold-window"), operatorCheck: true}
	a.update(ipnStatus(ipn.Running, true))
	a.win = NewMainWindow(a)
	t.Cleanup(func() {
		a.win.MainWindow.Destroy()
		a.win.Dispose()
	})
	self := NewSelfPage(a, ipnStatus(ipn.Running, true))
	a.win.pages["self"] = self

	before := &tsutil.EngineStatus{}
	a.update(before)
	require.Same(t, before, a.win.engine)

	a.update(ipnStatus(ipn.NoState, true))
	require.True(t, a.hold.active(), "NoState while online didn't start a hold")

	engine := &tsutil.EngineStatus{}
	a.update(engine)
	require.Same(t, before, a.win.engine, "engine status reached the window during the hold")

	files := []apitype.WaitingFile{{Name: "a.txt", Size: 1}}
	a.update(&tsutil.FileStatus{Files: files})
	require.Empty(t, slices.Collect(self.fileModel.All()), "waiting files reached the window during the hold")

	a.update(ipnStatus(ipn.Running, true))
	require.False(t, a.hold.active())
	require.Same(t, engine, a.win.engine, "held engine status not shown after the hold")
	require.Equal(t, files, slices.Collect(self.fileModel.All()), "held waiting files not shown after the hold")
}

// TestSyncWindowClearsWaitingFilesAfterHold checks that ending a switch
// hold which cleared a.waiting also clears the self-page list, instead
// of leaving the previous profile's rows on screen.
func TestSyncWindowClearsWaitingFilesAfterHold(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "hold-clear-files"), operatorCheck: true}
	a.update(ipnStatus(ipn.Running, true))
	a.win = NewMainWindow(a)
	t.Cleanup(func() {
		a.win.MainWindow.Destroy()
		a.win.Dispose()
	})
	self := NewSelfPage(a, ipnStatus(ipn.Running, true))
	a.win.pages["self"] = self

	files := []apitype.WaitingFile{{Name: "old.txt", Size: 1}}
	a.update(&tsutil.FileStatus{Files: files})
	require.Equal(t, files, slices.Collect(self.fileModel.All()))

	a.update(ipnStatus(ipn.NoState, true))
	require.True(t, a.hold.active(), "NoState while online didn't start a hold")
	require.Nil(t, a.waiting)
	require.Equal(t, files, slices.Collect(self.fileModel.All()), "precondition: rows remain during the hold")

	a.update(ipnStatus(ipn.Running, true))
	require.False(t, a.hold.active())
	require.Empty(t, slices.Collect(self.fileModel.All()), "previous profile's waiting files still shown after the hold")
}

// TestDisconnectClearsEngine checks that the engine status from before
// a disconnect isn't shown on the pages made after reconnecting.
func TestDisconnectClearsEngine(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "disconnect-engine"), operatorCheck: true}
	a.update(ipnStatus(ipn.Running, true))
	a.win = NewMainWindow(a)
	t.Cleanup(func() {
		a.win.MainWindow.Destroy()
		a.win.Dispose()
	})

	a.update(&tsutil.EngineStatus{})
	require.NotNil(t, a.win.engine)

	a.update(ipnStatus(ipn.Stopped, false))
	require.Nil(t, a.engine, "app kept the engine status after disconnecting")
	require.Nil(t, a.win.engine, "window kept the engine status after disconnecting")

	a.update(ipnStatus(ipn.Running, true))
	a.syncWindow()
	require.Nil(t, a.win.engine, "reconnecting showed the old engine status")
}

// TestOfflineIgnoresEngine checks that the engine status tailscaled
// keeps sending while offline is neither kept nor shown, so it can't
// reach the pages made after reconnecting.
func TestOfflineIgnoresEngine(t *testing.T) {
	requireGTK(t)

	a := &App{app: newTestApp(t, "offline-engine"), operatorCheck: true}
	a.update(ipnStatus(ipn.Running, true))
	a.win = NewMainWindow(a)
	t.Cleanup(func() {
		a.win.MainWindow.Destroy()
		a.win.Dispose()
	})
	a.update(ipnStatus(ipn.Stopped, false))

	a.update(&tsutil.EngineStatus{})
	require.Nil(t, a.engine, "app kept an engine status that arrived while offline")
	require.Nil(t, a.win.engine, "window got an engine status that arrived while offline")

	a.update(ipnStatus(ipn.Running, true))
	a.syncWindow()
	require.Nil(t, a.win.engine, "reconnecting showed an engine status from while offline")
}
