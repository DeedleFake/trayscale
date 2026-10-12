package ui

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func mullvadNode(i int) tailcfg.NodeView {
	cc := []string{"US", "DE", "SE", "JP"}[i%4]
	hi := &tailcfg.Hostinfo{
		Hostname: fmt.Sprintf("%s-wg-%03d", cc, i),
		Location: &tailcfg.Location{Country: cc, CountryCode: cc, City: fmt.Sprintf("City %d", i%40)},
	}
	return (&tailcfg.Node{
		StableID: tailcfg.StableNodeID(fmt.Sprintf("mullvad-%d", i)),
		Name:     hi.Hostname + ".mullvad.ts.net.",
		Hostinfo: hi.View(),
		Tags:     []string{"tag:mullvad-exit-node"},
	}).View()
}

// slotsInUse builds n peer pages and a Mullvad page with m nodes and
// reports how many callback slots they hold, or -1 if building them ran
// out of slots.
func slotsInUse(t *testing.T, n, m int) (used int) {
	stack := adw.NewViewStack()
	defer stack.Unref()

	before := gutil.FreeCallbackSlots()
	var peers []*PeerPage
	var mullvad *MullvadPage
	defer func() {
		if recover() != nil {
			used = -1
		}
		for _, p := range peers {
			stack.Remove(p.Widget())
			p.Destroy()
		}
		if mullvad != nil {
			stack.Remove(mullvad.Widget())
			mullvad.Destroy()
		}
		// Shared callbacks made on first use stay, so wait for the
		// main loop to go idle rather than for the count to return.
		for glib.MainContextDefault().Iteration(false) {
		}
	}()

	for i := range n {
		peer := layoutPeer(t, fmt.Sprintf("peer-%d", i), fmt.Sprintf("peer-%d", i), true, false, false)
		s := layoutStatus([]tailcfg.NodeView{peer})
		s.Prefs = ipn.NewPrefs().View()
		page := NewPeerPage(&App{}, s, peer)
		peers = append(peers, page)
		vp := stack.AddNamed(page.Widget(), string(peer.StableID()))
		page.Bind(vp)
		vp.Unref()
	}
	if m > 0 {
		mullvad = NewMullvadPage(&App{}, &tsutil.IPNStatus{State: ipn.Running})
		vp := stack.AddNamed(mullvad.Widget(), "mullvad")
		mullvad.Bind(vp)
		vp.Unref()
		for i := range m {
			mullvad.getExitNodeRow(mullvadNode(i))
		}
		mullvad.applySearch()
	}
	return before - gutil.FreeCallbackSlots()
}

// TestCallbackSlotsScale checks that the callback slots the UI holds
// don't depend on the number of peers or Mullvad nodes.
func TestCallbackSlotsScale(t *testing.T) {
	requireGTK(t)
	slotsInUse(t, 1, 1) // Shared callbacks are created on first use.
	want := slotsInUse(t, 1, 1)
	for _, n := range []int{0, 10, 50, 200} {
		for _, m := range []int{0, 100, 535, 2000} {
			if n == 0 && m == 0 {
				continue
			}
			got := slotsInUse(t, n, m)
			t.Logf("peers=%d mullvad=%d slots=%d", n, m, got)
			if got != want {
				t.Errorf("peers=%d mullvad=%d: %d slots, want %d as with 1 of each", n, m, got, want)
			}
		}
	}
}

// TestIdleAddBurst checks that pending IdleAdd calls don't each hold a
// callback slot.
func TestIdleAddBurst(t *testing.T) {
	requireGTK(t)
	gutil.IdleAdd(func() {})
	before := gutil.FreeCallbackSlots()
	const k = 10000
	var wg sync.WaitGroup
	var ran atomic.Int32
	wg.Go(func() {
		defer func() {
			if recover() != nil {
				t.Error("ran out of callback slots")
			}
		}()
		for range k {
			gutil.IdleAdd(func() { ran.Add(1) })
		}
	})
	wg.Wait()
	held := before - gutil.FreeCallbackSlots()
	iterateUntil(t, func() bool { return ran.Load() == int32(k) || t.Failed() })
	t.Logf("pending idle=%d slots=%d", k, held)
	if held != 0 {
		t.Errorf("%d pending idle callbacks hold %d slots", k, held)
	}
}

// BenchmarkMullvadPage builds a Mullvad page with 535 nodes.
func BenchmarkMullvadPage(b *testing.B) {
	if !initGTK() {
		b.Skip("no display")
	}
	stack := adw.NewViewStack()
	defer stack.Unref()
	for b.Loop() {
		page := NewMullvadPage(&App{}, &tsutil.IPNStatus{State: ipn.Running})
		vp := stack.AddNamed(page.Widget(), "mullvad")
		page.Bind(vp)
		vp.Unref()
		for i := range 535 {
			page.getExitNodeRow(mullvadNode(i))
		}
		page.applySearch()
		stack.Remove(page.Widget())
		page.Destroy()
	}
}

// BenchmarkSignalEmit emits a signal connected with gutil.Connect.
func BenchmarkSignalEmit(b *testing.B) {
	if !initGTK() {
		b.Skip("no display")
	}
	btn := gtk.NewButton()
	defer btn.Unref()
	var n int
	gutil.Connect(btn, btn.ConnectClicked, func(gtk.Button) { n++ })
	obj := gutil.Obj(btn)
	for b.Loop() {
		gobject.SignalEmitByName(obj, "clicked")
	}
	if n == 0 {
		b.Fatal("handler never ran")
	}
}
