package ui

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"slices"

	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

const mullvadPageBaseName = "🟡 Mullvad Exit Nodes"

//go:embed mullvadpage.ui
var mullvadPageXML string

type MullvadPage struct {
	*adw.StatusPage `gtk:"Page"`

	ExitNodesGroup *adw.PreferencesGroup

	name string

	nodeLocationRows rowManager[[]*ipnstate.PeerStatus]
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

	page.nodeLocationRows.Parent = page.ExitNodesGroup
	page.nodeLocationRows.New = func(peers []*ipnstate.PeerStatus) row[[]*ipnstate.PeerStatus] {
		r := nodeLocationRow{
			w: adw.NewExpanderRow(),
		}
		r.m.Parent = rowAdderParent{r.w}
		r.m.New = func(peer *ipnstate.PeerStatus) row[*ipnstate.PeerStatus] {
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
				a.poller.Poll() <- struct{}{}
				return true
			})

			return &row
		}

		return &r
	}
}

func (page *MullvadPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.name = mullvadPageBaseName

	var exitNodeID tailcfg.StableNodeID
	if status.Status.ExitNodeStatus != nil {
		exitNodeID = status.Status.ExitNodeStatus.ID
	}

	nodes := make([]*ipnstate.PeerStatus, 0, len(status.Status.Peer))
	for _, peer := range status.Status.Peer {
		if tsutil.IsMullvad(peer) {
			nodes = append(nodes, peer)
			if peer.ID == exitNodeID {
				page.name = fmt.Sprintf("%v [%v]", mullvadPageBaseName, mullvadLocationName(peer.Location))
			}
		}
	}
	slices.SortFunc(nodes, tsutil.ComparePeers)

	type locID struct {
		CountryCode string
		CityCode    string
	}
	locs := xslices.ChunkBy(nodes, func(peer *ipnstate.PeerStatus) locID {
		return locID{peer.Location.CountryCode, peer.Location.CityCode}
	})

	page.nodeLocationRows.Update(locs)
}

type nodeLocationRow struct {
	w *adw.ExpanderRow
	m rowManager[*ipnstate.PeerStatus]
}

func (row *nodeLocationRow) Update(nodes []*ipnstate.PeerStatus) {
	loc := nodes[0].Location

	row.w.SetTitle(mullvadLocationName(loc))
	row.w.SetSubtitle("")
	for _, peer := range nodes {
		if peer.ExitNode {
			row.w.SetSubtitle("Current exit node location")
			break
		}
	}

	row.m.Update(nodes)
}

func (row *nodeLocationRow) Widget() gtk.Widgetter {
	return row.w
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

	row.w.SetTitle(peer.HostName)

	row.r().SetState(peer.ExitNode)
	row.r().SetActive(peer.ExitNode)
}

func (row *exitNodeRow) Widget() gtk.Widgetter {
	return row.w
}

func mullvadLocationName(loc *tailcfg.Location) string {
	return fmt.Sprintf(
		"%v %v, %v",
		countryCodeToFlag(loc.CountryCode),
		loc.City,
		loc.Country,
	)
}

func countryCodeToFlag(code string) string {
	var raw [2]rune
	for i, c := range code {
		raw[i] = 127397 + c
	}

	return string(raw[:])
}
