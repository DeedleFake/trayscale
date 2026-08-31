package ui

import (
	"cmp"
	"context"
	_ "embed"
	"log/slog"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
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
	PeersSidebar    *adw.ViewSwitcherSidebar
	PeersStack      *adw.ViewStack
	WorkSpinner     *adw.Spinner
	ProfileDropDown *gtk.DropDown
	PageMenuButton  *gtk.MenuButton

	pages map[string]Page

	profiles         []ipn.LoginProfile
	profileModel     *gtk.StringList
	profileSortModel *gtk.SortListModel
	updatingProfiles bool
	activeProfileID  ipn.ProfileID
}

type stackedPage struct {
	name string
	page Page
}

func NewMainWindow(app *App) *MainWindow {
	win := MainWindow{
		app:   app,
		pages: make(map[string]Page),
	}
	gutil.FillFromUI(&win, menuXML, mainWindowXML)

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
		if win.updatingProfiles {
			return
		}

		obj, ok := win.ProfileDropDown.SelectedItem().Cast().(*gtk.StringObject)
		if !ok {
			return
		}

		item := obj.String()
		index := slices.IndexFunc(win.profiles, func(p ipn.LoginProfile) bool {
			// TODO: Find a reasonable way to do this by profile ID instead.
			return p.Name == item
		})
		if index < 0 {
			slog.Error("selected unknown profile", "name", item)
			return
		}
		profile := win.profiles[index]

		if profile.ID == win.activeProfileID {
			return
		}

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
	win.PeersSidebar.ConnectActivated(func() {
		win.SplitView.ActivateAction("navigation.push", contentVariant)
	})

	return &win
}

func (win *MainWindow) addPage(name string, page Page) *adw.ViewStackPage {
	win.pages[name] = page
	vp := win.PeersStack.AddNamed(page.Widget(), name)
	page.Init(vp)
	return vp
}

func (win *MainWindow) viewStackPages() []*adw.ViewStackPage {
	model := win.PeersStack.Pages()
	pages := make([]*adw.ViewStackPage, 0, model.NItems())
	for i := range model.NItems() {
		obj := model.Item(i)
		if obj == nil {
			continue
		}
		vp, ok := obj.Cast().(*adw.ViewStackPage)
		if !ok {
			continue
		}
		pages = append(pages, vp)
	}
	return pages
}

func (win *MainWindow) removePage(name string, page Page) {
	reselect := win.PeersStack.VisibleChildName() == name

	delete(win.pages, name)
	win.PeersStack.Remove(page.Widget())

	if !reselect {
		return
	}
	remaining := win.viewStackPages()
	if len(remaining) == 0 {
		return
	}
	win.PeersStack.SetVisibleChildName(remaining[0].Name())
}

func (win *MainWindow) Update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
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

func (win *MainWindow) updatePeers(status *tsutil.IPNStatus) {
	if !status.Online() {
		if _, ok := win.pages["offline"]; !ok {
			win.addPage("offline", NewOfflinePage(win.app))
		}
		win.updatePages(status)
		return
	}

	win.ensureSectionPages(status)

	var newPeers []tailcfg.NodeView
	for id, peer := range status.Peers {
		if tsutil.IsMullvad(peer) {
			continue
		}
		if _, ok := win.pages[string(id)]; ok {
			continue
		}
		newPeers = append(newPeers, peer)
	}
	slices.SortFunc(newPeers, func(p1, p2 tailcfg.NodeView) int {
		return cmp.Or(
			cmp.Compare(peerName(p1), peerName(p2)),
			tsutil.ComparePeers(p1, p2),
		)
	})
	for _, peer := range newPeers {
		win.addPage(string(peer.StableID()), NewPeerPage(win.app, status, peer))
	}

	win.updatePages(status)
}

func (win *MainWindow) ensureSectionPages(status *tsutil.IPNStatus) {
	self, hasSelf := status.Self()
	needSelf := hasSelf && win.pages["self"] == nil
	needMullvad := hasSelf && tsutil.CanMullvad(self) && win.pages["mullvad"] == nil
	if !needSelf && !needMullvad {
		return
	}

	// ViewStack only appends, so lift peers (and Mullvad, if Self is late)
	// before inserting these section pages.
	visible := win.PeersStack.VisibleChildName()
	peers := win.detachPeerPages()
	if needSelf {
		if mullvad := win.pages["mullvad"]; mullvad != nil {
			win.PeersStack.Remove(mullvad.Widget())
			win.addPage("self", NewSelfPage(win.app, status))
			win.addPage("mullvad", mullvad)
		} else {
			win.addPage("self", NewSelfPage(win.app, status))
		}
	}
	if needMullvad {
		win.addPage("mullvad", NewMullvadPage(win.app, status))
	}
	for _, e := range peers {
		win.addPage(e.name, e.page)
	}
	if win.pages[visible] != nil {
		win.PeersStack.SetVisibleChildName(visible)
	}
}

func (win *MainWindow) detachPeerPages() []stackedPage {
	var peers []stackedPage
	for _, vp := range win.viewStackPages() {
		name := vp.Name()
		switch name {
		case "self", "mullvad", "offline":
			continue
		}
		page := win.pages[name]
		if page == nil {
			continue
		}
		peers = append(peers, stackedPage{name, page})
	}
	for _, e := range peers {
		win.PeersStack.Remove(e.page.Widget())
	}
	return peers
}

func (win *MainWindow) updatePages(status *tsutil.IPNStatus) {
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

	win.syncPeerSection()
}

func (win *MainWindow) syncPeerSection() {
	first := true
	for _, vp := range win.viewStackPages() {
		switch vp.Name() {
		case "self", "mullvad", "offline":
			continue
		}
		if first {
			vp.SetStartsSection(true)
			vp.SetSectionTitle("Peers")
			first = false
			continue
		}
		vp.SetStartsSection(false)
		vp.SetSectionTitle("")
	}
}

func (win *MainWindow) updateProfiles(status *tsutil.ProfileStatus) {
	win.updatingProfiles = true
	defer func() { win.updatingProfiles = false }()

	win.profiles = status.Profiles
	win.activeProfileID = status.Profile.ID

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
