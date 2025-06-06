package tray

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"sync"

	"deedles.dev/tray"
	"deedles.dev/trayscale/internal/tsutil"
)

var (
	//go:embed status-icon-active.png
	statusIconActiveData []byte
	statusIconActive     = decode(statusIconActiveData)

	//go:embed status-icon-inactive.png
	statusIconInactiveData []byte
	statusIconInactive     = decode(statusIconInactiveData)

	//go:embed status-icon-exit-node.png
	statusIconExitNodeData []byte
	statusIconExitNode     = decode(statusIconExitNodeData)
)

func decode(data []byte) tray.Pixmap {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	return tray.ToPixmap(img)
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

	m    sync.RWMutex
	item *tray.Item
	icon *tray.Pixmap

	showItem       *tray.MenuItem
	connToggleItem *tray.MenuItem
	exitToggleItem *tray.MenuItem
	selfNodeItem   *tray.MenuItem
	quitItem       *tray.MenuItem
}

func (t *Tray) Start(status *tsutil.IPNStatus) error {
	if t.item != nil {
		return nil
	}

	t.m.Lock()
	defer t.m.Unlock()

	item, err := tray.New(
		tray.ItemID("dev.deedles.Trayscale"),
		tray.ItemTitle("Trayscale"),
		tray.ItemHandler(tray.ActivateHandler(func(x, y int) error {
			t.OnShow()
			return nil
		})),
	)
	if err != nil {
		return err
	}
	t.item = item

	menu := item.Menu()

	t.showItem, _ = menu.AddChild(tray.MenuItemLabel("Show"), handler(t.OnShow))
	menu.AddChild(tray.MenuItemType(tray.Separator))
	t.connToggleItem, _ = menu.AddChild(handler(t.OnConnToggle))
	t.exitToggleItem, _ = menu.AddChild(handler(t.OnExitToggle))
	t.selfNodeItem, _ = menu.AddChild(handler(t.OnSelfNode))
	menu.AddChild(tray.MenuItemType(tray.Separator))
	t.quitItem, _ = menu.AddChild(tray.MenuItemLabel("Quit"), handler(t.OnQuit))

	t.update(status)

	return nil
}

func (t *Tray) Close() error {
	if t == nil {
		return nil
	}

	t.m.Lock()
	defer t.m.Unlock()

	if t.item == nil {
		return nil
	}

	err := t.item.Close()
	t.item = nil
	t.icon = nil
	return err
}

func (t *Tray) Update(s tsutil.Status) {
	if t == nil {
		return
	}

	status, ok := s.(*tsutil.IPNStatus)
	if !ok {
		return
	}

	t.m.RLock()
	defer t.m.RUnlock()

	t.update(status)
}

func (t *Tray) update(status *tsutil.IPNStatus) {
	if t.item == nil {
		return
	}

	selfTitle, connected := selfTitle(status)

	t.updateStatusIcon(status)

	t.connToggleItem.SetProps(tray.MenuItemLabel(connToggleText(status.Online())))
	t.exitToggleItem.SetProps(
		tray.MenuItemLabel(exitToggleText(status)),
		tray.MenuItemEnabled(connected),
	)
	t.selfNodeItem.SetProps(
		tray.MenuItemLabel(fmt.Sprintf("This machine: %v", selfTitle)),
		tray.MenuItemEnabled(connected),
	)
}

func (t *Tray) updateStatusIcon(status *tsutil.IPNStatus) {
	newIcon := statusIcon(status)
	if newIcon == t.icon {
		return
	}
	t.icon = newIcon

	t.item.SetProps(tray.ItemIconPixmap(newIcon))
}

func statusIcon(status *tsutil.IPNStatus) *tray.Pixmap {
	if !status.Online() {
		return &statusIconInactive
	}
	if status.ExitNodeActive() {
		return &statusIconExitNode
	}
	return &statusIconActive
}

func selfTitle(status *tsutil.IPNStatus) (string, bool) {
	addr := status.SelfAddr()
	if !addr.IsValid() {
		return "Not connected", false
	}

	return fmt.Sprintf("%v (%v)", status.NetMap.SelfNode.DisplayName(true), addr), true
}

func connToggleText(online bool) string {
	if online {
		return "Disconnect"
	}

	return "Connect"
}

func exitToggleText(status *tsutil.IPNStatus) string {
	if status.ExitNodeActive() {
		// TODO: Show some actual information about the current exit node?
		return "Disable exit node"
	}

	return "Enable exit node"
}
