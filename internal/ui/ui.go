package ui

/*
#cgo pkg-config: libadwaita-1

#include <adwaita.h>

#include "ui.h"

gboolean _idle(gpointer h);

void _class_init(gpointer p);
void _instance_init(gpointer p);
*/
import "C"

import (
	"embed"
	"io/fs"
	"reflect"
	"runtime/cgo"
	"sync"
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

func cstrings(str []string) []*C.char {
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

var types sync.Map

type typeDefinition[Class, Instance any, ClassP interface {
	*Class
	Initter
}, InstanceP interface {
	*Instance
	Initter
}] struct {
	once func() C.GType
}

func (d *typeDefinition[Class, Instance, ClassP, InstanceP]) init() C.GType {
	return d.once()
}

func (d *typeDefinition[Class, Instance, ClassP, InstanceP]) initClass(p C.gpointer) {
	(ClassP)(p).Init()
}

func (d *typeDefinition[Class, Instance, ClassP, InstanceP]) initInstance(p C.gpointer) {
	(InstanceP)(p).Init()
}

type Initter interface {
	Init()
}

func defineType[Class, Instance any, ClassP interface {
	*Class
	Initter
}, InstanceP interface {
	*Instance
	Initter
}](parent C.GType, name string) C.GType {
	definition := typeDefinition[Class, Instance, ClassP, InstanceP]{
		once: sync.OnceValue(func() C.GType {
			cname := C.CString(name)
			defer C.free(unsafe.Pointer(cname))

			var c Class
			var i Instance

			return C.g_type_register_static_simple(
				parent,
				cname,
				C.guint(unsafe.Sizeof(c)),
				(*[0]byte)(C._class_init),
				C.guint(unsafe.Sizeof(i)),
				(*[0]byte)(C._instance_init),
				0,
			)
		}),
	}

	once, _ := types.LoadOrStore(name, &definition)
	return once.(interface{ init() C.GType }).init()
}

//export _class_init
func _class_init(p C.gpointer) {
	cname := C.g_type_name_from_class((*C.GTypeClass)(p))
	d, _ := types.Load(C.GoString(cname))
	d.(interface{ initClass(C.gpointer) }).initClass(p)
}

//export _instance_init
func _instance_init(p C.gpointer) {
	cname := C.g_type_name_from_instance((*C.GTypeInstance)(p))
	d, _ := types.Load(C.GoString(cname))
	d.(interface{ initInstance(C.gpointer) }).initInstance(p)
}
