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

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/peersearch"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/tailcfg"
	"tailscale.com/util/set"
)

//go:embed mullvadpage.ui
var mullvadPageXML string

type MullvadPage struct {
	app       *App
	stackPage *adw.ViewStackPage

	Page         *adw.StatusPage
	SearchEntry  *gtk.SearchEntry
	LocationList *gtk.ListBox
	EmptyLabel   *gtk.Label

	locations    map[string]*adw.ExpanderRow // Each holds a reference.
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

	setSortFunc(page.LocationList, page.compareListRows)
	setFilterFunc(page.LocationList, page.listRowVisible)

	gutil.Connect(page.SearchEntry, page.SearchEntry.ConnectSearchChanged, func(gtk.SearchEntry) {
		page.setQuery(page.SearchEntry.GetText())
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
	if node := page.exitNodes[tailcfg.StableNodeID(row.GetName())]; node != nil {
		return page.searching() && node.match
	}
	return !page.searching()
}

func (page *MullvadPage) compareListRows(r1, r2 *gtk.ListBoxRow) int {
	if page.searching() {
		n1 := page.exitNodes[tailcfg.StableNodeID(r1.GetName())]
		n2 := page.exitNodes[tailcfg.StableNodeID(r2.GetName())]
		if n1 != nil && n2 != nil {
			return cmp.Or(
				cmp.Compare(n2.score, n1.score),
				strings.Compare(n1.row.GetTitle(), n2.row.GetTitle()),
				strings.Compare(r1.GetName(), r2.GetName()),
			)
		}
		return strings.Compare(r1.GetName(), r2.GetName())
	}
	return strings.Compare(page.expanderTitle(r1), page.expanderTitle(r2))
}

func (page *MullvadPage) expanderTitle(row *gtk.ListBoxRow) string {
	if loc := page.locations[row.GetName()]; loc != nil {
		return loc.GetTitle()
	}
	return row.GetName()
}

func (page *MullvadPage) Widget() *gtk.Widget {
	return &page.Page.Widget
}

func (page *MullvadPage) Actions() *gio.SimpleActionGroup {
	return nil
}

func (page *MullvadPage) Destroy() {
	for _, node := range page.exitNodes {
		node.row.Unref()
	}
	clear(page.exitNodes)
	for _, loc := range page.locations {
		loc.Unref()
	}
	clear(page.locations)
	gutil.UnrefFields(page)
}

func (page *MullvadPage) Bind(stackPage *adw.ViewStackPage) {
	page.stackPage = stackPage
	stackPage.SetTitle(locale.Get("Mullvad Exit Nodes"))
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
	nodesChanged := false
	for id, peer := range status.Peers {
		if tsutil.IsMullvad(peer) {
			found.Add(id)
			exitNode := id == exitNodeID

			_, existed := page.exitNodes[id]
			row := page.getExitNodeRow(peer)
			if !existed {
				nodesChanged = true
			}
			row.peer = peer
			setSwitch(row.row, exitNode)

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
			nodesChanged = true
		}
	}

	if exitNodeCountryCode != "" {
		page.locations[exitNodeCountryCode].SetSubtitle(locale.Get("Current exit node location"))
		page.stackPage.SetTitle(mullvadLongLocationName(exitLoc))
		page.stackPage.SetNeedsAttention(true)
	} else {
		page.stackPage.SetTitle(locale.Get("Mullvad Exit Nodes"))
		page.stackPage.SetNeedsAttention(false)
	}

	if nodesChanged {
		page.applySearch()
	}
	return true
}

func (page *MullvadPage) applySearch() {
	searching := page.searching()
	anyMatch := false
	for _, node := range page.exitNodes {
		match := true
		score := 0
		if searching {
			score, match = peersearch.ScoreFields(mullvadSearchFields(node.peer), page.searchTokens)
		}
		node.score = score
		node.match = match
		if match {
			anyMatch = true
		}
		page.setNodeTitle(node, searching)
		page.setNodeFlat(node, searching)
	}

	empty := searching && !anyMatch
	page.LocationList.SetVisible(!empty)
	page.EmptyLabel.SetVisible(empty)
	if empty {
		return
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
			loc.Remove(&node.row.Widget)
		}
		page.LocationList.Append(&node.row.Widget)
		node.flat = true
		return
	}
	page.LocationList.Remove(&node.row.Widget)
	if loc != nil {
		loc.AddRow(&node.row.Widget)
	}
	node.flat = false
}

func (page *MullvadPage) removeNodeRow(node *mullvadExitNodeRow) {
	if node.flat {
		page.LocationList.Remove(&node.row.Widget)
	} else if loc := page.locations[node.country]; loc != nil {
		loc.Remove(&node.row.Widget)
	}
	node.row.Unref()
	if page.countryHasNodes(node.country) {
		return
	}
	if loc := page.locations[node.country]; loc != nil {
		delete(page.locations, node.country)
		page.LocationList.Remove(&loc.Widget)
		loc.Unref()
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
	setSortFunc(gutil.ExpanderRowListBox(row), func(r1, r2 *gtk.ListBoxRow) int {
		sw1 := gutil.MustCast[adw.SwitchRow](r1, adw.SwitchRowGLibType())
		sw2 := gutil.MustCast[adw.SwitchRow](r2, adw.SwitchRowGLibType())
		c1, s1 := splitCityState(sw1.GetTitle())
		c2, s2 := splitCityState(sw2.GetTitle())
		return cmp.Or(
			strings.Compare(s1, s2),
			strings.Compare(c1, c2),
			strings.Compare(sw1.GetSubtitle(), sw2.GetSubtitle()),
		)
	})

	page.locations[loc.CountryCode()] = row
	page.LocationList.Append(&row.Widget)
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

	sw := rowSwitch(row)
	sw.SetMarginTop(12)
	sw.SetMarginBottom(12)
	gutil.Connect(sw, sw.ConnectStateSet, func(_ gtk.Switch, s bool) bool {
		if s == sw.GetState() {
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

	locRow := page.getLocationRow(info.Location())
	node := &mullvadExitNodeRow{
		country: info.Location().CountryCode(),
		row:     row,
		peer:    peer,
		flat:    page.searching(),
	}
	if node.flat {
		page.LocationList.Append(&row.Widget)
	} else {
		locRow.AddRow(&row.Widget)
	}
	page.exitNodes[peer.StableID()] = node
	return node
}

type mullvadExitNodeRow struct {
	country string
	row     *adw.SwitchRow // Holds a reference.
	peer    tailcfg.NodeView
	score   int
	match   bool
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
