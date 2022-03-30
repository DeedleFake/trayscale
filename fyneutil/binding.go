package fyneutil

import "fyne.io/fyne/v2/data/binding"

type Binding[T any] interface {
	binding.DataItem
	Get() (T, error)
	Set(T) error
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
