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

	<-a.poller.Poll()
	slog.Info("done saving file")
}

func (a *App) autoSaveSettings() (enabled bool, dir string) {
	if a.settings == nil {
		return false, ""
	}
	return a.settings.Boolean("taildrop-auto-save"), a.settings.String("taildrop-auto-save-dir")
}

// maybeAutoSaveFiles saves any waiting files when auto-save is enabled
// and a destination directory is configured. Safe to call from the GTK
// thread; work runs in background goroutines.
func (a *App) maybeAutoSaveFiles() {
	if a.files == nil {
		return
	}

	enabled, dir := a.autoSaveSettings()
	inFlight := make(map[string]bool)
	a.autoSaving.Range(func(key, _ any) bool {
		if name, ok := key.(string); ok {
			inFlight[name] = true
		}
		return true
	})

	for _, name := range FilesToAutoSave(enabled, dir, *a.files, inFlight) {
		if _, loaded := a.autoSaving.LoadOrStore(name, struct{}{}); loaded {
			continue
		}

		dest := AutoSavePath(dir, name)
		go func(name, dest string) {
			defer a.autoSaving.Delete(name)
			a.saveFile(context.Background(), name, gio.NewFileForPath(dest))
		}(name, dest)
	}
}
