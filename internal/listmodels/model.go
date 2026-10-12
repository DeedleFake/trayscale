package listmodels

import (
	"iter"
	"slices"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

// Model is a list model of Go values. GTK only sees a GtkStringList of
// keys; the values stay in a Go map. No Go value is ever stored inside a
// GObject, so the model needs no GObject subclass and no per-item
// pinning.
//
// Keys must be unique per value. Values with equal keys are treated as
// the same entry.
//
// The Model owns its GtkStringList; call Unref when done with it.
type Model[T any] struct {
	list   *gtk.StringList
	key    func(T) string
	values map[string]T
}

// NewModel returns an empty Model that identifies values by key.
func NewModel[T any](key func(T) string) *Model[T] {
	return &Model[T]{
		list:   gtk.NewStringList(nil),
		key:    key,
		values: make(map[string]T),
	}
}

// Unref drops the Model's reference to its list.
func (m *Model[T]) Unref() { m.list.Unref() }

// ListModel returns the underlying GTK list model, borrowed from m.
func (m *Model[T]) ListModel() gio.ListModel { return m.list }

// Len returns the number of entries.
func (m *Model[T]) Len() int { return int(m.list.GetNItems()) }

// Value returns the value for an item of m's list model, or of a model
// wrapping it such as a GtkSortListModel.
func (m *Model[T]) Value(obj gobject.Ptr) T {
	return m.values[gutil.MustCast[gtk.StringObject](obj, gtk.StringObjectGLibType()).GetString()]
}

// All yields the values in model order.
func (m *Model[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, k := range StringsForward(m.list) {
			if !yield(m.values[k]) {
				return
			}
		}
	}
}

// Append adds v at the end, unless an entry with its key exists.
func (m *Model[T]) Append(v T) {
	k := m.key(v)
	if _, ok := m.values[k]; ok {
		return
	}
	m.values[k] = v
	m.list.Append(k)
}

// Update removes the entries s does not yield, then appends the ones m
// lacks. Existing entries keep their order and new ones follow s's
// order. s is read once, so a single-pass sequence is fine.
func (m *Model[T]) Update(s iter.Seq[T]) {
	m.list.FreezeNotify()
	defer m.list.ThawNotify()

	want := slices.Collect(s)
	byKey := make(map[string]T, len(want))
	for _, v := range want {
		byKey[m.key(v)] = v
	}

	// Backwards, so a removal never shifts an entry still to be visited.
	for i, k := range StringsBackward(m.list) {
		v, ok := byKey[k]
		if !ok {
			m.list.Remove(i)
			delete(m.values, k)
			continue
		}
		m.values[k] = v // Same key, possibly newer data.
	}

	for _, v := range want {
		m.Append(v)
	}
}

// NewSorter returns a GtkCustomSorter over m's items, owned by the
// caller. Its callbacks are freed when the sorter is destroyed.
func (m *Model[T]) NewSorter(f func(T, T) int) *gtk.CustomSorter {
	var compare glib.CompareDataFunc = func(a, b, _ uintptr) int32 {
		return int32(f(m.valueAt(a), m.valueAt(b)))
	}
	var destroy glib.DestroyNotify
	destroy = func(uintptr) {
		glib.UnrefCallback(&compare)
		glib.UnrefCallback(&destroy)
	}
	return gtk.NewCustomSorter(&compare, 0, &destroy)
}

func (m *Model[T]) valueAt(p uintptr) T {
	return m.values[gtk.StringObjectNewFromInternalPtr(p).GetString()]
}

// NewSortListModel returns a sorted view of m, owned by the caller,
// using f to compare values.
func (m *Model[T]) NewSortListModel(f func(T, T) int) *gtk.SortListModel {
	// gtk_sort_list_model_new takes ownership of both the model and
	// the sorter, so pass it a new sorter and an extra model ref.
	sorter := m.NewSorter(f)
	m.list.Ref()
	return gtk.NewSortListModel(m.list, &sorter.Sorter)
}

// BindListBox binds lb to list, which must be m's list model or a model
// wrapping it, creating each row with f. f returns a new widget that
// the list box takes over. The callbacks are freed when lb is unbound
// or destroyed.
func (m *Model[T]) BindListBox(lb *gtk.ListBox, list gio.ListModel, f func(T) *gtk.Widget) {
	var create gtk.ListBoxCreateWidgetFunc = func(item, _ uintptr) uintptr {
		w := f(m.valueAt(item))
		// puregotk's constructors sink the floating reference. Hand
		// ours back as floating, which the list box then sinks.
		w.ForceFloating()
		return w.GoPointer()
	}
	var destroy glib.DestroyNotify
	destroy = func(uintptr) {
		glib.UnrefCallback(&create)
		glib.UnrefCallback(&destroy)
	}
	lb.BindModel(list, &create, 0, &destroy)
}
