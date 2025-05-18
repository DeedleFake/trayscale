package listmodels

import (
	"iter"
	"slices"

	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func convertObject[T any](obj *glib.Object) T {
	if v, ok := obj.Cast().(T); ok {
		return v
	}
	return gioutil.ObjectValue[T](obj)
}

func Objects(list gio.ListModeller) iter.Seq2[uint, *glib.Object] {
	return func(yield func(uint, *glib.Object) bool) {
		length := list.NItems()
		for i := uint(0); i < length; i++ {
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
			if !yield(i, convertObject[T](obj)) {
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
			if !yield(i, convertObject[T](obj)) {
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

func UpdateStrings(m *gtk.StringList, s iter.Seq[string]) {
	m.FreezeNotify()
	defer m.ThawNotify()

	for i, v := range StringsBackward(m) {
		if !xiter.Contains(s, v) {
			m.Remove(i)
		}
	}

	for v := range s {
		if !xiter.Contains(xiter.V2(StringsBackward(m)), v) {
			m.Append(v)
		}
	}
}

func Update[T comparable](m *gioutil.ListModel[T], s iter.Seq[T]) {
	m.FreezeNotify()
	defer m.ThawNotify()

	for i, v := range ValuesBackward[T](m) {
		if !xiter.Contains(s, v) {
			m.Remove(int(i))
		}
	}

	for v := range s {
		if !xiter.Contains(m.All(), v) {
			m.Append(v)
		}
	}
}

func Index[T any](m gio.ListModeller, f func(T) bool) (uint, bool) {
	length := m.NItems()
	for i := uint(0); i < length; i++ {
		if f(convertObject[T](m.Item(i))) {
			return i, true
		}
	}
	return 0, false
}

func BindListBox[T any](lb *gtk.ListBox, m gio.ListModeller, f func(T) gtk.Widgetter) {
	lb.BindModel(m, func(obj *glib.Object) gtk.Widgetter {
		return f(convertObject[T](obj))
	})
}

func Bind[T any](
	add func(int, gtk.Widgetter),
	remove func(int, gtk.Widgetter),
	m gio.ListModeller,
	f func(T) gtk.Widgetter,
) func() {
	widgets := make([]gtk.Widgetter, 0, m.NItems())
	h := m.ConnectItemsChanged(func(index, removed, added uint) {
		for i, w := range widgets[index : index+removed] {
			remove(int(index)+i, w)
		}

		new := make([]gtk.Widgetter, 0, added)
		for i := index; i < added; i++ {
			item := m.Item(i)
			new = append(new, f(convertObject[T](item)))
		}
		widgets = slices.Replace(widgets, int(index), int(removed), new...)

		for i, w := range new {
			add(int(index)+i, w)
		}
	})

	return func() {
		m.HandlerDisconnect(h)
	}
}
