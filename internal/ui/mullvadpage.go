package ui

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
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

	locations map[string]*adw.ExpanderRow
	exitNodes map[tailcfg.StableNodeID]*mullvadExitNodeRow
}

func NewMullvadPage(a *App, status *tsutil.NetStatus) *MullvadPage {
	page := MullvadPage{
		app:       a,
		locations: make(map[string]*adw.ExpanderRow),
		exitNodes: make(map[tailcfg.StableNodeID]*mullvadExitNodeRow),
	}
	fillFromBuilder(&page, mullvadPageXML)

	page.LocationList.SetSortFunc(func(r1, r2 *gtk.ListBoxRow) int {
		e1 := r1.Cast().(*adw.ExpanderRow)
		e2 := r2.Cast().(*adw.ExpanderRow)
		return strings.Compare(e1.Title(), e2.Title())
	})

	return &page
}

func (page *MullvadPage) Widget() gtk.Widgetter {
	return page.Page
}

func (page *MullvadPage) Actions() gio.ActionGrouper {
	return nil
}

func (page *MullvadPage) Init(row *PageRow) {
	page.row = row
	row.SetTitle(mullvadPageBaseName)
}

func (page *MullvadPage) Update(s tsutil.Status) bool {
	status, ok := s.(*tsutil.NetStatus)
	if !ok {
		return true
	}
	if !status.Online() {
		return false
	}

	if !tsutil.CanMullvad(status.Status.Self) {
		return false
	}

	var subtitle string
	icon := "network-workgroup-symbolic"

	var exitNodeID tailcfg.StableNodeID
	if status.Status.ExitNodeStatus != nil {
		exitNodeID = status.Status.ExitNodeStatus.ID
	}

	var exitNodeCountryCode string
	found := make(set.Set[tailcfg.StableNodeID])
	for _, peer := range status.Status.Peer {
		if tsutil.IsMullvad(peer) {
			found.Add(peer.ID)
			exitNode := peer.ID == exitNodeID

			row := page.getExitNodeRow(peer)
			sw := row.row.ActivatableWidget().(*gtk.Switch)
			sw.SetState(exitNode)
			sw.SetActive(exitNode)

			if exitNode {
				subtitle = mullvadLongLocationName(peer.Location)
				icon = "network-vpn-symbolic"
				exitNodeCountryCode = peer.Location.CountryCode
			}
			page.locations[peer.Location.CountryCode].SetSubtitle("")
		}
	}
	for id, row := range page.exitNodes {
		if !found.Contains(id) {
			delete(page.exitNodes, id)

			locRow := page.locations[row.country]
			locRow.Remove(row.row)
			if locRow.HasCSSClass("empty") {
				delete(page.locations, row.country)
				page.LocationList.Remove(locRow)
			}
		}
	}

	page.row.SetSubtitle(subtitle)
	page.row.SetIconName(icon)
	if exitNodeCountryCode != "" {
		page.locations[exitNodeCountryCode].SetSubtitle("Current exit node location")
	}

	return true
}

func (page *MullvadPage) getLocationRow(loc *tailcfg.Location) *adw.ExpanderRow {
	if row, ok := page.locations[loc.CountryCode]; ok {
		return row
	}

	row := adw.NewExpanderRow()
	row.SetTitle(mullvadLocationName(loc))
	expanderRowListBox(row).SetSortFunc(func(r1, r2 *gtk.ListBoxRow) int {
		sw1 := r1.Cast().(*adw.SwitchRow)
		sw2 := r2.Cast().(*adw.SwitchRow)
		c1, s1 := splitCityState(sw1.Title())
		c2, s2 := splitCityState(sw2.Title())
		return cmp.Or(
			strings.Compare(s1, s2),
			strings.Compare(c1, c2),
			strings.Compare(sw1.Subtitle(), sw2.Subtitle()),
		)
	})

	page.locations[loc.CountryCode] = row
	page.LocationList.Append(row)
	return row
}

func (page *MullvadPage) getExitNodeRow(peer *ipnstate.PeerStatus) *mullvadExitNodeRow {
	if row, ok := page.exitNodes[peer.ID]; ok {
		return row
	}

	row := adw.NewSwitchRow()
	row.SetTitle(peer.Location.City)
	row.SetSubtitle(peer.HostName)

	sw := row.ActivatableWidget().(*gtk.Switch)
	sw.SetMarginTop(12)
	sw.SetMarginBottom(12)
	sw.ConnectStateSet(func(s bool) bool {
		if s == sw.State() {
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
			sw.SetActive(!s)
			return true
		}
		<-page.app.poller.Poll()
		return true
	})

	page.getLocationRow(peer.Location).AddRow(row)

	exitNodeRow := mullvadExitNodeRow{
		country: peer.Location.CountryCode,
		row:     row,
	}
	page.exitNodes[peer.ID] = &exitNodeRow
	return &exitNodeRow
}

type mullvadExitNodeRow struct {
	country string
	row     *adw.SwitchRow
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

func countryCodeToFlag(code string) string {
	var raw [2]rune
	for i, c := range code {
		raw[i] = 127397 + c
	}

	return string(raw[:])
}

var cityStateRE = regexp.MustCompile(`^(.*),?\s+([A-Z]{2})$`)

func splitCityState(str string) (city, state string) {
	parts := cityStateRE.FindStringSubmatch(str)
	if len(parts) == 0 {
		return str, ""
	}
	return parts[1], parts[2]
}
