package set

type Set[T comparable] map[T]struct{}

func (s Set[T]) Add(v T) T {
	s[v] = struct{}{}
	return v
}

func (s Set[T]) Has(v T) bool {
	_, ok := s[v]
	return ok
}

func (s Set[T]) Delete(v T) T {
	delete(s, v)
	return v
}
