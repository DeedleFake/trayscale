package giofs

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/gutil"
	"github.com/klauspost/compress/zstd"
)

// Reader opens file for reading. A directory is streamed as a
// zstd-compressed tar archive. file is borrowed; the returned reader
// doesn't keep it.
func Reader(ctx context.Context, file *gio.FileBase) (io.ReadCloser, int64, string, error) {
	c, release := Cancellable(ctx)
	defer release()

	if file.QueryFileType(0, c) == gio.GFileTypeDirectoryValue {
		return dirReader(ctx, file)
	}

	info, err := file.QueryInfo(gio.FILE_ATTRIBUTE_STANDARD_SIZE, 0, c)
	err = gutil.Err(err)
	if err != nil {
		return nil, 0, "", fmt.Errorf("query file size: %w", err)
	}
	size := info.GetSize()
	info.Unref()

	s, err := file.Read(c)
	err = gutil.Err(err)
	if err != nil {
		return nil, 0, "", fmt.Errorf("open: %w", err)
	}

	return NewReader(ctx, &s.InputStream), size, gutil.FileBasename(file), nil
}

func dirReader(ctx context.Context, file *gio.FileBase) (io.ReadCloser, int64, string, error) {
	r, w := io.Pipe()

	// The goroutine outlives this call, so it needs its own reference.
	ref(file)
	go func() {
		defer unref(file)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		context.AfterFunc(ctx, func() { w.Close() })

		z, _ := zstd.NewWriter(w)
		defer z.Close()

		w := tar.NewWriter(z)
		defer w.Close()

		root := New(ctx, file)
		err := w.AddFS(root)
		if err != nil {
			slog.Error("write tar file", "source", gutil.FilePath(file), "err", err)
		}
	}()

	return r, -1, gutil.FileBasename(file) + ".tar.zst", nil
}
