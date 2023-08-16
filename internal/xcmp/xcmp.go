package xcmp

// Or is a basic implementation of the proposed [cmp.Or] function.
//
// TODO: Remove when cmp.Or is added, maybe in Go 1.22.
func Or[T comparable](vals ...T) (r T) {
	for _, v := range vals {
		if v != r {
			return v
		}
	}
	return r
}
