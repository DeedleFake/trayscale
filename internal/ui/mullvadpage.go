package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

//go:embed mullvadpage.ui
var mullvadPageXML string

type MullvadPage struct {
	*adw.StatusPage `gtk:"Page"`

	ExitNodesGroup *adw.PreferencesGroup
}

func NewMullvadPage() *MullvadPage {
	var page MullvadPage
	fillFromBuilder(&page, mullvadPageXML)
	return &page
}

func (page *MullvadPage) Root() gtk.Widgetter {
	return page.StatusPage
}

func (page *MullvadPage) ID() string {
	return "mullvad"
}

func (page *MullvadPage) Name() string {
	return "Mullvad Exit Nodes"
}

func (page *MullvadPage) Init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
}

func (page *MullvadPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
}
