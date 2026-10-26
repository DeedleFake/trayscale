package listmodels_test

import (
	"iter"
	"slices"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gtk"
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
