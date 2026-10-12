package gutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
)

func TestFirstIconName(t *testing.T) {
	dir := t.TempDir()
	// Without an index on disk, GTK loads hicolor's from its own
	// resources, which only gtk.Init registers.
	for _, name := range []string{"hicolor", "test"} {
		writeFile(t, filepath.Join(dir, name, "index.theme"), "[Icon Theme]\nName="+name+"\nDirectories=status\n\n[status]\nSize=16\nType=Scalable\n")
	}
	writeFile(t, filepath.Join(dir, "test", "status", "present-symbolic.svg"), `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"/>`)

	theme := gtk.NewIconTheme()
	defer unrefIconTheme(theme)
	theme.SetSearchPath([]string{dir})
	theme.SetThemeName("test")

	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{name: "first present", names: []string{"present-symbolic", "missing-symbolic"}, want: "present-symbolic"},
		{name: "fallback present", names: []string{"missing-symbolic", "present-symbolic"}, want: "present-symbolic"},
		{name: "none present", names: []string{"missing-symbolic", "other-symbolic"}, want: "missing-symbolic"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := gutil.FirstIconName(theme, test.names...)
			if got != test.want {
				t.Fatalf("FirstIconName(%q) = %q, want %q", test.names, got, test.want)
			}
		})
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// unrefIconTheme drops the last reference to theme. GTK's icon theme
// dispose takes a reference to itself through gtk_icon_theme_ref_aquire
// and then never releases it, because it clears ref->theme before
// gtk_icon_theme_ref_release looks at it, so the theme resurrects with
// one reference left. If that happened, drop it so that the theme is
// finalized and go test -asan doesn't report it.
func unrefIconTheme(theme *gtk.IconTheme) {
	finalized := false
	var notify glib.DestroyNotify = func(uintptr) { finalized = true }
	theme.SetDataFull("trayscale-test-finalized", 1, &notify)
	defer glib.UnrefCallback(&notify)

	theme.Unref()
	if !finalized {
		theme.Unref()
	}
}
