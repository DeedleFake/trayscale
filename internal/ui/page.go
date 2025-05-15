package ui

import (
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

// Page represents the UI for a single page of the app. This usually
// corresponds to information about a specific peer in the tailnet.
type Page interface {
	// Root returns the root widget that is can be placed into a container.
	Root() gtk.Widgetter

	// An identifier for the page.
	ID() string

	// Name returns a displayable name for the page.
	Name() string

	// Icon returns the name of an icon for the page.
	Icon() string

	// Update performs an update of the UI to match new state.
	Update(*App, *ipnstate.PeerStatus, tsutil.Status)
}

type stackPage struct {
	page      Page
	stackPage *adw.ViewStackPage
}

func newStackPage(a *App, page Page) *stackPage {
	return &stackPage{
		page: page,
		stackPage: a.win.PeersStack.AddTitled(
			page.Root(),
			page.ID(),
			page.Name(),
		),
	}
}

func (page *stackPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.page.Update(a, peer, status)

	page.stackPage.SetTitle(page.page.Name())
	page.stackPage.SetIconName(page.page.Icon())
}
