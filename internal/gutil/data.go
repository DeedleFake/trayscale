package gutil

import (
	"time"

	"sync"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// Callbacks that GLib calls with a user-data pointer share one callback
// slot each and find their Go function by an ID passed as that pointer.

var data = struct {
	sync.Mutex
	m    map[uintptr]any
	next uintptr
}{m: make(map[uintptr]any)}

// Hold stores v and returns an ID for it to pass to GLib as user data.
// Release it with Release or FreeData.
func Hold(v any) uintptr {
	data.Lock()
	defer data.Unlock()
	data.next++
	data.m[data.next] = v
	return data.next
}

// Held returns the value stored with id.
func Held[T any](id uintptr) T {
	data.Lock()
	defer data.Unlock()
	return data.m[id].(T)
}

// Release removes and returns the value stored with id.
func Release[T any](id uintptr) T {
	data.Lock()
	defer data.Unlock()
	v := data.m[id].(T)
	delete(data.m, id)
	return v
}

// FreeData is a GDestroyNotify that releases the ID it's given.
var FreeData glib.DestroyNotify = func(id uintptr) {
	data.Lock()
	defer data.Unlock()
	delete(data.m, id)
}

var idle glib.SourceFunc = func(id uintptr) bool {
	Release[func()](id)()
	return false
}

// IdleAdd runs f once on the main loop. It is safe to call from any
// goroutine.
func IdleAdd(f func()) {
	glib.IdleAdd(&idle, Hold(f))
}

// TimeoutAdd runs f once on the main loop after the given delay. It is
// safe to call from any goroutine.
func TimeoutAdd(delay time.Duration, f func()) {
	glib.TimeoutAdd(uint32(delay.Milliseconds()), &idle, Hold(f))
}

// AsyncCallback is the GAsyncReadyCallback for functions passed to
// Async. Pass it along with Async's ID as the user data.
var AsyncCallback gio.AsyncReadyCallback = func(_, res, id uintptr) {
	Release[func(*gio.AsyncResultBase)](id)(&gio.AsyncResultBase{Ptr: res})
}

// Async returns the user data for AsyncCallback to run f with the
// result. Use a new one for each operation.
func Async(f func(res *gio.AsyncResultBase)) uintptr {
	return Hold(f)
}

var compare glib.CompareDataFunc = func(a, b, id uintptr) int32 {
	return int32(Held[func(a, b uintptr) int](id)(a, b))
}

// NewCustomSorter returns a new sorter, owned by the caller, that
// compares items with f. f is released when the sorter is destroyed.
func NewCustomSorter(f func(a, b uintptr) int) *gtk.CustomSorter {
	return gtk.NewCustomSorter(&compare, Hold(f), &FreeData)
}
