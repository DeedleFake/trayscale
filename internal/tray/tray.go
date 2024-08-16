package tray

import (
	_ "embed"
	"fmt"

	"deedles.dev/trayscale/internal/tsutil"
	"fyne.io/systray"
)

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

type Tray struct {
	connToggleItem *systray.MenuItem
	selfNodeItem   *systray.MenuItem
	showItem       *systray.MenuItem
	quitItem       *systray.MenuItem
}

func New(online bool) *Tray {
	systray.SetIcon(statusIcon(online))
	systray.SetTitle("Trayscale")

	showWindow := systray.AddMenuItem("Show", "")
	systray.AddSeparator()
	connToggleItem := systray.AddMenuItem(connToggleText(online), "")
	selfNodeItem := systray.AddMenuItem("", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	return &Tray{
		connToggleItem: connToggleItem,
		selfNodeItem:   selfNodeItem,
		showItem:       showWindow,
		quitItem:       quit,
	}
}

func (t *Tray) ConnToggleChan() <-chan struct{} {
	return t.connToggleItem.ClickedCh
}

func (t *Tray) SelfNodeChan() <-chan struct{} {
	return t.selfNodeItem.ClickedCh
}

func (t *Tray) ShowChan() <-chan struct{} {
	return t.showItem.ClickedCh
}

func (t *Tray) QuitChan() <-chan struct{} {
	return t.quitItem.ClickedCh
}

func (t *Tray) setOnlineStatus(online bool) {
	systray.SetIcon(statusIcon(online))
	t.connToggleItem.SetTitle(connToggleText(online))
}

func (t *Tray) Update(s tsutil.Status, previousOnlineStatus bool) {
	if t == nil {
		return
	}

	if s.Online() != previousOnlineStatus {
		t.setOnlineStatus(s.Online())
	}

	selfTitle, connected := selfTitle(s)
	t.selfNodeItem.SetTitle(fmt.Sprintf("This machine: %v", selfTitle))
	if connected {
		t.selfNodeItem.Enable()
	} else {
		t.selfNodeItem.Disable()
	}
}

var systrayExit = make(chan func(), 1)

func Start(onStart func()) {
	start, stop := systray.RunWithExternalLoop(onStart, nil)
	select {
	case f := <-systrayExit:
		f()
	default:
	}

	start()
	systrayExit <- stop
}

func Stop() {
	select {
	case f := <-systrayExit:
		f()
	default:
	}
}

func selfTitle(s tsutil.Status) (string, bool) {
	addr, ok := s.SelfAddr()
	if !ok {
		if len(s.Status.Self.TailscaleIPs) == 0 {
			return "Address unknown", false
		}
		return "Not connected", false
	}

	return fmt.Sprintf("%v (%v)", tsutil.DNSOrQuoteHostname(s.Status, s.Status.Self), addr), true
}

func connToggleText(online bool) string {
	if online {
		return "Disconnect"
	}

	return "Connect"
}
