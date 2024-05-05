package ui

import (
	"cmp"
	"context"
	_ "embed"
	"log/slog"
	"net/netip"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xmaps"
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/inhies/go-bytesize"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
)

//go:embed selfpage.ui
var selfPageXML string

type SelfPage struct {
	*adw.StatusPage `gtk:"Page"`

	IPGroup                 *adw.PreferencesGroup
	OptionsGroup            *adw.PreferencesGroup
	AdvertiseExitNodeRow    *adw.ActionRow
	AdvertiseExitNodeSwitch *gtk.Switch
	AllowLANAccessRow       *adw.ActionRow
	AllowLANAccessSwitch    *gtk.Switch
	AcceptRoutesRow         *adw.ActionRow
	AcceptRoutesSwitch      *gtk.Switch
	AdvertisedRoutesGroup   *adw.PreferencesGroup
	AdvertiseRouteButton    *gtk.Button
	NetCheckGroup           *adw.PreferencesGroup
	NetCheckButton          *gtk.Button
	LastNetCheckRow         *adw.ActionRow
	LastNetCheck            *gtk.Label
	UDPRow                  *adw.ActionRow
	UDP                     *gtk.Image
	IPv4Row                 *adw.ActionRow
	IPv4Icon                *gtk.Image
	IPv4Addr                *gtk.Label
	IPv6Row                 *adw.ActionRow
	IPv6Icon                *gtk.Image
	IPv6Addr                *gtk.Label
	UPnPRow                 *adw.ActionRow
	UPnP                    *gtk.Image
	PMPRow                  *adw.ActionRow
	PMP                     *gtk.Image
	PCPRow                  *adw.ActionRow
	PCP                     *gtk.Image
	HairPinningRow          *adw.ActionRow
	HairPinning             *gtk.Image
	PreferredDERPRow        *adw.ActionRow
	PreferredDERP           *gtk.Label
	DERPLatencies           *adw.ExpanderRow
	FilesGroup              *adw.PreferencesGroup

	peer *ipnstate.PeerStatus
	name string

	routes []netip.Prefix

	addrRows  rowManager[netip.Addr]
	routeRows rowManager[enum[netip.Prefix]]
	fileRows  rowManager[apitype.WaitingFile]
}

func NewSelfPage(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) *SelfPage {
	var page SelfPage
	fillFromBuilder(&page, selfPageXML)
	page.init(a, peer, status)
	return &page
}

func (page *SelfPage) Root() gtk.Widgetter {
	return page.StatusPage
}

func (page *SelfPage) ID() string {
	return page.peer.PublicKey.String()
}

func (page *SelfPage) Name() string {
	return page.name
}

func (page *SelfPage) init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
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

	page.fileRows.Parent = page.FilesGroup
	page.fileRows.New = func(file apitype.WaitingFile) row[apitype.WaitingFile] {
		row := fileRow{
			file: file,

			w: adw.NewActionRow(),
			s: gtk.NewButtonFromIconName("document-save-symbolic"),
			d: gtk.NewButtonFromIconName("edit-delete-symbolic"),
		}

		row.w.AddSuffix(row.s)
		row.w.AddSuffix(row.d)
		row.w.SetTitle(file.Name)
		row.w.SetSubtitle(bytesize.ByteSize(file.Size).String())

		row.s.SetMarginTop(12)
		row.s.SetMarginBottom(12)
		row.s.SetHasFrame(false)
		row.s.SetTooltipText("Save")
		row.s.ConnectClicked(func() {
			fc := gtk.NewFileChooserNative("", &a.win.Window, gtk.FileChooserActionSave, "", "")
			fc.SetModal(true)
			fc.SetCurrentName(row.file.Name)
			fc.ConnectResponse(func(id int) {
				switch gtk.ResponseType(id) {
				case gtk.ResponseAccept:
					go a.saveFile(context.TODO(), row.file.Name, fc.File())
				}
			})
			fc.Show()
		})

		row.d.SetMarginTop(12)
		row.d.SetMarginBottom(12)
		row.d.SetHasFrame(false)
		row.d.SetTooltipText("Delete")
		row.d.ConnectClicked(func() {
			Confirmation{
				Heading: "Delete file?",
				Body:    "If you delete this file, you will no longer be able to save it to your local machine.",
				Accept:  "_Delete",
				Reject:  "_Cancel",
			}.Show(a, func(accept bool) {
				if accept {
					err := a.TS.DeleteWaitingFile(context.TODO(), row.file.Name)
					if err != nil {
						slog.Error("delete file", "err", err)
						return
					}
					a.poller.Poll() <- struct{}{}
				}
			})
		})

		return &row
	}

	page.AdvertiseExitNodeSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.AdvertiseExitNodeSwitch.State() {
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
			page.AdvertiseExitNodeSwitch.SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.AllowLANAccessSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.AllowLANAccessSwitch.State() {
			return false
		}

		err := a.TS.AllowLANAccess(context.TODO(), s)
		if err != nil {
			slog.Error("allow LAN access", "err", err)
			page.AllowLANAccessSwitch.SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.AcceptRoutesSwitch.ConnectStateSet(func(s bool) bool {
		if s == page.AcceptRoutesSwitch.State() {
			return false
		}

		err := a.TS.AcceptRoutes(context.TODO(), s)
		if err != nil {
			slog.Error("accept routes", "err", err)
			page.AcceptRoutesSwitch.SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.AdvertiseRouteButton.ConnectClicked(func() {
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
		Parent: rowAdderParent{page.DERPLatencies},
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

	page.NetCheckButton.ConnectClicked(func() {
		r, dm, err := a.TS.NetCheck(context.TODO(), true)
		if err != nil {
			slog.Error("netcheck", "err", err)
			return
		}

		page.LastNetCheck.SetText(formatTime(time.Now()))
		page.UDPRow.SetVisible(true)
		page.UDP.SetFromIconName(boolIcon(r.UDP))
		page.IPv4Row.SetVisible(true)
		page.IPv4Icon.SetVisible(!r.IPv4)
		page.IPv4Icon.SetFromIconName(boolIcon(r.IPv4))
		page.IPv4Addr.SetVisible(r.IPv4)
		page.IPv4Addr.SetText(r.GlobalV4)
		page.IPv6Row.SetVisible(true)
		page.IPv6Icon.SetVisible(!r.IPv6)
		page.IPv6Icon.SetFromIconName(boolIcon(r.IPv6))
		page.IPv6Addr.SetVisible(r.IPv6)
		page.IPv6Addr.SetText(r.GlobalV6)
		page.UPnPRow.SetVisible(true)
		page.UPnP.SetFromIconName(optBoolIcon(r.UPnP))
		page.PMPRow.SetVisible(true)
		page.PMP.SetFromIconName(optBoolIcon(r.PMP))
		page.PCPRow.SetVisible(true)
		page.PCP.SetFromIconName(optBoolIcon(r.PCP))
		page.HairPinningRow.SetVisible(true)
		page.HairPinning.SetFromIconName(optBoolIcon(r.HairPinning))
		page.PreferredDERPRow.SetVisible(true)
		page.PreferredDERP.SetText(dm.Regions[r.PreferredDERP].RegionName)

		page.DERPLatencies.SetVisible(true)
		latencies := xmaps.Entries(r.RegionLatency)
		slices.SortFunc(latencies, func(e1, e2 xmaps.Entry[int, time.Duration]) int { return int(e1.Val - e2.Val) })
		namedLats := make([]xmaps.Entry[string, time.Duration], 0, len(latencies))
		for _, lat := range latencies {
			namedLats = append(namedLats, xmaps.Entry[string, time.Duration]{
				Key: dm.Regions[lat.Key].RegionName,
				Val: lat.Val,
			})
		}
		latencyRows.Update(namedLats)
	})
}

func (page *SelfPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.peer = peer
	page.name = peerName(status, peer)

	page.SetTitle(peer.HostName)
	page.SetDescription(peer.DNSName)

	slices.SortFunc(peer.TailscaleIPs, netip.Addr.Compare)
	page.addrRows.Update(peer.TailscaleIPs)

	page.AdvertiseExitNodeSwitch.SetState(status.Prefs.AdvertisesExitNode())
	page.AdvertiseExitNodeSwitch.SetActive(status.Prefs.AdvertisesExitNode())
	page.AllowLANAccessSwitch.SetState(status.Prefs.ExitNodeAllowLANAccess)
	page.AllowLANAccessSwitch.SetActive(status.Prefs.ExitNodeAllowLANAccess)
	page.AcceptRoutesSwitch.SetState(status.Prefs.RouteAll)
	page.AcceptRoutesSwitch.SetActive(status.Prefs.RouteAll)

	page.fileRows.Update(status.Files)
	page.FilesGroup.SetVisible(len(status.Files) > 0)

	page.routes = status.Prefs.AdvertiseRoutes
	page.routes = xslices.Filter(page.routes, func(p netip.Prefix) bool { return p.Bits() != 0 })
	slices.SortFunc(page.routes, func(p1, p2 netip.Prefix) int {
		return cmp.Or(p1.Addr().Compare(p2.Addr()), p1.Bits()-p2.Bits())
	})
	if len(page.routes) == 0 {
		page.routes = append(page.routes, netip.Prefix{})
	}
	eroutes := make([]enum[netip.Prefix], 0, len(page.routes))
	for i, r := range page.routes {
		eroutes = append(eroutes, enumerate(i, r))
	}
	page.routeRows.Update(eroutes)
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

type fileRow struct {
	file apitype.WaitingFile

	w *adw.ActionRow
	s *gtk.Button
	d *gtk.Button
}

func (row *fileRow) Update(file apitype.WaitingFile) {
	row.file = file
	row.w.SetTitle(file.Name)
	row.w.SetSubtitle(bytesize.ByteSize(file.Size).String())
}

func (row *fileRow) Widget() gtk.Widgetter {
	return row.w
}
