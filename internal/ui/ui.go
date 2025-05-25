package ui

/*
#cgo pkg-config: libadwaita-1

#include <adwaita.h>
*/
import "C"

import (
	"runtime"
	"unsafe"
)

func toCStrings(str []string) []*C.char {
	cstr := make([]*C.char, 0, len(str))
	for _, s := range str {
		cstr = append(cstr, C.CString(s))
	}
	return cstr
}

func freeAll[T any, P *T](cstr []P) {
	for _, s := range cstr {
		C.free(unsafe.Pointer(s))
	}
}

type ref[T any, P *T] struct {
	p P
}

func take[T any, P *T](p P) *ref[T, P] {
	r := borrow(p)
	C.g_object_ref(r.gpointer())
	runtime.AddCleanup(r, func(p C.gpointer) { C.g_object_unref(p) }, r.gpointer())
	return r
}

func borrow[T any, P *T](p P) *ref[T, P] {
	return &ref[T, P]{p: p}
}

func (r *ref[T, P]) gpointer() C.gpointer {
	return C.gpointer(unsafe.Pointer(r.p))
}
