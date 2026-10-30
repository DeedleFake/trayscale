package ui

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/autosave"
	"deedles.dev/trayscale/internal/giofs"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/tailcfg"
)

func (a *App) pushFile(ctx context.Context, peerID tailcfg.StableNodeID, file *gio.FileBase) {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("peer", peerID, "path", gutil.FilePath(file))
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

func (a *App) saveFile(ctx context.Context, name string, file *gio.FileBase) error {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("path", gutil.FilePath(file), "filename", name)
	slog.Info("starting file save")

	r, size, err := tsutil.GetWaitingFile(ctx, name)
	if err != nil {
		slog.Error("get file", "err", err)
		return err
	}
	defer r.Close()

	// Replace writes to a temporary name and only renames over the
	// destination when the stream is closed. Leaving it open leaves
	// hidden .goutputstream-* files and never updates an existing target.
	c, release := giofs.Cancellable(ctx)
	s, err := file.Replace("", false, gio.GFileCreateNoneValue, c)
	release()
	if err = gutil.Err(err); err != nil {
		slog.Error("create file", "err", err)
		return err
	}

	w := giofs.NewWriter(ctx, &s.OutputStream)
	_, copyErr := io.CopyN(w, r, size)
	closeErr := w.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		slog.Error("write file", "err", err)
		return err
	}

	err = a.files.Delete(ctx, name)
	if err != nil {
		slog.Error("delete file", "err", err)
		return err
	}

	slog.Info("done saving file")
	return nil
}

func (a *App) autoSaveSettings() (enabled bool, dir string) {
	if a.settings == nil {
		return false, ""
	}
	return a.settings.GetBoolean("taildrop-auto-save"), gutil.SettingsString(a.settings, "taildrop-auto-save-dir")
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
	if a.waiting == nil {
		return
	}

	enabled, dir := a.autoSaveSettings()
	if !autosave.Enabled(enabled, dir) {
		return
	}

	if err := autosave.DirOK(dir); err != nil {
		// Do not mark individual files failed: when the directory is
		// recreated, the next status update should resume saving.
		if a.autoSaveDirBad != dir {
			a.autoSaveDirBad = dir
			slog.Error("taildrop auto-save directory unavailable", "dir", dir, "err", err)
		}
		return
	}
	a.autoSaveDirBad = ""

	waiting := make(map[string]bool, len(*a.waiting))
	for _, f := range *a.waiting {
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

	for _, name := range autosave.Files(enabled, dir, *a.waiting, skip) {
		if _, loaded := a.autoSaving.LoadOrStore(name, struct{}{}); loaded {
			continue
		}

		// Avoid overwriting an existing file; pick "name (1).ext", etc.
		dest := autosave.UniquePath(dir, name)
		go func(name, dest string) {
			defer a.autoSaving.Delete(name)
			f := gio.FileNewForPath(dest)
			err := a.saveFile(context.Background(), name, f)
			gutil.IdleAdd(gutil.Obj(f).Unref)
			if err != nil {
				a.autoSaveFailed.Store(name, struct{}{})
			}
		}(name, dest)
	}
}
