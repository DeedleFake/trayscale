package ui

import (
	"cmp"
	"context"
	_ "embed"
	"iter"
	"log/slog"
	"net/netip"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
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

	IPGroup               *adw.PreferencesGroup
	OptionsGroup          *adw.PreferencesGroup
	AdvertiseExitNodeRow  *adw.SwitchRow
	AllowLANAccessRow     *adw.SwitchRow
	AcceptRoutesRow       *adw.SwitchRow
	AdvertisedRoutesGroup *adw.PreferencesGroup
	AdvertiseRouteButton  *gtk.Button
	NetCheckGroup         *adw.PreferencesGroup
	NetCheckButton        *gtk.Button
	LastNetCheckRow       *adw.ActionRow
	LastNetCheck          *gtk.Label
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
	CaptivePortalRow      *adw.ActionRow
	CaptivePortal         *gtk.Image
	PreferredDERPRow      *adw.ActionRow
	PreferredDERP         *gtk.Label
	DERPLatencies         *adw.ExpanderRow
	FilesGroup            *adw.PreferencesGroup

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
	return string(page.peer.ID)
}

func (page *SelfPage) Name() string {
	return page.name
}

func (page *SelfPage) init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.peer = peer

	actions := gio.NewSimpleActionGroup()
	page.InsertActionGroup("peer", actions)

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
			err := tsutil.AdvertiseRoutes(context.TODO(), routes)
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
			dialog := gtk.NewFileDialog()
			dialog.SetModal(true)
			dialog.SetInitialName(row.file.Name)
			dialog.Save(context.TODO(), &a.win.Window, func(res gio.AsyncResulter) {
				file, err := dialog.SaveFinish(res)
				if err != nil {
					if !errHasCode(err, int(gtk.DialogErrorDismissed)) {
						slog.Error("save file", "err", err)
					}
					return
				}

				go a.saveFile(context.TODO(), row.file.Name, file)
			})
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
					err := tsutil.DeleteWaitingFile(context.TODO(), row.file.Name)
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

	page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).ConnectStateSet(func(s bool) bool {
		if s == page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).State() {
			return false
		}

		if s {
			err := tsutil.ExitNode(context.TODO(), nil)
			if err != nil {
				slog.Error("disable existing exit node", "err", err)
				// Continue anyways.
			}
		}

		err := tsutil.AdvertiseExitNode(context.TODO(), s)
		if err != nil {
			slog.Error("advertise exit node", "err", err)
			page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).ConnectStateSet(func(s bool) bool {
		if s == page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).State() {
			return false
		}

		err := tsutil.AllowLANAccess(context.TODO(), s)
		if err != nil {
			slog.Error("allow LAN access", "err", err)
			page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).ConnectStateSet(func(s bool) bool {
		if s == page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).State() {
			return false
		}

		err := tsutil.AcceptRoutes(context.TODO(), s)
		if err != nil {
			slog.Error("accept routes", "err", err)
			page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).SetActive(!s)
			return true
		}
		a.poller.Poll() <- struct{}{}
		return true
	})

	page.AdvertiseRouteButton.ConnectClicked(func() {
		Prompt{
			Heading: "Add IP",
			Body:    "IP prefix to advertise",
			Responses: []PromptResponse{
				{ID: "cancel", Label: "_Cancel"},
				{ID: "add", Label: "_Add", Appearance: adw.ResponseSuggested, Default: true},
			},
		}.Show(a, "", func(response, val string) {
			if response != "add" {
				return
			}

			p, err := netip.ParsePrefix(val)
			if err != nil {
				slog.Error("parse prefix", "err", err)
				return
			}

			prefs, err := tsutil.Prefs(context.TODO())
			if err != nil {
				slog.Error("get prefs", "err", err)
				return
			}

			err = tsutil.AdvertiseRoutes(
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

	type latencyEntry = xiter.Pair[string, time.Duration]
	latencyRows := rowManager[latencyEntry]{
		Parent: rowAdderParent{page.DERPLatencies},
		New: func(lat latencyEntry) row[latencyEntry] {
			label := gtk.NewLabel(lat.V2.String())

			row := adw.NewActionRow()
			row.SetTitle(lat.V1)
			row.AddSuffix(label)

			return &simpleRow[latencyEntry]{
				W: row,
				U: func(lat latencyEntry) {
					label.SetText(lat.V2.String())
					row.SetTitle(lat.V1)
				},
			}
		},
	}

	page.NetCheckButton.ConnectClicked(func() {
		r, dm, err := tsutil.NetCheck(context.TODO(), true)
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
		page.IPv4Addr.SetText(r.GlobalV4.String())
		page.IPv6Row.SetVisible(true)
		page.IPv6Icon.SetVisible(!r.IPv6)
		page.IPv6Icon.SetFromIconName(boolIcon(r.IPv6))
		page.IPv6Addr.SetVisible(r.IPv6)
		page.IPv6Addr.SetText(r.GlobalV6.String())
		page.UPnPRow.SetVisible(true)
		page.UPnP.SetFromIconName(optBoolIcon(r.UPnP))
		page.PMPRow.SetVisible(true)
		page.PMP.SetFromIconName(optBoolIcon(r.PMP))
		page.PCPRow.SetVisible(true)
		page.PCP.SetFromIconName(optBoolIcon(r.PCP))
		page.CaptivePortalRow.SetVisible(true)
		page.CaptivePortal.SetFromIconName(optBoolIcon(r.CaptivePortal))
		page.PreferredDERPRow.SetVisible(true)
		page.PreferredDERP.SetText(dm.Regions[r.PreferredDERP].RegionName)

		page.DERPLatencies.SetVisible(true)
		namedLats := slices.SortedFunc(iter.Seq[xiter.Pair[string, time.Duration]](xiter.Map(xiter.ToPair(xiter.OfMap(r.RegionLatency)),
			func(p xiter.Pair[int, time.Duration]) xiter.Pair[string, time.Duration] {
				return xiter.P(dm.Regions[p.V1].RegionName, p.V2)
			})),
			func(p1, p2 xiter.Pair[string, time.Duration]) int { return cmp.Compare(p1.V2, p2.V2) })
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

	page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).SetState(status.Prefs.AdvertisesExitNode())
	page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(status.Prefs.AdvertisesExitNode())
	page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).SetState(status.Prefs.ExitNodeAllowLANAccess)
	page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).SetActive(status.Prefs.ExitNodeAllowLANAccess)
	page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).SetState(status.Prefs.RouteAll)
	page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).SetActive(status.Prefs.RouteAll)

	page.fileRows.Update(status.Files)
	page.FilesGroup.SetVisible(len(status.Files) > 0)

	routes := func(yield func(netip.Prefix) bool) {
		for _, r := range status.Prefs.AdvertiseRoutes {
			if r.Bits() == 0 {
				continue
			}
			if !yield(r) {
				return
			}
		}
	}
	routes = xiter.Or(
		routes,
		xiter.Of(netip.Prefix{}),
	)

	clear(page.routes)
	page.routes = page.routes[:0]
	page.routes = slices.AppendSeq(page.routes, routes)
	slices.SortFunc(page.routes, func(p1, p2 netip.Prefix) int {
		return cmp.Or(
			p1.Addr().Compare(p2.Addr()),
			cmp.Compare(p1.Bits(), p2.Bits()),
		)
	})

	eroutes := func(yield func(enum[netip.Prefix]) bool) {
		for i, r := range page.routes {
			if !yield(enumerate(i, r)) {
				return
			}
		}
	}
	page.routeRows.UpdateFromSeq(eroutes, len(page.routes))
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
