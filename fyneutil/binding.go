package fyneutil

import "fyne.io/fyne/v2/data/binding"

type Binding[T any] interface {
	binding.DataItem

	Get() (T, error)
	Set(T) error
}

type ListBinding[T any, S ~[]T] interface {
	binding.DataList
	Binding[S]

	Append(T) error
	Prepend(T) error
	GetValue(int) (T, error)
	SetValue(int, T) error
}

func NewListBinding[T any, S ~[]T]() ListBinding[T, S] {
	return &listBinding[T, S]{UntypedList: binding.NewUntypedList()}
}

type listBinding[T any, S ~[]T] struct {
	// TODO: Implement this properly.
	binding.UntypedList
}

func (b *listBinding[T, S]) Get() (S, error) {
	v, err := b.UntypedList.Get()
	s := make(S, 0, len(v))
	for _, v := range v {
		s = append(s, v.(T))
	}
	return s, err
}

func (b *listBinding[T, S]) Set(v S) error {
	s := make([]interface{}, 0, len(v))
	for _, v := range v {
		s = append(s, v)
	}
	return b.UntypedList.Set(s)
}

func (b *listBinding[T, S]) Append(v T) error {
	return b.UntypedList.Append(v)
}

func (b *listBinding[T, S]) Prepend(v T) error {
	return b.UntypedList.Prepend(v)
}

func (b *listBinding[T, S]) GetValue(index int) (T, error) {
	v, err := b.UntypedList.GetValue(index)
	t, _ := v.(T)
	return t, err
}

func (b *listBinding[T, S]) SetValue(index int, v T) error {
	return b.UntypedList.SetValue(index, v)
}

func Transform[
	To, From any,
	ToBinding Binding[To], FromBinding Binding[From],
](
	to ToBinding,
	from FromBinding,
	transform func(From) To,
) (cancel func()) {
	listener := binding.NewDataListener(func() {
		v, err := from.Get()
		if err != nil {
			return
		}

		to.Set(transform(v))
	})
	from.AddListener(listener)

	return func() {
		from.RemoveListener(listener)
	}
}
