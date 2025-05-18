package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/util/set"
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
	statusPage     *adw.StatusPage

	ProfileModel     *gtk.StringList
	ProfileSortModel *gtk.SortListModel
}

func NewMainWindow(app *App) *MainWindow {
	win := MainWindow{
		app:   app,
		pages: make(map[string]Page),
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

		stack.Remove(page.Widget())
		delete(win.pages, name)
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

	stack := win.PeersStack
	if stack.ChildByName("status") != nil {
		stack.Remove(win.statusPage)
	}

	found := make(set.Set[string])
	for name, page := range win.pages {
		vp := stack.Page(page.Widget())
		ok := page.Update(win.app, vp, status)
		if !ok {
			stack.Remove(vp.Child())
			delete(win.pages, name)
			continue
		}
		found.Add(name)
	}

	if !found.Contains("self") {
		page := NewSelfPage(win.app, status)
		vp := stack.AddNamed(page.Widget(), "self")
		page.Update(win.app, vp, status)
		win.pages["self"] = page
	}
	if !found.Contains("mullvad") && tsutil.CanMullvad(status.Status.Self) {
		page := NewMullvadPage(win.app, status)
		vp := stack.AddNamed(page.Widget(), "mullvad")
		page.Update(win.app, vp, status)
		win.pages["mullvad"] = page
	}

	for _, peer := range status.Status.Peer {
		if !found.Contains(string(peer.ID)) && !tsutil.IsMullvad(peer) {
			page := NewPeerPage(win.app, status, peer)
			vp := stack.AddNamed(page.Widget(), string(peer.ID))
			page.Update(win.app, vp, status)
			win.pages[string(peer.ID)] = page
		}
	}

	win.SortPeers()
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

func (win *MainWindow) SortPeers() {
	page, ok := win.PeersModel.Item(win.PeersModel.Selection().Minimum()).Cast().(*adw.ViewStackPage)
	win.PeersSortModel.SetSorter(nil)
	win.PeersSortModel.SetSorter(&peersListSorter.Sorter)
	if ok {
		i, ok := listmodels.Index(win.PeersSortModel, func(vp *adw.ViewStackPage) bool {
			return vp.Name() == page.Name()
		})
		if ok {
			win.PeersList.SelectRow(win.PeersList.RowAtIndex(int(i)))
		}
	}
}
