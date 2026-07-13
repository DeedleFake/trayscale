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

func (a *App) saveFile(ctx context.Context, name string, file gio.Filer) error {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("path", file.Path(), "filename", name)
	slog.Info("starting file save")

	r, size, err := tsutil.GetWaitingFile(ctx, name)
	if err != nil {
		slog.Error("get file", "err", err)
		return err
	}
	defer r.Close()

	s, err := file.Replace(ctx, "", false, gio.FileCreateNone)
	if err != nil {
		slog.Error("create file", "err", err)
		return err
	}

	w := gioutil.Writer(ctx, s)
	_, err = io.CopyN(w, r, size)
	if err != nil {
		slog.Error("write file", "err", err)
		return err
	}

	err = tsutil.DeleteWaitingFile(ctx, name)
	if err != nil {
		slog.Error("delete file", "err", err)
		return err
	}

	<-a.poller.Poll()
	slog.Info("done saving file")
	return nil
}

func (a *App) autoSaveSettings() (enabled bool, dir string) {
	if a.settings == nil {
		return false, ""
	}
	return a.settings.Boolean("taildrop-auto-save"), a.settings.String("taildrop-auto-save-dir")
}

// clearAutoSaveFailures drops remembered auto-save failures so waiting
// files can be tried again (for example after the destination directory
// is fixed or the user reconfigures auto-save).
func (a *App) clearAutoSaveFailures() {
	a.autoSaveFailed.Range(func(key, _ any) bool {
		a.autoSaveFailed.Delete(key)
		return true
	})
	a.autoSaveDirBad = ""
}

// maybeAutoSaveFiles saves any waiting files when auto-save is enabled
// and a destination directory is configured. Safe to call from the GTK
// thread; work runs in background goroutines.
//
// A missing destination directory is logged once until the path becomes
// usable again. Individual save failures are remembered so the same
// waiting file is not retried on every poll (which would spam the log).
func (a *App) maybeAutoSaveFiles() {
	if a.files == nil {
		return
	}

	enabled, dir := a.autoSaveSettings()
	if !AutoSaveEnabled(enabled, dir) {
		return
	}

	if err := AutoSaveDirOK(dir); err != nil {
		// Do not mark individual files failed: when the directory is
		// recreated, the next status update should resume saving.
		if a.autoSaveDirBad != dir {
			a.autoSaveDirBad = dir
			slog.Error("taildrop auto-save directory unavailable", "dir", dir, "err", err)
		}
		return
	}
	a.autoSaveDirBad = ""

	waiting := make(map[string]bool, len(*a.files))
	for _, f := range *a.files {
		waiting[f.Name] = true
	}
	// Drop failure memory for files that are no longer waiting so a
	// later re-transfer of the same name can be auto-saved again.
	a.autoSaveFailed.Range(func(key, _ any) bool {
		if name, ok := key.(string); ok && !waiting[name] {
			a.autoSaveFailed.Delete(name)
		}
		return true
	})

	skip := make(map[string]bool)
	a.autoSaving.Range(func(key, _ any) bool {
		if name, ok := key.(string); ok {
			skip[name] = true
		}
		return true
	})
	a.autoSaveFailed.Range(func(key, _ any) bool {
		if name, ok := key.(string); ok {
			skip[name] = true
		}
		return true
	})

	for _, name := range FilesToAutoSave(enabled, dir, *a.files, skip) {
		if _, loaded := a.autoSaving.LoadOrStore(name, struct{}{}); loaded {
			continue
		}

		dest := AutoSavePath(dir, name)
		go func(name, dest string) {
			defer a.autoSaving.Delete(name)
			err := a.saveFile(context.Background(), name, gio.NewFileForPath(dest))
			if err != nil {
				a.autoSaveFailed.Store(name, struct{}{})
			}
		}(name, dest)
	}
}
