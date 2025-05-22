package ui

import (
	"context"
	_ "embed"
	"log/slog"
	"slices"
	"strings"
	"time"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn"
)

//go:embed mainwindow.ui
var mainWindowXML string

type MainWindow struct {
	app *App

	MainWindow      *adw.ApplicationWindow
	ToastOverlay    *adw.ToastOverlay
	SplitView       *adw.NavigationSplitView
	StatusSwitch    *gtk.Switch
	MainMenuButton  *gtk.MenuButton
	PeersList       *gtk.ListBox
	PeersStack      *adw.ViewStack
	WorkSpinner     *adw.Spinner
	ProfileDropDown *gtk.DropDown
	PageMenuButton  *gtk.MenuButton

	pages map[string]Page

	profiles         []ipn.LoginProfile
	profileModel     *gtk.StringList
	profileSortModel *gtk.SortListModel
}

func NewMainWindow(app *App) *MainWindow {
	win := MainWindow{
		app:   app,
		pages: make(map[string]Page),
	}
	fillFromBuilder(&win, mainWindowXML)

	win.MainWindow.SetApplication(&app.app.Application)

	win.PeersStack.NotifyProperty("visible-child-name", func() {
		page := win.pages[win.PeersStack.VisibleChildName()]

		var actions gio.ActionGrouper
		if page != nil {
			actions = page.Actions()
		}
		win.MainWindow.InsertActionGroup("peer", actions)
		win.PageMenuButton.SetSensitive(actions != nil)
	})

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
			vp := row.Page()
			row.SetTitle(vp.Title())
			row.SetIconName(vp.IconName())

			pages[row.Row().Object.Native()] = row
			win.PeersList.Append(row.Row())

			page := win.pages[vp.Name()]
			if page != nil {
				page.Init(row)
				return
			}

			vp.NotifyProperty("title", func() {
				row.SetTitle(vp.Title())
			})
			vp.NotifyProperty("icon-name", func() {
				row.SetIconName(vp.IconName())
			})
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
		name := page.Page().Name()

		win.PeersStack.SetVisibleChildName(name)
	})

	win.profileModel = gtk.NewStringList(nil)
	win.profileSortModel = gtk.NewSortListModel(win.profileModel, &stringListSorter.Sorter)
	win.ProfileDropDown.SetModel(win.profileSortModel)

	win.StatusSwitch.ConnectStateSet(func(s bool) bool {
		if s == win.StatusSwitch.State() {
			return false
		}

		// TODO: Handle this, and other switches, asynchrounously instead
		// of freezing the entire UI.
		ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
		defer cancel()

		f := app.stopTS
		if s {
			f = app.startTS
		}

		err := f(ctx)
		if err != nil {
			slog.Error("set Tailscale status", "err", err)
			win.StatusSwitch.SetActive(!s)
			return true
		}
		return true
	})

	win.ProfileDropDown.NotifyProperty("selected-item", func() {
		item := win.ProfileDropDown.SelectedItem().Cast().(*gtk.StringObject).String()
		index := slices.IndexFunc(win.profiles, func(p ipn.LoginProfile) bool {
			// TODO: Find a reasonable way to do this by profile ID instead.
			return p.Name == item
		})
		if index < 0 {
			slog.Error("selected unknown profile", "name", item)
			return
		}
		profile := win.profiles[index]

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := tsutil.SwitchProfile(ctx, profile.ID)
		if err != nil {
			slog.Error("failed to switch profiles", "err", err, "id", profile.ID, "name", profile.Name)
			return
		}
		<-app.poller.Poll()
	})

	contentVariant := glib.NewVariantString("content")
	win.PeersStack.NotifyProperty("visible-child", func() {
		win.SplitView.ActivateAction("navigation.push", contentVariant)
	})

	return &win
}

func (win *MainWindow) addPage(name string, page Page) *adw.ViewStackPage {
	win.pages[name] = page
	return win.PeersStack.AddNamed(page.Widget(), name)
}

func (win *MainWindow) removePage(name string, page Page) {
	delete(win.pages, name)
	win.PeersStack.Remove(page.Widget())
}

func (win *MainWindow) Update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.NetStatus:
		online := status.Online()
		win.StatusSwitch.SetState(online)
		win.StatusSwitch.SetActive(online)

		win.updatePeers(status)

	case *tsutil.FileStatus:
		if self, ok := win.pages["self"].(*SelfPage); ok {
			self.UpdateFiles(status)
		}

	case *tsutil.ProfileStatus:
		win.updateProfiles(status)
	}
}

func (win *MainWindow) updatePeers(status *tsutil.NetStatus) {
	if !status.Online() {
		if _, ok := win.pages["offline"]; !ok {
			win.addPage("offline", NewOfflinePage(win.app))
		}
		win.updatePages(status)
		return
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

	win.updatePages(status)
}

func (win *MainWindow) updatePages(status *tsutil.NetStatus) {
	var remove []string
	for name, page := range win.pages {
		ok := page.Update(status)
		if !ok {
			remove = append(remove, name)
		}
	}
	for _, name := range remove {
		win.removePage(name, win.pages[name])
	}

	win.PeersList.InvalidateSort()
}

func (win *MainWindow) updateProfiles(status *tsutil.ProfileStatus) {
	win.profiles = status.Profiles
	listmodels.UpdateStrings(win.profileModel, func(yield func(string) bool) {
		for _, profile := range status.Profiles {
			name := profile.Name
			if metadata.Private {
				name = "profile@example.com"
			}
			if !yield(name) {
				return
			}
		}
	})

	profileIndex, ok := listmodels.Index(win.profileSortModel, func(obj *gtk.StringObject) bool {
		return obj.String() == status.Profile.Name
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
