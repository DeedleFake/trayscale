package giofs

import (
	"context"
	"io"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/gutil"
)

// Cancellable returns a GCancellable that is cancelled when ctx is
// done, and a function that releases it. Call release when the
// operations using it are finished.
func Cancellable(ctx context.Context) (c *gio.Cancellable, release func()) {
	c = gio.NewCancellable()
	// The AfterFunc goroutine holds its own reference so that release
	// can't free c while Cancel is still running on it.
	c.Ref()
	stop := context.AfterFunc(ctx, func() {
		defer c.Unref()
		c.Cancel()
	})
	return c, func() {
		if stop() {
			c.Unref() // AfterFunc will never run; drop its reference.
		}
		c.Unref()
	}
}

// StreamReader is an io.ReadCloser over a GInputStream. It owns the
// stream: Close closes and releases it.
type StreamReader struct {
	s       *gio.InputStream
	c       *gio.Cancellable
	release func()
}

// NewReader returns a reader that takes over s.
func NewReader(ctx context.Context, s *gio.InputStream) *StreamReader {
	c, release := Cancellable(ctx)
	return &StreamReader{s: s, c: c, release: release}
}

func (r *StreamReader) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	// InputStream.Read's buffer parameter is generated as *[]byte,
	// which passes the address of the slice header rather than its
	// data, so read into a GBytes and copy instead.
	b, err := r.s.ReadBytes(uint(len(buf)), r.c)
	err = gutil.Err(err)
	if err != nil {
		return 0, err
	}
	defer b.Unref()

	var size uint
	p := b.GetData(&size)
	if size == 0 {
		return 0, io.EOF
	}
	data := unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), size)
	return copy(buf, data), nil
}

func (r *StreamReader) Close() error {
	_, err := r.s.Close(r.c)
	err = gutil.Err(err)
	r.s.Unref()
	r.release()
	return err
}

// StreamWriter is an io.WriteCloser over a GOutputStream. It owns the
// stream: Close closes and releases it. For a stream from
// File.Replace, closing is what renames the result into place.
type StreamWriter struct {
	s       *gio.OutputStream
	c       *gio.Cancellable
	release func()
}

// NewWriter returns a writer that takes over s.
func NewWriter(ctx context.Context, s *gio.OutputStream) *StreamWriter {
	c, release := Cancellable(ctx)
	return &StreamWriter{s: s, c: c, release: release}
}

func (w *StreamWriter) Write(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	var n uint
	_, err := w.s.WriteAll(buf, uint(len(buf)), &n, w.c)
	err = gutil.Err(err)
	if err == nil && int(n) < len(buf) {
		err = io.ErrShortWrite
	}
	return int(n), err
}

func (w *StreamWriter) Close() error {
	_, err := w.s.Close(w.c)
	err = gutil.Err(err)
	w.s.Unref()
	w.release()
	return err
}
