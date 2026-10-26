package gobj

import (
	"cmp"
	"slices"
	"strconv"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

type entry struct {
	key, val int
}

func byKey(a, b entry) int { return cmp.Compare(a.key, b.key) }

func liveItems() int {
	items.Lock()
	defer items.Unlock()
	return len(items.values)
}

func TestListSet(t *testing.T) {
	requireGTK(t)

	tests := []struct {
		name        string
		in, set     []entry
		want        []entry
		changedRows int
	}{
		{"empty", nil, []entry{{2, 0}, {1, 0}}, []entry{{1, 0}, {2, 0}}, 2},
		{"remove", []entry{{1, 0}, {2, 0}, {3, 0}}, []entry{{2, 0}}, []entry{{2, 0}}, 0},
		{"keep", []entry{{1, 0}, {2, 0}}, []entry{{2, 0}, {1, 0}}, []entry{{1, 0}, {2, 0}}, 0},
		{"change", []entry{{1, 0}, {2, 0}}, []entry{{1, 0}, {2, 9}}, []entry{{1, 0}, {2, 9}}, 1},
		{"mixed", []entry{{1, 0}, {3, 0}, {5, 0}}, []entry{{0, 0}, {3, 1}, {4, 0}, {6, 0}}, []entry{{0, 0}, {3, 1}, {4, 0}, {6, 0}}, 4},
		{"duplicates", nil, []entry{{1, 0}, {1, 0}}, []entry{{1, 0}}, 1},
		{"clear", []entry{{1, 0}, {2, 0}}, nil, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewList(byKey)
			defer l.Unref()
			l.Set(tt.in)

			lb := gtk.NewListBox()
			defer lb.Unref()
			built := 0
			BindListBox(lb, l, func(e entry) *gtk.Widget {
				built++
				return &gtk.NewLabel(strconv.Itoa(e.val)).Widget
			})
			built = 0

			l.Set(tt.set)
			if got := slices.Collect(l.All()); !slices.Equal(got, tt.want) {
				t.Errorf("All() = %v, want %v", got, tt.want)
			}
			if built != tt.changedRows {
				t.Errorf("rebuilt %d rows, want %d", built, tt.changedRows)
			}
			for i, e := range tt.want {
				r := lb.GetRowAtIndex(int32(i))
				l := gtk.LabelNewFromInternalPtr(r.GetChild().GoPointer())
				if l.GetLabel() != strconv.Itoa(e.val) {
					t.Errorf("row %d shows %q, want %d", i, l.GetLabel(), e.val)
				}
			}
		})
	}
}

func TestListFreesItems(t *testing.T) {
	requireGTK(t)

	const n = 2000
	l := NewList(byKey)
	lb := gtk.NewListBox()
	BindListBox(lb, l, func(e entry) *gtk.Widget { return &gtk.NewLabel("").Widget })
	before := gutil.FreeCallbackSlots()
	base := liveItems()

	es := make([]entry, n)
	for i := range es {
		es[i] = entry{i, 0}
	}
	l.Set(es)
	if after := gutil.FreeCallbackSlots(); after != before {
		t.Errorf("free callback slots %d -> %d over %d items", before, after, n)
	}
	l.Set(es[:n/2])
	if got := liveItems() - base; got != n/2 {
		t.Errorf("%d items live after removing half, want %d", got, n/2)
	}
	lb.Unref()
	l.Unref()
	if got := liveItems() - base; got != 0 {
		t.Errorf("%d items live after dropping the list", got)
	}
}
