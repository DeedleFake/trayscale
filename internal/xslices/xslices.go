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

// ToMap inserts the elements of s into m, calling f() with each
// element and its index to generate a key for the map entry.
func ToMap[K comparable, V any, S ~[]V, M ~map[K]V](m M, s S, f func(int, V) K) {
	for i, v := range s {
		m[f(i, v)] = v
	}
}
