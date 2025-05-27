package ui

/*
#cgo pkg-config: libadwaita-1

#include <adwaita.h>

#include "ui.h"

gboolean _idle(gpointer h);
*/
import "C"

import (
	"embed"
	"io/fs"
	"runtime/cgo"
	"unsafe"

	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
)

//go:embed *.ui *.css
var files embed.FS

func init() {
	C.APP_ID = C.CString(metadata.AppID)
}

//export ui_get_file
func ui_get_file(name *C.char) *C.char {
	data, err := fs.ReadFile(files, C.GoString(name))
	if err != nil {
		panic(err)
	}

	return C.CString(string(data))
}

//export ui_get_file_bytes
func ui_get_file_bytes(name *C.char) *C.GBytes {
	data, err := fs.ReadFile(files, C.GoString(name))
	if err != nil {
		panic(err)
	}

	return C.g_bytes_new(C.gconstpointer(unsafe.SliceData(data)), C.gsize(len(data)))
}

func cbool(v bool) C.gboolean {
	if v {
		return C.TRUE
	}
	return C.FALSE
}

func toCStrings(str []string) []*C.char {
	cstr := make([]*C.char, 0, len(str))
	for _, s := range str {
		cstr = append(cstr, C.CString(s))
	}
	return cstr
}

func freeAll[T any, P *T](cstr []P) {
	for _, s := range cstr {
		C.free(unsafe.Pointer(s))
	}
}

func idle(f func()) {
	C.g_idle_add((*[0]byte)(C._idle), C.gpointer(cgo.NewHandle(f)))
}

//export _idle
func _idle(p C.gpointer) C.gboolean {
	h := cgo.Handle(p)
	defer h.Delete()

	h.Value().(func())()
	return C.G_SOURCE_REMOVE
}

//export cgo_handle_delete
func cgo_handle_delete(p C.uintptr_t) {
	if p != 0 {
		cgo.Handle(p).Delete()
	}
}

type TSApp interface {
	Poller() *tsutil.Poller
	Tray() *tray.Tray
	Quit()
}

//export ts_app_quit
func ts_app_quit(ts_app C.TsApp) {
	tsApp := cgo.Handle(ts_app).Value().(TSApp)
	tsApp.Quit()
}

//export tsutil_is_ipnstatus
func tsutil_is_ipnstatus(tsutil_status C.TsutilStatus) C.gboolean {
	_, ok := cgo.Handle(tsutil_status).Value().(*tsutil.IPNStatus)
	if ok {
		return C.TRUE
	}
	return C.FALSE
}

//export tsutil_is_filestatus
func tsutil_is_filestatus(tsutil_status C.TsutilStatus) C.gboolean {
	_, ok := cgo.Handle(tsutil_status).Value().(*tsutil.FileStatus)
	if ok {
		return C.TRUE
	}
	return C.FALSE
}

//export tsutil_is_profilestatus
func tsutil_is_profilestatus(tsutil_status C.TsutilStatus) C.gboolean {
	_, ok := cgo.Handle(tsutil_status).Value().(*tsutil.ProfileStatus)
	if ok {
		return C.TRUE
	}
	return C.FALSE
}

//export tsutil_ipnstatus_online
func tsutil_ipnstatus_online(tsutil_status C.TsutilStatus) C.gboolean {
	ipnstatus := cgo.Handle(tsutil_status).Value().(*tsutil.IPNStatus)
	if ipnstatus.Online() {
		return C.TRUE
	}
	return C.FALSE
}
