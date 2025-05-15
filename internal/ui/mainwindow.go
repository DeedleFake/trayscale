package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/efogdev/gotk4-adwaita/pkg/adw"
)

var (
	//go:embed mainwindow.ui
	mainWindowXML string

	//go:embed menu.ui
	menuXML string
)

type MainWindow struct {
	*adw.ApplicationWindow `gtk:"MainWindow"`

	ToastOverlay    *adw.ToastOverlay
	SplitView       *adw.NavigationSplitView
	StatusSwitch    *gtk.Switch
	MainMenuButton  *gtk.MenuButton
	PeersList       *gtk.ListBox
	PeersStack      *adw.ViewStack
	WorkSpinner     *gtk.Spinner
	ProfileDropDown *gtk.DropDown

	PeersModel     *gtk.SelectionModel
	PeersSortModel *gtk.SortListModel

	ProfileModel     *gtk.StringList
	ProfileSortModel *gtk.SortListModel
}

func NewMainWindow(app *gtk.Application) *MainWindow {
	var win MainWindow
	fillFromBuilder(&win, menuXML, mainWindowXML)

	win.SetApplication(app)

	win.PeersModel = win.PeersStack.Pages()
	win.PeersSortModel = gtk.NewSortListModel(win.PeersModel, &peersListSorter.Sorter)
	BindListBoxModel(win.PeersList, win.PeersSortModel, win.createPeersRow)
	win.PeersList.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row != nil {
			win.PeersModel.SelectItem(uint(row.Index()), true)
		}
	})

	win.ProfileModel = gtk.NewStringList(nil)
	win.ProfileSortModel = gtk.NewSortListModel(win.ProfileModel, &stringListSorter.Sorter)
	win.ProfileDropDown.SetModel(win.ProfileSortModel)

	return &win
}

func (win *MainWindow) createPeersRow(page *adw.ViewStackPage) gtk.Widgetter {
	icon := gtk.NewImageFromIconName(page.IconName())
	page.NotifyProperty("icon-name", func() {
		icon.SetFromIconName(page.IconName())
	})

	row := adw.NewActionRow()
	row.AddPrefix(icon)

	row.SetTitle(page.Title())
	page.NotifyProperty("title", func() {
		row.SetTitle(page.Title())
	})

	return row
}
