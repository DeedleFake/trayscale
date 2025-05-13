package ui

import (
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

func (a *App) pushFile(ctx context.Context, peerID tailcfg.StableNodeID, file gio.Filer) {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("peer", peerID, "path", file.Path())
	slog.Info("starting file push")

	s, err := file.Read(ctx)
	if err != nil {
		slog.Error("open file", "err", err)
		return
	}
	defer s.Close(ctx)

	info, err := s.QueryInfo(ctx, gio.FILE_ATTRIBUTE_STANDARD_SIZE)
	if err != nil {
		slog.Error("query file info", "err", err)
		return
	}

	r := gioutil.Reader(ctx, s)
	err = tsutil.PushFile(ctx, peerID, info.Size(), file.Basename(), r)
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

	file := fsys.root.ResolveRelativePath(fpath)
	dir := file.QueryFileType(context.TODO(), 0) == gio.FileTypeDirectory

	wrapper := gioFile{
		file: file,
	}

	if !dir {
		stream, err := file.Read(context.TODO())
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: fpath, Err: err}
		}
		wrapper.stream = gioutil.Reader(context.TODO(), stream)
	}

	if dir {
		children, err := file.EnumerateChildren(context.TODO(), "*", 0)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: fpath, Err: err}
		}
		wrapper.children = children
	}

	return &wrapper, nil
}

type gioFile struct {
	file     gio.Filer
	stream   *gioutil.StreamReader
	children *gio.FileEnumerator
}

func (file *gioFile) Stat() (fs.FileInfo, error) {
	info, err := file.file.QueryFilesystemInfo(context.TODO(), "*")
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

	for n > 0 {
		info, err := file.children.NextFile(context.TODO())
		if err != nil {
			return entries, err
		}
		if info == nil {
			return entries, io.EOF
		}

		entries = append(entries, fs.FileInfoToDirEntry(&gioFileInfo{FileInfo: info}))
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

	return mode | 0755 // TODO: Properly handle permissions?
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
