package ui

/*
#include <adwaita.h>
#include "ui.h"

void _g_object_dispose(GObject *cobj);
*/
import "C"

import (
	"slices"
	"unsafe"
)

func GValueFromString(str string) C.GValue {
	cstr := C.CString(str)
	defer C.free(unsafe.Pointer(cstr))

	var val C.GValue
	C.g_value_init(&val, 16<<2)
	C.g_value_set_string(&val, cstr)
	return val
}

type GType[T any] struct {
	_ [unsafe.Sizeof(*new(C.GType))]byte
}

func ToGType[T any](c C.GType) GType[T] {
	return *(*GType[T])(unsafe.Pointer(&c))
}

func (t *GType[T]) c() C.GType {
	return *(*C.GType)(unsafe.Pointer(t))
}

func (t GType[T]) New(props ...any) *T {
	names := make([]*C.char, 0, len(props)/2)
	vals := make([]C.GValue, 0, len(props)/2)
	for n, v := range pairs(slices.Values(props)) {
		names = append(names, C.CString(n.(string)))
		vals = append(vals, v.(C.GValue))
	}

	return (*T)(unsafe.Pointer(C.g_object_new_with_properties(
		t.c(),
		C.guint(len(names)),
		(**C.char)(unsafe.SliceData(names)),
		(*C.GValue)(unsafe.SliceData(vals)),
	)))
}

func (t GType[T]) Cast(obj *GTypeInstance) *T {
	target := C.g_type_from_name(C.g_type_name_from_instance(obj.c()))
	switch {
	case C.g_type_is_a(t.c(), target) == 0, C.g_type_is_a(target, t.c()) == 0:
		panic("type is not convertible")
	default:
		return (*T)(unsafe.Pointer(obj))
	}
}

type GTypeInstance struct {
	_ [unsafe.Sizeof(*new(C.GTypeInstance))]byte
}

func (ti *GTypeInstance) c() *C.GTypeInstance {
	return (*C.GTypeInstance)(unsafe.Pointer(ti))
}

func (ti *GTypeInstance) AsGTypeInstance() *GTypeInstance { return ti }

type GObjectClass struct {
	_ [unsafe.Sizeof(*new(C.GObjectClass))]byte
}

func (class *GObjectClass) c() *C.GObjectClass {
	return (*C.GObjectClass)(unsafe.Pointer(class))
}

func (class *GObjectClass) AsGObjectClass() *GObjectClass { return class }

//export _g_object_dispose
func _g_object_dispose(cobj *C.GObject) {
}

func (class *GObjectClass) SetDispose(dispose func(*GObject)) {
	class.c().dispose = cfunc(C._g_object_dispose)
}

type GObject struct {
	GTypeInstance
	_ [unsafe.Sizeof(*new(C.GObject)) - unsafe.Sizeof(*new(C.GTypeInstance))]byte
}

func (obj *GObject) c() *C.GObject {
	return (*C.GObject)(unsafe.Pointer(obj))
}

func (obj *GObject) AsGObject() *GObject { return obj }

func (obj *GObject) Ref() {
	C.g_object_ref(C.gpointer(obj.c()))
}

func (obj *GObject) Unref() {
	C.g_object_unref(C.gpointer(obj.c()))
}

var TypeGApplication = ToGType[GApplication](C.g_application_get_type())

type GApplicationClass struct {
	GObjectClass
	_ [unsafe.Sizeof(*new(C.GApplicationClass)) - unsafe.Sizeof(*new(C.GObjectClass))]byte
}

func (class *GApplicationClass) c() *C.GApplicationClass {
	return (*C.GApplicationClass)(unsafe.Pointer(class))
}

func (class *GApplicationClass) AsGApplicationClass() *GApplicationClass { return class }

type GApplication struct {
	GObject
	_ [unsafe.Sizeof(*new(C.GApplication)) - unsafe.Sizeof(*new(C.GObject))]byte
}

func (app *GApplication) c() *C.GApplication {
	return (*C.GApplication)(unsafe.Pointer(app))
}

func (app *GApplication) AsGApplication() *GApplication { return app }

func (app *GApplication) Run(args []string) {
	cargs := cstrings(args)
	defer freeAll(cargs)

	C.g_application_run(app.c(), C.int(len(cargs)), unsafe.SliceData(cargs))
}

func (app *GApplication) Quit() {
	C.g_application_quit(app.c())
}

func (app *GApplication) SendNotification(id string, notification *GNotification) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))

	C.g_application_send_notification(app.c(), cid, notification.c())
}

var TypeAdwApplication = ToGType[AdwApplication](C.adw_application_get_type())

type AdwApplicationClass struct {
	GApplicationClass
	_ [unsafe.Sizeof(*new(C.AdwApplicationClass)) - unsafe.Sizeof(*new(C.GApplicationClass))]byte
}

func (class *AdwApplicationClass) c() *C.AdwApplicationClass {
	return (*C.AdwApplicationClass)(unsafe.Pointer(class))
}

func (class *AdwApplicationClass) AsAdwApplicationClass() *AdwApplicationClass { return class }

type AdwApplication struct {
	GApplication
	_ [unsafe.Sizeof(*new(C.AdwApplication)) - unsafe.Sizeof(*new(C.GApplication))]byte
}

func (app *AdwApplication) c() *C.AdwApplication {
	return (*C.AdwApplication)(unsafe.Pointer(app))
}

func (app *AdwApplication) AsAdwApplication() *AdwApplication { return app }

var TypeGNotification = ToGType[GNotification](C.g_notification_get_type())

type GNotification struct {
	GObject
	//_ [unsafe.Sizeof(*new(C.GNotification)) - unsafe.Sizeof(*new(C.GObject))]byte
}

func GNotificationNew(title string) *GNotification {
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))

	return (*GNotification)(unsafe.Pointer(C.g_notification_new(ctitle)))
}

func (n *GNotification) c() *C.GNotification {
	return (*C.GNotification)(unsafe.Pointer(n))
}

func (n *GNotification) AsGNotification() *GNotification { return n }

func (n *GNotification) SetBody(body string) {
	cbody := C.CString(body)
	defer C.free(unsafe.Pointer(cbody))

	C.g_notification_set_body(n.c(), cbody)
}
