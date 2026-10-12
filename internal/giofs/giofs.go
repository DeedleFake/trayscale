package giofs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"deedles.dev/trayscale/internal/gutil"
)

type gioFS struct {
	ctx  context.Context
	root *gio.FileBase
}

// New returns an fs.FS rooted at root. root is borrowed and must stay
// alive as long as the FS is used.
func New(ctx context.Context, root *gio.FileBase) fs.FS {
	return &gioFS{
		ctx:  ctx,
		root: root,
	}
}

func (fsys *gioFS) Open(fpath string) (fs.File, error) {
	if !fs.ValidPath(fpath) {
		return nil, &fs.PathError{Op: "open", Path: fpath, Err: fs.ErrInvalid}
	}

	c, release := Cancellable(fsys.ctx)
	file := file{c: c, release: release, file: fsys.root.ResolveRelativePath(fpath)}
	err := file.init(fpath)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &file, nil
}

type file struct {
	c        *gio.Cancellable
	release  func()
	file     *gio.FileBase // owned
	stream   *StreamReader
	children *gio.FileEnumerator
}

func (file *file) init(fpath string) error {
	dir := file.file.QueryFileType(0, file.c) == gio.GFileTypeDirectoryValue
	if dir {
		children, err := file.file.EnumerateChildren("*", 0, file.c)
		err = gutil.Err(err)
		if err != nil {
			return &fs.PathError{Op: "open", Path: fpath, Err: err}
		}
		file.children = children

		return nil
	}

	stream, err := file.file.Read(file.c)
	err = gutil.Err(err)
	if err != nil {
		return &fs.PathError{Op: "open", Path: fpath, Err: err}
	}
	file.stream = &StreamReader{s: &stream.InputStream, c: file.c, release: func() {}}

	return nil
}

func (file *file) Stat() (fs.FileInfo, error) {
	info, err := file.file.QueryInfo("*", 0, file.c)
	err = gutil.Err(err)
	if err != nil {
		return nil, err
	}
	return newFileInfo(info), nil
}

func (file *file) Read(buf []byte) (int, error) {
	if file.stream == nil {
		return 0, fs.ErrInvalid
	}
	return file.stream.Read(buf)
}

func (file *file) ReadDir(n int) (entries []fs.DirEntry, err error) {
	if file.children == nil {
		return nil, fs.ErrInvalid
	}

	for n != 0 {
		info, err := file.children.NextFile(file.c)
		err = gutil.Err(err)
		if err != nil {
			return entries, err
		}
		if info == nil {
			if n < 0 {
				return entries, nil
			}
			return entries, io.EOF
		}

		entries = append(entries, fs.FileInfoToDirEntry(newFileInfo(info)))
		n--
	}

	return entries, nil
}

func (file *file) Close() error {
	var errs [2]error
	if file.stream != nil {
		errs[0] = file.stream.Close()
		file.stream = nil
	}
	if file.children != nil {
		_, errs[1] = file.children.Close(file.c)
		errs[1] = gutil.Err(errs[1])
		file.children.Unref()
		file.children = nil
	}
	if file.file != nil {
		unref(file.file)
		file.file = nil
	}
	if file.release != nil {
		file.release()
		file.release = nil
	}
	return errors.Join(errs[:]...)
}

// fileInfo copies what it needs out of a GFileInfo and releases it, so
// fs.FileInfo values never hold GObjects.
type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func newFileInfo(info *gio.FileInfo) *fileInfo {
	defer info.Unref()

	fi := fileInfo{
		name: info.GetName(),
		size: info.GetSize(),
		mode: fileMode(info.GetFileType()) | fs.FileMode(info.GetAttributeUint32(gio.FILE_ATTRIBUTE_UNIX_MODE)),
	}
	if t := info.GetModificationDateTime(); t != nil {
		fi.modTime = time.UnixMicro(t.ToUnixUsec())
		t.Unref()
	}
	return &fi
}

func fileMode(ftype gio.FileType) (mode fs.FileMode) {
	switch ftype {
	case gio.GFileTypeUnknownValue:
		mode |= fs.ModeIrregular
	case gio.GFileTypeRegularValue:
	case gio.GFileTypeDirectoryValue:
		mode |= fs.ModeDir
	case gio.GFileTypeSymbolicLinkValue:
		mode |= fs.ModeSymlink
	case gio.GFileTypeSpecialValue:
		mode |= fs.ModeDevice | fs.ModeNamedPipe | fs.ModeSocket | fs.ModeCharDevice | fs.ModeIrregular
	case gio.GFileTypeShortcutValue:
		mode |= fs.ModeIrregular
	case gio.GFileTypeMountableValue:
		mode |= fs.ModeDevice
	default:
		panic(fmt.Errorf("unexpected file type: %v", ftype))
	}
	return mode
}

func (info *fileInfo) Name() string       { return info.name }
func (info *fileInfo) Size() int64        { return info.size }
func (info *fileInfo) Mode() fs.FileMode  { return info.mode }
func (info *fileInfo) ModTime() time.Time { return info.modTime }
func (info *fileInfo) IsDir() bool        { return info.mode.IsDir() }
func (info *fileInfo) Sys() any           { return nil }

func ref(f *gio.FileBase)   { gobject.ObjectNewFromInternalPtr(f.GoPointer()).Ref() }
func unref(f *gio.FileBase) { gobject.ObjectNewFromInternalPtr(f.GoPointer()).Unref() }
