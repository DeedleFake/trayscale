package giofs_test

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"deedles.dev/trayscale/internal/giofs"
	"deedles.dev/trayscale/internal/gutil"
	"github.com/klauspost/compress/zstd"
)

func newFile(t *testing.T, path string) *gio.FileBase {
	t.Helper()
	f := gio.FileNewForPath(path)
	t.Cleanup(func() { gobject.ObjectNewFromInternalPtr(f.GoPointer()).Unref() })
	return f
}

func TestReaderFile(t *testing.T) {
	dir := t.TempDir()
	want := bytes.Repeat([]byte("trayscale"), 100000) // several reads
	path := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	r, size, name, err := giofs.Reader(context.Background(), newFile(t, path))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if size != int64(len(want)) || name != "data.bin" || !bytes.Equal(got, want) {
		t.Errorf("Reader = %d bytes (size %d, name %q), want %d bytes named data.bin", len(got), size, name, len(want))
	}
}

func TestReaderDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "d", "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "d", "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "d", "sub", "b.txt"), []byte("world"), 0o644)

	r, _, name, err := giofs.Reader(context.Background(), newFile(t, filepath.Join(dir, "d")))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if name != "d.tar.zst" {
		t.Errorf("name = %q", name)
	}
	z, err := zstd.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	files := map[string]string{}
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		if h.Typeflag == tar.TypeReg {
			files[h.Name] = string(data)
		}
	}
	if files["a.txt"] != "hello" || files["sub/b.txt"] != "world" {
		t.Errorf("tar contents = %v", files)
	}
}

func TestWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")
	want := bytes.Repeat([]byte("x"), 300000)

	s, err := newFile(t, path).Replace("", false, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := giofs.NewWriter(context.Background(), &s.OutputStream)
	if _, err := w.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Errorf("wrote %d bytes, read back %d (err %v)", len(want), len(got), err)
	}
}

// TestNoCallbackLeak: the stream adapters use no callbacks; make sure
// that stays true.
func TestNoCallbackLeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	os.WriteFile(path, []byte("abc"), 0o644)
	f := newFile(t, path)
	before := gutil.FreeCallbackSlots()
	for range 100 {
		r, _, _, err := giofs.Reader(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		io.ReadAll(r)
		r.Close()
	}
	if after := gutil.FreeCallbackSlots(); after != before {
		t.Errorf("free callback slots %d -> %d", before, after)
	}
}
