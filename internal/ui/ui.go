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
	"iter"
	"reflect"
	"runtime/cgo"
	"sync"
	"unsafe"

	"deedles.dev/trayscale/internal/metadata"
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

func pairs[T any](seq iter.Seq[T]) iter.Seq2[T, T] {
	return func(yield func(T, T) bool) {
		var prev *T
		for v := range seq {
			if prev == nil {
				prev = &v
				continue
			}

			if !yield(*prev, v) {
				return
			}
			prev = nil
		}
		if prev != nil {
			panic("odd length of paris")
		}
	}
}

var types sync.Map

type typeDefinition[Class, Instance any, ClassP interface {
	*Class
	Initter
}, InstanceP interface {
	*Instance
	Initter
}] struct {
	once func() GType[Instance]
}

func (d *typeDefinition[Class, Instance, ClassP, InstanceP]) init() GType[Instance] {
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

func DefineType[Class, Instance any, ClassP interface {
	*Class
	Initter
}, InstanceP interface {
	*Instance
	Initter
}, ParentType any](parent GType[ParentType], name string) GType[Instance] {
	definition := typeDefinition[Class, Instance, ClassP, InstanceP]{
		once: sync.OnceValue(func() GType[Instance] {
			cname := C.CString(name)
			defer C.free(unsafe.Pointer(cname))

			var c Class
			var i Instance

			return ToGType[Instance](C.g_type_register_static_simple(
				parent.c(),
				cname,
				C.guint(unsafe.Sizeof(c)),
				(*[0]byte)(C._class_init),
				C.guint(unsafe.Sizeof(i)),
				(*[0]byte)(C._instance_init),
				0,
			))
		}),
	}

	once, _ := types.LoadOrStore(name, &definition)
	return once.(interface{ init() GType[Instance] }).init()
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
