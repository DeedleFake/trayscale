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
	"io"
	"runtime/cgo"
	"unsafe"

	"deedles.dev/trayscale/internal/metadata"
)

//go:embed *.ui *.css
var files embed.FS

func init() {
	C.APP_ID = C.CString(metadata.AppID)

	C.APP_CSS, C.APP_CSS_LEN = exportFile("app.css")
}

func exportFile(name string) (*C.char, C.int) {
	file, err := files.Open(name)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}

	return C.CString(string(data)), C.int(len(data))
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
func _idle(h C.gpointer) C.gboolean {
	cgo.Handle(h).Value().(func())()
	return C.G_SOURCE_REMOVE
}
