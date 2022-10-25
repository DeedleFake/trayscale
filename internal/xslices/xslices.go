package xslices

import "golang.org/x/exp/slices"

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

// Subtract returns x minus any elements that are in y.
func Subtract[E comparable, X ~[]E, Y ~[]E](x X, y Y) X {
	for i := 0; i < len(x); i++ {
		if slices.Contains(y, x[i]) {
			x = slices.Delete(x, i, 1)
			i--
		}
	}
	return x
}
