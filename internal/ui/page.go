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

	// Init performs first-time initialization of the page, i.e. setting
	// values to their defaults and whatnot. It should not call Update
	// unless doing so is idempotent, though even then it's better not
	// to.
	Init(*App, *ipnstate.PeerStatus, tsutil.Status)

	// Update performs an update of the UI to match new state.
	Update(*App, *ipnstate.PeerStatus, tsutil.Status)
}

// NewPage returns an instance of page that represents the given peer.
func NewPage(peer *ipnstate.PeerStatus, status tsutil.Status) Page {
	if peer.PublicKey == status.Status.Self.PublicKey {
		return NewSelfPage()
	}
	return NewPeerPage()
}

type stackPage struct {
	page      Page
	stackPage *gtk.StackPage
}

func (page *stackPage) Root() gtk.Widgetter {
	return page.page.Root()
}

func (page *stackPage) Init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.stackPage = a.win.PeersStack.AddTitled(
		page.Root(),
		peer.PublicKey.String(),
		peerName(status, peer, peer.PublicKey == status.Status.Self.PublicKey),
	)

	page.page.Init(a, peer, status)
}

func (page *stackPage) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	self := peer.PublicKey == status.Status.Self.PublicKey

	page.stackPage.SetIconName(peerIcon(peer))
	page.stackPage.SetTitle(peerName(status, peer, self))

	page.page.Update(a, peer, status)
}
