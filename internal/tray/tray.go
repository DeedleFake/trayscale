package tray

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"
	"slices"
	"sync"
	"unique"

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

	selfHandle       = unique.Make("self")
	connToggleHandle = unique.Make("connToggle")
	exitToggleHandle = unique.Make("exitToggle")
	statusIconHandle = unique.Make("statusIcon")
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

	m    sync.Mutex
	item *tray.Item
	prev map[unique.Handle[string]][]any

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
	t.prev = make(map[unique.Handle[string]][]any)

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
	t.prev = nil
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

	t.m.Lock()
	defer t.m.Unlock()

	t.update(status)
}

func (t *Tray) dirty(key unique.Handle[string], vals ...any) bool {
	prev := t.prev[key]
	if slices.Equal(vals, prev) {
		return false
	}

	t.prev[key] = vals
	return true
}

func (t *Tray) update(status *tsutil.IPNStatus) {
	if t.item == nil {
		return
	}

	selfTitle, connected := selfTitle(status)
	connToggleLabel := connToggleText(status.Online())
	exitToggleLabel := exitToggleText(status)

	t.updateStatusIcon(status)

	if t.dirty(selfHandle, selfTitle, connected) {
		t.selfNodeItem.SetProps(
			tray.MenuItemLabel(fmt.Sprintf("This machine: %v", selfTitle)),
			tray.MenuItemEnabled(connected),
		)
	}

	if t.dirty(connToggleHandle, connToggleLabel) {
		t.connToggleItem.SetProps(tray.MenuItemLabel(connToggleLabel))
	}

	if t.dirty(exitToggleHandle, exitToggleLabel, connected) {
		t.exitToggleItem.SetProps(
			tray.MenuItemLabel(exitToggleLabel),
			tray.MenuItemEnabled(connected),
		)
	}
}

func (t *Tray) updateStatusIcon(status *tsutil.IPNStatus) {
	newIcon := statusIcon(status)
	if !t.dirty(statusIconHandle, newIcon) {
		return
	}

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
