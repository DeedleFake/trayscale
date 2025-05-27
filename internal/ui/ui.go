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
	"reflect"
	"runtime/cgo"
	"unsafe"

	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
)

func init() {
	C.APP_ID = C.CString(metadata.AppID)
}

//go:embed *.ui *.css
var files embed.FS

func getFile(name string) []byte {
	return must(fs.ReadFile(files, name))
}

//export ui_get_file
func ui_get_file(name *C.char) *C.char {
	return C.CString(string(getFile(C.GoString(name))))
}

//export ui_get_file_bytes
func ui_get_file_bytes(name *C.char) *C.GBytes {
	data := getFile(C.GoString(name))
	return newGBytes(data)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func get[T any](v T, ok bool) T {
	if !ok {
		panic("!ok")
	}
	return v
}

func newGBytes(data []byte) *C.GBytes {
	return C.g_bytes_new(C.gconstpointer(unsafe.SliceData(data)), C.gsize(len(data)))
}

func cbool(v bool) C.gboolean {
	if v {
		return C.TRUE
	}
	return C.FALSE
}

func cfunc(f unsafe.Pointer) *[0]byte {
	return (*[0]byte)(f)
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

func to[T any](val any) *T {
	target := reflect.TypeFor[*T]()

	v := reflect.ValueOf(val)
	for {
		t := v.Type()
		if t == target {
			return (*T)(v.UnsafePointer())
		}

		v = v.Elem().Field(0).Addr()
	}
}

func gtk_widget_class_bind_template_child[T any](gtk_widget_class *C.GtkWidgetClass, name string) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	offset := get(reflect.TypeFor[T]().FieldByName(name)).Offset

	C.gtk_widget_class_bind_template_child_full(gtk_widget_class, cname, C.FALSE, C.gssize(offset))
}

func idle(f func()) {
	C.g_idle_add(cfunc(C._idle), C.gpointer(cgo.NewHandle(f)))
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
