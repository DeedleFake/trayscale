package gutil

import (
	"codeberg.org/puregotk/purego"
	"codeberg.org/puregotk/puregotk/pkg/core"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
)

// puregotk binds functions that return transfer-full strings as
// returning a Go string, which copies the C string and never frees
// it. These bindings return the raw pointer instead so that it can be
// copied and then freed with [glib.Free].
var (
	xSettingsGetString func(uintptr, string) uintptr
	xFileGetPath       func(uintptr) uintptr
	xFileGetBasename   func(uintptr) uintptr
)

func init() {
	var libs []uintptr
	for _, path := range core.GetPaths("GIO") {
		lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		libs = append(libs, lib)
	}

	core.PuregoSafeRegister(&xSettingsGetString, libs, "g_settings_get_string")
	core.PuregoSafeRegister(&xFileGetPath, libs, "g_file_get_path")
	core.PuregoSafeRegister(&xFileGetBasename, libs, "g_file_get_basename")
}

// TakeString copies the transfer-full C string p into a Go string and
// frees p. A nil p yields "".
func TakeString(p uintptr) string {
	if p == 0 {
		return ""
	}
	defer glib.Free(p)
	return core.GoString(p)
}

// SettingsString is like [gio.Settings.GetString] but does not leak
// the returned string.
func SettingsString(s *gio.Settings, key string) string {
	return TakeString(xSettingsGetString(s.GoPointer(), key))
}

// FilePath is like [gio.File.GetPath] but does not leak the returned
// string. It returns "" if file has no local path.
func FilePath(file interface{ GoPointer() uintptr }) string {
	return TakeString(xFileGetPath(file.GoPointer()))
}

// FileBasename is like [gio.File.GetBasename] but does not leak the
// returned string.
func FileBasename(file interface{ GoPointer() uintptr }) string {
	return TakeString(xFileGetBasename(file.GoPointer()))
}
