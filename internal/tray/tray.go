package tray

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"

	"deedles.dev/tray"
	"deedles.dev/trayscale/internal/tsutil"
)

var (
	//go:embed status-icon-active.png
	statusIconActiveData []byte
	statusIconActive     image.Image

	//go:embed status-icon-inactive.png
	statusIconInactiveData []byte
	statusIconInactive     image.Image

	//go:embed status-icon-exit-node.png
	statusIconExitNodeData []byte
	statusIconExitNode     image.Image
)

func init() {
	decode := func(data []byte) image.Image {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		return img
	}

	statusIconActive = decode(statusIconActiveData)
	statusIconInactive = decode(statusIconInactiveData)
	statusIconExitNode = decode(statusIconExitNodeData)
}

func statusIcon(s tsutil.Status) image.Image {
	if !s.Online() {
		return statusIconInactive
	}
	if s.Status.ExitNodeStatus != nil {
		return statusIconExitNode
	}
	return statusIconActive
}

func handler(f func()) tray.MenuItemProp {
	return tray.MenuItemHandler(tray.ClickedHandler(func(data any, timestamp uint32) error {
		f()
		return nil
	}))
}

type Tray struct {
	OnShow       func()
	OnConnToggle func()
	OnExitToggle func()
	OnSelfNode   func()
	OnQuit       func()

	item *tray.Item

	showItem       *tray.MenuItem
	connToggleItem *tray.MenuItem
	exitToggleItem *tray.MenuItem
	selfNodeItem   *tray.MenuItem
	quitItem       *tray.MenuItem
}

func (t *Tray) Start(online bool) error {
	if t.item != nil {
		return nil
	}

	item, err := tray.New(
		tray.ItemID("dev.deedles.Trayscale"),
		tray.ItemTitle("Trayscale"),
		tray.ItemIconPixmap(statusIcon(tsutil.Status{})),
	)
	if err != nil {
		return err
	}
	t.item = item

	menu := item.Menu()

	t.showItem, _ = menu.AddChild(tray.MenuItemLabel("Show"), handler(t.OnShow))
	menu.AddChild(tray.MenuItemType(tray.Separator))
	t.connToggleItem, _ = menu.AddChild(tray.MenuItemLabel(connToggleText(online)), handler(t.OnConnToggle))
	t.exitToggleItem, _ = menu.AddChild(tray.MenuItemLabel(exitToggleText(tsutil.Status{})), handler(t.OnExitToggle))
	t.selfNodeItem, _ = menu.AddChild(tray.MenuItemLabel(""), handler(t.OnSelfNode))
	menu.AddChild(tray.MenuItemType(tray.Separator))
	t.quitItem, _ = menu.AddChild(tray.MenuItemLabel("Quit"), handler(t.OnQuit))

	return nil
}

func (t *Tray) Close() error {
	if t.item == nil {
		return nil
	}

	err := t.item.Close()
	t.item = nil
	return err
}

func (t *Tray) Update(s tsutil.Status) {
	if t == nil || t.item == nil {
		return
	}

	selfTitle, connected := selfTitle(s)

	t.item.SetProps(tray.ItemIconPixmap(statusIcon(s)))

	t.connToggleItem.SetProps(tray.MenuItemLabel(connToggleText(s.Online())))
	t.exitToggleItem.SetProps(
		tray.MenuItemLabel(exitToggleText(s)),
		tray.MenuItemEnabled(connected),
	)
	t.selfNodeItem.SetProps(
		tray.MenuItemLabel(fmt.Sprintf("This machine: %v", selfTitle)),
		tray.MenuItemEnabled(connected),
	)
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

func exitToggleText(s tsutil.Status) string {
	if s.Status != nil && s.Status.ExitNodeStatus != nil {
		// TODO: Show some actual information about the current exit node?
		return "Disable exit node"
	}

	return "Enable exit node"
}
