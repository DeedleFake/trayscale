package ui

import (
	"cmp"
	"context"
	_ "embed"
	"log/slog"
	"slices"
	"strings"
	"time"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/peersearch"
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

	MainWindow       *adw.ApplicationWindow
	ToastOverlay     *adw.ToastOverlay
	SplitView        *adw.NavigationSplitView
	StatusSwitch     *gtk.Switch
	MainMenuButton   *gtk.MenuButton
	PeerSearchButton *gtk.ToggleButton
	PeerSearchBar    *gtk.SearchBar
	PeerSearchEntry  *gtk.SearchEntry
	PeersSidebar     *adw.ViewSwitcherSidebar
	PeersStack       *adw.ViewStack
	WorkSpinner      *adw.Spinner
	ProfileDropDown  *gtk.DropDown
	PageMenuButton   *gtk.MenuButton

	pages       map[string]Page
	showOffline bool
	peerQuery   string
	ipn         *tsutil.IPNStatus

	profiles         []ipn.LoginProfile
	profileModel     *gtk.StringList
	profileSortModel *gtk.SortListModel
	updatingProfiles bool
	activeProfileID  ipn.ProfileID
}

func NewMainWindow(app *App) *MainWindow {
	win := MainWindow{
		app:         app,
		pages:       make(map[string]Page),
		showOffline: true,
	}
	if app.settings != nil {
		win.showOffline = app.settings.Boolean("show-offline-peers")
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

	win.initPeerSearch()

	return &win
}

func (win *MainWindow) initPeerSearch() {
	win.PeerSearchBar.SetKeyCaptureWidget(win.MainWindow)
	win.PeerSearchBar.ConnectEntry(win.PeerSearchEntry)

	win.PeerSearchButton.Connect("notify::active", func() {
		win.PeerSearchBar.SetSearchMode(win.PeerSearchButton.Active())
	})
	win.PeerSearchBar.NotifyProperty("search-mode-enabled", func() {
		searching := win.PeerSearchBar.SearchMode()
		if win.PeerSearchButton.Active() != searching {
			win.PeerSearchButton.SetActive(searching)
		}
		if !searching {
			win.PeerSearchEntry.SetText("")
			win.setPeerQuery("")
		}
	})
	win.PeerSearchEntry.ConnectSearchChanged(func() {
		win.setPeerQuery(win.PeerSearchEntry.Text())
	})
}

func (win *MainWindow) OpenPeerSearch() {
	win.PeerSearchBar.SetSearchMode(true)
	win.PeerSearchEntry.GrabFocus()
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

func (win *MainWindow) SetShowOffline(show bool) {
	if win.showOffline == show {
		return
	}
	win.showOffline = show
	win.applyPeerLayout()
}

func (win *MainWindow) setPeerQuery(q string) {
	q = strings.TrimSpace(q)
	if win.peerQuery == q {
		return
	}
	win.peerQuery = q
	win.applyPeerLayout()
}

func (win *MainWindow) applyPeerLayout() {
	if win.ipn == nil || !win.ipn.Online() {
		return
	}
	if win.restackIfNeeded(win.ipn) {
		for _, vp := range win.viewStackPages() {
			if page := win.pages[vp.Name()]; page != nil {
				page.Update(win.ipn)
			}
		}
	}
	win.syncSections(win.ipn)
}

func (win *MainWindow) updatePeers(status *tsutil.IPNStatus) {
	win.ipn = status
	if !status.Online() {
		if _, ok := win.pages["offline"]; !ok {
			win.addPage("offline", NewOfflinePage(win.app))
		}
		win.updatePages(status)
		return
	}

	win.ensureSectionPages(status)

	for id, peer := range status.Peers {
		if tsutil.IsMullvad(peer) || tsutil.IsShareeNode(peer) {
			continue
		}
		if _, ok := win.pages[string(id)]; ok {
			continue
		}
		win.addPage(string(id), NewPeerPage(win.app, status, peer))
	}

	win.updatePages(status)
}

func (win *MainWindow) ensureSectionPages(status *tsutil.IPNStatus) {
	self, hasSelf := status.Self()
	if hasSelf && win.pages["self"] == nil {
		win.addPage("self", NewSelfPage(win.app, status))
	}
	if hasSelf && tsutil.CanMullvad(self) && win.pages["mullvad"] == nil {
		win.addPage("mullvad", NewMullvadPage(win.app, status))
	}
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

	if win.restackIfNeeded(status) {
		for _, vp := range win.viewStackPages() {
			if page := win.pages[vp.Name()]; page != nil {
				page.Update(status)
			}
		}
	}
	win.syncSections(status)
}

func (win *MainWindow) stackPageNames() []string {
	vps := win.viewStackPages()
	names := make([]string, 0, len(vps))
	for _, vp := range vps {
		names = append(names, vp.Name())
	}
	return names
}

func (win *MainWindow) desiredPageOrder(status *tsutil.IPNStatus) []string {
	if !status.Online() {
		if win.pages["offline"] != nil {
			return []string{"offline"}
		}
		return nil
	}

	if win.peerQuery != "" {
		return win.searchPageOrder(status)
	}

	var order []string
	if win.pages["self"] != nil {
		order = append(order, "self")
	}
	if win.pages["mullvad"] != nil {
		order = append(order, "mullvad")
	}

	var exits, others, offline []string
	for name := range win.pages {
		switch name {
		case "self", "mullvad", "offline":
			continue
		}
		peer, ok := status.Peers[tailcfg.StableNodeID(name)]
		if !ok || !peer.Valid() || tsutil.IsShareeNode(peer) {
			continue
		}
		if peerIsExitNodeOption(peer) {
			exits = append(exits, name)
			continue
		}
		if !peerIsOnline(peer) {
			if win.showOffline {
				offline = append(offline, name)
			}
			continue
		}
		others = append(others, name)
	}
	sortPeerPageNames(status, exits)
	sortPeerPageNames(status, others)
	sortPeerPageNames(status, offline)
	return slices.Concat(order, exits, others, offline)
}

func (win *MainWindow) searchPageOrder(status *tsutil.IPNStatus) []string {
	type hit struct {
		name  string
		peer  tailcfg.NodeView
		score int
	}
	tokens := strings.Fields(win.peerQuery)
	var hits []hit
	for name := range win.pages {
		switch name {
		case "self", "mullvad", "offline":
			continue
		}
		peer, ok := status.Peers[tailcfg.StableNodeID(name)]
		if !ok || !peer.Valid() || tsutil.IsShareeNode(peer) {
			continue
		}
		score, match := peersearch.Score(peer, tokens)
		if !match {
			continue
		}
		hits = append(hits, hit{name: name, peer: peer, score: score})
	}
	slices.SortFunc(hits, func(a, b hit) int {
		return cmp.Or(
			cmp.Compare(b.score, a.score),
			cmp.Compare(peerName(a.peer), peerName(b.peer)),
			tsutil.ComparePeers(a.peer, b.peer),
		)
	})
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.name
	}
	return names
}

func sortPeerPageNames(status *tsutil.IPNStatus, names []string) {
	slices.SortFunc(names, func(a, b string) int {
		p1 := status.Peers[tailcfg.StableNodeID(a)]
		p2 := status.Peers[tailcfg.StableNodeID(b)]
		return cmp.Or(
			cmp.Compare(peerName(p1), peerName(p2)),
			tsutil.ComparePeers(p1, p2),
		)
	})
}

func (win *MainWindow) restackIfNeeded(status *tsutil.IPNStatus) bool {
	desired := win.desiredPageOrder(status)
	if slices.Equal(win.stackPageNames(), desired) {
		return false
	}
	win.restack(desired)
	return true
}

func (win *MainWindow) restack(order []string) {
	visible := win.PeersStack.VisibleChildName()
	for _, vp := range win.viewStackPages() {
		if page := win.pages[vp.Name()]; page != nil {
			win.PeersStack.Remove(page.Widget())
		}
	}
	for _, name := range order {
		if page := win.pages[name]; page != nil {
			win.addPage(name, page)
		}
	}
	win.restoreVisible(visible)
}

func (win *MainWindow) restoreVisible(name string) {
	for _, vp := range win.viewStackPages() {
		if vp.Name() == name {
			win.PeersStack.SetVisibleChildName(name)
			return
		}
	}
	if remaining := win.viewStackPages(); len(remaining) > 0 {
		win.PeersStack.SetVisibleChildName(remaining[0].Name())
	}
}

func (win *MainWindow) syncSections(status *tsutil.IPNStatus) {
	if win.peerQuery != "" {
		for _, vp := range win.viewStackPages() {
			vp.SetStartsSection(false)
			vp.SetSectionTitle("")
		}
		return
	}

	var startedExit, startedOther, startedOffline bool
	for _, vp := range win.viewStackPages() {
		name := vp.Name()
		switch name {
		case "self":
			vp.SetStartsSection(true)
			vp.SetSectionTitle("This machine")
			continue
		case "offline":
			vp.SetStartsSection(false)
			vp.SetSectionTitle("")
			continue
		case "mullvad":
			startSection(vp, &startedExit, "Exit Nodes")
			continue
		}

		peer := status.Peers[tailcfg.StableNodeID(name)]
		if peerIsExitNodeOption(peer) {
			startSection(vp, &startedExit, "Exit Nodes")
			continue
		}
		if !peerIsOnline(peer) {
			startSection(vp, &startedOffline, "Offline")
			continue
		}
		startSection(vp, &startedOther, "Online")
	}
}

func startSection(vp *adw.ViewStackPage, started *bool, title string) {
	if *started {
		vp.SetStartsSection(false)
		vp.SetSectionTitle("")
		return
	}
	vp.SetStartsSection(true)
	vp.SetSectionTitle(title)
	*started = true
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
