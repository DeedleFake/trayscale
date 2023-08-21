package ui

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xcmp"
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

type peerPage struct {
	page      *gtk.StackPage
	container *PeerPage

	self   bool
	routes []netip.Prefix

	addrRows  rowManager[netip.Addr]
	routeRows rowManager[enum[netip.Prefix]]
	fileRows  rowManager[apitype.WaitingFile]
}

func (a *App) newPeerPage(status tsutil.Status, peer *ipnstate.PeerStatus) *peerPage {
	page := peerPage{
		container: NewPeerPage(),
		self:      peer.PublicKey == status.Status.Self.PublicKey,
	}

	actions := gio.NewSimpleActionGroup()
	page.container.InsertActionGroup("peer", actions)

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

	if !page.self {
		page.container.AddController(page.container.DropTarget)
		page.container.DropTarget.SetGTypes([]glib.Type{gio.GTypeFile})
		// BUG: ConnectDrop() doesn't work. See
		// https://github.com/diamondburned/gotk4/issues/107#issuecomment-1685377125
		page.container.DropTarget.Connect("drop", func(val *glib.Value, x, y float64) bool {
			file, ok := val.Object().Cast().(*gio.File)
			if !ok {
				return true
			}
			go a.pushFile(context.TODO(), peer.ID, file)
			return true
		})
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

	if page.self {
		page.fileRows.Parent = page.container.FilesGroup
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

	return &page
}

func (a *App) updatePeerPage(page *peerPage, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.self = peer.PublicKey == status.Status.Self.PublicKey
	// TODO: Disconnect drag-and-drop when this changes to true.

	page.page.SetIconName(peerIcon(peer))
	page.page.SetTitle(peerName(status, peer, page.self))

	page.container.SetTitle(peer.HostName)
	page.container.SetDescription(peer.DNSName)

	slices.SortFunc(peer.TailscaleIPs, netip.Addr.Compare)
	page.addrRows.Update(peer.TailscaleIPs)

	page.container.OptionsGroup.SetVisible(page.self)
	if page.self {
		page.container.AdvertiseExitNodeSwitch.SetState(status.Prefs.AdvertisesExitNode())
		page.container.AdvertiseExitNodeSwitch.SetActive(status.Prefs.AdvertisesExitNode())
		page.container.AllowLANAccessSwitch.SetState(status.Prefs.ExitNodeAllowLANAccess)
		page.container.AllowLANAccessSwitch.SetActive(status.Prefs.ExitNodeAllowLANAccess)
	}

	if page.self {
		page.fileRows.Update(status.Files)
	}
	page.container.FilesGroup.SetVisible(page.self && (len(status.Files) > 0))
	page.container.SendFileGroup.SetVisible(!page.self)

	page.container.AdvertiseRouteButton.SetVisible(page.self)

	switch {
	case page.self:
		page.routes = status.Prefs.AdvertiseRoutes
	case peer.PrimaryRoutes != nil:
		page.routes = peer.PrimaryRoutes.AsSlice()
	}
	page.routes = xslices.Filter(page.routes, func(p netip.Prefix) bool { return p.Bits() != 0 })
	slices.SortFunc(page.routes, func(p1, p2 netip.Prefix) int {
		return xcmp.Or(p1.Addr().Compare(p2.Addr()), p1.Bits()-p2.Bits())
	})
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
