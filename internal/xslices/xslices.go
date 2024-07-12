package xslices

import "slices"

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

// ChunkBy returns a slice of subsequent subsclies of s such that each
// subslice is a continuous run of elements for which val(element)
// returns the same value. In other words, a new chunk starts each
// time that the return value of val(element) changes.
//
// If len(s) == 0, the length of the returned slice will also be 0.
func ChunkBy[E any, S ~[]E, R comparable](s S, val func(E) R) []S {
	if len(s) == 0 {
		return nil
	}

	r := make([]S, 0, len(s)/2)

	prev := val(s[0])
	var start int
	for i := 1; i < len(s); i++ {
		v := s[i]
		cur := val(v)
		if cur == prev {
			continue
		}

		r = append(r, s[start:i])
		prev, start = cur, i
	}

	last := s[start:]
	if len(last) != 0 {
		r = append(r, last)
	}

	return slices.Clip(r)
}
