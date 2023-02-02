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

// Filter removes any elements from s for which keep(element) is
// false.
func Filter[E any, S ~[]E](s S, keep func(E) bool) S {
	to := 0
	for _, v := range s {
		if keep(v) {
			s[to] = v
			to++
		}
	}
	return s[:to]
}

// Clear sets every element of s to the zero value of E.
//
// TODO: Replace with clear() builtin when Go 1.21 comes out?
func Clear[E any](s []E) {
	var z E
	for i := range s {
		s[i] = z
	}
}
