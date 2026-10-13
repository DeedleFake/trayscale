package ui

import (
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"codeberg.org/puregotk/purego"
	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale"
	"deedles.dev/trayscale/internal/giofs"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/ipn"
)

// objectRefCount reads the GObject ref_count field. LookupAction's
// extra reference is only visible there.
func objectRefCount(obj gobject.Ptr) uint32 {
	p := obj.GoPointer() + unsafe.Sizeof(uintptr(0))
	return *(*uint32)(*(*unsafe.Pointer)(unsafe.Pointer(&p)))
}

func TestUpdateDoesNotLeakExitNodeAction(t *testing.T) {
	requireGTK(t)

	a := &App{operatorCheck: true, online: true}
	a.app = adw.NewApplication("dev.deedles.Trayscale.test.exit-node", gio.GApplicationNonUniqueValue)
	defer a.app.Unref()

	action := gio.NewSimpleActionStateful("use_exit_node", nil, glib.NewVariantBoolean(false))
	var finalized int
	gutil.OnFinalize(action, func() { finalized++ })
	a.addAction(action, nil)

	before := objectRefCount(action)
	status := &tsutil.IPNStatus{State: ipn.Running, Prefs: ipn.NewPrefs().View()}
	for range 100 {
		a.update(status)
	}
	after := objectRefCount(action)
	if after != before {
		t.Fatalf("use_exit_node ref_count = %d after 100 updates, want %d", after, before)
	}

	replacement := gio.NewSimpleActionStateful("use_exit_node", nil, glib.NewVariantBoolean(false))
	a.app.AddAction(replacement)
	replacement.Unref()
	if finalized != 1 {
		t.Fatalf("replaced use_exit_node finalized %d times, want 1", finalized)
	}
	a.app.RemoveAction("use_exit_node")
}

type stackStub struct {
	w    *gtk.Widget
	page *adw.ViewStackPage
}

func (p *stackStub) Widget() *gtk.Widget             { return p.w }
func (p *stackStub) Actions() *gio.SimpleActionGroup { return nil }
func (p *stackStub) Bind(vp *adw.ViewStackPage)      { p.page = vp }
func (p *stackStub) Update(tsutil.Status) bool       { p.page.SetTitle("still-bound"); return true }
func (p *stackStub) Destroy()                        { p.w.Unref() }

func TestRestackReleasesReplacedStackPage(t *testing.T) {
	requireGTK(t)

	win := &MainWindow{
		pages:      make(map[string]Page),
		stackPages: make(map[string]*adw.ViewStackPage),
		PeersStack: adw.NewViewStack(),
	}
	win.profileModel = gtk.NewStringList(nil)
	win.profileModel.Ref()
	win.profileSortModel = gtk.NewSortListModel(win.profileModel, &newStringSorter().Sorter)
	aBox := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	bBox := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	pageA := &stackStub{w: &aBox.Widget}
	pageB := &stackStub{w: &bBox.Widget}
	win.addPage("a", pageA)
	win.addPage("b", pageB)

	var replaced, kept int
	gutil.OnFinalize(win.stackPages["a"], func() { replaced++ })
	gutil.OnFinalize(win.stackPages["b"], func() { kept++ })

	win.restack([]string{"a"})
	if kept != 0 {
		t.Fatalf("filtered stack page finalized %d times, want 0", kept)
	}
	if !pageB.Update(nil) || pageB.page.GetTitle() != "still-bound" {
		t.Fatal("filtered page cannot update its stack page")
	}
	if replaced != 1 {
		t.Fatalf("replaced stack page finalized %d times, want 1", replaced)
	}

	win.Destroy()
	if kept != 1 {
		t.Fatalf("filtered stack page finalized %d times after Destroy, want 1", kept)
	}
}

func TestReplaceExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := gio.FileNewForPath(path)
	defer gutil.Obj(file).Unref()

	c, release := giofs.Cancellable(context.Background())
	s, err := file.Replace("", false, gio.GFileCreateNoneValue, c)
	release()
	if err = gutil.Err(err); err != nil {
		t.Fatalf("Replace existing file: %v", err)
	}
	w := giofs.NewWriter(context.Background(), &s.OutputStream)
	if _, err := io.WriteString(w, "new"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("file = %q, want %q", got, "new")
	}
}

func TestHelpOmitsCatalogHeader(t *testing.T) {
	if os.Getenv("TRAYSCALE_HELP_REPRO") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelpOmitsCatalogHeader$")
		cmd.Env = append(os.Environ(), "TRAYSCALE_HELP_REPRO=1")
		out, err := cmd.CombinedOutput()
		text := string(out)
		if strings.Contains(text, "Project-Id-Version") {
			t.Fatalf("help contains the catalog header:\n%s", text)
		}
		if !strings.Contains(text, "--hide-window") || !strings.Contains(text, "Hide window on initial start") {
			t.Fatalf("help subprocess (err %v):\n%s", err, text)
		}
		return
	}

	requireGTK(t)
	locale.SanitizeEnvironment()
	t.Setenv("LANGUAGE", "en")
	po, err := fs.Sub(trayscale.Assets(), "po")
	if err != nil {
		t.Fatal(err)
	}
	locale.Init(po)
	header := dgettext("trayscale", "")
	if !strings.Contains(header, "Project-Id-Version") {
		t.Fatalf("catalog header = %q, want Project-Id-Version", header)
	}

	app := gio.NewApplication("dev.deedles.Trayscale.test.help", gio.GApplicationNonUniqueValue)
	defer app.Unref()
	addHideWindowOption(app)
	app.Run(2, []string{"trayscale", "--help"})
}

func dgettext(domain, msgid string) string {
	lib, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	var fn func(string, string) string
	purego.RegisterLibFunc(&fn, lib, "dgettext")
	return fn(domain, msgid)
}
