package gobj

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

var initGTK = sync.OnceValue(func() bool {
	os.Setenv("GSK_RENDERER", "cairo")
	if !gtk.InitCheck() {
		return false
	}
	adw.Init()
	return true
})

func requireGTK(t *testing.T) {
	t.Helper()
	if !initGTK() {
		t.Skip("no display")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	runtime.GC()
	os.Exit(code)
}

const testRowXML = `<?xml version="1.0" encoding="UTF-8"?>
<interface>
  <template class="GobjTestRow" parent="AdwBin">
    <property name="child">
      <object class="GtkLabel" id="Label">
        <property name="label">unset</property>
      </object>
    </property>
  </template>
</interface>`

type testRow struct {
	Base
	Label *gtk.Label

	disposed *int
}

func (r *testRow) Dispose() {
	// Template children are still alive here.
	r.Label.SetLabel("disposed")
	if r.disposed != nil {
		*r.disposed++
	}
}

var testRowClass = Widget[testRow](WidgetSpec{
	Name:     "GobjTestRow",
	Parent:   adw.BinGLibType,
	Template: testRowXML,
})

func TestNewBindsTemplate(t *testing.T) {
	requireGTK(t)

	r := testRowClass.New()
	defer r.Widget().Unref()
	if got := r.Label.GetLabel(); got != "unset" {
		t.Fatalf("label = %q, want %q", got, "unset")
	}
	if r.Widget().IsFloating() {
		t.Fatal("New returned a floating instance")
	}
	got, ok := Lookup[testRow](r)
	if !ok || got != r {
		t.Fatalf("Lookup = %p, %v; want %p", got, ok, r)
	}
	if _, ok := Lookup[testRow](gtk.NewLabel("x")); ok {
		t.Fatal("Lookup found a testRow for a plain label")
	}
}

func TestBuilderInstantiatesClass(t *testing.T) {
	requireGTK(t)

	testRowClass.GType()
	b := gtk.NewBuilderFromString(`<interface><object class="GobjTestRow" id="row"/></interface>`, -1)
	obj := b.GetObject("row")
	b.Unref()
	defer obj.Unref()
	r, ok := Lookup[testRow](obj)
	if !ok || r.Label.GetLabel() != "unset" {
		t.Fatal("builder-made instance has no bound Go value")
	}
}

func TestFinalizeFreesState(t *testing.T) {
	requireGTK(t)

	const n = 5000
	// class_init's slots come once per class, and OnFinalize's once.
	warm := testRowClass.New()
	gutil.OnFinalize(warm.Label, func() {})
	warm.Widget().Unref()
	before := gutil.FreeCallbackSlots()
	disposed, children := 0, 0
	for range n {
		r := testRowClass.New()
		r.disposed = &disposed
		gutil.OnFinalize(r.Label, func() { children++ })
		r.Widget().Unref()
	}
	if after := gutil.FreeCallbackSlots(); after != before {
		t.Errorf("free callback slots %d -> %d over %d instances", before, after, n)
	}
	if disposed != n {
		t.Errorf("Dispose ran %d times, want %d", disposed, n)
	}
	if children != n {
		t.Errorf("%d template children finalized, want %d", children, n)
	}
	if live := testRowClass.live(); live != 0 {
		t.Errorf("%d instances still in the Go map after finalize", live)
	}
}

type wrongRow struct {
	Base
	Label *gtk.Button
}

var wrongRowClass = Widget[wrongRow](WidgetSpec{
	Name:     "GobjTestWrongRow",
	Parent:   adw.BinGLibType,
	Template: strings.ReplaceAll(testRowXML, "GobjTestRow", "GobjTestWrongRow"),
})

func TestWrongChildTypePanics(t *testing.T) {
	requireGTK(t)

	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "want GtkButton") {
			t.Fatalf("recovered %v, want a child type panic", r)
		}
	}()
	wrongRowClass.New()
}

type testWindow struct{ Base }

var testWindowClass = Widget[testWindow](WidgetSpec{
	Name:     "GobjTestWindow",
	Parent:   gtk.WindowGLibType,
	Template: `<interface><template class="GobjTestWindow" parent="GtkWindow"/></interface>`,
})

// TestNewOwnsWindow checks that New hands back an owned reference even
// for a type whose init sinks itself, as GtkWindow does.
func TestNewOwnsWindow(t *testing.T) {
	requireGTK(t)

	w := testWindowClass.New()
	finalized := 0
	gutil.OnFinalize(w, func() { finalized++ })
	gtk.WindowNewFromInternalPtr(w.GoPointer()).Destroy()
	if finalized != 0 {
		t.Fatal("window finalized while the caller still owned it")
	}
	w.Widget().Unref()
	if finalized != 1 {
		t.Fatalf("window finalized %d times after the last unref, want 1", finalized)
	}
}
