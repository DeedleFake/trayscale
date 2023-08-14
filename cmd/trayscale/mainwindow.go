package main

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	//go:embed mainwindow.ui
	mainWindowXML []byte

	//go:embed menu.ui
	menuXML []byte
)

type MainWindow struct {
	adw.ApplicationWindow

	ToastOverlay   *adw.ToastOverlay
	Leaflet        *adw.Leaflet
	StatusSwitch   *gtk.Switch
	MainMenuButton *gtk.MenuButton
	BackButton     *gtk.Button
	PeersStack     *gtk.Stack
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
