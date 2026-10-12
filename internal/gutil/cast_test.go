package gutil_test

import (
	"testing"

	"deedles.dev/trayscale/internal/gutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func TestCast(t *testing.T) {
	str := gtk.NewStringObject("a")
	asObject := glib.BaseObject(str) // the shape list models hand out

	if v, ok := gutil.Cast[*gtk.StringObject](asObject, gtk.GTypeStringObject); !ok || v.String() != "a" {
		t.Errorf("Cast(StringObject) = %v, %v; want the object", v, ok)
	}
	if _, ok := gutil.Cast[*gio.SimpleAction](asObject, gio.GTypeSimpleAction); ok {
		t.Error("Cast(StringObject as SimpleAction) succeeded")
	}

	// Interface GType: a GFile implementation is a gio.Filer.
	file := gio.NewFileForPath("/tmp")
	if _, ok := gutil.Cast[gio.Filer](glib.BaseObject(file), gio.GTypeFile); !ok {
		t.Error("Cast(GFile as gio.Filer) failed")
	}

	var nilObj *glib.Object
	if _, ok := gutil.Cast[*gtk.StringObject](nilObj, gtk.GTypeStringObject); ok {
		t.Error("Cast(typed nil) succeeded")
	}
	if _, ok := gutil.Cast[*gtk.StringObject](nil, gtk.GTypeStringObject); ok {
		t.Error("Cast(nil) succeeded")
	}
	var nilStr *gtk.StringObject
	if _, ok := gutil.Cast[*gtk.StringObject](nilStr, gtk.GTypeStringObject); ok {
		t.Error("Cast(typed nil wrapper) succeeded")
	}
}

func TestMustCastPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustCast on the wrong GType did not panic")
		}
	}()
	gutil.MustCast[*gio.SimpleAction](gtk.NewStringObject("a"), gio.GTypeSimpleAction)
}
