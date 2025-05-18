package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
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
	app *App

	MainWindow      *adw.ApplicationWindow
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
	pages          map[string]Page
	rows           map[string]*PageRow
	statusPage     *adw.StatusPage

	ProfileModel     *gtk.StringList
	ProfileSortModel *gtk.SortListModel
}

func NewMainWindow(app *App) *MainWindow {
	win := MainWindow{
		app:   app,
		pages: make(map[string]Page),
		rows:  make(map[string]*PageRow),
	}
	fillFromBuilder(&win, menuXML, mainWindowXML)

	win.MainWindow.SetApplication(&app.app.Application)

	win.statusPage = adw.NewStatusPage()
	win.statusPage.SetTitle("Not Connected")
	win.statusPage.SetIconName("network-offline-symbolic")
	win.statusPage.SetDescription("Tailscale is not connected")

	win.PeersModel = win.PeersStack.Pages()
	win.PeersSortModel = gtk.NewSortListModel(win.PeersModel, &peersListSorter.Sorter)
	listmodels.BindListBox(win.PeersList, win.PeersSortModel, win.createPeersRow)
	win.PeersList.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil {
			return
		}

		page := win.PeersSortModel.Item(uint(row.Index())).Cast().(*adw.ViewStackPage)
		win.PeersStack.SetVisibleChildName(page.Name())
	})

	win.ProfileModel = gtk.NewStringList(nil)
	win.ProfileSortModel = gtk.NewSortListModel(win.ProfileModel, &stringListSorter.Sorter)
	win.ProfileDropDown.SetModel(win.ProfileSortModel)

	return &win
}

func (win *MainWindow) createPeersRow(page *adw.ViewStackPage) gtk.Widgetter {
	icon := gtk.NewImage()
	icon.NotifyProperty("icon-name", func() {
		page.SetIconName(icon.IconName())
	})

	row := adw.NewActionRow()
	row.AddPrefix(icon)
	row.NotifyProperty("title", func() {
		page.SetTitle(row.Title())
	})

	win.rows[page.Name()] = &PageRow{
		Row:  row,
		Icon: icon,
	}

	return row
}

func (win *MainWindow) addPage(name string, page Page) *adw.ViewStackPage {
	win.pages[name] = page
	return win.PeersStack.AddNamed(page.Widget(), name)
}

func (win *MainWindow) removePage(name string, page Page) {
	delete(win.rows, name)
	delete(win.pages, name)
	win.PeersStack.Remove(page.Widget())
}

func (win *MainWindow) Update(status tsutil.Status) {
	online := status.Online()
	win.StatusSwitch.SetState(online)
	win.StatusSwitch.SetActive(online)

	win.updateProfiles(status)
	win.updatePeers(status)
}

func (win *MainWindow) updatePeersOffline() {
	stack := win.PeersStack

	var found bool
	for name, page := range win.pages {
		if name == "status" {
			found = true
			continue
		}

		win.removePage(name, page)
	}
	if !found {
		stack.AddTitled(win.statusPage, "status", "Not Connected")
	}
}

func (win *MainWindow) updatePeers(status tsutil.Status) {
	if !status.Online() {
		win.updatePeersOffline()
		return
	}

	if win.PeersStack.ChildByName("status") != nil {
		win.PeersStack.Remove(win.statusPage)
	}

	if _, ok := win.pages["self"]; !ok {
		win.addPage("self", NewSelfPage(win.app, status))
	}
	if _, ok := win.pages["mullvad"]; !ok && tsutil.CanMullvad(status.Status.Self) {
		win.addPage("mullvad", NewMullvadPage(win.app, status))
	}

	for _, peer := range status.Status.Peer {
		if tsutil.IsMullvad(peer) {
			continue
		}

		name := string(peer.ID)
		if _, ok := win.pages[name]; ok {
			continue
		}

		win.addPage(name, NewPeerPage(win.app, status, peer))
	}

	var remove []string
	for name, page := range win.pages {
		row := win.rows[name]
		ok := page.Update(row, status)
		if !ok {
			remove = append(remove, name)
		}
	}
	for _, name := range remove {
		win.removePage(name, win.pages[name])
	}

	win.PeersList.InvalidateSort()
}

func (win *MainWindow) updateProfiles(s tsutil.Status) {
	listmodels.UpdateStrings(win.ProfileModel, func(yield func(string) bool) {
		for _, profile := range s.Profiles {
			if !yield(profile.Name) {
				return
			}
		}
	})

	profileIndex, ok := listmodels.Index(win.ProfileSortModel, func(obj *gtk.StringObject) bool {
		return obj.String() == s.Profile.Name
	})
	if ok {
		win.ProfileDropDown.SetSelected(uint(profileIndex))
	}
}
