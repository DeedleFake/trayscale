package ui

import (
	"fmt"
	"net/netip"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

// TestPeerPageLeaks opens, updates, and destroys peer pages repeatedly
// and checks that callback slots stay flat and every page is finalized.
func TestPeerPageLeaks(t *testing.T) {
	requireGTK(t)

	peerAt := func(i int) tailcfg.NodeView {
		v := layoutPeer(t, "peer", "peer", true, i%2 == 0, false)
		n := v.AsStruct()
		for j := range i % 4 {
			n.Addresses = append(n.Addresses, netip.MustParsePrefix(fmt.Sprintf("100.64.0.%d/32", j+1)))
			n.PrimaryRoutes = append(n.PrimaryRoutes, netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", j)))
		}
		return n.View()
	}

	status := func(peer tailcfg.NodeView) *tsutil.IPNStatus {
		s := layoutStatus([]tailcfg.NodeView{peer})
		s.Prefs = ipn.NewPrefs().View()
		return s
	}

	stack := adw.NewViewStack()
	defer stack.Unref()

	var finalized int
	cycle := func(i int) {
		peer := peerAt(i)
		page := NewPeerPage(&App{}, status(peer), peer)
		gutil.OnFinalize(page.Page, func() { finalized++ })
		vp := stack.AddNamed(page.Widget(), "peer")
		page.Bind(vp)
		for j := range 10 {
			p := peerAt(i + j)
			page.Update(status(p))
		}
		stack.Remove(page.Widget())
		vp.Unref()
		page.Destroy()
	}

	const warmup, n = 5, 200
	for i := range warmup {
		cycle(i)
	}
	before := gutil.FreeCallbackSlots()
	for i := range n {
		cycle(i)
	}
	after := gutil.FreeCallbackSlots()
	t.Logf("free callback slots: before=%d after=%d; pages finalized %d/%d", before, after, finalized, warmup+n)
	if after < before {
		t.Errorf("leaked %d callback slots over %d page cycles", before-after, n)
	}
	if finalized != warmup+n {
		t.Errorf("finalized %d of %d pages", finalized, warmup+n)
	}
}

// TestSelfPageLeaks does the same for the self page, cycling incoming
// file rows (each with its own buttons and handlers).
func TestSelfPageLeaks(t *testing.T) {
	requireGTK(t)

	stack := adw.NewViewStack()
	defer stack.Unref()

	files := func(i int) *tsutil.FileStatus {
		var s tsutil.FileStatus
		for j := range i % 5 {
			s.Files = append(s.Files, apitype.WaitingFile{Name: fmt.Sprintf("file-%d", j), Size: int64(j * 1000)})
		}
		return &s
	}

	var finalized int
	cycle := func(i int) {
		page := NewSelfPage(&App{}, &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()})
		gutil.OnFinalize(page.Page, func() { finalized++ })
		vp := stack.AddNamed(page.Widget(), "self")
		page.Bind(vp)
		for j := range 10 {
			page.UpdateFiles(files(i + j))
		}
		stack.Remove(page.Widget())
		vp.Unref()
		page.Destroy()
	}

	const warmup, n = 5, 200
	for i := range warmup {
		cycle(i)
	}
	before := gutil.FreeCallbackSlots()
	for i := range n {
		cycle(i)
	}
	after := gutil.FreeCallbackSlots()
	t.Logf("free callback slots: before=%d after=%d; pages finalized %d/%d", before, after, finalized, warmup+n)
	if after < before {
		t.Errorf("leaked %d callback slots over %d page cycles", before-after, n)
	}
	if finalized != warmup+n {
		t.Errorf("finalized %d of %d pages", finalized, warmup+n)
	}
}
