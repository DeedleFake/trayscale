package ui

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/listmodels"
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
		track(&finalized, page.Page, page.actions, page.sendFileAction, page.DropTarget,
			page.addrModel.Model(), page.routeModel.Model())
		vp := stack.AddNamed(page.Widget(), "peer")
		page.Bind(vp)
		for j := range 10 {
			p := peerAt(i + j)
			page.Update(status(p))
		}
		stack.Remove(page.Widget())
		vp.Unref()
		page.Widget().Unref()
	}

	const warmup, n, objs = 5, 200, 6
	for i := range warmup {
		cycle(i)
	}
	before := gutil.FreeCallbackSlots()
	for i := range n {
		cycle(i)
	}
	after := gutil.FreeCallbackSlots()
	checkLeaks(t, before, after, n, finalized, (warmup+n)*objs)
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
		track(&finalized, page.Page, page.actions,
			page.addrModel.Model(), page.routeModel.Model(), page.fileModel.Model())
		vp := stack.AddNamed(page.Widget(), "self")
		page.Bind(vp)
		for j := range 10 {
			page.UpdateFiles(files(i + j))
		}
		stack.Remove(page.Widget())
		vp.Unref()
		page.Widget().Unref()
	}

	const warmup, n, objs = 5, 200, 5
	for i := range warmup {
		cycle(i)
	}
	before := gutil.FreeCallbackSlots()
	for i := range n {
		cycle(i)
	}
	after := gutil.FreeCallbackSlots()
	checkLeaks(t, before, after, n, finalized, (warmup+n)*objs)
}

// TestMullvadPageLeaks builds Mullvad pages with exit node rows under
// several countries, searches, removes nodes, and destroys the page.
func TestMullvadPageLeaks(t *testing.T) {
	requireGTK(t)

	stack := adw.NewViewStack()
	defer stack.Unref()

	countries := []string{"US", "DE", "SE"}
	node := func(i int) tailcfg.NodeView {
		cc := countries[i%len(countries)]
		hi := &tailcfg.Hostinfo{
			Hostname: fmt.Sprintf("%s-wg-%03d", cc, i),
			Location: &tailcfg.Location{Country: cc, CountryCode: cc, City: fmt.Sprintf("City %d", i)},
		}
		n := &tailcfg.Node{
			StableID: tailcfg.StableNodeID(fmt.Sprintf("mullvad-%d", i)),
			Name:     hi.Hostname + ".mullvad.ts.net.",
			Hostinfo: hi.View(),
			Tags:     []string{"tag:mullvad-exit-node"},
		}
		return n.View()
	}

	var finalized, rows int
	cycle := func(i int) {
		page := NewMullvadPage(&App{}, &tsutil.IPNStatus{State: ipn.Running})
		track(&finalized, page.Page)
		vp := stack.AddNamed(page.Widget(), "mullvad")
		page.Bind(vp)

		for j := range 9 {
			page.getExitNodeRow(node(i + j))
		}
		for _, n := range page.exitNodes {
			track(&finalized, n.row)
			rows++
		}
		for _, loc := range page.locations {
			track(&finalized, loc)
			rows++
		}
		page.applySearch()
		page.setQuery("city")
		page.setQuery("")
		for id, n := range page.exitNodes {
			if n.country == countries[0] {
				delete(page.exitNodes, id)
				page.removeNodeRow(n)
			}
		}

		stack.Remove(page.Widget())
		vp.Unref()
		page.Widget().Unref()
	}

	const warmup, n = 5, 100
	for i := range warmup {
		cycle(i)
	}
	before := gutil.FreeCallbackSlots()
	for i := range n {
		cycle(i)
	}
	after := gutil.FreeCallbackSlots()
	checkLeaks(t, before, after, n, finalized, warmup+n+rows)
}

// TestPushFileLeaks pushes a real file through goPushFile, which fails
// without tailscaled after opening and reading it, and checks that the
// file and every callback are released.
func TestPushFileLeaks(t *testing.T) {
	requireGTK(t)

	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	var finalized int
	cycle := func() {
		file := gio.FileNewForPath(path)
		track(&finalized, file)
		before := finalized
		(&App{}).goPushFile("peer", file)
		gutil.Obj(file).Unref()
		iterateUntil(t, func() bool { return finalized > before })
	}

	const warmup, n = 2, 50
	for range warmup {
		cycle()
	}
	before := gutil.FreeCallbackSlots()
	for range n {
		cycle()
	}
	iterateUntil(t, func() bool { return gutil.FreeCallbackSlots() >= before })
	checkLeaks(t, before, gutil.FreeCallbackSlots(), n, finalized, warmup+n)
}

// TestDialogLeaks shows and closes each kind of dialog with no main
// window and checks that the dialogs and their handlers are released.
func TestDialogLeaks(t *testing.T) {
	requireGTK(t)

	a := &App{}
	shows := []func(){
		func() { Confirmation{Heading: "h", Accept: "y", Reject: "n"}.Show(a, func(bool) {}) },
		func() { Prompt{Heading: "h"}.Show(a, "v", func(string, string) {}) },
		func() { Info{Heading: "h"}.Show(a, func() {}) },
		func() {
			Select[int]{Heading: "h", Options: []SelectOption[int]{{Title: "a"}, {Title: "b", Value: 1}}}.Show(a, func([]SelectOption[int]) {})
		},
	}

	var finalized int
	cycle := func() {
		for _, show := range shows {
			show()
			dialog := findDialog(t)
			track(&finalized, dialog)
			before := finalized
			dialog.ForceClose()
			dialog.Unref()
			iterateUntil(t, func() bool { return finalized > before })
		}
	}

	const warmup, n = 2, 25
	for range warmup {
		cycle()
	}
	before := gutil.FreeCallbackSlots()
	for range n {
		cycle()
	}
	checkLeaks(t, before, gutil.FreeCallbackSlots(), n, finalized, (warmup+n)*len(shows))
}

// findDialog returns the one open AdwDialog, owned by the caller.
func findDialog(t *testing.T) *adw.Dialog {
	t.Helper()
	windows := gtk.WindowGetToplevels()
	defer gutil.Obj(windows).Unref()
	var found *adw.Dialog
	for _, obj := range listmodels.Objects(windows) {
		w := gutil.MustCast[gtk.Widget](obj, gtk.WidgetGLibType())
		for c := range gutil.WidgetChildren(w) {
			if d, ok := gutil.Cast[adw.Dialog](c, adw.DialogGLibType()); ok && found == nil {
				d.Ref()
				found = d
			}
		}
	}
	if found == nil {
		t.Fatal("no open dialog")
	}
	return found
}

// track counts each of objs into *n when it's finalized.
func track(n *int, objs ...gobject.Ptr) {
	for _, obj := range objs {
		gutil.OnFinalize(obj, func() { *n++ })
	}
}

// iterateUntil runs the default main context until done reports true.
func iterateUntil(t *testing.T, done func() bool) {
	t.Helper()
	ctx := glib.MainContextDefault()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting on the main loop")
		}
		if !ctx.Iteration(false) {
			time.Sleep(time.Millisecond)
		}
	}
}

func checkLeaks(t *testing.T, before, after, cycles, finalized, objs int) {
	t.Helper()
	t.Logf("free callback slots: before=%d after=%d; objects finalized %d/%d", before, after, finalized, objs)
	if after < before {
		t.Errorf("leaked %d callback slots over %d cycles", before-after, cycles)
	}
	if finalized != objs {
		t.Errorf("finalized %d of %d objects", finalized, objs)
	}
}
