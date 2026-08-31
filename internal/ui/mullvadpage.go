package ui

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"iter"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/peersearch"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/tailcfg"
	"tailscale.com/util/set"
)

const mullvadPageBaseName = "Mullvad Exit Nodes"

//go:embed mullvadpage.ui
var mullvadPageXML string

type MullvadPage struct {
	app       *App
	stackPage *adw.ViewStackPage

	Page         *adw.StatusPage
	SearchEntry  *gtk.SearchEntry
	LocationList *gtk.ListBox

	locations    map[string]*adw.ExpanderRow
	exitNodes    map[tailcfg.StableNodeID]*mullvadExitNodeRow
	searchTokens []string
}

func NewMullvadPage(a *App, status *tsutil.IPNStatus) *MullvadPage {
	page := MullvadPage{
		app:       a,
		locations: make(map[string]*adw.ExpanderRow),
		exitNodes: make(map[tailcfg.StableNodeID]*mullvadExitNodeRow),
	}
	gutil.FillFromUI(&page, mullvadPageXML)

	page.LocationList.SetSortFunc(func(r1, r2 *gtk.ListBoxRow) int {
		return page.compareListRows(r1, r2)
	})
	page.LocationList.SetFilterFunc(func(row *gtk.ListBoxRow) bool {
		return page.listRowVisible(row)
	})
	placeholder := gtk.NewLabel("No matching exit nodes")
	placeholder.AddCSSClass("dim-label")
	page.LocationList.SetPlaceholder(placeholder)

	page.SearchEntry.ConnectSearchChanged(func() {
		page.setQuery(page.SearchEntry.Text())
	})

	return &page
}

func (page *MullvadPage) setQuery(q string) {
	tokens := strings.Fields(q)
	if slices.Equal(tokens, page.searchTokens) {
		return
	}
	page.searchTokens = tokens
	page.applySearch()
}

func (page *MullvadPage) searching() bool {
	return len(page.searchTokens) > 0
}

func (page *MullvadPage) listRowVisible(row *gtk.ListBoxRow) bool {
	if _, isNode := page.exitNodes[tailcfg.StableNodeID(row.Name())]; isNode {
		return page.searching()
	}
	return !page.searching()
}

func (page *MullvadPage) compareListRows(r1, r2 *gtk.ListBoxRow) int {
	if page.searching() {
		n1 := page.exitNodes[tailcfg.StableNodeID(r1.Name())]
		n2 := page.exitNodes[tailcfg.StableNodeID(r2.Name())]
		if n1 != nil && n2 != nil {
			return cmp.Or(
				cmp.Compare(n2.score, n1.score),
				strings.Compare(n1.row.Title(), n2.row.Title()),
				strings.Compare(r1.Name(), r2.Name()),
			)
		}
		return strings.Compare(r1.Name(), r2.Name())
	}
	return strings.Compare(page.expanderTitle(r1), page.expanderTitle(r2))
}

func (page *MullvadPage) expanderTitle(row *gtk.ListBoxRow) string {
	if loc := page.locations[row.Name()]; loc != nil {
		return loc.Title()
	}
	return row.Name()
}

func (page *MullvadPage) Widget() gtk.Widgetter {
	return page.Page
}

func (page *MullvadPage) Actions() gio.ActionGrouper {
	return nil
}

func (page *MullvadPage) Bind(stackPage *adw.ViewStackPage) {
	page.stackPage = stackPage
	stackPage.SetTitle(mullvadPageBaseName)
	stackPage.SetIconName("network-workgroup-symbolic")
}

func (page *MullvadPage) Update(s tsutil.Status) bool {
	status, ok := s.(*tsutil.IPNStatus)
	if !ok {
		return true
	}
	if !status.Online() {
		return false
	}

	self, ok := status.Self()
	if !ok {
		return true
	}
	if !tsutil.CanMullvad(self) {
		return false
	}

	var exitNodeID tailcfg.StableNodeID
	if exitNode := status.ExitNode(); exitNode.Valid() {
		exitNodeID = exitNode.StableID()
	}

	var exitNodeCountryCode string
	var exitLoc tailcfg.LocationView
	found := make(set.Set[tailcfg.StableNodeID])
	for id, peer := range status.Peers {
		if tsutil.IsMullvad(peer) {
			found.Add(id)
			exitNode := id == exitNodeID

			row := page.getExitNodeRow(peer)
			row.peer = peer
			sw := row.row.ActivatableWidget().(*gtk.Switch)
			sw.SetState(exitNode)
			sw.SetActive(exitNode)

			loc := peer.Hostinfo().Location()
			countryCode := loc.CountryCode()
			page.locations[countryCode].SetSubtitle("")

			if exitNode {
				exitNodeCountryCode = countryCode
				exitLoc = loc
			}
		}
	}
	for id, row := range page.exitNodes {
		if !found.Contains(id) {
			delete(page.exitNodes, id)
			page.removeNodeRow(row)
		}
	}

	if exitNodeCountryCode != "" {
		page.locations[exitNodeCountryCode].SetSubtitle("Current exit node location")
		page.stackPage.SetTitle(mullvadLongLocationName(exitLoc))
		page.stackPage.SetNeedsAttention(true)
	} else {
		page.stackPage.SetTitle(mullvadPageBaseName)
		page.stackPage.SetNeedsAttention(false)
	}

	page.applySearch()
	return true
}

func (page *MullvadPage) applySearch() {
	searching := page.searching()
	for _, node := range page.exitNodes {
		match := true
		score := 0
		if searching {
			score, match = peersearch.ScoreFields(mullvadSearchFields(node.peer), page.searchTokens)
		}
		node.score = score
		page.setNodeTitle(node, searching)
		page.setNodeFlat(node, searching && match)
	}
	page.LocationList.InvalidateFilter()
	page.LocationList.InvalidateSort()
}

func (page *MullvadPage) setNodeTitle(node *mullvadExitNodeRow, searching bool) {
	loc := node.peer.Hostinfo().Location()
	if searching {
		node.row.SetTitle(mullvadLongLocationName(loc))
		return
	}
	node.row.SetTitle(loc.City())
}

func (page *MullvadPage) setNodeFlat(node *mullvadExitNodeRow, flat bool) {
	if node.flat == flat {
		return
	}
	loc := page.locations[node.country]
	if flat {
		if loc != nil {
			loc.Remove(node.row)
		}
		page.LocationList.Append(node.row)
		node.flat = true
		return
	}
	page.LocationList.Remove(node.row)
	if loc != nil {
		loc.AddRow(node.row)
	}
	node.flat = false
}

func (page *MullvadPage) removeNodeRow(node *mullvadExitNodeRow) {
	if node.flat {
		page.LocationList.Remove(node.row)
	} else if loc := page.locations[node.country]; loc != nil {
		loc.Remove(node.row)
	}
	if page.countryHasNodes(node.country) {
		return
	}
	if loc := page.locations[node.country]; loc != nil {
		delete(page.locations, node.country)
		page.LocationList.Remove(loc)
	}
}

func (page *MullvadPage) countryHasNodes(code string) bool {
	for _, node := range page.exitNodes {
		if node.country == code {
			return true
		}
	}
	return false
}

func mullvadSearchFields(peer tailcfg.NodeView) iter.Seq[string] {
	return func(yield func(string) bool) {
		if !peer.Valid() {
			return
		}
		hi := peer.Hostinfo()
		if hi.Valid() {
			if !yield(hi.Hostname()) {
				return
			}
			if loc := hi.Location(); loc.Valid() {
				city := loc.City()
				for _, s := range []string{city, loc.Country(), loc.CountryCode(), loc.CityCode(), usStateName(city)} {
					if s == "" {
						continue
					}
					if !yield(s) {
						return
					}
				}
			}
		}
		yield(strings.TrimSuffix(peer.Name(), "."))
	}
}

func (page *MullvadPage) getLocationRow(loc tailcfg.LocationView) *adw.ExpanderRow {
	if row, ok := page.locations[loc.CountryCode()]; ok {
		return row
	}

	row := adw.NewExpanderRow()
	row.SetName(loc.CountryCode())
	row.SetTitle(mullvadLocationName(loc))
	gutil.ExpanderRowListBox(row).SetSortFunc(func(r1, r2 *gtk.ListBoxRow) int {
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

	page.locations[loc.CountryCode()] = row
	page.LocationList.Append(row)
	return row
}

func (page *MullvadPage) getExitNodeRow(peer tailcfg.NodeView) *mullvadExitNodeRow {
	if row, ok := page.exitNodes[peer.StableID()]; ok {
		return row
	}

	info := peer.Hostinfo()

	row := adw.NewSwitchRow()
	row.SetName(string(peer.StableID()))
	row.SetTitle(info.Location().City())
	row.SetSubtitle(info.Hostname())

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

		var node tailcfg.StableNodeID
		if s {
			node = peer.StableID()
		}
		err := tsutil.ExitNode(context.TODO(), node)
		if err != nil {
			slog.Error("set exit node", "err", err)
			sw.SetActive(!s)
			return true
		}
		return true
	})

	page.getLocationRow(info.Location()).AddRow(row)

	exitNodeRow := mullvadExitNodeRow{
		country: info.Location().CountryCode(),
		row:     row,
		peer:    peer,
	}
	page.exitNodes[peer.StableID()] = &exitNodeRow
	return &exitNodeRow
}

type mullvadExitNodeRow struct {
	country string
	row     *adw.SwitchRow
	peer    tailcfg.NodeView
	score   int
	flat    bool
}

func mullvadLongLocationName(loc tailcfg.LocationView) string {
	return fmt.Sprintf(
		"%v %v, %v",
		countryCodeToFlag(loc.CountryCode()),
		loc.City(),
		loc.Country(),
	)
}

func mullvadLocationName(loc tailcfg.LocationView) string {
	return fmt.Sprintf(
		"%v %v",
		countryCodeToFlag(loc.CountryCode()),
		loc.Country(),
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

func usStateName(city string) string {
	_, code := splitCityState(city)
	return usStateNames[code]
}
