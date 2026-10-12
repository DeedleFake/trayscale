package gutil_test

import (
	"os"
	"path/filepath"
	"testing"

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

	// Deliberately never released: GTK leaks part of every icon theme
	// that is freed (gtk_icon_theme_ref_dispose keeps a reference it
	// never drops, and finalize never destroys icon_cache), which
	// go test -asan would report. Keeping it reachable avoids that.
	theme := gtk.NewIconTheme()
	keepIconTheme = theme
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

// keepIconTheme keeps TestFirstIconName's theme reachable until exit.
var keepIconTheme *gtk.IconTheme
