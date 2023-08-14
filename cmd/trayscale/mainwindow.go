package main

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed mainwindow.ui
var mainWindowXML []byte

type MainWindow struct {
	adw.ApplicationWindow

	ToastOverlay *adw.ToastOverlay
	PeersStack   *gtk.Stack
	StatusSwitch *gtk.Switch
	Leaflet      *adw.Leaflet
	BackButton   *gtk.Button
}

var mainWindowType = coreglib.RegisterSubclass[*MainWindow](
	coreglib.WithClassInit(func(class *gtk.WidgetClass) {
		class.SetTemplate(glib.NewBytesWithGo(mainWindowXML))
	}),
)

func NewMainWindow(app *gtk.Application) *MainWindow {
	return mainWindowType.NewWithProperties(map[string]any{
		"application": app,
	})
}
