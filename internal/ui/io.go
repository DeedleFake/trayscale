package ui

import (
	"context"
	"io"
	"log/slog"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"tailscale.com/tailcfg"
)

func (a *App) pushFile(ctx context.Context, peerID tailcfg.StableNodeID, file *gio.File) {
	slog := slog.With("path", file.Path())
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

	r := NewGReader(ctx, s)
	err = a.TS.PushFile(ctx, peerID, info.Size(), file.Basename(), r)
	if err != nil {
		slog.Error("push file", "err", err)
		return
	}

	slog.Info("done pushing file")
}

func (a *App) saveFile(ctx context.Context, name string, file *gio.File) {
	slog := slog.With("path", file.Path(), "filename", name)
	slog.Info("starting file save")

	r, size, err := a.TS.GetWaitingFile(ctx, name)
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

	w := NewGWriter(ctx, s)
	_, err = io.CopyN(w, r, size)
	if err != nil {
		slog.Error("write file", "err", err)
		return
	}

	err = a.TS.DeleteWaitingFile(ctx, name)
	if err != nil {
		slog.Error("delete file", "err", err)
		return
	}

	a.poller.Poll() <- struct{}{}
	slog.Info("done saving file")
}

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
	return r.s.Read(r.ctx, buf)
}
