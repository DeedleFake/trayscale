package ui

import (
	"cmp"
	"context"
	_ "embed"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"time"

	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xnetip"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/inhies/go-bytesize"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

//go:embed selfpage.ui
var selfPageXML string

type SelfPage struct {
	app     *App
	row     *PageRow
	peer    tailcfg.NodeView
	actions *gio.SimpleActionGroup

	Page                 *adw.StatusPage
	IPList               *gtk.ListBox
	OptionsGroup         *adw.PreferencesGroup
	AdvertiseExitNodeRow *adw.SwitchRow
	AllowLANAccessRow    *adw.SwitchRow
	AcceptRoutesRow      *adw.SwitchRow
	AdvertisedRoutesList *gtk.ListBox
	AdvertiseRouteButton *gtk.Button
	NetCheckGroup        *adw.PreferencesGroup
	NetCheckButton       *gtk.Button
	LastNetCheckRow      *adw.ActionRow
	LastNetCheck         *gtk.Label
	UDPRow               *adw.ActionRow
	UDP                  *gtk.Image
	IPv4Row              *adw.ActionRow
	IPv4Icon             *gtk.Image
	IPv4Addr             *gtk.Label
	IPv6Row              *adw.ActionRow
	IPv6Icon             *gtk.Image
	IPv6Addr             *gtk.Label
	UPnPRow              *adw.ActionRow
	UPnP                 *gtk.Image
	PMPRow               *adw.ActionRow
	PMP                  *gtk.Image
	PCPRow               *adw.ActionRow
	PCP                  *gtk.Image
	CaptivePortalRow     *adw.ActionRow
	CaptivePortal        *gtk.Image
	PreferredDERPRow     *adw.ActionRow
	PreferredDERP        *gtk.Label
	DERPLatencies        *adw.ExpanderRow
	FilesList            *gtk.ListBox

	addrModel  *gioutil.ListModel[netip.Addr]
	routeModel *gioutil.ListModel[netip.Prefix]
	fileModel  *gioutil.ListModel[apitype.WaitingFile]
}

func NewSelfPage(a *App, status *tsutil.IPNStatus) *SelfPage {
	var page SelfPage
	fillFromBuilder(&page, selfPageXML)
	page.init(a, status)
	return &page
}

func (page *SelfPage) init(a *App, status *tsutil.IPNStatus) {
	page.app = a
	page.peer = status.NetMap.SelfNode

	page.actions = gio.NewSimpleActionGroup()

	copyFQDN := gio.NewSimpleAction("copyFQDN", nil)
	copyFQDN.ConnectActivate(func(p *glib.Variant) {
		a.clip(glib.NewValue(strings.TrimSuffix(page.peer.Name(), ".")))
		a.win.Toast("Copied FQDN to clipboard")
	})
	page.actions.AddAction(copyFQDN)

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

	page.fileModel = gioutil.NewListModel[apitype.WaitingFile]()
	listmodels.BindListBox(
		page.FilesList,
		gtk.NewSortListModel(page.fileModel, &waitingFileSorter.Sorter),
		func(file apitype.WaitingFile) gtk.Widgetter {
			saveButton := gtk.NewButtonFromIconName("document-save-symbolic")
			saveButton.SetMarginTop(12)
			saveButton.SetMarginBottom(12)
			saveButton.SetHasFrame(false)
			saveButton.SetTooltipText("Save")
			saveButton.ConnectClicked(func() {
				dialog := gtk.NewFileDialog()
				dialog.SetModal(true)
				dialog.SetInitialName(file.Name)
				dialog.Save(context.TODO(), &a.win.MainWindow.Window, func(res gio.AsyncResulter) {
					f, err := dialog.SaveFinish(res)
					if err != nil {
						if !errHasCode(err, int(gtk.DialogErrorDismissed)) {
							slog.Error("save file", "err", err)
						}
						return
					}

					go a.saveFile(context.TODO(), file.Name, f)
				})
			})

			deleteButton := gtk.NewButtonFromIconName("edit-delete-symbolic")
			deleteButton.SetMarginTop(12)
			deleteButton.SetMarginBottom(12)
			deleteButton.SetHasFrame(false)
			deleteButton.SetTooltipText("Delete")
			deleteButton.ConnectClicked(func() {
				Confirmation{
					Heading: "Delete file?",
					Body:    "If you delete this file, you will no longer be able to save it to your local machine.",
					Accept:  "_Delete",
					Reject:  "_Cancel",
				}.Show(a, func(accept bool) {
					if accept {
						err := tsutil.DeleteWaitingFile(context.TODO(), file.Name)
						if err != nil {
							slog.Error("delete file", "err", err)
							return
						}
						<-a.poller.Poll()
					}
				})
			})

			row := adw.NewActionRow()
			row.AddSuffix(saveButton)
			row.AddSuffix(deleteButton)
			row.SetTitle(file.Name)
			row.SetSubtitle(bytesize.ByteSize(file.Size).String())

			return row
		},
	)

	filesListPlaceholder := adw.NewActionRow()
	filesListPlaceholder.SetTitle("No incoming files.")
	page.FilesList.SetPlaceholder(filesListPlaceholder)

	page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).ConnectStateSet(func(s bool) bool {
		if s == page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).State() {
			return false
		}

		if s {
			err := tsutil.ExitNode(context.TODO(), "")
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
		return true
	})

	page.AdvertiseRouteButton.ConnectClicked(func() {
		Prompt{
			Heading:     "Add IP Prefix",
			Placeholder: "10.0.0.0/24",
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
		namedLats := func(yield func(latencyEntry) bool) {
			for id, latency := range r.RegionLatency {
				named := xiter.P(dm.Regions[id].RegionName, latency)
				if !yield(named) {
					return
				}
			}
		}
		sortedLats := slices.SortedFunc(namedLats, func(p1, p2 latencyEntry) int { return cmp.Compare(p1.V2, p2.V2) })
		latencyRows.Update(sortedLats)
	})
}

func (page *SelfPage) Widget() gtk.Widgetter {
	return page.Page
}

func (page *SelfPage) Actions() gio.ActionGrouper {
	return page.actions
}

func (page *SelfPage) Init(row *PageRow) {
	page.row = row
	row.SetSubtitle("This machine")
}

func (page *SelfPage) Update(status tsutil.Status) bool {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
		return page.UpdateIPN(status)
	case *tsutil.FileStatus:
		return page.UpdateFiles(status)
	default:
		return true
	}
}

func (page *SelfPage) UpdateIPN(status *tsutil.IPNStatus) bool {
	if !status.Online() {
		return false
	}

	page.peer = status.NetMap.SelfNode

	page.row.SetTitle(peerName(page.peer))
	page.row.SetIconName("computer-symbolic")

	page.Page.SetTitle(page.peer.Hostinfo().Hostname())
	page.Page.SetDescription(page.peer.Name())

	page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).SetState(status.Prefs.AdvertisesExitNode())
	page.AdvertiseExitNodeRow.ActivatableWidget().(*gtk.Switch).SetActive(status.Prefs.AdvertisesExitNode())
	page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).SetState(status.Prefs.ExitNodeAllowLANAccess())
	page.AllowLANAccessRow.ActivatableWidget().(*gtk.Switch).SetActive(status.Prefs.ExitNodeAllowLANAccess())
	page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).SetState(status.Prefs.RouteAll())
	page.AcceptRoutesRow.ActivatableWidget().(*gtk.Switch).SetActive(status.Prefs.RouteAll())

	routes := func(yield func(netip.Prefix) bool) {
		for _, r := range status.Prefs.AdvertiseRoutes().All() {
			if r.Bits() != 0 {
				if !yield(r) {
					return
				}
			}
		}
	}

	listmodels.Update(page.addrModel, xiter.Map(xiter.V2(page.peer.Addresses().All()), netip.Prefix.Addr))
	listmodels.Update(page.routeModel, routes)

	return true
}

func (page *SelfPage) UpdateFiles(status *tsutil.FileStatus) bool {
	listmodels.Update(page.fileModel, slices.Values(status.Files))
	return true
}
