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
	"deedles.dev/trayscale/internal/locale"
)

// FillFromBuilder sets fields in the struct dst using objects looked
// up in builder. If the field has a `gtk` tag, the value of it is
// used as the name to look up in builder, unless the tag is exactly
// `"-"` in which case the field is skipped. If it does not have a
// tag, the field name is used instead. If an object by that name does
// not exist in the builder, it is quietly skipped. If it does exist
// but is the wrong type, this function will panic.
//
// Only exported pointer fields to puregotk wrappers are considered.
// puregotk's casts are unchecked, so a field's type must be registered
// with [RegisterType] for its GType to be checked; filling an
// unregistered type panics.
//
// Each stored object gets its own reference, so dst stays valid after
// builder is released. Release them with [UnrefFields].
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
	for fv, name := range builderFields(dst) {
		obj := builder.GetObject(name)
		if obj == nil {
			continue
		}

		et := fv.Type().Elem()
		gtype, ok := typeRegistry[et]
		if !ok {
			panic(fmt.Sprintf("gutil.FillFromBuilder: no GType registered for %v (field for %q)", et, name))
		}
		if !IsA(obj, gtype()) {
			panic(fmt.Sprintf("gutil.FillFromBuilder: %q is a %v, want %v", name, TypeName(obj), et))
		}

		obj.Ref()
		nv := reflect.New(et)
		nv.Interface().(gobject.Ptr).SetGoPointer(obj.GoPointer())
		fv.Set(nv)
	}
}

// UnrefFields drops the references [FillFromBuilder] took for dst's
// fields and clears them.
func UnrefFields[T any](dst *T) {
	for fv := range builderFields(dst) {
		if fv.IsNil() {
			continue
		}
		p := fv.Interface().(gobject.Ptr).GoPointer()
		if p != 0 {
			gobject.ObjectNewFromInternalPtr(p).Unref()
		}
		fv.SetZero()
	}
}

func builderFields[T any](dst *T) iter.Seq2[reflect.Value, string] {
	return func(yield func(reflect.Value, string) bool) {
		v := reflect.ValueOf(dst).Elem()
		t := v.Type()
		for i := range t.NumField() {
			ft := t.Field(i)
			if !ft.IsExported() || ft.Type.Kind() != reflect.Pointer || !ft.Type.Implements(ptrType) {
				continue
			}
			name := ft.Name
			if tag, ok := ft.Tag.Lookup("gtk"); ok {
				if tag == "-" {
					continue
				}
				name = tag
			}
			if !yield(v.Field(i), name) {
				return
			}
		}
	}
}

var ptrType = reflect.TypeFor[gobject.Ptr]()

var typeRegistry = map[reflect.Type]func() types.GType{}

// RegisterType records the GType of the puregotk wrapper T so that
// [FillFromBuilder] can check and fill fields of type *T.
func RegisterType[T any](gtype func() types.GType) {
	typeRegistry[reflect.TypeFor[T]()] = gtype
}

func init() {
	RegisterType[gtk.Box](gtk.BoxGLibType)
	RegisterType[gtk.Button](gtk.ButtonGLibType)
	RegisterType[gtk.DropDown](gtk.DropDownGLibType)
	RegisterType[gtk.DropTarget](gtk.DropTargetGLibType)
	RegisterType[gtk.Image](gtk.ImageGLibType)
	RegisterType[gtk.Label](gtk.LabelGLibType)
	RegisterType[gtk.ListBox](gtk.ListBoxGLibType)
	RegisterType[gtk.MenuButton](gtk.MenuButtonGLibType)
	RegisterType[gtk.ScrolledWindow](gtk.ScrolledWindowGLibType)
	RegisterType[gtk.SearchBar](gtk.SearchBarGLibType)
	RegisterType[gtk.SearchEntry](gtk.SearchEntryGLibType)
	RegisterType[gtk.Adjustment](gtk.AdjustmentGLibType)
	RegisterType[gtk.Window](gtk.WindowGLibType)
	RegisterType[adw.Spinner](adw.SpinnerGLibType)
	RegisterType[adw.ViewSwitcherSidebar](adw.ViewSwitcherSidebarGLibType)
	RegisterType[gtk.Switch](gtk.SwitchGLibType)
	RegisterType[gtk.ToggleButton](gtk.ToggleButtonGLibType)
	RegisterType[adw.ActionRow](adw.ActionRowGLibType)
	RegisterType[adw.ApplicationWindow](adw.ApplicationWindowGLibType)
	RegisterType[adw.ButtonRow](adw.ButtonRowGLibType)
	RegisterType[adw.ComboRow](adw.ComboRowGLibType)
	RegisterType[adw.EntryRow](adw.EntryRowGLibType)
	RegisterType[adw.ExpanderRow](adw.ExpanderRowGLibType)
	RegisterType[adw.NavigationSplitView](adw.NavigationSplitViewGLibType)
	RegisterType[adw.PreferencesDialog](adw.PreferencesDialogGLibType)
	RegisterType[adw.PreferencesGroup](adw.PreferencesGroupGLibType)
	RegisterType[adw.PreferencesPage](adw.PreferencesPageGLibType)
	RegisterType[adw.SpinRow](adw.SpinRowGLibType)
	RegisterType[adw.StatusPage](adw.StatusPageGLibType)
	RegisterType[adw.SwitchRow](adw.SwitchRowGLibType)
	RegisterType[adw.ToastOverlay](adw.ToastOverlayGLibType)
	RegisterType[adw.ViewStack](adw.ViewStackGLibType)
}

// FillFromUI loads the given xml data in the order that it is passed
// and fills into with it in the same way that [FillFromBuilder] does.
// Each xml value should be an entire XML UI definition file. Invalid
// XML is a programming error and panics. The builder is released
// before returning.
func FillFromUI[T any](into *T, xml ...string) {
	builder := gtk.NewBuilder()
	defer builder.Unref()

	builder.SetTranslationDomain(locale.TranslationDomain)
	for _, v := range xml {
		if _, err := builder.AddFromString(v, -1); err != nil {
			panic(err)
		}
	}

	FillFromBuilder(into, builder)
}

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

func widgetChildrenPush(yield func(*gtk.Widget) bool, w *gtk.Widget) bool {
	for cur := w.GetFirstChild(); cur != nil; cur = cur.GetNextSibling() {
		if !yield(cur) {
			return false
		}
		if !widgetChildrenPush(yield, cur) {
			return false
		}
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
