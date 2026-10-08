package gutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"deedles.dev/trayscale/internal/gutil"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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
