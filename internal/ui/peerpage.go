package ui

import (
	"cmp"
	"context"
	_ "embed"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

//go:embed peerpage.ui
var peerPageXML string

type PeerPage struct {
	*adw.StatusPage `gtk:"Page"`

	IPGroup               *adw.PreferencesGroup
	AdvertisedRoutesGroup *adw.PreferencesGroup
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
	SendFileGroup         *adw.PreferencesGroup
	SendFileRow           *adw.ActionRow
	DropTarget            *gtk.DropTarget

	peer *ipnstate.PeerStatus
	name string

	routes []netip.Prefix

	addrRows  rowManager[netip.Addr]
	routeRows rowManager[enum[netip.Prefix]]
}

func NewPeerPage(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) *PeerPage {
	var page PeerPage
	fillFromBuilder(&page, peerPageXML)
	page.init(a, peer, status)
	return &page
}

func (page *PeerPage) Root() gtk.Widgetter {
	return page.StatusPage
}

func (page *PeerPage) ID() string {
	return page.peer.PublicKey.String()
}

func (page *PeerPage) Name() string {
	return page.name
}

func (page *PeerPage) init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.peer = peer

	actions := gio.NewSimpleActionGroup()
	page.InsertActionGroup("peer", actions)

	sendFileAction := gio.NewSimpleAction("sendfile", nil)
	sendFileAction.ConnectActivate(func(p *glib.Variant) {
		fc := gtk.NewFileChooserNative("", &a.win.Window, gtk.FileChooserActionOpen, "", "")
		fc.SetModal(true)
		fc.SetSelectMultiple(true)
		fc.ConnectResponse(func(id int) {
			switch gtk.ResponseType(id) {
			case gtk.ResponseAccept:
				files := fc.Files()
				for i := uint(0); i < files.NItems(); i++ {
					file := files.Item(i).Cast().(*gio.File)
					go a.pushFile(context.TODO(), peer.ID, file)
				}
			}
		})
		fc.Show()
	})
	actions.AddAction(sendFileAction)

	page.AddController(page.DropTarget)
	page.DropTarget.SetGTypes([]glib.Type{gio.GTypeFile})
	page.DropTarget.ConnectDrop(func(val *glib.Value, x, y float64) bool {
		file, ok := val.Object().Cast().(*gio.File)
		if !ok {
			return true
		}
		go a.pushFile(context.TODO(), peer.ID, file)
		return true
	})

	page.addrRows.Parent = page.IPGroup
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
			a.clip(glib.NewValue(row.ip.String()))
			a.toast("Copied to clipboard")
		})

		row.w.SetObjectProperty("title-selectable", true)
		row.w.AddSuffix(row.c)
		row.w.SetActivatableWidget(row.c)
		row.w.SetTitle(ip.String())

		return &row
	}

	page.routeRows.Parent = page.AdvertisedRoutesGroup
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

	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).ConnectStateSet(func(s bool) bool {
		if s == page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).State() {
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
			page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})
}

func (page *PeerPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.peer = peer
	page.name = peerName(status, peer)

	page.SetTitle(peer.HostName)
	page.SetDescription(peer.DNSName)

	slices.SortFunc(peer.TailscaleIPs, netip.Addr.Compare)
	page.addrRows.Update(peer.TailscaleIPs)

	if peer.PrimaryRoutes != nil {
		page.routes = peer.PrimaryRoutes.AsSlice()
	}
	page.routes = xslices.Filter(page.routes, func(p netip.Prefix) bool { return p.Bits() != 0 })
	slices.SortFunc(page.routes, func(p1, p2 netip.Prefix) int {
		return cmp.Or(p1.Addr().Compare(p2.Addr()), p1.Bits()-p2.Bits())
	})
	if len(page.routes) == 0 {
		page.routes = append(page.routes, netip.Prefix{})
	}
	eroutes := make([]enum[netip.Prefix], 0, len(page.routes))
	for i, r := range page.routes {
		i = -1
		eroutes = append(eroutes, enumerate(i, r))
	}
	page.routeRows.Update(eroutes)

	page.ExitNodeRow.SetVisible(peer.ExitNodeOption)
	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetState(peer.ExitNode)
	page.ExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(peer.ExitNode)
	page.RxBytes.SetText(strconv.FormatInt(peer.RxBytes, 10))
	page.TxBytes.SetText(strconv.FormatInt(peer.TxBytes, 10))
	page.Created.SetText(formatTime(peer.Created))
	page.LastSeen.SetText(formatTime(peer.LastSeen))
	page.LastSeenRow.SetVisible(!peer.Online)
	page.LastWrite.SetText(formatTime(peer.LastWrite))
	page.LastHandshake.SetText(formatTime(peer.LastHandshake))
	page.Online.SetFromIconName(boolIcon(peer.Online))
}
