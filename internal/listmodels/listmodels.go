package listmodels

import (
	"iter"
	"slices"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func Convert[T any](obj *glib.Object) T {
	return obj.Cast().(T)
}

func Objects(list gio.ListModeller) iter.Seq2[uint, *glib.Object] {
	return func(yield func(uint, *glib.Object) bool) {
		length := list.NItems()
		for i := range length {
			item := list.Item(i)
			if !yield(i, item) {
				return
			}
		}
	}
}

func Values[T any](list gio.ListModeller) iter.Seq2[uint, T] {
	return func(yield func(uint, T) bool) {
		for i, obj := range Objects(list) {
			if !yield(i, Convert[T](obj)) {
				return
			}
		}
	}
}

func Backward(list gio.ListModeller) iter.Seq2[uint, *glib.Object] {
	return func(yield func(uint, *glib.Object) bool) {
		for i := int(list.NItems()) - 1; i >= 0; i-- {
			if !yield(uint(i), list.Item(uint(i))) {
				return
			}
		}
	}
}

func ValuesBackward[T any](list gio.ListModeller) iter.Seq2[uint, T] {
	return func(yield func(uint, T) bool) {
		for i, obj := range Backward(list) {
			if !yield(i, Convert[T](obj)) {
				return
			}
		}
	}
}

func StringsBackward(m *gtk.StringList) iter.Seq2[uint, string] {
	return func(yield func(uint, string) bool) {
		for i := m.NItems(); i > 0; i-- {
			if !yield(i-1, m.String(i-1)) {
				return
			}
		}
	}
}

const (
	unwanted uint8 = iota
	wanted
	inModel
)

// UpdateStrings removes from m the strings s does not yield, then appends the ones
// m lacks. Existing entries keep their order and new ones follow s's order. s is
// read once, so a single-pass sequence is fine.
func UpdateStrings(m *gtk.StringList, s iter.Seq[string]) {
	m.FreezeNotify()
	defer m.ThawNotify()

	want := slices.Collect(s)

	state := make(map[string]uint8, len(want))
	for _, v := range want {
		state[v] = wanted
	}

	// Backwards, so a removal never shifts an entry still to be visited.
	for i, v := range StringsBackward(m) {
		if state[v] == unwanted {
			m.Remove(i)
			continue
		}
		state[v] = inModel
	}

	for _, v := range want {
		if state[v] == inModel {
			continue
		}
		state[v] = inModel
		m.Append(v)
	}
}

func Index[T any](m gio.ListModeller, f func(T) bool) (uint, bool) {
	length := m.NItems()
	for i := range length {
		if f(Convert[T](m.Item(i))) {
			return i, true
		}
	}
	return 0, false
}

func BindListBox[T any](lb *gtk.ListBox, m gio.ListModeller, f func(T) gtk.Widgetter) {
	lb.BindModel(m, func(obj *glib.Object) gtk.Widgetter {
		return f(Convert[T](obj))
	})
}
