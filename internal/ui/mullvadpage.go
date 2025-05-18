package ui

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/xiter"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/util/set"
)

const mullvadPageBaseName = "Mullvad Exit Nodes"

//go:embed mullvadpage.ui
var mullvadPageXML string

type MullvadPage struct {
	app *App
	row *PageRow

	Page         *adw.StatusPage
	LocationList *gtk.ListBox

	rows map[city]*locationRow

	// These are used to cache some intermediate variables between
	// updates to cut down on the number of necessary allocations.
	nodes []*ipnstate.PeerStatus
	locs  [][]*ipnstate.PeerStatus
	found set.Set[*locationRow]
}

func NewMullvadPage(a *App, status tsutil.Status) *MullvadPage {
	page := MullvadPage{
		rows:  make(map[city]*locationRow),
		found: make(set.Set[*locationRow]),
	}
	fillFromBuilder(&page, mullvadPageXML)
	page.init(a, status)
	return &page
}

func (page *MullvadPage) init(a *App, status tsutil.Status) {
	page.app = a

	page.LocationList.SetSortFunc(func(r1, r2 *gtk.ListBoxRow) int {
		e1 := r1.Cast().(*adw.ExpanderRow)
		e2 := r2.Cast().(*adw.ExpanderRow)
		return strings.Compare(e1.Title(), e2.Title())
	})
}

func (page *MullvadPage) Widget() gtk.Widgetter {
	return page.Page
}

func (page *MullvadPage) Init(row *PageRow) {
	page.row = row
	row.SetTitle(mullvadPageBaseName)
}

func (page *MullvadPage) Update(status tsutil.Status) bool {
	if !tsutil.CanMullvad(status.Status.Self) {
		return false
	}

	var subtitle string
	icon := "network-workgroup-symbolic"

	var exitNodeID tailcfg.StableNodeID
	if status.Status.ExitNodeStatus != nil {
		exitNodeID = status.Status.ExitNodeStatus.ID
	}

	for _, peer := range status.Status.Peer {
		if tsutil.IsMullvad(peer) {
			page.nodes = append(page.nodes, peer)
			if peer.ID == exitNodeID {
				subtitle = mullvadLongLocationName(peer.Location)
				icon = "network-vpn-symbolic"
			}
		}
	}
	slices.SortFunc(page.nodes, tsutil.ComparePeers)

	clear(page.locs)
	page.locs = slices.AppendSeq(page.locs[:0], xiter.SliceChunksFunc(page.nodes, func(peer *ipnstate.PeerStatus) string {
		return peer.Location.CountryCode
	}))

	for _, peers := range page.locs {
		row := page.getRow(peers[0].Location)
		page.found.Add(row)
		row.Update(peers)
	}
	for city, row := range page.rows {
		if !page.found.Contains(row) {
			delete(page.rows, city)
			page.LocationList.Remove(row.Row)
		}
	}
	clear(page.found)

	clear(page.nodes)
	page.nodes = page.nodes[:0]

	page.row.SetSubtitle(subtitle)
	page.row.SetIconName(icon)

	return true
}

func (page *MullvadPage) getRow(loc *tailcfg.Location) *locationRow {
	city := cityFromLocation(loc)
	if row, ok := page.rows[city]; ok {
		return row
	}

	erow := adw.NewExpanderRow()
	erow.SetTitle(mullvadLocationName(loc))

	lrow := locationRow{
		Row: erow,
	}

	lrow.Manager.Parent = rowAdderParent{erow}
	lrow.Manager.New = func(peer *ipnstate.PeerStatus) row[*ipnstate.PeerStatus] {
		row := exitNodeRow{
			peer: peer,

			w: adw.NewSwitchRow(),
		}

		row.w.SetTitle(peer.HostName)

		row.r().SetMarginTop(12)
		row.r().SetMarginBottom(12)
		row.r().ConnectStateSet(func(s bool) bool {
			if s == row.r().State() {
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
				node = row.peer
			}
			err := tsutil.ExitNode(context.TODO(), node)
			if err != nil {
				slog.Error("set exit node", "err", err)
				row.r().SetActive(!s)
				return true
			}
			page.app.poller.Poll() <- struct{}{}
			return true
		})

		return &row
	}

	page.rows[city] = &lrow
	page.LocationList.Append(erow)
	return &lrow
}

type city struct {
	Country string
	City    string
}

func cityFromLocation(loc *tailcfg.Location) city {
	return city{
		Country: loc.CountryCode,
		City:    loc.City,
	}
}

type locationRow struct {
	Row     *adw.ExpanderRow
	Manager rowManager[*ipnstate.PeerStatus]
}

func (row *locationRow) Update(nodes []*ipnstate.PeerStatus) {
	row.Row.SetSubtitle("")
	for _, peer := range nodes {
		if peer.ExitNode {
			row.Row.SetSubtitle("Current exit node location")
			break
		}
	}

	row.Manager.Update(nodes)
}

func (row *locationRow) Widget() gtk.Widgetter {
	return row.Row
}

type exitNodeRow struct {
	peer *ipnstate.PeerStatus

	w *adw.SwitchRow
}

func (row *exitNodeRow) r() *gtk.Switch {
	return row.w.ActivatableWidget().(*gtk.Switch)
}

func (row *exitNodeRow) Update(peer *ipnstate.PeerStatus) {
	row.peer = peer

	row.w.SetTitle(mullvadNodeName(peer))

	row.r().SetState(peer.ExitNode)
	row.r().SetActive(peer.ExitNode)
}

func (row *exitNodeRow) Widget() gtk.Widgetter {
	return row.w
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
