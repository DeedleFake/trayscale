package ui

/*
#cgo pkg-config: libadwaita-1

#include <adwaita.h>

char *APP_ID = NULL;

#define DEFINE_RESOURCE(name) char *name = NULL; int name##_LEN = 0
DEFINE_RESOURCE(APP_CSS);
*/
import "C"

import (
	"embed"
	"io"
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
