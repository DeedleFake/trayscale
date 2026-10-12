package ui

import (
	"iter"
	"slices"

	"deedles.dev/xiter"
	"codeberg.org/puregotk/puregotk/v4/gtk"
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
			r.Widget().Unref()
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
	Add(*gtk.Widget)
	Remove(*gtk.Widget)
}

type rowAdder interface {
	AddRow(*gtk.Widget)
	Remove(*gtk.Widget)
}

type rowAdderParent struct {
	rowAdder
}

func (r rowAdderParent) Add(w *gtk.Widget) {
	r.AddRow(w)
}

// row is one managed row. The row owns a reference to its widget,
// which the manager drops when the row is removed.
type row[Data any] interface {
	Update(Data)
	Widget() *gtk.Widget
}

type simpleRow[Data any] struct {
	W *gtk.Widget
	U func(Data)
}

func (row *simpleRow[Data]) Update(data Data) {
	row.U(data)
}

func (row *simpleRow[Data]) Widget() *gtk.Widget {
	return row.W
}
