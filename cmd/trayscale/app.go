package main

import (
	"context"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"deedles.dev/mk"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/internal/xmaps"
	"deedles.dev/trayscale/internal/xslices"
	"fyne.io/systray"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

//go:generate go run deedles.dev/trayscale/cmd/gtkbuildergen -out ui.go mainwindow.ui peerpage.ui preferences.ui menu.ui

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	// TS is the Tailscale Client instance to use for interaction with
	// Tailscale.
	TS *tsutil.Client

	poller *tsutil.Poller
	online bool

	app      *adw.Application
	win      *MainWindow
	settings *gio.Settings

	statusPage *adw.StatusPage
	peerPages  map[key.NodePublic]*peerPage
}

func (a *App) showPreferences() {
	if a.settings == nil {
		a.toast("Settings schema not found")
		return
	}

	win := NewPreferencesWindow()
	a.settings.Bind("tray-icon", win.UseTrayIcon.Object, "active", gio.SettingsBindDefault)
	win.SetTransientFor(&a.win.Window)
	win.Show()

	a.app.AddWindow(&win.Window.Window)
}

// showAbout shows the app's about dialog.
func (a *App) showAbout() {
	dialog := adw.NewAboutWindow()
	dialog.SetDevelopers([]string{"DeedleFake"})
	dialog.SetCopyright("Copyright (c) 2023 DeedleFake")
	dialog.SetLicense(readAssetString("LICENSE"))
	dialog.SetLicenseType(gtk.LicenseCustom)
	dialog.SetApplicationIcon(appID)
	dialog.SetApplicationName("Trayscale")
	dialog.SetWebsite("https://github.com/DeedleFake/trayscale")
	dialog.SetIssueURL("https://github.com/DeedleFake/trayscale/issues")
	if v, ok := version.Get(); ok {
		dialog.SetVersion(v)
	}
	dialog.SetTransientFor(&a.win.Window)
	dialog.Show()

	a.app.AddWindow(&dialog.Window.Window)
}

func (a *App) updatePeerPage(page *peerPage, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.page.SetIconName(peerIcon(peer))
	page.page.SetTitle(peerName(status, peer, page.self))

	page.container.SetTitle(peer.HostName)
	page.container.SetDescription(peer.DNSName)

	slices.SortFunc(peer.TailscaleIPs, netip.Addr.Less)
	page.addrRows.Update(peer.TailscaleIPs)

	page.container.OptionsGroup.SetVisible(page.self)
	if page.self {
		page.container.AdvertiseExitNodeSwitch.SetState(status.Prefs.AdvertisesExitNode())
		page.container.AdvertiseExitNodeSwitch.SetActive(status.Prefs.AdvertisesExitNode())
		page.container.AllowLANAccessSwitch.SetState(status.Prefs.ExitNodeAllowLANAccess)
		page.container.AllowLANAccessSwitch.SetActive(status.Prefs.ExitNodeAllowLANAccess)
	}

	page.container.AdvertiseRouteButton.SetVisible(page.self)

	switch {
	case page.self:
		page.routes = status.Prefs.AdvertiseRoutes
	case peer.PrimaryRoutes != nil:
		page.routes = peer.PrimaryRoutes.AsSlice()
	}
	page.routes = xslices.Filter(page.routes, func(p netip.Prefix) bool { return p.Bits() != 0 })
	slices.SortFunc(page.routes, func(p1, p2 netip.Prefix) bool { return p1.Addr().Less(p2.Addr()) || p1.Bits() < p2.Bits() })
	if len(page.routes) == 0 {
		page.routes = append(page.routes, netip.Prefix{})
	}
	eroutes := make([]enum[netip.Prefix], 0, len(page.routes))
	for i, r := range page.routes {
		if !page.self {
			i = -1
		}
		eroutes = append(eroutes, enumerate(i, r))
	}
	page.routeRows.Update(eroutes)

	page.container.NetCheckGroup.SetVisible(page.self)

	page.container.MiscGroup.SetVisible(!page.self)
	page.container.ExitNodeRow.SetVisible(peer.ExitNodeOption)
	page.container.ExitNodeSwitch.SetState(peer.ExitNode)
	page.container.ExitNodeSwitch.SetActive(peer.ExitNode)
	page.container.RxBytes.SetText(strconv.FormatInt(peer.RxBytes, 10))
	page.container.TxBytes.SetText(strconv.FormatInt(peer.TxBytes, 10))
	page.container.Created.SetText(formatTime(peer.Created))
	page.container.LastSeen.SetText(formatTime(peer.LastSeen))
	page.container.LastSeenRow.SetVisible(!peer.Online)
	page.container.LastWrite.SetText(formatTime(peer.LastWrite))
	page.container.LastHandshake.SetText(formatTime(peer.LastHandshake))
	page.container.Online.SetFromIconName(boolIcon(peer.Online))
}

func (a *App) notify(status bool) {
	body := "Tailscale is not connected."
	if status {
		body = "Tailscale is connected."
	}

	icon, iconerr := gio.NewIconForString(appID)

	n := gio.NewNotification("Tailscale Status")
	n.SetBody(body)
	if iconerr == nil {
		n.SetIcon(icon)
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) toast(msg string) *adw.Toast {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	a.win.ToastOverlay.AddToast(toast)
	return toast
}

func (a *App) updatePeers(status tsutil.Status) {
	const statusPageName = "status"

	w := a.win.PeersStack

	var peerMap map[key.NodePublic]*ipnstate.PeerStatus
	var peers []key.NodePublic

	if status.Online() {
		if c := w.ChildByName(statusPageName); c != nil {
			w.Remove(c)
		}

		peerMap = status.Status.Peer

		peers = slices.Insert(status.Status.Peers(), 0, status.Status.Self.PublicKey) // Add this manually to guarantee ordering.
		peerMap[status.Status.Self.PublicKey] = status.Status.Self
	}

	oldPeers, newPeers := xslices.Partition(peers, func(peer key.NodePublic) bool {
		_, ok := a.peerPages[peer]
		return ok
	})

	for id, page := range a.peerPages {
		_, ok := peerMap[id]
		if !ok {
			w.Remove(page.container)
			delete(a.peerPages, id)
		}
	}

	for _, p := range newPeers {
		peerStatus := peerMap[p]
		peerPage := a.newPeerPage(status, peerStatus)
		peerPage.page = w.AddTitled(
			peerPage.container,
			p.String(),
			peerName(status, peerStatus, p == status.Status.Self.PublicKey),
		)
		peerPage.self = p == status.Status.Self.PublicKey
		a.updatePeerPage(peerPage, peerStatus, status)
		a.peerPages[p] = peerPage
	}

	for _, p := range oldPeers {
		page := a.peerPages[p]
		page.self = p == status.Status.Self.PublicKey
		a.updatePeerPage(page, peerMap[p], status)
	}

	if w.Pages().NItems() == 0 {
		w.AddTitled(a.statusPage, statusPageName, "Not Connected")
		return
	}
}

func (a *App) update(s tsutil.Status) {
	online := s.Online()
	if a.online != online {
		a.online = online
		a.notify(online) // TODO: Notify on startup if not connected?
		systray.SetIcon(statusIcon(online))
	}
	if a.win == nil {
		return
	}

	a.win.StatusSwitch.SetState(online)
	a.win.StatusSwitch.SetActive(online)
	a.updatePeers(s)
}

func (a *App) init(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)
	mk.Map(&a.peerPages, 0)

	var hideWindow bool
	a.app.AddMainOption("hide-window", 0, glib.OptionFlagNone, glib.OptionArgNone, "Hide window on initial start", "")
	a.app.ConnectHandleLocalOptions(func(options *glib.VariantDict) int {
		if options.Contains("hide-window") {
			hideWindow = true
		}

		return -1
	})

	a.app.ConnectStartup(func() {
		a.app.Hold()
	})

	a.app.ConnectActivate(func() {
		if hideWindow {
			hideWindow = false
			return
		}
		a.onAppActivate(ctx)
	})

	a.initSettings(ctx)
}

func (a *App) initSettings(ctx context.Context) {
	if !slices.Contains(gio.SettingsListSchemas(), appID) {
		goto init
	}

	a.settings = gio.NewSettings(appID)
	a.settings.ConnectChanged(func(key string) {
		switch key {
		case "tray-icon":
			if a.settings.Boolean("tray-icon") {
				go startSystray(func() { a.initTray(ctx) })
				return
			}
			stopSystray()
		}
	})

init:
	if (a.settings == nil) || a.settings.Boolean("tray-icon") {
		go startSystray(func() { a.initTray(ctx) })
	}
}

func (a *App) startTS(ctx context.Context) error {
	status := <-a.poller.Get()
	if status.NeedsAuth() {
		Confirmation{
			Heading: "Login Required",
			Body:    "Open a browser to authenticate with Tailscale?",
			Accept:  "_Open Browser",
			Reject:  "_Cancel",
		}.Show(a, func(accept bool) {
			if accept {
				gtk.ShowURI(&a.win.Window, status.Status.AuthURL, gdk.CURRENT_TIME)
			}
		})
		return nil
	}

	err := a.TS.Start(ctx)
	if err != nil {
		return err
	}
	a.poller.Poll() <- struct{}{}
	return nil
}

func (a *App) stopTS(ctx context.Context) error {
	err := a.TS.Stop(ctx)
	if err != nil {
		return err
	}
	a.poller.Poll() <- struct{}{}
	return nil
}

func (a *App) onAppActivate(ctx context.Context) {
	if a.win != nil {
		a.win.Present()
		return
	}

	preferencesAction := gio.NewSimpleAction("preferences", nil)
	preferencesAction.ConnectActivate(func(p *glib.Variant) { a.showPreferences() })
	a.app.AddAction(preferencesAction)

	aboutAction := gio.NewSimpleAction("about", nil)
	//aboutAction.ConnectActivate(func(p *glib.Variant) { a.showAbout() })
	aboutAction.ConnectActivate(func(p *glib.Variant) {
		fc := gtk.NewFileChooserNative("", &a.win.Window, gtk.FileChooserActionSave, "", "")
		fc.ConnectResponse(func(id int) {
			switch gtk.ResponseType(id) {
			case gtk.ResponseAccept:
				file := fc.File()
				slog := slog.With("path", file.Path())

				s, err := file.Replace(context.TODO(), "", false, gio.FileCreateNone)
				if err != nil {
					slog.Error("create file", "err", err)
					return
				}
				defer s.Close(context.TODO())

				w := NewGWriter(context.TODO(), s)
				_, err = io.Copy(w, strings.NewReader("This is a test."))
				if err != nil {
					slog.Error("write file", "err", err)
				}
			}
		})
		fc.Show()
	})
	a.app.AddAction(aboutAction)

	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(p *glib.Variant) { a.Quit() })
	a.app.AddAction(quitAction)
	a.app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

	a.statusPage = adw.NewStatusPage()
	a.statusPage.SetTitle("Not Connected")
	a.statusPage.SetIconName("network-offline-symbolic")
	a.statusPage.SetDescription("Tailscale is not connected")

	a.win = NewMainWindow(&a.app.Application)

	a.win.StatusSwitch.ConnectStateSet(func(s bool) bool {
		if s == a.win.StatusSwitch.State() {
			return false
		}

		// TODO: Handle this, and other switches, asynchrounously instead
		// of freezing the entire UI.
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		f := a.stopTS
		if s {
			f = a.startTS
		}

		err := f(ctx)
		if err != nil {
			slog.Error("set Tailscale status", "err", err)
			a.win.StatusSwitch.SetActive(!s)
			return true
		}
		return true
	})

	a.win.PeersStack.NotifyProperty("visible-child", func() {
		if a.win.PeersStack.VisibleChild() != nil {
			a.win.Leaflet.Navigate(adw.NavigationDirectionForward)
		}
	})

	a.win.BackButton.ConnectClicked(func() {
		a.win.Leaflet.Navigate(adw.NavigationDirectionBack)
	})

	a.win.ConnectCloseRequest(func() bool {
		maps.Clear(a.peerPages)
		a.win = nil
		return false
	})
	a.poller.Poll() <- struct{}{}
	a.win.Show()
}

func (a *App) initTray(ctx context.Context) {
	systray.SetIcon(statusIcon(a.online))
	systray.SetTitle("Trayscale")

	showWindow := systray.AddMenuItem("Show", "").ClickedCh
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "").ClickedCh

	for {
		select {
		case <-ctx.Done():
			return
		case <-showWindow:
			glib.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})
		case <-quit:
			a.Quit()
		}
	}
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	stopSystray()
	a.app.Quit()
}

// Run runs the app, initializing everything and then entering the
// main loop. It will return if either ctx is cancelled or Quit is
// called.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.init(ctx)

	err := a.app.Register(ctx)
	if err != nil {
		slog.Error("register application", "err", err)
		return
	}

	a.poller = &tsutil.Poller{
		TS:  a.TS,
		New: func(s tsutil.Status) { glib.IdleAdd(func() { a.update(s) }) },
	}
	go a.poller.Run(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}

type peerPage struct {
	page      *gtk.StackPage
	container *PeerPage

	self   bool
	routes []netip.Prefix

	addrRows  rowManager[netip.Addr]
	routeRows rowManager[enum[netip.Prefix]]
}

type addrRow struct {
	ip netip.Addr

	w *adw.ActionRow
	c *gtk.Button
}

func (row *addrRow) Update(ip netip.Addr) {
	row.ip = ip
	row.w.SetTitle(ip.String())
}

func (row *addrRow) Widget() gtk.Widgetter {
	return row.w
}

type routeRow struct {
	route enum[netip.Prefix]

	w *adw.ActionRow
	r *gtk.Button
}

func (row *routeRow) Update(route enum[netip.Prefix]) {
	row.route = route

	if !route.Val.IsValid() {
		row.r.SetVisible(false)
		row.w.SetTitle("No advertised routes.")
		return
	}

	row.r.SetVisible(route.Index >= 0)
	row.w.SetTitle(route.Val.String())
}

func (row *routeRow) Widget() gtk.Widgetter {
	return row.w
}

func (a *App) newPeerPage(status tsutil.Status, peer *ipnstate.PeerStatus) *peerPage {
	page := peerPage{
		container: NewPeerPage(),
	}

	page.addrRows.Parent = page.container.IPGroup
	page.addrRows.New = func(ip netip.Addr) row[netip.Addr] {
		row := addrRow{
			ip: ip,

			w: adw.NewActionRow(),
			c: gtk.NewButtonFromIconName("edit-copy-symbolic"),
		}

		row.c.SetMarginTop(12) // Why is this necessary?
		row.c.SetMarginBottom(12)
		row.c.SetHasFrame(false)
		row.c.SetTooltipText("Copy to Clipboard")
		row.c.ConnectClicked(func() {
			row.c.Clipboard().Set(glib.NewValue(row.ip.String()))
			a.toast("Copied to clipboard")
		})

		row.w.SetObjectProperty("title-selectable", true)
		row.w.AddSuffix(row.c)
		row.w.SetActivatableWidget(row.c)
		row.w.SetTitle(ip.String())

		return &row
	}

	page.routeRows.Parent = page.container.AdvertisedRoutesGroup
	page.routeRows.New = func(route enum[netip.Prefix]) row[enum[netip.Prefix]] {
		row := routeRow{
			route: route,

			w: adw.NewActionRow(),
			r: gtk.NewButtonFromIconName("list-remove-symbolic"),
		}

		row.w.SetObjectProperty("title-selectable", true)
		row.w.AddSuffix(row.r)
		row.w.SetTitle(route.Val.String())

		row.r.SetMarginTop(12)
		row.r.SetMarginBottom(12)
		row.r.SetHasFrame(false)
		row.r.SetTooltipText("Remove")
		row.r.ConnectClicked(func() {
			routes := slices.Delete(page.routes, row.route.Index, row.route.Index+1)
			err := a.TS.AdvertiseRoutes(context.TODO(), routes)
			if err != nil {
				slog.Error("advertise routes", "err", err)
				return
			}
			a.poller.Poll() <- struct{}{}
		})

		return &row
	}

	page.container.ExitNodeSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.container.ExitNodeSwitch.State() {
			return false
		}

		if s {
			err := a.TS.AdvertiseExitNode(context.TODO(), false)
			if err != nil {
				slog.Error("disable exit node advertisement", "err", err)
				// Continue anyways.
			}
		}

		var node *ipnstate.PeerStatus
		if s {
			node = peer
		}
		err := a.TS.ExitNode(context.TODO(), node)
		if err != nil {
			slog.Error("set exit node", "err", err)
			page.container.ExitNodeSwitch.SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.container.AdvertiseExitNodeSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.container.AdvertiseExitNodeSwitch.State() {
			return false
		}

		if s {
			err := a.TS.ExitNode(context.TODO(), nil)
			if err != nil {
				slog.Error("disable existing exit node", "err", err)
				// Continue anyways.
			}
		}

		err := a.TS.AdvertiseExitNode(context.TODO(), s)
		if err != nil {
			slog.Error("advertise exit node", "err", err)
			page.container.AdvertiseExitNodeSwitch.SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.container.AllowLANAccessSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.container.AllowLANAccessSwitch.State() {
			return false
		}

		err := a.TS.AllowLANAccess(context.TODO(), s)
		if err != nil {
			slog.Error("allow LAN access", "err", err)
			page.container.AllowLANAccessSwitch.SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.container.AdvertiseRouteButton.ConnectClicked(func() {
		Prompt{"Add IP", "IP prefix to advertise"}.Show(a, func(val string) {
			p, err := netip.ParsePrefix(val)
			if err != nil {
				slog.Error("parse prefix", "err", err)
				return
			}

			prefs, err := a.TS.Prefs(context.TODO())
			if err != nil {
				slog.Error("get prefs", "err", err)
				return
			}

			err = a.TS.AdvertiseRoutes(
				context.TODO(),
				append(prefs.AdvertiseRoutes, p),
			)
			if err != nil {
				slog.Error("advertise routes", "err", err)
				return
			}

			a.poller.Poll() <- struct{}{}
		})
	})

	type latencyEntry = xmaps.Entry[string, time.Duration]
	latencyRows := rowManager[latencyEntry]{
		Parent: rowAdderParent{page.container.DERPLatencies},
		New: func(lat latencyEntry) row[latencyEntry] {
			label := gtk.NewLabel(lat.Val.String())

			row := adw.NewActionRow()
			row.SetTitle(lat.Key)
			row.AddSuffix(label)

			return &simpleRow[latencyEntry]{
				W: row,
				U: func(lat latencyEntry) {
					label.SetText(lat.Val.String())
					row.SetTitle(lat.Key)
				},
			}
		},
	}

	page.container.NetCheckButton.ConnectClicked(func() {
		r, dm, err := a.TS.NetCheck(context.TODO(), true)
		if err != nil {
			slog.Error("netcheck", "err", err)
			return
		}

		page.container.LastNetCheck.SetText(formatTime(time.Now()))
		page.container.UDPRow.SetVisible(true)
		page.container.UDP.SetFromIconName(boolIcon(r.UDP))
		page.container.IPv4Row.SetVisible(true)
		page.container.IPv4Icon.SetVisible(!r.IPv4)
		page.container.IPv4Icon.SetFromIconName(boolIcon(r.IPv4))
		page.container.IPv4Addr.SetVisible(r.IPv4)
		page.container.IPv4Addr.SetText(r.GlobalV4)
		page.container.IPv6Row.SetVisible(true)
		page.container.IPv6Icon.SetVisible(!r.IPv6)
		page.container.IPv6Icon.SetFromIconName(boolIcon(r.IPv6))
		page.container.IPv6Addr.SetVisible(r.IPv6)
		page.container.IPv6Addr.SetText(r.GlobalV6)
		page.container.UPnPRow.SetVisible(true)
		page.container.UPnP.SetFromIconName(optBoolIcon(r.UPnP))
		page.container.PMPRow.SetVisible(true)
		page.container.PMP.SetFromIconName(optBoolIcon(r.PMP))
		page.container.PCPRow.SetVisible(true)
		page.container.PCP.SetFromIconName(optBoolIcon(r.PCP))
		page.container.HairPinningRow.SetVisible(true)
		page.container.HairPinning.SetFromIconName(optBoolIcon(r.HairPinning))
		page.container.PreferredDERPRow.SetVisible(true)
		page.container.PreferredDERP.SetText(dm.Regions[r.PreferredDERP].RegionName)

		page.container.DERPLatencies.SetVisible(true)
		latencies := xmaps.Entries(r.RegionLatency)
		slices.SortFunc(latencies, func(e1, e2 xmaps.Entry[int, time.Duration]) bool { return e1.Val < e2.Val })
		namedLats := make([]xmaps.Entry[string, time.Duration], 0, len(latencies))
		for _, lat := range latencies {
			namedLats = append(namedLats, xmaps.Entry[string, time.Duration]{
				Key: dm.Regions[lat.Key].RegionName,
				Val: lat.Val,
			})
		}
		latencyRows.Update(namedLats)
	})

	return &page
}
