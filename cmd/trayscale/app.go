package main

import (
	"context"
	"net/netip"
	"os"
	"strconv"
	"time"

	"deedles.dev/mk"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/internal/xmaps"
	"deedles.dev/trayscale/internal/xslices"
	"fyne.io/systray"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

//go:generate go run deedles.dev/trayscale/cmd/gtkbuildergen -out ui.go mainwindow.ui peerpage.ui menu.ui

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	// TS is the Tailscale Client instance to use for interaction with
	// Tailscale.
	TS *tsutil.Client

	poll   chan struct{}
	online bool

	app *adw.Application
	win *MainWindow

	statusPage *adw.StatusPage
	peerPages  map[key.NodePublic]*peerPage
}

// poller runs a loop that continues until ctx is cancelled. The loop
// polls Tailscale at regular intervals to determine the network's
// status, updating the App's state with the result.
func (a *App) poller(ctx context.Context) {
	const ticklen = 5 * time.Second
	check := time.NewTicker(ticklen)

	for {
		status, err := a.TS.Status(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale status", err)
			continue
		}

		prefs, err := a.TS.Prefs(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("get Tailscale prefs", err)
			continue
		}

		glib.IdleAdd(func() { a.update(status, prefs) })

		select {
		case <-ctx.Done():
			return
		case <-check.C:
		case <-a.poll:
			check.Reset(ticklen)
		}
	}
}

// showAboutDialog shows the app's about dialog.
func (a *App) showAboutDialog() {
	dialog := gtk.NewAboutDialog()
	dialog.SetAuthors([]string{"DeedleFake"})
	dialog.SetComments("A simple, unofficial GUI wrapper for the Tailscale CLI client.")
	dialog.SetCopyright("Copyright (c) 2022 DeedleFake")
	dialog.SetLicense(readAssetString("LICENSE"))
	dialog.SetLogoIconName("dev.deedles.Trayscale")
	dialog.SetProgramName("Trayscale")
	if v, ok := version.Get(); ok {
		dialog.SetVersion(v)
	}
	dialog.SetTransientFor(&a.win.Window)
	dialog.SetModal(true)
	dialog.Show()

	a.app.AddWindow(&dialog.Window)
}

func (a *App) updatePeerPage(page *peerPage, peer *ipnstate.PeerStatus, prefs *ipn.Prefs) {
	page.page.SetIconName(peerIcon(peer))
	page.page.SetTitle(peerName(peer, page.self))

	page.container.SetTitle(peer.HostName)
	page.container.SetDescription(peer.DNSName)

	slices.SortFunc(peer.TailscaleIPs, netip.Addr.Less)
	page.addrRows.Update(peer.TailscaleIPs)

	page.container.OptionsGroup.SetVisible(page.self)
	if page.self {
		page.container.AdvertiseExitNodeSwitch.SetState(prefs.AdvertisesExitNode())
		page.container.AllowLANAccessSwitch.SetState(prefs.ExitNodeAllowLANAccess)
	}

	page.container.AdvertiseRouteButton.SetVisible(page.self)

	switch {
	case page.self:
		page.routes = prefs.AdvertiseRoutes
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

	icon, iconerr := gio.NewIconForString("dev.deedles.Trayscale")

	n := gio.NewNotification("Tailscale Status")
	n.SetBody(body)
	if iconerr == nil {
		n.SetIcon(icon)
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) updatePeers(status *ipnstate.Status, prefs *ipn.Prefs) {
	const statusPageName = "status"

	w := a.win.PeersStack

	var peerMap map[key.NodePublic]*ipnstate.PeerStatus
	var peers []key.NodePublic

	if status != nil {
		if c := w.ChildByName(statusPageName); c != nil {
			w.Remove(c)
		}

		peerMap = status.Peer

		peers = slices.Insert(status.Peers(), 0, status.Self.PublicKey) // Add this manually to guarantee ordering.
		peerMap[status.Self.PublicKey] = status.Self
	}

	u, n := xslices.Partition(peers, func(peer key.NodePublic) bool {
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

	for _, p := range n {
		ps := peerMap[p]
		pw := a.newPeerPage(ps)
		pw.page = w.AddTitled(pw.container, p.String(), peerName(ps, p == status.Self.PublicKey))
		pw.self = p == status.Self.PublicKey
		a.updatePeerPage(pw, ps, prefs)
		a.peerPages[p] = pw
	}

	for _, p := range u {
		page := a.peerPages[p]
		page.self = p == status.Self.PublicKey
		a.updatePeerPage(page, peerMap[p], prefs)
	}

	if w.Pages().NItems() == 0 {
		w.AddTitled(a.statusPage, statusPageName, "Not Connected")
		return
	}
}

func (a *App) update(status *ipnstate.Status, prefs *ipn.Prefs) {
	online := status != nil
	if a.online != online {
		a.online = online
		a.notify(online) // TODO: Notify on startup if not connected?
		systray.SetIcon(statusIcon(online))
	}
	if a.win == nil {
		return
	}

	a.win.StatusSwitch.SetState(online)
	a.updatePeers(status, prefs)
}

func (a *App) init(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)
	mk.Map(&a.peerPages, 0)

	a.app.ConnectStartup(func() {
		a.app.Hold()
	})

	a.app.ConnectActivate(func() {
		if a.win != nil {
			a.win.Present()
			return
		}

		aboutAction := gio.NewSimpleAction("about", nil)
		aboutAction.ConnectActivate(func(p *glib.Variant) { a.showAboutDialog() })
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

			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			f := a.TS.Stop
			if s {
				f = a.TS.Start
			}

			err := f(ctx)
			if err != nil {
				slog.Error("set Tailscale status", err)
				a.win.StatusSwitch.SetActive(!s)
				return true
			}
			a.poll <- struct{}{}
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
		a.poll <- struct{}{}
		a.win.Show()
	})

	go systray.Run(func() {
		systray.SetIcon(statusIconInactive)
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
	}, nil)
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	systray.Quit()
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
		slog.Error("register application", err)
		return
	}

	mk.Chan(&a.poll, 1)
	go a.poller(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}

func (a *App) prompt(prompt string, res func(val string)) {
	input := gtk.NewText()
	input.SetPlaceholderText(prompt)
	input.SetSizeRequest(200, 30)

	dialog := gtk.NewMessageDialog(
		&a.win.Window,
		gtk.DialogModal|gtk.DialogDestroyWithParent|gtk.DialogUseHeaderBar,
		gtk.MessageQuestion,
		gtk.ButtonsNone,
	)
	dialog.ContentArea().Append(input)
	dialog.AddButton("Cancel", 1)
	dialog.AddButton("Add", 0)

	input.ConnectActivate(func() {
		dialog.Response(0)
	})

	dialog.ConnectResponse(func(id int) {
		defer dialog.Close()

		switch id {
		case 0:
			res(input.Buffer().Text())
		}
	})

	dialog.Show()
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

func (a *App) newPeerPage(peer *ipnstate.PeerStatus) *peerPage {
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

			t := adw.NewToast("Copied to clipboard")
			t.SetTimeout(3)
			a.win.ToastOverlay.AddToast(t)
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
				slog.Error("advertise routes", err)
				return
			}
			a.poll <- struct{}{}
		})

		return &row
	}

	page.container.ExitNodeSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.container.ExitNodeSwitch.State() {
			return false
		}

		var node *ipnstate.PeerStatus
		if s {
			node = peer
		}
		err := a.TS.ExitNode(context.TODO(), node)
		if err != nil {
			slog.Error("set exit node", err)
			page.container.ExitNodeSwitch.SetActive(!s)
			return true
		}
		a.poll <- struct{}{}
		return true
	})

	page.container.AdvertiseExitNodeSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.container.AdvertiseExitNodeSwitch.State() {
			return false
		}

		err := a.TS.AdvertiseExitNode(context.TODO(), s)
		if err != nil {
			slog.Error("advertise exit node", err)
			page.container.AdvertiseExitNodeSwitch.SetActive(!s)
			return true
		}
		a.poll <- struct{}{}
		return true
	})

	page.container.AllowLANAccessSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.container.AllowLANAccessSwitch.State() {
			return false
		}

		err := a.TS.AllowLANAccess(context.TODO(), s)
		if err != nil {
			slog.Error("allow LAN access", err)
			page.container.AllowLANAccessSwitch.SetActive(!s)
			return true
		}
		a.poll <- struct{}{}
		return true
	})

	page.container.AdvertiseRouteButton.ConnectClicked(func() {
		a.prompt("IP prefix to advertise", func(val string) {
			p, err := netip.ParsePrefix(val)
			if err != nil {
				slog.Error("parse prefix", err)
				return
			}

			prefs, err := a.TS.Prefs(context.TODO())
			if err != nil {
				slog.Error("get prefs", err)
				return
			}

			err = a.TS.AdvertiseRoutes(
				context.TODO(),
				append(prefs.AdvertiseRoutes, p),
			)
			if err != nil {
				slog.Error("advertise routes", err)
				return
			}

			a.poll <- struct{}{}
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
			slog.Error("netcheck", err)
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
