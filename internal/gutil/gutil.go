package gutil

import (
	"errors"
	"fmt"
	"iter"
	"reflect"

	"deedles.dev/trayscale/internal/locale"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// FillFromBuilder sets fields in the struct dst using objects looked
// up in builder. If the field has a `gtk` tag, the value of it is
// used as the name to look up in builder, unless the tag is exactly
// `"-"` in which case the field is skipped. If it does not have a
// tag, the field name is used instead. If an object by that name does
// not exist in the builder, it is quietly skipped. If it does exist
// but is the wrong type, this function will panic.
//
// For example,
//
//	var example struct {
//		Label *gtk.Label
//		Button *gtk.Button `gtk:"DoneButton"`
//	}
//	FillFromBuilder(&example, builder)
//
// will cause the Label field to get filled with the object named
// "Label" and the Button field to get filled with the object named
// "DoneButton".
func FillFromBuilder[T any](dst *T, builder *gtk.Builder) {
	v := reflect.ValueOf(dst).Elem()
	t := v.Type()

	for i := range t.NumField() {
		fv := v.Field(i)
		ft := t.Field(i)

		name := ft.Name
		if tag, ok := ft.Tag.Lookup("gtk"); ok {
			if tag == "-" {
				continue
			}
			name = tag
		}
		obj := builder.GetObject(name)
		if obj == nil {
			continue
		}

		fv.Set(reflect.ValueOf(obj.Cast()))
	}
}

// FillFromUI loads the given xml data in the order that it is passed
// and fills into with it in the same way that [FillFromBuilder] does.
// Each xml value should be an entire XML UI definition file. This is
// mostly a convenience function to make it easier to load widgets
// from embedded files.
func FillFromUI[T any](into *T, xml ...string) {
	builder := gtk.NewBuilder()
	builder.SetTranslationDomain(locale.TranslationDomain)
	for _, v := range xml {
		builder.AddFromString(v)
	}

	FillFromBuilder(into, builder)
}

// ErrHasCode returns true if and only if err is a [gerror.GError] and
// its error code is code.
func ErrHasCode(err error, code int) bool {
	gerr, ok := errors.AsType[*gerror.GError](err)
	if !ok {
		return false
	}
	return gerr.ErrorCode() == code
}

// WidgetParent is any type that has child widgets. Gererally
// speaking, this is every Gtk widget.
type WidgetParent interface {
	FirstChild() gtk.Widgetter
}

// WidgetChildren returns an iterator that performs a pre-order
// traversal of the widget tree rooted at w. It does not yield w
// itself.
func WidgetChildren(w WidgetParent) iter.Seq[gtk.Widgetter] {
	return func(yield func(gtk.Widgetter) bool) {
		widgetChildrenPush(yield, w)
	}
}

func widgetChildrenPush(yield func(gtk.Widgetter) bool, w WidgetParent) bool {
	type siblingNexter interface{ NextSibling() gtk.Widgetter }

	cur := w.FirstChild()
	for cur != nil {
		if !yield(cur) {
			return false
		}
		if !widgetChildrenPush(yield, cur.(WidgetParent)) {
			return false
		}

		cur = cur.(siblingNexter).NextSibling()
	}

	return true
}

// ExpanderRowListBox returns the [gtk.ListBox] that is used
// internally by an adw.ExpanderRow to hold the rows that are shown
// when it is expanded. This is quite hacky and should be avoided if
// it can be, but is unfortunately necessary for a couple of things.
func ExpanderRowListBox(row *adw.ExpanderRow) *gtk.ListBox {
	var revealer bool
	for child := range WidgetChildren(row) {
		if !revealer {
			_, revealer = Cast[*gtk.Revealer](child, gtk.GTypeRevealer)
			continue
		}

		box, ok := Cast[*gtk.ListBox](child, gtk.GTypeListBox)
		if ok {
			return box
		}
	}

	panic("ExpanderRow ListBox not found")
}

// PointerToWidgetter converts a *T that implements gtk.Widgetter to
// a gtk.Widgetter, returning nil if the *T is nil. This avoids the
// nil interface problem.
func PointerToWidgetter[T any, P interface {
	gtk.Widgetter
	*T
}](p P) gtk.Widgetter {
	if p == nil {
		return nil
	}
	return p
}

// Cast returns obj as T if obj's runtime GType is gtype or derives
// from (or, for an interface, implements) it. It returns false for a
// nil obj or a GType mismatch. It never relies on the bindings guessing
// the Go wrapper type, so it maps directly onto bindings whose casts
// are unchecked: check the GType, then convert.
//
// gtype must be the GType that T wraps. If the GType matches but obj
// can't be converted to T, the two disagree, which is a bug, so Cast
// panics.
func Cast[T any](obj glib.Objector, gtype glib.Type) (T, bool) {
	var zero T
	base := baseObject(obj)
	if base == nil {
		return zero, false
	}
	if !base.Type().IsA(gtype) {
		return zero, false
	}
	if v, ok := obj.(T); ok {
		return v, true
	}
	v, ok := base.Cast().(T)
	if !ok {
		panic(fmt.Sprintf("gutil.Cast: %v is a %v but its wrapper %T is not %T", base.Type(), gtype, base.Cast(), zero))
	}
	return v, true
}

// MustCast is like [Cast] but panics if obj is not a gtype.
func MustCast[T any](obj glib.Objector, gtype glib.Type) T {
	v, ok := Cast[T](obj, gtype)
	if !ok {
		var zero T
		panic(fmt.Sprintf("gutil.MustCast: %v is not a %v (%T)", typeName(obj), gtype, zero))
	}
	return v
}

func typeName(obj glib.Objector) string {
	base := baseObject(obj)
	if base == nil {
		return "nil"
	}
	return base.Type().String()
}

// baseObject returns obj's underlying object, or nil if obj is nil,
// a typed nil pointer, or wraps no instance.
func baseObject(obj glib.Objector) *glib.Object {
	if obj == nil {
		return nil
	}
	if rv := reflect.ValueOf(obj); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil
	}
	base := glib.BaseObject(obj)
	if base == nil || base.Native() == 0 {
		return nil
	}
	return base
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
