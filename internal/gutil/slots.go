package gutil

import "codeberg.org/puregotk/purego"

// FreeCallbackSlots reports how many purego callback slots remain, by
// allocating until the table is full and then releasing them all. purego
// has a fixed number of slots, so leak checks compare this before and
// after repeated work. It's for tests and debugging only, and must not
// run while other goroutines create callbacks: it briefly takes every
// slot, so their callback creation would panic.
func FreeCallbackSlots() (n int) {
	var held []*func() uintptr
	defer func() {
		recover()
		for _, f := range held {
			purego.UnrefCallbackFnPtr(f)
		}
	}()
	for {
		f := func() uintptr { return 0 }
		purego.NewCallbackFnPtr(&f)
		held = append(held, &f)
		n++
	}
}
