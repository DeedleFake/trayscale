package main

import (
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/slices"
)

type rowManager[Row rowwer, Data any] struct {
	Parent rowManagerParent
	Create func() Row
	Set    func(Row, Data)

	rows []Row
}

func (m *rowManager[Row, Data]) resize(size int) {
	if size == len(m.rows) {
		return
	}

	if size < len(m.rows) {
		for _, r := range m.rows[size:] {
			m.Parent.Remove(r.Row())
		}
		xslices.Clear(m.rows[size:])
		m.rows = m.rows[:size]
		return
	}

	m.rows = slices.Grow(m.rows, size-cap(m.rows))
	for len(m.rows) < size {
		row := m.Create()
		m.Parent.Add(row.Row())
		m.rows = append(m.rows, row)
	}
}

func (m *rowManager[Row, Data]) Update(data []Data) {
	m.resize(len(data))

	for i, d := range data {
		m.Set(m.rows[i], d)
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

type rowwer interface {
	Row() gtk.Widgetter
}

type simpleActionRow[T gtk.Widgetter] struct {
	action T
	row    *adw.ActionRow
}

func (row simpleActionRow[T]) Row() gtk.Widgetter {
	return row.row
}

type (
	buttonRow = simpleActionRow[*gtk.Button]
	labelRow  = simpleActionRow[*gtk.Label]
)
