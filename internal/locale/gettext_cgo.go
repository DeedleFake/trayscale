//go:build cgo

package locale

/*
#include <libintl.h>
#include <locale.h>
#include <stdlib.h>
*/
import "C"

import "unsafe"

// bindTextDomain registers dir as the gettext catalog root for domain
// (<dir>/<lang>/LC_MESSAGES/<domain>.mo) and selects UTF-8 codeset.
func bindTextDomain(domain, dir string) {
	cDomain := C.CString(domain)
	cDir := C.CString(dir)
	cUTF8 := C.CString("UTF-8")
	defer C.free(unsafe.Pointer(cDomain))
	defer C.free(unsafe.Pointer(cDir))
	defer C.free(unsafe.Pointer(cUTF8))

	C.bindtextdomain(cDomain, cDir)
	C.bind_textdomain_codeset(cDomain, cUTF8)
	C.textdomain(cDomain)
}

// setLocaleAll calls setlocale(LC_ALL, name). Empty name means "".
// Returns the resulting locale name, or "" on failure.
func setLocaleAll(name string) string {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	res := C.setlocale(C.LC_ALL, cName)
	if res == nil {
		return ""
	}
	return C.GoString(res)
}

// dGettext looks up msgid in domain (for tests).
func dGettext(domain, msgid string) string {
	cDomain := C.CString(domain)
	cMsgid := C.CString(msgid)
	defer C.free(unsafe.Pointer(cDomain))
	defer C.free(unsafe.Pointer(cMsgid))
	p := C.dgettext(cDomain, cMsgid)
	return C.GoString(p)
}
