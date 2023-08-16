package ui

import (
	"context"
	"io"
)

type GOutputStream interface {
	Write(context.Context, []byte) (int, error)
}

type gwriter struct {
	ctx context.Context
	s   GOutputStream
}

func NewGWriter(ctx context.Context, s GOutputStream) io.Writer {
	return gwriter{ctx, s}
}

func (w gwriter) Write(data []byte) (int, error) {
	// TODO: Make this async and probably add a progress bar to the UI.
	return w.s.Write(w.ctx, data)
}

type GInputStream interface {
	Read(context.Context, []byte) (int, error)
}

type greader struct {
	ctx context.Context
	s   GInputStream
}

func NewGReader(ctx context.Context, s GInputStream) io.Reader {
	return greader{ctx, s}
}

func (r greader) Read(buf []byte) (int, error) {
	// TODO: Make this async and probably add a progress bar to the UI.
	return r.s.Read(r.ctx, buf)
}
