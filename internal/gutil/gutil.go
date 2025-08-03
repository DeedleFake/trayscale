package gutil

import (
	"errors"
	"iter"
	"reflect"

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
	for _, v := range xml {
		builder.AddFromString(v)
	}

	FillFromBuilder(into, builder)
}

// ErrHasCode returns true if and only if err is a [gerror.GError] and
// its error code is code.
func ErrHasCode(err error, code int) bool {
	var gerr *gerror.GError
	if !errors.As(err, &gerr) {
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
	type caster interface{ Cast() glib.Objector }

	var revealer bool
	for child := range WidgetChildren(row) {
		if !revealer {
			_, ok := child.(caster).Cast().(*gtk.Revealer)
			revealer = ok
			continue
		}

		box, ok := child.(caster).Cast().(*gtk.ListBox)
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

// Classy wraps the CSS-related methods of a gtk.Widget.
type Classy interface {
	AddCSSClass(string)
	RemoveCSSClass(string)
	HasCSSClass(string) bool
	CSSClasses() []string
}

// SetCSSClass adds or removes a CSS class based on a boolean
// argument.
func SetCSSClass(w Classy, class string, force bool) {
	if force {
		w.AddCSSClass(class)
		return
	}

	if !force {
		w.RemoveCSSClass(class)
	}
}

// Caster wraps the [glib.Object.Cast] method.
type Caster interface {
	Cast() glib.Objector
}

// Assert casts anything that implements Cast to the specified type.
// Its main point is to safely and conveniently handle nil.
func Assert[T any](obj Caster) (v T, ok bool) {
	if obj == nil {
		return v, false
	}

	v, ok = obj.Cast().(T)
	return v, ok
}
