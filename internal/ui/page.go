package ui

import (
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"tailscale.com/ipn/ipnstate"
)

type Page interface {
	Root() gtk.Widgetter

	Init(*App, *ipnstate.PeerStatus, tsutil.Status)
	Update(*App, *ipnstate.PeerStatus, tsutil.Status)
}

func NewPage(peer *ipnstate.PeerStatus, status tsutil.Status) Page {
	if peer.PublicKey == status.Status.Self.PublicKey {
		return NewSelfPage()
	}
	return NewPeerPage()
}

type pageInfo struct {
	page      Page
	stackPage *gtk.StackPage
}

func (page *pageInfo) Root() gtk.Widgetter {
	return page.page.Root()
}

func (page *pageInfo) Init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.stackPage = a.win.PeersStack.AddTitled(
		page.Root(),
		peer.PublicKey.String(),
		peerName(status, peer, peer.PublicKey == status.Status.Self.PublicKey),
	)

	page.page.Init(a, peer, status)
}

func (page *pageInfo) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	self := peer.PublicKey == status.Status.Self.PublicKey

	page.stackPage.SetIconName(peerIcon(peer))
	page.stackPage.SetTitle(peerName(status, peer, self))

	page.page.Update(a, peer, status)
}
