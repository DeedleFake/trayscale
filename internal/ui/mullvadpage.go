package ui

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"slices"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

var (
	locationListModel = gioutil.NewListModelType[[]*ipnstate.PeerStatus]()
	peerStatusModel   = gioutil.NewListModelType[*ipnstate.PeerStatus]()
)

const mullvadPageBaseName = "🟡 Mullvad Exit Nodes"

//go:embed mullvadpage.ui
var mullvadPageXML string

type MullvadPage struct {
	*adw.StatusPage `gtk:"Page"`

	LocationList *gtk.ListBox

	name string

	locationsModel *gioutil.ListModel[[]*ipnstate.PeerStatus]
	nodeModels     []*gioutil.ListModel[*ipnstate.PeerStatus]

	// These are used to cache some intermediate variables between
	// updates to cut down on the number of necessary allocations.
	nodes []*ipnstate.PeerStatus
	locs  [][]*ipnstate.PeerStatus
}

func NewMullvadPage(a *App, status tsutil.Status) *MullvadPage {
	var page MullvadPage
	fillFromBuilder(&page, mullvadPageXML)
	page.init(a, status)
	return &page
}

func (page *MullvadPage) Root() gtk.Widgetter {
	return page.StatusPage
}

func (page *MullvadPage) ID() string {
	return "mullvad"
}

func (page *MullvadPage) Name() string {
	return page.name
}

func (page *MullvadPage) init(a *App, status tsutil.Status) {
	page.name = mullvadPageBaseName

	page.locationsModel = locationListModel.New()
	page.LocationList.BindModel(page.locationsModel, func(obj *glib.Object) gtk.Widgetter {
		peers := locationListModel.ObjectValue(obj)

		expander := adw.NewExpanderRow()
		expander.SetTitle(mullvadLocationName(peers[0].Location))

		for _, peer := range peers {
			node := adw.NewActionRow()
			node.SetTitle(peer.HostName)

			r := gtk.NewSwitch()

			r.SetMarginTop(12)
			r.SetMarginBottom(12)
			r.ConnectStateSet(func(s bool) bool {
				if s == r.State() {
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
					node = peer
				}
				err := tsutil.ExitNode(context.TODO(), node)
				if err != nil {
					slog.Error("set exit node", "err", err)
					r.SetActive(!s)
					return true
				}
				a.poller.Poll() <- struct{}{}
				return true
			})

			expander.AddRow(node)
		}

		row := gtk.NewListBoxRow()
		row.SetChild(expander)

		return row

		//row := gtk.NewListBoxRow()
		//row.SetChild(w)
		//return row
	})
}

func (page *MullvadPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.name = mullvadPageBaseName

	var exitNodeID tailcfg.StableNodeID
	if status.Status.ExitNodeStatus != nil {
		exitNodeID = status.Status.ExitNodeStatus.ID
	}

	for _, peer := range status.Status.Peer {
		if tsutil.IsMullvad(peer) {
			page.nodes = append(page.nodes, peer)
			if peer.ID == exitNodeID {
				page.name = fmt.Sprintf("%v [%v]", mullvadPageBaseName, mullvadLongLocationName(peer.Location))
			}
		}
	}
	slices.SortFunc(page.nodes, tsutil.ComparePeers)

	page.locs = page.locs[:0]
	page.locs = slices.AppendSeq(page.locs, xiter.SliceChunksFunc(page.nodes, func(peer *ipnstate.PeerStatus) string {
		return peer.Location.CountryCode
	}))

	page.locationsModel.Splice(0, page.locationsModel.Len(), page.locs...)

	clear(page.nodes)
	page.nodes = page.nodes[:0]
}

func mullvadLongLocationName(loc *tailcfg.Location) string {
	return fmt.Sprintf(
		"%v %v, %v",
		countryCodeToFlag(loc.CountryCode),
		loc.City,
		loc.Country,
	)
}

func mullvadLocationName(loc *tailcfg.Location) string {
	return fmt.Sprintf(
		"%v %v",
		countryCodeToFlag(loc.CountryCode),
		loc.Country,
	)
}

func mullvadNodeName(peer *ipnstate.PeerStatus) string {
	if peer.Location == nil {
		return peer.HostName
	}

	return fmt.Sprintf("%v (%v)", peer.Location.City, peer.HostName)
}

func countryCodeToFlag(code string) string {
	var raw [2]rune
	for i, c := range code {
		raw[i] = 127397 + c
	}

	return string(raw[:])
}
