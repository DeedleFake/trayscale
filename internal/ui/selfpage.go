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

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
	"github.com/inhies/go-bytesize"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

//go:embed selfpage.ui
var selfPageXML string

type SelfPage struct {
	app       *App
	stackPage *adw.ViewStackPage
	peer      tailcfg.NodeView
	actions   *gio.SimpleActionGroup

	Page                 *adw.StatusPage
	IPList               *gtk.ListBox
	OptionsGroup         *adw.PreferencesGroup
	ExitNodeRow          *adw.ActionRow
	ExitNodeIcon         *gtk.Image
	AdvertiseExitNodeRow *adw.SwitchRow
	AllowLANAccessRow    *adw.SwitchRow
	AcceptRoutesRow      *adw.SwitchRow
	AcceptDNSRow         *adw.SwitchRow
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

	addrModel  *listmodels.Model[netip.Addr]
	routeModel *listmodels.Model[netip.Prefix]
	fileModel  *listmodels.Model[apitype.WaitingFile]

	incomingFiles int
	exitNode      tailcfg.NodeView
	latencyRows   rowManager[xiter.Pair[string, time.Duration]]
}

func NewSelfPage(a *App, status *tsutil.IPNStatus) *SelfPage {
	var page SelfPage
	gutil.FillFromUI(&page, selfPageXML)
	page.init(a, status)
	return &page
}

func (page *SelfPage) init(a *App, status *tsutil.IPNStatus) {
	page.app = a
	page.peer, _ = status.Self()
	if a.files != nil {
		page.incomingFiles = len(*a.files)
	}

	page.actions = gio.NewSimpleActionGroup()

	addPageAction(page.actions, gio.NewSimpleAction("copyFQDN", nil), func(*glib.Variant) {
		a.clip(strings.TrimSuffix(page.peer.Name(), "."))
		a.win.Toast(locale.Get("Copied FQDN to clipboard"))
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
		return page.routeRow(route)
	})
	routes.Unref()
	setPlaceholder(page.AdvertisedRoutesList, locale.Get("No advertised routes."))

	page.fileModel = listmodels.NewModel(waitingFileKey)
	files := page.fileModel.NewSortListModel(compareWaitingFiles)
	page.fileModel.BindListBox(page.FilesList, files, func(file apitype.WaitingFile) *gtk.Widget {
		return page.fileRow(file)
	})
	files.Unref()
	setPlaceholder(page.FilesList, locale.Get("No incoming files."))

	gutil.Connect(page.ExitNodeRow, page.ExitNodeRow.ConnectActivated, func(adw.ActionRow) {
		page.showExitNode()
	})

	page.connectSwitch(page.AdvertiseExitNodeRow, "advertise exit node", func(s bool) error {
		if s {
			err := tsutil.ExitNode(context.TODO(), "")
			if err != nil {
				slog.Error("disable existing exit node", "err", err)
				// Continue anyways.
			}
		}
		return tsutil.AdvertiseExitNode(context.TODO(), s)
	})
	page.connectSwitch(page.AllowLANAccessRow, "allow LAN access", func(s bool) error {
		return tsutil.AllowLANAccess(context.TODO(), s)
	})
	page.connectSwitch(page.AcceptRoutesRow, "accept routes", func(s bool) error {
		return tsutil.AcceptRoutes(context.TODO(), s)
	})
	page.connectSwitch(page.AcceptDNSRow, "accept DNS", func(s bool) error {
		return tsutil.AcceptDNS(context.TODO(), s)
	})

	gutil.Connect(page.AdvertiseRouteButton, page.AdvertiseRouteButton.ConnectClicked, func(gtk.Button) {
		Prompt{
			Heading:     locale.Get("Add IP Prefix"),
			Placeholder: "10.0.0.0/24",
			Responses: []PromptResponse{
				{ID: "cancel", Label: locale.Get("_Cancel")},
				{ID: "add", Label: locale.Get("_Add"), Appearance: adw.ResponseSuggestedValue, Default: true},
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
	page.latencyRows = rowManager[latencyEntry]{
		Parent: rowAdderParent{page.DERPLatencies},
		New: func(lat latencyEntry) row[latencyEntry] {
			label := gtk.NewLabel(lat.V2.String())

			row := adw.NewActionRow()
			row.SetTitle(lat.V1)
			row.AddSuffix(&label.Widget)
			label.Unref() // The row holds it; the closure below borrows it.

			return &simpleRow[latencyEntry]{
				W: &row.Widget,
				U: func(lat latencyEntry) {
					label.SetText(lat.V2.String())
					row.SetTitle(lat.V1)
				},
			}
		},
	}

	gutil.Connect(page.NetCheckButton, page.NetCheckButton.ConnectClicked, func(gtk.Button) {
		r, dm, err := tsutil.NetCheck(context.TODO(), true)
		if err != nil {
			slog.Error("netcheck", "err", err)
			return
		}

		page.LastNetCheck.SetText(formatTime(time.Now()))
		page.UDPRow.SetVisible(true)
		page.UDP.SetFromGicon(boolIcon(r.UDP))
		page.IPv4Row.SetVisible(true)
		page.IPv4Icon.SetVisible(!r.IPv4)
		page.IPv4Icon.SetFromGicon(boolIcon(r.IPv4))
		page.IPv4Addr.SetVisible(r.IPv4)
		page.IPv4Addr.SetText(r.GlobalV4.String())
		page.IPv6Row.SetVisible(true)
		page.IPv6Icon.SetVisible(!r.IPv6)
		page.IPv6Icon.SetFromGicon(boolIcon(r.IPv6))
		page.IPv6Addr.SetVisible(r.IPv6)
		page.IPv6Addr.SetText(r.GlobalV6.String())
		page.UPnPRow.SetVisible(true)
		page.UPnP.SetFromGicon(optBoolIcon(r.UPnP))
		page.PMPRow.SetVisible(true)
		page.PMP.SetFromGicon(optBoolIcon(r.PMP))
		page.PCPRow.SetVisible(true)
		page.PCP.SetFromGicon(optBoolIcon(r.PCP))
		page.CaptivePortalRow.SetVisible(true)
		page.CaptivePortal.SetFromGicon(optBoolIcon(r.CaptivePortal))
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
		page.latencyRows.Update(sortedLats)
	})
}

// connectSwitch connects a switch row to set, which applies a new
// state. On error the switch flips back.
func (page *SelfPage) connectSwitch(row *adw.SwitchRow, what string, set func(bool) error) {
	sw := rowSwitch(row)
	gutil.Connect(sw, sw.ConnectStateSet, func(_ gtk.Switch, s bool) bool {
		if s == sw.GetState() {
			return false
		}

		err := set(s)
		if err != nil {
			slog.Error(what, "err", err)
			sw.SetActive(!s)
			return true
		}
		return true
	})
}

func (page *SelfPage) routeRow(route netip.Prefix) *gtk.Widget {
	removeButton := gtk.NewButtonFromIconName("list-remove-symbolic")
	removeButton.SetMarginTop(12)
	removeButton.SetMarginBottom(12)
	removeButton.SetHasFrame(false)
	removeButton.SetTooltipText(locale.Get("Remove"))
	gutil.Connect(removeButton, removeButton.ConnectClicked, func(gtk.Button) {
		routes := slices.Collect(xiter.Filter(page.routeModel.All(), func(p netip.Prefix) bool {
			return p.Compare(route) != 0
		}))
		err := tsutil.AdvertiseRoutes(context.TODO(), routes)
		if err != nil {
			slog.Error("advertise routes", "err", err)
			return
		}
	})

	row := routeActionRow(route)
	row.AddSuffix(&removeButton.Widget)
	removeButton.Unref()
	return &row.Widget
}

func (page *SelfPage) fileRow(file apitype.WaitingFile) *gtk.Widget {
	a := page.app

	saveButton := gtk.NewButtonFromIconName("document-save-symbolic")
	saveButton.SetMarginTop(12)
	saveButton.SetMarginBottom(12)
	saveButton.SetHasFrame(false)
	saveButton.SetTooltipText(locale.Get("Save"))
	gutil.Connect(saveButton, saveButton.ConnectClicked, func(gtk.Button) {
		dialog := gtk.NewFileDialog()
		defer dialog.Unref() // The operation keeps it alive.
		dialog.SetModal(true)
		dialog.SetInitialName(file.Name)
		dialog.Save(&a.win.MainWindow.Window, nil, &gutil.AsyncCallback, gutil.Async(func(res *gio.AsyncResultBase) {
			f, err := dialog.SaveFinish(res)
			if err = gutil.Err(err); err != nil || f == nil {
				if !gutil.ErrHasCode(err, int(gtk.DialogErrorDismissedValue)) {
					slog.Error("save file", "err", err)
				}
				return
			}

			go func() {
				a.saveFile(context.TODO(), file.Name, f)
				gutil.IdleAdd(gutil.Obj(f).Unref)
			}()
		}))
	})

	deleteButton := gtk.NewButtonFromIconName("edit-delete-symbolic")
	deleteButton.SetMarginTop(12)
	deleteButton.SetMarginBottom(12)
	deleteButton.SetHasFrame(false)
	deleteButton.SetTooltipText(locale.Get("Delete"))
	gutil.Connect(deleteButton, deleteButton.ConnectClicked, func(gtk.Button) {
		Confirmation{
			Heading: locale.Get("Delete file?"),
			Body:    locale.Get("If you delete this file, you will no longer be able to save it to your local machine."),
			Accept:  locale.Get("_Delete"),
			Reject:  locale.Get("_Cancel"),
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
	row.AddSuffix(&saveButton.Widget)
	row.AddSuffix(&deleteButton.Widget)
	saveButton.Unref()
	deleteButton.Unref()
	row.SetTitle(file.Name)
	row.SetSubtitle(bytesize.ByteSize(file.Size).String())
	return &row.Widget
}

func (page *SelfPage) Widget() *gtk.Widget {
	return &page.Page.Widget
}

func (page *SelfPage) Actions() *gio.SimpleActionGroup {
	return page.actions
}

func (page *SelfPage) Destroy() {
	page.latencyRows.Update(nil)
	page.addrModel.Unref()
	page.routeModel.Unref()
	page.fileModel.Unref()
	page.actions.Unref()
	gutil.UnrefFields(page)
}

func (page *SelfPage) Bind(stackPage *adw.ViewStackPage) {
	page.stackPage = stackPage
	stackPage.SetIconName("computer-symbolic")
	page.applyIncomingBadge()
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
	self, ok := status.Self()
	if !ok {
		return true
	}

	page.peer = self

	page.stackPage.SetTitle(peerName(page.peer))

	page.Page.SetTitle(page.peer.Hostinfo().Hostname())
	page.Page.SetDescription(page.peer.Name())

	page.updateExitNodeRow(status)
	setSwitch(page.AdvertiseExitNodeRow, status.Prefs.AdvertisesExitNode())
	setSwitch(page.AllowLANAccessRow, status.Prefs.ExitNodeAllowLANAccess())
	setSwitch(page.AcceptRoutesRow, status.Prefs.RouteAll())
	setSwitch(page.AcceptDNSRow, status.Prefs.CorpDNS())

	routes := func(yield func(netip.Prefix) bool) {
		for _, r := range status.Prefs.AdvertiseRoutes().All() {
			if r.Bits() != 0 {
				if !yield(r) {
					return
				}
			}
		}
	}

	page.addrModel.Update(xiter.Map(xiter.V2(page.peer.Addresses().All()), netip.Prefix.Addr))
	page.routeModel.Update(routes)

	return true
}

func (page *SelfPage) UpdateFiles(status *tsutil.FileStatus) bool {
	page.fileModel.Update(slices.Values(status.Files))
	page.incomingFiles = len(status.Files)
	page.applyIncomingBadge()
	return true
}

func (page *SelfPage) applyIncomingBadge() {
	if page.stackPage == nil {
		return
	}
	page.stackPage.SetBadgeNumber(uint32(page.incomingFiles))
}

func waitingFileKey(f apitype.WaitingFile) string { return f.Name }

func compareWaitingFiles(f1, f2 apitype.WaitingFile) int {
	return cmp.Or(
		cmp.Compare(f1.Name, f2.Name),
		cmp.Compare(f1.Size, f2.Size),
	)
}

func (page *SelfPage) updateExitNodeRow(status *tsutil.IPNStatus) {
	page.exitNode = status.ExitNode()
	switch {
	case page.exitNode.Valid() && tsutil.IsMullvad(page.exitNode):
		page.ExitNodeRow.SetSubtitle(mullvadLongLocationName(page.exitNode.Hostinfo().Location()))
	case page.exitNode.Valid():
		page.ExitNodeRow.SetSubtitle(peerName(page.exitNode))
	case status.Prefs.ExitNodeIP().IsValid():
		page.ExitNodeRow.SetSubtitle(status.Prefs.ExitNodeIP().String())
	case status.Prefs.AutoExitNode().IsSet():
		page.ExitNodeRow.SetSubtitle(locale.Get("Waiting for an exit node"))
	case status.Prefs.ExitNodeID() != "":
		page.ExitNodeRow.SetSubtitle(locale.Get("Unavailable"))
	default:
		page.ExitNodeRow.SetSubtitle(locale.Get("None"))
	}

	// Sharee nodes have no page to go to.
	activatable := page.exitNode.Valid() && !tsutil.IsShareeNode(page.exitNode)
	page.ExitNodeRow.SetActivatable(activatable)
	page.ExitNodeIcon.SetVisible(activatable)
}

func (page *SelfPage) showExitNode() {
	if !page.exitNode.Valid() || page.app.win == nil {
		return
	}
	if tsutil.IsMullvad(page.exitNode) {
		page.app.win.ShowMullvadNode(page.exitNode.StableID())
		return
	}
	page.app.win.ShowPage(string(page.exitNode.StableID()))
}
