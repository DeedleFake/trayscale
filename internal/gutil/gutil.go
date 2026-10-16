package gutil

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gobject/types"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// ErrHasCode returns true if and only if err is a [glib.Error] and
// its error code is code.
func ErrHasCode(err error, code int) bool {
	gerr, ok := errors.AsType[*glib.Error](err)
	if !ok {
		return false
	}
	return int(gerr.Code) == code
}

// WidgetChildren returns an iterator that performs a pre-order
// traversal of the widget tree rooted at w. It does not yield w
// itself. The yielded widgets are borrowed from their parents.
func WidgetChildren(w *gtk.Widget) iter.Seq[*gtk.Widget] {
	return func(yield func(*gtk.Widget) bool) {
		widgetChildrenPush(yield, w)
	}
}

// widgetChildrenPush holds a reference to the current child and its
// next sibling while yielding, so yield may remove the child.
func widgetChildrenPush(yield func(*gtk.Widget) bool, w *gtk.Widget) bool {
	cur := w.GetFirstChild()
	for cur != nil {
		next := cur.GetNextSibling()
		ok := yield(cur) && widgetChildrenPush(yield, cur)
		cur.Unref()
		if !ok {
			if next != nil {
				next.Unref()
			}
			return false
		}
		cur = next
	}
	return true
}

// ExpanderRowListBox returns the [gtk.ListBox] that is used
// internally by an adw.ExpanderRow to hold the rows that are shown
// when it is expanded. This is quite hacky and should be avoided if
// it can be, but is unfortunately necessary for a couple of things.
// The result is borrowed from row.
func ExpanderRowListBox(row *adw.ExpanderRow) *gtk.ListBox {
	var revealer bool
	for child := range WidgetChildren(&row.Widget) {
		if !revealer {
			revealer = IsA(child, gtk.RevealerGLibType())
			continue
		}

		if box, ok := Cast[gtk.ListBox](child, gtk.ListBoxGLibType()); ok {
			return box
		}
	}

	panic("ExpanderRow ListBox not found")
}

// Cast returns obj as a *T if obj's runtime GType is gtype or
// derives from (or, for an interface, implements) it. It returns false
// for a nil obj or a GType mismatch. No reference is taken: the result
// shares obj's.
//
// gtype must be the GType that T wraps.
func Cast[T any, P interface {
	*T
	gobject.Ptr
}](obj gobject.Ptr, gtype types.GType) (*T, bool) {
	if !IsA(obj, gtype) {
		return nil, false
	}
	v := P(new(T))
	v.SetGoPointer(obj.GoPointer())
	return (*T)(v), true
}

// MustCast is like [Cast] but panics if obj is not a gtype.
func MustCast[T any, P interface {
	*T
	gobject.Ptr
}](obj gobject.Ptr, gtype types.GType) *T {
	v, ok := Cast[T, P](obj, gtype)
	if !ok {
		panic(fmt.Sprintf("gutil.MustCast: %v is not a %v", TypeName(obj), gobject.TypeName(gtype)))
	}
	return v
}

// IsA reports whether obj is non-nil and its runtime GType is gtype or
// derives from (or implements) it.
func IsA(obj gobject.Ptr, gtype types.GType) bool {
	p := goPointer(obj)
	if p == 0 {
		return false
	}
	return gobject.TypeIsA(instanceType(p), gtype)
}

// TypeName returns the name of obj's runtime GType, or "nil".
func TypeName(obj gobject.Ptr) string {
	p := goPointer(obj)
	if p == 0 {
		return "nil"
	}
	return gobject.TypeName(instanceType(p))
}

func goPointer(obj gobject.Ptr) uintptr {
	if obj == nil {
		return 0
	}
	if rv := reflect.ValueOf(obj); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return 0
	}
	return obj.GoPointer()
}

// instanceType reads G_TYPE_FROM_INSTANCE:
// GTypeInstance { GTypeClass *g_class }; GTypeClass { GType g_type }.
func instanceType(p uintptr) types.GType {
	return ***(***types.GType)(unsafe.Pointer(&p))
}

// FirstIconName returns the first of names that theme has. If theme
// has none of them, it returns the first name so that the usual
// missing icon handling applies.
func FirstIconName(theme *gtk.IconTheme, names ...string) string {
	for _, name := range names {
		if theme.HasIcon(name) {
			return name
		}
	}
	return names[0]
}

// Err fixes up an error returned by puregotk. Functions that return an
// object hand back their *glib.Error even when it's nil if the object
// is nil, as in NextFile at the end of an enumeration, giving a non-nil
// error interface holding a nil pointer. Err turns that into nil.
func Err(err error) error {
	if gerr, ok := err.(*glib.Error); ok && gerr == nil {
		return nil
	}
	return err
}

// Obj returns obj as a *gobject.Object sharing its reference, for
// calling GObject methods such as Ref and Unref on interface wrappers.
func Obj(obj gobject.Ptr) *gobject.Object {
	return gobject.ObjectNewFromInternalPtr(obj.GoPointer())
}

// Ptr converts a C pointer received as a uintptr, such as a callback
// argument, to a *T for puregotk's boxed types like glib.Variant.
func Ptr[T any](p uintptr) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
}
