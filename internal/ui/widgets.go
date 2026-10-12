package ui

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
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
	return gutil.MustCast[gtk.Switch](row.GetActivatableWidget(), gtk.SwitchGLibType())
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
	go func() {
		a.pushFile(context.TODO(), peer, file)
		gutil.IdleAdd(gutil.Obj(file).Unref)
	}()
}

// routeRow returns a new row for an advertised route with a button that
// stops advertising it. The caller owns the row.
func routeRow(model *listmodels.Model[netip.Prefix], route netip.Prefix) *gtk.Widget {
	removeButton := gtk.NewButtonFromIconName("list-remove-symbolic")
	removeButton.SetMarginTop(12)
	removeButton.SetMarginBottom(12)
	removeButton.SetHasFrame(false)
	removeButton.SetTooltipText(locale.Get("Remove"))
	gutil.Connect(removeButton, removeButton.ConnectClicked, func(gtk.Button) {
		routes := slices.Collect(xiter.Filter(model.All(), func(p netip.Prefix) bool {
			return p.Compare(route) != 0
		}))
		err := tsutil.AdvertiseRoutes(context.TODO(), routes)
		if err != nil {
			slog.Error("advertise routes", "err", err)
			return
		}
	})

	row := adw.NewActionRow()
	row.SetTitleSelectable(true)
	row.AddSuffix(&removeButton.Widget)
	removeButton.Unref()
	row.SetTitle(route.String())
	return &row.Widget
}

// setSortFunc sets lb's sort function. The callbacks are freed when
// GTK drops the function (replaced or the list box destroyed).
func setSortFunc(lb *gtk.ListBox, f func(r1, r2 *gtk.ListBoxRow) int) {
	var sort gtk.ListBoxSortFunc = func(r1, r2, _ uintptr) int32 {
		return int32(f(gtk.ListBoxRowNewFromInternalPtr(r1), gtk.ListBoxRowNewFromInternalPtr(r2)))
	}
	var destroy glib.DestroyNotify
	destroy = func(uintptr) {
		glib.UnrefCallback(&sort)
		glib.UnrefCallback(&destroy)
	}
	lb.SetSortFunc(&sort, 0, &destroy)
}

// setFilterFunc sets lb's filter function, freeing the callbacks like
// setSortFunc.
func setFilterFunc(lb *gtk.ListBox, f func(row *gtk.ListBoxRow) bool) {
	var filter gtk.ListBoxFilterFunc = func(row, _ uintptr) bool {
		return f(gtk.ListBoxRowNewFromInternalPtr(row))
	}
	var destroy glib.DestroyNotify
	destroy = func(uintptr) {
		glib.UnrefCallback(&filter)
		glib.UnrefCallback(&destroy)
	}
	lb.SetFilterFunc(&filter, 0, &destroy)
}
