package giofs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

func New(root gio.Filer) fs.FS {
	return &gioFS{
		root: root,
	}
}

type gioFS struct {
	root gio.Filer
}

func (fsys *gioFS) Open(fpath string) (fs.File, error) {
	if !fs.ValidPath(fpath) {
		return nil, &fs.PathError{Op: "open", Path: fpath, Err: fs.ErrInvalid}
	}

	file := file{file: fsys.root.ResolveRelativePath(fpath)}
	err := file.init(fpath)
	if err != nil {
		return nil, err
	}

	return &file, nil
}

type file struct {
	file     gio.Filer
	stream   *gioutil.StreamReader
	children *gio.FileEnumerator
}

func (file *file) init(fpath string) error {
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

func (file *file) Stat() (fs.FileInfo, error) {
	info, err := file.file.QueryInfo(context.TODO(), "*", 0)
	if err != nil {
		return nil, err
	}
	return &fileInfo{FileInfo: info}, nil
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

		entries = append(entries, fs.FileInfoToDirEntry(&fileInfo{FileInfo: info}))
		n--
	}

	return entries, nil
}

func (file *file) Close() error {
	var errs [2]error
	if file.stream != nil {
		errs[0] = file.stream.Close()
	}
	if file.children != nil {
		errs[1] = file.children.Close(context.TODO())
	}
	return errors.Join(errs[:]...)
}

type fileInfo struct {
	*gio.FileInfo
}

func (info *fileInfo) Mode() (mode fs.FileMode) {
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

func (info *fileInfo) ModTime() time.Time {
	gtime := info.ModificationDateTime()
	return time.UnixMicro(gtime.ToUnixUsec())
}

func (info *fileInfo) IsDir() bool {
	return info.FileType() == gio.FileTypeDirectory
}

func (info *fileInfo) Sys() any {
	return info.FileInfo
}
