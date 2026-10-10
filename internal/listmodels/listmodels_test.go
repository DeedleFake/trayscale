package listmodels_test

import (
	"iter"
	"net/netip"
	"slices"
	"strconv"
	"testing"

	"deedles.dev/trayscale/internal/listmodels"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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
			m := gioutil.NewListModel[int]()
			for _, v := range tt.in {
				m.Append(v)
			}

			listmodels.Update(m, once(t, tt.s...))

			if got := slices.Collect(m.All()); !slices.Equal(got, tt.want) {
				t.Errorf("Update(%v, %v) = %v, want %v", tt.in, tt.s, got, tt.want)
			}
		})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := gtk.NewStringList(tt.in)

			listmodels.UpdateStrings(m, once(t, tt.s...))

			got := make([]string, 0, m.NItems())
			for i := range m.NItems() {
				got = append(got, m.String(i))
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
			m := gioutil.NewListModel[netip.Prefix]()
			prefixes := makePrefixes(n)
			listmodels.Update(m, slices.Values(prefixes))

			b.ResetTimer()
			for range b.N {
				listmodels.Update(m, slices.Values(prefixes))
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
