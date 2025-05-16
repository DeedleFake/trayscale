package ui

import (
	"context"
	_ "embed"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

//go:embed peerpage.ui
var peerPageXML string

var PeerPageClass = coreglib.RegisterSubclass[*PeerPage]()

type PeerPage struct {
	gtk.Widget `gtk:"-"`

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

	peer *ipnstate.PeerStatus

	addrModel  *gioutil.ListModel[netip.Addr]
	routeModel *gioutil.ListModel[netip.Prefix]
}

func NewPeerPage(a *App, status tsutil.Status, peer *ipnstate.PeerStatus) *PeerPage {
	page := PeerPageClass.New()
	fillFromBuilder(page, peerPageXML)
	page.Page.SetParent(page)
	page.init(a, status, peer)
	return page
}

func (page *PeerPage) init(a *App, status tsutil.Status, peer *ipnstate.PeerStatus) {
	page.peer = peer

	actions := gio.NewSimpleActionGroup()
	page.InsertActionGroup("peer", actions)

	sendFileAction := gio.NewSimpleAction("sendfile", glib.NewVariantType("s"))
	sendFileAction.ConnectActivate(func(p *glib.Variant) {
		dialog := gtk.NewFileDialog()
		dialog.SetModal(true)

		open, finish := dialog.OpenMultiple, dialog.OpenMultipleFinish
		if p.String() == "dir" {
			open, finish = dialog.SelectMultipleFolders, dialog.SelectMultipleFoldersFinish
		}

		open(context.TODO(), &a.win.Window, func(res gio.AsyncResulter) {
			files, err := finish(res)
			if err != nil {
				if !errHasCode(err, int(gtk.DialogErrorDismissed)) {
					slog.Error("open files", "err", err)
				}
				return
			}

			for file := range listmodels.Objects(files) {
				go a.pushFile(context.TODO(), page.peer.ID, file.Cast().(gio.Filer))
			}
		})
	})
	actions.AddAction(sendFileAction)

	page.AddController(page.DropTarget)
	page.DropTarget.SetGTypes([]glib.Type{gio.GTypeFile})
	page.DropTarget.ConnectDrop(func(val *glib.Value, x, y float64) bool {
		file, ok := val.Object().Cast().(gio.Filer)
		if !ok {
			return true
		}
		go a.pushFile(context.TODO(), page.peer.ID, file)
		return true
	})

	page.addrModel = gioutil.NewListModel[netip.Addr]()
	listmodels.BindListBox(
		page.IPList,
		gtk.NewSortListModel(page.addrModel, &addrSorter.Sorter),
		func(addr netip.Addr) gtk.Widgetter {
			copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")

			copyButton.SetMarginTop(12) // Why is this necessary?
			copyButton.SetMarginBottom(12)
			copyButton.SetHasFrame(false)
			copyButton.SetTooltipText("Copy to Clipboard")
			copyButton.ConnectClicked(func() {
				a.clip(glib.NewValue(addr.String()))
				a.toast("Copied to clipboard")
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
				a.poller.Poll() <- struct{}{}
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

		var node *ipnstate.PeerStatus
		if s {
			node = page.peer
		}
		err := tsutil.ExitNode(context.TODO(), node)
		if err != nil {
			slog.Error("set exit node", "err", err)
			page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})
}

func (page *PeerPage) Update(a *App, vp *adw.ViewStackPage, status tsutil.Status) bool {
	page.peer = status.Status.Peer[page.peer.PublicKey]
	if page.peer == nil {
		return false
	}

	vp.SetTitle(peerName(status, page.peer))
	vp.SetIconName(peerIcon(page.peer))

	page.Page.SetTitle(page.peer.HostName)
	page.Page.SetDescription(page.peer.DNSName)

	page.ExitNodeRow.SetVisible(page.peer.ExitNodeOption)
	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetState(page.peer.ExitNode)
	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(page.peer.ExitNode)
	page.RxBytes.SetText(strconv.FormatInt(page.peer.RxBytes, 10))
	page.TxBytes.SetText(strconv.FormatInt(page.peer.TxBytes, 10))
	page.Created.SetText(formatTime(page.peer.Created))
	page.LastSeen.SetText(formatTime(page.peer.LastSeen))
	page.LastSeenRow.SetVisible(!page.peer.Online)
	page.LastWrite.SetText(formatTime(page.peer.LastWrite))
	page.LastHandshake.SetText(formatTime(page.peer.LastHandshake))
	page.Online.SetFromIconName(boolIcon(page.peer.Online))

	routes := func(yield func(netip.Prefix) bool) {
		if page.peer.PrimaryRoutes == nil {
			return
		}
		for _, r := range page.peer.PrimaryRoutes.All() {
			if r.Bits() == 0 {
				continue
			}
			if !yield(r) {
				return
			}
		}
	}

	listmodels.Update(page.addrModel, slices.Values(page.peer.TailscaleIPs))
	listmodels.Update(page.routeModel, routes)

	return true
}
