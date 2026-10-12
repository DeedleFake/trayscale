package listmodels

import (
	"iter"
	"slices"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Model is a list model of Go values. GTK only sees a GtkStringList of
// keys; the values stay in a Go map. No Go value is ever stored inside a
// GObject, so the model needs no GObject subclass and no per-item
// pinning, and the same shape works with bindings that require manual
// memory management.
//
// Keys must be unique per value. Values with equal keys are treated as
// the same entry.
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

// ListModel returns the underlying GTK list model.
func (m *Model[T]) ListModel() gio.ListModeller { return m.list }

// Len returns the number of entries.
func (m *Model[T]) Len() int { return int(m.list.NItems()) }

// Value returns the value for an item of m's list model, or of a model
// wrapping it such as a GtkSortListModel.
func (m *Model[T]) Value(obj *glib.Object) T {
	return m.values[obj.Cast().(*gtk.StringObject).String()]
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

// Comparer returns a comparison function for a GtkCustomSorter over m's
// items.
func (m *Model[T]) Comparer(f func(T, T) int) glib.CompareDataFunc {
	return glib.NewObjectComparer(func(o1, o2 *gtk.StringObject) int {
		return f(m.values[o1.String()], m.values[o2.String()])
	})
}

// BindListBox binds lb to list, which must be m's list model or a model
// wrapping it, creating each row with f.
func (m *Model[T]) BindListBox(lb *gtk.ListBox, list gio.ListModeller, f func(T) gtk.Widgetter) {
	lb.BindModel(list, func(obj *glib.Object) gtk.Widgetter {
		return f(m.Value(obj))
	})
}

// StringsForward yields the strings of a GtkStringList in order.
func StringsForward(m *gtk.StringList) iter.Seq2[uint, string] {
	return func(yield func(uint, string) bool) {
		for i := range m.NItems() {
			if !yield(i, m.String(i)) {
				return
			}
		}
	}
}
