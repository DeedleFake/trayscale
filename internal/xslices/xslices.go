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
