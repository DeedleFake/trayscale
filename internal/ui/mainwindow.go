package ui

import (
	_ "embed"
	"strings"

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

	pages      map[string]Page
	pageRows   map[string]*PageRow
	statusPage *adw.StatusPage

	ProfileModel     *gtk.StringList
	ProfileSortModel *gtk.SortListModel
}

func NewMainWindow(app *App) *MainWindow {
	win := MainWindow{
		app:      app,
		pages:    make(map[string]Page),
		pageRows: make(map[string]*PageRow),
	}
	fillFromBuilder(&win, menuXML, mainWindowXML)

	win.MainWindow.SetApplication(&app.app.Application)

	win.statusPage = adw.NewStatusPage()
	win.statusPage.SetTitle("Not Connected")
	win.statusPage.SetIconName("network-offline-symbolic")
	win.statusPage.SetDescription("Tailscale is not connected")

	pages := make(map[uintptr]*PageRow)
	pagesModel := win.PeersStack.Pages()
	listmodels.Bind(
		pagesModel,
		NewPageRow,
		func(i uint, row *PageRow) {
			delete(pages, row.Row().Object.Native())
			win.PeersList.Remove(row.Row())
		},
		func(i uint, row *PageRow) {
			win.pageRows[row.Page().Name()] = row

			pages[row.Row().Object.Native()] = row
			win.PeersList.Append(row.Row())
		},
	)
	win.PeersList.SetSortFunc(func(r1, r2 *gtk.ListBoxRow) int {
		p1 := pages[r1.Object.Native()].Page()
		p2 := pages[r2.Object.Native()].Page()

		if v, ok := prioritize("self", p1.Name(), p2.Name()); ok {
			return v
		}
		if v, ok := prioritize("mullvad", p1.Name(), p2.Name()); ok {
			return v
		}
		return strings.Compare(p1.Title(), p2.Title())
	})
	win.PeersList.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil {
			return
		}

		page := pages[row.Object.Native()]
		win.PeersStack.SetVisibleChildName(page.Page().Name())
	})

	win.ProfileModel = gtk.NewStringList(nil)
	win.ProfileSortModel = gtk.NewSortListModel(win.ProfileModel, &stringListSorter.Sorter)
	win.ProfileDropDown.SetModel(win.ProfileSortModel)

	return &win
}

func (win *MainWindow) addPage(name string, page Page) *adw.ViewStackPage {
	win.pages[name] = page
	return win.PeersStack.AddNamed(page.Widget(), name)
}

func (win *MainWindow) removePage(name string, page Page) {
	delete(win.pageRows, name)
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
		row := win.pageRows[name]
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

func (win *MainWindow) Toast(msg string) *adw.Toast {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	win.ToastOverlay.AddToast(toast)
	return toast
}
