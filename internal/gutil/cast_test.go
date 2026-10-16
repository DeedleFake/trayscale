package gutil_test

import (
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

func TestCast(t *testing.T) {
	str := gtk.NewStringObject("a")
	defer str.Unref()
	asObject := gobject.ObjectNewFromInternalPtr(str.GoPointer()) // the shape list models hand out

	if v, ok := gutil.Cast[gtk.StringObject](asObject, gtk.StringObjectGLibType()); !ok || v.GetString() != "a" {
		t.Errorf("Cast(StringObject) = %v, %v; want the object", v, ok)
	}
	if _, ok := gutil.Cast[gio.SimpleAction](asObject, gio.SimpleActionGLibType()); ok {
		t.Error("Cast(StringObject as SimpleAction) succeeded")
	}

	// Interface GType: a GFile implementation is a GFile.
	file := gio.FileNewForPath("/tmp")
	defer gobject.ObjectNewFromInternalPtr(file.GoPointer()).Unref()
	if !gutil.IsA(file, gio.FileGLibType()) {
		t.Error("IsA(GFile, GFile) = false")
	}

	var nilObj *gobject.Object
	if _, ok := gutil.Cast[gtk.StringObject](nilObj, gtk.StringObjectGLibType()); ok {
		t.Error("Cast(typed nil) succeeded")
	}
	if _, ok := gutil.Cast[gtk.StringObject](nil, gtk.StringObjectGLibType()); ok {
		t.Error("Cast(nil) succeeded")
	}
	if got := gutil.TypeName(str); got != "GtkStringObject" {
		t.Errorf("TypeName = %q", got)
	}
}

func TestMustCastPanics(t *testing.T) {
	str := gtk.NewStringObject("a")
	defer str.Unref()
	defer func() {
		if recover() == nil {
			t.Error("MustCast on the wrong GType did not panic")
		}
	}()
	gutil.MustCast[gio.SimpleAction](str, gio.SimpleActionGLibType())
}
