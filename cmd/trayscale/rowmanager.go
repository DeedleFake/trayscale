package main

import (
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/slices"
)

type ListModel[T any] struct {
	*gio.ListStore
	from func(T) *glib.Object
	to   func(*glib.Object) T
}

func NewListModel[T any](from func(T) *glib.Object, to func(*glib.Object) T) *ListModel[T] {
	return &ListModel[T]{
		ListStore: gio.NewListStore(from(*new(T)).Type()),
		from:      from,
		to:        to,
	}
}

func (m *ListModel[T]) Update(data []T) {
	objects := make([]*glib.Object, 0, len(data))
	for _, d := range data {
		objects = append(objects, m.from(d))
	}
	m.Splice(0, m.NItems(), objects)
}

type rowManager[Data any] struct {
	Parent rowManagerParent
	New    func(Data) row[Data]

	rows []row[Data]
}

func (m *rowManager[Data]) resize(size int) {
	if size >= cap(m.rows) {
		m.rows = slices.Grow(m.rows, size-cap(m.rows))
		return
	}

	if size < len(m.rows) {
		for _, r := range m.rows[size:] {
			m.Parent.Remove(r.Widget())
		}
		xslices.Clear(m.rows[size:])
		m.rows = m.rows[:size]
	}
}

func (m *rowManager[Data]) Update(data []Data) {
	m.resize(len(data))

	for i, d := range data {
		if i < len(m.rows) {
			m.rows[i].Update(d)
			continue
		}

		row := m.New(d)
		m.Parent.Add(row.Widget())
		m.rows = append(m.rows, row)
	}
}

type rowManagerParent interface {
	Add(gtk.Widgetter)
	Remove(gtk.Widgetter)
}

type rowAdder interface {
	AddRow(gtk.Widgetter)
	Remove(gtk.Widgetter)
}

type rowAdderParent struct {
	rowAdder
}

func (r rowAdderParent) Add(w gtk.Widgetter) {
	r.AddRow(w)
}

type row[Data any] interface {
	Update(Data)
	Widget() gtk.Widgetter
}

type simpleRow[Data any] struct {
	W gtk.Widgetter
	U func(Data)
}

func (row *simpleRow[Data]) Update(data Data) {
	row.U(data)
}

func (row *simpleRow[Data]) Widget() gtk.Widgetter {
	return row.W
}
