package gutil

import (
	"reflect"
	"sync"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
)

// purego has a fixed number of callback slots, and reuses the slot of
// a function pointer it has already seen. Instead of a new callback for
// every connection, every handler of a given signal goes through one
// shared callback per signal, which finds the Go handlers by the
// emitting object. Handlers are dropped when the object is finalized,
// through one shared weak-reference notify.

type objHandlers struct {
	handlers   map[any][]reflect.Value
	finalizers []func()
}

var objs = struct {
	sync.Mutex
	m map[uintptr]*objHandlers
}{m: make(map[uintptr]*objHandlers)}

// finalizeNotify is the weak-reference notify for every object in objs.
var finalizeNotify gobject.WeakNotify = func(_, obj uintptr) {
	objs.Lock()
	h := objs.m[obj]
	delete(objs.m, obj)
	objs.Unlock()
	if h != nil {
		for _, f := range h.finalizers {
			f()
		}
	}
}

// handlersFor returns obj's entry in objs, adding it and its weak
// reference if it's new. objs must be locked.
func handlersFor(obj gobject.Ptr) *objHandlers {
	p := obj.GoPointer()
	h, ok := objs.m[p]
	if !ok {
		h = &objHandlers{handlers: make(map[any][]reflect.Value)}
		objs.m[p] = h
		Obj(obj).WeakRef(&finalizeNotify, 0)
	}
	return h
}

// OnFinalize runs f once obj is finalized. It uses a weak reference,
// so it doesn't keep obj alive.
func OnFinalize(obj gobject.Ptr, f func()) {
	objs.Lock()
	defer objs.Unlock()
	h := handlersFor(obj)
	h.finalizers = append(h.finalizers, f)
}

// addHandler adds fn to obj's handlers for key and reports whether it's
// the first, in which case the caller connects the shared callback.
func addHandler(obj gobject.Ptr, key any, fn reflect.Value) (first bool) {
	objs.Lock()
	defer objs.Unlock()
	h := handlersFor(obj)
	first = len(h.handlers[key]) == 0
	h.handlers[key] = append(h.handlers[key], fn)
	return first
}

// handlers returns obj's handlers for key.
func handlers(obj uintptr, key any) []reflect.Value {
	objs.Lock()
	defer objs.Unlock()
	if h := objs.m[obj]; h != nil {
		return h.handlers[key]
	}
	return nil
}

var shared = struct {
	sync.Mutex
	m map[any]any
}{m: make(map[any]any)}

// sharedFor returns the shared callback for key, making it with mk the
// first time.
func sharedFor[T any](key any, mk func() *T) *T {
	shared.Lock()
	defer shared.Unlock()
	if f, ok := shared.m[key]; ok {
		return f.(*T)
	}
	f := mk()
	shared.m[key] = f
	return f
}

// Connect connects fn to a signal of obj with connect, which is one of
// puregotk's Connect methods on obj, such as btn.ConnectClicked. The
// first argument of fn must be the emitting object. fn is dropped when
// obj is finalized.
func Connect[F any](obj gobject.Ptr, connect func(*F) uint32, fn F) {
	// A method value's code pointer is the same for every receiver, so
	// it identifies the signal.
	key := reflect.ValueOf(connect).Pointer()
	if !addHandler(obj, key, reflect.ValueOf(fn)) {
		return
	}
	connect(sharedFor(key, func() *F {
		t := reflect.TypeFor[F]()
		f := reflect.MakeFunc(t, func(args []reflect.Value) []reflect.Value {
			return dispatch(t, handlers(instancePtr(args[0]), key), args)
		}).Interface().(F)
		return &f
	}))
}

// instancePtr returns the C pointer of a puregotk object wrapper.
func instancePtr(v reflect.Value) uintptr {
	return uintptr(v.FieldByName("Ptr").Uint())
}

// dispatch calls hs with args. Like GLib's handling of boolean
// signals, it stops at the first handler that returns true.
func dispatch(t reflect.Type, hs []reflect.Value, args []reflect.Value) []reflect.Value {
	var out []reflect.Value
	for _, h := range hs {
		out = h.Call(args)
		if len(out) == 1 && out[0].Kind() == reflect.Bool && out[0].Bool() {
			return out
		}
	}
	if out == nil {
		out = make([]reflect.Value, t.NumOut())
		for i := range out {
			out[i] = reflect.Zero(t.Out(i))
		}
	}
	return out
}

// ConnectNotify runs f whenever prop changes on obj ("notify::prop").
// f is dropped when obj is finalized.
func ConnectNotify(obj gobject.Ptr, prop string, f func()) {
	signal := "notify::" + prop
	if !addHandler(obj, signal, reflect.ValueOf(f)) {
		return
	}
	cb := sharedFor(signal, func() *func(uintptr, uintptr, uintptr) {
		f := func(obj, _, _ uintptr) {
			for _, h := range handlers(obj, signal) {
				h.Interface().(func())()
			}
		}
		return &f
	})
	gobject.SignalConnect(obj.GoPointer(), signal, glib.NewCallback(cb))
}
