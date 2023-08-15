package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	//go:embed mainwindow.ui
	mainWindowXML string

	//go:embed menu.ui
	menuXML string
)

type MainWindow struct {
	*adw.ApplicationWindow `gtk:"MainWindow"`

	ToastOverlay   *adw.ToastOverlay
	Leaflet        *adw.Leaflet
	StatusSwitch   *gtk.Switch
	MainMenuButton *gtk.MenuButton
	BackButton     *gtk.Button
	PeersStack     *gtk.Stack
}

func NewMainWindow(app *gtk.Application) *MainWindow {
	win := newFromBuilder[MainWindow](menuXML, mainWindowXML)
	win.SetApplication(app)
	return win
}
