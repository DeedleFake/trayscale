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
	SplitView      *adw.NavigationSplitView
	StatusSwitch   *gtk.Switch
	MainMenuButton *gtk.MenuButton
	PeersStack     *gtk.Stack
	WorkSpinner    *gtk.Spinner
}

func NewMainWindow(app *gtk.Application) *MainWindow {
	win := newFromBuilder[MainWindow](menuXML, mainWindowXML)
	win.SetApplication(app)
	return win
}
