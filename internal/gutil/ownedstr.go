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
// copied and then freed with [glib.Free]. This file can be removed
// once puregotk frees them itself:
// https://codeberg.org/puregotk/puregotk/issues/61
var (
	xSettingsGetString        func(uintptr, string) uintptr
	xFileGetPath              func(uintptr) uintptr
	xFileGetBasename          func(uintptr) uintptr
	xApplicationAddMainOption func(uintptr, string, byte, glib.OptionFlags, glib.OptionArg, string, uintptr)
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
	core.PuregoSafeRegister(&xApplicationAddMainOption, libs, "g_application_add_main_option")
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

// ApplicationAddMainOption registers one option with a NULL
// arg_description. puregotk's AddMainOption takes a Go string, so ""
// arrives as a non-NULL empty C string. GLib then gettext's it, and
// gettext("") is the catalog header.
func ApplicationAddMainOption(app interface{ GoPointer() uintptr }, longName string, shortName byte, flags glib.OptionFlags, arg glib.OptionArg, description string) {
	xApplicationAddMainOption(app.GoPointer(), longName, shortName, flags, arg, description, 0)
}
