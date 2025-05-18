package ui

import (
	"context"
	"io"
	"log/slog"

	"deedles.dev/trayscale/internal/giofs"
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

	r, size, name, err := giofs.Reader(ctx, file)
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
