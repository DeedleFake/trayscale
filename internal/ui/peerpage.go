package ui

import (
	"context"
	_ "embed"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gobject/types"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/tsaddr"
	"tailscale.com/tailcfg"
)

//go:embed peerpage.ui
var peerPageXML string

type PeerPage struct {
	app       *App
	stackPage *adw.ViewStackPage
	peer      tailcfg.NodeView
	actions   *gio.SimpleActionGroup
	iconNames *[]string

	Page                  *adw.StatusPage
	IPList                *gtk.ListBox
	AdvertisedRoutesGroup *adw.PreferencesGroup
	AdvertisedRoutesList  *gtk.ListBox
	MiscGroup             *adw.PreferencesGroup
	ExitNodeRow           *adw.SwitchRow
	OnlineRow             *adw.ActionRow
	Online                *gtk.Image
	LastSeenRow           *adw.ActionRow
	LastSeen              *gtk.Label
	CreatedRow            *adw.ActionRow
	Created               *gtk.Label
	LastHandshakeRow      *adw.ActionRow
	LastHandshake         *gtk.Label
	RxBytesRow            *adw.ActionRow
	RxBytes               *gtk.Label
	TxBytesRow            *adw.ActionRow
	TxBytes               *gtk.Label
	DropTarget            *gtk.DropTarget

	sendFileAction *gio.SimpleAction

	addrModel  *listmodels.Model[netip.Addr]
	routeModel *listmodels.Model[netip.Prefix]
}

func NewPeerPage(a *App, status *tsutil.IPNStatus, peer tailcfg.NodeView) *PeerPage {
	var page PeerPage
	gutil.FillFromUI(&page, peerPageXML)
	page.init(a, status, peer)
	return &page
}

func (page *PeerPage) init(a *App, status *tsutil.IPNStatus, peer tailcfg.NodeView) {
	page.app = a
	page.peer = peer

	page.actions = gio.NewSimpleActionGroup()

	addPageAction(page.actions, gio.NewSimpleAction("copyFQDN", nil), func(*glib.Variant) {
		a.clip(strings.TrimSuffix(page.peer.Name(), "."))
		a.win.Toast(locale.Get("Copied FQDN to clipboard"))
	})

	sType := glib.NewVariantType("s")
	page.sendFileAction = gio.NewSimpleAction("sendFile", sType)
	sType.Free()
	page.sendFileAction.Ref() // page.sendFileAction keeps one.
	addPageAction(page.actions, page.sendFileAction, func(p *glib.Variant) {
		dialog := gtk.NewFileDialog()
		defer dialog.Unref() // The operation keeps it alive.
		dialog.SetModal(true)

		mode := p.GetString(nil)
		open, finish := dialog.OpenMultiple, dialog.OpenMultipleFinish
		if mode == "dir" {
			open, finish = dialog.SelectMultipleFolders, dialog.SelectMultipleFoldersFinish
		}

		dialog.SetTitle(locale.Get("Select %v(s) to send to %v", mode, page.peer.Hostinfo().Hostname()))

		open(&a.win.MainWindow.Window, nil, gutil.Async(func(res *gio.AsyncResultBase) {
			files, err := finish(res)
			if err = gutil.Err(err); err != nil || files == nil {
				if !gutil.ErrHasCode(err, int(gtk.DialogErrorDismissedValue)) {
					slog.Error("open files", "err", err)
				}
				return
			}
			defer gutil.Obj(files).Unref()

			for _, obj := range listmodels.Objects(files) {
				a.goPushFile(page.peer.StableID(), obj)
			}
		}), 0)
	})

	page.Page.AddController(&page.DropTarget.EventController)
	page.DropTarget.SetGtypes([]types.GType{gio.FileGLibType()}, 1)
	gutil.Connect(page.DropTarget, page.DropTarget.ConnectDrop, func(_ gtk.DropTarget, val uintptr, x, y float64) bool {
		obj := gutil.Ptr[gobject.Value](val).GetObject()
		if !gutil.IsA(obj, gio.FileGLibType()) {
			return true
		}
		a.goPushFile(page.peer.StableID(), obj)
		return true
	})

	page.addrModel = listmodels.NewModel(netip.Addr.String)
	addrs := page.addrModel.NewSortListModel(netip.Addr.Compare)
	page.addrModel.BindListBox(page.IPList, addrs, func(addr netip.Addr) *gtk.Widget {
		return a.copyRow(addr.String())
	})
	addrs.Unref() // The list box holds it.
	setPlaceholder(page.IPList, locale.Get("No addresses."))

	page.routeModel = listmodels.NewModel(netip.Prefix.String)
	routes := page.routeModel.NewSortListModel(netip.Prefix.Compare)
	page.routeModel.BindListBox(page.AdvertisedRoutesList, routes, func(route netip.Prefix) *gtk.Widget {
		return routeRow(page.routeModel, route)
	})
	routes.Unref()
	setPlaceholder(page.AdvertisedRoutesList, locale.Get("No advertised routes."))

	sw := rowSwitch(page.ExitNodeRow)
	gutil.Connect(sw, sw.ConnectStateSet, func(_ gtk.Switch, s bool) bool {
		if s == sw.GetState() {
			return false
		}

		if s {
			err := tsutil.AdvertiseExitNode(context.TODO(), false)
			if err != nil {
				slog.Error("disable exit node advertisement", "err", err)
				// Continue anyways.
			}
		}

		var node tailcfg.StableNodeID
		if s {
			node = page.peer.StableID()
		}
		err := tsutil.ExitNode(context.TODO(), node)
		if err != nil {
			slog.Error("set exit node", "err", err)
			sw.SetActive(!s)
			return true
		}
		return true
	})
}

func (page *PeerPage) Widget() *gtk.Widget {
	return &page.Page.Widget
}

func (page *PeerPage) Actions() *gio.SimpleActionGroup {
	return page.actions
}

func (page *PeerPage) Destroy() {
	page.addrModel.Unref()
	page.routeModel.Unref()
	page.sendFileAction.Unref()
	page.actions.Unref()
	gutil.UnrefFields(page)
}

func (page *PeerPage) Bind(stackPage *adw.ViewStackPage) {
	page.stackPage = stackPage
	page.updateIcon()
}

func (page *PeerPage) updateIcon() {
	if page.iconNames == nil {
		// Bind runs before the first Update.
		return
	}
	theme := gtk.IconThemeGetForDisplay(page.Page.GetDisplay())
	page.stackPage.SetIconName(gutil.FirstIconName(theme, *page.iconNames...))
}

func (page *PeerPage) Update(s tsutil.Status) bool {
	status, ok := s.(*tsutil.IPNStatus)
	if !ok {
		return true
	}
	if !status.Online() {
		return false
	}

	page.peer = status.Peers[page.peer.StableID()]
	if !page.peer.Valid() || tsutil.IsShareeNode(page.peer) {
		return false
	}

	page.sendFileAction.SetEnabled(status.FileTargets.Contains(page.peer.StableID()))

	online := page.peer.Online().Get()
	exitNodeOption := peerIsExitNodeOption(page.peer)
	exitNode := page.peer.Equal(status.ExitNode())

	var enginePeer ipnstate.PeerStatusLite
	if status.Engine != nil {
		enginePeer = status.Engine.LivePeers[page.peer.Key()]
	}

	page.stackPage.SetTitle(peerName(page.peer))
	if iconNames := peerIconNames(online, exitNodeOption, exitNode); iconNames != page.iconNames {
		page.iconNames = iconNames
		page.updateIcon()
	}
	page.stackPage.SetNeedsAttention(exitNode)

	page.Page.SetTitle(page.peer.Hostinfo().Hostname())
	page.Page.SetDescription(page.peer.Name())

	page.ExitNodeRow.SetVisible(exitNodeOption)
	setSwitch(page.ExitNodeRow, exitNode)
	page.RxBytes.SetText(strconv.FormatInt(enginePeer.RxBytes, 10))
	page.TxBytes.SetText(strconv.FormatInt(enginePeer.TxBytes, 10))
	page.Created.SetText(formatTime(page.peer.Created()))
	page.LastSeen.SetText(formatTime(page.peer.LastSeen().Get()))
	page.LastSeenRow.SetVisible(!online)
	page.LastHandshake.SetText(formatTime(enginePeer.LastHandshake))
	page.Online.SetFromGicon(boolIcon(online))

	routes := func(yield func(netip.Prefix) bool) {
		for _, r := range page.peer.PrimaryRoutes().All() {
			if r.Bits() == 0 {
				continue
			}
			if !yield(r) {
				return
			}
		}
	}

	page.addrModel.Update(xiter.Map(xiter.V2(page.peer.Addresses().All()), netip.Prefix.Addr))
	page.routeModel.Update(routes)

	return true
}

func peerName(peer tailcfg.NodeView) string {
	if !peer.Valid() {
		return ""
	}
	return peer.DisplayName(true)
}

func peerIsExitNodeOption(peer tailcfg.NodeView) bool {
	return peer.Valid() && tsaddr.ContainsExitRoutes(peer.AllowedIPs())
}

func peerIsOnline(peer tailcfg.NodeView) bool {
	return peer.Valid() && peer.Online().Get()
}

// Peer icon names in order of preference: the name Adwaita uses, then
// Breeze's if Adwaita's is missing there, then one that nearly every
// theme has.
var (
	offlineExitNodeIcons = []string{"security-low-symbolic"}
	onlineExitNodeIcons  = []string{"security-high-symbolic"}
	offlinePeerIcons     = []string{"network-offline-symbolic"}
	exitNodeOptionIcons  = []string{"network-vpn-symbolic", "channel-secure-symbolic"}
	onlinePeerIcons      = []string{"network-transmit-receive-symbolic", "network-wired-activated-symbolic", "network-server-symbolic"}
)

func peerIconNames(online, exitNodeOption, exitNode bool) *[]string {
	if exitNode {
		if !online {
			return &offlineExitNodeIcons
		}
		return &onlineExitNodeIcons
	}
	if !online {
		return &offlinePeerIcons
	}
	if exitNodeOption {
		return &exitNodeOptionIcons
	}
	return &onlinePeerIcons
}
