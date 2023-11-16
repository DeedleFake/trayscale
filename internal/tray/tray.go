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
	showItem     *systray.MenuItem
	quitItem     *systray.MenuItem
	selfNodeItem *systray.MenuItem
}

func New(online bool) *Tray {
	systray.SetIcon(statusIcon(online))
	systray.SetTitle("Trayscale")

	selfNodeItem := systray.AddMenuItem("", "")
	systray.AddSeparator()
	showWindow := systray.AddMenuItem("Show", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	return &Tray{
		showItem:     showWindow,
		quitItem:     quit,
		selfNodeItem: selfNodeItem,
	}
}

func (t *Tray) QuitChan() <-chan struct{} {
	return t.quitItem.ClickedCh
}

func (t *Tray) ShowChan() <-chan struct{} {
	return t.showItem.ClickedCh
}

func (t *Tray) setOnlineStatus(online bool) {
	systray.SetIcon(statusIcon(online))
}

func (t *Tray) Update(s tsutil.Status, previousOnlineStatus bool) {
	if t == nil {
		return
	}

	if s.Online() != previousOnlineStatus {
		t.setOnlineStatus(s.Online())
	}

	var title string
	if s.Status != nil && s.Status.Self != nil {
		// A naive approach to get first available TS IP.
		// Ways to refine: sort and get the "Less"er or prefer
		// first IPv4 in the list.
		var ipInfo string
		if len(s.Status.Self.TailscaleIPs) > 0 {
			ipInfo = fmt.Sprintf(
				" (%s)",
				s.Status.Self.TailscaleIPs[0].String(),
			)
		}

		title = fmt.Sprintf(
			"This device: %s%s",
			s.Status.Self.HostName,
			ipInfo,
		)
	}

	if title == "" {
		t.selfNodeItem.Hide()
	} else {
		t.selfNodeItem.SetTitle(title)
		t.selfNodeItem.Show()
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
