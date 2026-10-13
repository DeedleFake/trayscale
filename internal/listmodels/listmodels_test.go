package listmodels_test

import (
	"cmp"
	"iter"
	"net/netip"
	"slices"
	"strconv"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
)

// once returns a sequence over vs that fails the test if it is read twice.
func once[T any](t *testing.T, vs ...T) iter.Seq[T] {
	t.Helper()
	used := false
	return func(yield func(T) bool) {
		if used {
			t.Error("sequence was consumed more than once")
			return
		}
		used = true
		for _, v := range vs {
			if !yield(v) {
				return
			}
		}
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		s    []int
		want []int
	}{
		{"empty model", nil, []int{1, 2, 3}, []int{1, 2, 3}},
		{"no change", []int{1, 2, 3}, []int{1, 2, 3}, []int{1, 2, 3}},
		{"append", []int{1, 2}, []int{1, 2, 3}, []int{1, 2, 3}},
		{"remove", []int{1, 2, 3}, []int{1, 3}, []int{1, 3}},
		{"remove all", []int{1, 2, 3}, nil, nil},
		{"add and remove at once", []int{1, 2, 3}, []int{3, 4, 1}, []int{1, 3, 4}},
		{"existing order wins", []int{3, 1, 2}, []int{1, 2, 3}, []int{3, 1, 2}},
		{"new values keep s order", []int{3}, []int{1, 2, 3}, []int{3, 1, 2}},
		{"duplicate in s appended once", nil, []int{1, 1, 2, 1}, []int{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := listmodels.NewModel(strconv.Itoa)
			defer m.Unref()
			for _, v := range tt.in {
				m.Append(v)
			}

			m.Update(once(t, tt.s...))

			if got := slices.Collect(m.All()); !slices.Equal(got, tt.want) {
				t.Errorf("Update(%v, %v) = %v, want %v", tt.in, tt.s, got, tt.want)
			}
		})
	}
}

// TestUpdateRefreshesValues checks that an entry whose key is unchanged
// picks up the newer value, and that All and Value agree with the list.
func TestUpdateRefreshesValues(t *testing.T) {
	type file struct {
		name string
		size int
	}
	m := listmodels.NewModel(func(f file) string { return f.name })
	defer m.Unref()
	m.Update(slices.Values([]file{{"a", 1}, {"b", 2}}))
	m.Update(slices.Values([]file{{"b", 3}, {"c", 4}}))

	want := []file{{"b", 3}, {"c", 4}}
	if got := slices.Collect(m.All()); !slices.Equal(got, want) {
		t.Fatalf("All() = %v, want %v", got, want)
	}
	if m.Len() != len(want) {
		t.Fatalf("Len() = %d, want %d", m.Len(), len(want))
	}
	for i, w := range want {
		obj := m.ListModel().GetObject(uint32(i))
		if got := m.Value(obj); got != w {
			t.Errorf("Value(item %d) = %v, want %v", i, got, w)
		}
		obj.Unref()
	}
}

// TestUpdateRefreshesBoundRow checks that a row built from a value is
// rebuilt when a later update keeps the key and changes the value.
func TestUpdateRefreshesBoundRow(t *testing.T) {
	if !gtk.InitCheck() {
		t.Skip("no display")
	}
	type file struct {
		name string
		size string
	}
	m := listmodels.NewModel(func(f file) string { return f.name })
	defer m.Unref()
	lb := gtk.NewListBox()
	defer lb.Unref()
	m.BindListBox(lb, m.ListModel(), func(f file) *gtk.Widget {
		label := gtk.NewLabel(f.size)
		return &label.Widget
	})

	m.Update(slices.Values([]file{{name: "notes.txt", size: "1.00B"}}))
	m.Update(slices.Values([]file{{name: "notes.txt", size: "5.00MB"}}))

	row := lb.GetRowAtIndex(0)
	if row == nil {
		t.Fatal("no row")
	}
	defer row.Unref()
	child := row.GetChild()
	if child == nil {
		t.Fatal("row has no child")
	}
	defer child.Unref()
	label := gutil.MustCast[gtk.Label](child, gtk.LabelGLibType())
	if got := label.GetLabel(); got != "5.00MB" {
		t.Fatalf("row label = %q, want %q", got, "5.00MB")
	}
}

// TestComparer sorts through a GtkSortListModel, the way the pages use it.
func TestComparer(t *testing.T) {
	m := listmodels.NewModel(strconv.Itoa)
	defer m.Unref()
	m.Update(slices.Values([]int{3, 1, 2}))
	sorted := m.NewSortListModel(cmp.Compare[int])
	defer sorted.Unref()

	var got []int
	for _, obj := range listmodels.Objects(sorted) {
		got = append(got, m.Value(obj))
	}
	if want := []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("sorted = %v, want %v", got, want)
	}
}

// TestSorterFreesCallbacks checks that sort models free their callback
// slots when destroyed, so pages can be opened and closed forever.
func TestSorterFreesCallbacks(t *testing.T) {
	m := listmodels.NewModel(strconv.Itoa)
	defer m.Unref()
	m.Update(slices.Values([]int{3, 1, 2}))

	before := gutil.FreeCallbackSlots()
	for range 500 {
		sorted := m.NewSortListModel(cmp.Compare[int])
		sorted.GetNItems()
		sorted.Unref()
	}
	if after := gutil.FreeCallbackSlots(); after != before {
		t.Errorf("free callback slots: %d before, %d after 500 sort models", before, after)
	}
}

func TestUpdateStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		s    []string
		want []string
	}{
		{"empty model", nil, []string{"a", "b"}, []string{"a", "b"}},
		{"no change", []string{"a", "b"}, []string{"a", "b"}, []string{"a", "b"}},
		{"append", []string{"a"}, []string{"a", "b"}, []string{"a", "b"}},
		{"remove", []string{"a", "b"}, []string{"b"}, []string{"b"}},
		{"remove all", []string{"a", "b"}, nil, nil},
		{"add and remove at once", []string{"a", "b", "c"}, []string{"c", "d", "a"}, []string{"a", "c", "d"}},
		{"existing order wins", []string{"b", "a"}, []string{"a", "b"}, []string{"b", "a"}},
		{"new values keep s order", []string{"b"}, []string{"a", "b"}, []string{"b", "a"}},
		{"duplicate in s appended once", nil, []string{"a", "a", "b", "a"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := gtk.NewStringList(tt.in)
			defer m.Unref()

			listmodels.UpdateStrings(m, once(t, tt.s...))

			got := make([]string, 0, m.GetNItems())
			for _, v := range listmodels.StringsForward(m) {
				got = append(got, v)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("UpdateStrings(%v, %v) = %v, want %v", tt.in, tt.s, got, tt.want)
			}
		})
	}
}

// BenchmarkUpdate measures the update the UI performs on every status change,
// with the model already holding every value.
func BenchmarkUpdate(b *testing.B) {
	for _, n := range []int{100, 1000, 5478} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			m := listmodels.NewModel(netip.Prefix.String)
			defer m.Unref()
			prefixes := makePrefixes(n)
			m.Update(slices.Values(prefixes))

			b.ResetTimer()
			for range b.N {
				m.Update(slices.Values(prefixes))
			}
		})
	}
}

// makePrefixes returns n distinct /32 prefixes in 10.0.0.0/8.
func makePrefixes(n int) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, n)
	for i := range n {
		addr := netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})
		prefixes = append(prefixes, netip.PrefixFrom(addr, 32))
	}
	return prefixes
}
