package xslices

// Partition returns two slices, t and f. t contains all elements of s
// for which pred() returns true, and f contains all elements for
// which pred() returns false.
func Partition[E any, S ~[]E](s S, pred func(E) bool) (t, f S) {
	t = make(S, 0, len(s))
	f = make(S, 0, len(s))

	for _, v := range s {
		if pred(v) {
			t = append(t, v)
			continue
		}
		f = append(f, v)
	}

	return t, f
}

// Filter creates a new slice containing only the values in s for
// which keep(value) returns true.
func Filter[E any, S ~[]E](s S, keep func(E) bool) S {
	r := make(S, 0, len(s))
	for _, v := range s {
		if keep(v) {
			r = append(r, v)
		}
	}
	return r
}
