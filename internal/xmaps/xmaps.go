package xmaps

type Entry[K comparable, V any] struct {
	Key K
	Val V
}

func Entries[M ~map[K]V, K comparable, V any](m M) []Entry[K, V] {
	r := make([]Entry[K, V], 0, len(m))
	for k, v := range m {
		r = append(r, Entry[K, V]{Key: k, Val: v})
	}
	return r
}
