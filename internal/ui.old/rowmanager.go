package ui

import (
	"iter"
	"slices"

	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// TODO: Move this into a different package and remove the need to keep
// track of the parent and other pieces of tight coupling to GTK.
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
		clear(m.rows[size:])
		m.rows = m.rows[:size]
	}
}

func (m *rowManager[Data]) Update(data []Data) {
	m.UpdateFromSeq(slices.Values(data), len(data))
}

func (m *rowManager[Data]) UpdateFromSeq(data iter.Seq[Data], size int) {
	m.resize(size)

	edata := xiter.Enumerate(data)
	for i, d := range edata {
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
