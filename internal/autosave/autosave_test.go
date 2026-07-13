package autosave_test

import (
	"os"
	"path/filepath"
	"testing"

	"deedles.dev/trayscale/internal/autosave"
	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
)

func TestEnabled(t *testing.T) {
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
			require.Equal(t, tt.want, autosave.Enabled(tt.enabled, tt.dir))
		})
	}
}

func TestPath(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "dest", "inbox")

	require.Equal(t, filepath.Join(dir, "report.pdf"), autosave.Path(dir, "report.pdf"))
	// Waiting-file names must not escape the destination directory.
	require.Equal(t, filepath.Join(dir, "evil.txt"), autosave.Path(dir, "../evil.txt"))
	require.Equal(t, filepath.Join(dir, "nested"), autosave.Path(dir, "a/b/nested"))
	require.Equal(t, filepath.Join(dir, "only"), autosave.Path(dir, "/abs/only"))
}

func TestFiles(t *testing.T) {
	files := []apitype.WaitingFile{
		{Name: "a.txt", Size: 1},
		{Name: "b.txt", Size: 2},
		{Name: "c.txt", Size: 3},
	}

	t.Run("disabled", func(t *testing.T) {
		require.Nil(t, autosave.Files(false, "/tmp", files, nil))
	})

	t.Run("empty dir", func(t *testing.T) {
		require.Nil(t, autosave.Files(true, "", files, nil))
	})

	t.Run("all pending", func(t *testing.T) {
		got := autosave.Files(true, "/tmp/inbox", files, nil)
		require.Equal(t, []string{"a.txt", "b.txt", "c.txt"}, got)
	})

	t.Run("skips in-flight", func(t *testing.T) {
		skip := map[string]bool{"b.txt": true}
		got := autosave.Files(true, "/tmp/inbox", files, skip)
		require.Equal(t, []string{"a.txt", "c.txt"}, got)
	})

	t.Run("skips previously failed", func(t *testing.T) {
		skip := map[string]bool{"a.txt": true, "c.txt": true}
		got := autosave.Files(true, "/tmp/inbox", files, skip)
		require.Equal(t, []string{"b.txt"}, got)
	})

	t.Run("no files", func(t *testing.T) {
		got := autosave.Files(true, "/tmp/inbox", nil, nil)
		require.Empty(t, got)
	})
}

func TestDirOK(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		err := autosave.DirOK(filepath.Join(t.TempDir(), "nope"))
		require.Error(t, err)
	})

	t.Run("file not dir", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
		err := autosave.DirOK(path)
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, autosave.DirOK(dir))
	})
}

func TestDestinationPaths(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "var", "taildrop")
	files := []apitype.WaitingFile{
		{Name: "report.pdf", Size: 10},
		{Name: "../escape.txt", Size: 20},
	}
	inFlight := map[string]bool{"other.bin": true}

	names := autosave.Files(true, dir, files, inFlight)
	require.Equal(t, []string{"report.pdf", "../escape.txt"}, names)

	dests := make([]string, len(names))
	for i, name := range names {
		dests[i] = autosave.Path(dir, name)
	}
	require.Equal(t, []string{
		filepath.Join(dir, "report.pdf"),
		filepath.Join(dir, "escape.txt"),
	}, dests)
}

func TestUniqueName(t *testing.T) {
	t.Run("free original", func(t *testing.T) {
		got := autosave.UniqueName("report.pdf", func(string) bool { return false })
		require.Equal(t, "report.pdf", got)
	})

	t.Run("series with extension", func(t *testing.T) {
		taken := map[string]bool{
			"report.pdf":     true,
			"report (1).pdf": true,
			"report (2).pdf": true,
		}
		got := autosave.UniqueName("report.pdf", func(name string) bool { return taken[name] })
		require.Equal(t, "report (3).pdf", got)
	})

	t.Run("no extension", func(t *testing.T) {
		taken := map[string]bool{"LICENSE": true}
		got := autosave.UniqueName("LICENSE", func(name string) bool { return taken[name] })
		require.Equal(t, "LICENSE (1)", got)
	})

	t.Run("strips path components", func(t *testing.T) {
		got := autosave.UniqueName("../evil.txt", func(string) bool { return false })
		require.Equal(t, "evil.txt", got)
	})

	t.Run("uses first free slot", func(t *testing.T) {
		taken := map[string]bool{
			"photo.jpg":     true,
			"photo (2).jpg": true, // gap at (1)
		}
		got := autosave.UniqueName("photo.jpg", func(name string) bool { return taken[name] })
		require.Equal(t, "photo (1).jpg", got)
	})
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()

	// Nothing there yet.
	require.Equal(t, filepath.Join(dir, "note.txt"), autosave.UniquePath(dir, "note.txt"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "note.txt"), []byte("a"), 0o644))
	require.Equal(t, filepath.Join(dir, "note (1).txt"), autosave.UniquePath(dir, "note.txt"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "note (1).txt"), []byte("b"), 0o644))
	require.Equal(t, filepath.Join(dir, "note (2).txt"), autosave.UniquePath(dir, "subdir/note.txt"))
}
