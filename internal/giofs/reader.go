package giofs

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/klauspost/compress/zstd"
)

func Reader(ctx context.Context, file gio.Filer) (io.ReadCloser, int64, string, error) {
	if file.QueryFileType(ctx, 0) == gio.FileTypeDirectory {
		return dirReader(ctx, file)
	}

	info, err := file.QueryInfo(ctx, gio.FILE_ATTRIBUTE_STANDARD_SIZE, 0)
	if err != nil {
		return nil, 0, "", fmt.Errorf("query file size: %w", err)
	}

	s, err := file.Read(ctx)
	if err != nil {
		return nil, 0, "", fmt.Errorf("open: %w", err)
	}

	return gioutil.Reader(ctx, s), info.Size(), file.Basename(), nil
}

func dirReader(ctx context.Context, file gio.Filer) (io.ReadCloser, int64, string, error) {
	done := make(chan struct{})

	r, w := io.Pipe()
	go func() {
		defer w.Close()

		select {
		case <-ctx.Done():
		case <-done:
		}
	}()

	go func() {
		defer close(done)

		z, _ := zstd.NewWriter(w)
		defer z.Close()

		w := tar.NewWriter(z)
		defer w.Close()

		root := gioFS{root: file}
		err := w.AddFS(&root)
		if err != nil {
			slog.Error("write tar file", "source", file.Path(), "err", err)
		}
	}()

	return r, -1, file.Basename() + ".tar.zst", nil
}
