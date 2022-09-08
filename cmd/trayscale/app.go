package main

import (
	"context"
	_ "embed"
	"log"
	"net/netip"
	"os"
	"strconv"
	"time"

	"deedles.dev/trayscale/internal/version"
	"deedles.dev/trayscale/internal/xslices"
	"deedles.dev/trayscale/tailscale"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

var (
	//go:embed trayscale.ui
	uiXML string

	//go:embed page.ui
	pageXML string

	//go:embed menu.ui
	menuXML string
)

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	// TS is the Tailscale Client instance to use for interaction with
	// Tailscale.
	TS *tailscale.Client

	poll   chan struct{}
	online bool

	app     *adw.Application
	toaster *adw.ToastOverlay
	win     *adw.ApplicationWindow

	statusSwitch *gtk.Switch
	statusPage   *adw.StatusPage

	peersStack *gtk.Stack
	peerPages  map[key.NodePublic]*peerPage
}

// pollStatus runs a loop that continues until ctx is cancelled. The
// loop polls Tailscale at regular intervals to determine the
// network's status, updating the App's state with the result.
func (a *App) pollStatus(ctx context.Context) {
	const ticklen = 5 * time.Second
	check := time.NewTicker(ticklen)

	for {
		status, err := a.TS.Status(ctx)
		if err != nil {
			log.Printf("Error: Tailscale status: %v", err)
			continue
		}
		glib.IdleAdd(func() { a.update(status) })

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
	dialog.SetLogoIconName("com.tailscale-tailscale")
	dialog.SetProgramName("Trayscale")
	if v, ok := version.Get(); ok {
		dialog.SetVersion(v)
	}
	dialog.SetTransientFor(&a.win.Window)
	dialog.SetModal(true)
	dialog.Show()

	a.app.AddWindow(&dialog.Window)
}

func (a *App) updatePeerPage(page *peerPage, peer *ipnstate.PeerStatus, self bool) {
	page.page.SetIconName(peerIcon(peer))
	page.page.SetTitle(peerName(peer, self))

	page.container.SetTitle(peer.HostName)

	for _, row := range page.addrRows {
		page.addrs.Remove(row)
	}
	page.addrRows = page.addrRows[:0]

	slices.SortFunc(peer.TailscaleIPs, netip.Addr.Less)
	for _, ip := range peer.TailscaleIPs {
		ipstr := ip.String()

		copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
		copyButton.SetMarginTop(12) // Why is this necessary?
		copyButton.SetMarginBottom(12)
		copyButton.SetTooltipText("Copy to Clipboard")
		copyButton.ConnectClicked(func() {
			copyButton.Clipboard().Set(glib.NewValue(ipstr))

			t := adw.NewToast("Copied to clipboard")
			t.SetTimeout(3)
			a.toaster.AddToast(t)
		})

		iprow := adw.NewActionRow()
		iprow.SetTitle(ipstr)
		iprow.SetObjectProperty("title-selectable", true)
		iprow.AddSuffix(copyButton)

		page.addrs.Add(iprow)
		page.addrRows = append(page.addrRows, iprow)
	}

	page.miscGroup.SetVisible(!self)
	page.exitNodeRow.SetVisible(peer.ExitNodeOption)
	page.exitNodeSwitch.SetState(peer.ExitNode)
	page.rxBytes.SetText(strconv.FormatInt(peer.RxBytes, 10))
	page.txBytes.SetText(strconv.FormatInt(peer.TxBytes, 10))
	page.created.SetText(formatTime(peer.Created))
	page.lastSeen.SetText(formatTime(peer.LastSeen))
	page.lastSeenRow.SetVisible(!peer.Online)
	page.lastWrite.SetText(formatTime(peer.LastWrite))
	page.lastHandshake.SetText(formatTime(peer.LastHandshake))

	var onlineIcon string
	if peer.Online {
		onlineIcon = "emblem-ok-symbolic"
	}
	page.online.SetFromIconName(onlineIcon)
}

func (a *App) notify(status bool) {
	body := "Tailscale is not connected."
	if status {
		body = "Tailscale is connected."
	}

	icon, iconerr := gio.NewIconForString("com.tailscale-tailscale")

	n := gio.NewNotification("Tailscale Status")
	n.SetBody(body)
	if iconerr == nil {
		n.SetIcon(icon)
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) updatePeers(status *ipnstate.Status) {
	const statusPageName = "status"

	w := a.peersStack

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
		a.updatePeerPage(pw, ps, p == status.Self.PublicKey)
		a.peerPages[p] = pw
	}

	for _, p := range u {
		page := a.peerPages[p]
		a.updatePeerPage(page, peerMap[p], p == status.Self.PublicKey)
	}

	if w.Pages().NItems() == 0 {
		w.AddTitled(a.statusPage, statusPageName, "Not Connected")
		return
	}
}

func (a *App) update(status *ipnstate.Status) {
	online := status != nil
	if a.online != online {
		a.online = online
		a.notify(online) // TODO: Notify on startup if not connected?
	}
	if a.win == nil {
		return
	}

	a.statusSwitch.SetState(online)
	a.updatePeers(status)
}

// init initializes the App, loading the builder XML, creating a
// window, and so on.
func (a *App) init(ctx context.Context) {
	a.app = adw.NewApplication(appID, 0)
	makeMap(&a.peerPages, 0)

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

		builder := gtk.NewBuilder()
		builder.AddFromString(uiXML, len(uiXML))
		builder.AddFromString(menuXML, len(menuXML))

		// Workaround for Cambalache limitations.
		withWidget(builder, "MainMenuButton", func(w *gtk.MenuButton) {
			w.SetMenuModel(builder.GetObject("MainMenu").Cast().(gio.MenuModeller))
		})

		withWidget(builder, "StatusSwitch", func(w *gtk.Switch) {
			a.statusSwitch = w
			w.ConnectStateSet(func(s bool) bool {
				if s == w.State() {
					return false
				}

				f := a.TS.Stop
				if s {
					f = a.TS.Start
				}

				err := f(ctx)
				if err != nil {
					log.Printf("Error: set Tailscale status: %v", err)
					w.SetActive(!s)
					return true
				}
				a.poll <- struct{}{}
				return true
			})
		})

		getObject(&a.peersStack, builder, "PeersStack")

		getObject(&a.toaster, builder, "ToastOverlay")

		getObject(&a.win, builder, "MainWindow")
		a.app.AddWindow(&a.win.Window)
		a.win.ConnectCloseRequest(func() bool {
			maps.Clear(a.peerPages)
			a.win = nil
			return false
		})
		a.poll <- struct{}{}
		a.win.Show()
	})
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	a.app.Quit()
}

// Run runs the app, initializing everything and then entering the
// main loop. It will return if either ctx is cancelled or Quit is
// called.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.init(ctx)

	makeChan(&a.poll, 1)
	go a.pollStatus(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}

type peerPage struct {
	page *gtk.StackPage

	container *adw.StatusPage

	addrs    *adw.PreferencesGroup
	addrRows []*adw.ActionRow

	miscGroup        *adw.PreferencesGroup
	exitNodeRow      *adw.ActionRow
	exitNodeSwitch   *gtk.Switch
	rxBytesRow       *adw.ActionRow
	rxBytes          *gtk.Label
	txBytesRow       *adw.ActionRow
	txBytes          *gtk.Label
	createdRow       *adw.ActionRow
	created          *gtk.Label
	lastWriteRow     *adw.ActionRow
	lastWrite        *gtk.Label
	lastSeenRow      *adw.ActionRow
	lastSeen         *gtk.Label
	lastHandshakeRow *adw.ActionRow
	lastHandshake    *gtk.Label
	onlineRow        *adw.ActionRow
	online           *gtk.Image
}

func (a *App) newPeerPage(peer *ipnstate.PeerStatus) *peerPage {
	builder := gtk.NewBuilderFromString(pageXML, len(pageXML))

	var page peerPage
	getObject(&page.container, builder, "Container")
	getObject(&page.addrs, builder, "IPGroup")
	getObject(&page.miscGroup, builder, "MiscGroup")
	getObject(&page.exitNodeRow, builder, "ExitNodeRow")
	getObject(&page.exitNodeSwitch, builder, "ExitNodeSwitch")
	getObject(&page.rxBytesRow, builder, "RxBytesRow")
	getObject(&page.rxBytes, builder, "RxBytes")
	getObject(&page.txBytesRow, builder, "TxBytesRow")
	getObject(&page.txBytes, builder, "TxBytes")
	getObject(&page.createdRow, builder, "CreatedRow")
	getObject(&page.created, builder, "Created")
	getObject(&page.lastWriteRow, builder, "LastWriteRow")
	getObject(&page.lastWrite, builder, "LastWrite")
	getObject(&page.lastSeenRow, builder, "LastSeenRow")
	getObject(&page.lastSeen, builder, "LastSeen")
	getObject(&page.lastHandshakeRow, builder, "LastHandshakeRow")
	getObject(&page.lastHandshake, builder, "LastHandshake")
	getObject(&page.onlineRow, builder, "OnlineRow")
	getObject(&page.online, builder, "Online")

	page.exitNodeSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.exitNodeSwitch.State() {
			return false
		}

		var node *ipnstate.PeerStatus
		if s {
			node = peer
		}
		err := a.TS.ExitNode(context.TODO(), node)
		if err != nil {
			log.Printf("Error: set exit node: %v", err)
			page.exitNodeSwitch.SetActive(!s)
			return true
		}
		a.poll <- struct{}{}
		return true
	})

	return &page
}
