package ui

import (
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

// Page represents the UI for a single page of the app. This usually
// corresponds to information about a specific peer in the tailnet.
type Page interface {
	// Root returns the root widget that is can be placed into a container.
	Root() gtk.Widgetter

	// Name returns a displayable name for the page.
	Name() string

	// Init performs first-time initialization of the page, i.e. setting
	// values to their defaults and whatnot. It should not call Update
	// unless doing so is idempotent, though even then it's better not
	// to.
	Init(*App, *ipnstate.PeerStatus, tsutil.Status)

	// Update performs an update of the UI to match new state.
	Update(*App, *ipnstate.PeerStatus, tsutil.Status)
}

type stackPage struct {
	page      Page
	stackPage *gtk.StackPage
}

func (page *stackPage) Init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.page.Init(a, peer, status)

	page.stackPage = a.win.PeersStack.AddTitled(
		page.page.Root(),
		peer.PublicKey.String(),
		page.page.Name(),
	)
}

func (page *stackPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.stackPage.SetIconName(peerIcon(peer))
	page.stackPage.SetTitle(page.page.Name())

	page.page.Update(a, peer, status)
}
