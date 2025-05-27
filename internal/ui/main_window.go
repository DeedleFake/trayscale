package ui

/*
#include <adwaita.h>

#include "ui.h"
#include "app.h"
#include "main_window.h"
*/
import "C"

import (
	"context"
	"log/slog"
	"runtime/cgo"
	"time"
	"unsafe"

	"deedles.dev/trayscale/internal/ctxutil"
	"deedles.dev/trayscale/internal/tsutil"
)

var (
	str_main_menu = C.CString("main_menu")
	str_page_menu = C.CString("page_menu")
)

func init() {
	menu_ui := C.CString(string(getFile("menu.ui")))
	defer C.free(unsafe.Pointer(menu_ui))

	gtk_builder := C.gtk_builder_new_from_string(menu_ui, -1)
	defer C.g_object_unref(C.gpointer(gtk_builder))

	C.menu_model_main = (*C.GMenuModel)(unsafe.Pointer(C.gtk_builder_get_object(gtk_builder, str_main_menu)))
	C.menu_model_page = (*C.GMenuModel)(unsafe.Pointer(C.gtk_builder_get_object(gtk_builder, str_page_menu)))

	C.g_object_ref(C.gpointer(C.menu_model_main))
	C.g_object_ref(C.gpointer(C.menu_model_page))
}

//export ui_main_window_status_switch_state_set
func ui_main_window_status_switch_state_set(gtk_switch *C.GtkSwitch, state C.gboolean, ui_main_window *C.UiMainWindow) C.gboolean {
	if state == C.gtk_switch_get_state(gtk_switch) {
		return C.FALSE
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	f := tsutil.Stop
	if state != 0 {
		f = tsutil.Start
	}

	err := f(ctx)
	if err != nil {
		slog.Error("failed to set Tailscale status", "err", err)
		C.gtk_switch_set_active(gtk_switch, ^state)
		return C.TRUE
	}

	tsApp := (*App)(ui_main_window.ui_app).tsApp()
	ctxutil.Recv(ctx, tsApp.Poller().Poll())
	return C.TRUE
}

//export ui_main_window_update
func ui_main_window_update(ui_app *App, tsutil_status C.TsutilStatus, ui_main_window *C.UiMainWindow) {
	switch status := cgo.Handle(tsutil_status).Value().(type) {
	case *tsutil.IPNStatus:
		online := status.Online()
		C.gtk_switch_set_state(ui_main_window.status_switch, cbool(online))
		C.gtk_switch_set_active(ui_main_window.status_switch, cbool(online))

		//ui_main_window_update_peers(ui_main_window, status)
	}
}
