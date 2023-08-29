package xcmp

import "deedles.dev/xiter"

// Or is a basic implementation of the proposed [cmp.Or] function.
//
// TODO: Remove when cmp.Or is added, maybe in Go 1.22.
func Or[T comparable](vals ...T) (r T) {
	r, _ = xiter.Find(xiter.OfSlice(vals), func(v T) bool { return v != r })
	return r
}
