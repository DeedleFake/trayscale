package gobj

import (
	"iter"
	"slices"
	"sync"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gobject/types"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

// Every List shares one item type. Each item's Go value lives in
// items, keyed by the item's instance pointer, and is deleted when the
// item is finalized, so no item costs a callback slot.
var items = struct {
	once   sync.Once
	gtype  types.GType
	parent *gobject.ObjectClass

	sync.Mutex
	values map[uintptr]any
}{values: map[uintptr]any{}}

func itemType() types.GType {
	items.once.Do(func() {
		var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, _ uintptr) {
			items.parent = (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			(*gobject.ObjectClass)(unsafe.Pointer(tc)).OverrideFinalize(func(o *gobject.Object) {
				items.Lock()
				delete(items.values, o.GoPointer())
				items.Unlock()
				items.parent.GetFinalize()(o)
			})
		}
		var instInit gobject.InstanceInitFunc = func(*gobject.TypeInstance, *gobject.TypeClass) {}
		var q gobject.TypeQuery
		gobject.NewTypeQuery(gobject.ObjectGLibType(), &q)
		items.gtype = gobject.TypeRegisterStaticSimple(q.Type, "TrayscaleListItem", q.ClassSize, &classInit, q.InstanceSize, &instInit, 0)
	})
	return items.gtype
}

func newItem(v any) *gobject.Object {
	o := gobject.NewObjectWithProperties(itemType(), 0, nil, nil)
	items.Lock()
	items.values[o.GoPointer()] = v
	items.Unlock()
	return o
}

func setItem(p uintptr, v any) {
	items.Lock()
	items.values[p] = v
	items.Unlock()
}

// Item returns the value carried by an item of a List's model, or of a
// model wrapping it.
func Item[T any](obj gobject.Ptr) T {
	return itemAt[T](obj.GoPointer())
}

func itemAt[T any](p uintptr) T {
	items.Lock()
	defer items.Unlock()
	return items.values[p].(T)
}

// List is a GListModel of Go values, kept sorted by cmp. Values that
// compare equal are the same entry.
//
// The List owns its store; call Unref when done with it.
type List[T comparable] struct {
	store *gio.ListStore
	cmp   func(T, T) int
}

// NewList returns an empty List ordered by cmp.
func NewList[T comparable](cmp func(T, T) int) *List[T] {
	return &List[T]{store: gio.NewListStore(itemType()), cmp: cmp}
}

// Unref drops the List's reference to its store.
func (l *List[T]) Unref() { l.store.Unref() }

// Model returns the underlying list model, borrowed from l.
func (l *List[T]) Model() gio.ListModel { return l.store }

// Len returns the number of entries.
func (l *List[T]) Len() int { return int(l.store.GetNItems()) }

func (l *List[T]) at(i uint32) (uintptr, T) {
	p := l.store.GetItem(i)
	v := itemAt[T](p)
	gobject.ObjectNewFromInternalPtr(p).Unref()
	return p, v
}

// All yields the values in order.
func (l *List[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := range l.store.GetNItems() {
			if _, v := l.at(i); !yield(v) {
				return
			}
		}
	}
}

// Set makes values the contents of l. Entries that are unchanged keep
// their items, so only rows for added, removed, or changed values are
// rebuilt.
func (l *List[T]) Set(values []T) {
	want := slices.SortedStableFunc(slices.Values(values), l.cmp)
	want = slices.CompactFunc(want, func(a, b T) bool { return l.cmp(a, b) == 0 })

	var i uint32
	for _, v := range want {
		matched := false
		for i < l.store.GetNItems() {
			p, old := l.at(i)
			c := l.cmp(old, v)
			if c < 0 {
				l.store.Remove(i)
				continue
			}
			if c == 0 {
				matched = true
				if old != v {
					setItem(p, v)
					l.store.ItemsChanged(i, 1, 1)
				}
			}
			break
		}
		if !matched {
			o := newItem(v)
			l.store.Insert(i, o)
			o.Unref()
		}
		i++
	}
	if n := l.store.GetNItems(); i < n {
		l.store.Splice(i, n-i, nil, 0)
	}
}

// BindListBox binds lb to l, creating each row with row. row returns a
// new widget that the list box takes over.
func BindListBox[T comparable](lb *gtk.ListBox, l *List[T], row func(T) *gtk.Widget) {
	create := func(item uintptr) uintptr {
		w := row(itemAt[T](item))
		// puregotk's constructors sink the floating reference. Hand
		// ours back as floating, which the list box then sinks.
		w.ForceFloating()
		return w.GoPointer()
	}
	lb.BindModel(l.store, &listBoxCreate, gutil.Hold(create), &gutil.FreeData)
}

var listBoxCreate gtk.ListBoxCreateWidgetFunc = func(item, id uintptr) uintptr {
	return gutil.Held[func(uintptr) uintptr](id)(item)
}
