package ui

import (
	"context"
	"net/netip"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
	"tailscale.com/tailcfg"
)

// addPageAction adds action to group, connecting activate, and drops
// the caller's reference to action.
func addPageAction(group *gio.SimpleActionGroup, action *gio.SimpleAction, activate func(param *glib.Variant)) {
	gutil.Connect(action, action.ConnectActivate, func(_ gio.SimpleAction, p uintptr) {
		var param *glib.Variant
		if p != 0 {
			param = gutil.Ptr[glib.Variant](p)
		}
		activate(param)
	})
	group.AddAction(action)
	action.Unref()
}

// rowSwitch returns the GtkSwitch of a switch row, borrowed from it.
func rowSwitch(row *adw.SwitchRow) *gtk.Switch {
	w := row.GetActivatableWidget()
	w.Unref() // The row keeps it alive.
	return gutil.MustCast[gtk.Switch](w, gtk.SwitchGLibType())
}

// setSwitch sets both the state and the active property of a switch
// row's switch.
func setSwitch(row *adw.SwitchRow, v bool) {
	sw := rowSwitch(row)
	sw.SetState(v)
	sw.SetActive(v)
}

// setPlaceholder gives lb a placeholder row titled title.
func setPlaceholder(lb *gtk.ListBox, title string) {
	row := adw.NewActionRow()
	row.SetTitle(title)
	lb.SetPlaceholder(&row.Widget)
	row.Unref()
}

// copyRow returns a new row showing text with a button that copies
// it. The caller owns the row.
func (a *App) copyRow(text string) *gtk.Widget {
	copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
	copyButton.SetMarginTop(12) // Why is this necessary?
	copyButton.SetMarginBottom(12)
	copyButton.SetHasFrame(false)
	copyButton.SetTooltipText(locale.Get("Copy to Clipboard"))
	gutil.Connect(copyButton, copyButton.ConnectClicked, func(gtk.Button) {
		a.clip(text)
		a.win.Toast(locale.Get("Copied to clipboard"))
	})

	row := adw.NewActionRow()
	row.SetTitleSelectable(true)
	row.AddSuffix(&copyButton.Widget)
	row.SetActivatableWidget(&copyButton.Widget)
	copyButton.Unref()
	row.SetTitle(text)
	return &row.Widget
}

// goPushFile pushes the GFile obj, which is borrowed, to peer in a new
// goroutine, holding a reference to it until the push is done.
func (a *App) goPushFile(peer tailcfg.StableNodeID, obj gobject.Ptr) {
	file := &gio.FileBase{Ptr: obj.GoPointer()}
	gutil.Obj(file).Ref()
	// Released by the goroutine.
	go func() {
		a.pushFile(context.TODO(), peer, file)
		gutil.IdleAdd(gutil.Obj(file).Unref)
	}()
}

func routeActionRow(route netip.Prefix) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitleSelectable(true)
	row.SetTitle(route.String())
	return row
}

var listBoxSort gtk.ListBoxSortFunc = func(r1, r2, id uintptr) int32 {
	f := gutil.Held[func(r1, r2 *gtk.ListBoxRow) int](id)
	return int32(f(gtk.ListBoxRowNewFromInternalPtr(r1), gtk.ListBoxRowNewFromInternalPtr(r2)))
}

// setSortFunc sets lb's sort function. It's released when GTK drops it.
func setSortFunc(lb *gtk.ListBox, f func(r1, r2 *gtk.ListBoxRow) int) {
	lb.SetSortFunc(&listBoxSort, gutil.Hold(f), &gutil.FreeData)
}

var listBoxFilter gtk.ListBoxFilterFunc = func(row, id uintptr) bool {
	return gutil.Held[func(*gtk.ListBoxRow) bool](id)(gtk.ListBoxRowNewFromInternalPtr(row))
}

// setFilterFunc sets lb's filter function, releasing it like
// setSortFunc.
func setFilterFunc(lb *gtk.ListBox, f func(row *gtk.ListBoxRow) bool) {
	lb.SetFilterFunc(&listBoxFilter, gutil.Hold(f), &gutil.FreeData)
}
