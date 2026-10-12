package listmodels

import (
	"iter"
	"slices"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gobject/types"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

// Objects yields the items of list. Each item is only valid during
// its iteration: the reference GetObject returns is dropped as soon as
// the loop body returns. Ref it to keep it.
func Objects(list gio.ListModel) iter.Seq2[uint32, *gobject.Object] {
	return func(yield func(uint32, *gobject.Object) bool) {
		for i := range list.GetNItems() {
			obj := list.GetObject(i)
			if obj == nil {
				continue
			}
			ok := yield(i, obj)
			obj.Unref()
			if !ok {
				return
			}
		}
	}
}

// Values yields the items of list as *T, panicking on any item that
// isn't a gtype. The same lifetime rule as [Objects] applies.
func Values[T any, P interface {
	*T
	gobject.Ptr
}](list gio.ListModel, gtype types.GType) iter.Seq2[uint32, *T] {
	return func(yield func(uint32, *T) bool) {
		for i, obj := range Objects(list) {
			if !yield(i, gutil.MustCast[T, P](obj, gtype)) {
				return
			}
		}
	}
}

func StringsBackward(m *gtk.StringList) iter.Seq2[uint32, string] {
	return func(yield func(uint32, string) bool) {
		for i := m.GetNItems(); i > 0; i-- {
			if !yield(i-1, m.GetString(i-1)) {
				return
			}
		}
	}
}

// StringsForward yields the strings of a GtkStringList in order.
func StringsForward(m *gtk.StringList) iter.Seq2[uint32, string] {
	return func(yield func(uint32, string) bool) {
		for i := range m.GetNItems() {
			if !yield(i, m.GetString(i)) {
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

// Index returns the position of the first item of m for which f
// returns true. The *T passed to f is only valid during the call.
func Index[T any, P interface {
	*T
	gobject.Ptr
}](m gio.ListModel, gtype types.GType, f func(*T) bool) (uint32, bool) {
	for i, v := range Values[T, P](m, gtype) {
		if f(v) {
			return i, true
		}
	}
	return 0, false
}
