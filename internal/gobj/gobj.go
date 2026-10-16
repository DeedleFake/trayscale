// Package gobj registers Go-backed GObject types. It is the only place
// that touches type registration, class vfuncs, and the map from
// instance pointers to Go values.
package gobj

import (
	"cmp"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gobject/types"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

// Base is embedded in every Go-backed widget. gobj sets it before any
// Go code sees the struct.
type Base struct{ ptr uintptr }

func (b *Base) base() *Base { return b }

// GoPointer returns the instance pointer, so a *T can be passed where
// puregotk wants a gobject.Ptr.
func (b *Base) GoPointer() uintptr { return b.ptr }

// SetGoPointer exists to satisfy gobject.Ptr. Changing the instance of
// a Go-backed widget makes no sense, so it panics.
func (b *Base) SetGoPointer(uintptr) { panic("gobj: SetGoPointer on a Go-backed widget") }

// Widget returns the instance as a borrowed *gtk.Widget.
func (b *Base) Widget() *gtk.Widget { return gtk.WidgetNewFromInternalPtr(b.ptr) }

// Disposer is optional. Dispose runs once, from the class dispose,
// while template children are still alive.
type Disposer interface{ Dispose() }

// WidgetSpec describes one widget subclass. Template is a full .ui
// document whose <template class> must equal Name.
type WidgetSpec struct {
	Name     string
	Parent   func() types.GType
	Template string
}

type child struct {
	field int
	name  string
	gtype string
}

type instance[T any] struct {
	v        *T
	disposed bool
}

// Class is a registered widget type whose instances carry a T.
type Class[T any] struct {
	spec     WidgetSpec
	children []child
	once     sync.Once
	gtype    types.GType
	parent   *gobject.ObjectClass
	checked  bool

	mu        sync.Mutex
	instances map[uintptr]*instance[T]
}

var classes sync.Map // reflect.Type -> *Class[T]

// Widget declares T as a widget subclass. The type is registered on
// first use, so declaration order does not matter. T must embed Base
// and must be declared once. T's exported pointer fields to puregotk
// object types are filled with the template child named by their `gtk`
// tag, or by the field name if untagged, borrowed from the widget. A
// tag of "-" skips the field.
func Widget[T any, P interface {
	*T
	base() *Base
}](spec WidgetSpec) *Class[T] {
	c := &Class[T]{spec: spec, instances: map[uintptr]*instance[T]{}}
	rt := reflect.TypeFor[T]()
	for i := range rt.NumField() {
		f := rt.Field(i)
		name := cmp.Or(f.Tag.Get("gtk"), f.Name)
		if !f.IsExported() || name == "-" || f.Type.Kind() != reflect.Pointer {
			continue
		}
		gtype, ok := typeName(f.Type.Elem())
		if !ok {
			continue
		}
		c.children = append(c.children, child{field: i, name: name, gtype: gtype})
	}
	if _, dup := classes.LoadOrStore(rt, c); dup {
		panic(fmt.Sprintf("gobj: %v declared twice", rt))
	}
	return c
}

var prefixes = map[string]string{
	"codeberg.org/puregotk/puregotk/v4/gtk":     "Gtk",
	"codeberg.org/puregotk/puregotk/v4/adw":     "Adw",
	"codeberg.org/puregotk/puregotk/v4/gio":     "G",
	"codeberg.org/puregotk/puregotk/v4/gobject": "G",
}

// typeName maps a puregotk wrapper type to its GType name, which
// puregotk derives the same way, so no hand-kept registry is needed.
func typeName(t reflect.Type) (string, bool) {
	p, ok := prefixes[t.PkgPath()]
	return p + t.Name(), ok
}

// GType returns the registered type, registering it if needed.
func (c *Class[T]) GType() types.GType {
	c.once.Do(c.register)
	return c.gtype
}

func (c *Class[T]) register() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, _ uintptr) {
		c.parent = (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
		wc := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		// SetTemplate doesn't take our reference: GTK keeps a precompiled
		// copy, so the bytes must be dropped here.
		tmpl := glib.NewBytes([]byte(c.spec.Template), uint(len(c.spec.Template)))
		wc.SetTemplate(tmpl)
		tmpl.Unref()
		for _, ch := range c.children {
			wc.BindTemplateChildFull(ch.name, false, 0)
		}
		oc := (*gobject.ObjectClass)(unsafe.Pointer(tc))
		oc.OverrideDispose(c.dispose)
		oc.OverrideFinalize(c.finalize)
	}
	var instInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, _ *gobject.TypeClass) {
		c.init(ti.GoPointer())
	}
	var q gobject.TypeQuery
	gobject.NewTypeQuery(c.spec.Parent(), &q)
	c.gtype = gobject.TypeRegisterStaticSimple(q.Type, c.spec.Name, q.ClassSize, &classInit, q.InstanceSize, &instInit, 0)
	if c.gtype == 0 {
		panic(fmt.Sprintf("gobj: registering %s on %s (class %d, instance %d) failed", c.spec.Name, gobject.TypeName(q.Type), q.ClassSize, q.InstanceSize))
	}
}

func (c *Class[T]) init(p uintptr) {
	w := gtk.WidgetNewFromInternalPtr(p)
	w.InitTemplate()

	v := new(T)
	any(v).(interface{ base() *Base }).base().ptr = p
	rv := reflect.ValueOf(v).Elem()
	for _, ch := range c.children {
		obj := w.GetTemplateChild(c.gtype, ch.name)
		if obj == nil {
			panic(fmt.Sprintf("gobj: %s template has no child %q", c.spec.Name, ch.name))
		}
		if !c.checked {
			want := gobject.TypeFromName(ch.gtype)
			if want == 0 || !gutil.IsA(obj, want) {
				panic(fmt.Sprintf("gobj: %s child %q is a %s, want %s", c.spec.Name, ch.name, gutil.TypeName(obj), ch.gtype))
			}
		}
		fv := rv.Field(ch.field)
		nv := reflect.New(fv.Type().Elem())
		nv.Interface().(gobject.Ptr).SetGoPointer(obj.GoPointer())
		fv.Set(nv)
		// puregotk refs the transfer-none result. The template keeps
		// the child alive until dispose, so the field borrows it.
		obj.Unref()
	}
	c.checked = true

	c.mu.Lock()
	c.instances[p] = &instance[T]{v: v}
	c.mu.Unlock()
}

func (c *Class[T]) dispose(o *gobject.Object) {
	p := o.GoPointer()
	c.mu.Lock()
	inst := c.instances[p]
	first := inst != nil && !inst.disposed
	if first {
		inst.disposed = true
	}
	c.mu.Unlock()
	if first {
		if d, ok := any(inst.v).(Disposer); ok {
			d.Dispose()
		}
	}
	// No gtk_widget_dispose_template: it would unparent and free a
	// child that a parent such as AdwBin still points to. GtkWidget's
	// own dispose drops the template's references after the parent
	// class has released its children.
	c.parent.GetDispose()(o)
}

func (c *Class[T]) finalize(o *gobject.Object) {
	c.mu.Lock()
	delete(c.instances, o.GoPointer())
	c.mu.Unlock()
	c.parent.GetFinalize()(o)
}

// New returns a new instance. The caller owns one strong, non-floating
// reference to it.
func (c *Class[T]) New() *T {
	o := gobject.NewObjectWithProperties(c.GType(), 0, nil, nil)
	switch {
	case o.IsFloating():
		// puregotk's RefSink takes an extra reference on top of the
		// sunk floating one.
		o.RefSink().Unref()
	case gobject.TypeIsA(c.gtype, gobject.InitiallyUnownedGLibType()):
		// Already sunk by its own init, as GtkWindow is, so the only
		// reference belongs to GTK.
		o.Ref()
	}
	v, _ := c.lookup(o.GoPointer())
	return v
}

func (c *Class[T]) lookup(p uintptr) (*T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	inst, ok := c.instances[p]
	if !ok {
		return nil, false
	}
	return inst.v, true
}

// Lookup returns the Go value for a borrowed instance of T, or false
// if obj is nil or not a T.
func Lookup[T any](obj gobject.Ptr) (*T, bool) {
	if obj == nil || reflect.ValueOf(obj).IsNil() || obj.GoPointer() == 0 {
		return nil, false
	}
	c, ok := classes.Load(reflect.TypeFor[T]())
	if !ok {
		return nil, false
	}
	return c.(*Class[T]).lookup(obj.GoPointer())
}

// live reports the number of instances whose Go value is still held.
// For tests.
func (c *Class[T]) live() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.instances)
}
