package ui

import (
	"cmp"
	"context"
	_ "embed"
	"iter"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/peersearch"
	"deedles.dev/trayscale/internal/tsutil"
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
	stackPages  map[string]*adw.ViewStackPage // Each holds a reference.
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
		stackPages:  make(map[string]*adw.ViewStackPage),
		showOffline: true,
	}
	if app.settings != nil {
		win.showOffline = app.settings.GetBoolean("show-offline-peers")
	}
	gutil.FillFromUI(&win, menuXML, mainWindowXML)

	win.MainWindow.SetApplication(&app.app.Application)

	gutil.ConnectNotify(win.PeersStack, "visible-child-name", func() {
		page := win.pages[win.PeersStack.GetVisibleChildName()]

		var actions *gio.SimpleActionGroup
		if page != nil {
			actions = page.Actions()
		}
		if actions != nil {
			win.MainWindow.InsertActionGroup("peer", actions)
		} else {
			removeActionGroup(&win.MainWindow.Widget, "peer")
		}
		win.PageMenuButton.SetSensitive(actions != nil)
	})

	win.profileModel = gtk.NewStringList(nil)
	// gtk_sort_list_model_new takes ownership of the model and sorter.
	win.profileModel.Ref()
	win.profileSortModel = gtk.NewSortListModel(win.profileModel, &newStringSorter().Sorter)
	win.ProfileDropDown.SetModel(win.profileSortModel)

	gutil.Connect(win.StatusSwitch, win.StatusSwitch.ConnectStateSet, func(_ gtk.Switch, s bool) bool {
		if s == win.StatusSwitch.GetState() {
			return false
		}
		win.setStatus(context.TODO(), s)
		return true
	})

	gutil.ConnectNotify(win.ProfileDropDown, "selected-item", func() {
		if win.updatingProfiles {
			return
		}

		sel := win.ProfileDropDown.GetSelectedItem()
		if sel == nil {
			return
		}
		defer sel.Unref()
		obj, ok := gutil.Cast[gtk.StringObject](sel, gtk.StringObjectGLibType())
		if !ok {
			return
		}

		item := obj.GetString()
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

	contentVariant := glib.NewVariantString("content").RefSink()
	gutil.Connect(win.PeersSidebar, win.PeersSidebar.ConnectActivated, func(adw.ViewSwitcherSidebar) {
		win.SplitView.ActivateActionVariant("navigation.push", contentVariant)
	})
	gutil.OnFinalize(win.PeersSidebar, contentVariant.Unref)

	win.initPeerSearch()

	return &win
}

func (win *MainWindow) initPeerSearch() {
	win.PeerSearchBar.SetKeyCaptureWidget(&win.MainWindow.Widget)
	win.PeerSearchBar.ConnectEntry(win.PeerSearchEntry)

	gutil.ConnectNotify(win.PeerSearchButton, "active", func() {
		win.PeerSearchBar.SetSearchMode(win.PeerSearchButton.GetActive())
	})
	gutil.ConnectNotify(win.PeerSearchBar, "search-mode-enabled", func() {
		searching := win.PeerSearchBar.GetSearchMode()
		if win.PeerSearchButton.GetActive() != searching {
			win.PeerSearchButton.SetActive(searching)
		}
		if !searching {
			win.PeerSearchEntry.SetText("")
			win.setPeerQuery("")
		}
	})
	gutil.Connect(win.PeerSearchEntry, win.PeerSearchEntry.ConnectSearchChanged, func(gtk.SearchEntry) {
		win.setPeerQuery(win.PeerSearchEntry.GetText())
	})
}

func (win *MainWindow) OpenPeerSearch() {
	win.PeerSearchBar.SetSearchMode(true)
	win.PeerSearchEntry.GrabFocus()
}

func (win *MainWindow) addPage(name string, page Page) {
	win.pages[name] = page
	if old := win.stackPages[name]; old != nil {
		old.Unref()
	}
	vp := win.PeersStack.AddNamed(page.Widget(), name)
	win.stackPages[name] = vp
	page.Bind(vp)
}

// viewStackPage is the name and the (borrowed) ViewStackPage of one
// page in the stack.
type viewStackPage struct {
	name string
	page *adw.ViewStackPage
}

func (win *MainWindow) viewStackPages() []viewStackPage {
	// GtkSelectionModel is a GListModel.
	model := &gio.ListModelBase{Ptr: win.PeersStack.GetPages().GoPointer()}
	defer gutil.Obj(model).Unref()

	pages := make([]viewStackPage, 0, model.GetNItems())
	for _, obj := range listmodels.Objects(model) {
		vp, ok := gutil.Cast[adw.ViewStackPage](obj, adw.ViewStackPageGLibType())
		if !ok {
			continue
		}
		// The stack keeps the page alive after the model's item
		// reference is dropped.
		pages = append(pages, viewStackPage{name: vp.GetName(), page: vp})
	}
	return pages
}

func (win *MainWindow) removePage(name string, page Page) {
	reselect := win.PeersStack.GetVisibleChildName() == name

	delete(win.pages, name)
	win.PeersStack.Remove(page.Widget())
	if vp := win.stackPages[name]; vp != nil {
		vp.Unref()
		delete(win.stackPages, name)
	}
	page.Destroy()

	if !reselect {
		return
	}
	remaining := win.viewStackPages()
	if len(remaining) == 0 {
		return
	}
	win.PeersStack.SetVisibleChildName(remaining[0].name)
}

// Destroy releases the window's pages and Go-side references after the
// window has been closed.
func (win *MainWindow) Destroy() {
	for _, page := range win.pages {
		page.Destroy()
	}
	clear(win.pages)
	for _, vp := range win.stackPages {
		vp.Unref()
	}
	clear(win.stackPages)
	win.profileModel.Unref()
	win.profileSortModel.Unref()
	gutil.UnrefFields(win)
}

func (win *MainWindow) Update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
		win.updateStatusSwitch(status.Online())

		win.updatePeers(status)

	case *tsutil.FileStatus:
		if self, ok := win.pages["self"].(*SelfPage); ok {
			self.UpdateFiles(status)
		}

	case *tsutil.ProfileStatus:
		win.updateProfiles(status)
	}
}

// updateStatusSwitch shows online on the status switch. It only calls
// SetActive if that changes anything, as SetActive stops a running
// toggle animation even when it doesn't, leaving the switch stuck
// partway.
func (win *MainWindow) updateStatusSwitch(online bool) {
	win.StatusSwitch.SetState(online)
	if win.StatusSwitch.GetActive() != online {
		win.StatusSwitch.SetActive(online)
	}
}

func (win *MainWindow) SetShowOffline(show bool) {
	if win.showOffline == show {
		return
	}
	win.showOffline = show
	win.applyLayout(win.ipn)
}

func (win *MainWindow) setPeerQuery(q string) {
	q = strings.TrimSpace(q)
	if win.peerQuery == q {
		return
	}
	win.peerQuery = q
	win.applyLayout(win.ipn)
}

// applyLayout restacks the sidebar when desired order changes, refreshes
// chrome on new ViewStackPages, then applies section headers. Search,
// hide-offline, and category membership all use this path.
func (win *MainWindow) applyLayout(status *tsutil.IPNStatus) {
	if status == nil {
		return
	}
	layout := sidebarLayout(maps.Keys(win.pages), status, win.showOffline, win.peerQuery)
	names := sidebarNames(layout)
	if !slices.Equal(win.stackPageNames(), names) {
		win.restack(names)
		win.updateStackPages(status)
	}
	win.applySections(layout)
}

func (win *MainWindow) updateStackPages(status *tsutil.IPNStatus) {
	for _, vp := range win.viewStackPages() {
		if page := win.pages[vp.name]; page != nil {
			page.Update(status)
		}
	}
}

func (win *MainWindow) updatePeers(status *tsutil.IPNStatus) {
	win.ipn = status
	if !status.Online() {
		if _, ok := win.pages["offline"]; !ok {
			win.addPage("offline", offlinePageClass.New())
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

	win.applyLayout(status)
}

func (win *MainWindow) stackPageNames() []string {
	vps := win.viewStackPages()
	names := make([]string, 0, len(vps))
	for _, vp := range vps {
		names = append(names, vp.name)
	}
	return names
}

// sidebarPage is one sidebar row. section is the header this page
// starts, or empty if it continues the previous section.
type sidebarPage struct {
	name    string
	section string
}

func sidebarNames(layout []sidebarPage) []string {
	names := make([]string, len(layout))
	for i, p := range layout {
		names[i] = p.name
	}
	return names
}

func appendGroup(layout []sidebarPage, names []string, section string) []sidebarPage {
	for i, name := range names {
		p := sidebarPage{name: name}
		if i == 0 {
			p.section = section
		}
		layout = append(layout, p)
	}
	return layout
}

func sidebarLayout(pageNames iter.Seq[string], status *tsutil.IPNStatus, showOffline bool, query string) []sidebarPage {
	var hasSelf, hasMullvad, hasOffline bool
	var peerNames []string
	for name := range pageNames {
		switch name {
		case "self":
			hasSelf = true
		case "mullvad":
			hasMullvad = true
		case "offline":
			hasOffline = true
		default:
			peerNames = append(peerNames, name)
		}
	}

	if !status.Online() {
		if hasOffline {
			return []sidebarPage{{name: "offline"}}
		}
		return nil
	}
	if query != "" {
		return searchSidebarLayout(peerNames, status, query)
	}

	var exits, others, offline []string
	for _, name := range peerNames {
		peer, ok := status.Peers[tailcfg.StableNodeID(name)]
		if !ok || !peer.Valid() || tsutil.IsShareeNode(peer) {
			continue
		}
		if peerIsExitNodeOption(peer) {
			exits = append(exits, name)
			continue
		}
		if !peerIsOnline(peer) {
			if showOffline {
				offline = append(offline, name)
			}
			continue
		}
		others = append(others, name)
	}
	sortPeerPageNames(status, exits)
	sortPeerPageNames(status, others)
	sortPeerPageNames(status, offline)

	var layout []sidebarPage
	if hasSelf {
		layout = appendGroup(layout, []string{"self"}, locale.Get("This machine"))
	}
	if hasMullvad {
		layout = appendGroup(layout, []string{"mullvad"}, locale.Get("Exit Nodes"))
		layout = appendGroup(layout, exits, "")
	} else {
		layout = appendGroup(layout, exits, locale.Get("Exit Nodes"))
	}
	layout = appendGroup(layout, others, locale.Get("Online"))
	layout = appendGroup(layout, offline, locale.Get("Offline"))
	return layout
}

func searchSidebarLayout(peerNames []string, status *tsutil.IPNStatus, query string) []sidebarPage {
	type hit struct {
		name  string
		peer  tailcfg.NodeView
		score int
	}
	tokens := strings.Fields(query)
	var hits []hit
	for _, name := range peerNames {
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
	layout := make([]sidebarPage, len(hits))
	for i, h := range hits {
		layout[i] = sidebarPage{name: h.name}
	}
	return layout
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

func (win *MainWindow) restack(order []string) {
	visible := win.PeersStack.GetVisibleChildName()
	for _, vp := range win.viewStackPages() {
		if page := win.pages[vp.name]; page != nil {
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
		if vp.name == name {
			win.PeersStack.SetVisibleChildName(name)
			return
		}
	}
	if remaining := win.viewStackPages(); len(remaining) > 0 {
		win.PeersStack.SetVisibleChildName(remaining[0].name)
	}
}

func (win *MainWindow) applySections(layout []sidebarPage) {
	section := make(map[string]string, len(layout))
	for _, p := range layout {
		section[p.name] = p.section
	}
	for _, vp := range win.viewStackPages() {
		title := section[vp.name]
		vp.page.SetStartsSection(title != "")
		vp.page.SetSectionTitle(title)
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

	profileIndex, ok := listmodels.Index[gtk.StringObject](win.profileSortModel, gtk.StringObjectGLibType(), func(obj *gtk.StringObject) bool {
		return obj.GetString() == status.Profile.Name
	})
	if ok {
		win.ProfileDropDown.SetSelected(profileIndex)
	}
}

func (win *MainWindow) Toast(msg string) {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	win.ToastOverlay.AddToast(toast) // Takes ownership.
}

// removeActionGroup removes the action group inserted into w under
// prefix, if any. puregotk calls GoPointer on interface arguments, so a
// nil group can't be passed; a wrapper around NULL can.
func removeActionGroup(w *gtk.Widget, prefix string) {
	w.InsertActionGroup(prefix, &gio.SimpleActionGroup{})
}

// setStatus asks the poller to start Tailscale if s is true and stop
// it otherwise. The status updates that follow set the switch, unless
// the request couldn't be sent, in which case it's turned back. It
// doesn't block.
func (win *MainWindow) setStatus(ctx context.Context, s bool) {
	app := win.app
	sw := win.StatusSwitch
	// Keep the switch alive until the result is back on the main
	// thread, in case the window is closed in the meantime.
	sw.Ref()

	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		sent, err := app.setRunning(ctx, s)
		if err != nil {
			slog.Error("set Tailscale status", "err", err)
			app.toast(locale.Get("Failed to start or stop Tailscale"))
		}
		gutil.IdleAdd(func() {
			if !sent {
				sw.SetActive(!s)
			}
			sw.Unref()
		})
	}()
}
