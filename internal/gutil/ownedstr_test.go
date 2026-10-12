package gutil_test

import (
	"testing"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/gutil"
)

func TestFileStrings(t *testing.T) {
	file := gio.FileNewForPath("/tmp/some dir/file.txt")
	defer gutil.Obj(file).Unref()

	if got := gutil.FilePath(file); got != "/tmp/some dir/file.txt" {
		t.Errorf("FilePath = %q", got)
	}
	if got := gutil.FileBasename(file); got != "file.txt" {
		t.Errorf("FileBasename = %q", got)
	}

	uri := gio.FileNewForUri("https://example.com/x")
	defer gutil.Obj(uri).Unref()
	if got := gutil.FilePath(uri); got != "" {
		t.Errorf("FilePath(non-local) = %q, want empty", got)
	}
}

// TestFileStringsNoLeak repeats the calls enough that go test -asan
// reports a leak if the returned strings aren't freed. LeakSanitizer
// can miss the last few because stale copies of the pointers stay in
// Go memory.
func TestFileStringsNoLeak(t *testing.T) {
	file := gio.FileNewForPath("/tmp/file.txt")
	defer gutil.Obj(file).Unref()
	for range 1000 {
		gutil.FilePath(file)
		gutil.FileBasename(file)
	}
}
