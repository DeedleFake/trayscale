package ui

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/tsaddr"
	"tailscale.com/tailcfg"
)

//go:embed peerpage.ui
var peerPageXML string

type PeerPage struct {
	app     *App
	row     *PageRow
	peer    tailcfg.NodeView
	actions *gio.SimpleActionGroup

	Page                  *adw.StatusPage
	IPList                *gtk.ListBox
	AdvertisedRoutesGroup *adw.PreferencesGroup
	AdvertisedRoutesList  *gtk.ListBox
	UDPRow                *adw.ActionRow
	UDP                   *gtk.Image
	IPv4Row               *adw.ActionRow
	IPv4Icon              *gtk.Image
	IPv4Addr              *gtk.Label
	IPv6Row               *adw.ActionRow
	IPv6Icon              *gtk.Image
	IPv6Addr              *gtk.Label
	UPnPRow               *adw.ActionRow
	UPnP                  *gtk.Image
	PMPRow                *adw.ActionRow
	PMP                   *gtk.Image
	PCPRow                *adw.ActionRow
	PCP                   *gtk.Image
	HairPinningRow        *adw.ActionRow
	HairPinning           *gtk.Image
	PreferredDERPRow      *adw.ActionRow
	PreferredDERP         *gtk.Label
	DERPLatencies         *adw.ExpanderRow
	MiscGroup             *adw.PreferencesGroup
	ExitNodeRow           *adw.SwitchRow
	OnlineRow             *adw.ActionRow
	Online                *gtk.Image
	LastSeenRow           *adw.ActionRow
	LastSeen              *gtk.Label
	CreatedRow            *adw.ActionRow
	Created               *gtk.Label
	LastWriteRow          *adw.ActionRow
	LastWrite             *gtk.Label
	LastHandshakeRow      *adw.ActionRow
	LastHandshake         *gtk.Label
	RxBytesRow            *adw.ActionRow
	RxBytes               *gtk.Label
	TxBytesRow            *adw.ActionRow
	TxBytes               *gtk.Label
	SendFileBurron        *adw.ButtonRow
	SendDirButton         *adw.ButtonRow
	DropTarget            *gtk.DropTarget

	addrModel  *gioutil.ListModel[netip.Prefix]
	routeModel *gioutil.ListModel[netip.Prefix]
}

func NewPeerPage(a *App, status *tsutil.IPNStatus, peer tailcfg.NodeView) *PeerPage {
	var page PeerPage
	fillFromBuilder(&page, peerPageXML)
	page.init(a, status, peer)
	return &page
}

func (page *PeerPage) init(a *App, status *tsutil.IPNStatus, peer tailcfg.NodeView) {
	page.app = a
	page.peer = peer

	page.actions = gio.NewSimpleActionGroup()

	copyFQDNAction := gio.NewSimpleAction("copyFQDN", nil)
	copyFQDNAction.ConnectActivate(func(p *glib.Variant) {
		a.clip(glib.NewValue(strings.TrimSuffix(page.peer.Name(), ".")))
		a.win.Toast("Copied FQDN to clipboard")
	})
	page.actions.AddAction(copyFQDNAction)

	sendFileAction := gio.NewSimpleAction("sendFile", glib.NewVariantType("s"))
	sendFileAction.ConnectActivate(func(p *glib.Variant) {
		dialog := gtk.NewFileDialog()
		dialog.SetModal(true)

		mode := p.String()
		open, finish := dialog.OpenMultiple, dialog.OpenMultipleFinish
		if mode == "dir" {
			open, finish = dialog.SelectMultipleFolders, dialog.SelectMultipleFoldersFinish
		}

		dialog.SetTitle(fmt.Sprintf("Select %v(s) to send to %v", mode, page.peer.Hostinfo().Hostname()))

		open(context.TODO(), &a.win.MainWindow.Window, func(res gio.AsyncResulter) {
			files, err := finish(res)
			if err != nil {
				if !errHasCode(err, int(gtk.DialogErrorDismissed)) {
					slog.Error("open files", "err", err)
				}
				return
			}

			for _, file := range listmodels.Values[gio.Filer](files) {
				go a.pushFile(context.TODO(), page.peer.StableID(), file)
			}
		})
	})
	page.actions.AddAction(sendFileAction)

	page.Page.AddController(page.DropTarget)
	page.DropTarget.SetGTypes([]glib.Type{gio.GTypeFile})
	page.DropTarget.ConnectDrop(func(val *glib.Value, x, y float64) bool {
		file, ok := val.Object().Cast().(gio.Filer)
		if !ok {
			return true
		}
		go a.pushFile(context.TODO(), page.peer.StableID(), file)
		return true
	})

	page.addrModel = gioutil.NewListModel[netip.Prefix]()
	listmodels.BindListBox(
		page.IPList,
		gtk.NewSortListModel(page.addrModel, &prefixSorter.Sorter),
		func(addr netip.Prefix) gtk.Widgetter {
			copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")

			copyButton.SetMarginTop(12) // Why is this necessary?
			copyButton.SetMarginBottom(12)
			copyButton.SetHasFrame(false)
			copyButton.SetTooltipText("Copy to Clipboard")
			copyButton.ConnectClicked(func() {
				a.clip(glib.NewValue(addr.String()))
				a.win.Toast("Copied to clipboard")
			})

			row := adw.NewActionRow()
			row.SetObjectProperty("title-selectable", true)
			row.AddSuffix(copyButton)
			row.SetActivatableWidget(copyButton)
			row.SetTitle(addr.String())

			return row
		},
	)

	ipListPlaceholder := adw.NewActionRow()
	ipListPlaceholder.SetTitle("No addresses.")
	page.IPList.SetPlaceholder(ipListPlaceholder)

	page.routeModel = gioutil.NewListModel[netip.Prefix]()
	listmodels.BindListBox(
		page.AdvertisedRoutesList,
		gtk.NewSortListModel(page.routeModel, &prefixSorter.Sorter),
		func(route netip.Prefix) gtk.Widgetter {
			removeButton := gtk.NewButtonFromIconName("list-remove-symbolic")

			removeButton.SetMarginTop(12)
			removeButton.SetMarginBottom(12)
			removeButton.SetHasFrame(false)
			removeButton.SetTooltipText("Remove")
			removeButton.ConnectClicked(func() {
				routes := slices.Collect(xiter.Filter(page.routeModel.All(), func(p netip.Prefix) bool {
					return xnetip.ComparePrefixes(p, route) != 0
				}))
				err := tsutil.AdvertiseRoutes(context.TODO(), routes)
				if err != nil {
					slog.Error("advertise routes", "err", err)
					return
				}
				<-a.poller.Poll()
			})

			row := adw.NewActionRow()
			row.SetObjectProperty("title-selectable", true)
			row.AddSuffix(removeButton)
			row.SetTitle(route.String())

			return row
		},
	)

	advertisedRoutesListPlaceholder := adw.NewActionRow()
	advertisedRoutesListPlaceholder.SetTitle("No advertised routes.")
	page.AdvertisedRoutesList.SetPlaceholder(advertisedRoutesListPlaceholder)

	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).ConnectStateSet(func(s bool) bool {
		if s == page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).State() {
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
			page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(!s)
			return true
		}
		<-a.poller.Poll()
		return true
	})
}

func (page *PeerPage) Widget() gtk.Widgetter {
	return page.Page
}

func (page *PeerPage) Actions() gio.ActionGrouper {
	return page.actions
}

func (page *PeerPage) Init(row *PageRow) {
	page.row = row
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
	if !page.peer.Valid() {
		return false
	}

	online := page.peer.Online().Get()
	exitNodeOption := tsaddr.ContainsExitRoutes(page.peer.AllowedIPs())
	exitNode := page.peer.Equal(status.ExitNode())

	var enginePeer ipnstate.PeerStatusLite
	if status.Engine != nil {
		enginePeer = status.Engine.LivePeers[page.peer.Key()]
	}

	page.row.SetTitle(peerName(page.peer))
	page.row.SetSubtitle(peerSubtitle(exitNodeOption, exitNode))
	page.row.SetIconName(peerIcon(online, exitNodeOption, exitNode))

	page.Page.SetTitle(page.peer.Hostinfo().Hostname())
	page.Page.SetDescription(page.peer.Name())

	page.ExitNodeRow.SetVisible(exitNodeOption)
	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetState(exitNode)
	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(exitNode)
	page.RxBytes.SetText(strconv.FormatInt(enginePeer.RxBytes, 10))
	page.TxBytes.SetText(strconv.FormatInt(enginePeer.TxBytes, 10))
	page.Created.SetText(formatTime(page.peer.Created()))
	page.LastSeen.SetText(formatTime(page.peer.LastSeen().Get()))
	page.LastSeenRow.SetVisible(!online)
	//page.LastWrite.SetText(formatTime(page.peer.LastWrite))
	page.LastHandshake.SetText(formatTime(enginePeer.LastHandshake))
	page.Online.SetFromIconName(boolIcon(online))

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

	listmodels.Update(page.addrModel, xiter.V2(page.peer.Addresses().All()))
	listmodels.Update(page.routeModel, routes)

	return true
}

func peerName(peer tailcfg.NodeView) string {
	return peer.DisplayName(true)
}

func peerSubtitle(exitNodeOption, exitNode bool) string {
	if exitNode {
		return "Current exit node"
	}
	if exitNodeOption {
		return "Exit node option"
	}
	return ""
}

func peerIcon(online bool, exitNodeOption, exitNode bool) string {
	if exitNode {
		if !online {
			return "network-vpn-acquiring-symbolic"
		}
		return "network-vpn-symbolic"
	}
	if !online {
		return "network-wired-offline-symbolic"
	}
	if exitNodeOption {
		return "folder-remote-symbolic"
	}

	return "network-wired-symbolic"
}
