package ui

import (
	"context"
	"slices"

	"deedles.dev/mk"
	"deedles.dev/trayscale/internal/tsutil"
	"deedles.dev/trayscale/internal/xslices"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

type state struct {
	a       *App
	updates chan tsutil.Status

	online    bool
	onlinereq chan bool

	peerPages map[key.NodePublic]*peerPage
}

func newState(a *App) *state {
	return &state{
		a:         a,
		updates:   make(chan tsutil.Status),
		onlinereq: make(chan bool),
		peerPages: make(map[key.NodePublic]*peerPage),
	}
}

func (s *state) update(status tsutil.Status) {
	online := status.Online()
	s.a.tray.Update(status, s.online)
	if s.online != online {
		s.online = online

		body := "Tailscale is not connected."
		if online {
			body = "Tailscale is connected."
		}
		glib.IdleAdd(func() {
			s.a.notify("Tailscale Status", body) // TODO: Notify on startup if not connected?
		})
	}
	if s.a.win == nil {
		return
	}

	glib.IdleAdd(func() {
		s.a.win.StatusSwitch.SetState(online)
		s.a.win.StatusSwitch.SetActive(online)
	})

	s.updatePeers(status)

	if s.a.settings != nil {
		glib.IdleAdd(func() {
			controlURL := s.a.settings.String("control-plane-server")
			if controlURL == "" {
				controlURL = ipn.DefaultControlURL
			}
			if controlURL != status.Prefs.ControlURL {
				s.a.settings.SetString("control-plane-server", status.Prefs.ControlURL)
			}
		})
	}

	if s.online && !s.a.operatorCheck {
		s.a.operatorCheck = true
		if !status.OperatorIsCurrent() {
			glib.IdleAdd(func() {
				Info{
					Heading: "User is not Tailscale Operator",
					Body:    "Some functionality may not work as expected. To resolve, run\n<tt>sudo tailscale set --operator=$USER</tt>\nin the command-line.",
				}.Show(s.a, nil)
			})
		}
	}
}

func (s *state) updatePeers(status tsutil.Status) {
	const statusPageName = "status"

	w := s.a.win.PeersStack

	var peerMap map[key.NodePublic]*ipnstate.PeerStatus
	var peers []key.NodePublic

	if status.Online() {
		glib.IdleAdd(func() {
			if c := w.ChildByName(statusPageName); c != nil {
				w.Remove(c)
			}
		})

		peerMap = status.Status.Peer
		if peerMap == nil {
			mk.Map(&peerMap, 0)
		}

		peers = slices.Insert(status.Status.Peers(), 0, status.Status.Self.PublicKey) // Add this manually to guarantee ordering.
		peerMap[status.Status.Self.PublicKey] = status.Status.Self
	}

	oldPeers, newPeers := xslices.Partition(peers, func(peer key.NodePublic) bool {
		_, ok := s.peerPages[peer]
		return ok
	})

	for id, page := range s.peerPages {
		_, ok := peerMap[id]
		if !ok {
			glib.IdleAdd(func() {
				w.Remove(page.container)
			})
			delete(s.peerPages, id)
		}
	}

	for _, p := range newPeers {
		peerStatus := peerMap[p]
		c := make(chan *peerPage)
		glib.IdleAdd(func() {
			peerPage := s.a.newPeerPage(status, peerStatus)
			peerPage.page = w.AddTitled(
				peerPage.container,
				p.String(),
				peerName(status, peerStatus, peerPage.self),
			)
			s.a.updatePeerPage(peerPage, peerStatus, status)
			c <- peerPage
		})
		s.peerPages[p] = <-c
	}

	for _, p := range oldPeers {
		page := s.peerPages[p]
		glib.IdleAdd(func() {
			s.a.updatePeerPage(page, peerMap[p], status)
		})
	}

	glib.IdleAdd(func() {
		if w.Pages().NItems() == 0 {
			w.AddTitled(s.a.statusPage, statusPageName, "Not Connected")
		}
	})
}

func (s *state) Updates() chan<- tsutil.Status {
	return s.updates
}

func (s *state) Online() <-chan bool {
	return s.onlinereq
}

func (s *state) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)

		case status := <-s.updates:
			s.update(status)

		case s.onlinereq <- s.online:
		}
	}
}
