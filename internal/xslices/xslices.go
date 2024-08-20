package xslices

import "slices"

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
	return AppendChunkBy(r, s, val)
}

// AppendChunkBy is the same as [ChunkBy] but it appends the results
// to to instead of allocating its own slice.
func AppendChunkBy[E any, S ~[]E, R comparable](to []S, s S, val func(E) R) []S {
	if len(s) == 0 {
		return to
	}

	prev := val(s[0])
	var start int
	for i := 1; i < len(s); i++ {
		v := s[i]
		cur := val(v)
		if cur == prev {
			continue
		}

		to = append(to, slices.Clip(s[start:i]))
		prev, start = cur, i
	}

	last := s[start:]
	if len(last) != 0 {
		to = append(to, slices.Clip(last))
	}

	return to
}
