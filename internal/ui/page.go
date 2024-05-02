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

func NewPage(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) Page {
	if peer.PublicKey == status.Status.Self.PublicKey {
		page := NewSelfPage()
		page.Init(a, peer, status)
		return page
	}
}

type pageInfo struct {
	stackPage *gtk.StackPage
	page      Page
}

func (page *pageInfo) Root() gtk.Widgetter {
	return page.page.Root()
}

func (page *pageInfo) Init(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	page.page.Init(a, peer, status)
}

func (page *pageInfo) Update(a *App, peer *ipnstate.PeerStatus, status tsutil.Status) {
	self := peer.PublicKey == status.Status.Self.PublicKey

	page.stackPage.SetIconName(peerIcon(peer))
	page.stackPage.SetTitle(peerName(status, peer, self))

	page.page.Update(a, peer, status)
}
