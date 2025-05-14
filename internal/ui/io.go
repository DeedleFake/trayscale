package ui

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"time"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"tailscale.com/tailcfg"
)

func gioReader(ctx context.Context, file gio.Filer) (io.ReadCloser, string, int64, error) {
	if file.QueryFileType(ctx, 0) == gio.FileTypeDirectory {
		return dirReader(ctx, file)
	}

	info, err := file.QueryInfo(ctx, gio.FILE_ATTRIBUTE_STANDARD_SIZE, 0)
	if err != nil {
		return nil, "", 0, fmt.Errorf("query file size: %w", err)
	}

	s, err := file.Read(ctx)
	if err != nil {
		return nil, "", 0, fmt.Errorf("open: %w", err)
	}

	return gioutil.Reader(ctx, s), file.Basename(), info.Size(), nil
}

func dirReader(ctx context.Context, file gio.Filer) (io.ReadCloser, string, int64, error) {
	done := make(chan struct{})

	r, w := io.Pipe()
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		w.Close()
	}()

	go func() {
		defer close(done)

		gz := gzip.NewWriter(w)
		defer gz.Close()

		w := tar.NewWriter(gz)
		defer w.Close()

		root := gioFS{root: file}
		err := w.AddFS(&root)
		if err != nil {
			slog.Error("write tar file", "source", file.Path(), "err", err)
		}
	}()

	return r, file.Basename() + ".tar.gz", -1, nil
}

func (a *App) pushFile(ctx context.Context, peerID tailcfg.StableNodeID, file gio.Filer) {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("peer", peerID, "path", file.Path())
	slog.Info("starting file push")

	r, name, size, err := gioReader(ctx, file)
	if err != nil {
		slog.Error("open file", "err", err)
		return
	}
	defer r.Close()

	err = tsutil.PushFile(ctx, peerID, size, name, r)
	if err != nil {
		slog.Error("push file", "err", err)
		return
	}

	slog.Info("done pushing file")
}

func (a *App) saveFile(ctx context.Context, name string, file gio.Filer) {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("path", file.Path(), "filename", name)
	slog.Info("starting file save")

	r, size, err := tsutil.GetWaitingFile(ctx, name)
	if err != nil {
		slog.Error("get file", "err", err)
		return
	}
	defer r.Close()

	s, err := file.Replace(ctx, "", false, gio.FileCreateNone)
	if err != nil {
		slog.Error("create file", "err", err)
		return
	}

	w := gioutil.Writer(ctx, s)
	_, err = io.CopyN(w, r, size)
	if err != nil {
		slog.Error("write file", "err", err)
		return
	}

	err = tsutil.DeleteWaitingFile(ctx, name)
	if err != nil {
		slog.Error("delete file", "err", err)
		return
	}

	a.poller.Poll() <- struct{}{}
	slog.Info("done saving file")
}

type gioFS struct {
	root gio.Filer
}

func (fsys *gioFS) Open(fpath string) (fs.File, error) {
	if !fs.ValidPath(fpath) {
		return nil, &fs.PathError{Op: "open", Path: fpath, Err: fs.ErrInvalid}
	}

	file := gioFile{file: fsys.root.ResolveRelativePath(fpath)}
	err := file.init(fpath)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

type gioFile struct {
	file     gio.Filer
	stream   *gioutil.StreamReader
	children *gio.FileEnumerator
}

func (file *gioFile) init(fpath string) error {
	dir := file.file.QueryFileType(context.TODO(), 0) == gio.FileTypeDirectory
	if dir {
		children, err := file.file.EnumerateChildren(context.TODO(), "*", 0)
		if err != nil {
			return &fs.PathError{Op: "open", Path: fpath, Err: err}
		}
		file.children = children

		return nil
	}

	stream, err := file.file.Read(context.TODO())
	if err != nil {
		return &fs.PathError{Op: "open", Path: fpath, Err: err}
	}
	file.stream = gioutil.Reader(context.TODO(), stream)

	return nil
}

func (file *gioFile) Stat() (fs.FileInfo, error) {
	info, err := file.file.QueryInfo(context.TODO(), "*", 0)
	if err != nil {
		return nil, err
	}
	return &gioFileInfo{FileInfo: info}, nil
}

func (file *gioFile) Read(buf []byte) (int, error) {
	if file.stream == nil {
		return 0, fs.ErrInvalid
	}
	return file.stream.Read(buf)
}

func (file *gioFile) ReadDir(n int) (entries []fs.DirEntry, err error) {
	if file.children == nil {
		return nil, fs.ErrInvalid
	}

	for n != 0 {
		info, err := file.children.NextFile(context.TODO())
		if err != nil {
			return entries, err
		}
		if info == nil {
			if n < 0 {
				return entries, nil
			}
			return entries, io.EOF
		}

		entries = append(entries, fs.FileInfoToDirEntry(&gioFileInfo{FileInfo: info}))
		n--
	}

	return entries, nil
}

func (file *gioFile) Close() error {
	var errs [2]error
	if file.stream != nil {
		errs[0] = file.stream.Close()
	}
	if file.children != nil {
		errs[1] = file.children.Close(context.TODO())
	}
	return errors.Join(errs[:]...)
}

type gioFileInfo struct {
	*gio.FileInfo
}

func (info *gioFileInfo) Mode() (mode fs.FileMode) {
	switch ftype := info.FileType(); ftype {
	case gio.FileTypeUnknown:
		mode |= fs.ModeIrregular
	case gio.FileTypeRegular:
	case gio.FileTypeDirectory:
		mode |= fs.ModeDir
	case gio.FileTypeSymbolicLink:
		mode |= fs.ModeSymlink
	case gio.FileTypeSpecial:
		mode |= fs.ModeDevice | fs.ModeNamedPipe | fs.ModeSocket | fs.ModeCharDevice | fs.ModeIrregular
	case gio.FileTypeShortcut:
		mode |= fs.ModeIrregular
	case gio.FileTypeMountable:
		mode |= fs.ModeDevice
	default:
		panic(fmt.Errorf("unexpected file type: %v", ftype))
	}

	return mode | fs.FileMode(info.AttributeUint32(gio.FILE_ATTRIBUTE_UNIX_MODE))
}

func (info *gioFileInfo) ModTime() time.Time {
	gtime := info.ModificationDateTime()
	return time.UnixMicro(gtime.ToUnixUsec())
}

func (info *gioFileInfo) IsDir() bool {
	return info.FileType() == gio.FileTypeDirectory
}

func (info *gioFileInfo) Sys() any {
	return info.FileInfo
}
