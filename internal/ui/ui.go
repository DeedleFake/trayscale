package ui

/*
#cgo pkg-config: libadwaita-1

#include <adwaita.h>

#include "ui.h"

gboolean _idle(gpointer h);
*/
import "C"

import (
	"embed"
	"io/fs"
	"runtime/cgo"
	"unsafe"

	"deedles.dev/trayscale/internal/metadata"
)

//go:embed *.ui *.css
var files embed.FS

func init() {
	C.APP_ID = C.CString(metadata.AppID)
}

//export ui_get_file
func ui_get_file(name *C.char) *C.char {
	data, err := fs.ReadFile(files, C.GoString(name))
	if err != nil {
		panic(err)
	}

	return C.CString(string(data))
}

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

func idle(f func()) {
	C.g_idle_add((*[0]byte)(C._idle), C.gpointer(cgo.NewHandle(f)))
}

//export _idle
func _idle(p C.gpointer) C.gboolean {
	h := cgo.Handle(p)
	defer h.Delete()

	h.Value().(func())()
	return C.G_SOURCE_REMOVE
}

//export cgo_handle_delete
func cgo_handle_delete(p C.uintptr_t) {
	if p != 0 {
		cgo.Handle(p).Delete()
	}
}
