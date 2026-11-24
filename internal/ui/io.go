package ui

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/autosave"
	"deedles.dev/trayscale/internal/giofs"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/tsutil"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
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

// saveFile saves profile's waiting file to dest and then deletes it
// from the waiting files. It fails with tsutil.ErrProfileChanged,
// saving nothing, if profile isn't current when it opens the file. If
// profile isn't current when it deletes the file, it leaves it waiting
// for retryPendingDeletes, tells the user, and succeeds.
func (a *App) saveFile(ctx context.Context, profile ipn.ProfileID, file apitype.WaitingFile, dest *gio.FileBase) error {
	a.spin()
	defer a.stopSpin()

	slog := slog.With("path", gutil.FilePath(dest), "filename", file.Name)
	slog.Info("starting file save")

	r, size, err := a.files.Open(ctx, profile, file.Name)
	if err != nil {
		slog.Error("get file", "err", err)
		return err
	}
	defer r.Close()

	// Replace writes to a temporary name and only renames over the
	// destination when the stream is closed. Leaving it open leaves
	// hidden .goutputstream-* files and never updates an existing target.
	c, release := giofs.Cancellable(ctx)
	s, err := dest.Replace("", false, gio.GFileCreateNoneValue, c)
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

	err = a.deleteSaved(ctx, profile, file)
	if errors.Is(err, tsutil.ErrProfileChanged) {
		slog.Info("saved a file of another profile, so it stays waiting until that profile is current")
		a.toast(locale.Get("File saved. It will leave the incoming files the next time its account is active"))
		return nil
	}
	if err != nil {
		slog.Error("delete file", "err", err)
		return err
	}

	slog.Info("done saving file")
	return nil
}

// deleteSaved deletes profile's waiting file once it's been saved. If
// profile isn't current, it fails with tsutil.ErrProfileChanged and
// adds the file to a.pending, so that retryPendingDeletes deletes it
// once profile is current again. It's safe to call from any thread.
func (a *App) deleteSaved(ctx context.Context, profile ipn.ProfileID, file apitype.WaitingFile) error {
	err := a.files.Delete(ctx, profile, file.Name)
	if errors.Is(err, tsutil.ErrProfileChanged) {
		gutil.IdleAdd(func() { a.pending.add(profile, file) })
	}
	return err
}

// retryPendingDeletes deletes the files in a.pending that were saved
// from status's profile while another one was current, if they're
// still waiting with the same name and size. It runs on the main
// thread and deletes in the background.
func (a *App) retryPendingDeletes(status *tsutil.FileStatus) {
	for _, file := range a.pending.take(status.Profile, status.Files) {
		// Taken by the name, so that maybeAutoSaveFiles doesn't save
		// it again.
		if _, loaded := a.autoSaving.LoadOrStore(file.Name, struct{}{}); loaded {
			a.pending.add(status.Profile, file)
			continue
		}
		go func() {
			defer a.autoSaving.Delete(file.Name)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := a.deleteSaved(ctx, status.Profile, file)
			if err != nil && !errors.Is(err, tsutil.ErrProfileChanged) {
				slog.Error("delete saved file", "filename", file.Name, "err", err)
			}
		}()
	}
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

	waiting := make(map[string]apitype.WaitingFile, len(a.waiting.Files))
	for _, f := range a.waiting.Files {
		waiting[f.Name] = f
	}
	// Drop failure memory for files that are no longer waiting so a
	// later re-transfer of the same name can be auto-saved again.
	a.autoSaveFailed.Range(func(key, _ any) bool {
		if _, ok := waiting[key.(string)]; !ok {
			a.autoSaveFailed.Delete(key)
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

	profile := a.waiting.Profile
	for _, name := range autosave.Files(enabled, dir, a.waiting.Files, skip) {
		if _, loaded := a.autoSaving.LoadOrStore(name, struct{}{}); loaded {
			continue
		}

		// Avoid overwriting an existing file; pick "name (1).ext", etc.
		dest := autosave.UniquePath(dir, name)
		go func(name, dest string) {
			defer a.autoSaving.Delete(name)
			f := gio.FileNewForPath(dest)
			err := a.saveFile(context.Background(), profile, waiting[name], f)
			gutil.IdleAdd(gutil.Obj(f).Unref)
			// A file of a profile that isn't current anymore is saved
			// once its profile's list arrives again.
			if err != nil && !errors.Is(err, tsutil.ErrProfileChanged) {
				a.autoSaveFailed.Store(name, struct{}{})
			}
		}(name, dest)
	}
}
