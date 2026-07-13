package ui_test

import (
	"path/filepath"
	"testing"

	"deedles.dev/trayscale/internal/ui"
	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
)

func TestAutoSaveEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		dir     string
		want    bool
	}{
		{name: "disabled with dir", enabled: false, dir: "/tmp/taildrop", want: false},
		{name: "enabled empty dir", enabled: true, dir: "", want: false},
		{name: "enabled whitespace dir", enabled: true, dir: "  \t  ", want: false},
		{name: "enabled with dir", enabled: true, dir: "/home/user/Downloads", want: true},
		{name: "disabled empty dir", enabled: false, dir: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ui.AutoSaveEnabled(tt.enabled, tt.dir))
		})
	}
}

func TestAutoSavePath(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "dest", "inbox")

	require.Equal(t, filepath.Join(dir, "report.pdf"), ui.AutoSavePath(dir, "report.pdf"))
	// Waiting-file names must not escape the destination directory.
	require.Equal(t, filepath.Join(dir, "evil.txt"), ui.AutoSavePath(dir, "../evil.txt"))
	require.Equal(t, filepath.Join(dir, "nested"), ui.AutoSavePath(dir, "a/b/nested"))
	require.Equal(t, filepath.Join(dir, "only"), ui.AutoSavePath(dir, "/abs/only"))
}

func TestFilesToAutoSave(t *testing.T) {
	files := []apitype.WaitingFile{
		{Name: "a.txt", Size: 1},
		{Name: "b.txt", Size: 2},
		{Name: "c.txt", Size: 3},
	}

	t.Run("disabled", func(t *testing.T) {
		require.Nil(t, ui.FilesToAutoSave(false, "/tmp", files, nil))
	})

	t.Run("empty dir", func(t *testing.T) {
		require.Nil(t, ui.FilesToAutoSave(true, "", files, nil))
	})

	t.Run("all pending", func(t *testing.T) {
		got := ui.FilesToAutoSave(true, "/tmp/inbox", files, nil)
		require.Equal(t, []string{"a.txt", "b.txt", "c.txt"}, got)
	})

	t.Run("skips in-flight", func(t *testing.T) {
		inFlight := map[string]bool{"b.txt": true}
		got := ui.FilesToAutoSave(true, "/tmp/inbox", files, inFlight)
		require.Equal(t, []string{"a.txt", "c.txt"}, got)
	})

	t.Run("no files", func(t *testing.T) {
		got := ui.FilesToAutoSave(true, "/tmp/inbox", nil, nil)
		require.Empty(t, got)
	})
}

func TestAutoSaveDestinationPaths(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "var", "taildrop")
	files := []apitype.WaitingFile{
		{Name: "report.pdf", Size: 10},
		{Name: "../escape.txt", Size: 20},
	}
	inFlight := map[string]bool{"other.bin": true}

	names := ui.FilesToAutoSave(true, dir, files, inFlight)
	require.Equal(t, []string{"report.pdf", "../escape.txt"}, names)

	dests := make([]string, len(names))
	for i, name := range names {
		dests[i] = ui.AutoSavePath(dir, name)
	}
	require.Equal(t, []string{
		filepath.Join(dir, "report.pdf"),
		filepath.Join(dir, "escape.txt"),
	}, dests)
}
