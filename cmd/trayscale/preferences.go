package main

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed preferences.ui
var preferencesXML []byte

type PreferencesWindow struct {
	adw.PreferencesWindow

	UseTrayIconRow *adw.ActionRow
	UseTrayIcon    *gtk.Switch
}

var preferencesWindowType = coreglib.RegisterSubclass[*PreferencesWindow](
	coreglib.WithClassInit(func(class *gtk.WidgetClass) {
		class.SetTemplate(glib.NewBytesWithGo(preferencesXML))
	}),
)

func NewPreferencesWindow() *PreferencesWindow {
	return preferencesWindowType.New()
}
